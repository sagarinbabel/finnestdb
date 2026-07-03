package api

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateLimitSet struct {
	loginIP         *fixedWindowRateLimiter
	loginEmail      *fixedWindowRateLimiter
	registerIP      *fixedWindowRateLimiter
	registerEmail   *fixedWindowRateLimiter
	parseIP         *fixedWindowRateLimiter
	parseAccount    *fixedWindowRateLimiter
	feedbackIP      *fixedWindowRateLimiter
	feedbackAccount *fixedWindowRateLimiter
}

func newRateLimitSetFromEnv() *rateLimitSet {
	window := time.Minute
	return &rateLimitSet{
		loginIP:         newFixedWindowRateLimiter(envInt("FINNESTDB_RATE_LIMIT_LOGIN_PER_MINUTE", 10), window),
		loginEmail:      newFixedWindowRateLimiter(envInt("FINNESTDB_RATE_LIMIT_LOGIN_ACCOUNT_PER_MINUTE", 10), window),
		registerIP:      newFixedWindowRateLimiter(envInt("FINNESTDB_RATE_LIMIT_REGISTER_PER_MINUTE", 5), window),
		registerEmail:   newFixedWindowRateLimiter(envInt("FINNESTDB_RATE_LIMIT_REGISTER_ACCOUNT_PER_MINUTE", 3), window),
		parseIP:         newFixedWindowRateLimiter(envInt("FINNESTDB_RATE_LIMIT_PARSE_PER_MINUTE", 60), window),
		parseAccount:    newFixedWindowRateLimiter(envInt("FINNESTDB_RATE_LIMIT_PARSE_ACCOUNT_PER_MINUTE", 120), window),
		feedbackIP:      newFixedWindowRateLimiter(envInt("FINNESTDB_RATE_LIMIT_FEEDBACK_PER_MINUTE", 20), window),
		feedbackAccount: newFixedWindowRateLimiter(envInt("FINNESTDB_RATE_LIMIT_FEEDBACK_ACCOUNT_PER_MINUTE", 20), window),
	}
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("warn: invalid %s=%q, using %d", name, raw, fallback)
		return fallback
	}
	return n
}

func (r *rateLimitSet) allowLogin(w http.ResponseWriter, req *http.Request, email string) bool {
	if r == nil {
		return true
	}
	return allowLimiter(w, r.loginIP, "auth-login", "ip:"+clientIP(req)) &&
		allowLimiter(w, r.loginEmail, "auth-login", "email:"+email)
}

func (r *rateLimitSet) allowRegister(w http.ResponseWriter, req *http.Request, email string) bool {
	if r == nil {
		return true
	}
	return allowLimiter(w, r.registerIP, "auth-register", "ip:"+clientIP(req)) &&
		allowLimiter(w, r.registerEmail, "auth-register", "email:"+email)
}

func (r *rateLimitSet) allowParse(w http.ResponseWriter, req *http.Request, auth *AuthContext) bool {
	if r == nil {
		return true
	}
	if !allowLimiter(w, r.parseIP, "parse", "ip:"+clientIP(req)) {
		return false
	}
	if auth != nil {
		return allowLimiter(w, r.parseAccount, "parse", "user:"+strconv.FormatInt(auth.UserID, 10))
	}
	return true
}

func (r *rateLimitSet) allowFeedback(w http.ResponseWriter, req *http.Request, auth *AuthContext) bool {
	if r == nil {
		return true
	}
	return allowLimiter(w, r.feedbackIP, "parse-feedback", "ip:"+clientIP(req)) &&
		allowLimiter(w, r.feedbackAccount, "parse-feedback", "user:"+strconv.FormatInt(auth.UserID, 10))
}

func allowLimiter(w http.ResponseWriter, limiter *fixedWindowRateLimiter, route, key string) bool {
	if limiter == nil || limiter.allow(key) {
		return true
	}
	log.Printf("rate limit rejected: route=%s key=%s", route, rateLimitLogKey(key))
	// Rate-limit windows are fixed at one minute (see newRateLimitSetFromEnv),
	// so a minute is the honest worst-case retry signal; the caller may
	// succeed sooner if their window started partway through, but per-key
	// window start times are not tracked separately from the reset counter.
	w.Header().Set("Retry-After", "60")
	http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
	return false
}

func rateLimitLogKey(key string) string {
	kind := "key"
	if prefix, _, ok := strings.Cut(key, ":"); ok && prefix != "" {
		kind = prefix
	}
	sum := sha256.Sum256([]byte(key))
	return kind + ":" + hex.EncodeToString(sum[:6])
}

func clientIP(r *http.Request) string {
	remoteIP := remoteAddrIP(r.RemoteAddr)
	if trustForwardHeaders(remoteIP) {
		if forwarded := rightmostForwardedIP(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			return forwarded
		}
		if realIP := validHeaderIP(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}
	}
	if remoteIP != "" {
		return remoteIP
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func remoteAddrIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
		return ip.String()
	}
	return strings.TrimSpace(host)
}

func trustForwardHeaders(remoteIP string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FINNESTDB_TRUST_FORWARD_HEADERS"))) {
	case "1", "true", "yes", "on":
		return true
	}
	ip := net.ParseIP(remoteIP)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

func rightmostForwardedIP(header string) string {
	parts := strings.Split(header, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if ip := validHeaderIP(parts[i]); ip != "" {
			return ip
		}
	}
	return ""
}

func validHeaderIP(header string) string {
	ip := net.ParseIP(strings.TrimSpace(header))
	if ip == nil {
		return ""
	}
	return ip.String()
}

type fixedWindowRateLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	lastPrune time.Time
	entries   map[string]rateLimitEntry
}

type rateLimitEntry struct {
	count      int
	windowEnds time.Time
}

func newFixedWindowRateLimiter(limit int, window time.Duration) *fixedWindowRateLimiter {
	return &fixedWindowRateLimiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]rateLimitEntry),
	}
}

func (r *fixedWindowRateLimiter) allow(key string) bool {
	if r == nil || r.limit <= 0 {
		return true
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.lastPrune.IsZero() || now.Sub(r.lastPrune) >= r.window {
		for key, entry := range r.entries {
			if !now.Before(entry.windowEnds) {
				delete(r.entries, key)
			}
		}
		r.lastPrune = now
	}

	entry := r.entries[key]
	if entry.windowEnds.IsZero() || !now.Before(entry.windowEnds) {
		r.entries[key] = rateLimitEntry{count: 1, windowEnds: now.Add(r.window)}
		return true
	}
	if entry.count >= r.limit {
		return false
	}
	entry.count++
	r.entries[key] = entry
	return true
}
