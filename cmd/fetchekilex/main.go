// cmd/fetchekilex fetches Ekilex /api/word/details payloads for every Estonian
// public headword in data/ekilex/eki-public-words-2026-et.jsonl.
//
// Subcommands:
//
//	refresh-queue  Re-fetch /api/public_word/eki and overwrite the queue file
//	               only if the headword set changed.
//	sample         Fetch a small set of words for inspection, optionally
//	               comparing dataset-filter variants (e.g. "eki" vs no filter).
//	fetch          Resumable multi-worker fetch of /api/word/details for every
//	               queued word_id, writing gzipped raw JSON under
//	               localdata/ekilex/details/raw/ and tracking progress in a
//	               local SQLite checkpoint.
//
// All API calls require EKILEX_API_KEY in the environment. The key is sent as
// the `ekilex-api-key` header.
package main

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// publicWord is the on-disk snapshot format. Kept stable so cmd/importekilex
// continues to read it without changes.
type publicWord struct {
	WordID        int64  `json:"word_id"`
	Lemma         string `json:"lemma"`
	Lang          string `json:"lang"`
	SourceDataset string `json:"source_dataset"`
	MorphExists   bool   `json:"morph_exists"`
}

// apiPublicWord matches the live /api/public_word/{datasetCode} response,
// which uses camelCase and includes non-Estonian entries (the dataset is
// multilingual).
type apiPublicWord struct {
	WordID      int64  `json:"wordId"`
	Value       string `json:"value"`
	Lang        string `json:"lang"`
	MorphExists bool   `json:"morphExists"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "refresh-queue":
		err = cmdRefreshQueue(args)
	case "sample":
		err = cmdSample(args)
	case "fetch":
		err = cmdFetch(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: fetchekilex <subcommand> [flags]

subcommands:
  refresh-queue   Refresh the local queue file from /api/public_word/eki.
  sample          Fetch a few words and compare dataset-filter variants.
  fetch           Multi-worker resumable fetch of /api/word/details.

env:
  EKILEX_API_KEY  required (sent as ekilex-api-key header)`)
}

// HTTP -----------------------------------------------------------------------

type apiClient struct {
	base   string
	key    string
	client *http.Client
}

func newAPIClient(base, key string) *apiClient {
	return &apiClient{
		base:   strings.TrimRight(base, "/"),
		key:    key,
		client: &http.Client{}, // no client-level timeout; callers use context deadlines
	}
}

type fetchResult struct {
	Status     int
	Body       []byte
	RetryAfter time.Duration
}

func (c *apiClient) get(ctx context.Context, path string) (fetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+path, nil)
	if err != nil {
		return fetchResult{}, err
	}
	req.Header.Set("ekilex-api-key", c.key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fetchResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fetchResult{Status: resp.StatusCode}, err
	}
	var ra time.Duration
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			ra = time.Duration(secs) * time.Second
		}
	}
	return fetchResult{Status: resp.StatusCode, Body: body, RetryAfter: ra}, nil
}

