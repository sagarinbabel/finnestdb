package main

import (
	"testing"
	"time"
)

// TestPercentileBoundaries proves the percentile helper picks the right
// index at both ends — an off-by-one here would silently misreport p95/p99
// in every report this tool produces, which is the entire point of the tool.
func TestPercentileBoundaries(t *testing.T) {
	sorted := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		100 * time.Millisecond,
	}
	if got := percentile(sorted, 0.0); got != 10*time.Millisecond {
		t.Fatalf("p0=%v want 10ms", got)
	}
	if got := percentile(sorted, 1.0); got != 100*time.Millisecond {
		t.Fatalf("p100=%v want 100ms", got)
	}
	if got := percentile(nil, 0.5); got != 0 {
		t.Fatalf("percentile of empty slice=%v want 0", got)
	}
}

// TestAssignVUKindRespectsRatios confirms the deterministic round-robin
// split produces (approximately) the configured anon/signed/review mix, so a
// given -concurrency always reproduces the same traffic shape across staged
// runs (important for comparing e.g. the 200 vs 500 vs 1000 VU stages in the
// launch load-test report).
func TestAssignVUKindRespectsRatios(t *testing.T) {
	cfg := config{
		concurrency:      100,
		anonParseRatio:   0.5,
		signedParseRatio: 0.3,
		reviewReadRatio:  0.2,
	}
	var anon, signed, review int
	for i := 0; i < cfg.concurrency; i++ {
		switch assignVUKind(i, cfg) {
		case kindAnonParse:
			anon++
		case kindSignedParse:
			signed++
		case kindReviewRead:
			review++
		}
	}
	if anon != 50 {
		t.Fatalf("anon=%d want 50", anon)
	}
	if signed != 30 {
		t.Fatalf("signed=%d want 30", signed)
	}
	if review != 20 {
		t.Fatalf("review=%d want 20", review)
	}
}

// TestAssignVUKindAllVirtualUsersClassified guards against a VU silently
// falling through unclassified (which would make it a no-op goroutine that
// never generates load, quietly understating the requested concurrency).
func TestAssignVUKindAllVirtualUsersClassified(t *testing.T) {
	cfg := config{concurrency: 37, anonParseRatio: 0.5, signedParseRatio: 0.3, reviewReadRatio: 0.2}
	for i := 0; i < cfg.concurrency; i++ {
		switch assignVUKind(i, cfg) {
		case kindAnonParse, kindSignedParse, kindReviewRead:
			// classified
		default:
			t.Fatalf("VU %d got an unrecognized kind", i)
		}
	}
}
