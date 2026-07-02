package api

import (
	"net/http"
	"testing"
	"time"
)

func TestClientIPIgnoresForwardedHeadersFromUntrustedRemote(t *testing.T) {
	t.Setenv("FINNESTDB_TRUST_FORWARD_HEADERS", "")
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.RemoteAddr = "198.51.100.20:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 203.0.113.11")
	req.Header.Set("X-Real-IP", "203.0.113.12")

	if got := clientIP(req); got != "198.51.100.20" {
		t.Fatalf("clientIP=%q want remote address", got)
	}
}

func TestClientIPUsesRightmostForwardedForFromTrustedProxy(t *testing.T) {
	t.Setenv("FINNESTDB_TRUST_FORWARD_HEADERS", "")
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.RemoteAddr = "10.0.0.5:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.200, 198.51.100.30")
	req.Header.Set("X-Real-IP", "203.0.113.201")

	if got := clientIP(req); got != "198.51.100.30" {
		t.Fatalf("clientIP=%q want rightmost X-Forwarded-For entry", got)
	}
}

func TestClientIPUsesRealIPFromTrustedProxy(t *testing.T) {
	t.Setenv("FINNESTDB_TRUST_FORWARD_HEADERS", "")
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.RemoteAddr = "127.0.0.1:4321"
	req.Header.Set("X-Real-IP", "203.0.113.12")

	if got := clientIP(req); got != "203.0.113.12" {
		t.Fatalf("clientIP=%q want X-Real-IP from trusted proxy", got)
	}
}

func TestClientIPSkipsMalformedForwardedHeaders(t *testing.T) {
	t.Setenv("FINNESTDB_TRUST_FORWARD_HEADERS", "")
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.RemoteAddr = "10.0.0.5:4321"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	req.Header.Set("X-Real-IP", "also-not-an-ip")

	if got := clientIP(req); got != "10.0.0.5" {
		t.Fatalf("clientIP=%q want trusted proxy remote address", got)
	}
}

func TestClientIPTrustForwardHeadersOverride(t *testing.T) {
	t.Setenv("FINNESTDB_TRUST_FORWARD_HEADERS", "true")
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.RemoteAddr = "198.51.100.20:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 203.0.113.11")

	if got := clientIP(req); got != "203.0.113.11" {
		t.Fatalf("clientIP=%q want rightmost X-Forwarded-For entry with explicit trust", got)
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
