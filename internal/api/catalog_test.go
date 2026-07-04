package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"finnestdb/internal/catalog"
)

// reviewedCatalogIDs returns the set of catalog entry ids that
// internal/catalog/reviews.json signs off on. The review pin is derived from
// the same source the generator uses, so the API-level assertion stays honest
// as texts are added or replaced without a test edit.
func reviewedCatalogIDs(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("../catalog/reviews.json")
	if err != nil {
		t.Fatalf("read reviews.json: %v", err)
	}
	var f struct {
		Reviews map[string]json.RawMessage `json:"reviews"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse reviews.json: %v", err)
	}
	ids := make(map[string]bool, len(f.Reviews))
	for id := range f.Reviews {
		ids[id] = true
	}
	return ids
}

// The catalog endpoints are signed-in only, must serve the embedded metadata,
// compute personalized coverage from the learner's known lemmas, and 404 on
// unknown ids. These tests encode those product guarantees; they use the real
// embedded catalog so a shipped-artifact regression is caught.

func TestCatalogRequiresAuth(t *testing.T) {
	mux := newTestMux(t, newTestAPI(t))

	for _, path := range []string{"/api/catalog", "/api/catalog/fi-kesaaamu-poem/text"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without auth: status=%d want %d", path, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestCatalogListReturnsEmbeddedEntries(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "catalog-list@example.com")

	req := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp CatalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Entries) != len(catalog.Entries()) {
		t.Fatalf("returned %d entries, embedded catalog has %d", len(resp.Entries), len(catalog.Entries()))
	}
	reviewed := reviewedCatalogIDs(t)
	for _, e := range resp.Entries {
		if e.ID == "" || e.Title == "" || e.License == "" {
			t.Errorf("entry missing id/title/license: %+v", e)
		}
		// An entry is approved iff reviews.json signed it off; everything else
		// is pending. Derived from the same source the generator uses, so it
		// stays honest as texts are added or replaced.
		if reviewed[e.ID] {
			if e.DifficultyReview != "approved" {
				t.Errorf("%s: difficulty_review=%q want approved (reviews.json signs it off)", e.ID, e.DifficultyReview)
			}
		} else {
			if e.DifficultyReview != "pending" {
				t.Errorf("%s: difficulty_review=%q want pending (no reviews.json sign-off)", e.ID, e.DifficultyReview)
			}
		}
		// No known words yet -> no coverage overlay, and no known-word flag.
		if e.Coverage != nil {
			t.Errorf("%s: coverage should be nil with no known words", e.ID)
		}
	}
	if resp.HasKnownWords["FI"] || resp.HasKnownWords["ET"] {
		t.Errorf("has_known_words should be false for a fresh account: %+v", resp.HasKnownWords)
	}
}

func TestCatalogCoverageMath(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "catalog-cov@example.com")

	user, err := api.store.GetUserByEmail("catalog-cov@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}

	// Pick a real FI entry from the embedded catalog and mark exactly half of
	// its distinct lemmas known, so coverage is a deterministic 50%.
	var target catalog.Entry
	for _, e := range catalog.Entries() {
		if e.Language == "fi" && len(e.Lemmas) >= 2 {
			target = e
			break
		}
	}
	if target.ID == "" {
		t.Fatal("no FI entry with >=2 lemmas in embedded catalog")
	}
	half := len(target.Lemmas) / 2
	for _, l := range target.Lemmas[:half] {
		if err := api.store.MarkLemmaKnown(user.ID, "FI", l.Lemma, l.POS); err != nil {
			t.Fatalf("MarkLemmaKnown: %v", err)
		}
	}
	wantPct := int(float64(half)/float64(len(target.Lemmas))*100 + 0.5)

	req := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	var resp CatalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.HasKnownWords["FI"] {
		t.Error("has_known_words[FI] should be true after marking lemmas known")
	}

	var got *CatalogEntryResponse
	for i := range resp.Entries {
		if resp.Entries[i].ID == target.ID {
			got = &resp.Entries[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("target entry %q not in response", target.ID)
	}
	if got.Coverage == nil {
		t.Fatalf("expected coverage overlay for %q", target.ID)
	}
	if got.Coverage.TotalLemmas != len(target.Lemmas) || got.Coverage.KnownLemmas != half {
		t.Fatalf("coverage counts: known=%d total=%d, want %d/%d",
			got.Coverage.KnownLemmas, got.Coverage.TotalLemmas, half, len(target.Lemmas))
	}
	if got.Coverage.KnownPct != wantPct {
		t.Fatalf("coverage pct = %d, want %d", got.Coverage.KnownPct, wantPct)
	}

	// An ET entry should still have no coverage overlay: known words were only
	// added for FI, so language scoping must hold.
	for _, e := range resp.Entries {
		if e.Language == "et" && e.Coverage != nil {
			t.Errorf("ET entry %q got a coverage overlay from FI-only known words", e.ID)
		}
	}
}

func TestCatalogTextLazyLoad(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "catalog-text@example.com")

	entries := catalog.Entries()
	if len(entries) == 0 {
		t.Fatal("no embedded entries")
	}
	id := entries[0].ID

	req := httptest.NewRequest(http.MethodGet, "/api/catalog/"+id+"/text", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	var resp CatalogTextResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != id || resp.Text == "" {
		t.Fatalf("unexpected text payload: id=%q len=%d", resp.ID, len(resp.Text))
	}
}

func TestCatalogTextUnknownID404(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "catalog-404@example.com")

	for _, path := range []string{
		"/api/catalog/nope/text",
		"/api/catalog/some/nested/text",
		"/api/catalog/fi-kesaaamu-poem", // missing /text suffix
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status=%d want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

// The landing demo-text endpoint is intentionally anonymous but restricted to a
// fixed allowlist of embedded ids (the three "or try →" chips). These tests
// encode both product guarantees: allowlisted texts serve without auth, and
// everything else — including real-but-not-allowlisted catalog ids — 404s, so
// the endpoint can't be used to enumerate the otherwise-private catalog.

func TestDemoTextServesAllowlistedTextsAnonymously(t *testing.T) {
	mux := newTestMux(t, newTestAPI(t))

	for id := range demoTextAllowlist {
		req := httptest.NewRequest(http.MethodGet, "/api/demo/text/"+id, nil)
		// No cookies — anonymous on purpose.
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("demo %s: status=%d body=%q", id, rec.Code, rec.Body.String())
		}
		var resp CatalogTextResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("demo %s decode: %v", id, err)
		}
		if resp.ID != id || resp.Text == "" || resp.Language == "" {
			t.Fatalf("demo %s: unexpected payload id=%q lang=%q len=%d", id, resp.ID, resp.Language, len(resp.Text))
		}
	}
}

func TestDemoTextRejectsNonAllowlistedAndUnknownIDs(t *testing.T) {
	mux := newTestMux(t, newTestAPI(t))

	// A real embedded id that is NOT on the demo allowlist must 404 anonymously
	// (it stays reachable only through the signed-in /api/catalog surface), so
	// this endpoint can't leak the private catalog.
	nonAllowlisted := "fi-kesaaamu-poem"
	if demoTextAllowlist[nonAllowlisted] {
		t.Fatalf("test assumes %q is not on the demo allowlist", nonAllowlisted)
	}
	if _, ok := catalog.Find(nonAllowlisted); !ok {
		t.Fatalf("test assumes %q is a real embedded catalog id", nonAllowlisted)
	}

	for _, path := range []string{
		"/api/demo/text/" + nonAllowlisted,
		"/api/demo/text/nope",
		"/api/demo/text/",
		"/api/demo/text/fi-sauna-article/extra",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status=%d want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

// The three demo ids must actually exist in the shipped catalog, or the landing
// chips would 404 in production. Pin them here so a catalog regeneration that
// drops or renames one of them fails a Go test rather than only breaking the UI.
func TestDemoAllowlistIDsExistInCatalog(t *testing.T) {
	for id := range demoTextAllowlist {
		if _, ok := catalog.Find(id); !ok {
			t.Errorf("demo allowlist id %q is not in the embedded catalog", id)
		}
	}
}
