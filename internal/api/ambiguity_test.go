package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installFITable writes a minimal FI FST table so the store's lemmatizer merges
// FST-only homograph senses (kuusi/NOUN) into the ambiguity candidate set. It
// mirrors the store package's installTestLemmatizerTable but lives here so the
// API tests are self-contained.
func installFITable(t *testing.T, table map[string]any) {
	t.Helper()
	dir := t.TempDir()
	b, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		t.Fatalf("marshal FI table: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fi_min.json"), append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write FI table: %v", err)
	}
	t.Setenv("LEMMATIZER_TABLES_DIR", dir)
}

func seedForm(t *testing.T, api *API, form, lemma, pos, lang string) {
	t.Helper()
	if err := api.store.InsertFormForTest(form, lemma, pos, lang); err != nil {
		t.Fatalf("seed form %s: %v", form, err)
	}
}

func seedLemma(t *testing.T, api *API, lemma, pos, gloss, lang string) {
	t.Helper()
	if err := api.store.InsertLemmaForTest(lemma, pos, gloss, lang); err != nil {
		t.Fatalf("seed lemma %s: %v", lemma, err)
	}
}

func parseRequest(t *testing.T, mux *http.ServeMux, body string, cookies []*http.Cookie) *ParseResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("parse status=%d body=%q", rec.Code, rec.Body.String())
	}
	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode parse: %v", err)
	}
	return &resp
}

// TestParseAmbiguityMetadataForSignedIn verifies /api/parse attaches the
// Multiple-possible-meanings metadata for a signed-in learner: the ambiguous
// surface carries both senses (dict + FST-merged), a gloss where available, a
// source marker, and the first-occurrence example sentence.
func TestParseAmbiguityMetadataForSignedIn(t *testing.T) {
	installFITable(t, map[string]any{
		"kuusi": []map[string]string{
			{"Lemma": "kuusi", "UPOS": "NOUN"},
			{"Lemma": "kuusi", "UPOS": "NUM"},
		},
	})
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	seedForm(t, api, "kuusi", "kuusi", "NUM", "FI")
	seedLemma(t, api, "kuusi", "NUM", "six", "FI")
	seedLemma(t, api, "kuusi", "NOUN", "spruce", "FI")

	cookies := loginAndReturnCookies(t, mux, "amb@example.com")
	resp := parseRequest(t, mux, `{"lang":"FI","text":"Pihalla kasvaa kuusi."}`, cookies)

	if len(resp.AmbiguousSurfaces) != 1 {
		t.Fatalf("ambiguous_surfaces=%d want 1: %+v", len(resp.AmbiguousSurfaces), resp.AmbiguousSurfaces)
	}
	amb := resp.AmbiguousSurfaces[0]
	if amb.Surface != "kuusi" {
		t.Fatalf("surface=%q want kuusi", amb.Surface)
	}
	if len(amb.Candidates) != 2 {
		t.Fatalf("candidates=%d want 2: %+v", len(amb.Candidates), amb.Candidates)
	}
	if !strings.Contains(amb.Example, "kuusi") {
		t.Errorf("example missing context: %q", amb.Example)
	}
	sawFST, sawDict := false, false
	for _, c := range amb.Candidates {
		switch c.Source {
		case "fst":
			sawFST = true
		case "dict":
			sawDict = true
		}
	}
	if !sawFST || !sawDict {
		t.Errorf("expected both a dict and an FST candidate: %+v", amb.Candidates)
	}
}

// TestParseAmbiguityOmittedForAnonymous proves anonymous parses are unchanged:
// no ambiguity metadata leaks into the stateless demo response.
func TestParseAmbiguityOmittedForAnonymous(t *testing.T) {
	installFITable(t, map[string]any{
		"kuusi": []map[string]string{
			{"Lemma": "kuusi", "UPOS": "NOUN"},
			{"Lemma": "kuusi", "UPOS": "NUM"},
		},
	})
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	seedForm(t, api, "kuusi", "kuusi", "NUM", "FI")
	seedLemma(t, api, "kuusi", "NUM", "six", "FI")
	seedLemma(t, api, "kuusi", "NOUN", "spruce", "FI")

	// No cookies → anonymous.
	resp := parseRequest(t, mux, `{"lang":"FI","text":"Pihalla kasvaa kuusi."}`, nil)
	if len(resp.AmbiguousSurfaces) != 0 {
		t.Fatalf("anonymous parse leaked ambiguity metadata: %+v", resp.AmbiguousSurfaces)
	}
	// The word list itself must still be present — anonymous parsing works.
	if len(resp.Words) == 0 {
		t.Fatalf("anonymous parse returned no words")
	}
}

