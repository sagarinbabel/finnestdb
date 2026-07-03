// Package catalog is the curated Embedded Text catalog for signed-in cold
// start (CONTEXT.md "Embedded Text" / "Embedded Catalog"). It ships checked-in
// metadata (catalog.json) plus one checked-in plain-text fixture per text,
// both embedded into the server binary via go:embed so production carries no
// dependency on the corpus pipeline or on-disk corpus material.
//
// Metadata loads eagerly; full text is served lazily from the embedded
// fixture only when a learner selects a text. Each entry carries a precomputed
// (lemma, pos) list so per-learner known-token coverage is a cheap set
// intersection against user_known_lemmas at request time — no re-parse needed.
//
// The catalog is regenerated deterministically by cmd/gencatalog. The
// checked-in catalog.json must be exactly what the generator emits; do not
// hand-edit computed fields (difficulty, metrics, lemmas, counts).
package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed data/catalog.json
var catalogJSON []byte

//go:embed data/*.txt
var fixtureFS embed.FS

// Difficulty is the fixed Global Difficulty bucket (CONTEXT.md "Global
// Difficulty"): a text-level label computed by cmd/gencatalog from the metrics
// below. It is not a per-learner signal; Personalized Text Fit is layered on
// top at request time.
type Difficulty string

const (
	DifficultyEasy   Difficulty = "easy"
	DifficultyMedium Difficulty = "medium"
	DifficultyHard   Difficulty = "hard"
)

// Genre is the coarse content type used for the alpha catalog matrix.
type Genre string

const (
	GenreStory   Genre = "story"
	GenreArticle Genre = "article"
	GenrePoem    Genre = "poem"
)

// Metrics are the raw text-level difficulty inputs the generator computes from
// a real custom-mode parse. They are stored alongside the assigned bucket so a
// human sanity-check (Sagar for FI, an Estonian reviewer for ET) can see why a
// text landed where it did, and so the thresholds stay auditable.
type Metrics struct {
	// TokenCount is the number of non-punctuation tokens.
	TokenCount int `json:"token_count"`
	// SentenceCount is the number of sentences the parser segmented.
	SentenceCount int `json:"sentence_count"`
	// UniqueLemmaCount is the number of distinct (lemma, pos) entries.
	UniqueLemmaCount int `json:"unique_lemma_count"`
	// UniqueFormRatio is unique surface forms / total tokens (lexical variety;
	// higher means more distinct wordforms per token, i.e. harder).
	UniqueFormRatio float64 `json:"unique_form_ratio"`
	// UnresolvedRate is unresolved tokens / total tokens (dictionary/FST miss
	// rate; higher means more out-of-dictionary vocabulary, i.e. harder).
	UnresolvedRate float64 `json:"unresolved_rate"`
	// MeanFrequencyRank is the mean OpenSubtitles frequency rank of resolved
	// forms (lower rank = more common word; higher mean = rarer vocabulary,
	// i.e. harder). -1 when no frequency baseline was available at generation
	// time. RareFormRate is the share of tokens whose form is absent from or
	// beyond the frequency baseline.
	MeanFrequencyRank float64 `json:"mean_frequency_rank"`
	RareFormRate      float64 `json:"rare_form_rate"`
	// FeatsVariety is the number of distinct non-empty FEATS strings observed
	// (morphological richness; higher means more inflectional variety).
	FeatsVariety int `json:"feats_variety"`
	// MeanSentenceLength is tokens / sentences.
	MeanSentenceLength float64 `json:"mean_sentence_length"`
	// Score is the composite difficulty score in [0,1] the thresholds bucket.
	Score float64 `json:"score"`
}

// LemmaCount is one precomputed (lemma, pos) entry with the number of token
// occurrences in the text. The lemma set (ignoring counts) is what coverage
// intersects against; counts let the UI weight coverage by occurrence if it
// wants to later.
type LemmaCount struct {
	Lemma string `json:"lemma"`
	POS   string `json:"pos"`
	Count int    `json:"count"`
}

