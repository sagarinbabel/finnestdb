package main

import (
	"encoding/json"
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
// that order. Meanings come from a single batch lookup so we don't fire
// one query per row at FI-full scale (~5M rows).
func (s *state) writeUserFriendlyWordlistWithExampleResolver(
	path string,
	resolveExample userFriendlyExampleResolver,
) error {
	// Single batch lookup: collect every distinct (lemma, pos) once, hit
	// the dict DB once, reuse the map for all wordlist rows.
	keys := s.distinctLemmaKeys()
	glosses := s.dictDB.BatchLookupGlosses(keys, s.langUpper)

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

// distinctLemmaKeys collects each unique (lemma, pos) seen in
// s.wordlistRows. The order isn't significant — BatchLookupGlosses
// returns a map. Pre-allocates assuming most rows have distinct
// (lemma, pos), which is true at FI-full scale (5M rows ≈ 5M distinct
// pairs because the FST emits a different lemma per analysis).
func (s *state) distinctLemmaKeys() []store.LemmaKey {
	seen := make(map[store.LemmaKey]struct{}, len(s.wordlistRows))
	for _, r := range s.wordlistRows {
		seen[store.LemmaKey{Lemma: r.lemma, POS: r.pos}] = struct{}{}
	}
	keys := make([]store.LemmaKey, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return keys
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
