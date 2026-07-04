// cmd/gencatalog regenerates the checked-in Embedded Text catalog
// (internal/catalog/data/catalog.json) deterministically from a specs file
// plus the checked-in text fixtures.
//
// The specs file holds only human-authored provenance metadata (id, language,
// title, author, genre, source URL, corpus source, license, attribution,
// fixture filename, import date). Everything computed — difficulty, the raw
// difficulty metrics, the (lemma, pos) + count list, text length, word count —
// is derived here by parsing each fixture through the REAL custom-mode
// pipeline (internal/parsecore) and applying the deterministic difficulty
// model in internal/catalog. The checked-in catalog.json must be exactly what
// this tool emits; re-running it on an unchanged specs file + fixtures is a
// no-op.
//
// Usage:
//
//	go run ./cmd/gencatalog \
//	    -specs internal/catalog/specs.json \
//	    -data internal/catalog/data \
//	    -db finnestdb.db \
//	    -freq-dir localdata/frequency \
//	    -out internal/catalog/data/catalog.json
//
// -check verifies the catalog on disk matches a fresh regeneration and exits
// non-zero on drift (for CI / reproducibility guarding) without writing.
//
// Difficulty thresholds and the metric weights are documented in
// docs/GO_LIVE_CHECKLIST.md "Embedded catalog difficulty model" and pinned by
// internal/catalog/difficulty_test.go.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"finnestdb/internal/catalog"
	"finnestdb/internal/parsecore"
	"finnestdb/internal/store"
)

// spec is one human-authored catalog source entry. Computed fields are NOT in
// the spec — they are derived from the fixture at generation time.
type spec struct {
	ID           string `json:"id"`
	Language     string `json:"language"` // "fi" | "et"
	Title        string `json:"title"`
	Author       string `json:"author,omitempty"`
	Genre        string `json:"genre"` // story | article | poem
	SourceURL    string `json:"source_url,omitempty"`
	CorpusSource string `json:"corpus_source"`
	License      string `json:"license"`
	Attribution  string `json:"attribution,omitempty"`
	Fixture      string `json:"fixture"`
	ImportDate   string `json:"import_date"`
}

type specsDoc struct {
	Version int    `json:"version"`
	Specs   []spec `json:"specs"`
}

func main() {
	specsPath := flag.String("specs", "internal/catalog/specs.json", "Path to the catalog specs file")
	dataDir := flag.String("data", "internal/catalog/data", "Directory holding the text fixtures")
	dbPath := flag.String("db", "finnestdb.db", "Path to the SQLite dictionary database")
	freqDir := flag.String("freq-dir", "localdata/frequency", "Directory with OpenSubtitles frequency lists (optional)")
	out := flag.String("out", "internal/catalog/data/catalog.json", "Output catalog.json path")
	check := flag.Bool("check", false, "Verify on-disk catalog matches a fresh regeneration; do not write")
	reviewsPath := flag.String("reviews", "internal/catalog/reviews.json", "Path to the human difficulty-review file (optional)")
	flag.Parse()

	if err := run(*specsPath, *dataDir, *dbPath, *freqDir, *out, *reviewsPath, *check); err != nil {
		fmt.Fprintf(os.Stderr, "gencatalog: %v\n", err)
		os.Exit(1)
	}
}

func run(specsPath, dataDir, dbPath, freqDir, out, reviewsPath string, check bool) error {
	specs, version, err := loadSpecs(specsPath)
	if err != nil {
		return err
	}

	db, err := store.NewDB(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Load frequency baselines per language once. Missing files are fine —
	// difficulty falls back to the no-baseline path and logs it.
	freqByLang := map[string]map[string]int{}
	for _, lang := range []string{"fi", "et"} {
		path := filepath.Join(freqDir, lang, fmt.Sprintf("opensubtitles-2018-%s-50k.txt", lang))
		ranks, ferr := frequencyRanks(path)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "gencatalog: no %s frequency baseline (%v); difficulty will omit the frequency signal for %s texts\n", lang, ferr, lang)
			continue
		}
		freqByLang[lang] = ranks
	}

	reviews, err := loadReviews(reviewsPath)
	if err != nil {
		return fmt.Errorf("reviews %q: %w", reviewsPath, err)
	}

	entries := make([]catalog.Entry, 0, len(specs))
	for _, s := range specs {
		entry, err := buildEntry(db, dataDir, freqByLang, s)
		if err != nil {
			return fmt.Errorf("entry %q: %w", s.ID, err)
		}
		if r, ok := reviews[s.ID]; ok {
			if err := applyReview(&entry, r); err != nil {
				return fmt.Errorf("entry %q: %w", s.ID, err)
			}
		}
		entries = append(entries, entry)
	}
	for id := range reviews {
		found := false
		for _, e := range entries {
			if e.ID == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("reviews %q: review for unknown entry %q", reviewsPath, id)
		}
	}
	catalog.SortEntries(entries)

	doc := catalog.Catalog{
		Version:   version,
		Generated: time.Now().UTC().Format("2006-01-02"),
		Entries:   entries,
	}
	rendered, err := renderCatalog(doc)
	if err != nil {
		return err
	}

	if check {
		existing, err := os.ReadFile(out)
		if err != nil {
			return fmt.Errorf("read existing catalog for -check: %w", err)
		}
		// Ignore the "generated" date on -check: it is a timestamp, not
		// content. Compare everything else.
		if !catalogContentEqual(existing, rendered) {
			return fmt.Errorf("catalog on disk differs from a fresh regeneration; run gencatalog without -check")
		}
		fmt.Println("gencatalog: catalog is up to date")
		return nil
	}

	if err := os.WriteFile(out, rendered, 0o644); err != nil {
		return fmt.Errorf("write catalog: %w", err)
	}
	fmt.Printf("gencatalog: wrote %d entries to %s\n", len(entries), out)
	return nil
}

