package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
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
	if resp.ParseID != nil {
		t.Fatalf("parse_id=%d want nil for anonymous parse", *resp.ParseID)
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

func TestHandleParseCreatesParseSessionForAuthenticatedUser(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		if parser == "" {
			parser = "custom"
		}
		return &parsecore.ParseResult{
			Lang:            lang,
			Parser:          parser,
			TotalTokens:     2,
			ParseDurationMs: 11,
			Stats:           parsecore.ParseStats{},
			Words: []parsecore.WordEntry{
				{Lemma: "kissa", POS: "NOUN", Forms: []string{"kissa"}, Count: 1},
			},
		}, nil
	}
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "parse-user@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"FI","text":"kissa","parser":"custom"}`))
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ParseID == nil || *resp.ParseID == 0 {
		t.Fatalf("parse_id=%v want non-nil", resp.ParseID)
	}

	session, err := api.store.GetParseSession(*resp.ParseID)
	if err != nil {
		t.Fatalf("GetParseSession: %v", err)
	}
	if session.UserID == nil || *session.UserID == 0 {
		t.Fatalf("expected parse session user_id to be recorded, got %+v", session.UserID)
	}
	if session.SourceText != "kissa" {
		t.Fatalf("source_text=%q want kissa", session.SourceText)
	}
}

func TestHandleParseHydratesLemmaStateForAuthenticatedUser(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, _, parser string) (*parsecore.ParseResult, error) {
		if parser == "" {
			parser = "custom"
		}
		return &parsecore.ParseResult{
			Lang:            lang,
			Parser:          parser,
			TotalTokens:     2,
			ParseDurationMs: 11,
			Stats:           parsecore.ParseStats{},
			Words: []parsecore.WordEntry{
				{Lemma: "kissa", POS: "NOUN", Forms: []string{"kissa"}, Count: 1},
				{Lemma: "juosta", POS: "VERB", Forms: []string{"juoksen"}, Count: 1},
			},
		}, nil
	}
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "parse-state@example.com")

	markKnownReq := httptest.NewRequest(http.MethodPost, "/api/lemma-state", strings.NewReader(`{"lang":"FI","lemma":"kissa","pos":"NOUN","status":"known"}`))
	for _, cookie := range cookies {
		markKnownReq.AddCookie(cookie)
	}
	markKnownRec := httptest.NewRecorder()
	mux.ServeHTTP(markKnownRec, markKnownReq)
	if markKnownRec.Code != http.StatusOK {
		t.Fatalf("mark known status=%d want %d body=%q", markKnownRec.Code, http.StatusOK, markKnownRec.Body.String())
	}

	markIgnoredReq := httptest.NewRequest(http.MethodPost, "/api/lemma-state", strings.NewReader(`{"lang":"FI","lemma":"juosta","pos":"VERB","status":"ignored"}`))
	for _, cookie := range cookies {
		markIgnoredReq.AddCookie(cookie)
	}
	markIgnoredRec := httptest.NewRecorder()
	mux.ServeHTTP(markIgnoredRec, markIgnoredReq)
	if markIgnoredRec.Code != http.StatusOK {
		t.Fatalf("mark ignored status=%d want %d body=%q", markIgnoredRec.Code, http.StatusOK, markIgnoredRec.Body.String())
	}

	parseReq := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"FI","text":"kissa juoksen","parser":"custom"}`))
	for _, cookie := range cookies {
		parseReq.AddCookie(cookie)
	}
	parseRec := httptest.NewRecorder()
	mux.ServeHTTP(parseRec, parseReq)
	if parseRec.Code != http.StatusOK {
		t.Fatalf("parse status=%d want %d body=%q", parseRec.Code, http.StatusOK, parseRec.Body.String())
	}

	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(parseRec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode parse response: %v", err)
	}
	byLemma := map[string]WordEntry{}
	for _, word := range resp.Words {
		byLemma[word.Lemma] = word
	}
	if byLemma["kissa"].LearningState != "known" {
		t.Fatalf("kissa learning_state=%q want known", byLemma["kissa"].LearningState)
	}
	if byLemma["juosta"].LearningState != "ignored" {
		t.Fatalf("juosta learning_state=%q want ignored", byLemma["juosta"].LearningState)
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

func TestAuthenticatedReviewNextReturnsNoContentWithoutDueCards(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	cookies := loginAndReturnCookies(t, mux, "test@example.com")

	reviewReq := httptest.NewRequest(http.MethodGet, "/api/review/next", nil)
	for _, cookie := range cookies {
		reviewReq.AddCookie(cookie)
	}
	reviewRec := httptest.NewRecorder()
	mux.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusNoContent {
		t.Fatalf("review next status=%d want %d body=%q", reviewRec.Code, http.StatusNoContent, reviewRec.Body.String())
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

func TestHandleLogoutClearsSessionCookieAndDropsAuth(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"alice@example.com","password":"secret"}`))
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d want %d", loginRec.Code, http.StatusOK)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected login to set a session cookie")
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	for _, c := range cookies {
		logoutReq.AddCookie(c)
	}
	logoutRec := httptest.NewRecorder()
	mux.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("logout status=%d want %d body=%q", logoutRec.Code, http.StatusOK, logoutRec.Body.String())
	}

	var clearing *http.Cookie
	for _, c := range logoutRec.Result().Cookies() {
		if c.Name == "user_id" {
			clearing = c
		}
	}
	if clearing == nil {
		t.Fatal("expected logout response to set the user_id cookie")
	}
	if clearing.MaxAge >= 0 {
		t.Fatalf("expected user_id cookie to be expired (MaxAge<0), got MaxAge=%d", clearing.MaxAge)
	}
	if clearing.Value != "" {
		t.Fatalf("expected cleared cookie value, got %q", clearing.Value)
	}

	// Hitting /api/me with the now-expired cookie applied (browser would not send it)
	// must report unauthenticated. Simulate by replaying only the clearing cookie.
	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meReq.AddCookie(clearing)
	meRec := httptest.NewRecorder()
	mux.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("me status=%d want %d", meRec.Code, http.StatusOK)
	}
	var meResp MeResponse
	if err := json.NewDecoder(bytes.NewReader(meRec.Body.Bytes())).Decode(&meResp); err != nil {
		t.Fatalf("decode /api/me response: %v", err)
	}
	if meResp.Authenticated {
		t.Fatal("expected /api/me to be unauthenticated after logout")
	}
}

