package api

import (
	"net/http"
	"strings"

	"finnestdb/internal/catalog"
	"finnestdb/internal/store"
)

// Curated Embedded Text catalog endpoints (signed-in only), backing the
// dashboard/Inspect cold-start empty states (docs/USER_FLOWS.md §4, Decision
// 23/27). Metadata is served from the embedded catalog; full text is lazy.
//
//   GET /api/catalog            -> metadata list + per-learner coverage
//   GET /api/catalog/{id}/text  -> full text for one embedded text

// CatalogCoverage is the Personalized Text Fit overlay for one embedded text.
// It is present only when the learner has known-word data for that text's
// language; otherwise the client shows Global Difficulty and prompts import.
type CatalogCoverage struct {
	// KnownPct is the percentage (0–100, rounded) of the text's distinct
	// (lemma, pos) entries the learner already knows.
	KnownPct int `json:"known_pct"`
	// KnownLemmas / TotalLemmas back the raw fraction the UI can show as
	// "N of M words".
	KnownLemmas int `json:"known_lemmas"`
	TotalLemmas int `json:"total_lemmas"`
}

// CatalogEntryResponse is the metadata-only view of one embedded text sent in
// the list. The full (lemma, pos) list and fixture text are intentionally
// omitted; text is fetched lazily via /api/catalog/{id}/text.
type CatalogEntryResponse struct {
	ID          string `json:"id"`
	Language    string `json:"language"`
	Title       string `json:"title"`
	Author      string `json:"author,omitempty"`
	Genre       string `json:"genre"`
	Difficulty  string `json:"difficulty"`
	// DifficultyReview is "pending" until a human sanity-checks the bucket.
	// The client can surface this so an unreviewed label is not presented as
	// authoritative.
	DifficultyReview string `json:"difficulty_review"`
	SourceURL        string `json:"source_url,omitempty"`
	CorpusSource     string `json:"corpus_source"`
	License          string `json:"license"`
	Attribution      string `json:"attribution,omitempty"`
	TextLength       int    `json:"text_length"`
	WordCount        int    `json:"word_count"`
	// Coverage is nil when the learner has no known-word data for this text's
	// language; the client should prompt known-word import in that case.
	Coverage *CatalogCoverage `json:"coverage,omitempty"`
}

// CatalogResponse is the full /api/catalog payload.
type CatalogResponse struct {
	Entries []CatalogEntryResponse `json:"entries"`
	// HasKnownWords reports, per language code (e.g. "FI"), whether the
	// learner has any known words. The client uses it to decide between the
	// coverage overlay and the "import known words" prompt.
	HasKnownWords map[string]bool `json:"has_known_words"`
}

func (a *API) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	a.requireAuth(a.handleCatalog).ServeHTTP(w, r)
}

func (a *API) handleCatalog(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entries := catalog.Entries()

	// Load the learner's known lemmas once per language present in the
	// catalog, then compute coverage per entry as a cheap set intersection
	// against each entry's precomputed lemma list.
	knownByLang := map[string][]catalog.KnownLemma{}
	hasKnown := map[string]bool{}
	for _, e := range entries {
		lang := strings.ToUpper(e.Language)
		if _, done := knownByLang[lang]; done {
			continue
		}
		known, err := a.store.ListKnownWords(auth.UserID, lang)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		knownByLang[lang] = toCatalogKnown(known)
		hasKnown[lang] = len(known) > 0
	}

	resp := CatalogResponse{
		Entries:       make([]CatalogEntryResponse, 0, len(entries)),
		HasKnownWords: hasKnown,
	}
	for _, e := range entries {
		item := CatalogEntryResponse{
			ID:               e.ID,
			Language:         e.Language,
			Title:            e.Title,
			Author:           e.Author,
			Genre:            string(e.Genre),
			Difficulty:       string(e.Difficulty),
			DifficultyReview: e.DifficultyReview,
			SourceURL:        e.SourceURL,
			CorpusSource:     e.CorpusSource,
			License:          e.License,
			Attribution:      e.Attribution,
			TextLength:       e.TextLength,
			WordCount:        e.WordCount,
		}
		lang := strings.ToUpper(e.Language)
		if hasKnown[lang] {
			frac, matched, total := e.Coverage(knownByLang[lang])
			item.Coverage = &CatalogCoverage{
				KnownPct:    int(frac*100 + 0.5),
				KnownLemmas: matched,
				TotalLemmas: total,
			}
		}
		resp.Entries = append(resp.Entries, item)
	}

	writeJSON(w, http.StatusOK, resp)
}

// CatalogTextResponse is the lazy full-text payload.
type CatalogTextResponse struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	Title    string `json:"title"`
	Text     string `json:"text"`
}

func (a *API) HandleCatalogText(w http.ResponseWriter, r *http.Request) {
	a.requireAuth(a.handleCatalogText).ServeHTTP(w, r)
}

func (a *API) handleCatalogText(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Path: /api/catalog/{id}/text
	rest := strings.TrimPrefix(r.URL.Path, "/api/catalog/")
	id := strings.TrimSuffix(rest, "/text")
	if id == rest || id == "" || strings.Contains(id, "/") {
		http.Error(w, "Text not found", http.StatusNotFound)
		return
	}
	entry, ok := catalog.Find(id)
	if !ok {
		http.Error(w, "Text not found", http.StatusNotFound)
		return
	}
	text, err := catalog.Text(id)
	if err != nil {
		http.Error(w, "Text not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, CatalogTextResponse{
		ID:       entry.ID,
		Language: entry.Language,
		Title:    entry.Title,
		Text:     text,
	})
}

func toCatalogKnown(known []store.KnownLemma) []catalog.KnownLemma {
	out := make([]catalog.KnownLemma, 0, len(known))
	for _, k := range known {
		out = append(out, catalog.KnownLemma{Lemma: k.Lemma, POS: k.POS})
	}
	return out
}
