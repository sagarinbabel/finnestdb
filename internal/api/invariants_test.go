package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"finnestdb/internal/store"
)

// Regression tests for the handler-layer halves of the 2026-11-07 invariant
// audit. The store-layer tests live in internal/store/invariants_test.go.

// I-13a. A stranger cannot lock a named account out of logging in.
//
// Why it matters: the per-account login limiter (10/min, keyed on the submitted
// address) used to be charged by EVERY attempt, not just failures. Anyone who
// knew an address could spend that budget from any IP and the real owner was
// refused even with the correct password - a limiter meant to slow credential
// stuffing doubled as a denial-of-service lever against a named user.
//
// The budget is now charged only by failures and cleared by a success, so ten
// wrong guesses still close the window (brute-force protection intact) while a
// correct password always gets through.
func TestFailedLoginsDoNotLockOutTheRealOwner(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	loginAndReturnCookies(t, mux, "victim@example.com") // registers

	// The per-account limit is 10/min by default. Spend most of it on wrong
	// guesses, staying under the threshold, then prove the owner still gets in.
	wrong := `{"email":"victim@example.com","password":"wrong-password-123"}`
	for i := 0; i < 8; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(wrong))
		req.RemoteAddr = fmt.Sprintf("203.0.113.%d:1234", i+1) // distinct IPs, so the per-IP limit is not what we are testing
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status=%d want 401 body=%q", i, rec.Code, rec.Body.String())
		}
	}

	right := fmt.Sprintf(`{"email":"victim@example.com","password":%q}`, testPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(right))
	req.RemoteAddr = "203.0.113.200:1234"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("the account owner was refused after other people's failed guesses: status=%d body=%q",
			rec.Code, rec.Body.String())
	}

	// And the success cleared the failure budget, so a fresh burst is possible
	// rather than the owner being stuck near the limit.
	for i := 0; i < 8; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(wrong))
		req.RemoteAddr = fmt.Sprintf("198.51.100.%d:1234", i+1)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d was rate limited; a successful login should have reset the account budget", i)
		}
	}
}

// I-13b. Brute force against one account is still throttled.
//
// The counterpart to the test above: charging only failures must not mean
// charging nothing. Enough wrong guesses in one window must start returning
// 429 regardless of which IP they come from.
func TestRepeatedFailedLoginsAreThrottledPerAccount(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	loginAndReturnCookies(t, mux, "target@example.com")

	wrong := `{"email":"target@example.com","password":"wrong-password-123"}`
	sawLimit := false
	for i := 0; i < 40; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(wrong))
		req.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", (i%200)+1)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			sawLimit = true
			break
		}
	}
	if !sawLimit {
		t.Error("40 failed guesses against one account were never rate limited")
	}
}

// I-8. The parse-feedback endpoint rejects parser names the parser registry
// does not know.
//
// Why it matters: this endpoint never calls parsecore.Analyze, so it was the
// one path where an arbitrary parser string reached the database. The value is
// a component of the correction_issues uniqueness fingerprint
// (UNIQUE(lang, parser, norm_surface, lemma, pos)), so a junk or differently
// spelled name opened a SECOND issue row for an analysis already tracked under
// the real parser - defeating the deduplication that ledger exists to provide.
func TestParseFeedbackRejectsUnknownParser(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "feedback@example.com")

	body := `{"parse_session_id":1,"lang":"FI","parser":"Custom","surface":"kissat",` +
		`"proposed_lemma":"kissa","proposed_pos":"NOUN"}`
	req := httptest.NewRequest(http.MethodPost, "/api/parse/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 for parser=%q; body=%q", rec.Code, "Custom", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported parser") {
		t.Errorf("body=%q should name the unsupported parser", rec.Body.String())
	}
}

