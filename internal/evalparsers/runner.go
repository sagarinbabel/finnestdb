package evalparsers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/parserffi"
	"finnestdb/internal/store"
)

// Runner owns per-evaluation parser state. External analyzer baselines are
// expensive to start, so Runner keeps one adapter process alive per
// parser/language/command for the lifetime of a parsertest run.
type Runner struct {
	mu        sync.Mutex
	processes map[externalProcessKey]*persistentExternalCommand
	oneShot   map[externalProcessKey]struct{}
}

type externalProcessKey struct {
	name    string
	lang    string
	cmdSpec string
}

func NewRunner() *Runner {
	return &Runner{
		processes: make(map[externalProcessKey]*persistentExternalCommand),
		oneShot:   make(map[externalProcessKey]struct{}),
	}
}

func (r *Runner) Analyze(db *store.DB, lang, text, parserName string) (*parsecore.ParseResult, error) {
	if parserName == "" {
		parserName = "basic"
	}
	switch parserName {
	case "basic", "custom":
		return parsecore.Analyze(db, lang, text, parserName)
	case "omorfi":
		return parsecore.AnalyzeWithExternalAnalyzer(db, lang, text, externalAnalyzerConfig("omorfi", r.runExternalOmorfi))
	case "estnltk":
		return parsecore.AnalyzeWithExternalAnalyzer(db, lang, text, externalAnalyzerConfig("estnltk", r.runExternalEstNLTK))
	default:
		if err := parsecore.ValidateInput(lang, text); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unsupported parser %q", parserName)
	}
}

func (r *Runner) Close() error {
	r.mu.Lock()
	processes := make([]*persistentExternalCommand, 0, len(r.processes))
	for _, proc := range r.processes {
		processes = append(processes, proc)
	}
	r.processes = make(map[externalProcessKey]*persistentExternalCommand)
	r.mu.Unlock()

	var errs []error
	for _, proc := range processes {
		if err := proc.close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Runner) runExternalOmorfi(lang, text string) (*parserffi.AnalysisResult, error) {
	cmdSpec, err := resolveOmorfiCommandSpec()
	if err != nil {
		return nil, err
	}
	return r.runExternalCommand(cmdSpec, lang, text, "omorfi", omorfiTimeoutEnv, omorfiDefaultTimeout)
}

func (r *Runner) runExternalEstNLTK(lang, text string) (*parserffi.AnalysisResult, error) {
	cmdSpec, err := resolveEstNLTKCommandSpec()
	if err != nil {
		return nil, err
	}
	return r.runExternalCommand(cmdSpec, lang, text, "estnltk", estnltkTimeoutEnv, estnltkDefaultTimeout)
}

func (r *Runner) runExternalCommand(cmdSpec, lang, text, name, timeoutEnv string, defaultTimeout time.Duration) (*parserffi.AnalysisResult, error) {
	key := externalProcessKey{name: name, lang: lang, cmdSpec: cmdSpec}
	if r.usesOneShot(key) {
		return runExternalCommand(cmdSpec, lang, text, name, timeoutEnv, defaultTimeout)
	}
	proc, err := r.process(key)
	if err != nil {
		return nil, err
	}
	result, err := proc.analyze(text, analyzerTimeout(timeoutEnv, defaultTimeout))
	if err != nil {
		r.dropProcess(key, proc)
		if isUnsupportedServerMode(err) {
			r.markOneShot(key)
			return runExternalCommand(cmdSpec, lang, text, name, timeoutEnv, defaultTimeout)
		}
		return nil, err
	}
	return result, nil
}

func (r *Runner) usesOneShot(key externalProcessKey) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.oneShot[key]
	return ok
}

func (r *Runner) markOneShot(key externalProcessKey) {
	r.mu.Lock()
	r.oneShot[key] = struct{}{}
	r.mu.Unlock()
}

func (r *Runner) process(key externalProcessKey) (*persistentExternalCommand, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if proc := r.processes[key]; proc != nil {
		return proc, nil
	}
	proc, err := startPersistentExternalCommand(key)
	if err != nil {
		return nil, err
	}
	r.processes[key] = proc
	return proc, nil
}

func (r *Runner) dropProcess(key externalProcessKey, proc *persistentExternalCommand) {
	r.mu.Lock()
	if r.processes[key] == proc {
		delete(r.processes, key)
	}
	r.mu.Unlock()
	_ = proc.close()
}

type persistentExternalCommand struct {
	name   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *lockedBuffer
	done   chan error
	mu     sync.Mutex
}

type analyzerServerRequest struct {
	Text string `json:"text"`
}

