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
			name:       "missing text and chapters",
			method:     http.MethodPost,
			body:       `{"lang":"FI","text":""}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Text or chapters is required",
		},
		{
			name:       "both text and chapters",
			method:     http.MethodPost,
			body:       `{"lang":"FI","text":"hi","chapters":[{"title":"A","text":"hello"}]}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Provide either text or chapters, not both",
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

func TestHandleParseRejectsOversizedJSONBeforeAnalyze(t *testing.T) {
	api := newTestAPI(t)
	called := false
	api.analyze = func(_ *store.DB, _, _, _ string) (*parsecore.ParseResult, error) {
		called = true
		return nil, fmt.Errorf("analyze should not run")
	}
	mux := newTestMux(t, api)

	body := `{"lang":"FI","text":"` + strings.Repeat("a", maxJSONBodyBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(body))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("analyze was called for oversized request body")
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
			ParseDurationNs: 17_000_000,
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
					AnalyzeNs:          5_000_000,
					LookupFormsNs:      4_000_000,
					LookupGlossesNs:    3_000_000,
					ResolveSentencesNs: 2_000_000,
					EnrichWordsNs:      1_000_000,
					TotalNs:            17_000_000,
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
		t.Fatalf("parse_duration_ms=%v want 17", resp.ParseDurationMs)
	}
	if resp.Stats.UniqueForms != 2 {
		t.Fatalf("stats.unique_forms=%d want 2", resp.Stats.UniqueForms)
	}
	if resp.Stats.ResolvedTokens != 2 {
		t.Fatalf("stats.resolved_tokens=%d want 2", resp.Stats.ResolvedTokens)
	}
	if resp.Stats.Timings.TotalNs != 17_000_000 {
		t.Fatalf("stats.timings.total_ns=%d want 17_000_000", resp.Stats.Timings.TotalNs)
	}
	if len(resp.Words) != 2 {
		t.Fatalf("words=%d want 2", len(resp.Words))
	}
	if resp.Words[0].Lemma != "kissa" {
		t.Fatalf("first lemma=%q want kissa", resp.Words[0].Lemma)
	}
}

func TestHandleParseChaptersPayloadReturnsPerChapterWordsAndState(t *testing.T) {
	api := newTestAPI(t)
	textCalled := 0
	api.analyze = func(_ *store.DB, _, _, _ string) (*parsecore.ParseResult, error) {
		textCalled++
		return nil, nil
	}
	idx := func(i int) *int { return &i }
	api.analyzeChapters = func(_ *store.DB, lang string, chapters []parsecore.ChapterInput, parser string) (*parsecore.ParseResult, error) {
		if parser == "" {
			parser = "custom"
		}
		// Two chapters, three sentences, two of which belong to chapter 0.
		// kissa appears in both chapters so the per-chapter Words split must
		// not just be a filter on whole-book Words.
		return &parsecore.ParseResult{
			Lang:            lang,
			Parser:          parser,
			TotalTokens:     5,
			ParseDurationNs: 12_000_000,
			Stats:           parsecore.ParseStats{},
			Sentences: []parsecore.SentenceResult{
				{
					Text:       "Kissa juoksee.",
					ChapterIdx: idx(0),
					Tokens: []parsecore.TokenResult{
						{Form: "Kissa", Lemma: "kissa", POS: "NOUN", Resolved: true},
						{Form: "juoksee", Lemma: "juosta", POS: "VERB", Resolved: true},
					},
				},
				{
					Text:       "Lisää kissoja.",
					ChapterIdx: idx(0),
					Tokens: []parsecore.TokenResult{
						{Form: "kissoja", Lemma: "kissa", POS: "NOUN", Resolved: true},
					},
				},
				{
					Text:       "Kissa nukkuu.",
					ChapterIdx: idx(1),
					Tokens: []parsecore.TokenResult{
						{Form: "Kissa", Lemma: "kissa", POS: "NOUN", Resolved: true},
						{Form: "nukkuu", Lemma: "nukkua", POS: "VERB", Resolved: true},
					},
				},
			},
			Words: []parsecore.WordEntry{
				{Lemma: "kissa", POS: "NOUN", Forms: []string{"Kissa", "kissoja"}, Count: 3},
				{Lemma: "juosta", POS: "VERB", Forms: []string{"juoksee"}, Count: 1},
				{Lemma: "nukkua", POS: "VERB", Forms: []string{"nukkuu"}, Count: 1},
			},
			Chapters: []parsecore.ChapterResult{
				{Title: "First", CharCount: 30, SentenceCount: 2, TokenCount: 3, ResolvedTokens: 3},
				{Title: "Second", CharCount: 15, SentenceCount: 1, TokenCount: 2, ResolvedTokens: 2},
			},
		}, nil
	}
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "chapters-user@example.com")

	// Pre-mark kissa as known so we can verify learning_state propagates onto
	// the per-chapter Words too, not just the whole-book Words.
	markReq := httptest.NewRequest(http.MethodPost, "/api/lemma-state", strings.NewReader(`{"lang":"FI","lemma":"kissa","pos":"NOUN","status":"known"}`))
	for _, c := range cookies {
		markReq.AddCookie(c)
	}
	if markRec := httptest.NewRecorder(); true {
		mux.ServeHTTP(markRec, markReq)
		if markRec.Code != http.StatusOK {
			t.Fatalf("mark kissa known: status=%d body=%q", markRec.Code, markRec.Body.String())
		}
	}

	body := `{"lang":"FI","parser":"custom","chapters":[{"title":"First","text":"Kissa juoksee. Lisää kissoja."},{"title":"Second","text":"Kissa nukkuu."}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(body))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if textCalled != 0 {
		t.Fatalf("analyze (text path) was called %d times; chapters payload must not fall through to the text analyzer", textCalled)
	}

	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Chapters) != 2 {
		t.Fatalf("len(chapters)=%d want 2", len(resp.Chapters))
	}
	// Per-chapter Words must be populated by the handler's per-chapter
	// expansion pass — the mock doesn't pre-fill Words on Chapters[].
	if len(resp.Chapters[0].Words) == 0 {
		t.Fatalf("chapter 0 has no Words; handler should re-aggregate per chapter")
	}
	if len(resp.Chapters[1].Words) == 0 {
		t.Fatalf("chapter 1 has no Words; handler should re-aggregate per chapter")
	}
	// LemmaCount should match the produced Words length (handler sets it).
	if resp.Chapters[0].LemmaCount != len(resp.Chapters[0].Words) {
		t.Fatalf("chapter 0 lemma_count=%d but len(words)=%d", resp.Chapters[0].LemmaCount, len(resp.Chapters[0].Words))
	}
	// kissa must appear in both chapters' Words (it's in both).
	kissaIn := func(words []WordEntry) bool {
		for _, w := range words {
			if w.Lemma == "kissa" && w.POS == "NOUN" {
				return true
			}
		}
		return false
	}
	if !kissaIn(resp.Chapters[0].Words) || !kissaIn(resp.Chapters[1].Words) {
		t.Fatalf("kissa missing from a chapter's words; ch0=%+v ch1=%+v", resp.Chapters[0].Words, resp.Chapters[1].Words)
	}
	// nukkua must appear ONLY in chapter 1, never in chapter 0.
	for _, w := range resp.Chapters[0].Words {
		if w.Lemma == "nukkua" {
			t.Fatalf("nukkua leaked into chapter 0; sentence subset is broken")
		}
	}
	// learning_state must apply to both whole-book and per-chapter kissa.
	wholeBookKissaKnown := false
	for _, w := range resp.Words {
		if w.Lemma == "kissa" && w.POS == "NOUN" && w.LearningState == "known" {
			wholeBookKissaKnown = true
		}
	}
	if !wholeBookKissaKnown {
		t.Fatalf("whole-book kissa missing learning_state=known: %+v", resp.Words)
	}
	for chIdx, ch := range resp.Chapters {
		gotKnown := false
		for _, w := range ch.Words {
			if w.Lemma == "kissa" && w.POS == "NOUN" && w.LearningState == "known" {
				gotKnown = true
			}
		}
		if !gotKnown {
			t.Fatalf("chapter %d kissa missing learning_state=known: %+v", chIdx, ch.Words)
		}
	}
}

func TestHandleParseChaptersPayloadKeepsEmptyChapterWordsEmpty(t *testing.T) {
	api := newTestAPI(t)
	idx := func(i int) *int { return &i }
	api.analyzeChapters = func(_ *store.DB, lang string, chapters []parsecore.ChapterInput, parser string) (*parsecore.ParseResult, error) {
		if parser == "" {
			parser = "custom"
		}
		return &parsecore.ParseResult{
			Lang:            lang,
			Parser:          parser,
			TotalTokens:     1,
			ParseDurationNs: 4_000_000,
			Sentences: []parsecore.SentenceResult{
				{
					Text:       "Kissa.",
					ChapterIdx: idx(0),
					Tokens: []parsecore.TokenResult{
						{Form: "Kissa", Lemma: "kissa", POS: "NOUN", Resolved: true},
					},
				},
			},
			Words: []parsecore.WordEntry{
				{Lemma: "kissa", POS: "NOUN", Forms: []string{"Kissa"}, Count: 1, ExampleSentence: "whole-book parser entry"},
			},
			Chapters: []parsecore.ChapterResult{
				{
					Title: "Cat", Words: []parsecore.WordEntry{
						{Lemma: "kissa", POS: "NOUN", Forms: []string{"Kissa"}, Count: 1, ExampleSentence: "chapter parser entry"},
					},
				},
				{Title: "Empty"},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	body := `{"lang":"FI","parser":"custom","chapters":[{"title":"Cat","text":"Kissa."},{"title":"Empty","text":"   "}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Chapters) != 2 {
		t.Fatalf("len(chapters)=%d want 2", len(resp.Chapters))
	}
	if got := resp.Chapters[1].Words; len(got) != 0 {
		t.Fatalf("empty chapter words=%+v, want empty instead of whole-book fallback", got)
	}
	if resp.Chapters[1].LemmaCount != 0 {
		t.Fatalf("empty chapter lemma_count=%d want 0", resp.Chapters[1].LemmaCount)
	}
	if len(resp.Chapters[0].Words) == 0 || resp.Chapters[0].Words[0].ExampleSentence != "chapter parser entry" {
		t.Fatalf("chapter parser metadata not preserved: %+v", resp.Chapters[0].Words)
	}
}

