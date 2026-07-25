// Command loadtest is a dependency-free HTTP load generator for FinEstDB's
// public-alpha capacity gate (see docs/GO_LIVE_CHECKLIST.md "Capacity and
// Graceful Degradation"). It models the GO_LIVE traffic mix - anonymous
// paste/parse, signed-in parse, and signed-in review/deck reads - against a
// running server, and reports per-endpoint latency percentiles, throughput,
// and error/429/503 counts.
//
// It registers exactly one scratch account per run (clearly named, see
// scratchEmail) and creates at most one deck on that account. It never
// mutates review state (answers) or the dictionary; review traffic is reads
// only ("review/deck reads" in the checklist wording).
//
// Usage:
//
//	go run ./cmd/loadtest -url http://localhost:8090 -concurrency 200 -duration 30s
//
// See -help for the full flag set.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Short FI/ET texts, deliberately tiny so a laptop-scale run models request
// *volume* rather than accidentally load-testing parser throughput on long
// documents (that is a separate, already-measured concern - see
// docs/DEPLOYMENT.md "Latency expectations"). Real anonymous demo traffic is
// dominated by short pastes, not book-length uploads.
var (
	sampleTextsFI = []string{
		"Kissa istuu ikkunalla ja katselee lintuja pihalla.",
		"Suomen kesä on lyhyt mutta valoisa, ja ihmiset nauttivat siitä ulkona.",
		"Hän osti kaupasta maitoa, leipää ja muutaman omenan.",
		"Metsässä kasvaa mäntyjä, kuusia ja koivuja eri puolilla Suomea.",
	}
	sampleTextsET = []string{
		"Kass istub akna peal ja vaatab õues linde.",
		"Eesti suvi on lühike, kuid valge ja inimesed naudivad seda õues.",
		"Ta ostis poest piima, leiba ja mõned õunad.",
		"Metsas kasvavad männid, kuused ja kased üle kogu Eesti.",
	}
)

type config struct {
	baseURL          string
	duration         time.Duration
	concurrency      int
	rampUp           time.Duration
	anonParseRatio   float64
	signedParseRatio float64
	reviewReadRatio  float64
	requestTimeout   time.Duration
	outPath          string
	maxP95           time.Duration // abort the run if p95 exceeds this (0 = disabled)
	seed             string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.baseURL, "url", "http://localhost:8080", "Target server base URL")
	flag.DurationVar(&cfg.duration, "duration", 30*time.Second, "How long to generate load")
	flag.IntVar(&cfg.concurrency, "concurrency", 50, "Number of concurrent virtual users")
	flag.DurationVar(&cfg.rampUp, "ramp", 5*time.Second, "Time to ramp up to full concurrency (spreads virtual-user start times)")
	flag.Float64Var(&cfg.anonParseRatio, "anon-parse-ratio", 0.5, "Fraction of virtual users doing anonymous parse traffic")
	flag.Float64Var(&cfg.signedParseRatio, "signed-parse-ratio", 0.3, "Fraction of virtual users doing signed-in parse traffic")
	flag.Float64Var(&cfg.reviewReadRatio, "review-read-ratio", 0.2, "Fraction of virtual users doing signed-in review/deck read traffic")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 15*time.Second, "Per-request HTTP client timeout")
	flag.StringVar(&cfg.outPath, "out", "", "Path to write the JSON summary (default: stdout only)")
	flag.DurationVar(&cfg.maxP95, "max-p95", 0, "If >0, abort the run early once p95 latency for any endpoint exceeds this")
	flag.StringVar(&cfg.seed, "run-id", "", "Run identifier used in the scratch account email (default: random hex)")
	flag.Parse()

	total := cfg.anonParseRatio + cfg.signedParseRatio + cfg.reviewReadRatio
	if math.Abs(total-1.0) > 0.01 {
		log.Fatalf("ratios must sum to 1.0, got %.3f (anon=%.2f signed=%.2f review=%.2f)", total, cfg.anonParseRatio, cfg.signedParseRatio, cfg.reviewReadRatio)
	}
	if cfg.seed == "" {
		cfg.seed = randomHex(6)
	}

	result := run(cfg)
	printReport(result)

	if cfg.outPath != "" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Fatalf("marshal summary: %v", err)
		}
		if err := os.WriteFile(cfg.outPath, data, 0o644); err != nil {
			log.Fatalf("write summary %s: %v", cfg.outPath, err)
		}
		fmt.Fprintf(os.Stderr, "\nJSON summary written to %s\n", cfg.outPath)
	}

	if result.Aborted {
		os.Exit(1)
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

// endpointKind labels which logical traffic class a sample belongs to, for
// per-endpoint reporting.
type endpointKind string

const (
	kindAnonParse   endpointKind = "anon_parse"
	kindSignedParse endpointKind = "signed_parse"
	kindDeckRead    endpointKind = "deck_read"
	kindReviewRead  endpointKind = "review_read"
	kindDeckCreate  endpointKind = "deck_create"
	kindRegister    endpointKind = "register"
)

type sample struct {
	kind     endpointKind
	status   int
	latency  time.Duration
	err      bool
	is429    bool
	is503    bool
	retryHdr bool
}

type collector struct {
	mu      sync.Mutex
	samples map[endpointKind][]sample
}

func newCollector() *collector {
	return &collector{samples: make(map[endpointKind][]sample)}
}

func (c *collector) add(s sample) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.samples[s.kind] = append(c.samples[s.kind], s)
}