// I-4. The admin correction-issue endpoint answers 409, not 500, when an
// action does not apply to the issue's current state.
//
// Why it matters: "restore" used to succeed from any status, marking an issue
// fixed that had never been quarantined. Now that the store refuses it, the
// admin UI needs a status it can present as "reload and look again" rather
// than an opaque database error.
func TestAdminIssueIllegalTransitionReturnsConflict(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")
	reporter := createTestUserForAPI(t, api, "reporter@example.com")

	issueID := seedIssueViaFeedback(t, api, reporter.ID, "FI", "koira", "koira", "NOUN")

	// Triage, then fix without ever quarantining. open -> fixed is a real case
	// (an issue classified as a source-extraction problem and fixed upstream
	// never needed suppression), so it must succeed.
	patchIssue(t, mux, adminCookies, issueID,
		`{"action":"triage","alpha_class":"source_extraction_issue"}`, http.StatusOK)
	patchIssue(t, mux, adminCookies, issueID,
		`{"action":"restore","fix_note":"fixed upstream"}`, http.StatusOK)

	// fixed -> quarantined is NOT legal: a regression has to arrive as a new
	// learner report, which moves the issue to "reopened" and bumps
	// reopened_count so the queue can tell it from a fresh case. Allowing the
	// direct edge is what let a row carry both fixed_at and quarantined_at.
	patchIssue(t, mux, adminCookies, issueID,
		`{"action":"quarantine","reason":"actually still broken"}`, http.StatusConflict)
}

// I-5d. An accidentally accepted correction can be taken back through the
// admin API, and the old status-only path fails loudly instead of with a 500.
//
// Why it matters: making acceptance one-way in the store broke the existing
// Reject / Follow up buttons for accepted rows — handleAdminParseFeedback
// turned the refused transition into an opaque HTTP 500, and nothing in
// production called the replacement helper, so an accidentally accepted
// correction could not be undone through the app at all. Found by review on
// #295.
func TestAdminCanRetractAnAcceptedCorrection(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")
	reporter := createTestUserForAPI(t, api, "reporter@example.com")

	if err := api.store.InsertFormForTest("kissat", "kissa", "NOUN", "FI"); err != nil {
		t.Fatal(err)
	}
	psID, err := api.store.CreateParseSession(&reporter.ID, "FI", "custom", "kissat", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	fbID, err := api.store.CreateParseFeedback(store.ParseFeedback{
		ParseSessionID: psID, UserID: reporter.ID, Lang: "FI", Parser: "custom",
		Surface: "kissat", ProposedLemma: "kissa", ProposedPOS: "NOUN",
	})
	if err != nil {
		t.Fatal(err)
	}

	patchFeedback := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch,
			fmt.Sprintf("/api/admin/parse-feedback?id=%d", fbID), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = requestWithCookies(req, adminCookies)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := patchFeedback(`{"status":"accepted","review_note":"looks right"}`); rec.Code != http.StatusOK {
		t.Fatalf("accept: status=%d body=%q", rec.Code, rec.Body.String())
	}

	// The status-only reject is refused with 409 and a message that says what
	// to do instead - not a 500.
	rec := patchFeedback(`{"status":"rejected","review_note":"changed my mind"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status-only reject of an accepted row: status=%d want 409; body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "retract") {
		t.Errorf("the 409 should point the admin at retraction; body=%q", rec.Body.String())
	}

	// The retraction works and reports whether a gold candidate is implicated.
	rec = patchFeedback(`{"status":"rejected","review_note":"wrong after all","retract":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("retract: status=%d body=%q", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode retract response: %v", err)
	}
	if _, ok := resp["gold_candidate_needs_review"]; !ok {
		t.Error("retract response should report gold_candidate_needs_review")
	}

	// The override is gone from the dictionary, which is the whole point.
	var overrides int
	if err := api.store.CountFormsBySourceForTest("custom_overrides", &overrides); err != nil {
		t.Fatal(err)
	}
	if overrides != 0 {
		t.Errorf("%d custom_overrides form row(s) survived the retraction", overrides)
	}

	// Retracting again is a 409, not a silent success.
	if rec := patchFeedback(`{"status":"rejected","retract":true}`); rec.Code != http.StatusConflict {
		t.Errorf("second retract: status=%d want 409", rec.Code)
	}
}
