package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"finnestdb/internal/store"
)

// User-friendly wordlist export — item 2 of corpus_pipeline/docs/PR_ROADMAP.md.
//
// Goal: a derived TSV that a learner-facing UI can consume directly. The
// canonical wordlist.tsv is parser evidence — one row per analysis with
// raw FEATS strings and provenance fields. The user-friendly export adds:
//
//   - meaning: lemmas.gloss (from the dict DB), so each row has a human-
//     readable meaning attached. Empty when no gloss is available.
//   - case / number / mood / tense / person / voice / verbform: parsed
//     UD FEATS columns. UI code doesn't have to split the pipe-delimited
//     feats string itself; the canonical full feats remains for
//     completeness.
//   - example_ref_type + example_ref_id: same as canonical. Downstream
//     joins against sentences.tsv (or poems.tsv) to render the example.
//
// One row per analysis (all rows, not just parser-choice). is_parser_choice
// + analysis_rank stay in the schema so consumers can pick the
// disambiguated reading (`is_parser_choice=1`) or display the alternatives
// when the parser was uncertain.

// userFriendlyExampleResolver returns the (example_ref_type, example_ref_id)
// pair for a surface. Two implementations — the in-memory state has direct
// access to s.sentences; the scratch path resolves via the hashToID map
// built during phase 4. Both code paths reuse the same writer.
type userFriendlyExampleResolver func(ss *surfaceStats) (refType, refID string)

// writeUserFriendlyWordlist is the in-memory-mode writer. The scratch path
// calls writeUserFriendlyWordlistWithExampleResolver with its own resolver.
func (s *state) writeUserFriendlyWordlist(path string) error {
	return s.writeUserFriendlyWordlistWithExampleResolver(path, s.exampleRefFor)
}

// writeUserFriendlyWordlistWithExampleResolver is the shared writer body —
// the only difference between in-memory and scratch modes is how
// (sentence-hash → sentence-id) resolves, which the resolver closure
// captures.
//
// The wordlist rows are already sorted (by descending prose count, then
// surface, then analysis rank) by the caller, so this writer streams in
// that order. Meanings come from a single bulk SELECT against `lemmas`
// — see bulkLoadGlosses below — so per-row gloss lookup is a hash hit,
// not a SQL round-trip. Critical at FI-full scale where the wordlist
// has ~5M rows.
func (s *state) writeUserFriendlyWordlistWithExampleResolver(
	path string,
	resolveExample userFriendlyExampleResolver,
) error {
	glosses, err := bulkLoadGlosses(s.roots.DBPath, s.langUpper)
	if err != nil {
		return fmt.Errorf("bulk-load glosses: %w", err)
	}

	header := []string{
		"surface", "meaning",
		"lang", "lemma", "pos",
		"case", "number", "mood", "tense", "person", "voice", "verbform",
		"feats",
		"surface_count_prose", "surface_count_poetry", "surface_count_total",
		"doc_count_prose", "doc_count_poetry", "source_counts_json",
		"analysis_sources", "analysis_rank", "is_parser_choice",
		"parser_version", "fst_tables_sha", "dict_fingerprint",
		"example_ref_type", "example_ref_id",
	}
	return writeTSV(path, header, func(yield func([]string)) {
		for _, r := range s.wordlistRows {
			ss := s.surfaces[r.surface]
			exType, exID := resolveExample(ss)
			srcJSON, _ := json.Marshal(ss.sourceCount)
			feats := parseUDFeats(r.feats)
			gloss := glosses[store.LemmaKey{Lemma: r.lemma, POS: r.pos}]
			yield([]string{
				r.surface, gloss,
				s.langLower, r.lemma, r.pos,
				feats["Case"], feats["Number"], feats["Mood"],
				feats["Tense"], feats["Person"], feats["Voice"],
				feats["VerbForm"],
				r.feats,
				itoa(ss.prose), itoa(ss.poetry), itoa(ss.prose + ss.poetry),
				itoa(ss.docCountProse), itoa(ss.docCountPoetry),
				string(srcJSON),
				strings.Join(r.analysisSources, ";"), itoa(r.analysisRank), boolStr(r.isParserChoice),
				s.parserVersion, s.fstTablesSHA, s.dictFingerprint,
				exType, exID,
			})
		}
	})
}