func TestHandleParseDoesNotPersistForAuthenticatedUser(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		if parser == "" {
			parser = "custom"
		}
		return &parsecore.ParseResult{
			Lang:            lang,
			Parser:          parser,
			TotalTokens:     2,
			ParseDurationNs: 11_000_000,
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
	// /api/parse is no longer persisting — feedback flow lazily creates the
	// parse_session at submit time, deck save creates one too. A bare parse
	// press should leave parse_sessions untouched.
	if resp.ParseID != nil {
		t.Fatalf("expected parse_id to be nil (parse is ephemeral), got %v", resp.ParseID)
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
			ParseDurationNs: 11_000_000,
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

// TestHandleParseExpandsHomonyms verifies that the import overview's words
// list is dict-expanded the same way handleCreateDeck expands tokens, so the
// unique-lemma count in the import overview matches the deck's unique count.
func TestHandleParseExpandsHomonyms(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("joon", "NOUN", "line", "ET"); err != nil {
		t.Fatalf("UpsertLemma joon: %v", err)
	}
	if err := api.store.UpsertLemma("jooma", "VERB", "drink", "ET"); err != nil {
		t.Fatalf("UpsertLemma jooma: %v", err)
	}
	for _, r := range [][4]string{
		{"joon", "joon", "NOUN", "ET"},
		{"joon", "jooma", "VERB", "ET"},
	} {
		if err := api.store.UpsertForm(r[0], r[1], r[2], r[3]); err != nil {
			t.Fatalf("UpsertForm %v: %v", r, err)
		}
	}

	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang:        lang,
			Parser:      "custom",
			TotalTokens: 3,
			Sentences: []parsecore.SentenceResult{
				{
					Text: "Ma joon vett.",
					Tokens: []parsecore.TokenResult{
						{Form: "Ma", Lemma: "mina", POS: "PRON"},
						{Form: "joon", Lemma: "jooma", POS: "VERB"},
						{Form: "vett", Lemma: "vesi", POS: "NOUN"},
						{Form: ".", POS: "PUNCT"},
					},
				},
			},
			// Parser's pick: only one entry for "joon" (jooma/VERB).
			Words: []parsecore.WordEntry{
				{Lemma: "mina", POS: "PRON", Forms: []string{"Ma"}, Count: 1},
				{Lemma: "jooma", POS: "VERB", Forms: []string{"joon"}, Count: 1, Gloss: "drink"},
				{Lemma: "vesi", POS: "NOUN", Forms: []string{"vett"}, Count: 1},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"ET","text":"Ma joon vett."}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}

	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Parser produced 3 entries (mina, jooma, vesi). After expansion the
	// dict-known homonym joon/NOUN appears too — total 4, matching what the
	// deck would have.
	if len(resp.Words) != 4 {
		t.Fatalf("len(words)=%d want 4 (mina, jooma, vesi, joon-noun): %+v", len(resp.Words), resp.Words)
	}
	byKey := map[string]parsecore.WordEntry{}
	for _, w := range resp.Words {
		byKey[w.Lemma+"/"+w.POS] = w
	}
	joonNoun, ok := byKey["joon/NOUN"]
	if !ok {
		t.Fatalf("joon/NOUN missing from expanded words: %+v", resp.Words)
	}
	if joonNoun.Gloss != "line" {
		t.Errorf("joon/NOUN gloss=%q want line", joonNoun.Gloss)
	}
	if len(joonNoun.Forms) != 1 || joonNoun.Forms[0] != "joon" {
		t.Errorf("joon/NOUN forms=%v want [joon]", joonNoun.Forms)
	}
	if joonNoun.Count != 1 {
		t.Errorf("joon/NOUN count=%d want 1", joonNoun.Count)
	}

	// Order contract matches parsecore.enrichWords / GetDeckDetails: count
	// desc, lemma asc. Map iteration in expandParsedWords would otherwise
	// produce non-deterministic ordering and silently drift the API contract.
	for i := 1; i < len(resp.Words); i++ {
		prev, cur := resp.Words[i-1], resp.Words[i]
		if cur.Count > prev.Count {
			t.Errorf("words[%d..%d] not count-desc: %s(count=%d) before %s(count=%d)",
				i-1, i, prev.Lemma, prev.Count, cur.Lemma, cur.Count)
		}
		if cur.Count == prev.Count && cur.Lemma < prev.Lemma {
			t.Errorf("words[%d..%d] tie not lemma-asc: %s before %s",
				i-1, i, prev.Lemma, cur.Lemma)
		}
	}
	// Forms within each entry are alphabetical (parsecore.enrichWords:729 +
	// GetDeckDetails:1223 contract).
	for _, w := range resp.Words {
		for j := 1; j < len(w.Forms); j++ {
			if w.Forms[j-1] > w.Forms[j] {
				t.Errorf("%s/%s forms not sorted: %v", w.Lemma, w.POS, w.Forms)
				break
			}
		}
	}
}

func TestHandleParseUsesLexOverlayWhenExpandingWords(t *testing.T) {
	tests := []struct {
		name       string
		lang       string
		text       string
		lemmas     [][4]string
		forms      [][4]string
		tokens     []parsecore.TokenResult
		words      []parsecore.WordEntry
		wantKeys   []string
		rejectKeys []string
	}{
		{
			name: "FI overlay beats raw dict traps",
			lang: "FI",
			text: "varsin vuotta",
			lemmas: [][4]string{
				{"varsin", "ADV", "quite", "FI"},
				{"vuosi", "NOUN", "year", "FI"},
				{"varsi", "NOUN", "stalk", "FI"},
				{"vuo", "NOUN", "stream", "FI"},
			},
			forms: [][4]string{
				{"varsin", "varsi", "NOUN", "FI"},
				{"vuotta", "vuo", "NOUN", "FI"},
			},
			tokens: []parsecore.TokenResult{
				{Form: "varsin", Lemma: "varsin", POS: "ADV", Source: "lex-overlay", Resolved: true},
				{Form: "vuotta", Lemma: "vuosi", POS: "NOUN", Source: "lex-overlay", Resolved: true},
			},
			words: []parsecore.WordEntry{
				{Lemma: "varsin", POS: "ADV", Forms: []string{"varsin"}, Count: 1, Gloss: "quite"},
				{Lemma: "vuosi", POS: "NOUN", Forms: []string{"vuotta"}, Count: 1, Gloss: "year"},
			},
			wantKeys:   []string{"varsin/ADV", "vuosi/NOUN"},
			rejectKeys: []string{"varsi/NOUN", "vuo/NOUN"},
		},
		{
			name: "ET overlay beats raw dict traps",
			lang: "ET",
			text: "Ta välja peale sisse veel jaoks",
			lemmas: [][4]string{
				{"tema", "PRON", "he; she", "ET"},
				{"välja", "ADV", "out", "ET"},
				{"peale", "ADP", "on", "ET"},
				{"sisse", "ADV", "in", "ET"},
				{"veel", "ADV", "still", "ET"},
				{"jaoks", "ADP", "for", "ET"},
				{"TA", "NOUN", "technical abbreviation", "ET"},
				{"Ta", "X", "raw symbol", "ET"},
				{"tema", "NOUN", "his or her", "ET"},
				{"väli", "NOUN", "field", "ET"},
				{"väljama", "VERB", "raw verb", "ET"},
				{"pea", "NOUN", "head", "ET"},
				{"siss", "NOUN", "ranger", "ET"},
				{"vesi", "NOUN", "water", "ET"},
				{"jagu", "NOUN", "part", "ET"},
			},
			forms: [][4]string{
				{"ta", "TA", "NOUN", "ET"},
				{"ta", "Ta", "X", "ET"},
				{"ta", "tema", "NOUN", "ET"},
				{"välja", "väli", "NOUN", "ET"},
				{"välja", "väljama", "VERB", "ET"},
				{"peale", "pea", "NOUN", "ET"},
				{"sisse", "siss", "NOUN", "ET"},
				{"veel", "vesi", "NOUN", "ET"},
				{"jaoks", "jagu", "NOUN", "ET"},
			},
			tokens: []parsecore.TokenResult{
				{Form: "Ta", Lemma: "tema", POS: "PRON", Source: "lex-overlay", Resolved: true},
				{Form: "välja", Lemma: "välja", POS: "ADV", Source: "lex-overlay", Resolved: true},
				{Form: "peale", Lemma: "peale", POS: "ADP", Source: "lex-overlay", Resolved: true},
				{Form: "sisse", Lemma: "sisse", POS: "ADV", Source: "lex-overlay", Resolved: true},
				{Form: "veel", Lemma: "veel", POS: "ADV", Source: "lex-overlay", Resolved: true},
				{Form: "jaoks", Lemma: "jaoks", POS: "ADP", Source: "lex-overlay", Resolved: true},
			},
			words: []parsecore.WordEntry{
				{Lemma: "tema", POS: "PRON", Forms: []string{"Ta"}, Count: 1, Gloss: "he; she"},
				{Lemma: "välja", POS: "ADV", Forms: []string{"välja"}, Count: 1, Gloss: "out"},
				{Lemma: "peale", POS: "ADP", Forms: []string{"peale"}, Count: 1, Gloss: "on"},
				{Lemma: "sisse", POS: "ADV", Forms: []string{"sisse"}, Count: 1, Gloss: "in"},
				{Lemma: "veel", POS: "ADV", Forms: []string{"veel"}, Count: 1, Gloss: "still"},
				{Lemma: "jaoks", POS: "ADP", Forms: []string{"jaoks"}, Count: 1, Gloss: "for"},
			},
			wantKeys: []string{
				"tema/PRON", "välja/ADV", "peale/ADP", "sisse/ADV", "veel/ADV", "jaoks/ADP",
			},
			rejectKeys: []string{
				"TA/NOUN", "Ta/X", "tema/NOUN", "väli/NOUN", "väljama/VERB", "pea/NOUN",
				"siss/NOUN", "vesi/NOUN", "jagu/NOUN",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newTestAPI(t)
			for _, r := range tt.lemmas {
				if err := api.store.UpsertLemma(r[0], r[1], r[2], r[3]); err != nil {
					t.Fatalf("UpsertLemma %v: %v", r, err)
				}
			}
			for _, r := range tt.forms {
				if err := api.store.UpsertForm(r[0], r[1], r[2], r[3]); err != nil {
					t.Fatalf("UpsertForm %v: %v", r, err)
				}
			}
			api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
				return &parsecore.ParseResult{
					Lang:        lang,
					Parser:      "custom",
					TotalTokens: len(tt.tokens),
					Stats: parsecore.ParseStats{SourceCounts: map[string]int{
						"lex-overlay": len(tt.tokens),
					}},
					Sentences: []parsecore.SentenceResult{{
						Text:   text,
						Tokens: tt.tokens,
					}},
					Words: tt.words,
				}, nil
			}
			mux := newTestMux(t, api)

			req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(fmt.Sprintf(`{"lang":%q,"text":%q,"parser":"custom"}`, tt.lang, tt.text)))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
			}

			var resp ParseResponse
			if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			byKey := map[string]parsecore.WordEntry{}
			for _, w := range resp.Words {
				byKey[w.Lemma+"/"+w.POS] = w
			}
			for _, key := range tt.wantKeys {
				if _, ok := byKey[key]; !ok {
					t.Fatalf("%s missing from words: %+v", key, resp.Words)
				}
			}
			for _, key := range tt.rejectKeys {
				if _, ok := byKey[key]; ok {
					t.Fatalf("%s leaked into words: %+v", key, resp.Words)
				}
			}
		})
	}
}

func TestHandleParseBasicModeDoesNotUseLexOverlayWhenExpandingWords(t *testing.T) {
	api := newTestAPI(t)
	for _, r := range [][4]string{
		{"varsin", "ADV", "quite", "FI"},
		{"varsi", "NOUN", "stalk", "FI"},
	} {
		if err := api.store.UpsertLemma(r[0], r[1], r[2], r[3]); err != nil {
			t.Fatalf("UpsertLemma %v: %v", r, err)
		}
	}
	if err := api.store.UpsertForm("varsin", "varsi", "NOUN", "FI"); err != nil {
		t.Fatalf("UpsertForm varsin: %v", err)
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang:        lang,
			Parser:      "basic",
			TotalTokens: 1,
			Sentences: []parsecore.SentenceResult{{
				Text: text,
				Tokens: []parsecore.TokenResult{
					{Form: "varsin", Lemma: "varsi", POS: "NOUN", Source: "dict", Resolved: true},
				},
			}},
			Words: []parsecore.WordEntry{
				{Lemma: "varsi", POS: "NOUN", Forms: []string{"varsin"}, Count: 1, Gloss: "stalk"},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"FI","text":"varsin","parser":"basic"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}

	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Words) != 1 || resp.Words[0].Lemma != "varsi" || resp.Words[0].POS != "NOUN" {
		t.Fatalf("words=%+v want basic raw dict varsi/NOUN", resp.Words)
	}
}

func TestHandleParseSuppressesGlosslessAlternativeWhenSurfaceHasGlossedCandidate(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("liiga", "ADV", "too", "ET"); err != nil {
		t.Fatalf("UpsertLemma liiga/ADV: %v", err)
	}
	if err := api.store.UpsertLemma("liiga", "X", "", "ET"); err != nil {
		t.Fatalf("UpsertLemma liiga/X: %v", err)
	}
	for _, r := range [][4]string{
		{"liiga", "liiga", "ADV", "ET"},
		{"liiga", "liiga", "X", "ET"},
	} {
		if err := api.store.UpsertForm(r[0], r[1], r[2], r[3]); err != nil {
			t.Fatalf("UpsertForm %v: %v", r, err)
		}
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang:   lang,
			Parser: "custom",
			Sentences: []parsecore.SentenceResult{
				{
					Text: "liiga",
					Tokens: []parsecore.TokenResult{
						{Form: "liiga", Lemma: "liiga", POS: "ADV", Resolved: true},
					},
				},
			},
			Words: []parsecore.WordEntry{
				{Lemma: "liiga", POS: "ADV", Forms: []string{"liiga"}, Count: 1, Gloss: "too"},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"ET","text":"liiga","parser":"custom"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}

	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Words) != 1 {
		t.Fatalf("len(words)=%d want 1; words=%+v", len(resp.Words), resp.Words)
	}
	if got := resp.Words[0]; got.Lemma != "liiga" || got.POS != "ADV" || got.Gloss != "too" {
		t.Fatalf("word=%+v want liiga/ADV with gloss", got)
	}
}