// TestDeckSaveCreatesCardForExplicitFSTSense is the core bypass test: an FST-only
// homograph sense (kuusi/NOUN) is normally filtered out of dict-only deck
// expansion, but when the learner explicitly selects it to study, saving the
// deck MUST create that card.
func TestDeckSaveCreatesCardForExplicitFSTSense(t *testing.T) {
	installFITable(t, map[string]any{
		"kuusi": []map[string]string{
			{"Lemma": "kuusi", "UPOS": "NOUN"},
			{"Lemma": "kuusi", "UPOS": "NUM"},
		},
	})
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	seedForm(t, api, "kuusi", "kuusi", "NUM", "FI")
	seedLemma(t, api, "kuusi", "NUM", "six", "FI")
	seedLemma(t, api, "kuusi", "NOUN", "spruce", "FI")

	cookies := loginAndReturnCookies(t, mux, "deck@example.com")
	user, err := api.store.GetOrCreateUser("deck@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	// Baseline: saving WITHOUT selecting the FST sense must NOT create the NOUN
	// card (dict-only expansion drops it).
	saveDeck(t, mux, cookies, `{"title":"Plain","lang":"FI","text":"Pihalla kasvaa kuusi."}`)
	if hasCard(t, api, user.ID, "FI", "kuusi", "NOUN") {
		t.Fatalf("dict-only deck save must not create the FST-only kuusi/NOUN card")
	}

	// With the explicit selection, the NOUN card MUST be created.
	saveDeck(t, mux, cookies, `{"title":"Chosen","lang":"FI","text":"Pihalla kasvaa kuusi.","selected_senses":[{"surface":"kuusi","lemma":"kuusi","pos":"NOUN"}]}`)
	if !hasCard(t, api, user.ID, "FI", "kuusi", "NOUN") {
		t.Fatalf("explicit Study-this-meaning selection must create the kuusi/NOUN card")
	}
}

// TestDeckSaveRejectsUnsupportedSelectedSense proves the injection is validated:
// a crafted selection that is not in the real candidate set is ignored.
func TestDeckSaveRejectsUnsupportedSelectedSense(t *testing.T) {
	installFITable(t, map[string]any{
		"kuusi": []map[string]string{
			{"Lemma": "kuusi", "UPOS": "NOUN"},
			{"Lemma": "kuusi", "UPOS": "NUM"},
		},
	})
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	seedForm(t, api, "kuusi", "kuusi", "NUM", "FI")
	seedLemma(t, api, "kuusi", "NUM", "six", "FI")
	seedLemma(t, api, "kuusi", "NOUN", "spruce", "FI")

	cookies := loginAndReturnCookies(t, mux, "deck2@example.com")
	user, err := api.store.GetOrCreateUser("deck2@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	// "bogus/PROPN" is not a supported candidate for kuusi → no card.
	saveDeck(t, mux, cookies, `{"title":"Crafted","lang":"FI","text":"Pihalla kasvaa kuusi.","selected_senses":[{"surface":"kuusi","lemma":"bogus","pos":"PROPN"}]}`)
	if hasCard(t, api, user.ID, "FI", "bogus", "PROPN") {
		t.Fatalf("unsupported crafted selection must not inject a card")
	}
}

// TestKnownWordImportReportsAmbiguousCount proves the import summary counts
// surfaces with more than one possible meaning (lazy resolution — no upfront
// disambiguation).
func TestKnownWordImportReportsAmbiguousCount(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	// Dict-only ET homograph joon → NOUN + jooma/VERB (no FST needed); vesi is
	// single-sense.
	seedForm(t, api, "joon", "joon", "NOUN", "ET")
	seedForm(t, api, "joon", "jooma", "VERB", "ET")
	seedForm(t, api, "vesi", "vesi", "NOUN", "ET")
	seedLemma(t, api, "joon", "NOUN", "line", "ET")
	seedLemma(t, api, "jooma", "VERB", "to drink", "ET")
	seedLemma(t, api, "vesi", "NOUN", "water", "ET")

	cookies := loginAndReturnCookies(t, mux, "import@example.com")
	req := httptest.NewRequest(http.MethodPost, "/api/known-words", strings.NewReader(`{"lang":"ET","words":["joon","vesi"]}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import status=%d body=%q", rec.Code, rec.Body.String())
	}
	var resp KnownWordsResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	if resp.NeedsSenseConfirmation != 1 {
		t.Fatalf("needs_sense_confirmation=%d want 1 (only joon is ambiguous)", resp.NeedsSenseConfirmation)
	}
}

// TestKnowThisMeaningRecordsKnownState proves the "I know this meaning" action
// records known state for the chosen (lemma, pos) via the current lemma-state
// model. The UI posts to /api/lemma-state with the candidate's identity; the
// server persists it so the sense is excluded from future study.
func TestKnowThisMeaningRecordsKnownState(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "know@example.com")
	user, err := api.store.GetOrCreateUser("know@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	// The "I know this meaning" button records the specific candidate sense.
	req := httptest.NewRequest(http.MethodPost, "/api/lemma-state", strings.NewReader(`{"lang":"FI","lemma":"kuusi","pos":"NOUN","status":"known"}`))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("know status=%d body=%q", rec.Code, rec.Body.String())
	}

	known, err := api.store.IsKnownOrIgnored(user.ID, "FI", "kuusi", "NOUN")
	if err != nil {
		t.Fatalf("IsKnownOrIgnored: %v", err)
	}
	if !known {
		t.Fatalf("expected kuusi/NOUN recorded known after 'I know this meaning'")
	}
	// The sibling sense must remain unknown — knowing one homograph sense does
	// not mark the other.
	otherKnown, err := api.store.IsKnownOrIgnored(user.ID, "FI", "kuusi", "NUM")
	if err != nil {
		t.Fatalf("IsKnownOrIgnored(NUM): %v", err)
	}
	if otherKnown {
		t.Fatalf("knowing kuusi/NOUN must not mark kuusi/NUM known")
	}
}

func saveDeck(t *testing.T, mux *http.ServeMux, cookies []*http.Cookie, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save deck status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func hasCard(t *testing.T, api *API, userID int64, lang, lemma, pos string) bool {
	t.Helper()
	ok, err := api.store.HasCardForTest(userID, lang, lemma, pos)
	if err != nil {
		t.Fatalf("HasCardForTest: %v", err)
	}
	return ok
}
