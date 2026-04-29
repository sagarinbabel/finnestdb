package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/store"
)

func newTestAPI(t *testing.T) *API {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}

	return NewAPI(db)
}

func newTestMux(t *testing.T, api *API) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	api.SetupRoutes(mux)
	return mux
}

func TestHandleParseRejectsInvalidRequests(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "wrong method",
			method:     http.MethodGet,
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   "Method not allowed",
		},
		{
			name:       "invalid json",
			method:     http.MethodPost,
			body:       "{",
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid request",
		},
		{
			name:       "invalid language",
			method:     http.MethodPost,
			body:       `{"lang":"SV","text":"hei"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Language must be FI or ET",
		},
		{
			name:       "missing text",
			method:     http.MethodPost,
			body:       `{"lang":"FI","text":""}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Text is required",
		},
		{
			name:       "unsupported parser",
			method:     http.MethodPost,
			body:       `{"lang":"FI","text":"kissa","parser":"bogus"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `unsupported parser "bogus"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/parse", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status=%d want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body=%q does not contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHandleParseReturnsJSONResponse(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		if parser == "" {
			parser = "basic"
		}
		return &parsecore.ParseResult{
			Lang:            lang,
			Parser:          parser,
			TotalTokens:     3,
			ParseDurationMs: 17,
			Stats: parsecore.ParseStats{
				UniqueForms:      2,
				TotalSentences:   1,
				ResolvedTokens:   2,
				UnresolvedTokens: 1,
				SourceCounts: map[string]int{
					"dict": 2,
					"stub": 1,
				},
				Timings: parsecore.ParseTimings{
					AnalyzeMs:          5,
					LookupFormsMs:      4,
					LookupGlossesMs:    3,
					ResolveSentencesMs: 2,
					EnrichWordsMs:      1,
					TotalMs:            17,
				},
			},
			Words: []parsecore.WordEntry{
				{Lemma: "kissa", POS: "NOUN", Forms: []string{"Kissa"}, Count: 1, Gloss: "cat"},
				{Lemma: "juosta", POS: "VERB", Forms: []string{"juoksee"}, Count: 1, Gloss: "run"},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"FI","text":"Kissa juoksee."}`))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type=%q want application/json", got)
	}

	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Lang != "FI" {
		t.Fatalf("lang=%q want FI", resp.Lang)
	}
	if resp.TotalTokens != 3 {
		t.Fatalf("total_tokens=%d want 3", resp.TotalTokens)
	}
	if resp.ParseDurationMs != 17 {
		t.Fatalf("parse_duration_ms=%d want 17", resp.ParseDurationMs)
	}
	if resp.Stats.UniqueForms != 2 {
		t.Fatalf("stats.unique_forms=%d want 2", resp.Stats.UniqueForms)
	}
	if resp.Stats.ResolvedTokens != 2 {
		t.Fatalf("stats.resolved_tokens=%d want 2", resp.Stats.ResolvedTokens)
	}
	if resp.Stats.Timings.TotalMs != 17 {
		t.Fatalf("stats.timings.total_ms=%d want 17", resp.Stats.Timings.TotalMs)
	}
	if len(resp.Words) != 2 {
		t.Fatalf("words=%d want 2", len(resp.Words))
	}
	if resp.Words[0].Lemma != "kissa" {
		t.Fatalf("first lemma=%q want kissa", resp.Words[0].Lemma)
	}
}

func TestHandleParseMapsAnalyzerValidationErrorsToBadRequest(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, _, _, _ string) (*parsecore.ParseResult, error) {
		return nil, fmt.Errorf("text exceeds 300000 character limit")
	}
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"FI","text":"x"}`))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "text exceeds 300000 character limit") {
		t.Fatalf("body=%q missing analyzer error", rec.Body.String())
	}
}

func TestStubRoutesRemainReachable(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"test@example.com","password":"secret"}`))
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d want %d", loginRec.Code, http.StatusOK)
	}
	if len(loginRec.Result().Cookies()) == 0 {
		t.Fatal("expected login to set a session cookie")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	for _, cookie := range loginRec.Result().Cookies() {
		meReq.AddCookie(cookie)
	}
	meRec := httptest.NewRecorder()
	mux.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("me status=%d want %d", meRec.Code, http.StatusOK)
	}
	var meResp MeResponse
	if err := json.NewDecoder(bytes.NewReader(meRec.Body.Bytes())).Decode(&meResp); err != nil {
		t.Fatalf("decode /api/me response: %v", err)
	}
	if !meResp.Authenticated {
		t.Fatal("expected authenticated /api/me response")
	}
	if meResp.User == nil || meResp.User.Email != "test@example.com" {
		t.Fatalf("unexpected user payload: %+v", meResp.User)
	}
	if meResp.User.IsAdmin {
		t.Fatal("expected default test user to be non-admin")
	}
	if meResp.Dashboard == nil {
		t.Fatal("expected dashboard payload for authenticated user")
	}

	reviewReq := httptest.NewRequest(http.MethodGet, "/api/review/next", nil)
	reviewRec := httptest.NewRecorder()
	mux.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("review next status=%d want %d", reviewRec.Code, http.StatusOK)
	}
	if got := reviewRec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("review next content-type=%q want application/json", got)
	}
}

func TestHandleLoginReturnsAuthIdentity(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"alice@example.com","password":"secret"}`))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected login to set a session cookie")
	}

	var resp LoginResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if !resp.Authenticated {
		t.Fatal("expected authenticated login response")
	}
	if resp.User == nil {
		t.Fatal("expected user payload in login response")
	}
	if resp.User.Email != "alice@example.com" {
		t.Fatalf("email=%q want alice@example.com", resp.User.Email)
	}
	if resp.User.IsAdmin {
		t.Fatal("expected regular login to be non-admin")
	}
}

func TestHandleMeReturnsAnonymousStateWithoutCookie(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp MeResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode /api/me response: %v", err)
	}
	if resp.Authenticated {
		t.Fatal("expected anonymous response")
	}
	if resp.User != nil {
		t.Fatalf("expected nil user for anonymous response, got %+v", resp.User)
	}
	if resp.Dashboard != nil {
		t.Fatal("expected no dashboard payload for anonymous response")
	}
}

func TestHandleMeTreatsUnknownCookieUserAsAnonymous(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "user_id", Value: "999999"})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp MeResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode /api/me response: %v", err)
	}
	if resp.Authenticated {
		t.Fatal("expected unknown-cookie user to be treated as anonymous")
	}
	if resp.User != nil {
		t.Fatalf("expected nil user for unknown-cookie response, got %+v", resp.User)
	}
}

func TestHandleMeReturnsAdminState(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")

	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "admin@example.com")

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp MeResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode /api/me response: %v", err)
	}
	if !resp.Authenticated || resp.User == nil {
		t.Fatalf("expected authenticated admin response, got %+v", resp)
	}
	if !resp.User.IsAdmin {
		t.Fatal("expected admin user")
	}
}

func TestRequireAuthRejectsAnonymousUser(t *testing.T) {
	api := newTestAPI(t)
	handler := api.requireAuth(func(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Authentication required") {
		t.Fatalf("body=%q missing auth error", rec.Body.String())
	}
}

func TestRequireAdminRejectsNonAdminUser(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "user@example.com")

	handler := api.requireAdmin(func(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Admin access required") {
		t.Fatalf("body=%q missing admin error", rec.Body.String())
	}
}

func loginAndReturnCookies(t *testing.T, mux *http.ServeMux, email string) []*http.Cookie {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(fmt.Sprintf(`{"email":"%s","password":"secret"}`, email)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected login to set cookies")
	}
	return cookies
}