func TestHandleParseSuppressesInflectedFormGlossWhenSurfaceHasLexicalCandidate(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("olema", "VERB", "be", "ET"); err != nil {
		t.Fatalf("UpsertLemma olema/VERB: %v", err)
	}
	if err := api.store.UpsertLemma("olen", "VERB", "first-person singular present indicative of olema", "ET"); err != nil {
		t.Fatalf("UpsertLemma olen/VERB: %v", err)
	}
	for _, r := range [][4]string{
		{"olen", "olema", "VERB", "ET"},
		{"olen", "olen", "VERB", "ET"},
	} {
		if err := api.store.UpsertForm(r[0], r[1], r[2], r[3]); err != nil {
			t.Fatalf("UpsertForm %v: %v", r, err)
		}
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang:   lang,
			Parser: "custom",
			Sentences: []parsecore.SentenceResult{
				{
					Text: "olen",
					Tokens: []parsecore.TokenResult{
						{Form: "olen", Lemma: "olema", POS: "VERB", Resolved: true},
					},
				},
			},
			Words: []parsecore.WordEntry{
				{Lemma: "olema", POS: "VERB", Forms: []string{"olen"}, Count: 1, Gloss: "be"},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"ET","text":"olen","parser":"custom"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}

	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Words) != 1 {
		t.Fatalf("len(words)=%d want 1; words=%+v", len(resp.Words), resp.Words)
	}
	if got := resp.Words[0]; got.Lemma != "olema" || got.POS != "VERB" || got.Gloss != "be" {
		t.Fatalf("word=%+v want olema/VERB with lexical gloss", got)
	}
}

func TestHandleParseKeepsParserProtectedLemmaWhenRawDictHasTrap(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("tuskin", "ADV", "hardly", "FI"); err != nil {
		t.Fatalf("UpsertLemma tuskin/ADV: %v", err)
	}
	if err := api.store.UpsertLemma("tuska", "NOUN", "pain", "FI"); err != nil {
		t.Fatalf("UpsertLemma tuska/NOUN: %v", err)
	}
	if err := api.store.UpsertForm("tuskin", "tuska", "NOUN", "FI"); err != nil {
		t.Fatalf("UpsertForm tuskin->tuska: %v", err)
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang:   lang,
			Parser: "custom",
			Sentences: []parsecore.SentenceResult{
				{
					Text: "Tuskin.",
					Tokens: []parsecore.TokenResult{
						{Form: "Tuskin", Lemma: "tuskin", POS: "ADV", Source: "lex-overlay", Resolved: true},
						{Form: ".", POS: "PUNCT"},
					},
				},
			},
			Words: []parsecore.WordEntry{
				{Lemma: "tuskin", POS: "ADV", Forms: []string{"Tuskin"}, Count: 1, Gloss: "hardly"},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"FI","text":"Tuskin.","parser":"custom"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}

	var resp ParseResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Words) != 1 {
		t.Fatalf("len(words)=%d want 1 protected parser lemma: %+v", len(resp.Words), resp.Words)
	}
	if got := resp.Words[0]; got.Lemma != "tuskin" || got.POS != "ADV" || got.Gloss != "hardly" {
		t.Fatalf("word=%+v want protected tuskin/ADV", got)
	}
}

func TestFilterLowValueAlternativesKeepsLexicalGlossesWithLooseMarkers(t *testing.T) {
	dict := map[string][]store.FormResolution{
		"vana": {
			{Lemma: "vana", POS: "ADJ"},
			{Lemma: "vana", POS: "NOUN"},
		},
		"vanad": {
			{Lemma: "vana", POS: "ADJ"},
			{Lemma: "vana", POS: "NOUN"},
		},
		"oma": {
			{Lemma: "oma", POS: "ADJ"},
			{Lemma: "oma", POS: "NOUN"},
		},
	}
	glosses := map[store.LemmaKey]string{
		{Lemma: "vana", POS: "ADJ"}:  "old; out of order; past",
		{Lemma: "vana", POS: "NOUN"}: "old person",
		{Lemma: "oma", POS: "ADJ"}:   "own; one of a kind; singular",
		{Lemma: "oma", POS: "NOUN"}:  "property",
	}

	got := filterLowValueAlternatives(dict, glosses)
	for form, candidates := range got {
		seen := map[string]bool{}
		for _, candidate := range candidates {
			seen[candidate.Lemma+"/"+candidate.POS] = true
		}
		switch form {
		case "vana", "vanad":
			if !seen["vana/ADJ"] || !seen["vana/NOUN"] {
				t.Fatalf("%s candidates=%+v want both lexical ADJ and NOUN retained", form, candidates)
			}
		case "oma":
			if !seen["oma/ADJ"] || !seen["oma/NOUN"] {
				t.Fatalf("%s candidates=%+v want both lexical ADJ and NOUN retained", form, candidates)
			}
		default:
			t.Fatalf("unexpected form %q", form)
		}
	}
}

func TestFilterLowValueAlternativesKeepsFormOfWhenNoLexicalCandidate(t *testing.T) {
	dict := map[string][]store.FormResolution{
		"olen": {{Lemma: "olen", POS: "VERB"}},
	}
	glosses := map[store.LemmaKey]string{
		{Lemma: "olen", POS: "VERB"}: "first-person singular present indicative of olema",
	}

	got := filterLowValueAlternatives(dict, glosses)
	candidates := got["olen"]
	if len(candidates) != 1 || candidates[0].Lemma != "olen" || candidates[0].POS != "VERB" {
		t.Fatalf("candidates=%+v want form-of candidate retained when no lexical base exists", candidates)
	}
}

func TestFilterLowValueAlternativesSuppressesSlashAndConnegativeFormOfAlternatives(t *testing.T) {
	dict := map[string][]store.FormResolution{
		"aega": {
			{Lemma: "aeg", POS: "NOUN"},
			{Lemma: "aega", POS: "NOUN"},
		},
		"menne": {
			{Lemma: "mennä", POS: "VERB"},
			{Lemma: "menne", POS: "VERB"},
		},
	}
	glosses := map[store.LemmaKey]string{
		{Lemma: "aeg", POS: "NOUN"}:   "time",
		{Lemma: "aega", POS: "NOUN"}:  "genitive/partitive/illative singular of aeg",
		{Lemma: "mennä", POS: "VERB"}: "go",
		{Lemma: "menne", POS: "VERB"}: "connegative potential of mennä",
	}

	got := filterLowValueAlternatives(dict, glosses)
	if candidates := got["aega"]; len(candidates) != 1 || candidates[0].Lemma != "aeg" || candidates[0].POS != "NOUN" {
		t.Fatalf("aega candidates=%+v want only lexical aeg/NOUN", candidates)
	}
	if candidates := got["menne"]; len(candidates) != 1 || candidates[0].Lemma != "mennä" || candidates[0].POS != "VERB" {
		t.Fatalf("menne candidates=%+v want only lexical mennä/VERB", candidates)
	}
}

func TestFilterLowValueAlternativesSuppressesDefinitionFallbackAndAcronymHomonyms(t *testing.T) {
	dict := map[string][]store.FormResolution{
		"ei": {
			{Lemma: "ei", POS: "ADV"},
			{Lemma: "ei", POS: "INTJ"},
			{Lemma: "ei", POS: "NOUN"},
			{Lemma: "ei", POS: "X"},
		},
		"ta": {
			{Lemma: "ta", POS: "PRON"},
			{Lemma: "TA", POS: "NOUN"},
			{Lemma: "Ta", POS: "X"},
		},
		"ja": {
			{Lemma: "ja", POS: "CCONJ"},
			{Lemma: "ja", POS: "X"},
		},
	}
	glosses := map[store.LemmaKey]string{
		{Lemma: "ei", POS: "ADV"}:   "not; no",
		{Lemma: "ei", POS: "INTJ"}:  "[ET] esineb rõõmu väljendavates lausetes",
		{Lemma: "ei", POS: "NOUN"}:  "[ET] ei-sõna, ei-otsus",
		{Lemma: "ei", POS: "X"}:     "[ET] kogu lause v öeldu kohta",
		{Lemma: "ta", POS: "PRON"}:  "he; she",
		{Lemma: "TA", POS: "NOUN"}:  "R&D",
		{Lemma: "Ta", POS: "X"}:     "[ET] keemiline element",
		{Lemma: "ja", POS: "CCONJ"}: "and",
		{Lemma: "ja", POS: "X"}:     "and",
	}

	got := filterLowValueAlternatives(dict, glosses)
	assertOnlyCandidate := func(form, lemma, pos string) {
		t.Helper()
		candidates := got[form]
		if len(candidates) != 1 || candidates[0].Lemma != lemma || candidates[0].POS != pos {
			t.Fatalf("%s candidates=%+v want only %s/%s", form, candidates, lemma, pos)
		}
	}
	assertOnlyCandidate("ei", "ei", "ADV")
	assertOnlyCandidate("ta", "ta", "PRON")
	assertOnlyCandidate("ja", "ja", "CCONJ")
}

func TestExpandParsedWordsOrdersSameLemmaByPOS(t *testing.T) {
	api := newTestAPI(t)
	parsed := &parsecore.ParseResult{
		Lang: "ET",
		Sentences: []parsecore.SentenceResult{
			{
				Text: "joon",
				Tokens: []parsecore.TokenResult{
					{Form: "joon", Lemma: "joon", POS: "NOUN"},
				},
			},
		},
	}
	dict := map[string][]store.FormResolution{
		"joon": {
			{Lemma: "joon", POS: "VERB"},
			{Lemma: "joon", POS: "NOUN"},
		},
	}

	got := api.expandParsedWords(parsed, dict, nil, nil)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(got), got)
	}
	if got[0].Lemma != "joon" || got[0].POS != "NOUN" {
		t.Fatalf("first=%s/%s want joon/NOUN: %+v", got[0].Lemma, got[0].POS, got)
	}
	if got[1].Lemma != "joon" || got[1].POS != "VERB" {
		t.Fatalf("second=%s/%s want joon/VERB: %+v", got[1].Lemma, got[1].POS, got)
	}
}

func TestExpandParsedWordsUsesPreloadedGlosses(t *testing.T) {
	api := newTestAPI(t)
	parsed := &parsecore.ParseResult{
		Lang: "ET",
		Sentences: []parsecore.SentenceResult{
			{
				Text: "joon",
				Tokens: []parsecore.TokenResult{
					{Form: "joon", Lemma: "joon", POS: "NOUN"},
				},
			},
		},
	}
	dict := map[string][]store.FormResolution{
		"joon": {{Lemma: "joon", POS: "NOUN"}},
	}
	key := store.LemmaKey{Lemma: "joon", POS: "NOUN"}

	got := api.expandParsedWords(parsed, dict, map[store.LemmaKey]string{key: "line"}, map[store.LemmaKey]struct{}{key: {}})
	if len(got) != 1 {
		t.Fatalf("len=%d want 1: %+v", len(got), got)
	}
	if got[0].Gloss != "line" {
		t.Fatalf("gloss=%q want line", got[0].Gloss)
	}
}

func TestExpandParsedWordsDoesNotCopyGrammarOntoMixedFormExpansion(t *testing.T) {
	api := newTestAPI(t)
	parsed := &parsecore.ParseResult{
		Lang: "ET",
		Sentences: []parsecore.SentenceResult{
			{
				Text: "Meile ja ma.",
				Tokens: []parsecore.TokenResult{
					{Form: "Meile", Lemma: "me", POS: "PRON"},
					{Form: "ma", Lemma: "mina", POS: "PRON"},
				},
			},
		},
		Words: []parsecore.WordEntry{
			{Lemma: "me", POS: "PRON", Forms: []string{"Meile"}, Count: 1, GrammarLabel: "allative", Feats: "Case=All|Number=Plur"},
		},
	}
	dict := map[string][]store.FormResolution{
		"Meile": {{Lemma: "me", POS: "PRON"}},
		"ma": {
			{Lemma: "mina", POS: "PRON"},
			{Lemma: "me", POS: "PRON"},
		},
	}

	got := api.expandParsedWords(parsed, dict, nil, nil)
	var expanded *parsecore.WordEntry
	for i := range got {
		if got[i].Lemma == "me" && got[i].POS == "PRON" {
			expanded = &got[i]
			break
		}
	}
	if expanded == nil {
		t.Fatalf("me/PRON missing from expanded words: %+v", got)
	}
	if expanded.GrammarLabel != "" || expanded.Feats != "" {
		t.Fatalf("expanded grammar=(%q,%q), want empty because forms=%v mix cases", expanded.GrammarLabel, expanded.Feats, expanded.Forms)
	}
}

