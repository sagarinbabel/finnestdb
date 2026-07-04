// Command genlemmatizerwordlist emits the surface wordlist consumed by
// cmd/genlemmatizertables when regenerating the FI lemmatizer FST table.
//
// Three sources, unioned and deduped:
//
//  1. Every distinct lemma in `forms` for the target language. This is
//     the base/citation form of every dict entry. The previous
//     wordlist was lemma-only; keeping it is what preserves the
//     existing 150k-key FST coverage.
//
//  2. Every distinct surface in `forms` whose suffix matches a Finnish
//     non-finite paradigm slot — MA-infinitive case forms (5 slots),
//     E-infinitive inessive, and 4th-infinitive minen-noun. Kaikki
//     ships these inflected surfaces but mostly tags them as the
//     verb headword in `forms`; running them through Voikko gives a
//     deterministic verb-lemma + Inf-FEATS reading that the dict
//     path can lock onto.
//
//     The dict has 26M distinct FI surfaces (every compound × case ×
//     possessive); we deliberately skip the bulk and select by suffix
//     pattern to keep the wordlist on the order of half a million.
//
//  3. For FI verb lemmas (1st-infinitive citation form), synthesize
//     A-infinitive long candidates: lemma + {kseen, kseni, ksesi,
//     ksemme, ksenne}. This is the "in order to V" / "for V-ing"
//     translative+possessive construction. The set is built
//     mechanically because kaikki ships only a small fraction of
//     these surfaces, and even when it does, it often keys them with
//     the surface as its own lemma (mennäkseen→mennäkseen) or with
//     the wrong POS (ymmärtääkseen→ADV). Voikko is the source of
//     truth: it accepts only the valid forms and returns the
//     correctly-stemmed verb lemma + InfForm=1|Person[psor]=N feats.
//
// Output is one surface per line, lowercased, deduped, sorted. The
// downstream cmd/genlemmatizertables handles wordlist parsing itself
// (skip blanks and `#`-comments, lowercase, dedupe) so any prefix-
// preserving emission would work; sorting here is for diff stability
// across runs.
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// aInfLongSuffixes are the five Finnish A-infinitive-long endings: the
// translative case marker -kse- followed by a possessive suffix.
//
// Vowel harmony note: the suffix vowels are e/i throughout, which are
// neutral in Finnish vowel harmony. So `lemma + kseen` works for both
// back-harmony verbs (tulla → tullakseen) and front-harmony verbs
// (mennä → mennäkseen) without per-stem branching.
var aInfLongSuffixes = []string{"kseen", "kseni", "ksesi", "ksemme", "ksenne"}

func main() {
	var (
		dbPath  = flag.String("db", "finnestdb.db", "finnestdb SQLite database")
		lang    = flag.String("lang", "fi", "target language (fi only)")
		outPath = flag.String("out", "", "output file (default stdout)")
	)
	flag.Parse()

	if l := strings.ToLower(*lang); l != "fi" {
		fatalf("only -lang fi is supported (got %q)", *lang)
	}

	db, err := sql.Open("sqlite3", *dbPath+"?_busy_timeout=5000")
	if err != nil {
		fatalf("open db: %v", err)
	}
	defer db.Close()

	surfaces := map[string]struct{}{}

	lemmaCount, err := collectDictLemmas(db, "FI", surfaces)
	if err != nil {
		fatalf("collect dict lemmas: %v", err)
	}

	nonFiniteCount, err := collectNonFiniteSurfaces(db, "FI", surfaces)
	if err != nil {
		fatalf("collect non-finite surfaces: %v", err)
	}

	synthCount, err := synthesizeAInfLong(db, "FI", surfaces)
	if err != nil {
		fatalf("synthesize A-inf-long: %v", err)
	}

	keys := make([]string, 0, len(surfaces))
	for k := range surfaces {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			fatalf("create %s: %v", *outPath, err)
		}
		defer f.Close()
		out = f
	}

	w := bufio.NewWriter(out)
	for _, k := range keys {
		fmt.Fprintln(w, k)
	}
	if err := w.Flush(); err != nil {
		fatalf("flush: %v", err)
	}

	fmt.Fprintf(os.Stderr,
		"wrote %d surfaces (%d lemmas + %d non-finite dict surfaces + %d synthesized A-inf-long)\n",
		len(keys), lemmaCount, nonFiniteCount, synthCount)
}

