package api

import (
	"context"
	"net/http"
	"runtime"
	"strconv"
	"time"
)

// parserLimiter bounds how many expensive parse-path calls (parsecore.Analyze
// / AnalyzeChapters) can run at once, independent of the per-IP/per-account
// rate limiters in rate_limit.go. Rate limits cap request *frequency*; this
// caps request *concurrency*, which is what actually protects CPU/memory when
// many allowed requests land at once (e.g. a burst from many distinct IPs).
//
// Mechanism: two counting semaphores sized off a single total budget.
//   - total: FINNESTDB_PARSER_MAX_CONCURRENCY slots (default max(2, NumCPU-1),
//     leaving a core for the DB/HTTP goroutines and Rust FFI overhead).
//   - anonymous slots are capped at half of total (rounded down, min 1) via a
//     dedicated smaller semaphore, so an anonymous-load burst cannot claim
//     every slot in the shared pool. Signed-in requests always draw from the
//     full pool.
//
// This is the simpler of the two designs called out in the launch checklist
// (a single split budget vs. two independently-sized pools with separate
// envs). A single total keeps capacity planning to one number; the anonymous
// share is a fixed fraction of it rather than a second tunable.
//
// A caller that cannot acquire a slot within the queue timeout gets 503 with
// Retry-After, before any parser work runs.
type parserLimiter struct {
	total   chan struct{}
	anon    chan struct{}
	timeout time.Duration
}

const (
	defaultParserQueueTimeoutMS = 2000
	// anonShareDivisor caps anonymous parse concurrency at 1/N of the total
	// parser budget. N=2 matches the checklist's "at most half the slots".
	anonShareDivisor = 2
)

func defaultParserMaxConcurrency() int {
	n := runtime.NumCPU() - 1
	if n < 2 {
		n = 2
	}
	return n
}

func newParserLimiterFromEnv() *parserLimiter {
	total := envInt("FINNESTDB_PARSER_MAX_CONCURRENCY", defaultParserMaxConcurrency())
	if total < 1 {
		total = 1
	}
	timeoutMS := envInt("FINNESTDB_PARSER_QUEUE_TIMEOUT_MS", defaultParserQueueTimeoutMS)
	if timeoutMS < 0 {
		timeoutMS = 0
	}
	return newParserLimiter(total, time.Duration(timeoutMS)*time.Millisecond)
}

func newParserLimiter(total int, timeout time.Duration) *parserLimiter {
	if total < 1 {
		total = 1
	}
	anonSlots := total / anonShareDivisor
	if anonSlots < 1 {
		anonSlots = 1
	}
	return &parserLimiter{
		total:   make(chan struct{}, total),
		anon:    make(chan struct{}, anonSlots),
		timeout: timeout,
	}
}

// acquire blocks until a parser slot is free or the queue timeout elapses.
// Anonymous callers (auth == nil) must additionally win a slot from the
// smaller anonymous sub-pool, so they shed before signed-in traffic exhausts
// the shared pool. It returns a release func and whether the acquisition
// succeeded; on failure the caller must respond 503 + Retry-After without
// calling release.
func (p *parserLimiter) acquire(ctx context.Context, anonymous bool) (release func(), ok bool) {
	if p == nil {
		return func() {}, true
	}

	deadlineCtx := ctx
	var cancel context.CancelFunc
	if p.timeout > 0 {
		deadlineCtx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	if anonymous {
		select {
		case p.anon <- struct{}{}:
		case <-deadlineCtx.Done():
			return nil, false
		}
		select {
		case p.total <- struct{}{}:
			return func() {
				<-p.total
				<-p.anon
			}, true
		case <-deadlineCtx.Done():
			<-p.anon
			return nil, false
		}
	}

	select {
	case p.total <- struct{}{}:
		return func() { <-p.total }, true
	case <-deadlineCtx.Done():
		return nil, false
	}
}

// retryAfterSeconds is the Retry-After value (seconds) sent with 429/503
// responses. It is intentionally small and fixed rather than computed from
// queue depth: callers just need a signal to back off and retry shortly, not
// a precise ETA.
const retryAfterSeconds = 2

// writeServiceUnavailable is the 503 counterpart to allowLimiter's 429: the
// server is healthy but momentarily out of parser capacity. Both carry
// Retry-After so clients get one consistent retry signal regardless of which
// kind of backpressure they hit.
func writeServiceUnavailable(w http.ResponseWriter, reason string) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error":       reason,
		"retry_after": retryAfterSeconds,
	})
}