func TestHandleParseMapsAnalyzerValidationErrorsToBadRequest(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, _, _, _ string) (*parsecore.ParseResult, error) {
		return nil, fmt.Errorf("text exceeds 1500000 character limit")
	}
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"FI","text":"x"}`))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "text exceeds 1500000 character limit") {
		t.Fatalf("body=%q missing analyzer error", rec.Body.String())
	}
}

func TestHandleParseRejectsLabParserModes(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/parse", strings.NewReader(`{"lang":"FI","text":"kissa","parser":"omorfi"}`))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), `unsupported parser "omorfi"`) {
		t.Fatalf("body=%q missing unsupported parser error", rec.Body.String())
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

func TestHandleRegisterReturnsAuthIdentity(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	body := fmt.Sprintf(`{"email":"alice@example.com","password":%q}`, testPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "session_token" || cookies[0].Value == "" {
		t.Fatalf("expected register to set a non-empty session_token cookie, got %+v", cookies)
	}

	var resp LoginResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if !resp.Authenticated {
		t.Fatal("expected authenticated register response")
	}
	if resp.User == nil || resp.User.Email != "alice@example.com" {
		t.Fatalf("expected alice@example.com user, got %+v", resp.User)
	}
	if resp.User.IsAdmin {
		t.Fatal("expected fresh register to be non-admin")
	}
}

func TestHandleLoginVerifiesPassword(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	loginAndReturnCookies(t, mux, "alice@example.com") // registers

	// Wrong password is rejected.
	wrongBody := `{"email":"alice@example.com","password":"wrong-password-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(wrongBody))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status=%d want 401 body=%q", rec.Code, rec.Body.String())
	}

	// Correct password succeeds.
	rightBody := fmt.Sprintf(`{"email":"alice@example.com","password":%q}`, testPassword)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(rightBody))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct password status=%d want 200 body=%q", rec.Code, rec.Body.String())
	}
}

func TestHandleLoginRejectsUnknownEmail(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	body := fmt.Sprintf(`{"email":"ghost@example.com","password":%q}`, testPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%q", rec.Code, rec.Body.String())
	}
}

func TestHandleRegisterRejectsDuplicateEmail(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	loginAndReturnCookies(t, mux, "alice@example.com") // first registration

	body := fmt.Sprintf(`{"email":"alice@example.com","password":%q}`, testPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409 body=%q", rec.Code, rec.Body.String())
	}
}

func TestHandleRegisterRejectsWeakPassword(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"email":"weak@example.com","password":"short"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%q", rec.Code, rec.Body.String())
	}
}

func TestHandleLoginBootstrapsExistingAccountWithoutPassword(t *testing.T) {
	// Simulate a pre-migration alpha account: created via GetOrCreateUser
	// with no password. The first /api/auth/login should accept any password
	// and persist it as the user's permanent password.
	api := newTestAPI(t)
	mux := newTestMux(t, api)

	if _, err := api.store.GetOrCreateUser("legacy@example.com"); err != nil {
		t.Fatalf("seed legacy user: %v", err)
	}

	// First login with chosen password sets it.
	body := fmt.Sprintf(`{"email":"legacy@example.com","password":%q}`, testPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first login status=%d want 200 body=%q", rec.Code, rec.Body.String())
	}

	// Subsequent login with a different password is rejected.
	wrongBody := `{"email":"legacy@example.com","password":"different-passwordxx"}`
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(wrongBody))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("second login (wrong pw) status=%d want 401 body=%q", rec.Code, rec.Body.String())
	}
}

func TestHandleLogoutClearsSessionCookieAndDropsAuth(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "alice@example.com")

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
		if c.Name == "session_token" {
			clearing = c
		}
	}
	if clearing == nil {
		t.Fatal("expected logout response to set the session_token cookie")
	}
	if clearing.MaxAge >= 0 {
		t.Fatalf("expected session_token cookie to be expired (MaxAge<0), got MaxAge=%d", clearing.MaxAge)
	}
	if clearing.Value != "" {
		t.Fatalf("expected cleared cookie value, got %q", clearing.Value)
	}

	// The original session token must also be revoked server-side, so replaying
	// the original cookies on /api/me should report unauthenticated.
	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	for _, c := range cookies {
		meReq.AddCookie(c)
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
	if meResp.Authenticated {
		t.Fatal("expected /api/me to be unauthenticated after logout (session must be revoked server-side)")
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
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "not-a-real-token"})
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

	// "qwerty123" is the deliberately-unresolvable element: it's not in the
	// dictionary or Voikko's FST. (Earlier the test used "tuntematon", but
	// that's a real Finnish word ("unknown" / negative-passive participle of
	// "tuntea") and the VFST step in BatchLookupForms now resolves it.)
	importReq := httptest.NewRequest(http.MethodPost, "/api/known-words", strings.NewReader(`{"lang":"FI","words":["kissoja","juoksen","qwerty123","juoksen"]}`))
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
	if len(importResp.Unresolved) != 1 || importResp.Unresolved[0] != "qwerty123" {
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

func TestKnownWordsDeleteAll(t *testing.T) {
	api := newTestAPI(t)
	// Seed FI and ET lemmas so we can prove the bulk delete is scoped per
	// language: wiping FI must NOT remove ET rows for the same user.
	if err := api.store.UpsertLemma("kissa", "NOUN", "cat", "FI"); err != nil {
		t.Fatalf("UpsertLemma FI: %v", err)
	}
	if err := api.store.UpsertForm("kissa", "kissa", "NOUN", "FI"); err != nil {
		t.Fatalf("UpsertForm FI: %v", err)
	}
	if err := api.store.UpsertLemma("juosta", "VERB", "run", "FI"); err != nil {
		t.Fatalf("UpsertLemma FI verb: %v", err)
	}
	if err := api.store.UpsertForm("juosta", "juosta", "VERB", "FI"); err != nil {
		t.Fatalf("UpsertForm FI verb: %v", err)
	}
	if err := api.store.UpsertLemma("kass", "NOUN", "cat", "ET"); err != nil {
		t.Fatalf("UpsertLemma ET: %v", err)
	}
	if err := api.store.UpsertForm("kass", "kass", "NOUN", "ET"); err != nil {
		t.Fatalf("UpsertForm ET: %v", err)
	}

	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "deleteall@example.com")

	importFI := httptest.NewRequest(http.MethodPost, "/api/known-words", strings.NewReader(`{"lang":"FI","words":["kissa","juosta"]}`))
	for _, c := range cookies {
		importFI.AddCookie(c)
	}
	importFIRec := httptest.NewRecorder()
	mux.ServeHTTP(importFIRec, importFI)
	if importFIRec.Code != http.StatusOK {
		t.Fatalf("import FI status=%d body=%q", importFIRec.Code, importFIRec.Body.String())
	}
	importET := httptest.NewRequest(http.MethodPost, "/api/known-words", strings.NewReader(`{"lang":"ET","words":["kass"]}`))
	for _, c := range cookies {
		importET.AddCookie(c)
	}
	importETRec := httptest.NewRecorder()
	mux.ServeHTTP(importETRec, importET)
	if importETRec.Code != http.StatusOK {
		t.Fatalf("import ET status=%d body=%q", importETRec.Code, importETRec.Body.String())
	}

	// all=1 combined with lemma/pos must be rejected so accidental clients
	// can't wipe a whole language by also passing per-row args.
	bad := httptest.NewRequest(http.MethodDelete, "/api/known-words?lang=FI&all=1&lemma=kissa&pos=NOUN", nil)
	for _, c := range cookies {
		bad.AddCookie(c)
	}
	badRec := httptest.NewRecorder()
	mux.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("all=1+lemma status=%d want 400 body=%q", badRec.Code, badRec.Body.String())
	}

	// all=1 without lang must still 400.
	noLang := httptest.NewRequest(http.MethodDelete, "/api/known-words?all=1", nil)
	for _, c := range cookies {
		noLang.AddCookie(c)
	}
	noLangRec := httptest.NewRecorder()
	mux.ServeHTTP(noLangRec, noLang)
	if noLangRec.Code != http.StatusBadRequest {
		t.Fatalf("all=1 no lang status=%d want 400", noLangRec.Code)
	}

	// Happy path: wipe FI.
	wipeFI := httptest.NewRequest(http.MethodDelete, "/api/known-words?lang=FI&all=1", nil)
	for _, c := range cookies {
		wipeFI.AddCookie(c)
	}
	wipeRec := httptest.NewRecorder()
	mux.ServeHTTP(wipeRec, wipeFI)
	if wipeRec.Code != http.StatusOK {
		t.Fatalf("wipe FI status=%d body=%q", wipeRec.Code, wipeRec.Body.String())
	}
	var wipeResp struct {
		Status  string `json:"status"`
		Deleted int64  `json:"deleted"`
	}
	if err := json.NewDecoder(bytes.NewReader(wipeRec.Body.Bytes())).Decode(&wipeResp); err != nil {
		t.Fatalf("decode wipe response: %v", err)
	}
	if wipeResp.Deleted != 2 {
		t.Fatalf("deleted=%d want 2", wipeResp.Deleted)
	}

	// FI list is empty.
	fiReq := httptest.NewRequest(http.MethodGet, "/api/known-words?lang=FI", nil)
	for _, c := range cookies {
		fiReq.AddCookie(c)
	}
	fiRec := httptest.NewRecorder()
	mux.ServeHTTP(fiRec, fiReq)
	var fiList KnownWordsListResponse
	_ = json.NewDecoder(bytes.NewReader(fiRec.Body.Bytes())).Decode(&fiList)
	if len(fiList.KnownWords) != 0 {
		t.Fatalf("FI after wipe=%d want 0 (%+v)", len(fiList.KnownWords), fiList.KnownWords)
	}

	// ET is untouched — the wipe is language-scoped.
	etReq := httptest.NewRequest(http.MethodGet, "/api/known-words?lang=ET", nil)
	for _, c := range cookies {
		etReq.AddCookie(c)
	}
	etRec := httptest.NewRecorder()
	mux.ServeHTTP(etRec, etReq)
	var etList KnownWordsListResponse
	_ = json.NewDecoder(bytes.NewReader(etRec.Body.Bytes())).Decode(&etList)
	if len(etList.KnownWords) != 1 {
		t.Fatalf("ET after FI wipe=%d want 1 (%+v)", len(etList.KnownWords), etList.KnownWords)
	}

	// A second wipe with nothing to delete still 200s and returns 0.
	wipeAgain := httptest.NewRequest(http.MethodDelete, "/api/known-words?lang=FI&all=1", nil)
	for _, c := range cookies {
		wipeAgain.AddCookie(c)
	}
	wipeAgainRec := httptest.NewRecorder()
	mux.ServeHTTP(wipeAgainRec, wipeAgain)
	if wipeAgainRec.Code != http.StatusOK {
		t.Fatalf("second wipe status=%d body=%q", wipeAgainRec.Code, wipeAgainRec.Body.String())
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

func TestLemmaStatesBatchLookupReturnsCurrentStates(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "lemma-state-batch@example.com")

	for _, body := range []string{
		`{"lang":"FI","lemma":"kissa","pos":"NOUN","status":"known"}`,
		`{"lang":"FI","lemma":"juosta","pos":"VERB","status":"ignored"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/lemma-state", strings.NewReader(body))
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("seed state status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lemma-states", strings.NewReader(`{
		"lang":"FI",
		"lemmas":[
			{"lemma":"kissa","pos":"NOUN"},
			{"lemma":"juosta","pos":"VERB"},
			{"lemma":"talo","pos":"NOUN"},
			{"lemma":"kissa","pos":"NOUN"}
		]
	}`))
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp LemmaStateLookupResponse
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]string{}
	for _, state := range resp.States {
		got[state.Lemma+"/"+state.POS] = state.Status
	}
	if got["kissa/NOUN"] != "known" {
		t.Fatalf("kissa/NOUN=%q want known (states=%+v)", got["kissa/NOUN"], resp.States)
	}
	if got["juosta/VERB"] != "ignored" {
		t.Fatalf("juosta/VERB=%q want ignored (states=%+v)", got["juosta/VERB"], resp.States)
	}
	if _, ok := got["talo/NOUN"]; ok {
		t.Fatalf("neutral talo/NOUN should be omitted from response: %+v", resp.States)
	}
}

func TestKnownWordsImportReturnsUnresolvedWithoutCreatingRows(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "unknown@example.com")

	// "qwerty123" and "asdfg" are deliberately unresolvable — neither in the
	// dictionary nor in Voikko's FST. (The earlier inputs "tuntematon" and
	// "mysteeri" both resolve via the VFST step now: tuntematon → tuntea/ADJ,
	// mysteeri → mysteeri/NOUN.)
	importReq := httptest.NewRequest(http.MethodPost, "/api/known-words", strings.NewReader(`{"lang":"FI","words":["qwerty123","asdfg"]}`))
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

// TestClearLemmaStateEnsuresCardWhenDeckSkippedSeeding covers both ways a
// deck-create can skip ensureCard for a lemma — the lemma was already known,
// or already ignored — and verifies that clearing the state via /api/lemma-state
// seeds a card so the lemma becomes reviewable. Both paths share the same
// ClearLemmaState body, but parallel coverage guards against a future
// regression that would split them.
func TestClearLemmaStateEnsuresCardWhenDeckSkippedSeeding(t *testing.T) {
	cases := []struct {
		name        string
		preState    string // "known" or "ignored" — applied before deck create
		emailSuffix string
	}{
		{name: "known", preState: "known", emailSuffix: "known"},
		{name: "ignored", preState: "ignored", emailSuffix: "ignored"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
			cookies := loginAndReturnCookies(t, mux, "clear-card-"+tc.emailSuffix+"@example.com")

			// Mark kissa with the pre-state via /api/lemma-state, mirroring the
			// existing real-user flows (known-word import for "known", trash-
			// icon button on a parse result for "ignored").
			preReq := httptest.NewRequest(http.MethodPost, "/api/lemma-state",
				strings.NewReader(`{"lang":"FI","lemma":"kissa","pos":"NOUN","status":"`+tc.preState+`"}`))
			for _, cookie := range cookies {
				preReq.AddCookie(cookie)
			}
			preRec := httptest.NewRecorder()
			mux.ServeHTTP(preRec, preReq)
			if preRec.Code != http.StatusOK {
				t.Fatalf("pre-state mark status=%d body=%q", preRec.Code, preRec.Body.String())
			}

			createReq := httptest.NewRequest(http.MethodPost, "/api/decks",
				strings.NewReader(`{"title":"Test deck","lang":"FI","text":"Kissa juoksee."}`))
			for _, cookie := range cookies {
				createReq.AddCookie(cookie)
			}
			createRec := httptest.NewRecorder()
			mux.ServeHTTP(createRec, createReq)
			if createRec.Code != http.StatusOK {
				t.Fatalf("create deck status=%d body=%q", createRec.Code, createRec.Body.String())
			}

			auth, err := api.getCurrentUser(requestWithCookies(httptest.NewRequest(http.MethodGet, "/api/me", nil), cookies))
			if err != nil || auth == nil {
				t.Fatal("expected authenticated user")
			}

			// Pre-condition: only juosta has a card; kissa was skipped because
			// it was already known/ignored at deck-create time.
			if cardCount, _ := api.store.CountCards(auth.UserID, "FI"); cardCount != 1 {
				t.Fatalf("pre-clear card_count=%d want 1", cardCount)
			}

			// Clear via the API — the user "marks as unknown" / "stops ignoring".
			clearReq := httptest.NewRequest(http.MethodPost, "/api/lemma-state",
				strings.NewReader(`{"lang":"FI","lemma":"kissa","pos":"NOUN","status":""}`))
			for _, cookie := range cookies {
				clearReq.AddCookie(cookie)
			}
			clearRec := httptest.NewRecorder()
			mux.ServeHTTP(clearRec, clearReq)
			if clearRec.Code != http.StatusOK {
				t.Fatalf("clear status=%d body=%q", clearRec.Code, clearRec.Body.String())
			}

			// Post-condition: kissa now has a card row, so it's reachable from
			// the review queue.
			cardCount, err := api.store.CountCards(auth.UserID, "FI")
			if err != nil {
				t.Fatalf("CountCards: %v", err)
			}
			if cardCount != 2 {
				t.Fatalf("post-clear card_count=%d want 2 (juosta + kissa now seeded)", cardCount)
			}
		})
	}
}

// TestCreateDeckExpandsAmbiguousTokenIntoMultipleCards verifies the
// multi-lemma model: when the dict knows that "joon" is both noun and 1Sg of
// jooma, a single occurrence in the source text creates one card per
// candidate. Both lemmas register against the deck's word count.
func TestCreateDeckExpandsAmbiguousTokenIntoMultipleCards(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("joon", "NOUN", "line", "ET"); err != nil {
		t.Fatalf("UpsertLemma joon: %v", err)
	}
	if err := api.store.UpsertLemma("jooma", "VERB", "drink", "ET"); err != nil {
		t.Fatalf("UpsertLemma jooma: %v", err)
	}
	for _, r := range [][4]string{
		{"joon", "joon", "NOUN", "ET"},
		{"joon", "jooma", "VERB", "ET"},
	} {
		if err := api.store.UpsertForm(r[0], r[1], r[2], r[3]); err != nil {
			t.Fatalf("UpsertForm %v: %v", r, err)
		}
	}

	// Parser picks ONE lemma for the ambiguous "joon" — which one doesn't
	// matter: the deck-ingest layer re-queries the dict and emits all
	// candidates regardless of the parser's pick.
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{
				{
					Text: "Ma joon vett.",
					Tokens: []parsecore.TokenResult{
						{Form: "Ma", Lemma: "mina", POS: "PRON"},
						{Form: "joon", Lemma: "jooma", POS: "VERB"},
						{Form: "vett", Lemma: "vesi", POS: "NOUN"},
						{Form: ".", POS: "PUNCT"},
					},
				},
			},
		}, nil
	}

	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "joon@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Joon test","lang":"ET","text":"Ma joon vett."}`))
	for _, cookie := range cookies {
		createReq.AddCookie(cookie)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("create deck status=%d want %d body=%q", createRec.Code, http.StatusOK, createRec.Body.String())
	}

	auth, err := api.getCurrentUser(requestWithCookies(httptest.NewRequest(http.MethodGet, "/api/me", nil), cookies))
	if err != nil || auth == nil {
		t.Fatal("expected authenticated user")
	}

	// Cards: mina (PRON), joon (NOUN) and jooma (VERB) for the ambiguous
	// token, plus vesi (NOUN) — 4 total. PUNCT is dropped.
	cardCount, err := api.store.CountCards(auth.UserID, "ET")
	if err != nil {
		t.Fatalf("CountCards: %v", err)
	}
	if cardCount != 4 {
		t.Fatalf("card_count=%d want 4 (mina, joon-noun, jooma-verb, vesi)", cardCount)
	}

	// Deck stats: Unique counts distinct (lemma, pos) pairs across the deck's
	// occurrences. Both joon/NOUN and jooma/VERB should be present even
	// though only one surface "joon" appeared.
	stats, err := api.store.GetUserDeckStats(auth.UserID)
	if err != nil {
		t.Fatalf("GetUserDeckStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("got %d deck stats rows, want 1", len(stats))
	}
	if stats[0].Unique != 4 {
		t.Errorf("Unique=%d want 4 (mina, joon-noun, jooma-verb, vesi)", stats[0].Unique)
	}
}

