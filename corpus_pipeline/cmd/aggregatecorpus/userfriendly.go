package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"finnestdb/internal/store"
)

// User-friendly wordlist export - item 2 of corpus_pipeline/docs/PR_ROADMAP.md.
//
// Goal: a derived TSV that a learner-facing UI can consume directly. The
// canonical wordlist.tsv is parser evidence - one row per analysis with
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
// pair for a surface. Two implementations - the in-memory state has direct
// access to s.sentences; the scratch path resolves via the hashToID map
// built during phase 4. Both code paths reuse the same writer.
type userFriendlyExampleResolver func(ss *surfaceStats) (refType, refID string)

// writeUserFriendlyWordlist is the in-memory-mode writer. The scratch path
// calls writeUserFriendlyWordlistWithExampleResolver with its own resolver.
func (s *state) writeUserFriendlyWordlist(path string) error {
	return s.writeUserFriendlyWordlistWithExampleResolver(path, s.exampleRefFor)
}

// writeUserFriendlyWordlistWithExampleResolver is the shared writer body -
// the only difference between in-memory and scratch modes is how
// (sentence-hash → sentence-id) resolves, which the resolver closure
// captures.
//
// Honors s.ufWordlistBudget when set: rows are written in user-facing
// usefulness order (surface_count_total desc, is_parser_choice desc,
// analysis_rank asc, surface asc, lemma asc, pos asc) and the writer
// stops accepting rows once the configured byte cap is reached. The
// audit fields s.auditUFWordlistBytes / s.auditUFWordlistCapHit are
// populated so build_metadata.json + qa-report.json can show whether
// the cap actually constrained the export.
//
// Note this writer re-sorts a *copy* of the wordlist row indices - it
// doesn't mutate s.wordlistRows. The canonical wordlist.tsv has its own
// sort (descending prose count) that the caller has already applied;
// this learner-facing export wants a different ordering, and changing
// s.wordlistRows would corrupt any subsequent mining writers that
// consume it.
//
// Meanings come from a single bulk SELECT against `lemmas` - see
// bulkLoadGlosses below - so per-row gloss lookup is a hash hit, not a
// SQL round-trip. Critical at FI-full scale where the wordlist has
// ~5M rows.
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

	// Index-based usefulness sort. Avoids mutating s.wordlistRows so the
	// canonical wordlist + mining writers see their own established order.
	indices := make([]int, len(s.wordlistRows))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(a, b int) bool {
		ra, rb := s.wordlistRows[indices[a]], s.wordlistRows[indices[b]]
		sa, sb := s.surfaces[ra.surface], s.surfaces[rb.surface]
		ta, tb := sa.prose+sa.poetry, sb.prose+sb.poetry
		if ta != tb {
			return ta > tb // surface_count_total desc
		}
		if ra.isParserChoice != rb.isParserChoice {
			return ra.isParserChoice // true (1) before false (0)
		}
		if ra.analysisRank != rb.analysisRank {
			return ra.analysisRank < rb.analysisRank
		}
		if ra.surface != rb.surface {
			return ra.surface < rb.surface
		}
		if ra.lemma != rb.lemma {
			return ra.lemma < rb.lemma
		}
		return ra.pos < rb.pos
	})

	w, err := newCappedTSVWriter(path, header, s.ufWordlistBudget)
	if err != nil {
		return fmt.Errorf("create wordlist_user_friendly.tsv: %w", err)
	}
	for _, idx := range indices {
		r := s.wordlistRows[idx]
		ss := s.surfaces[r.surface]
		exType, exID := resolveExample(ss)
		srcJSON, _ := json.Marshal(ss.sourceCount)
		feats := parseUDFeats(r.feats)
		gloss := glosses[store.LemmaKey{Lemma: r.lemma, POS: r.pos}]
		row := []string{
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
		}
		if !w.Write(row) {
			break // budget hit; stop walking - remaining rows wouldn't fit either
		}
	}
	bytes, capHit, err := w.Close()
	if err != nil {
		return fmt.Errorf("close wordlist_user_friendly.tsv: %w", err)
	}
	s.auditUFWordlistBytes = bytes
	s.auditUFWordlistCapHit = capHit
	progress("phase4", fmt.Sprintf("  wordlist_user_friendly.tsv: %s (cap-hit=%v, %d rows accepted, %d rejected)",
		formatBytes(bytes), capHit, w.rowsWritten, w.rowsRejected))
	return nil
}