// collectDictLemmas inserts every distinct lowercase lemma in
// forms.lang=lang into out. Lemmas are the citation forms — for
// Finnish that's the 1st-infinitive for verbs, nominative for nouns,
// etc. They're what the previous wordlist sourced from, so keeping
// them preserves the existing FST table's headword coverage.
func collectDictLemmas(db *sql.DB, lang string, out map[string]struct{}) (int, error) {
	rows, err := db.Query(`SELECT DISTINCT lower(lemma) FROM forms WHERE lang = ?`, lang)
	if err != nil {
		return 0, fmt.Errorf("query lemmas: %w", err)
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return n, fmt.Errorf("scan lemma: %w", err)
		}
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		out[l] = struct{}{}
		n++
	}
	return n, rows.Err()
}

// nonFiniteSuffixGlobs is the set of Finnish suffix patterns that mark
// a non-finite verb form. SQLite's `LIKE` is used with a leading `%`
// so any compound preceding the suffix matches.
//
//   - MA-infinitive case forms (3rd infinitive): 5 cases × 2 vowel
//     harmonies = 10 patterns. Covers learner-relevant constructions
//     like illative ("for V-ing"), inessive ("while V-ing"), etc.
//   - E-infinitive inessive (2nd infinitive, temporal "while V-ing"):
//     4 patterns. The instructive (-en) overlaps too much with
//     nominal endings to filter by suffix alone, so we skip it.
//   - minen-noun (4th infinitive): 1 pattern. The dict tags these as
//     nouns already; we add them here so the FST gives a
//     verb-anchored reading the dict path can compare against.
var nonFiniteSuffixGlobs = []string{
	// MA-inf illative
	"%maan", "%mään",
	// MA-inf inessive
	"%massa", "%mässä",
	// MA-inf elative
	"%masta", "%mästä",
	// MA-inf adessive
	"%malla", "%mällä",
	// MA-inf abessive
	"%matta", "%mättä",
	// E-inf inessive
	"%essa", "%essä", "%iessa", "%iessä",
	// minen-noun nominative
	"%minen",
}

// collectNonFiniteSurfaces inserts every distinct lowercase surface
// matching a non-finite suffix pattern into out. Voikko at table-gen
// time discards any surface it can't analyse, so over-matching here
// (e.g. compound nouns ending in `-massa` like `betonimassa`) is
// safe — they survive as legitimate noun readings.
func collectNonFiniteSurfaces(db *sql.DB, lang string, out map[string]struct{}) (int, error) {
	// Build one disjunctive WHERE clause so we do a single table scan.
	parts := make([]string, len(nonFiniteSuffixGlobs))
	args := make([]any, 0, len(nonFiniteSuffixGlobs)+1)
	args = append(args, lang)
	for i, p := range nonFiniteSuffixGlobs {
		parts[i] = "form LIKE ?"
		args = append(args, p)
	}
	q := `SELECT DISTINCT lower(form) FROM forms WHERE lang = ? AND (` +
		strings.Join(parts, " OR ") + `)`

	rows, err := db.Query(q, args...)
	if err != nil {
		return 0, fmt.Errorf("query non-finite surfaces: %w", err)
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return n, fmt.Errorf("scan form: %w", err)
		}
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		out[f] = struct{}{}
		n++
	}
	return n, rows.Err()
}

// synthesizeAInfLong inserts lemma+suffix for every (verb lemma, A-inf
// long suffix) pair. The synthesis is mechanical — Voikko will reject
// any non-Finnish or invalid combinations at table-generation time, so
// over-generating here is safe.
func synthesizeAInfLong(db *sql.DB, lang string, out map[string]struct{}) (int, error) {
	rows, err := db.Query(
		`SELECT DISTINCT lower(lemma) FROM forms WHERE lang = ? AND pos = 'VERB'`,
		lang,
	)
	if err != nil {
		return 0, fmt.Errorf("query verb lemmas: %w", err)
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var lemma string
		if err := rows.Scan(&lemma); err != nil {
			return n, fmt.Errorf("scan lemma: %w", err)
		}
		lemma = strings.TrimSpace(lemma)
		if lemma == "" {
			continue
		}
		for _, sfx := range aInfLongSuffixes {
			out[lemma+sfx] = struct{}{}
			n++
		}
	}
	return n, rows.Err()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