func TestCreateDeckKeepsParserProtectedLemmaWhenRawDictHasTrap(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("tuskin", "ADV", "hardly", "FI"); err != nil {
		t.Fatalf("UpsertLemma tuskin/ADV: %v", err)
	}
	if err := api.store.UpsertLemma("tuska", "NOUN", "pain", "FI"); err != nil {
		t.Fatalf("UpsertLemma tuska/NOUN: %v", err)
	}
	if err := api.store.UpsertForm("tuskin", "tuska", "NOUN", "FI"); err != nil {
		t.Fatalf("UpsertForm tuskin->tuska: %v", err)
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang:        lang,
			Parser:      "custom",
			TotalTokens: 1,
			Sentences: []parsecore.SentenceResult{
				{
					Text: "Tuskin.",
					Tokens: []parsecore.TokenResult{
						{Form: "Tuskin", Lemma: "tuskin", POS: "ADV", Source: "lex-overlay", Resolved: true},
						{Form: ".", POS: "PUNCT"},
					},
				},
			},
			Words: []parsecore.WordEntry{
				{Lemma: "tuskin", POS: "ADV", Forms: []string{"Tuskin"}, Count: 1, Gloss: "hardly"},
			},
		}, nil
	}

	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "tuskin@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Tuskin","lang":"FI","text":"Tuskin."}`))
	for _, cookie := range cookies {
		createReq.AddCookie(cookie)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create deck status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	var createResp CreateDeckResponse
	if err := json.NewDecoder(bytes.NewReader(createRec.Body.Bytes())).Decode(&createResp); err != nil {
		t.Fatalf("decode create deck response: %v", err)
	}
	auth, err := api.getCurrentUser(requestWithCookies(httptest.NewRequest(http.MethodGet, "/api/me", nil), cookies))
	if err != nil || auth == nil {
		t.Fatal("expected authenticated user")
	}
	details, err := api.store.GetDeckDetails(auth.UserID, createResp.DeckID)
	if err != nil {
		t.Fatalf("GetDeckDetails: %v", err)
	}
	if len(details.Lemmas) != 1 {
		t.Fatalf("deck lemmas=%+v want only protected parser lemma", details.Lemmas)
	}
	if got := details.Lemmas[0]; got.Lemma != "tuskin" || got.POS != "ADV" || got.Gloss != "hardly" {
		t.Fatalf("deck lemma=%+v want tuskin/ADV", got)
	}
}

func TestCreateDeckSuppressesGlosslessAlternativeWhenSurfaceHasGlossedCandidate(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("liiga", "ADV", "too", "ET"); err != nil {
		t.Fatalf("UpsertLemma liiga/ADV: %v", err)
	}
	if err := api.store.UpsertLemma("liiga", "X", "", "ET"); err != nil {
		t.Fatalf("UpsertLemma liiga/X: %v", err)
	}
	for _, r := range [][4]string{
		{"liiga", "liiga", "ADV", "ET"},
		{"liiga", "liiga", "X", "ET"},
	} {
		if err := api.store.UpsertForm(r[0], r[1], r[2], r[3]); err != nil {
			t.Fatalf("UpsertForm %v: %v", r, err)
		}
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang:        lang,
			Parser:      "custom",
			TotalTokens: 1,
			Sentences: []parsecore.SentenceResult{
				{
					Text: "liiga",
					Tokens: []parsecore.TokenResult{
						{Form: "liiga", Lemma: "liiga", POS: "ADV", Resolved: true},
					},
				},
			},
			Words: []parsecore.WordEntry{
				{Lemma: "liiga", POS: "ADV", Forms: []string{"liiga"}, Count: 1, Gloss: "too"},
			},
		}, nil
	}

	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "liiga@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Liiga","lang":"ET","text":"liiga"}`))
	for _, cookie := range cookies {
		createReq.AddCookie(cookie)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create deck status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	auth, err := api.getCurrentUser(requestWithCookies(httptest.NewRequest(http.MethodGet, "/api/me", nil), cookies))
	if err != nil || auth == nil {
		t.Fatal("expected authenticated user")
	}
	cardCount, err := api.store.CountCards(auth.UserID, "ET")
	if err != nil {
		t.Fatalf("CountCards: %v", err)
	}
	if cardCount != 1 {
		t.Fatalf("card_count=%d want 1; glossless X alternative should not create a card", cardCount)
	}
}