// Entry is one Embedded Text's checked-in metadata. Full text lives in the
// separate embedded fixture named by Fixture and is loaded lazily.
type Entry struct {
	ID       string `json:"id"`
	Language string `json:"language"` // "fi" or "et"
	Title    string `json:"title"`
	Author   string `json:"author,omitempty"`
	Genre    Genre  `json:"genre"`
	SourceURL string `json:"source_url,omitempty"`
	// CorpusSource names where the text came from (e.g. "project-gutenberg",
	// "finnest-original"). License is the plain redistribution basis; both are
	// license-discipline provenance, not decoration.
	CorpusSource string `json:"corpus_source"`
	License      string `json:"license"`
	Attribution  string `json:"attribution,omitempty"`
	// TextLength is the character length (runes) of the full fixture text.
	TextLength int    `json:"text_length"`
	WordCount  int    `json:"word_count"`
	ImportDate string `json:"import_date"` // YYYY-MM-DD

	Difficulty Difficulty `json:"difficulty"`
	Metrics    Metrics    `json:"metrics"`
	// DifficultyReview is "pending" until a human sanity-checks the bucket
	// (Sagar for FI, an Estonian reviewer for ET). Never emit "confirmed"
	// from the generator.
	DifficultyReview string `json:"difficulty_review"`

	// Fixture is the embedded plain-text filename under data/ (basename only).
	Fixture string `json:"fixture"`
	// Lemmas is the precomputed (lemma, pos) + count list for the text.
	Lemmas []LemmaCount `json:"lemmas"`
}

// Catalog is the parsed catalog.json document.
type Catalog struct {
	// Version is a monotonic schema/content version for the checked-in catalog.
	Version   int     `json:"version"`
	Generated string  `json:"generated"` // YYYY-MM-DD the catalog was last regenerated
	Entries   []Entry `json:"entries"`
}

// Load parses the embedded catalog.json. It is cheap enough to call per
// request, but callers that want a stable slice can call it once at startup.
func Load() (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(catalogJSON, &c); err != nil {
		return nil, fmt.Errorf("catalog: parse catalog.json: %w", err)
	}
	return &c, nil
}

// Entries returns the catalog entries, or an empty slice if the catalog fails
// to parse. Metadata-only: no fixture text is read.
func Entries() []Entry {
	c, err := Load()
	if err != nil {
		return nil
	}
	return c.Entries
}

// Find returns the entry with the given id, or false.
func Find(id string) (Entry, bool) {
	for _, e := range Entries() {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// Text lazily loads the full text of the given entry from its embedded
// fixture. Returns an error if the id is unknown or the fixture is missing.
func Text(id string) (string, error) {
	e, ok := Find(id)
	if !ok {
		return "", fmt.Errorf("catalog: unknown text id %q", id)
	}
	if e.Fixture == "" || strings.Contains(e.Fixture, "/") {
		return "", fmt.Errorf("catalog: entry %q has an invalid fixture reference", id)
	}
	b, err := fixtureFS.ReadFile("data/" + e.Fixture)
	if err != nil {
		return "", fmt.Errorf("catalog: read fixture for %q: %w", id, err)
	}
	return string(b), nil
}

// KnownLemma is the minimal (lemma, pos) shape used to compute coverage. It
// mirrors the fields of store.KnownLemma without importing the store package,
// keeping this package free of a database dependency.
type KnownLemma struct {
	Lemma string
	POS   string
}

// Coverage returns the fraction of the text's distinct (lemma, pos) entries
// that appear in the learner's known set, in [0,1], plus the matched and total
// distinct-lemma counts. Matching is case-insensitive on lemma and exact on
// POS, mirroring how known words are stored. A text with no lemmas returns 0.
func (e Entry) Coverage(known []KnownLemma) (fraction float64, matched, total int) {
	total = len(e.Lemmas)
	if total == 0 {
		return 0, 0, 0
	}
	knownSet := make(map[string]struct{}, len(known))
	for _, k := range known {
		knownSet[coverageKey(k.Lemma, k.POS)] = struct{}{}
	}
	for _, l := range e.Lemmas {
		if _, ok := knownSet[coverageKey(l.Lemma, l.POS)]; ok {
			matched++
		}
	}
	return float64(matched) / float64(total), matched, total
}

func coverageKey(lemma, pos string) string {
	return strings.ToLower(lemma) + "\x00" + pos
}

// SortEntries orders entries for stable presentation: by language, then genre,
// then difficulty (easy→hard), then title. Used by the generator and the API
// so the checked-in catalog and the served list agree.
func SortEntries(entries []Entry) {
	rank := map[Difficulty]int{DifficultyEasy: 0, DifficultyMedium: 1, DifficultyHard: 2}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Language != b.Language {
			return a.Language < b.Language
		}
		if a.Genre != b.Genre {
			return a.Genre < b.Genre
		}
		if rank[a.Difficulty] != rank[b.Difficulty] {
			return rank[a.Difficulty] < rank[b.Difficulty]
		}
		return a.Title < b.Title
	})
}