func loadSpecs(path string) ([]spec, int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read specs: %w", err)
	}
	var doc specsDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, 0, fmt.Errorf("parse specs: %w", err)
	}
	if len(doc.Specs) == 0 {
		return nil, 0, fmt.Errorf("specs file has no entries")
	}
	seen := map[string]bool{}
	for _, s := range doc.Specs {
		if s.ID == "" || s.Fixture == "" || s.Language == "" || s.Genre == "" {
			return nil, 0, fmt.Errorf("spec missing required field (id/fixture/language/genre): %+v", s)
		}
		if seen[s.ID] {
			return nil, 0, fmt.Errorf("duplicate spec id %q", s.ID)
		}
		seen[s.ID] = true
	}
	version := doc.Version
	if version == 0 {
		version = 1
	}
	return doc.Specs, version, nil
}

func buildEntry(db *store.DB, dataDir string, freqByLang map[string]map[string]int, s spec) (catalog.Entry, error) {
	lang := strings.ToLower(s.Language)
	if lang != "fi" && lang != "et" {
		return catalog.Entry{}, fmt.Errorf("language must be fi or et, got %q", s.Language)
	}
	textBytes, err := os.ReadFile(filepath.Join(dataDir, s.Fixture))
	if err != nil {
		return catalog.Entry{}, fmt.Errorf("read fixture: %w", err)
	}
	text := string(textBytes)
	if strings.TrimSpace(text) == "" {
		return catalog.Entry{}, fmt.Errorf("fixture is empty")
	}

	res, err := parsecore.Analyze(db, strings.ToUpper(lang), text, "custom")
	if err != nil {
		return catalog.Entry{}, fmt.Errorf("parse: %w", err)
	}

	metrics := computeMetrics(res, freqByLang[lang])
	difficulty, _ := catalog.BucketFromMetrics(metrics)

	entry := catalog.Entry{
		ID:                 s.ID,
		Language:           lang,
		Title:              s.Title,
		Author:             s.Author,
		Genre:              catalog.Genre(s.Genre),
		SourceURL:          s.SourceURL,
		CorpusSource:       s.CorpusSource,
		License:            s.License,
		Attribution:        s.Attribution,
		TextLength:         utf8.RuneCountInString(text),
		WordCount:          wordCount(text),
		ImportDate:         s.ImportDate,
		Difficulty:         difficulty,
		DifficultyComputed: difficulty,
		Metrics:            metrics,
		DifficultyReview:   "pending",
		Fixture:            s.Fixture,
		Lemmas:             lemmaCounts(res),
	}
	return entry, nil
}

// review is one human difficulty sign-off from the reviews file. Difficulty
// is an optional override of the computed label; the computed value is always
// preserved in DifficultyComputed for calibration.
type review struct {
	Reviewer   string `json:"reviewer"`
	Date       string `json:"date"`
	Difficulty string `json:"difficulty,omitempty"`
	Note       string `json:"note,omitempty"`
}

type reviewsFile struct {
	Version int               `json:"version"`
	Reviews map[string]review `json:"reviews"`
}

// loadReviews reads the human review file. A missing file is not an error —
// it simply means every entry stays "pending".
func loadReviews(path string) (map[string]review, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f reviewsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return f.Reviews, nil
}

var validDifficulties = map[catalog.Difficulty]bool{
	catalog.DifficultyEasy:       true,
	catalog.DifficultyEasyMedium: true,
	catalog.DifficultyMedium:     true,
	catalog.DifficultyMediumHard: true,
	catalog.DifficultyHard:       true,
}

// applyReview marks an entry approved and applies the reviewer's difficulty
// override when present.
func applyReview(e *catalog.Entry, r review) error {
	if r.Reviewer == "" || r.Date == "" {
		return fmt.Errorf("review must carry reviewer and date")
	}
	e.DifficultyReview = "approved"
	e.DifficultyReviewBy = r.Reviewer
	e.DifficultyReviewDate = r.Date
	e.DifficultyReviewNote = r.Note
	if r.Difficulty != "" {
		d := catalog.Difficulty(r.Difficulty)
		if !validDifficulties[d] {
			return fmt.Errorf("review difficulty %q is not a valid label", r.Difficulty)
		}
		e.Difficulty = d
	}
	return nil
}

func wordCount(text string) int {
	return len(strings.Fields(text))
}

// renderCatalog serializes the catalog with sorted, indented JSON so diffs are
// small and reproducible.
func renderCatalog(doc catalog.Catalog) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode catalog: %w", err)
	}
	return buf.Bytes(), nil
}

// catalogContentEqual compares two rendered catalogs ignoring the "generated"
// timestamp field, which is a run date rather than content.
func catalogContentEqual(a, b []byte) bool {
	var da, db catalog.Catalog
	if json.Unmarshal(a, &da) != nil || json.Unmarshal(b, &db) != nil {
		return false
	}
	da.Generated = ""
	db.Generated = ""
	// Normalize ordering the same way both are emitted.
	catalog.SortEntries(da.Entries)
	catalog.SortEntries(db.Entries)
	na, _ := json.Marshal(da)
	nb, _ := json.Marshal(db)
	return bytes.Equal(na, nb)
}