func TestCreateDeckSuppressesInflectedFormGlossWhenSurfaceHasLexicalCandidate(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("olema", "VERB", "be", "ET"); err != nil {
		t.Fatalf("UpsertLemma olema/VERB: %v", err)
	}
	if err := api.store.UpsertLemma("olen", "VERB", "first-person singular present indicative of olema", "ET"); err != nil {
		t.Fatalf("UpsertLemma olen/VERB: %v", err)
	}
	for _, r := range [][4]string{
		{"olen", "olema", "VERB", "ET"},
		{"olen", "olen", "VERB", "ET"},
	} {
		if err := api.store.UpsertForm(r[0], r[1], r[2], r[3]); err != nil {
			t.Fatalf("UpsertForm %v: %v", r, err)
		}
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang:        lang,
			Parser:      "custom",
			TotalTokens: 1,
			Sentences: []parsecore.SentenceResult{
				{
					Text: "olen",
					Tokens: []parsecore.TokenResult{
						{Form: "olen", Lemma: "olema", POS: "VERB", Resolved: true},
					},
				},
			},
			Words: []parsecore.WordEntry{
				{Lemma: "olema", POS: "VERB", Forms: []string{"olen"}, Count: 1, Gloss: "be"},
			},
		}, nil
	}

	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "olen@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Olen","lang":"ET","text":"olen"}`))
	for _, cookie := range cookies {
		createReq.AddCookie(cookie)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create deck status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	auth, err := api.getCurrentUser(requestWithCookies(httptest.NewRequest(http.MethodGet, "/api/me", nil), cookies))
	if err != nil || auth == nil {
		t.Fatal("expected authenticated user")
	}
	cardCount, err := api.store.CountCards(auth.UserID, "ET")
	if err != nil {
		t.Fatalf("CountCards: %v", err)
	}
	if cardCount != 1 {
		t.Fatalf("card_count=%d want 1; inflected-form gloss alternative should not create a card", cardCount)
	}
}

// TestCreateDeckSilentDictKeepsParserPick verifies that when the dict has no
// candidates for a surface form, the parser's lemma is used (no expansion).
func TestCreateDeckSilentDictKeepsParserPick(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{
				{
					Text: "Kassakaaperdaja kõnnib.",
					Tokens: []parsecore.TokenResult{
						{Form: "Kassakaaperdaja", Lemma: "kassakaaperdaja", POS: "NOUN"},
						{Form: "kõnnib", Lemma: "kõndima", POS: "VERB"},
						{Form: ".", POS: "PUNCT"},
					},
				},
			},
		}, nil
	}

	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "silent@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Silent","lang":"ET","text":"Kassakaaperdaja kõnnib."}`))
	for _, cookie := range cookies {
		createReq.AddCookie(cookie)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create deck status=%d body=%q", createRec.Code, createRec.Body.String())
	}

	auth, err := api.getCurrentUser(requestWithCookies(httptest.NewRequest(http.MethodGet, "/api/me", nil), cookies))
	if err != nil || auth == nil {
		t.Fatal("expected authenticated user")
	}
	cardCount, err := api.store.CountCards(auth.UserID, "ET")
	if err != nil {
		t.Fatalf("CountCards: %v", err)
	}
	if cardCount != 2 {
		t.Errorf("card_count=%d want 2 (no dict expansion)", cardCount)
	}
}

func TestExpandTokenLemmasUsesDictCandidatesWhenPresent(t *testing.T) {
	dict := map[string][]store.FormResolution{
		"joon": {{Lemma: "joon", POS: "NOUN", Source: "dict"}, {Lemma: "jooma", POS: "VERB", Source: "dict"}},
	}
	got := expandTokenLemmas(parsecore.TokenResult{Form: "joon", Lemma: "jooma", POS: "VERB"}, dict)
	if len(got) != 2 {
		t.Fatalf("len(got)=%d want 2: %+v", len(got), got)
	}
	want := map[tokenLemma]bool{{Lemma: "joon", POS: "NOUN"}: true, {Lemma: "jooma", POS: "VERB"}: true}
	for _, tl := range got {
		if !want[tl] {
			t.Errorf("unexpected expansion %+v", tl)
		}
		delete(want, tl)
	}
	if len(want) != 0 {
		t.Errorf("missing expansions: %+v", want)
	}
}

func TestExpandTokenLemmasKeepsParserPickWhenDictDoesNotContainIt(t *testing.T) {
	dict := map[string][]store.FormResolution{
		"olen": {{Lemma: "olen", POS: "VERB", Source: "dict"}},
	}

	got := expandTokenLemmas(parsecore.TokenResult{
		Form:   "olen",
		Lemma:  "olema",
		POS:    "VERB",
		Source: "fst",
	}, dict)

	if len(got) != 1 || got[0] != (tokenLemma{Lemma: "olema", POS: "VERB"}) {
		t.Fatalf("got %+v, want parser-selected olema/VERB instead of raw dict candidate", got)
	}
}

func TestExpandTokenLemmasKeepsParserPickWhenLowValueFilterRemovedIt(t *testing.T) {
	rawDict := map[string][]store.FormResolution{
		"olen": {
			{Lemma: "olema", POS: "VERB", Source: "dict"},
			{Lemma: "olen", POS: "VERB", Source: "dict"},
		},
	}
	glosses := map[store.LemmaKey]string{
		{Lemma: "olema", POS: "VERB"}: "be",
		{Lemma: "olen", POS: "VERB"}:  "first-person singular present indicative of olema",
	}
	filtered := filterLowValueAlternatives(rawDict, glosses)

	got := expandTokenLemmas(parsecore.TokenResult{
		Form:  "olen",
		Lemma: "olen",
		POS:   "VERB",
	}, filtered)

	if len(got) != 1 || got[0] != (tokenLemma{Lemma: "olen", POS: "VERB"}) {
		t.Fatalf("got %+v, want parser-selected olen/VERB after low-value filter removed it", got)
	}
}

func TestExpandTokenLemmasKeepsLexOverlayPickEvenIfDictContainsIt(t *testing.T) {
	dict := map[string][]store.FormResolution{
		"varsin": {
			{Lemma: "varsin", POS: "ADV", Source: "dict"},
			{Lemma: "varsi", POS: "NOUN", Source: "dict"},
		},
	}

	got := expandTokenLemmas(parsecore.TokenResult{
		Form:   "varsin",
		Lemma:  "varsin",
		POS:    "ADV",
		Source: "lex-overlay",
	}, dict)

	if len(got) != 1 || got[0] != (tokenLemma{Lemma: "varsin", POS: "ADV"}) {
		t.Fatalf("got %+v, want overlay-selected varsin/ADV only", got)
	}
}

func TestExpandTokenLemmasFallsBackToParserPickWhenDictSilent(t *testing.T) {
	got := expandTokenLemmas(
		parsecore.TokenResult{Form: "kassakaaperdaja", Lemma: "kassakaaperdaja", POS: "NOUN"},
		map[string][]store.FormResolution{},
	)
	if len(got) != 1 || got[0] != (tokenLemma{Lemma: "kassakaaperdaja", POS: "NOUN"}) {
		t.Errorf("got %+v, want single parser pick", got)
	}
}