func (c *apiClient) wordDetails(ctx context.Context, wordID int64, datasets string) (fetchResult, error) {
	p := "/api/word/details/" + strconv.FormatInt(wordID, 10)
	if datasets != "" {
		p += "/" + url.PathEscape(datasets)
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return c.get(ctx, p)
}

func (c *apiClient) publicWords(ctx context.Context, datasetCode string) ([]publicWord, error) {
	res, err := c.get(ctx, "/api/public_word/"+url.PathEscape(datasetCode))
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, fmt.Errorf("public_word/%s: HTTP %d: %s", datasetCode, res.Status, truncate(string(res.Body), 200))
	}
	var raw []apiPublicWord
	if err := json.Unmarshal(res.Body, &raw); err != nil {
		return nil, fmt.Errorf("decode public_word: %w", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("public_word returned an empty list")
	}
	out := make([]publicWord, 0, len(raw))
	seen := map[int64]struct{}{}
	zeroIDs := 0
	for _, w := range raw {
		if !strings.EqualFold(w.Lang, "est") {
			continue
		}
		if w.WordID == 0 {
			zeroIDs++
			continue
		}
		if _, dup := seen[w.WordID]; dup {
			continue
		}
		seen[w.WordID] = struct{}{}
		out = append(out, publicWord{
			WordID:        w.WordID,
			Lemma:         w.Value,
			Lang:          "ET",
			SourceDataset: datasetCode,
			MorphExists:   w.MorphExists,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("public_word returned %d entries but none matched lang=est (schema may have changed)", len(raw))
	}
	if zeroIDs > 0 {
		return nil, fmt.Errorf("public_word returned %d entries with wordId=0 — refusing to write a partial snapshot", zeroIDs)
	}
	return out, nil
}

// refresh-queue --------------------------------------------------------------

func cmdRefreshQueue(args []string) error {
	fs := flag.NewFlagSet("refresh-queue", flag.ExitOnError)
	base := fs.String("base-url", "https://ekilex.ee", "Ekilex base URL")
	keyEnv := fs.String("key-env", "EKILEX_API_KEY", "Env var holding API key")
	out := fs.String("out", "data/ekilex/eki-public-words-2026-et.jsonl", "Queue file path")
	dataset := fs.String("dataset", "eki", "Dataset code for /api/public_word/{code}")
	fs.Parse(args)

	key := os.Getenv(*keyEnv)
	if key == "" {
		return fmt.Errorf("env %s is required", *keyEnv)
	}

	cur, err := readQueue(*out)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	curIDs := map[int64]struct{}{}
	for _, w := range cur {
		curIDs[w.WordID] = struct{}{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	fresh, err := newAPIClient(*base, key).publicWords(ctx, *dataset)
	if err != nil {
		return err
	}
	freshIDs := map[int64]struct{}{}
	for _, w := range fresh {
		freshIDs[w.WordID] = struct{}{}
	}

	added, removed := 0, 0
	for id := range freshIDs {
		if _, ok := curIDs[id]; !ok {
			added++
		}
	}
	for id := range curIDs {
		if _, ok := freshIDs[id]; !ok {
			removed++
		}
	}
	fmt.Printf("/api/public_word/%s returned %d entries (was %d). added=%d removed=%d\n",
		*dataset, len(fresh), len(cur), added, removed)
	if added == 0 && removed == 0 {
		fmt.Println("No changes; queue file left untouched.")
		return nil
	}
	if err := writeQueue(*out, fresh); err != nil {
		return err
	}
	fmt.Printf("Wrote %d entries to %s\n", len(fresh), *out)
	return nil
}

func readQueue(path string) ([]publicWord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []publicWord
	dec := json.NewDecoder(f)
	for {
		var w publicWord
		if err := dec.Decode(&w); err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, err
		}
		out = append(out, w)
	}
}

func writeQueue(path string, words []publicWord) error {
	if len(words) == 0 {
		return errors.New("refusing to write empty queue")
	}
	for i, w := range words {
		if w.WordID == 0 || strings.TrimSpace(w.Lemma) == "" {
			return fmt.Errorf("refusing to write malformed entry at index %d (word_id=%d lemma=%q); aborting to avoid corrupting %s", i, w.WordID, w.Lemma, path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, w := range words {
		if err := enc.Encode(w); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// sample ---------------------------------------------------------------------

func cmdSample(args []string) error {
	fs := flag.NewFlagSet("sample", flag.ExitOnError)
	base := fs.String("base-url", "https://ekilex.ee", "")
	keyEnv := fs.String("key-env", "EKILEX_API_KEY", "")
	queue := fs.String("queue", "data/ekilex/eki-public-words-2026-et.jsonl", "")
	outDir := fs.String("out-dir", "localdata/ekilex/details/samples", "")
	wordsFlag := fs.String("words", "koer,maja,jooksma,kell,või", "comma-separated lemmas")
	idsFlag := fs.String("word-ids", "", "comma-separated word_ids (overrides -words)")
	variants := fs.String("datasets-compare", "eki,", "comma-separated dataset filters; empty value = no filter")
	fs.Parse(args)

	key := os.Getenv(*keyEnv)
	if key == "" {
		return fmt.Errorf("env %s is required", *keyEnv)
	}
	targets, err := resolveSampleTargets(*queue, *wordsFlag, *idsFlag)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("no sample targets")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	vs := strings.Split(*variants, ",")
	client := newAPIClient(*base, key)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("%-10s %-18s %-10s %-9s %s\n", "word_id", "lemma", "datasets", "size", "path")
	for _, t := range targets {
		for _, v := range vs {
			label := v
			if label == "" {
				label = "(all)"
			}
			res, err := client.wordDetails(ctx, t.WordID, v)
			if err != nil {
				fmt.Printf("%-10d %-18s %-10s ERR        %v\n", t.WordID, t.Lemma, label, err)
				continue
			}
			if res.Status != 200 {
				fmt.Printf("%-10d %-18s %-10s HTTP %-4d %s\n", t.WordID, t.Lemma, label, res.Status, truncate(string(res.Body), 100))
				continue
			}
			tag := v
			if tag == "" {
				tag = "all"
			}
			name := fmt.Sprintf("%d-%s-%s.json", t.WordID, sanitize(t.Lemma), tag)
			path := filepath.Join(*outDir, name)
			if err := os.WriteFile(path, res.Body, 0o644); err != nil {
				return err
			}
			fmt.Printf("%-10d %-18s %-10s %-9s %s\n", t.WordID, t.Lemma, label, humanBytes(int64(len(res.Body))), path)
		}
	}
	return nil
}

func resolveSampleTargets(queuePath, wordsFlag, idsFlag string) ([]publicWord, error) {
	wantLemma := map[string]struct{}{}
	for _, w := range splitCSV(wordsFlag) {
		wantLemma[w] = struct{}{}
	}
	wantID := map[int64]struct{}{}
	for _, s := range splitCSV(idsFlag) {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad word_id %q: %w", s, err)
		}
		wantID[n] = struct{}{}
	}
	if len(wantLemma) == 0 && len(wantID) == 0 {
		return nil, nil
	}
	q, err := readQueue(queuePath)
	if err != nil {
		return nil, fmt.Errorf("read queue %s: %w", queuePath, err)
	}
	var out []publicWord
	for _, w := range q {
		if _, ok := wantLemma[w.Lemma]; ok {
			out = append(out, w)
			delete(wantLemma, w.Lemma)
			continue
		}
		if _, ok := wantID[w.WordID]; ok {
			out = append(out, w)
			delete(wantID, w.WordID)
		}
	}
	if len(wantLemma) > 0 || len(wantID) > 0 {
		var miss []string
		for l := range wantLemma {
			miss = append(miss, l)
		}
		for id := range wantID {
			miss = append(miss, strconv.FormatInt(id, 10))
		}
		return out, fmt.Errorf("not found in queue: %s", strings.Join(miss, ","))
	}
	return out, nil
}

// fetch ----------------------------------------------------------------------

func cmdFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	base := fs.String("base-url", "https://ekilex.ee", "")
	keyEnv := fs.String("key-env", "EKILEX_API_KEY", "")
	queuePath := fs.String("queue", "data/ekilex/eki-public-words-2026-et.jsonl", "")
	outDir := fs.String("out-dir", "localdata/ekilex/details", "")
	workers := fs.Int("workers", 16, "concurrent workers")
	rps := fs.Float64("rps", 16, "global request rate cap (shared across all workers, not per-worker)")
	datasets := fs.String("datasets", "", "dataset filter for /api/word/details (empty = no filter)")
	limit := fs.Int("limit", 0, "stop after N successful fetches (0 = unlimited)")
	maxAttempts := fs.Int("max-attempts", 5, "")
	verbose := fs.Bool("verbose", false, "log every fetch outcome (very chatty)")
	circuitFailures := fs.Int("circuit-failures", 10, "trip circuit after this many consecutive failures (0 disables)")
	circuitPoll := fs.Duration("circuit-poll", 5*time.Minute, "while circuit is tripped, probe a known-good wordId at this interval")
	circuitProbeID := fs.Int64("circuit-probe-id", 183007, "wordId used as the health probe (default: koer)")
	fs.Parse(args)

	if *workers < 1 {
		return errors.New("-workers must be >= 1")
	}
	if *rps <= 0 {
		return errors.New("-rps must be > 0")
	}
	key := os.Getenv(*keyEnv)
	if key == "" {
		return fmt.Errorf("env %s is required", *keyEnv)
	}

	queue, err := readQueue(*queuePath)
	if err != nil {
		return fmt.Errorf("read queue: %w", err)
	}
	rawDir := filepath.Join(*outDir, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return err
	}
	cp, err := openCheckpoint(filepath.Join(*outDir, "checkpoint.sqlite"))
	if err != nil {
		return err
	}
	defer cp.Close()

	todo, alreadyDone, err := filterPending(cp, queue)
	if err != nil {
		return err
	}
	fmt.Printf("[%s] queue=%d done=%d pending=%d workers=%d rps=%.2f datasets=%q\n",
		nowHMS(), len(queue), alreadyDone, len(todo), *workers, *rps, *datasets)
	if len(todo) == 0 {
		return nil
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	interval := time.Duration(float64(time.Second) / *rps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	client := newAPIClient(*base, key)
	jobs := make(chan publicWord, *workers*4)
	var wg sync.WaitGroup
	var okCount, failCount, notFoundCount, bytesWritten, consecFailures atomic.Int64
	var fatalOnce sync.Once
	abort := func(reason string) {
		fatalOnce.Do(func() {
			log.Printf("aborting: %s", reason)
			cancel()
		})
	}

	gate := newPauseGate()
	tripCircuit := func() {
		if *circuitFailures <= 0 {
			return
		}
		if !gate.Pause() {
			return // already paused; another worker beat us to it
		}
		fmt.Printf("[%s] CIRCUIT OPEN: %d consecutive failures, pausing fetches; probing word_id=%d every %s\n",
			nowHMS(), consecFailures.Load(), *circuitProbeID, *circuitPoll)
		go func() {
			defer gate.Resume()
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(*circuitPoll):
				}
				probeCtx, probeCancel := context.WithTimeout(ctx, 30*time.Second)
				res, err := client.wordDetails(probeCtx, *circuitProbeID, *datasets)
				probeCancel()
				if err == nil && res.Status == 200 {
					consecFailures.Store(0)
					fmt.Printf("[%s] CIRCUIT CLOSED: probe word_id=%d returned HTTP 200 (%s), resuming fetches\n",
						nowHMS(), *circuitProbeID, humanBytes(int64(len(res.Body))))
					return
				}
				detail := ""
				if err != nil {
					detail = err.Error()
				} else {
					detail = fmt.Sprintf("HTTP %d", res.Status)
				}
				fmt.Printf("[%s] circuit still open: probe word_id=%d -> %s\n", nowHMS(), *circuitProbeID, detail)
			}
		}()
	}

	target := int64(*limit)
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range jobs {
				if target > 0 && okCount.Load() >= target {
					cancel()
					return
				}
				if err := gate.Wait(ctx); err != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				start := time.Now()
				o := fetchOne(ctx, client, w, *datasets, rawDir, *maxAttempts)
				elapsed := time.Since(start)
				if *verbose {
					fmt.Printf("[%s] word_id=%d lemma=%q status=%s http=%d attempts=%d size=%s elapsed=%s err=%q\n",
						nowHMS(), w.WordID, w.Lemma, o.status, o.httpStatus, o.attempts, humanBytes(o.size), elapsed.Round(time.Millisecond), o.err)
				}
				if err := recordOutcome(cp, w.WordID, o); err != nil {
					log.Printf("[%s] checkpoint write word_id=%d: %v", nowHMS(), w.WordID, err)
				}
				switch o.status {
				case "ok":
					okCount.Add(1)
					bytesWritten.Add(o.size)
					consecFailures.Store(0)
				case "not_found":
					notFoundCount.Add(1)
					consecFailures.Store(0)
				case "auth":
					abort(fmt.Sprintf("HTTP %d on word_id=%d: %s", o.httpStatus, w.WordID, o.err))
					return
				case "canceled":
					return
				default:
					failCount.Add(1)
					if n := consecFailures.Add(1); *circuitFailures > 0 && n >= int64(*circuitFailures) {
						tripCircuit()
					}
				}
				done := okCount.Load() + failCount.Load() + notFoundCount.Load()
				if done > 0 && done%500 == 0 {
					fmt.Printf("[%s] progress: ok=%d failed=%d not_found=%d bytes=%s pending=%d\n",
						nowHMS(), okCount.Load(), failCount.Load(), notFoundCount.Load(), humanBytes(bytesWritten.Load()), int64(len(todo))-done)
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, w := range todo {
			select {
			case <-ctx.Done():
				return
			case jobs <- w:
			}
		}
	}()

	wg.Wait()
	fmt.Printf("[%s] done: ok=%d failed=%d not_found=%d bytes=%s\n",
		nowHMS(), okCount.Load(), failCount.Load(), notFoundCount.Load(), humanBytes(bytesWritten.Load()))
	return nil
}

func nowHMS() string { return time.Now().Format("15:04:05") }

// pauseGate is a one-bit broadcast: workers call Wait before each fetch and
// block while the circuit is paused. Pause replaces the resume channel with a
// fresh unclosed one so subsequent waiters block; Resume closes the channel
// to release every blocked waiter at once.
type pauseGate struct {
	mu       sync.Mutex
	paused   bool
	resumeCh chan struct{}
}

func newPauseGate() *pauseGate {
	g := &pauseGate{resumeCh: make(chan struct{})}
	close(g.resumeCh) // start unpaused
	return g
}

func (g *pauseGate) Wait(ctx context.Context) error {
	g.mu.Lock()
	ch := g.resumeCh
	g.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *pauseGate) Pause() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.paused {
		return false
	}
	g.paused = true
	g.resumeCh = make(chan struct{})
	return true
}

func (g *pauseGate) Resume() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.paused {
		return
	}
	g.paused = false
	close(g.resumeCh)
}

type outcome struct {
	status     string // ok | not_found | failed | auth | canceled
	httpStatus int
	attempts   int
	size       int64
	err        string
}

func fetchOne(ctx context.Context, c *apiClient, w publicWord, datasets, rawDir string, maxAttempts int) outcome {
	var lastStatus int
	var lastErr error
	backoff := time.Second
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return outcome{status: "canceled", attempts: attempt - 1, err: ctx.Err().Error()}
		}
		res, err := c.wordDetails(ctx, w.WordID, datasets)
		if err != nil {
			lastErr = err
		} else {
			switch {
			case res.Status == 200:
				n, werr := writeRawGzip(rawDir, w.WordID, res.Body)
				if werr != nil {
					return outcome{status: "failed", httpStatus: 200, attempts: attempt, err: werr.Error()}
				}
				return outcome{status: "ok", httpStatus: 200, attempts: attempt, size: n}
			case res.Status == 404:
				return outcome{status: "not_found", httpStatus: 404, attempts: attempt}
			case res.Status == 401 || res.Status == 403:
				return outcome{status: "auth", httpStatus: res.Status, attempts: attempt, err: truncate(string(res.Body), 200)}
			default:
				lastStatus = res.Status
				lastErr = fmt.Errorf("http %d: %s", res.Status, truncate(string(res.Body), 100))
				if res.RetryAfter > 0 {
					backoff = res.RetryAfter
				}
			}
		}
		if attempt < maxAttempts {
			jitter := time.Duration(rand.Int63n(int64(backoff/4 + 1)))
			select {
			case <-ctx.Done():
				return outcome{status: "canceled", attempts: attempt, err: ctx.Err().Error()}
			case <-time.After(backoff + jitter):
			}
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
	msg := "exhausted retries"
	if lastErr != nil {
		msg = lastErr.Error()
	}
	return outcome{status: "failed", httpStatus: lastStatus, attempts: maxAttempts, err: msg}
}

// Bucketed output keeps any single dir under ~200 entries on the current
// 174k headword set, which matters on macOS / network filesystems where
// directories with hundreds of thousands of files become slow to traverse.
func writeRawGzip(rawDir string, wordID int64, body []byte) (int64, error) {
	bucket := fmt.Sprintf("%05d", wordID/1000)
	dir := filepath.Join(rawDir, bucket)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%d.json.gz", wordID))
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write(body); err != nil {
		gz.Close()
		f.Close()
		os.Remove(tmp)
		return 0, err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		os.Remove(tmp)
		return 0, err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return 0, err
	}
	fi, err := os.Stat(tmp)
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// checkpoint -----------------------------------------------------------------

func openCheckpoint(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
CREATE TABLE IF NOT EXISTS fetch_status (
    word_id INTEGER PRIMARY KEY,
    status TEXT NOT NULL,
    http_status INTEGER,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    raw_size_bytes INTEGER,
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func recordOutcome(db *sql.DB, wordID int64, o outcome) error {
	_, err := db.Exec(`INSERT INTO fetch_status
    (word_id, status, http_status, attempts, last_error, raw_size_bytes, fetched_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(word_id) DO UPDATE SET
    status=excluded.status,
    http_status=excluded.http_status,
    attempts=excluded.attempts,
    last_error=excluded.last_error,
    raw_size_bytes=excluded.raw_size_bytes,
    fetched_at=excluded.fetched_at`,
		wordID, o.status, nullableInt(o.httpStatus), o.attempts, nullableStr(o.err), nullableInt64(o.size))
	return err
}

func filterPending(db *sql.DB, queue []publicWord) (todo []publicWord, alreadyDone int, err error) {
	rows, err := db.Query(`SELECT word_id FROM fetch_status WHERE status IN ('ok','not_found')`)
	if err != nil {
		return nil, 0, err
	}
	done := map[int64]struct{}{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, 0, err
		}
		done[id] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return nil, 0, err
	}
	for _, w := range queue {
		if _, ok := done[w.WordID]; ok {
			alreadyDone++
			continue
		}
		todo = append(todo, w)
	}
	return todo, alreadyDone, nil
}

// helpers --------------------------------------------------------------------

func nullableInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func nullableInt64(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '/', '\\', ':':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func humanBytes(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%dB", n)
	case n < k*k:
		return fmt.Sprintf("%.1fK", float64(n)/k)
	case n < k*k*k:
		return fmt.Sprintf("%.1fM", float64(n)/(k*k))
	default:
		return fmt.Sprintf("%.1fG", float64(n)/(k*k*k))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
