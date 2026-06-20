package api

import (
	"net/http"
	"testing"
	"time"
)

func TestClientIPIgnoresForwardedHeadersFromUntrustedRemote(t *testing.T) {
	t.Setenv("FINNESTDB_TRUST_FORWARD_HEADERS", "")
	req := &http.Request{
		RemoteAddr: "198.51.100.20:4321",
		Header: http.Header{
			"X-Forwarded-For": []string{"203.0.113.10, 203.0.113.11"},
			"X-Real-IP":       []string{"203.0.113.12"},
		},
	}

	if got := clientIP(req); got != "198.51.100.20" {
		t.Fatalf("clientIP=%q want remote address", got)
	}
}

func TestClientIPUsesRightmostForwardedForFromTrustedProxy(t *testing.T) {
	t.Setenv("FINNESTDB_TRUST_FORWARD_HEADERS", "")
	req := &http.Request{
		RemoteAddr: "10.0.0.5:4321",
		Header: http.Header{
			"X-Forwarded-For": []string{"203.0.113.200, 198.51.100.30"},
			"X-Real-IP":       []string{"203.0.113.201"},
		},
	}

	if got := clientIP(req); got != "198.51.100.30" {
		t.Fatalf("clientIP=%q want rightmost X-Forwarded-For entry", got)
	}
}

func TestFixedWindowRateLimiterPrunesExpiredEntries(t *testing.T) {
	limiter := newFixedWindowRateLimiter(10, time.Minute)
	limiter.entries["expired"] = rateLimitEntry{
		count:      1,
		windowEnds: time.Now().Add(-time.Minute),
	}

	if !limiter.allow("fresh") {
		t.Fatal("fresh key should be allowed")
	}
	if _, ok := limiter.entries["expired"]; ok {
		t.Fatal("expired rate limit entry was not pruned")
	}
}