func TestExpandTokenLemmasDropsPunctAndEmptyLemmaTokens(t *testing.T) {
	if got := expandTokenLemmas(parsecore.TokenResult{Form: "?", POS: "PUNCT"}, nil); len(got) != 0 {
		t.Errorf("PUNCT: got %+v, want empty", got)
	}
	if got := expandTokenLemmas(parsecore.TokenResult{Form: "", Lemma: "", POS: "NOUN"}, nil); len(got) != 0 {
		t.Errorf("empty form: got %+v, want empty", got)
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

func TestGetDeckDetail(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.UpsertLemma("kissa", "NOUN", "cat", "FI"); err != nil {
		t.Fatalf("UpsertLemma kissa: %v", err)
	}
	if err := api.store.UpsertLemma("juosta", "VERB", "to run", "FI"); err != nil {
		t.Fatalf("UpsertLemma juosta: %v", err)
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
					},
				},
				{
					Text: "Kissa nukkuu.",
					Tokens: []parsecore.TokenResult{
						{Form: "Kissa", Lemma: "kissa", POS: "NOUN"},
						{Form: "nukkuu", Lemma: "nukkua", POS: "VERB"},
					},
				},
			},
		}, nil
	}
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "deck-detail@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Detail deck","lang":"FI","text":"Kissa juoksee. Kissa nukkuu."}`))
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

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/decks/%d", created.DeckID), nil)
	for _, cookie := range cookies {
		getReq.AddCookie(cookie)
	}
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status=%d want %d body=%q", getRec.Code, http.StatusOK, getRec.Body.String())
	}

	var detail DeckDetailResponse
	if err := json.NewDecoder(bytes.NewReader(getRec.Body.Bytes())).Decode(&detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detail.ID != created.DeckID {
		t.Errorf("ID=%d want %d", detail.ID, created.DeckID)
	}
	if detail.Title != "Detail deck" {
		t.Errorf("Title=%q want %q", detail.Title, "Detail deck")
	}
	if detail.Lang != "FI" {
		t.Errorf("Lang=%q want FI", detail.Lang)
	}
	if detail.Parser != "custom" {
		t.Errorf("Parser=%q want custom", detail.Parser)
	}
	if detail.ParseID == nil || *detail.ParseID <= 0 {
		t.Errorf("ParseID=%v want non-nil positive", detail.ParseID)
	}
	if detail.TotalTokens != 4 {
		t.Errorf("TotalTokens=%d want 4 (2 sentences × 2 tokens)", detail.TotalTokens)
	}
	wordsByLemma := map[string]WordEntry{}
	for _, w := range detail.Words {
		wordsByLemma[w.Lemma] = w
	}
	kissa, ok := wordsByLemma["kissa"]
	if !ok {
		t.Fatalf("kissa missing from words: %+v", detail.Words)
	}
	if kissa.Count != 2 {
		t.Errorf("kissa count=%d want 2", kissa.Count)
	}
	if kissa.Gloss != "cat" {
		t.Errorf("kissa gloss=%q want cat", kissa.Gloss)
	}
	if got := strings.Join(kissa.Forms, ","); got != "Kissa" {
		t.Errorf("kissa forms=%v want [Kissa]", kissa.Forms)
	}
	if kissa.ExampleSentence == "" {
		t.Errorf("kissa example sentence empty")
	}
	juosta, ok := wordsByLemma["juosta"]
	if !ok {
		t.Errorf("juosta missing from words: %+v", detail.Words)
	} else if got := strings.Join(juosta.Forms, ","); got != "juoksee" {
		t.Errorf("juosta forms=%v want [juoksee]", juosta.Forms)
	}

	// 404 for someone else's deck.
	otherCookies := loginAndReturnCookies(t, mux, "other-user@example.com")
	otherReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/decks/%d", created.DeckID), nil)
	for _, cookie := range otherCookies {
		otherReq.AddCookie(cookie)
	}
	otherRec := httptest.NewRecorder()
	mux.ServeHTTP(otherRec, otherReq)
	if otherRec.Code != http.StatusNotFound {
		t.Errorf("other-user GET status=%d want 404 body=%q", otherRec.Code, otherRec.Body.String())
	}
}

func TestPublicDeckCreationRequiresAdmin(t *testing.T) {
	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{{
				Text: "Kissa juoksee.",
				Tokens: []parsecore.TokenResult{
					{Form: "Kissa", Lemma: "kissa", POS: "NOUN"},
				},
			}},
		}, nil
	}
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "regular-user@example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Sneaky","lang":"FI","text":"Kissa juoksee.","is_public":true}`))
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestPublicDeckLifecycleAdminCreateAndSubscribe(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")

	api := newTestAPI(t)
	if err := api.store.UpsertLemma("kissa", "NOUN", "cat", "FI"); err != nil {
		t.Fatalf("UpsertLemma: %v", err)
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{{
				Text: "Kissa juoksee.",
				Tokens: []parsecore.TokenResult{
					{Form: "Kissa", Lemma: "kissa", POS: "NOUN"},
				},
			}},
		}, nil
	}
	mux := newTestMux(t, api)

	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")
	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Beginner Finnish","lang":"FI","text":"Kissa juoksee.","is_public":true}`))
	for _, cookie := range adminCookies {
		createReq.AddCookie(cookie)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status=%d want %d body=%q", createRec.Code, http.StatusOK, createRec.Body.String())
	}
	var created CreateDeckResponse
	if err := json.NewDecoder(bytes.NewReader(createRec.Body.Bytes())).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	userCookies := loginAndReturnCookies(t, mux, "learner@example.com")

	// Learner should see the official deck in the catalog, unsubscribed.
	catalogReq := httptest.NewRequest(http.MethodGet, "/api/decks/public", nil)
	for _, cookie := range userCookies {
		catalogReq.AddCookie(cookie)
	}
	catalogRec := httptest.NewRecorder()
	mux.ServeHTTP(catalogRec, catalogReq)
	if catalogRec.Code != http.StatusOK {
		t.Fatalf("catalog status=%d body=%q", catalogRec.Code, catalogRec.Body.String())
	}
	var catalog PublicDeckListResponse
	if err := json.NewDecoder(bytes.NewReader(catalogRec.Body.Bytes())).Decode(&catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(catalog.Decks) != 1 || catalog.Decks[0].ID != created.DeckID {
		t.Fatalf("catalog=%+v want one deck id=%d", catalog.Decks, created.DeckID)
	}
	if catalog.Decks[0].Subscribed {
		t.Fatal("expected unsubscribed by default")
	}
	if !catalog.Decks[0].IsPublic {
		t.Fatal("expected is_public=true")
	}
	if catalog.Decks[0].IsOwner {
		t.Fatal("learner should not be marked as owner of admin's deck")
	}

	// Admin (the owner) also sees their own deck in the catalog so they can
	// verify what other users will see — but marked as is_owner so the UI
	// can suppress the subscribe button.
	adminCatalogReq := httptest.NewRequest(http.MethodGet, "/api/decks/public", nil)
	for _, cookie := range adminCookies {
		adminCatalogReq.AddCookie(cookie)
	}
	adminCatalogRec := httptest.NewRecorder()
	mux.ServeHTTP(adminCatalogRec, adminCatalogReq)
	var adminCatalog PublicDeckListResponse
	if err := json.NewDecoder(bytes.NewReader(adminCatalogRec.Body.Bytes())).Decode(&adminCatalog); err != nil {
		t.Fatalf("decode admin catalog: %v", err)
	}
	if len(adminCatalog.Decks) != 1 || adminCatalog.Decks[0].ID != created.DeckID {
		t.Fatalf("admin catalog=%+v want one deck id=%d", adminCatalog.Decks, created.DeckID)
	}
	if !adminCatalog.Decks[0].IsOwner {
		t.Fatal("expected is_owner=true for the admin viewing their own publication")
	}

	// Read-only GET of the public deck succeeds for the learner.
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/decks/%d", created.DeckID), nil)
	for _, cookie := range userCookies {
		getReq.AddCookie(cookie)
	}
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("learner GET status=%d body=%q", getRec.Code, getRec.Body.String())
	}
	var detail DeckDetailResponse
	if err := json.NewDecoder(bytes.NewReader(getRec.Body.Bytes())).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if !detail.IsPublic || detail.IsOwner || detail.Subscribed {
		t.Fatalf("detail flags wrong: %+v", detail)
	}

	// Rename + delete are owner-only; learner gets 404 for both.
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		var body string
		if method == http.MethodPatch {
			body = `{"title":"hijack"}`
		}
		req := httptest.NewRequest(method, fmt.Sprintf("/api/decks/%d", created.DeckID), strings.NewReader(body))
		for _, cookie := range userCookies {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s as learner status=%d want 404 body=%q", method, rec.Code, rec.Body.String())
		}
	}

	// Subscribe: seeds cards for the user.
	subReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/decks/%d/subscribe", created.DeckID), nil)
	for _, cookie := range userCookies {
		subReq.AddCookie(cookie)
	}
	subRec := httptest.NewRecorder()
	mux.ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusOK {
		t.Fatalf("subscribe status=%d body=%q", subRec.Code, subRec.Body.String())
	}

	// Catalog now shows subscribed=true.
	catalog2Rec := httptest.NewRecorder()
	catalog2Req := httptest.NewRequest(http.MethodGet, "/api/decks/public", nil)
	for _, cookie := range userCookies {
		catalog2Req.AddCookie(cookie)
	}
	mux.ServeHTTP(catalog2Rec, catalog2Req)
	var catalog2 PublicDeckListResponse
	if err := json.NewDecoder(bytes.NewReader(catalog2Rec.Body.Bytes())).Decode(&catalog2); err != nil {
		t.Fatalf("decode catalog2: %v", err)
	}
	if len(catalog2.Decks) != 1 || !catalog2.Decks[0].Subscribed {
		t.Fatalf("catalog2 not subscribed: %+v", catalog2.Decks)
	}

	// Subscribed deck appears in the learner's "/api/decks" listing too.
	myReq := httptest.NewRequest(http.MethodGet, "/api/decks", nil)
	for _, cookie := range userCookies {
		myReq.AddCookie(cookie)
	}
	myRec := httptest.NewRecorder()
	mux.ServeHTTP(myRec, myReq)
	var myList DeckListResponse
	if err := json.NewDecoder(bytes.NewReader(myRec.Body.Bytes())).Decode(&myList); err != nil {
		t.Fatalf("decode my list: %v", err)
	}
	if len(myList.Decks) != 1 || !myList.Decks[0].Subscribed || !myList.Decks[0].IsPublic {
		t.Fatalf("my list: %+v", myList.Decks)
	}

	// Review queue surfaces the seeded card.
	reviewReq := httptest.NewRequest(http.MethodGet, "/api/review/next", nil)
	for _, cookie := range userCookies {
		reviewReq.AddCookie(cookie)
	}
	reviewRec := httptest.NewRecorder()
	mux.ServeHTTP(reviewRec, reviewReq)
	if reviewRec.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%q", reviewRec.Code, reviewRec.Body.String())
	}

	// Unsubscribe.
	unsubReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/decks/%d/subscribe", created.DeckID), nil)
	for _, cookie := range userCookies {
		unsubReq.AddCookie(cookie)
	}
	unsubRec := httptest.NewRecorder()
	mux.ServeHTTP(unsubRec, unsubReq)
	if unsubRec.Code != http.StatusOK {
		t.Fatalf("unsubscribe status=%d body=%q", unsubRec.Code, unsubRec.Body.String())
	}

	// Subscription removed from listing.
	myFinalReq := httptest.NewRequest(http.MethodGet, "/api/decks", nil)
	for _, cookie := range userCookies {
		myFinalReq.AddCookie(cookie)
	}
	myFinalRec := httptest.NewRecorder()
	mux.ServeHTTP(myFinalRec, myFinalReq)
	var myFinal DeckListResponse
	if err := json.NewDecoder(bytes.NewReader(myFinalRec.Body.Bytes())).Decode(&myFinal); err != nil {
		t.Fatalf("decode my final list: %v", err)
	}
	if len(myFinal.Decks) != 0 {
		t.Fatalf("expected empty deck list after unsubscribe, got %+v", myFinal.Decks)
	}
}

// TestUnpublishGrandfathersExistingSubscribers verifies that when an admin
// unpublishes an official deck, learners who had already added it to their
// studying list keep working access. Without grandfathering, GetUserDeckStats
// still listed the deck (via the subscription join) while GetDeckDetails
// 404'd on click — a "ghost row" bug. See review item #1.
func TestUnpublishGrandfathersExistingSubscribers(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")

	api := newTestAPI(t)
	if err := api.store.UpsertLemma("kissa", "NOUN", "cat", "FI"); err != nil {
		t.Fatalf("UpsertLemma: %v", err)
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{{
				Text: "Kissa juoksee.",
				Tokens: []parsecore.TokenResult{
					{Form: "Kissa", Lemma: "kissa", POS: "NOUN"},
				},
			}},
		}, nil
	}
	mux := newTestMux(t, api)

	// Admin creates a public deck.
	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")
	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Beginner","lang":"FI","text":"Kissa juoksee.","is_public":true}`))
	for _, c := range adminCookies {
		createReq.AddCookie(c)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var created CreateDeckResponse
	if err := json.NewDecoder(bytes.NewReader(createRec.Body.Bytes())).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Learner subscribes.
	learnerCookies := loginAndReturnCookies(t, mux, "grandfathered@example.com")
	subReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/decks/%d/subscribe", created.DeckID), nil)
	for _, c := range learnerCookies {
		subReq.AddCookie(c)
	}
	subRec := httptest.NewRecorder()
	mux.ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusOK {
		t.Fatalf("subscribe status=%d body=%q", subRec.Code, subRec.Body.String())
	}

	// Admin unpublishes.
	unpubReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/decks/%d", created.DeckID), strings.NewReader(`{"is_public":false}`))
	for _, c := range adminCookies {
		unpubReq.AddCookie(c)
	}
	unpubRec := httptest.NewRecorder()
	mux.ServeHTTP(unpubRec, unpubReq)
	if unpubRec.Code != http.StatusOK {
		t.Fatalf("unpublish status=%d body=%q", unpubRec.Code, unpubRec.Body.String())
	}

	// Catalog now hides the deck from new visitors (correct).
	newcomerCookies := loginAndReturnCookies(t, mux, "newcomer@example.com")
	catReq := httptest.NewRequest(http.MethodGet, "/api/decks/public", nil)
	for _, c := range newcomerCookies {
		catReq.AddCookie(c)
	}
	catRec := httptest.NewRecorder()
	mux.ServeHTTP(catRec, catReq)
	var catalog PublicDeckListResponse
	if err := json.NewDecoder(bytes.NewReader(catRec.Body.Bytes())).Decode(&catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(catalog.Decks) != 0 {
		t.Fatalf("expected catalog empty after unpublish, got %+v", catalog.Decks)
	}

	// Grandfathered learner still sees the deck in their listing AND can
	// open the detail page (used to 404 — listing/detail disagreed).
	listReq := httptest.NewRequest(http.MethodGet, "/api/decks", nil)
	for _, c := range learnerCookies {
		listReq.AddCookie(c)
	}
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	var list DeckListResponse
	if err := json.NewDecoder(bytes.NewReader(listRec.Body.Bytes())).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Decks) != 1 || list.Decks[0].ID != created.DeckID {
		t.Fatalf("expected grandfathered deck in learner listing, got %+v", list.Decks)
	}

	detailReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/decks/%d", created.DeckID), nil)
	for _, c := range learnerCookies {
		detailReq.AddCookie(c)
	}
	detailRec := httptest.NewRecorder()
	mux.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("grandfathered detail status=%d want 200 body=%q", detailRec.Code, detailRec.Body.String())
	}
	var detail DeckDetailResponse
	if err := json.NewDecoder(bytes.NewReader(detailRec.Body.Bytes())).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.IsPublic {
		t.Fatal("deck should report is_public=false after unpublish")
	}

	// A user who never subscribed must still get 404 on the now-private deck.
	stranger404Req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/decks/%d", created.DeckID), nil)
	for _, c := range newcomerCookies {
		stranger404Req.AddCookie(c)
	}
	stranger404Rec := httptest.NewRecorder()
	mux.ServeHTTP(stranger404Rec, stranger404Req)
	if stranger404Rec.Code != http.StatusNotFound {
		t.Fatalf("non-subscriber detail status=%d want 404 body=%q", stranger404Rec.Code, stranger404Rec.Body.String())
	}
}

