package api

import (
	"net/http"
	"testing"
)

func TestClientIPIgnoresForwardedHeadersFromPublicRemote(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.RemoteAddr = "198.51.100.10:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.Header.Set("X-Real-IP", "203.0.113.51")

	if got := clientIP(req); got != "198.51.100.10" {
		t.Fatalf("clientIP=%q want public remote address", got)
	}
}

func TestClientIPUsesForwardedHeadersFromTrustedProxy(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.RemoteAddr = "10.0.0.8:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.8")

	if got := clientIP(req); got != "203.0.113.50" {
		t.Fatalf("clientIP=%q want first forwarded address", got)
	}
}

func TestClientIPUsesRealIPFromTrustedProxy(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Real-IP", "203.0.113.51")

	if got := clientIP(req); got != "203.0.113.51" {
		t.Fatalf("clientIP=%q want real IP header", got)
	}
}

func TestClientIPRejectsMalformedForwardedHeader(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.RemoteAddr = "10.0.0.8:54321"
	req.Header.Set("X-Forwarded-For", "not-an-ip")

	if got := clientIP(req); got != "10.0.0.8" {
		t.Fatalf("clientIP=%q want proxy remote address", got)
	}
}