func TestHandleLogoutRejectsGet(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/logout", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusMethodNotAllowed)
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

func TestKnownWordsRequiresAuthentication(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/known-words?lang=FI", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestKnownWordsImportListAndDelete(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("kissa", "NOUN", "cat", "FI"); err != nil {
		t.Fatalf("UpsertLemma: %v", err)
	}
	if err := api.store.UpsertForm("kissoja", "kissa", "NOUN", "FI"); err != nil {
		t.Fatalf("UpsertForm: %v", err)
	}
	if err := api.store.UpsertLemma("juosta", "VERB", "run", "FI"); err != nil {
		t.Fatalf("UpsertLemma: %v", err)
	}
	if err := api.store.UpsertForm("juoksen", "juosta", "VERB", "FI"); err != nil {
		t.Fatalf("UpsertForm: %v", err)
	}

	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "known@example.com")

	importReq := httptest.NewRequest(http.MethodPost, "/api/known-words", strings.NewReader(`{"lang":"FI","words":["kissoja","juoksen","tuntematon","juoksen"]}`))
	for _, cookie := range cookies {
		importReq.AddCookie(cookie)
	}
	importRec := httptest.NewRecorder()
	mux.ServeHTTP(importRec, importReq)

	if importRec.Code != http.StatusOK {
		t.Fatalf("import status=%d want %d body=%q", importRec.Code, http.StatusOK, importRec.Body.String())
	}

	var importResp KnownWordsResponse
	if err := json.NewDecoder(bytes.NewReader(importRec.Body.Bytes())).Decode(&importResp); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if len(importResp.Imported) != 2 {
		t.Fatalf("imported=%d want 2 (%+v)", len(importResp.Imported), importResp.Imported)
	}
	if len(importResp.Unresolved) != 1 || importResp.Unresolved[0] != "tuntematon" {
		t.Fatalf("unexpected unresolved payload: %+v", importResp.Unresolved)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/known-words?lang=FI", nil)
	for _, cookie := range cookies {
		listReq.AddCookie(cookie)
	}
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d want %d body=%q", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var listResp KnownWordsListResponse
	if err := json.NewDecoder(bytes.NewReader(listRec.Body.Bytes())).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.KnownWords) != 2 {
		t.Fatalf("known_words=%d want 2 (%+v)", len(listResp.KnownWords), listResp.KnownWords)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/known-words?lang=FI&lemma=kissa&pos=NOUN", nil)
	for _, cookie := range cookies {
		deleteReq.AddCookie(cookie)
	}
	deleteRec := httptest.NewRecorder()
	mux.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status=%d want %d body=%q", deleteRec.Code, http.StatusOK, deleteRec.Body.String())
	}

	listAgainReq := httptest.NewRequest(http.MethodGet, "/api/known-words?lang=FI", nil)
	for _, cookie := range cookies {
		listAgainReq.AddCookie(cookie)
	}
	listAgainRec := httptest.NewRecorder()
	mux.ServeHTTP(listAgainRec, listAgainReq)

	var listAgainResp KnownWordsListResponse
	if err := json.NewDecoder(bytes.NewReader(listAgainRec.Body.Bytes())).Decode(&listAgainResp); err != nil {
		t.Fatalf("decode second list response: %v", err)
	}
	if len(listAgainResp.KnownWords) != 1 {
		t.Fatalf("known_words=%d want 1 (%+v)", len(listAgainResp.KnownWords), listAgainResp.KnownWords)
	}
	if listAgainResp.KnownWords[0].Lemma != "juosta" {
		t.Fatalf("remaining lemma=%q want juosta", listAgainResp.KnownWords[0].Lemma)
	}
}