// TestDeckPatchNonOwnerWithTitleAndIsPublic verifies that an admin
// PATCHing both title and is_public on a deck they don't own is rejected
// BEFORE either mutation runs. The previous handler applied is_public
// first and then 404'd on the title update, leaving the visibility
// flipped without telling the caller. See review item #2.
func TestDeckPatchNonOwnerWithTitleAndIsPublic(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")

	api := newTestAPI(t)
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{{
				Text: "Kissa juoksee.",
				Tokens: []parsecore.TokenResult{
					{Form: "Kissa", Lemma: "kissa", POS: "NOUN"},
				},
			}},
		}, nil
	}
	mux := newTestMux(t, api)

	// User creates a private deck.
	userCookies := loginAndReturnCookies(t, mux, "owner@example.com")
	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"Private","lang":"FI","text":"Kissa juoksee."}`))
	for _, c := range userCookies {
		createReq.AddCookie(c)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var created CreateDeckResponse
	if err := json.NewDecoder(bytes.NewReader(createRec.Body.Bytes())).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Admin (not the owner) PATCHes both fields. The title update would
	// fail the owner check, so the whole PATCH must abort before is_public
	// commits.
	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")
	body := fmt.Sprintf(`{"title":"Hijacked","is_public":true}`)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/decks/%d", created.DeckID), strings.NewReader(body))
	for _, c := range adminCookies {
		patchReq.AddCookie(c)
	}
	patchRec := httptest.NewRecorder()
	mux.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusForbidden {
		t.Fatalf("PATCH status=%d want 403 body=%q", patchRec.Code, patchRec.Body.String())
	}

	// Verify is_public did NOT change, AND the title did NOT change.
	detailReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/decks/%d", created.DeckID), nil)
	for _, c := range userCookies {
		detailReq.AddCookie(c)
	}
	detailRec := httptest.NewRecorder()
	mux.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("owner detail status=%d body=%q", detailRec.Code, detailRec.Body.String())
	}
	var detail DeckDetailResponse
	if err := json.NewDecoder(bytes.NewReader(detailRec.Body.Bytes())).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.IsPublic {
		t.Fatal("is_public must not have flipped when title update was disallowed")
	}
	if detail.Title != "Private" {
		t.Fatalf("title=%q want 'Private'", detail.Title)
	}

	// Admin can still patch just is_public on the non-owned deck — the
	// pre-check only blocks combined PATCHes that include a title.
	patchPublicReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/decks/%d", created.DeckID), strings.NewReader(`{"is_public":true}`))
	for _, c := range adminCookies {
		patchPublicReq.AddCookie(c)
	}
	patchPublicRec := httptest.NewRecorder()
	mux.ServeHTTP(patchPublicRec, patchPublicReq)
	if patchPublicRec.Code != http.StatusOK {
		t.Fatalf("is_public-only PATCH status=%d body=%q", patchPublicRec.Code, patchPublicRec.Body.String())
	}
}

func TestDeckPatchTogglesIsPublicAdminOnly(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")

	api := newTestAPI(t)
	if err := api.store.UpsertLemma("kissa", "NOUN", "cat", "FI"); err != nil {
		t.Fatalf("UpsertLemma: %v", err)
	}
	api.analyze = func(_ *store.DB, lang, text, parser string) (*parsecore.ParseResult, error) {
		return &parsecore.ParseResult{
			Lang: lang,
			Sentences: []parsecore.SentenceResult{{
				Text: "Kissa juoksee.",
				Tokens: []parsecore.TokenResult{
					{Form: "Kissa", Lemma: "kissa", POS: "NOUN"},
				},
			}},
		}, nil
	}
	mux := newTestMux(t, api)

	// A regular user creates a private deck.
	userCookies := loginAndReturnCookies(t, mux, "owner@example.com")
	createReq := httptest.NewRequest(http.MethodPost, "/api/decks", strings.NewReader(`{"title":"My private","lang":"FI","text":"Kissa juoksee."}`))
	for _, cookie := range userCookies {
		createReq.AddCookie(cookie)
	}
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%q", createRec.Code, createRec.Body.String())
	}
	var created CreateDeckResponse
	if err := json.NewDecoder(bytes.NewReader(createRec.Body.Bytes())).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Non-admin owner tries to publish their own deck → 403.
	patchByOwnerReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/decks/%d", created.DeckID), strings.NewReader(`{"is_public":true}`))
	for _, cookie := range userCookies {
		patchByOwnerReq.AddCookie(cookie)
	}
	patchByOwnerRec := httptest.NewRecorder()
	mux.ServeHTTP(patchByOwnerRec, patchByOwnerReq)
	if patchByOwnerRec.Code != http.StatusForbidden {
		t.Fatalf("non-admin patch status=%d want 403 body=%q", patchByOwnerRec.Code, patchByOwnerRec.Body.String())
	}

	// Confirm the deck is still private.
	verifyReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/decks/%d", created.DeckID), nil)
	for _, cookie := range userCookies {
		verifyReq.AddCookie(cookie)
	}
	verifyRec := httptest.NewRecorder()
	mux.ServeHTTP(verifyRec, verifyReq)
	var detail DeckDetailResponse
	if err := json.NewDecoder(bytes.NewReader(verifyRec.Body.Bytes())).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.IsPublic {
		t.Fatal("deck is_public should still be false after 403")
	}

	// Admin publishes the deck (admin doesn't own it — that's fine).
	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")
	patchByAdminReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/decks/%d", created.DeckID), strings.NewReader(`{"is_public":true}`))
	for _, cookie := range adminCookies {
		patchByAdminReq.AddCookie(cookie)
	}
	patchByAdminRec := httptest.NewRecorder()
	mux.ServeHTTP(patchByAdminRec, patchByAdminReq)
	if patchByAdminRec.Code != http.StatusOK {
		t.Fatalf("admin publish status=%d body=%q", patchByAdminRec.Code, patchByAdminRec.Body.String())
	}

	// Deck now appears in the public catalog for a different learner.
	learnerCookies := loginAndReturnCookies(t, mux, "learner-toggle@example.com")
	catReq := httptest.NewRequest(http.MethodGet, "/api/decks/public", nil)
	for _, cookie := range learnerCookies {
		catReq.AddCookie(cookie)
	}
	catRec := httptest.NewRecorder()
	mux.ServeHTTP(catRec, catReq)
	var catalog PublicDeckListResponse
	if err := json.NewDecoder(bytes.NewReader(catRec.Body.Bytes())).Decode(&catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(catalog.Decks) != 1 || catalog.Decks[0].ID != created.DeckID || !catalog.Decks[0].IsPublic {
		t.Fatalf("catalog after publish: %+v", catalog.Decks)
	}

	// Admin unpublishes. Catalog goes back to empty for new learners.
	unpublishReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/decks/%d", created.DeckID), strings.NewReader(`{"is_public":false}`))
	for _, cookie := range adminCookies {
		unpublishReq.AddCookie(cookie)
	}
	unpublishRec := httptest.NewRecorder()
	mux.ServeHTTP(unpublishRec, unpublishReq)
	if unpublishRec.Code != http.StatusOK {
		t.Fatalf("admin unpublish status=%d body=%q", unpublishRec.Code, unpublishRec.Body.String())
	}
	catAfterReq := httptest.NewRequest(http.MethodGet, "/api/decks/public", nil)
	for _, cookie := range learnerCookies {
		catAfterReq.AddCookie(cookie)
	}
	catAfterRec := httptest.NewRecorder()
	mux.ServeHTTP(catAfterRec, catAfterReq)
	var catalogAfter PublicDeckListResponse
	if err := json.NewDecoder(bytes.NewReader(catAfterRec.Body.Bytes())).Decode(&catalogAfter); err != nil {
		t.Fatalf("decode catalog after: %v", err)
	}
	if len(catalogAfter.Decks) != 0 {
		t.Fatalf("catalog after unpublish should be empty, got %+v", catalogAfter.Decks)
	}

	// Empty PATCH body (no title, no is_public) → 400.
	emptyReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/decks/%d", created.DeckID), strings.NewReader(`{}`))
	for _, cookie := range adminCookies {
		emptyReq.AddCookie(cookie)
	}
	emptyRec := httptest.NewRecorder()
	mux.ServeHTTP(emptyRec, emptyReq)
	if emptyRec.Code != http.StatusBadRequest {
		t.Fatalf("empty patch status=%d want 400 body=%q", emptyRec.Code, emptyRec.Body.String())
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

func TestParseFeedbackRejectsInlinePathWithoutSourceText(t *testing.T) {
	api := newTestAPI(t)
	mux := newTestMux(t, api)
	cookies := loginAndReturnCookies(t, mux, "inline-empty@example.com")

	// No parse_id AND no source_text → the handler can't create or attach
	// a session, so it 400s with a clear message.
	req := httptest.NewRequest(http.MethodPost, "/api/parse/feedback", strings.NewReader(`{
		"lang": "FI",
		"parser": "custom",
		"surface": "kissa",
		"proposed_lemma": "kissa",
		"proposed_pos": "NOUN"
	}`))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "source_text") {
		t.Fatalf("expected error mentioning source_text, got %q", rec.Body.String())
	}
}

func TestParseFeedbackLazilyCreatesParseSession(t *testing.T) {
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin-lazy@example.com")

	api := newTestAPI(t)
	mux := newTestMux(t, api)
	userCookies := loginAndReturnCookies(t, mux, "inline-lazy@example.com")
	adminCookies := loginAndReturnCookies(t, mux, "admin-lazy@example.com")

	body := `{
		"lang": "FI",
		"parser": "custom",
		"source_text": "kissa juoksee",
		"total_tokens": 2,
		"unique_lemma_count": 2,
		"surface": "kissa",
		"original_lemma": "kissa",
		"original_pos": "NOUN",
		"proposed_lemma": "kissa",
		"proposed_pos": "PROPN"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/parse/feedback", strings.NewReader(body))
	for _, c := range userCookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}

	// Pull the feedback row via admin listing; the lazily-created session
	// is referenced by parse_session_id. GetParseSession exposes its
	// source_text so we can verify the inline text landed.
	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/parse-feedback?status=submitted", nil)
	for _, c := range adminCookies {
		listReq.AddCookie(c)
	}
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("admin list status=%d body=%q", listRec.Code, listRec.Body.String())
	}
	var listResp ParseFeedbackListResponse
	if err := json.NewDecoder(bytes.NewReader(listRec.Body.Bytes())).Decode(&listResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listResp.Feedback) != 1 {
		t.Fatalf("feedback count = %d, want 1", len(listResp.Feedback))
	}
	sess, err := api.store.GetParseSession(listResp.Feedback[0].ParseSessionID)
	if err != nil {
		t.Fatalf("GetParseSession: %v", err)
	}
	if sess.SourceText != "kissa juoksee" {
		t.Fatalf("lazy session source_text=%q, want %q", sess.SourceText, "kissa juoksee")
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
			ParseDurationNs: 11_000_000,
			Stats:           parsecore.ParseStats{},
			Words: []parsecore.WordEntry{
				{Lemma: "kissa", POS: "NOUN", Forms: []string{"kissa"}, Count: 1},
			},
		}, nil
	}
	mux := newTestMux(t, api)

	userCookies := loginAndReturnCookies(t, mux, "user@example.com")
	adminCookies := loginAndReturnCookies(t, mux, "admin@example.com")

	// /api/parse no longer persists, so the user submits feedback with the
	// inline source_text path. The handler lazily creates a parse_session
	// from it.
	feedbackReq := httptest.NewRequest(http.MethodPost, "/api/parse/feedback", strings.NewReader(`{
		"lang": "FI",
		"parser": "custom",
		"source_text": "kissa",
		"total_tokens": 2,
		"unique_lemma_count": 1,
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

	// Seed a parse_session belonging to the original user, then verify a
	// different authenticated user can't submit feedback referencing that
	// session by ID (legacy parse_id path is still ownership-checked).
	userOwner, err := api.store.GetUserByEmail("user@example.com")
	if err != nil || userOwner == nil {
		t.Fatalf("lookup owner: %v", err)
	}
	seededID, err := api.store.CreateParseSession(&userOwner.ID, "FI", "custom", "kissa", 2, 1)
	if err != nil {
		t.Fatalf("seed parse session: %v", err)
	}

	otherUserCookies := loginAndReturnCookies(t, mux, "other@example.com")
	forbiddenReq := httptest.NewRequest(http.MethodPost, "/api/parse/feedback", strings.NewReader(`{
		"parse_id": `+fmt.Sprintf("%d", seededID)+`,
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

// testPassword is the password every test user uses. Real auth requires a
// password ≥ 8 chars; tests register-then-keep cookies from the response.
const testPassword = "test-pass-123"

func loginAndReturnCookies(t *testing.T, mux *http.ServeMux, email string) []*http.Cookie {
	t.Helper()

	body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, testPassword)
	// Try register first; if the email already exists, fall back to login.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusConflict {
		req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("auth status=%d want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected auth to set cookies")
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
