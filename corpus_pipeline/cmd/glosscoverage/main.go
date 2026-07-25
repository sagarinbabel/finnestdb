// cmd/glosscoverage measures lexical-source gloss coverage against the
// corpus wordlist for a given language and emits a JSON breakdown.
//
// Two metrics:
//
//	pair coverage  - distinct (lemma, pos, lang) tuples in the wordlist whose
//	                 lemmas.gloss is non-empty. The lemma table view.
//	token coverage - same join, but weighted by the surface_count_total of
//	                 each parser-choice row. The user-experience view, since
//	                 a wordlist row's frequency in the corpus is what drives
//	                 how often a learner actually sees the gloss.
//
// Usage:
//
//	go run ./cmd/glosscoverage -lang fi
//	go run ./cmd/glosscoverage -lang et -out reports/2026-05-09-et.json
//
// The command reads `localdata/{lang}-corpus/_derived/wordlist.tsv` and the
// dictionary DB; it does not modify either. Designed to run before and after
// a meaning-source import so the JSON outputs can be diffed.
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"finnestdb/corpus_pipeline/internal/cli"
)

// Report is the JSON shape this command emits. Stable fields so a before/
// after diff stays readable.
type Report struct {
	Lang             string         `json:"lang"`
	GeneratedAt      string         `json:"generated_at"`
	Wordlist         string         `json:"wordlist"`
	DB               string         `json:"db"`
	UniqueSurfaces   int64          `json:"unique_surfaces"`
	UniqueParserRows int64          `json:"unique_parser_choice_rows"`
	UniquePairs     int64           `json:"unique_lemma_pos_pairs"`
	TotalTokens     int64           `json:"total_tokens"`
	Pairs           CoverageBucket  `json:"pairs"`
	Tokens          CoverageBucket  `json:"tokens"`
	BySource        []SourceSummary `json:"by_dict_source"`
}

// CoverageBucket carries the four-way split - has gloss, in dict but missing
// gloss, not in dict, and the implicit total via With/InDictNoGloss/NotInDict.
type CoverageBucket struct {
	Total         int64   `json:"total"`
	WithGloss     int64   `json:"with_gloss"`
	InDictNoGloss int64   `json:"in_dict_no_gloss"`
	NotInDict     int64   `json:"not_in_dict"`
	WithGlossPct  float64 `json:"with_gloss_pct"`
	NoGlossPct    float64 `json:"in_dict_no_gloss_pct"`
	NotInDictPct  float64 `json:"not_in_dict_pct"`
}

// SourceSummary breaks gloss coverage down by lemmas.source value, useful for
// understanding which import path is bringing meanings vs leaving rows empty.
type SourceSummary struct {
	Source     string `json:"source"`
	Rows       int64  `json:"rows"`
	WithGloss  int64  `json:"with_gloss"`
	Coverage   string `json:"coverage_pct"`
}

// pairKey identifies a (lemma, pos) tuple. lang is implicit - one report per
// language.
type pairKey struct {
	Lemma string
	POS   string
}

func main() {
	var (
		dataRoot = flag.String("data-root", "../localdata", "")
		lang     = flag.String("lang", "fi", "fi or et")
		dbPath   = flag.String("db", "", "path to finnestdb.db (default: <repoRoot>/finnestdb.db)")
		outPath  = flag.String("out", "", "JSON output path; if empty, write to stdout")
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
	if err := cli.PreflightDB(roots.DBPath, langUpper); err != nil {
		log.Fatalf("preflight db: %v", err)
	}

	wordlistPath := filepath.Join(roots.DataRoot, langLower+"-corpus", "_derived", "wordlist.tsv")
	if _, err := os.Stat(wordlistPath); err != nil {
		log.Fatalf("wordlist missing: %s: %v", wordlistPath, err)
	}

	tokensByPair, parserChoiceRows, surfaces, err := readCorpusPairs(wordlistPath)
	if err != nil {
		log.Fatalf("read wordlist: %v", err)
	}

	db, err := sql.Open("sqlite3", roots.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	pairsBucket, tokensBucket, err := joinAndBucket(db, langUpper, tokensByPair)
	if err != nil {
		log.Fatalf("join: %v", err)
	}
	bySource, err := lemmaSourceBreakdown(db, langUpper)
	if err != nil {
		log.Fatalf("by_source: %v", err)
	}

	var totalTokens int64
	for _, n := range tokensByPair {
		totalTokens += n
	}

	rep := Report{
		Lang:             langLower,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		Wordlist:         wordlistPath,
		DB:               roots.DBPath,
		UniqueSurfaces:   surfaces,
		UniqueParserRows: parserChoiceRows,
		UniquePairs:      int64(len(tokensByPair)),
		TotalTokens:      totalTokens,
		Pairs:            pairsBucket,
		Tokens:           tokensBucket,
		BySource:         bySource,
	}

	out := io.Writer(os.Stdout)
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			log.Fatalf("create out: %v", err)
		}
		defer f.Close()
		out = f
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		log.Fatalf("encode: %v", err)
	}
}