func TestLemmaStateMarksKnownAndIgnored(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "lemma-state@example.com")
	user, err := api.store.GetOrCreateUser("lemma-state@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	markKnownReq := httptest.NewRequest(http.MethodPost, "/api/lemma-state", strings.NewReader(`{"lang":"FI","lemma":"kissa","pos":"NOUN","status":"known"}`))
	for _, cookie := range cookies {
		markKnownReq.AddCookie(cookie)
	}
	markKnownRec := httptest.NewRecorder()
	mux.ServeHTTP(markKnownRec, markKnownReq)
	if markKnownRec.Code != http.StatusOK {
		t.Fatalf("mark known status=%d want %d body=%q", markKnownRec.Code, http.StatusOK, markKnownRec.Body.String())
	}

	isKnownOrIgnored, err := api.store.IsKnownOrIgnored(user.ID, "FI", "kissa", "NOUN")
	if err != nil {
		t.Fatalf("IsKnownOrIgnored: %v", err)
	}
	if !isKnownOrIgnored {
		t.Fatalf("expected kissa/NOUN to be known or ignored")
	}

	markIgnoredReq := httptest.NewRequest(http.MethodPost, "/api/lemma-state", strings.NewReader(`{"lang":"FI","lemma":"kissa","pos":"NOUN","status":"ignored"}`))
	for _, cookie := range cookies {
		markIgnoredReq.AddCookie(cookie)
	}
	markIgnoredRec := httptest.NewRecorder()
	mux.ServeHTTP(markIgnoredRec, markIgnoredReq)
	if markIgnoredRec.Code != http.StatusOK {
		t.Fatalf("mark ignored status=%d want %d body=%q", markIgnoredRec.Code, http.StatusOK, markIgnoredRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/known-words?lang=FI", nil)
	for _, cookie := range cookies {
		listReq.AddCookie(cookie)
	}
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	var listResp KnownWordsListResponse
	if err := json.NewDecoder(bytes.NewReader(listRec.Body.Bytes())).Decode(&listResp); err != nil {
		t.Fatalf("decode known words: %v", err)
	}
	if len(listResp.KnownWords) != 0 {
		t.Fatalf("known_words=%d want 0 after ignore (%+v)", len(listResp.KnownWords), listResp.KnownWords)
	}
}

func TestLemmaStateRequiresAuthentication(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/lemma-state", strings.NewReader(`{"lang":"FI","lemma":"kissa","pos":"NOUN","status":"known"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestKnownWordsImportReturnsUnresolvedWithoutCreatingRows(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "unknown@example.com")

	importReq := httptest.NewRequest(http.MethodPost, "/api/known-words", strings.NewReader(`{"lang":"FI","words":["tuntematon","mysteeri"]}`))
	for _, cookie := range cookies {
		importReq.AddCookie(cookie)
	}
	importRec := httptest.NewRecorder()
	mux.ServeHTTP(importRec, importReq)

	if importRec.Code != http.StatusOK {
		t.Fatalf("import status=%d want %d body=%q", importRec.Code, http.StatusOK, importRec.Body.String())
	}

	var importResp KnownWordsResponse
	if err := json.NewDecoder(bytes.NewReader(importRec.Body.Bytes())).Decode(&importResp); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if len(importResp.Imported) != 0 {
		t.Fatalf("imported=%d want 0 (%+v)", len(importResp.Imported), importResp.Imported)
	}
	if len(importResp.Unresolved) != 2 {
		t.Fatalf("unresolved=%d want 2 (%+v)", len(importResp.Unresolved), importResp.Unresolved)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/known-words?lang=FI", nil)
	for _, cookie := range cookies {
		listReq.AddCookie(cookie)
	}
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)

	var listResp KnownWordsListResponse
	if err := json.NewDecoder(bytes.NewReader(listRec.Body.Bytes())).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.KnownWords) != 0 {
		t.Fatalf("known_words=%d want 0 (%+v)", len(listResp.KnownWords), listResp.KnownWords)
	}
}

func TestKnownWordsAreIsolatedByLanguage(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("kissa", "NOUN", "cat", "FI"); err != nil {
		t.Fatalf("UpsertLemma FI: %v", err)
	}
	if err := api.store.UpsertForm("kissoja", "kissa", "NOUN", "FI"); err != nil {
		t.Fatalf("UpsertForm FI: %v", err)
	}
	if err := api.store.UpsertLemma("kass", "NOUN", "cat", "ET"); err != nil {
		t.Fatalf("UpsertLemma ET: %v", err)
	}
	if err := api.store.UpsertForm("kasse", "kass", "NOUN", "ET"); err != nil {
		t.Fatalf("UpsertForm ET: %v", err)
	}

	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "bilingual@example.com")

	for _, body := range []string{
		`{"lang":"FI","words":["kissoja"]}`,
		`{"lang":"ET","words":["kasse"]}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/known-words", strings.NewReader(body))
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("import status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
		}
	}

	fiReq := httptest.NewRequest(http.MethodGet, "/api/known-words?lang=FI", nil)
	etReq := httptest.NewRequest(http.MethodGet, "/api/known-words?lang=ET", nil)
	for _, req := range []*http.Request{fiReq, etReq} {
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
	}

	fiRec := httptest.NewRecorder()
	mux.ServeHTTP(fiRec, fiReq)
	etRec := httptest.NewRecorder()
	mux.ServeHTTP(etRec, etReq)

	var fiResp KnownWordsListResponse
	if err := json.NewDecoder(bytes.NewReader(fiRec.Body.Bytes())).Decode(&fiResp); err != nil {
		t.Fatalf("decode FI list response: %v", err)
	}
	if len(fiResp.KnownWords) != 1 || fiResp.KnownWords[0].Lang != "FI" || fiResp.KnownWords[0].Lemma != "kissa" {
		t.Fatalf("unexpected FI known words: %+v", fiResp.KnownWords)
	}

	var etResp KnownWordsListResponse
	if err := json.NewDecoder(bytes.NewReader(etRec.Body.Bytes())).Decode(&etResp); err != nil {
		t.Fatalf("decode ET list response: %v", err)
	}
	if len(etResp.KnownWords) != 1 || etResp.KnownWords[0].Lang != "ET" || etResp.KnownWords[0].Lemma != "kass" {
		t.Fatalf("unexpected ET known words: %+v", etResp.KnownWords)
	}
}

func TestCreateDeckSkipsKnownWordsWhenSeedingCards(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("kissa", "NOUN", "cat", "FI"); err != nil {
		t.Fatalf("UpsertLemma: %v", err)
	}
	if err := api.store.UpsertForm("kissa", "kissa", "NOUN", "FI"); err != nil {
		t.Fatalf("UpsertForm: %v", err)
	}

	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{
				{
					Text: "Kissa juoksee.",
					Tokens: []parsecore.TokenResult{
						{Form: "Kissa", Lemma: "kissa", POS: "NOUN"},
						{Form: "juoksee", Lemma: "juosta", POS: "VERB"},
						{Form: ".", POS: "PUNCT"},
					},
				},
			},
		}, nil
	}

	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "deck@example.com")

	importReq := httptest.NewRequest(http.MethodPost, "/api/known-words", strings.NewReader(`{"lang":"FI","words":["kissa"]}`))
	for _, cookie := range cookies {
		importReq.AddCookie(cookie)
	}
	importRec := httptest.NewRecorder()
	mux.ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("known-word import status=%d want %d body=%q", importRec.Code, http.StatusOK, importRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Test deck","lang":"FI","text":"Kissa juoksee."}`))
	for _, cookie := range cookies {
		createReq.AddCookie(cookie)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("create deck status=%d want %d body=%q", createRec.Code, http.StatusOK, createRec.Body.String())
	}

	auth, err := api.getCurrentUser(requestWithCookies(httptest.NewRequest(http.MethodGet, "/api/me", nil), cookies))
	if err != nil {
		t.Fatalf("getCurrentUser: %v", err)
	}
	if auth == nil {
		t.Fatal("expected authenticated user")
	}

	cardCount, err := api.store.CountCards(auth.UserID, "FI")
	if err != nil {
		t.Fatalf("CountCards: %v", err)
	}
	if cardCount != 1 {
		t.Fatalf("card_count=%d want 1", cardCount)
	}
}

