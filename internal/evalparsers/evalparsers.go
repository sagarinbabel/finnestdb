package evalparsers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/parserffi"
	"finnestdb/internal/store"
)

const omorfiCommandEnv = "FINNESTDB_OMORFI_CMD"
const estnltkCommandEnv = "FINNESTDB_ESTNLTK_CMD"

// External-analyzer subprocess timeouts. Both can be overridden with a Go
// duration string ("30s", "1m"). EstNLTK defaults higher because each call
// pays ~1s of Vabamorf model load before any analysis runs.
const omorfiTimeoutEnv = "FINNESTDB_OMORFI_TIMEOUT"
const estnltkTimeoutEnv = "FINNESTDB_ESTNLTK_TIMEOUT"
const omorfiDefaultTimeout = 5 * time.Second
const estnltkDefaultTimeout = 30 * time.Second

func SupportedParsers() []string {
	names := parsecore.SupportedParsers()
	names = append(names, "estnltk", "omorfi")
	sort.Strings(names)
	return names
}

func Analyze(db *store.DB, lang, text, parserName string) (*parsecore.ParseResult, error) {
	runner := NewRunner()
	defer func() {
		_ = runner.Close()
	}()
	return runner.Analyze(db, lang, text, parserName)
}

func externalAnalyzerConfig(name string, analyze parsecore.AnalyzerFunc) parsecore.ExternalAnalyzerConfig {
	switch name {
	case "omorfi":
		return parsecore.ExternalAnalyzerConfig{
			Name:    "omorfi",
			Lang:    "FI",
			Source:  "analyzer:omorfi",
			Analyze: analyze,
			Rules:   defaultExternalAnalyzerRules(),
		}
	case "estnltk":
		return parsecore.ExternalAnalyzerConfig{
			Name:    "estnltk",
			Lang:    "ET",
			Source:  "analyzer:estnltk",
			Analyze: analyze,
			Rules:   defaultExternalAnalyzerRules(),
		}
	default:
		return parsecore.ExternalAnalyzerConfig{Name: name, Analyze: analyze}
	}
}

// analyzerTimeout reads a Go duration string from envVar and returns it,
// falling back to defaultDur on empty, malformed, or non-positive input.
func analyzerTimeout(envVar string, defaultDur time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(envVar))
	if raw == "" {
		return defaultDur
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return defaultDur
	}
	return parsed
}

type externalPreferDirectDictRule struct{}

func (externalPreferDirectDictRule) Name() string { return "prefer_direct_dict_when_unknown" }

func (externalPreferDirectDictRule) Apply(_ string, token *parsecore.TokenResult, direct, _ store.FormResolution) bool {
	if direct.Lemma == "" {
		return false
	}
	if token.Resolved && token.POS != "X" {
		return false
	}
	token.Trace = append(token.Trace, fmt.Sprintf("rule:direct_dict lemma=%s pos=%s", direct.Lemma, direct.POS))
	token.Lemma = direct.Lemma
	token.POS = direct.POS
	if direct.GrammarLabel != "" {
		token.GrammarLabel = direct.GrammarLabel
	}
	if direct.Feats != "" {
		token.Feats = direct.Feats
	}
	token.Source = "override:direct_dict"
	token.Resolved = true
	return true
}

type externalPreferCustomFallbackRule struct{}

func (externalPreferCustomFallbackRule) Name() string { return "prefer_custom_fallback_when_unknown" }

func (externalPreferCustomFallbackRule) Apply(_ string, token *parsecore.TokenResult, _, custom store.FormResolution) bool {
	if custom.Lemma == "" {
		return false
	}
	if token.Resolved && token.POS != "X" {
		return false
	}
	token.Trace = append(token.Trace, fmt.Sprintf("rule:custom_fallback lemma=%s pos=%s source=%s", custom.Lemma, custom.POS, custom.Source))
	token.Lemma = custom.Lemma
	token.POS = custom.POS
	token.GrammarLabel = custom.GrammarLabel
	token.Feats = custom.Feats
	token.Source = "override:" + custom.Source
	token.Resolved = true
	return true
}

type externalAttachMorphologyRule struct{}

func (externalAttachMorphologyRule) Name() string { return "attach_custom_morphology" }

// Apply attaches custom GrammarLabel and/or Feats to an already-resolved
// analyzer token when the analyzer has no morphology of its own and lemma/POS
// agree. Fires for label-only customs (legacy case-suffix path), feats-only
// customs (FST verb morphology like Number/Tense/Mood/Person — no case label),
// and the both-present case. The earlier label-only gate dropped FEATS-only
// FST analyses on the floor when omorfi/estnltk had the lemma but no FEATS.
func (externalAttachMorphologyRule) Apply(_ string, token *parsecore.TokenResult, _, custom store.FormResolution) bool {
	tokenNeedsLabel := token.GrammarLabel == "" && custom.GrammarLabel != ""
	tokenNeedsFeats := token.Feats == "" && custom.Feats != ""
	if !tokenNeedsLabel && !tokenNeedsFeats {
		return false
	}
	if custom.Lemma != "" && token.Lemma != custom.Lemma {
		return false
	}
	if custom.POS != "" && token.POS != custom.POS {
		return false
	}
	traceParts := make([]string, 0, 2)
	if tokenNeedsLabel {
		token.GrammarLabel = custom.GrammarLabel
		traceParts = append(traceParts, "label="+custom.GrammarLabel)
	}
	if tokenNeedsFeats {
		token.Feats = custom.Feats
		traceParts = append(traceParts, "feats="+custom.Feats)
	}
	token.Trace = append(token.Trace, "rule:attach_morphology "+strings.Join(traceParts, " "))
	return true
}