type analyzerServerResponse struct {
	Sentences []parserffi.Sentence `json:"sentences,omitempty"`
	Error     string               `json:"error,omitempty"`
}

func startPersistentExternalCommand(key externalProcessKey) (*persistentExternalCommand, error) {
	fields := strings.Fields(key.cmdSpec)
	if len(fields) == 0 {
		return nil, fmt.Errorf("%s parser command is empty", key.name)
	}

	args := make([]string, 0, len(fields)+2)
	args = append(args, fields[1:]...)
	args = append(args, "--lang", key.lang, "--server")
	cmd := exec.Command(fields[0], args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("%s parser server stdin: %w", key.name, err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%s parser server stdout: %w", key.name, err)
	}
	stderr := &lockedBuffer{max: 16 << 10}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s parser server failed to start: %w", key.name, err)
	}

	proc := &persistentExternalCommand{
		name:   key.name,
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
		stderr: stderr,
		done:   make(chan error, 1),
	}
	go func() {
		proc.done <- cmd.Wait()
	}()
	return proc, nil
}

func (p *persistentExternalCommand) analyze(text string, timeout time.Duration) (*parserffi.AnalysisResult, error) {
	type response struct {
		result *parserffi.AnalysisResult
		err    error
	}
	done := make(chan response, 1)
	go func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		result, err := p.analyzeLocked(text)
		done <- response{result: result, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case resp := <-done:
		return resp.result, resp.err
	case <-timer.C:
		_ = p.kill()
		return nil, fmt.Errorf("%s parser timed out", p.name)
	}
}

func (p *persistentExternalCommand) analyzeLocked(text string) (*parserffi.AnalysisResult, error) {
	if err := json.NewEncoder(p.stdin).Encode(analyzerServerRequest{Text: text}); err != nil {
		return nil, p.withContext("write request", err)
	}

	line, err := p.stdout.ReadBytes('\n')
	if err != nil {
		return nil, p.withContext("read response", err)
	}

	var response analyzerServerResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return nil, fmt.Errorf("%s parser server returned invalid JSON: %w", p.name, err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("%s parser failed: %s", p.name, response.Error)
	}
	return &parserffi.AnalysisResult{Sentences: response.Sentences}, nil
}

func (p *persistentExternalCommand) withContext(action string, err error) error {
	// When the child died (typical when its stdin/stdout pipes return EOF),
	// `cmd.Wait()` in our goroutine drains and closes the stderr pipe before
	// it publishes on `done`. We need that drain to finish before we read
	// `p.stderr` - otherwise the fallback heuristic in isUnsupportedServerMode
	// gets a bare "read response: EOF" with no "unrecognized arguments"
	// detail, and the legacy one-shot fallback never fires.
	//
	// A 1-second cap keeps the test reliable on slow CI without making a hung
	// child block the caller forever.
	var waitErr error
	waited := false
	select {
	case waitErr = <-p.done:
		waited = true
	case <-time.After(time.Second):
	}
	detail := p.stderr.String()
	if waited {
		p.done <- waitErr
		if waitErr != nil {
			if detail != "" {
				return fmt.Errorf("%s parser server %s: %w: %v: %s", p.name, action, err, waitErr, detail)
			}
			return fmt.Errorf("%s parser server %s: %w: %v", p.name, action, err, waitErr)
		}
	}
	if detail != "" {
		return fmt.Errorf("%s parser server %s: %w: %s", p.name, action, err, detail)
	}
	return fmt.Errorf("%s parser server %s: %w", p.name, action, err)
}

func (p *persistentExternalCommand) close() error {
	_ = p.stdin.Close()
	select {
	case err := <-p.done:
		return err
	case <-time.After(2 * time.Second):
		return p.kill()
	}
}

func (p *persistentExternalCommand) kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func isUnsupportedServerMode(err error) bool {
	// Compatibility fallback is intentionally best-effort: only familiar
	// unknown-flag messages mean "try the legacy one-shot adapter contract."
	// Other startup/read failures should stay loud instead of being guessed.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unrecognized arguments: --server") ||
		strings.Contains(msg, "no such option: --server") ||
		strings.Contains(msg, "unknown option --server") ||
		strings.Contains(msg, "flag provided but not defined: -server")
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.max > 0 && b.buf.Len()+len(p) > b.max {
		if len(p) >= b.max {
			b.buf.Reset()
			_, _ = b.buf.Write(p[len(p)-b.max:])
			return len(p), nil
		}
		b.buf.Next(b.buf.Len() + len(p) - b.max)
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.buf.String())
}