// bulkLoadGlosses returns one gloss per (lemma, pos) for the target
// language. The map is consulted per wordlist row at write time, so
// per-row gloss lookup costs a hash probe rather than a SQL round-trip.
//
// Semantics mirror store.BatchLookupGlosses exactly so the user-friendly
// export agrees with the server's read path for the same (lemma, pos):
//
//	COALESCE(
//	  matching-source translation at lowest sense_idx,
//	  lemmas.gloss,
//	  ''
//	)
//
// "Matching-source" means: the translations row whose source equals the
// lemmas row's source for the same (lemma, pos, lang). That coupling
// matters because cmd/importekilexdetails can upgrade a lemma's source
// to 'ekilex' without overwriting an older kaikki gloss (the empty-gloss
// guard preserves richer existing text). When that happens, lemmas.gloss
// is still the kaikki text but the canonical read path returns the
// ekilex translation instead - and the user-friendly export must agree.
//
// Why bulk vs. per-key: store.BatchLookupGlosses is a "batch" only by
// API shape - internally it issues one prepared QueryRow per LemmaKey.
// At FI-full scale (~5M wordlist rows, ~260k distinct dict rows) that
// is ~260k SQL calls in phase 4. The query below runs once and
// LEFT JOINs a CTE that picks the top-priority translation per
// (lemma, pos, lang). One scan, one Go map allocation, no per-row SQL.
//
// The connection is opened read-only (mode=ro) - not immutable. The
// importers run finnestdb.db in WAL mode, and SQLite's immutable flag
// makes the connection ignore the WAL file entirely. With another
// connection still open (state.dictDB) the WAL has not necessarily
// been checkpointed, so an immutable handle here would read a stale
// snapshot - including, in the worst case, missing committed tables.
// Plain mode=ro reads the WAL like any other reader, which is what we
// want.
func bulkLoadGlosses(dbPath, langUpper string) (map[store.LemmaKey]string, error) {
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open ro: %w", err)
	}
	defer db.Close()

	// translation_pick: per (lemma, pos, lang), the translation row whose
	// source matches the lemmas-table source, picked by source_priority
	// DESC then sense_idx ASC (rn=1 wins). The ROW_NUMBER mirrors
	// BatchLookupGlosses' ORDER BY clause; the JOIN to lemmas l2 on
	// source = t.source is the source-coupling that ties each translation
	// to its co-written lemma row.
	//
	// Outer query LEFT JOINs translation_pick into lemmas, then COALESCE
	// falls back to lemmas.gloss when no matching translation exists.
	// The final WHERE drops empties so the returned map only contains
	// non-empty meanings.
	rows, err := db.Query(`
		WITH translation_pick AS (
		  SELECT t.lemma, t.pos, t.lang, t.text,
		    ROW_NUMBER() OVER (
		      PARTITION BY t.lemma, t.pos, t.lang
		      ORDER BY l2.source_priority DESC, t.sense_idx ASC
		    ) AS rn
		  FROM translations t
		  JOIN lemmas l2
		    ON l2.lemma = t.lemma AND l2.pos = t.pos
		    AND l2.lang = t.lang AND l2.source = t.source
		  WHERE t.lang = ? AND t.target_lang = 'EN'
		)
		SELECT l.lemma, l.pos,
		  COALESCE(tp.text, l.gloss, '') AS gloss
		FROM lemmas l
		LEFT JOIN translation_pick tp
		  ON tp.lemma = l.lemma AND tp.pos = l.pos
		  AND tp.lang = l.lang AND tp.rn = 1
		WHERE l.lang = ?
		  AND COALESCE(tp.text, l.gloss, '') != ''
	`, langUpper, langUpper)
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