// readCorpusPairs streams the wordlist TSV and accumulates token weights per
// (lemma, pos) pair. Only parser-choice rows count - without that filter, a
// surface with N analyses would be counted N times for the same surface_count
// total, inflating both pair and token totals.
func readCorpusPairs(path string) (map[pairKey]int64, int64, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	if !scanner.Scan() {
		return nil, 0, 0, fmt.Errorf("empty wordlist: %s", path)
	}
	header := strings.Split(scanner.Text(), "\t")
	idx := func(name string) int {
		for i, h := range header {
			if h == name {
				return i
			}
		}
		return -1
	}
	iSurface := idx("surface")
	iLemma := idx("lemma")
	iPOS := idx("pos")
	iCount := idx("surface_count_total")
	iParser := idx("is_parser_choice")
	if iSurface < 0 || iLemma < 0 || iPOS < 0 || iCount < 0 || iParser < 0 {
		return nil, 0, 0, fmt.Errorf("wordlist missing required columns; got header %v", header)
	}

	tokens := make(map[pairKey]int64)
	var parserRows int64
	uniqueSurfaces := make(map[string]struct{})
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) <= iParser {
			continue
		}
		uniqueSurfaces[fields[iSurface]] = struct{}{}
		// "1" / "true" both accepted defensively - different writers stamped
		// the column differently across versions.
		if fields[iParser] != "1" && fields[iParser] != "true" {
			continue
		}
		parserRows++
		cnt, err := strconv.ParseInt(fields[iCount], 10, 64)
		if err != nil {
			continue
		}
		tokens[pairKey{Lemma: fields[iLemma], POS: fields[iPOS]}] += cnt
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, 0, err
	}
	return tokens, parserRows, int64(len(uniqueSurfaces)), nil
}

// joinAndBucket creates a temp table of corpus (lemma, pos, weight) and
// LEFT JOINs against lemmas to bucket each pair into has-gloss / in-dict-no-
// gloss / not-in-dict. Doing the join in SQL keeps memory predictable for
// the FI corpus (~5M pairs) and cleanly handles the lemma vs forms-table
// distinction.
func joinAndBucket(db *sql.DB, langUpper string, tokens map[pairKey]int64) (CoverageBucket, CoverageBucket, error) {
	tx, err := db.Begin()
	if err != nil {
		return CoverageBucket{}, CoverageBucket{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`CREATE TEMP TABLE corpus_lp(lemma TEXT, pos TEXT, weight INTEGER)`); err != nil {
		return CoverageBucket{}, CoverageBucket{}, err
	}
	stmt, err := tx.Prepare(`INSERT INTO corpus_lp(lemma, pos, weight) VALUES (?, ?, ?)`)
	if err != nil {
		return CoverageBucket{}, CoverageBucket{}, err
	}
	for k, w := range tokens {
		if _, err := stmt.Exec(k.Lemma, k.POS, w); err != nil {
			stmt.Close()
			return CoverageBucket{}, CoverageBucket{}, err
		}
	}
	stmt.Close()

	row := tx.QueryRow(`
		SELECT
		  COUNT(*) AS pairs,
		  SUM(c.weight) AS tokens,
		  SUM(CASE WHEN l.gloss IS NOT NULL AND TRIM(l.gloss) != '' THEN 1 ELSE 0 END) AS pairs_with_gloss,
		  SUM(CASE WHEN l.gloss IS NOT NULL AND TRIM(l.gloss) != '' THEN c.weight ELSE 0 END) AS tokens_with_gloss,
		  SUM(CASE WHEN l.lemma IS NOT NULL AND (l.gloss IS NULL OR TRIM(l.gloss) = '') THEN 1 ELSE 0 END) AS pairs_no_gloss,
		  SUM(CASE WHEN l.lemma IS NOT NULL AND (l.gloss IS NULL OR TRIM(l.gloss) = '') THEN c.weight ELSE 0 END) AS tokens_no_gloss
		FROM corpus_lp c
		LEFT JOIN lemmas l ON l.lemma = c.lemma AND l.pos = c.pos AND l.lang = ?
	`, langUpper)
	var pairs, totalTokens, pairsGloss, tokensGloss, pairsNoGloss, tokensNoGloss int64
	if err := row.Scan(&pairs, &totalTokens, &pairsGloss, &tokensGloss, &pairsNoGloss, &tokensNoGloss); err != nil {
		return CoverageBucket{}, CoverageBucket{}, fmt.Errorf("scan: %w", err)
	}

	pairBucket := newBucket(pairs, pairsGloss, pairsNoGloss)
	tokenBucket := newBucket(totalTokens, tokensGloss, tokensNoGloss)
	return pairBucket, tokenBucket, nil
}

func newBucket(total, withGloss, inDictNoGloss int64) CoverageBucket {
	notInDict := total - withGloss - inDictNoGloss
	pct := func(n int64) float64 {
		if total == 0 {
			return 0.0
		}
		return float64(n) / float64(total) * 100.0
	}
	return CoverageBucket{
		Total:         total,
		WithGloss:     withGloss,
		InDictNoGloss: inDictNoGloss,
		NotInDict:     notInDict,
		WithGlossPct:  round2(pct(withGloss)),
		NoGlossPct:    round2(pct(inDictNoGloss)),
		NotInDictPct:  round2(pct(notInDict)),
	}
}

func round2(f float64) float64 {
	return float64(int64(f*100.0+0.5)) / 100.0
}

func lemmaSourceBreakdown(db *sql.DB, langUpper string) ([]SourceSummary, error) {
	rows, err := db.Query(`
		SELECT
		  COALESCE(source, '<null>') AS s,
		  COUNT(*) AS rows,
		  SUM(CASE WHEN gloss IS NOT NULL AND TRIM(gloss) != '' THEN 1 ELSE 0 END) AS with_gloss
		FROM lemmas WHERE lang = ?
		GROUP BY s
		ORDER BY rows DESC
	`, langUpper)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceSummary
	for rows.Next() {
		var s SourceSummary
		if err := rows.Scan(&s.Source, &s.Rows, &s.WithGloss); err != nil {
			return nil, err
		}
		pct := 0.0
		if s.Rows > 0 {
			pct = float64(s.WithGloss) / float64(s.Rows) * 100.0
		}
		s.Coverage = fmt.Sprintf("%.2f%%", pct)
		out = append(out, s)
	}
	return out, rows.Err()
}
