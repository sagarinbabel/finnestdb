// cmd/epubdeck produces a per-book wordlist (a "deck" suitable for
// flashcard study). For each EPUB in localdata/{lang}-corpus/epub/raw/
// (or a single -epub <file>), runs aggregator logic scoped to that one
// book's text, writes to localdata/{lang}-corpus/epub/decks/<slug>.tsv.
//
// Usage:
//
//	go run ./cmd/epubdeck -lang fi                     # all books
//	go run ./cmd/epubdeck -lang fi -epub <filename>    # one specific book
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/parserffi"
	"finnestdb/internal/store"
	lemmatizer "finnestdb/pkg/lemmatizer-fi-et"

	"finnestdb/corpus_pipeline/internal/cli"
)

type stats struct {
	count       int
	exampleText string
}

type deckRow struct {
	surface         string
	count           int
	lemma           string
	pos             string
	feats           string
	analysisSources []string
	analysisRank    int
	isParserChoice  bool
	exampleText     string
}

func main() {
	var (
		dataRoot = flag.String("data-root", "../localdata", "")
		lang     = flag.String("lang", "fi", "")
		dbPath   = flag.String("db", "", "")
		epubName = flag.String("epub", "", "if set, only this filename in epub/raw/")
	)
	flag.Parse()

	roots, err := cli.Resolve(*dataRoot, *dbPath)
	if err != nil {
		log.Fatalf("resolve: %v", err)
	}
	langLower, langUpper, err := cli.LangCodes(*lang)
	if err != nil {
		log.Fatalf("lang: %v", err)
	}
	if err := cli.PreflightTables(roots.TablesDir); err != nil {
		log.Fatalf("preflight tables: %v", err)
	}
	if err := cli.PreflightDB(roots.DBPath, langUpper); err != nil {
		log.Fatalf("preflight db: %v", err)
	}

	dictDB, err := cli.OpenDB(roots.DBPath)
	if err != nil {
		log.Fatalf("open dict: %v", err)
	}
	defer dictDB.Close()
	lem, err := lemmatizer.NewFromDir(roots.TablesDir)
	if err != nil {
		log.Fatalf("lemmatizer: %v", err)
	}
	defer lem.Close()

	perBookDir := filepath.Join(roots.DataRoot, langLower+"-corpus", "epub", "per-book")
	decksDir := filepath.Join(roots.DataRoot, langLower+"-corpus", "epub", "decks")
	if err := os.MkdirAll(decksDir, 0o755); err != nil {
		log.Fatalf("mkdir decks: %v", err)
	}

	entries, err := os.ReadDir(perBookDir)
	if err != nil {
		log.Fatalf("read per-book dir %s (run extractcorpus first?): %v", perBookDir, err)
	}

	want := ""
	if *epubName != "" {
		want = slugifyEPUB(strings.TrimSuffix(*epubName, filepath.Ext(*epubName)))
	}
	processed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".txt") {
			continue
		}
		bookSlug := strings.TrimSuffix(name, ".txt")
		if want != "" && bookSlug != want {
			continue
		}
		textPath := filepath.Join(perBookDir, name)
		deckPath := filepath.Join(decksDir, bookSlug+".tsv")
		if err := buildDeck(textPath, deckPath, dictDB, lem, langLower, langUpper); err != nil {
			fmt.Fprintf(os.Stderr, "[epubdeck] %s: FAIL %v\n", bookSlug, err)
			continue
		}
		processed++
		if processed%25 == 0 {
			fmt.Fprintf(os.Stderr, "[epubdeck] %d books processed\n", processed)
		}
	}
	fmt.Fprintf(os.Stderr, "[epubdeck] done: %d decks written to %s\n", processed, decksDir)
}

