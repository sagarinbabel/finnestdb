package api

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPIgnoresUntrustedForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.20:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("X-Real-IP", "203.0.113.11")

	if got := clientIP(req); got != "198.51.100.20" {
		t.Fatalf("clientIP=%q want remote address", got)
	}
}

func TestClientIPTrustsForwardedHeadersFromLocalProxy(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 198.51.100.30")

	if got := clientIP(req); got != "198.51.100.30" {
		t.Fatalf("clientIP=%q want rightmost forwarded address", got)
	}
}

func TestClientIPTrustsForwardedHeadersWhenEnabled(t *testing.T) {
	t.Setenv("FINNESTDB_TRUST_FORWARD_HEADERS", "1")

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.20:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 198.51.100.30")

	if got := clientIP(req); got != "198.51.100.30" {
		t.Fatalf("clientIP=%q want rightmost forwarded address", got)
	}
}