func defaultExternalAnalyzerRules() []parsecore.ExternalAnalyzerRule {
	return []parsecore.ExternalAnalyzerRule{
		externalPreferDirectDictRule{},
		externalPreferCustomFallbackRule{},
		externalAttachMorphologyRule{},
	}
}

func resolveOmorfiCommandSpec() (string, error) {
	cmdSpec := strings.TrimSpace(os.Getenv(omorfiCommandEnv))
	if cmdSpec == "" {
		// Auto-default: when the bundled adapter script and python3 are
		// available, run them directly. Avoids requiring a per-shell env var
		// for the common dev-environment case after `make setup-omorfi`.
		//
		// Search order for the adapter script (cwd-independent — covers
		// `go run` from the repo root, installed binaries, and systemd):
		//   1. ./scripts/omorfi_adapter_example.py (cwd is the repo root)
		//   2. <repo>/scripts/omorfi_adapter_example.py where <repo> is
		//      walked up from the test executable / cwd looking for go.mod
		//   3. <executable-dir>/scripts/omorfi_adapter_example.py
		//
		// When a sibling .venv/bin/python exists (created by
		// `make setup-nlp`), prefer it over the system python3. Falls back
		// to the legacy .venv-omorfi/ name for backward compat. This is the
		// canonical install path on macOS (system python3 hits PEP 668).
		if py, err := exec.LookPath("python3"); err == nil {
			if path, ok := findOmorfiAdapter(); ok {
				if venvPy, ok := findSiblingVenvPython(path, ".venv"); ok {
					py = venvPy
				} else if venvPy, ok := findSiblingVenvPython(path, ".venv-omorfi"); ok {
					py = venvPy
				}
				cmdSpec = py + " " + path
			}
		}
	}
	if cmdSpec == "" {
		return "", fmt.Errorf("omorfi parser is not configured; set %s or run `make setup-nlp`", omorfiCommandEnv)
	}
	return cmdSpec, nil
}

func resolveEstNLTKCommandSpec() (string, error) {
	cmdSpec := strings.TrimSpace(os.Getenv(estnltkCommandEnv))
	if cmdSpec == "" {
		if py, err := exec.LookPath("python3"); err == nil {
			if path, ok := findEstNLTKAdapter(); ok {
				if venvPy, ok := findSiblingVenvPython(path, ".venv"); ok {
					py = venvPy
				} else if venvPy, ok := findSiblingVenvPython(path, ".venv-estnltk"); ok {
					py = venvPy
				}
				cmdSpec = py + " " + path
			}
		}
	}
	if cmdSpec == "" {
		return "", fmt.Errorf("estnltk parser is not configured; set %s or run `make setup-nlp`", estnltkCommandEnv)
	}
	return cmdSpec, nil
}

func runExternalCommand(cmdSpec, lang, text, name, timeoutEnv string, defaultTimeout time.Duration) (*parserffi.AnalysisResult, error) {
	fields := strings.Fields(cmdSpec)
	if len(fields) == 0 {
		return nil, fmt.Errorf("%s parser command is empty", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), analyzerTimeout(timeoutEnv, defaultTimeout))
	defer cancel()

	args := append(fields[1:], "--lang", lang)
	cmd := exec.CommandContext(ctx, fields[0], args...)
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("%s parser timed out", name)
	}
	if err != nil {
		return nil, fmt.Errorf("%s parser failed: %w", name, err)
	}

	var result parserffi.AnalysisResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("%s parser returned invalid JSON: %w", name, err)
	}
	return &result, nil
}

func findEstNLTKAdapter() (string, bool) {
	return findRepoScript("scripts/estnltk_adapter_example.py")
}

// findOmorfiAdapter locates the bundled python adapter script in a way that
// works whether the caller's cwd is the repo root, a sub-package directory
// (`go test ./internal/evalparsers`), or an installed-binary deployment.
//
// Returns the absolute path to the script and true on success.
func findOmorfiAdapter() (string, bool) {
	return findRepoScript("scripts/omorfi_adapter_example.py")
}

func findRepoScript(scriptRel string) (string, bool) {
	// 1. cwd-relative.
	if abs, err := filepath.Abs(scriptRel); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs, true
		}
	}

	// 2. Walk up from cwd looking for go.mod (repo root).
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 8; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				candidate := filepath.Join(dir, scriptRel)
				if _, err := os.Stat(candidate); err == nil {
					return candidate, true
				}
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 3. Same directory as the running executable.
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), scriptRel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}

	return "", false
}

func findSiblingVenvPython(scriptPath, venvName string) (string, bool) {
	dir := filepath.Dir(scriptPath)
	for i := 0; i < 4; i++ {
		if filepath.Base(dir) == "scripts" {
			repoRoot := filepath.Dir(dir)
			candidate := filepath.Join(repoRoot, venvName, "bin", "python")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, true
			}
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}
