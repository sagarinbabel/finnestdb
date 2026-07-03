package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/store"
)

// TestParserLimiterSheddsWhenSaturated proves the core backpressure contract:
// once every slot in the total pool is held, a new acquire cannot proceed and
// must give up at the queue timeout rather than block forever or silently let
// the parser run unbounded.
func TestParserLimiterSheddsWhenSaturated(t *testing.T) {
	lim := newParserLimiter(2, 30*time.Millisecond)

	release1, ok := lim.acquire(context.Background(), false)
	if !ok {
		t.Fatal("first acquire should succeed")
	}
	defer release1()
	release2, ok := lim.acquire(context.Background(), false)
	if !ok {
		t.Fatal("second acquire should succeed (pool size 2)")
	}
	defer release2()

	start := time.Now()
	_, ok = lim.acquire(context.Background(), false)
	elapsed := time.Since(start)
	if ok {
		t.Fatal("third acquire should be shed once the pool is saturated")
	}
	if elapsed < 30*time.Millisecond {
		t.Fatalf("acquire returned after %v, want it to wait out the queue timeout", elapsed)
	}
}

// TestParserLimiterAnonShedBeforeSignedIn proves the launch-gate requirement
// that anonymous parse load cannot starve signed-in traffic: with a total
// pool of 4 (anon sub-pool of 2), once 2 anonymous slots are held a third
// anonymous request is shed while a signed-in request still gets a slot from
// the shared pool.
func TestParserLimiterAnonShedBeforeSignedIn(t *testing.T) {
	lim := newParserLimiter(4, 30*time.Millisecond)

	releaseA1, ok := lim.acquire(context.Background(), true)
	if !ok {
		t.Fatal("first anonymous acquire should succeed")
	}
	defer releaseA1()
	releaseA2, ok := lim.acquire(context.Background(), true)
	if !ok {
		t.Fatal("second anonymous acquire should succeed (anon sub-pool size 2)")
	}
	defer releaseA2()

	if _, ok := lim.acquire(context.Background(), true); ok {
		t.Fatal("third anonymous acquire should be shed once the anon sub-pool (2) is exhausted")
	}

	releaseSignedIn, ok := lim.acquire(context.Background(), false)
	if !ok {
		t.Fatal("signed-in acquire should still succeed: only 2 of 4 total slots are held")
	}
	releaseSignedIn()
}

// TestParserLimiterQueueTimeoutRespected confirms a blocked acquire returns
// close to the configured timeout, not immediately and not indefinitely —
// callers use this value to decide the Retry-After they hand back to
// clients, so it must be trustworthy.
func TestParserLimiterQueueTimeoutRespected(t *testing.T) {
	lim := newParserLimiter(1, 50*time.Millisecond)
	release, ok := lim.acquire(context.Background(), false)
	if !ok {
		t.Fatal("first acquire should succeed")
	}
	defer release()

	start := time.Now()
	_, ok = lim.acquire(context.Background(), false)
	elapsed := time.Since(start)
	if ok {
		t.Fatal("acquire should fail while the only slot is held")
	}
	if elapsed < 50*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Fatalf("elapsed=%v want roughly the 50ms queue timeout", elapsed)
	}
}

// TestParserLimiterReleaseFreesSlotForNextWaiter confirms a released slot is
// immediately available to a subsequent acquire — otherwise every stage after
// the first burst would degrade forever instead of recovering once load
// drops.
func TestParserLimiterReleaseFreesSlotForNextWaiter(t *testing.T) {
	lim := newParserLimiter(1, 200*time.Millisecond)
	release, ok := lim.acquire(context.Background(), false)
	if !ok {
		t.Fatal("first acquire should succeed")
	}

	done := make(chan bool, 1)
	go func() {
		_, ok := lim.acquire(context.Background(), false)
		done <- ok
	}()

	time.Sleep(10 * time.Millisecond)
	release()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("waiter should acquire the slot once released")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("waiter never acquired the freed slot")
	}
}

