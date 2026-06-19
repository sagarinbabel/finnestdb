package api

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPIgnoresForwardedHeadersFromDirectClient(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/parse", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	req.Header.Set("X-Real-IP", "198.51.100.2")

	if got := clientIP(req); got != "203.0.113.10" {
		t.Fatalf("clientIP=%q want remote address", got)
	}
}

func TestClientIPTrustsRightmostForwardedIPFromPrivateProxy(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/parse", nil)
	req.RemoteAddr = "10.0.0.5:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.1, 203.0.113.20")

	if got := clientIP(req); got != "203.0.113.20" {
		t.Fatalf("clientIP=%q want rightmost forwarded IP", got)
	}
}

func TestClientIPCanTrustForwardedHeadersByEnv(t *testing.T) {
	t.Setenv("FINNESTDB_TRUST_FORWARD_HEADERS", "true")

	req := httptest.NewRequest("POST", "/api/parse", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Real-IP", "198.51.100.2")

	if got := clientIP(req); got != "198.51.100.2" {
		t.Fatalf("clientIP=%q want X-Real-IP with explicit trust", got)
	}
}