func buildDeck(textPath, deckPath string, dictDB *store.DB, lem *lemmatizer.Lemmatizer, langLower, langUpper string) error {
	data, err := os.ReadFile(textPath)
	if err != nil {
		return err
	}
	// Tokenize via parserffi paragraph-by-paragraph
	surfaces := map[string]*stats{}
	for _, paragraph := range strings.Split(string(data), "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		result, err := parserffi.AnalyzeText(langUpper, paragraph)
		if err != nil {
			continue
		}
		for _, sent := range result.Sentences {
			text := reconstructText(sent.Tokens)
			for _, t := range sent.Tokens {
				if isPunctOnly(t.Form) {
					continue
				}
				ss, ok := surfaces[t.Form]
				if !ok {
					ss = &stats{exampleText: text}
					surfaces[t.Form] = ss
				}
				ss.count++
			}
		}
	}
	// Enrich each unique surface
	rows := make([]deckRow, 0, len(surfaces))
	for surface, ss := range surfaces {
		drs := enrichForDeck(surface, ss, dictDB, lem, langUpper)
		rows = append(rows, drs...)
	}
	// Sort by count desc, surface asc, rank asc
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count != rows[j].count {
			return rows[i].count > rows[j].count
		}
		if rows[i].surface != rows[j].surface {
			return rows[i].surface < rows[j].surface
		}
		return rows[i].analysisRank < rows[j].analysisRank
	})
	// Write
	f, err := os.Create(deckPath)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Comma = '\t'
	if err := w.Write([]string{"surface", "count", "lang", "lemma", "pos", "feats", "analysis_sources", "analysis_rank", "is_parser_choice", "example_text"}); err != nil {
		return err
	}
	for _, r := range rows {
		_ = w.Write([]string{
			r.surface, fmt.Sprintf("%d", r.count), langLower,
			r.lemma, r.pos, r.feats,
			strings.Join(r.analysisSources, ";"),
			fmt.Sprintf("%d", r.analysisRank),
			boolStr01(r.isParserChoice),
			truncate(r.exampleText, 400),
		})
	}
	w.Flush()
	return w.Error()
}

func enrichForDeck(surface string, ss *stats, dictDB *store.DB, lem *lemmatizer.Lemmatizer, langUpper string) []deckRow {
	type key struct{ lemma, pos, feats string }
	rows := map[key]*deckRow{}
	if pr, err := parsecore.Analyze(dictDB, langUpper, surface, "custom"); err == nil && pr != nil {
		for _, sent := range pr.Sentences {
			for _, tok := range sent.Tokens {
				if tok.Form != surface {
					continue
				}
				k := key{tok.Lemma, tok.POS, tok.Feats}
				rows[k] = &deckRow{
					surface: surface, count: ss.count,
					lemma: tok.Lemma, pos: tok.POS, feats: tok.Feats,
					analysisSources: []string{"parser_choice"},
					isParserChoice:  true,
					exampleText:     ss.exampleText,
				}
				break
			}
		}
	}
	for _, an := range lem.Lemmatize(langUpper, surface) {
		k := key{an.Lemma, an.UPOS, an.Feats}
		if r, ok := rows[k]; ok {
			r.analysisSources = append(r.analysisSources, "fst")
		} else {
			rows[k] = &deckRow{
				surface: surface, count: ss.count,
				lemma: an.Lemma, pos: an.UPOS, feats: an.Feats,
				analysisSources: []string{"fst"},
				exampleText:     ss.exampleText,
			}
		}
	}
	if len(rows) == 0 {
		return []deckRow{{surface: surface, count: ss.count, exampleText: ss.exampleText, analysisRank: 1}}
	}
	out := make([]deckRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].isParserChoice != out[j].isParserChoice {
			return out[i].isParserChoice
		}
		return out[i].lemma < out[j].lemma
	})
	for i := range out {
		out[i].analysisRank = i + 1
	}
	return out
}

// ── shared helpers (duplicated from aggregatecorpus to avoid an internal pkg
//    just for these) ────────────────────────────────────────────────────

func reconstructText(tokens []parserffi.Token) string {
	var b strings.Builder
	for i, t := range tokens {
		if i > 0 && !isPunctOnly(t.Form) {
			b.WriteByte(' ')
		}
		b.WriteString(t.Form)
	}
	return b.String()
}

func isPunctOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r > 127:
			return false
		}
	}
	return true
}

func boolStr01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func truncate(s string, max int) string {
	count := 0
	for i := range s {
		if count >= max {
			return s[:i] + "…"
		}
		count++
	}
	return s
}

func slugifyEPUB(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r == ' ' || r == '_' || r == '-' || r == '.':
			b.WriteByte('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return strings.Trim(out, "-")
}