// TestHandleParseReturns503WithRetryAfterWhenParserSaturated exercises the
// full HTTP path: a caller that cannot get a parser slot within the queue
// timeout gets 503 (not a hang, not a 500) with a Retry-After header, and the
// error body matches the existing JSON error shape used elsewhere in this
// handler (an "error" field), not a bare http.Error text body.
func TestHandleParseReturns503WithRetryAfterWhenParserSaturated(t *testing.T) {
	api := newTestAPI(t)
	api.parserLimiter = newParserLimiter(1, 20*time.Millisecond)
	mux := newTestMux(t, api)

	block := make(chan struct{})
	unblock := make(chan struct{})
	api.analyze = func(_ *store.DB, lang, _, _ string) (*parsecore.ParseResult, error) {
		close(block)
		<-unblock
		return &parsecore.ParseResult{Lang: lang, Parser: "basic"}, nil
	}
	defer close(unblock)

	// Occupy the single slot with a request that blocks inside analyze.
	go func() {
		body := `{"lang":"FI","text":"hei"}`
		req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}()
	<-block

	body := `{"lang":"FI","text":"moi"}`
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Fatal("expected Retry-After header on 503")
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode 503 body as JSON: %v (body=%q)", err, rec.Body.String())
	}
	if resp.Error == "" {
		t.Fatal("503 body should carry a non-empty error field matching existing error shapes")
	}
}

// TestHandleParseAnonymousShedsBeforeSignedInUnderSaturation is the
// end-to-end proof of the launch-gate requirement: at saturation, anonymous
// parse traffic is shed while signed-in parse traffic keeps working. This is
// the scenario docs/GO_LIVE_CHECKLIST.md calls a no-go blocker if it fails
// ("anonymous parse load that can starve signed-in review/deck usage").
func TestHandleParseAnonymousShedsBeforeSignedInUnderSaturation(t *testing.T) {
	api := newTestAPI(t)
	// total=2 -> anon sub-pool=1. Hold the one anon slot, then prove a second
	// anonymous request is shed while a signed-in request still succeeds.
	api.parserLimiter = newParserLimiter(2, 20*time.Millisecond)
	mux := newTestMux(t, api)

	block := make(chan struct{})
	unblock := make(chan struct{})
	first := true
	var mu sync.Mutex
	api.analyze = func(_ *store.DB, lang, _, _ string) (*parsecore.ParseResult, error) {
		mu.Lock()
		isFirst := first
		first = false
		mu.Unlock()
		if isFirst {
			close(block)
			<-unblock
		}
		return &parsecore.ParseResult{Lang: lang, Parser: "basic"}, nil
	}
	defer close(unblock)

	cookies := loginAndReturnCookies(t, mux, "parser-limiter-signed-in@example.com")

	// Occupy the single anonymous slot.
	go func() {
		body := `{"lang":"FI","text":"hei"}`
		req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}()
	<-block

	// A second anonymous request should be shed (anon sub-pool exhausted).
	anonBody := `{"lang":"FI","text":"moi"}`
	anonReq := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(anonBody))
	anonRec := httptest.NewRecorder()
	mux.ServeHTTP(anonRec, anonReq)
	if anonRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("second anonymous parse status=%d want %d (should be shed)", anonRec.Code, http.StatusServiceUnavailable)
	}

	// A signed-in request should still succeed: the shared pool has 2 slots,
	// only 1 is held by the blocked anonymous request.
	signedBody := `{"lang":"FI","text":"terve","parser":"basic"}`
	signedReq := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(signedBody))
	for _, c := range cookies {
		signedReq.AddCookie(c)
	}
	signedRec := httptest.NewRecorder()
	mux.ServeHTTP(signedRec, signedReq)
	if signedRec.Code != http.StatusOK {
		t.Fatalf("signed-in parse status=%d want %d body=%q (signed-in traffic must survive anonymous saturation)", signedRec.Code, http.StatusOK, signedRec.Body.String())
	}
}

// TestNonParseEndpointsBypassParserLimiter confirms endpoints that do not run
// the parser (e.g. deck listing) are never blocked by an exhausted parser
// pool. If this regressed, review/deck reads would wrongly queue behind
// parser saturation even though they do no parser work at all.
func TestNonParseEndpointsBypassParserLimiter(t *testing.T) {
	api := newTestAPI(t)
	// Zero-capacity-equivalent: total=1 anon=1, and we hold the only slot
	// forever (never released within the test) to simulate full saturation.
	api.parserLimiter = newParserLimiter(1, 20*time.Millisecond)
	mux := newTestMux(t, api)

	cookies := loginAndReturnCookies(t, mux, "parser-limiter-nonparse@example.com")

	release, ok := api.parserLimiter.acquire(context.Background(), false)
	if !ok {
		t.Fatal("failed to saturate the parser pool for the test setup")
	}
	defer release()

	req := httptest.NewRequest(http.MethodGet, "/api/decks", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/decks status=%d want %d — non-parse endpoints must not go through the parser semaphore", rec.Code, http.StatusOK)
	}
}