func TestDeckListRenameAndDeleteFlow(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{
				{
					Text: "Kissa juoksee.",
					Tokens: []parsecore.TokenResult{
						{Form: "Kissa", Lemma: "kissa", POS: "NOUN"},
						{Form: "juoksee", Lemma: "juosta", POS: "VERB"},
					},
				},
			},
		}, nil
	}
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "deck-flow@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Original","lang":"FI","text":"Kissa juoksee."}`))
	for _, cookie := range cookies {
		createReq.AddCookie(cookie)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status=%d want %d body=%q", createRec.Code, http.StatusOK, createRec.Body.String())
	}

	var created CreateDeckResponse
	if err := json.NewDecoder(bytes.NewReader(createRec.Body.Bytes())).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/decks", nil)
	for _, cookie := range cookies {
		listReq.AddCookie(cookie)
	}
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d want %d body=%q", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var listResp DeckListResponse
	if err := json.NewDecoder(bytes.NewReader(listRec.Body.Bytes())).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Decks) != 1 || listResp.Decks[0].Title != "Original" {
		t.Fatalf("unexpected decks payload: %+v", listResp.Decks)
	}

	renameReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/decks/%d", created.DeckID), strings.NewReader(`{"title":"Renamed"}`))
	for _, cookie := range cookies {
		renameReq.AddCookie(cookie)
	}
	renameRec := httptest.NewRecorder()
	mux.ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("rename status=%d want %d body=%q", renameRec.Code, http.StatusOK, renameRec.Body.String())
	}

	listAgainReq := httptest.NewRequest(http.MethodGet, "/api/decks", nil)
	for _, cookie := range cookies {
		listAgainReq.AddCookie(cookie)
	}
	listAgainRec := httptest.NewRecorder()
	mux.ServeHTTP(listAgainRec, listAgainReq)
	var listAgainResp DeckListResponse
	if err := json.NewDecoder(bytes.NewReader(listAgainRec.Body.Bytes())).Decode(&listAgainResp); err != nil {
		t.Fatalf("decode second list response: %v", err)
	}
	if len(listAgainResp.Decks) != 1 || listAgainResp.Decks[0].Title != "Renamed" {
		t.Fatalf("unexpected second decks payload: %+v", listAgainResp.Decks)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/decks/%d", created.DeckID), nil)
	for _, cookie := range cookies {
		deleteReq.AddCookie(cookie)
	}
	deleteRec := httptest.NewRecorder()
	mux.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status=%d want %d body=%q", deleteRec.Code, http.StatusOK, deleteRec.Body.String())
	}

	finalListReq := httptest.NewRequest(http.MethodGet, "/api/decks", nil)
	for _, cookie := range cookies {
		finalListReq.AddCookie(cookie)
	}
	finalListRec := httptest.NewRecorder()
	mux.ServeHTTP(finalListRec, finalListReq)
	var finalListResp DeckListResponse
	if err := json.NewDecoder(bytes.NewReader(finalListRec.Body.Bytes())).Decode(&finalListResp); err != nil {
		t.Fatalf("decode final list response: %v", err)
	}
	if len(finalListResp.Decks) != 0 {
		t.Fatalf("decks=%d want 0 (%+v)", len(finalListResp.Decks), finalListResp.Decks)
	}
}

func TestReviewFlowAnswerAndMarkKnown(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("kissa", "NOUN", "cat", "FI"); err != nil {
		t.Fatalf("UpsertLemma: %v", err)
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{
				{
					Text: "Kissa juoksee.",
					Tokens: []parsecore.TokenResult{
						{Form: "Kissa", Lemma: "kissa", POS: "NOUN"},
					},
				},
			},
		}, nil
	}
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "review@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Review deck","lang":"FI","text":"Kissa juoksee."}`))
	for _, cookie := range cookies {
		createReq.AddCookie(cookie)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status=%d want %d body=%q", createRec.Code, http.StatusOK, createRec.Body.String())
	}

	reviewReq := httptest.NewRequest(http.MethodGet, "/api/review/next", nil)
	for _, cookie := range cookies {
		reviewReq.AddCookie(cookie)
	}
	reviewRec := httptest.NewRecorder()
	mux.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("review status=%d want %d body=%q", reviewRec.Code, http.StatusOK, reviewRec.Body.String())
	}

	var card CardResponse
	if err := json.NewDecoder(bytes.NewReader(reviewRec.Body.Bytes())).Decode(&card); err != nil {
		t.Fatalf("decode review card: %v", err)
	}
	cardID, err := strconv.ParseInt(card.CardID, 10, 64)
	if err != nil || cardID <= 0 {
		t.Fatalf("card_id=%q want positive integer", card.CardID)
	}

	answerReq := httptest.NewRequest(http.MethodPost, "/api/review/answer", strings.NewReader(fmt.Sprintf(`{"card_id":%d,"rating":"good"}`, cardID)))
	for _, cookie := range cookies {
		answerReq.AddCookie(cookie)
	}
	answerRec := httptest.NewRecorder()
	mux.ServeHTTP(answerRec, answerReq)
	if answerRec.Code != http.StatusOK {
		t.Fatalf("answer status=%d want %d body=%q", answerRec.Code, http.StatusOK, answerRec.Body.String())
	}

	nextReq := httptest.NewRequest(http.MethodGet, "/api/review/next", nil)
	for _, cookie := range cookies {
		nextReq.AddCookie(cookie)
	}
	nextRec := httptest.NewRecorder()
	mux.ServeHTTP(nextRec, nextReq)
	if nextRec.Code != http.StatusNoContent {
		t.Fatalf("next review status=%d want %d body=%q", nextRec.Code, http.StatusNoContent, nextRec.Body.String())
	}

	knownReq := httptest.NewRequest(http.MethodPost, "/api/card/known", strings.NewReader(fmt.Sprintf(`{"card_id":%d}`, cardID)))
	for _, cookie := range cookies {
		knownReq.AddCookie(cookie)
	}
	knownRec := httptest.NewRecorder()
	mux.ServeHTTP(knownRec, knownReq)
	if knownRec.Code != http.StatusOK {
		t.Fatalf("mark known status=%d want %d body=%q", knownRec.Code, http.StatusOK, knownRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/known-words?lang=FI", nil)
	for _, cookie := range cookies {
		listReq.AddCookie(cookie)
	}
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	var listResp KnownWordsListResponse
	if err := json.NewDecoder(bytes.NewReader(listRec.Body.Bytes())).Decode(&listResp); err != nil {
		t.Fatalf("decode known words response: %v", err)
	}
	if len(listResp.KnownWords) != 1 || listResp.KnownWords[0].Lemma != "kissa" {
		t.Fatalf("unexpected known words payload: %+v", listResp.KnownWords)
	}
}

func TestReviewNextDeckFilterUsesOwnedDeckData(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		lemma := "kissa"
		form := "Kissa"
		if strings.HasPrefix(strings.ToLower(text), "koira") {
			lemma = "koira"
			form = "Koira"
		}
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{
				{
					Text: text,
					Tokens: []parsecore.TokenResult{
						{Form: form, Lemma: lemma, POS: "NOUN"},
					},
				},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	ownerEmail := "owner@example.com"
	otherEmail := "other@example.com"
	ownerCookies := loginAndReturnCookies(t, mux, ownerEmail)
	otherCookies := loginAndReturnCookies(t, mux, otherEmail)

	createDeckAndReturnID(t, mux, ownerCookies, "Owner seed deck", "Koira bumps the deck id.")
	createDeckAndReturnID(t, mux, otherCookies, "Other deck", "Kissa from the other deck.")
	ownerDeckID := createDeckAndReturnID(t, mux, ownerCookies, "Owner deck", "Kissa from the owner deck.")

	ownerUser, err := api.store.GetOrCreateUser(ownerEmail)
	if err != nil {
		t.Fatalf("GetOrCreateUser(owner): %v", err)
	}
	if ownerDeckID == ownerUser.ID {
		t.Fatalf("test setup invalid: owner deck id %d matches owner user id %d", ownerDeckID, ownerUser.ID)
	}

	reviewReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/review/next?deck_id=%d", ownerDeckID), nil)
	for _, cookie := range ownerCookies {
		reviewReq.AddCookie(cookie)
	}
	reviewRec := httptest.NewRecorder()
	mux.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("review status=%d want %d body=%q", reviewRec.Code, http.StatusOK, reviewRec.Body.String())
	}

	var card CardResponse
	if err := json.NewDecoder(bytes.NewReader(reviewRec.Body.Bytes())).Decode(&card); err != nil {
		t.Fatalf("decode review card: %v", err)
	}
	if card.Front.Text != "Kissa from the owner deck." {
		t.Fatalf("front text=%q want owner sentence", card.Front.Text)
	}
	if len(card.Back.Examples) != 1 {
		t.Fatalf("examples=%d want 1 (%+v)", len(card.Back.Examples), card.Back.Examples)
	}
	if card.Back.Examples[0].Text != "Kissa from the owner deck." {
		t.Fatalf("example text=%q want owner sentence", card.Back.Examples[0].Text)
	}
	if card.Back.Examples[0].SourceDeck != "Owner deck" {
		t.Fatalf("source deck=%q want Owner deck", card.Back.Examples[0].SourceDeck)
	}
}

func TestReviewNextDeckFilterRejectsForeignDeck(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{
				{
					Text: text,
					Tokens: []parsecore.TokenResult{
						{Form: "Kissa", Lemma: "kissa", POS: "NOUN"},
					},
				},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	ownerCookies := loginAndReturnCookies(t, mux, "owner@example.com")
	otherCookies := loginAndReturnCookies(t, mux, "other@example.com")
	otherDeckID := createDeckAndReturnID(t, mux, otherCookies, "Other deck", "Kissa from the other deck.")

	reviewReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/review/next?deck_id=%d", otherDeckID), nil)
	for _, cookie := range ownerCookies {
		reviewReq.AddCookie(cookie)
	}
	reviewRec := httptest.NewRecorder()
	mux.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusNotFound {
		t.Fatalf("review status=%d want %d body=%q", reviewRec.Code, http.StatusNotFound, reviewRec.Body.String())
	}
	if !strings.Contains(reviewRec.Body.String(), "Deck not found") {
		t.Fatalf("body=%q missing deck-not-found error", reviewRec.Body.String())
	}
}

func TestParseFeedbackRequiresAuthentication(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/parse/feedback", strings.NewReader(`{"parse_id":1,"lang":"FI","parser":"custom","surface":"kissa","proposed_lemma":"kissa","proposed_pos":"NOUN"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestParseFeedbackRejectsWhitespaceOnlyRequiredFields(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "user@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/parse/feedback", strings.NewReader(`{"parse_id":1,"lang":"FI","parser":"custom","surface":"   ","proposed_lemma":" ","proposed_pos":"NOUN"}`))
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestParseFeedbackRejectsUnknownParseSession(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	userCookies := loginAndReturnCookies(t, mux, "user@example.com")

	feedbackReq := httptest.NewRequest(http.MethodPost, "/api/parse/feedback", strings.NewReader(`{
		"parse_id": 9999,
		"lang": "FI",
		"parser": "custom",
		"surface": "kissa",
		"proposed_lemma": "kissa",
		"proposed_pos": "NOUN"
	}`))
	for _, cookie := range userCookies {
		feedbackReq.AddCookie(cookie)
	}
	feedbackRec := httptest.NewRecorder()
	mux.ServeHTTP(feedbackRec, feedbackReq)

	if feedbackRec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d body=%q", feedbackRec.Code, http.StatusBadRequest, feedbackRec.Body.String())
	}
}

func TestParseFeedbackSubmissionAndAdminReview(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")

	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		if parser == "" {
			parser = "custom"
		}
		return &parsecore.ParseResult{
			Lang:            lang,
			Parser:          parser,
			TotalTokens:     2,
			ParseDurationMs: 11,
			Stats:           parsecore.ParseStats{},
			Words: []parsecore.WordEntry{
				{Lemma: "kissa", POS: "NOUN", Forms: []string{"kissa"}, Count: 1},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	userCookies := loginAndReturnCookies(t, mux, "user@example.com")
	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")

	parseReq := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"FI","text":"kissa","parser":"custom"}`))
	for _, cookie := range userCookies {
		parseReq.AddCookie(cookie)
	}
	parseRec := httptest.NewRecorder()
	mux.ServeHTTP(parseRec, parseReq)
	if parseRec.Code != http.StatusOK {
		t.Fatalf("parse status=%d want %d body=%q", parseRec.Code, http.StatusOK, parseRec.Body.String())
	}

	var parseResp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(parseRec.Body.Bytes())).Decode(&parseResp); err != nil {
		t.Fatalf("decode parse response: %v", err)
	}
	if parseResp.ParseID == nil || *parseResp.ParseID == 0 {
		t.Fatalf("parse_id=%v want non-nil", parseResp.ParseID)
	}

	feedbackReq := httptest.NewRequest(http.MethodPost, "/api/parse/feedback", strings.NewReader(`{
		"parse_id": `+fmt.Sprintf("%d", *parseResp.ParseID)+`,
		"lang": "FI",
		"parser": "custom",
		"surface": "kissa",
		"occurrence": 1,
		"original_lemma": "kissa",
		"original_pos": "NOUN",
		"proposed_lemma": "kissa",
		"proposed_pos": "PROPN",
		"note": "should be proper noun in this context"
	}`))
	for _, cookie := range userCookies {
		feedbackReq.AddCookie(cookie)
	}
	feedbackRec := httptest.NewRecorder()
	mux.ServeHTTP(feedbackRec, feedbackReq)
	if feedbackRec.Code != http.StatusOK {
		t.Fatalf("feedback status=%d want %d body=%q", feedbackRec.Code, http.StatusOK, feedbackRec.Body.String())
	}

	var feedbackResp ParseFeedbackResponse
	if err := json.NewDecoder(bytes.NewReader(feedbackRec.Body.Bytes())).Decode(&feedbackResp); err != nil {
		t.Fatalf("decode feedback response: %v", err)
	}
	if feedbackResp.FeedbackID == 0 {
		t.Fatal("expected feedback_id from feedback response")
	}
	if feedbackResp.Status != "submitted" {
		t.Fatalf("status=%q want submitted", feedbackResp.Status)
	}

	otherUserCookies := loginAndReturnCookies(t, mux, "other@example.com")
	forbiddenReq := httptest.NewRequest(http.MethodPost, "/api/parse/feedback", strings.NewReader(`{
		"parse_id": `+fmt.Sprintf("%d", *parseResp.ParseID)+`,
		"lang": "FI",
		"parser": "custom",
		"surface": "kissa",
		"proposed_lemma": "kissa",
		"proposed_pos": "NOUN"
	}`))
	for _, cookie := range otherUserCookies {
		forbiddenReq.AddCookie(cookie)
	}
	forbiddenRec := httptest.NewRecorder()
	mux.ServeHTTP(forbiddenRec, forbiddenReq)
	if forbiddenRec.Code != http.StatusForbidden {
		t.Fatalf("cross-user feedback status=%d want %d body=%q", forbiddenRec.Code, http.StatusForbidden, forbiddenRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/parse-feedback?status=submitted", nil)
	for _, cookie := range adminCookies {
		listReq.AddCookie(cookie)
	}
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("admin list status=%d want %d body=%q", listRec.Code, http.StatusOK, listRec.Body.String())
	}

	var listResp ParseFeedbackListResponse
	if err := json.NewDecoder(bytes.NewReader(listRec.Body.Bytes())).Decode(&listResp); err != nil {
		t.Fatalf("decode feedback list response: %v", err)
	}
	if len(listResp.Feedback) != 1 {
		t.Fatalf("feedback=%d want 1 (%+v)", len(listResp.Feedback), listResp.Feedback)
	}
	if listResp.Feedback[0].Status != "submitted" {
		t.Fatalf("queued feedback status=%q want submitted", listResp.Feedback[0].Status)
	}

	reviewReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/parse-feedback?id=%d", feedbackResp.FeedbackID), strings.NewReader(`{"status":"accepted","review_note":"looks correct"}`))
	for _, cookie := range adminCookies {
		reviewReq.AddCookie(cookie)
	}
	reviewRec := httptest.NewRecorder()
	mux.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("admin review status=%d want %d body=%q", reviewRec.Code, http.StatusOK, reviewRec.Body.String())
	}

	listAcceptedReq := httptest.NewRequest(http.MethodGet, "/api/admin/parse-feedback?status=accepted", nil)
	for _, cookie := range adminCookies {
		listAcceptedReq.AddCookie(cookie)
	}
	listAcceptedRec := httptest.NewRecorder()
	mux.ServeHTTP(listAcceptedRec, listAcceptedReq)

	var acceptedResp ParseFeedbackListResponse
	if err := json.NewDecoder(bytes.NewReader(listAcceptedRec.Body.Bytes())).Decode(&acceptedResp); err != nil {
		t.Fatalf("decode accepted feedback response: %v", err)
	}
	if len(acceptedResp.Feedback) != 1 {
		t.Fatalf("accepted feedback=%d want 1 (%+v)", len(acceptedResp.Feedback), acceptedResp.Feedback)
	}
	if acceptedResp.Feedback[0].Status != "accepted" {
		t.Fatalf("accepted status=%q want accepted", acceptedResp.Feedback[0].Status)
	}
	if acceptedResp.Feedback[0].ReviewedByUserID == nil {
		t.Fatal("expected reviewer to be recorded")
	}
}

func TestAdminParseFeedbackRejectsUnknownFeedbackReviewTarget(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")

	api := newTestAPI(t)
	mux := newTestMux(t, api)
	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/parse-feedback?id=9999", strings.NewReader(`{"status":"accepted"}`))
	for _, cookie := range adminCookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAdminParseFeedbackRejectsInvalidStatus(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")

	api := newTestAPI(t)
	mux := newTestMux(t, api)
	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/parse-feedback?id=1", strings.NewReader(`{"status":"approved"}`))
	for _, cookie := range adminCookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAdminParseFeedbackRejectsInvalidStatusQuery(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")

	api := newTestAPI(t)
	mux := newTestMux(t, api)
	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/parse-feedback?status=pending_review", nil)
	for _, cookie := range adminCookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAdminParseFeedbackRejectsNonAdminUser(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	userCookies := loginAndReturnCookies(t, mux, "user@example.com")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/parse-feedback", nil)
	for _, cookie := range userCookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusForbidden, rec.Body.String())
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

func createDeckAndReturnID(t *testing.T, mux *http.ServeMux, cookies []*http.Cookie, title, text string) int64 {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/decks",
		strings.NewReader(fmt.Sprintf(`{"title":%q,"lang":"FI","text":%q}`, title, text)),
	)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create deck status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var created CreateDeckResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&created); err != nil {
		t.Fatalf("decode create deck response: %v", err)
	}
	return created.DeckID
}

func requestWithCookies(req *http.Request, cookies []*http.Cookie) *http.Request {
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	return req
}