// bulkLoadGlosses reads every (lemma, pos, gloss) row for the target
// language in a single SELECT and returns it as a map keyed by
// (lemma, pos). The map is consulted per wordlist row at write time, so
// per-row gloss lookup costs a hash probe rather than a SQL round-trip.
//
// Why bulk vs. per-key: store.BatchLookupGlosses is a "batch" only by
// API shape — internally it issues one prepared QueryRow per LemmaKey.
// At FI-full scale (~5M wordlist rows, ~260k distinct dict rows) that
// is ~260k SQL calls in phase 4, against a ~30-minute total wall clock.
// Bulk-loading the entire dict for the language is one query, ~260k row
// scan, and a single Go map allocation — measurably cheaper and removes
// SQL latency from the writer's hot path.
//
// The translations-table priority logic that BatchLookupGlosses applies
// is intentionally not duplicated here: the importers already merge
// translations into lemmas.gloss with the same source-priority ladder,
// so reading lemmas.gloss alone is the same answer for the typical case
// (kaikki + ekilex). The few cases where a custom override modified
// lemmas.gloss but not translations are correctly handled — we read
// lemmas.gloss directly. The cases where richer translations text exists
// but lemmas.gloss is blank are exactly the cases this returns no
// meaning for; that's a dict-data shortcoming for a future PR (e.g.
// surfacing translations.text into the user-friendly path), not a
// regression on the BatchLookupGlosses behavior.
//
// The connection is opened read-only + immutable for two reasons:
//   - WAL recovery is unnecessary; the aggregator never writes to the
//     dict DB.
//   - The state already holds a write-mode connection (s.dictDB);
//     opening a second read-only handle here lets the writer query
//     without serializing on that connection's lock.
func bulkLoadGlosses(dbPath, langUpper string) (map[store.LemmaKey]string, error) {
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro&immutable=1")
	if err != nil {
		return nil, fmt.Errorf("open ro: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT lemma, pos, gloss FROM lemmas
		 WHERE lang = ? AND gloss IS NOT NULL AND TRIM(gloss) != ''`,
		langUpper)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	out := make(map[store.LemmaKey]string)
	for rows.Next() {
		var lemma, pos, gloss string
		if err := rows.Scan(&lemma, &pos, &gloss); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out[store.LemmaKey{Lemma: lemma, POS: pos}] = gloss
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter: %w", err)
	}
	return out, nil
}

// parseUDFeats splits a pipe-delimited UD FEATS string into a flat map of
// `Feature → Value`. Returns the empty map for empty input. The wordlist
// already validates feats during phase 2 by passing through parsecore, so
// the format is well-formed; this parser stays minimal and tolerates
// whitespace + missing `=` defensively.
//
// Examples:
//
//	"Case=Ine|Number=Sing"           → {Case: Ine, Number: Sing}
//	"Mood=Ind|Tense=Pres|Voice=Pass" → {Mood: Ind, Tense: Pres, Voice: Pass}
//	""                               → {}
func parseUDFeats(feats string) map[string]string {
	out := make(map[string]string)
	if feats == "" {
		return out
	}
	for _, part := range strings.Split(feats, "|") {
		eq := strings.IndexByte(part, '=')
		if eq <= 0 || eq == len(part)-1 {
			continue
		}
		out[part[:eq]] = part[eq+1:]
	}
	return out
}

// userFriendlyWordlistFilename is the canonical filename. Exported as a
// constant so tests can reference it without the typo risk of a string
// literal in two places.
const userFriendlyWordlistFilename = "wordlist_user_friendly.tsv"

// userFriendlyWordlistPath joins the derived dir to the canonical filename.
// Exported so tooling outside this package (verifier, future readers) can
// share the path computation. _ is here to mirror the path-helper pattern
// used elsewhere in the codebase; remove if a future PR consolidates them.
func userFriendlyWordlistPath(derived string) string {
	return filepath.Join(derived, userFriendlyWordlistFilename)
}