func (c *collector) snapshotCounts() (total, errs, r429, r503 int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, list := range c.samples {
		for _, s := range list {
			total++
			if s.err {
				errs++
			}
			if s.is429 {
				r429++
			}
			if s.is503 {
				r503++
			}
		}
	}
	return
}

// p95For returns the current p95 latency (ms) across all samples for kind, or
// 0 if there are no samples yet. Used for the optional early-abort guard.
func (c *collector) p95For(kind endpointKind) time.Duration {
	c.mu.Lock()
	list := append([]sample(nil), c.samples[kind]...)
	c.mu.Unlock()
	if len(list) == 0 {
		return 0
	}
	lat := make([]time.Duration, len(list))
	for i, s := range list {
		lat[i] = s.latency
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	return percentile(lat, 0.95)
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// EndpointSummary is the machine-readable per-endpoint result.
type EndpointSummary struct {
	Kind        string  `json:"kind"`
	Requests    int     `json:"requests"`
	Errors      int     `json:"errors"`
	Count429    int     `json:"count_429"`
	Count503    int     `json:"count_503"`
	P50Ms       float64 `json:"p50_ms"`
	P95Ms       float64 `json:"p95_ms"`
	P99Ms       float64 `json:"p99_ms"`
	MaxMs       float64 `json:"max_ms"`
	ThroughputR float64 `json:"throughput_req_per_sec"`
}

// Summary is the full JSON-serializable report for one run.
type Summary struct {
	RunID           string            `json:"run_id"`
	TargetURL       string            `json:"target_url"`
	Concurrency     int               `json:"concurrency"`
	Duration        string            `json:"duration"`
	RampUp          string            `json:"ramp_up"`
	StartedAt       time.Time         `json:"started_at"`
	FinishedAt      time.Time         `json:"finished_at"`
	Aborted         bool              `json:"aborted"`
	AbortReason     string            `json:"abort_reason,omitempty"`
	TotalRequests   int64             `json:"total_requests"`
	TotalErrors     int64             `json:"total_errors"`
	Total429        int64             `json:"total_429"`
	Total503        int64             `json:"total_503"`
	Endpoints       []EndpointSummary `json:"endpoints"`
	ScratchEmail    string            `json:"scratch_email"`
	ScratchDeckID   int64             `json:"scratch_deck_id,omitempty"`
	DurableRowsNote string            `json:"durable_rows_note"`
}

func run(cfg config) Summary {
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{
		Timeout: cfg.requestTimeout,
		Jar:     jar,
	}

	scratchEmail := fmt.Sprintf("loadtest-%s@finnestdb-scratch.invalid", cfg.seed)
	log.Printf("registering scratch account %s ...", scratchEmail)
	if err := registerScratchAccount(httpClient, cfg.baseURL, scratchEmail); err != nil {
		log.Fatalf("failed to register scratch account: %v", err)
	}

	var deckID int64
	log.Printf("seeding one scratch deck for review/deck-read traffic ...")
	deckID, err := createScratchDeck(httpClient, cfg.baseURL)
	if err != nil {
		log.Fatalf("failed to seed scratch deck: %v", err)
	}
	log.Printf("scratch deck id=%d created", deckID)

	col := newCollector()
	ctx, cancel := contextWithAbort()
	defer cancel()

	var aborted atomic.Bool
	var abortReason atomic.Value
	abortReason.Store("")

	if cfg.maxP95 > 0 {
		go watchdog(ctx, col, cfg.maxP95, &aborted, &abortReason, cancel)
	}

	var wg sync.WaitGroup
	started := time.Now()
	perUserDelay := time.Duration(0)
	if cfg.concurrency > 0 && cfg.rampUp > 0 {
		perUserDelay = cfg.rampUp / time.Duration(cfg.concurrency)
	}

	for i := 0; i < cfg.concurrency; i++ {
		wg.Add(1)
		vu := assignVUKind(i, cfg)
		delay := time.Duration(i) * perUserDelay
		go func(vu endpointKind, delay time.Duration) {
			defer wg.Done()
			select {
			case <-time.After(delay):
			case <-ctx:
				return
			}
			virtualUser(ctx, cfg, httpClient, col, vu, scratchEmail, deckID)
		}(vu, delay)
	}

	// Stop the run after cfg.duration (measured from run start, not counting
	// ramp-up separately - ramp-up is "part of" the duration window).
	go func() {
		select {
		case <-time.After(cfg.duration):
			cancel()
		case <-ctx:
		}
	}()

	wg.Wait()
	finished := time.Now()

	total, errs, r429, r503 := col.snapshotCounts()
	summary := Summary{
		RunID:         cfg.seed,
		TargetURL:     cfg.baseURL,
		Concurrency:   cfg.concurrency,
		Duration:      cfg.duration.String(),
		RampUp:        cfg.rampUp.String(),
		StartedAt:     started,
		FinishedAt:    finished,
		Aborted:       aborted.Load(),
		AbortReason:   abortReason.Load().(string),
		TotalRequests: total,
		TotalErrors:   errs,
		Total429:      r429,
		Total503:      r503,
		ScratchEmail:  scratchEmail,
		ScratchDeckID: deckID,
		DurableRowsNote: fmt.Sprintf(
			"1 user row (email=%s), 1 session, 1 deck (id=%d) with its sentences/cards. "+
				"No review answers were submitted (reads only). Safe to delete via the scratch account "+
				"or left as an identifiable loadtest-* row for manual cleanup.",
			scratchEmail, deckID,
		),
	}

	elapsed := finished.Sub(started).Seconds()
	for _, kind := range []endpointKind{kindRegister, kindDeckCreate, kindAnonParse, kindSignedParse, kindDeckRead, kindReviewRead} {
		es := summarizeEndpoint(col, kind, elapsed)
		if es.Requests == 0 {
			continue
		}
		summary.Endpoints = append(summary.Endpoints, es)
	}
	return summary
}

func contextWithAbort() (chan struct{}, func()) {
	ch := make(chan struct{})
	var once sync.Once
	return ch, func() { once.Do(func() { close(ch) }) }
}

func watchdog(ctx chan struct{}, col *collector, maxP95 time.Duration, aborted *atomic.Bool, reason *atomic.Value, cancel func()) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx:
			return
		case <-ticker.C:
			for _, kind := range []endpointKind{kindAnonParse, kindSignedParse, kindDeckRead, kindReviewRead} {
				p95 := col.p95For(kind)
				if p95 > maxP95 {
					aborted.Store(true)
					reason.Store(fmt.Sprintf("p95 for %s reached %v, exceeding -max-p95=%v", kind, p95, maxP95))
					log.Printf("ABORTING: %s", reason.Load())
					cancel()
					return
				}
			}
		}
	}
}