// writeUserFriendlyWordlistFromScratch is the bounded-memory writer for
// `wordlist_user_friendly.tsv`. Single streaming JOIN against
// `tmp_surface_order` (no per-surface SELECT loop, no in-memory wordlist
// slice). Rows leave SQLite already in the desired learner ordering:
//
//	o.count_total DESC, w.is_parser_choice DESC, w.analysis_rank ASC,
//	w.surface ASC, w.lemma ASC, w.pos ASC
//
// example_ref_type/example_ref_id come from the `example_final_id` /
// `example_poem_id` columns that step 5c populated, so each output row
// carries everything it needs without further lookups.
func (s *state) writeUserFriendlyWordlistFromScratch(path string) error {
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
	w, err := newCappedTSVWriter(path, header, s.ufWordlistBudget)
	if err != nil {
		return fmt.Errorf("create wordlist_user_friendly.tsv: %w", err)
	}

	rows, err := s.scratch.Query(`
		SELECT
		  w.surface, w.lemma, w.pos, w.feats, w.analysis_sources, w.analysis_rank, w.is_parser_choice,
		  o.count_prose, o.count_poetry, o.count_total,
		  o.doc_count_prose, o.doc_count_poetry,
		  o.example_final_id, o.example_poem_id
		FROM tmp_wordlist w
		JOIN tmp_surface_order o ON o.surface = w.surface
		ORDER BY o.count_total DESC,
		         w.is_parser_choice DESC,
		         w.analysis_rank ASC,
		         w.surface ASC,
		         w.lemma ASC,
		         w.pos ASC`)
	if err != nil {
		w.Close()
		return fmt.Errorf("uf wordlist join: %w", err)
	}
	for rows.Next() {
		var surface, lemma, pos, feats, srcs string
		var prose, poetry, total, docProse, docPoetry, rank, ipc int
		var exFinalID sql.NullInt64
		var exPoemID int
		if err := rows.Scan(
			&surface, &lemma, &pos, &feats, &srcs, &rank, &ipc,
			&prose, &poetry, &total,
			&docProse, &docPoetry,
			&exFinalID, &exPoemID,
		); err != nil {
			rows.Close()
			w.Close()
			return err
		}
		exType, exID := resolveExampleRefFromColumns(exFinalID, exPoemID)
		gloss := glosses[store.LemmaKey{Lemma: lemma, POS: pos}]
		fts := parseUDFeats(feats)
		ss := s.surfaces[surface]
		srcJSON, _ := json.Marshal(ss.sourceCount)
		row := []string{
			surface, gloss,
			s.langLower, lemma, pos,
			fts["Case"], fts["Number"], fts["Mood"],
			fts["Tense"], fts["Person"], fts["Voice"],
			fts["VerbForm"],
			feats,
			itoa(prose), itoa(poetry), itoa(total),
			itoa(docProse), itoa(docPoetry),
			string(srcJSON),
			srcs, itoa(rank), boolStr(ipc == 1),
			s.parserVersion, s.fstTablesSHA, s.dictFingerprint,
			exType, exID,
		}
		if !w.Write(row) {
			rows.Close()
			break
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		w.Close()
		return err
	}
	rows.Close()
	bytes, capHit, err := w.Close()
	if err != nil {
		return fmt.Errorf("close wordlist_user_friendly.tsv: %w", err)
	}
	s.auditUFWordlistBytes = bytes
	s.auditUFWordlistCapHit = capHit
	progress("phase4-scratch", fmt.Sprintf("  wordlist_user_friendly.tsv: %s (cap-hit=%v, %d rows accepted, %d rejected)",
		formatBytes(bytes), capHit, w.rowsWritten, w.rowsRejected))
	return nil
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