func assignVUKind(i int, cfg config) endpointKind {
	// Deterministic round-robin split by cumulative ratio, not random, so a
	// given -concurrency N always produces the same mix for reproducibility
	// across stages.
	frac := float64(i) / float64(max(cfg.concurrency, 1))
	switch {
	case frac < cfg.anonParseRatio:
		return kindAnonParse
	case frac < cfg.anonParseRatio+cfg.signedParseRatio:
		return kindSignedParse
	default:
		return kindReviewRead
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// virtualUser repeatedly issues requests of its assigned kind until ctx is
// cancelled (duration elapsed or watchdog abort).
func virtualUser(ctx chan struct{}, cfg config, client *http.Client, col *collector, vu endpointKind, scratchEmail string, deckID int64) {
	// Signed-in virtual users share the single scratch account's session
	// cookie via the shared cookiejar set up in run(); re-logging in per VU
	// would multiply durable session rows for no load-modeling benefit.
	i := 0
	for {
		select {
		case <-ctx:
			return
		default:
		}
		switch vu {
		case kindAnonParse:
			doAnonParse(client, cfg.baseURL, col, i)
		case kindSignedParse:
			doSignedParse(client, cfg.baseURL, col, i)
		case kindReviewRead:
			if i%2 == 0 {
				doDeckRead(client, cfg.baseURL, col)
			} else {
				doReviewRead(client, cfg.baseURL, col, deckID)
			}
		}
		i++
	}
}

func doAnonParse(client *http.Client, base string, col *collector, i int) {
	lang := "FI"
	text := sampleTextsFI[i%len(sampleTextsFI)]
	if i%2 == 1 {
		lang = "ET"
		text = sampleTextsET[i%len(sampleTextsET)]
	}
	body, _ := json.Marshal(map[string]string{"lang": lang, "text": text})
	postJSONNoAuth(client, base+"/api/parse", body, col, kindAnonParse)
}

func doSignedParse(client *http.Client, base string, col *collector, i int) {
	lang := "FI"
	text := sampleTextsFI[i%len(sampleTextsFI)]
	if i%2 == 1 {
		lang = "ET"
		text = sampleTextsET[i%len(sampleTextsET)]
	}
	body, _ := json.Marshal(map[string]string{"lang": lang, "text": text, "parser": "custom"})
	postJSON(client, base+"/api/parse", body, col, kindSignedParse)
}

func doDeckRead(client *http.Client, base string, col *collector) {
	getJSON(client, base+"/api/decks", col, kindDeckRead)
}

func doReviewRead(client *http.Client, base string, col *collector, deckID int64) {
	url := base + "/api/review/next"
	if deckID > 0 {
		url = fmt.Sprintf("%s?deck_id=%d", url, deckID)
	}
	getJSON(client, url, col, kindReviewRead)
}

func registerScratchAccount(client *http.Client, base, email string) error {
	body, _ := json.Marshal(map[string]string{"email": email, "password": "loadtest-scratch-password-1"})
	req, err := http.NewRequest(http.MethodPost, base+"/api/auth/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("register status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

func createScratchDeck(client *http.Client, base string) (int64, error) {
	text := strings.Join(sampleTextsFI, " ")
	body, _ := json.Marshal(map[string]any{
		"title": "loadtest scratch deck",
		"lang":  "FI",
		"text":  text,
	})
	req, err := http.NewRequest(http.MethodPost, base+"/api/decks", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("create deck status=%d body=%s", resp.StatusCode, string(respBody))
	}
	var out struct {
		DeckID int64 `json:"deck_id"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return 0, err
	}
	return out.DeckID, nil
}

func postJSON(client *http.Client, url string, body []byte, col *collector, kind endpointKind) {
	doRequest(client, http.MethodPost, url, body, col, kind)
}

// postJSONNoAuth uses a client with no cookie jar so anonymous parse traffic
// never accidentally rides the scratch account's session cookie.
var anonHTTPClient = &http.Client{Timeout: 15 * time.Second}

func postJSONNoAuth(client *http.Client, url string, body []byte, col *collector, kind endpointKind) {
	anonHTTPClient.Timeout = client.Timeout
	doRequest(anonHTTPClient, http.MethodPost, url, body, col, kind)
}

func getJSON(client *http.Client, url string, col *collector, kind endpointKind) {
	doRequest(client, http.MethodGet, url, nil, col, kind)
}

func doRequest(client *http.Client, method, url string, body []byte, col *collector, kind endpointKind) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		col.add(sample{kind: kind, err: true})
		return
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		col.add(sample{kind: kind, err: true, latency: latency})
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	s := sample{
		kind:    kind,
		status:  resp.StatusCode,
		latency: latency,
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		s.is429 = true
	case http.StatusServiceUnavailable:
		s.is503 = true
	}
	if resp.StatusCode >= 500 && resp.StatusCode != http.StatusServiceUnavailable {
		s.err = true
	}
	if (s.is429 || s.is503) && resp.Header.Get("Retry-After") != "" {
		s.retryHdr = true
	}
	col.add(s)
}

func summarizeEndpoint(col *collector, kind endpointKind, elapsedSeconds float64) EndpointSummary {
	col.mu.Lock()
	list := append([]sample(nil), col.samples[kind]...)
	col.mu.Unlock()

	es := EndpointSummary{Kind: string(kind), Requests: len(list)}
	if len(list) == 0 {
		return es
	}
	lat := make([]time.Duration, len(list))
	for i, s := range list {
		lat[i] = s.latency
		if s.err {
			es.Errors++
		}
		if s.is429 {
			es.Count429++
		}
		if s.is503 {
			es.Count503++
		}
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	es.P50Ms = float64(percentile(lat, 0.50)) / float64(time.Millisecond)
	es.P95Ms = float64(percentile(lat, 0.95)) / float64(time.Millisecond)
	es.P99Ms = float64(percentile(lat, 0.99)) / float64(time.Millisecond)
	es.MaxMs = float64(lat[len(lat)-1]) / float64(time.Millisecond)
	if elapsedSeconds > 0 {
		es.ThroughputR = float64(len(list)) / elapsedSeconds
	}
	return es
}

func printReport(s Summary) {
	fmt.Println()
	fmt.Printf("=== Load test report (run %s) ===\n", s.RunID)
	fmt.Printf("target=%s concurrency=%d duration=%s ramp=%s\n", s.TargetURL, s.Concurrency, s.Duration, s.RampUp)
	if s.Aborted {
		fmt.Printf("ABORTED: %s\n", s.AbortReason)
	}
	fmt.Printf("total requests=%d errors=%d 429s=%d 503s=%d\n", s.TotalRequests, s.TotalErrors, s.Total429, s.Total503)
	fmt.Println()
	fmt.Printf("%-14s %10s %8s %9s %9s %9s %9s %14s\n", "endpoint", "requests", "errors", "429", "503", "p50(ms)", "p95(ms)", "p99(ms)/thpt")
	for _, e := range s.Endpoints {
		fmt.Printf("%-14s %10d %8d %9d %9d %9.1f %9.1f %9.1f  %.1f req/s\n",
			e.Kind, e.Requests, e.Errors, e.Count429, e.Count503, e.P50Ms, e.P95Ms, e.P99Ms, e.ThroughputR)
	}
	fmt.Printf("\nscratch account: %s (deck id=%d)\n", s.ScratchEmail, s.ScratchDeckID)
	fmt.Printf("durable rows: %s\n", s.DurableRowsNote)
}
