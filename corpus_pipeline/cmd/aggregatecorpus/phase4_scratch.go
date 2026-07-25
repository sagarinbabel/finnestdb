package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"finnestdb/corpus_pipeline/internal/textfilter"
)

// phase4WriteScratch is the SQLite-backed phase 4. Streams data from
// scratch DB to TSV files using SELECT cursors, never loading all
// sentences into memory at once.
//
// Steps:
//  1. Compute first-occurrence (source, doc_id, sentence_ix) per hash via
//     a single grouped SELECT, sort in memory by stable key, assign final
//     IDs. Memory cost: ~50 bytes × num_unique_sentences.
//  2. Bulk-UPDATE tmp_sentences.final_id.
//  3. Stream SELECT tmp_sentences ORDER BY final_id → write sentences.tsv.
//  4. Stream SELECT occurrences ORDER BY final_id → write sentence_occurrences.tsv.
//  5. Documents: still in-memory (small), write directly.
//  6. Wordlist + mining: enriched in memory during phase 2 (s.wordlistRows
//     etc.), so write from in-memory like the in-memory path. Resolve
//     example_text via SELECT against tmp_sentences.
func (s *state) phase4WriteScratch(derived string, runStart time.Time) error {
	progress("phase4-scratch", "step 1: assigning deterministic sentence IDs")
	// --- Step 1: compute first-occurrence per sentence hash and the
	// deterministic sort order, then persist (hash, final_id) into
	// tmp_sentence_id so subsequent writers can JOIN against it without
	// keeping multi-GB hashToID / hashToText maps in RAM.
	rows, err := s.scratch.Query(`
		SELECT sentence_hash,
		       MIN(source) AS first_source,
		       MIN(document_id) AS first_doc,
		       MIN(sentence_ix) AS first_ix
		FROM tmp_sentence_occurrences
		GROUP BY sentence_hash`)
	if err != nil {
		return fmt.Errorf("first-occurrence query: %w", err)
	}
	type fo struct {
		hash, source, docID string
		ix                  int
	}
	var fos []fo
	for rows.Next() {
		var f fo
		if err := rows.Scan(&f.hash, &f.source, &f.docID, &f.ix); err != nil {
			rows.Close()
			return err
		}
		fos = append(fos, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("first-occurrence scan: %w", err)
	}
	sort.Slice(fos, func(i, j int) bool {
		if fos[i].source != fos[j].source {
			return fos[i].source < fos[j].source
		}
		if fos[i].docID != fos[j].docID {
			return fos[i].docID < fos[j].docID
		}
		if fos[i].ix != fos[j].ix {
			return fos[i].ix < fos[j].ix
		}
		return fos[i].hash < fos[j].hash
	})
	totalSentences := len(fos)
	progress("phase4-scratch", fmt.Sprintf("step 2: %d unique sentences sorted, persisting IDs to tmp_sentence_id", totalSentences))

	// --- Step 2: bulk-INSERT (hash, final_id) into tmp_sentence_id.
	// One transaction, prepared statement, batched commits every 200K rows
	// to keep WAL bounded.
	if _, err := s.scratch.Exec(`DELETE FROM tmp_sentence_id`); err != nil {
		return fmt.Errorf("clear tmp_sentence_id: %w", err)
	}
	const idFlushEvery = 200_000
	tx, err := s.scratch.Begin()
	if err != nil {
		return fmt.Errorf("begin sentence_id tx: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO tmp_sentence_id(hash, final_id) VALUES(?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare sentence_id insert: %w", err)
	}
	for i, f := range fos {
		if _, err := stmt.Exec(f.hash, i+1); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return fmt.Errorf("insert sentence_id (%d): %w", i, err)
		}
		if (i+1)%idFlushEvery == 0 {
			if err := stmt.Close(); err != nil {
				_ = tx.Rollback()
				return err
			}
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit sentence_id batch: %w", err)
			}
			tx, err = s.scratch.Begin()
			if err != nil {
				return err
			}
			stmt, err = tx.Prepare(`INSERT INTO tmp_sentence_id(hash, final_id) VALUES(?, ?)`)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sentence_id final batch: %w", err)
	}
	// fos contains all sentence hashes (~80 B × 70 M = ~5.6 GB at FI
	// scale). Drop it now that tmp_sentence_id holds the same data.
	fos = nil

	progress("phase4-scratch", "step 3: writing sentences.tsv (streaming JOIN)")
	// --- Step 3: stream sentences.tsv via SQL JOIN. No in-memory
	// hashToText map.
	sentRows, err := s.scratch.Query(`
		SELECT s.text, sid.final_id
		FROM tmp_sentences s
		JOIN tmp_sentence_id sid ON sid.hash = s.hash
		ORDER BY sid.final_id`)
	if err != nil {
		return fmt.Errorf("sentences join: %w", err)
	}
	if err := streamWriteTSV(filepath.Join(derived, "sentences.tsv"),
		[]string{"id", "lang", "text"},
		func(yield func([]string)) error {
			defer sentRows.Close()
			for sentRows.Next() {
				var text string
				var id int
				if err := sentRows.Scan(&text, &id); err != nil {
					return err
				}
				yield([]string{itoa(id), s.langLower, text})
			}
			return sentRows.Err()
		}); err != nil {
		return err
	}

	progress("phase4-scratch", fmt.Sprintf("step 3': writing sentences_user_friendly.tsv (budget=%s, streaming JOIN)",
		formatBytesOrUncapped(s.ufSentencesBudget)))
	ufSentPath := filepath.Join(derived, "sentences_user_friendly.tsv")
	ufSentW, err := newCappedTSVWriter(ufSentPath, []string{"id", "lang", "text"}, s.ufSentencesBudget)
	if err != nil {
		return fmt.Errorf("create sentences_user_friendly.tsv: %w", err)
	}
	ufRows, err := s.scratch.Query(`
		SELECT s.text, sid.final_id
		FROM tmp_sentences s
		JOIN tmp_sentence_id sid ON sid.hash = s.hash
		ORDER BY sid.final_id`)
	if err != nil {
		ufSentW.Close()
		return fmt.Errorf("sentences uf join: %w", err)
	}
	for ufRows.Next() {
		var text string
		var id int
		if err := ufRows.Scan(&text, &id); err != nil {
			ufRows.Close()
			ufSentW.Close()
			return err
		}
		if !textfilter.IsUserFriendlySentence(text) {
			continue
		}
		if !ufSentW.Write([]string{itoa(id), s.langLower, text}) {
			break // budget hit
		}
	}
	ufRows.Close()
	ufSentBytes, ufSentCapHit, err := ufSentW.Close()
	if err != nil {
		return fmt.Errorf("close sentences_user_friendly.tsv: %w", err)
	}
	s.auditUFSentencesBytes = ufSentBytes
	s.auditUFSentencesCapHit = ufSentCapHit
	progress("phase4-scratch", fmt.Sprintf("  sentences_user_friendly.tsv: %s (cap-hit=%v, %d rows accepted, %d rejected)",
		formatBytes(ufSentBytes), ufSentCapHit, ufSentW.rowsWritten, ufSentW.rowsRejected))

	progress("phase4-scratch", "step 4: writing sentence_occurrences.tsv (streaming JOIN)")
	// --- Step 4: stream sentence_occurrences.tsv via SQL JOIN against
	// tmp_sentence_id. Memory cost: one SELECT cursor.
	occRows, err := s.scratch.Query(`
		SELECT sid.final_id, oc.source, oc.document_id, oc.sentence_ix, oc.quality_flags
		FROM tmp_sentence_occurrences oc
		JOIN tmp_sentence_id sid ON sid.hash = oc.sentence_hash
		ORDER BY oc.source, oc.document_id, oc.sentence_ix`)
	if err != nil {
		return fmt.Errorf("occurrences join: %w", err)
	}
	if err := streamWriteTSV(filepath.Join(derived, "sentence_occurrences.tsv"),
		[]string{"sentence_id", "source", "document_id", "sentence_ix", "quality_flags"},
		func(yield func([]string)) error {
			defer occRows.Close()
			for occRows.Next() {
				var source, docID, flags string
				var id, ix int
				if err := occRows.Scan(&id, &source, &docID, &ix, &flags); err != nil {
					return err
				}
				yield([]string{itoa(id), source, docID, itoa(ix), flags})
			}
			return occRows.Err()
		}); err != nil {
		return err
	}

	// --- Step 5: documents.tsv (still in-memory, fits easily) ---
	if err := writeTSV(filepath.Join(derived, "documents.tsv"),
		[]string{"document_id", "source", "title", "author", "raw_path"},
		func(yield func([]string)) {
			for _, id := range s.docOrder {
				d := s.documents[id]
				yield([]string{d.id, d.source, d.title, d.author, d.rawPath})
			}
		}); err != nil {
		return err
	}

	// poems.tsv (empty)
	if err := writeTSV(filepath.Join(derived, "poems.tsv"),
		[]string{"id", "lang", "source", "document_id", "title", "author", "line_count", "text"},
		func(yield func([]string)) {}); err != nil {
		return err
	}

	// manifest.tsv
	if err := writeTSV(filepath.Join(derived, "manifest.tsv"),
		[]string{"source", "kind", "license", "url", "format", "fetched_utc"},
		func(yield func([]string)) {
			for _, m := range s.manifests {
				yield([]string{m.Slug, m.Kind, m.License, m.URL, m.Format, runStart.Format(time.RFC3339)})
			}
		}); err != nil {
		return err
	}

	progress("phase4-scratch", fmt.Sprintf("step 5: writing documents.tsv (%d docs)", len(s.docOrder)))

	// --- Step 5b: populate tmp_surface_order ---
	//
	// One row per surface, holding the sort keys (count_prose, count_total)
	// and the example-ref columns. The wordlist writer JOINs against this
	// table once per output (canonical and user-friendly) instead of doing
	// per-surface SELECTs. After the INSERT loop we run a single
	// UPDATE…FROM to fill in example_final_id from tmp_sentence_id -
	// hash-join, not a correlated subquery, so it's O(N) on 18M surfaces.
	progress("phase4-scratch", fmt.Sprintf("step 5b: populating tmp_surface_order (%d surfaces)", len(s.surfaces)))
	if _, err := s.scratch.Exec(`DELETE FROM tmp_surface_order`); err != nil {
		return fmt.Errorf("clear tmp_surface_order: %w", err)
	}
	const surfaceFlushEvery = 50_000
	soTx, err := s.scratch.Begin()
	if err != nil {
		return fmt.Errorf("begin surface_order tx: %w", err)
	}
	soStmt, err := soTx.Prepare(`INSERT INTO tmp_surface_order(surface, count_prose, count_poetry, count_total, doc_count_prose, doc_count_poetry, example_hash, example_poem_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = soTx.Rollback()
		return fmt.Errorf("prepare surface_order insert: %w", err)
	}
	soPending := 0
	for surface, ss := range s.surfaces {
		if _, err := soStmt.Exec(
			surface,
			ss.prose, ss.poetry, ss.prose+ss.poetry,
			ss.docCountProse, ss.docCountPoetry,
			ss.exampleHash, ss.examplePoem,
		); err != nil {
			_ = soStmt.Close()
			_ = soTx.Rollback()
			return fmt.Errorf("insert surface_order: %w", err)
		}
		soPending++
		if soPending >= surfaceFlushEvery {
			if err := soStmt.Close(); err != nil {
				_ = soTx.Rollback()
				return err
			}
			if err := soTx.Commit(); err != nil {
				return fmt.Errorf("commit surface_order batch: %w", err)
			}
			soTx, err = s.scratch.Begin()
			if err != nil {
				return err
			}
			soStmt, err = soTx.Prepare(`INSERT INTO tmp_surface_order(surface, count_prose, count_poetry, count_total, doc_count_prose, doc_count_poetry, example_hash, example_poem_id) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`)
			if err != nil {
				_ = soTx.Rollback()
				return err
			}
			soPending = 0
		}
	}
	_ = soStmt.Close()
	if err := soTx.Commit(); err != nil {
		return fmt.Errorf("commit surface_order final batch: %w", err)
	}

	// Resolve example_final_id via UPDATE…FROM (SQLite ≥ 3.33). Hash-join
	// runs in seconds on 18M surfaces; the previous correlated-subquery
	// alternative would have taken hours.
	progress("phase4-scratch", "step 5c: resolving example_final_id via UPDATE…FROM JOIN")
	if _, err := s.scratch.Exec(`
		UPDATE tmp_surface_order
		SET example_final_id = sid.final_id
		FROM tmp_sentence_id sid
		WHERE sid.hash = tmp_surface_order.example_hash`); err != nil {
		return fmt.Errorf("update example_final_id: %w", err)
	}

	// --- Step 6: write wordlist.tsv via streaming JOIN ---
	progress("phase4-scratch", fmt.Sprintf("step 6: writing wordlist.tsv (streaming JOIN, %d surfaces)", len(s.surfaces)))
	wlRows, err := s.scratch.Query(`
		SELECT
		  w.surface, w.lemma, w.pos, w.feats, w.analysis_sources, w.analysis_rank, w.is_parser_choice,
		  o.count_prose, o.count_poetry, o.count_total,
		  o.doc_count_prose, o.doc_count_poetry,
		  o.example_final_id, o.example_poem_id
		FROM tmp_wordlist w
		JOIN tmp_surface_order o ON o.surface = w.surface
		ORDER BY o.count_prose DESC, o.count_total DESC, w.surface ASC, w.analysis_rank ASC`)
	if err != nil {
		return fmt.Errorf("wordlist canonical join: %w", err)
	}
	wlRowCount := 0
	if err := streamWriteTSV(filepath.Join(derived, "wordlist.tsv"),
		[]string{
			"surface", "surface_count_prose", "surface_count_poetry", "surface_count_total",
			"doc_count_prose", "doc_count_poetry", "source_counts_json",
			"lang", "lemma", "pos", "feats",
			"analysis_sources", "analysis_rank", "is_parser_choice",
			"parser_version", "fst_tables_sha", "dict_fingerprint",
			"example_ref_type", "example_ref_id",
		},
		func(yield func([]string)) error {
			defer wlRows.Close()
			for wlRows.Next() {
				var surface, lemma, pos, feats, srcs string
				var prose, poetry, total, docProse, docPoetry, rank, ipc int
				var exFinalID sql.NullInt64
				var exPoemID int
				if err := wlRows.Scan(
					&surface, &lemma, &pos, &feats, &srcs, &rank, &ipc,
					&prose, &poetry, &total,
					&docProse, &docPoetry,
					&exFinalID, &exPoemID,
				); err != nil {
					return err
				}
				exType, exID := resolveExampleRefFromColumns(exFinalID, exPoemID)
				ss := s.surfaces[surface]
				srcJSON, _ := json.Marshal(ss.sourceCount)
				yield([]string{
					surface,
					itoa(prose), itoa(poetry), itoa(total),
					itoa(docProse), itoa(docPoetry),
					string(srcJSON),
					s.langLower, lemma, pos, feats,
					srcs, itoa(rank), boolStr(ipc == 1),
					s.parserVersion, s.fstTablesSHA, s.dictFingerprint,
					exType, exID,
				})
				wlRowCount++
			}
			return wlRows.Err()
		}); err != nil {
		return err
	}
	progress("phase4-scratch", fmt.Sprintf("  wordlist.tsv: %d rows", wlRowCount))

	// User-friendly wordlist - same JOIN against tmp_surface_order, but
	// with a learner-facing surface ordering + per-row budget cap.
	if err := s.writeUserFriendlyWordlistFromScratch(
		filepath.Join(derived, "wordlist_user_friendly.tsv"),
	); err != nil {
		return err
	}

	// Mining TSVs (sort + write - same logic as in-memory path)
	miningDir := filepath.Join(derived, "mining")
	miningWriter := func(name string, surfaces []wordlistRow) error {
		return writeTSV(filepath.Join(miningDir, name),
			[]string{"surface", "surface_count_prose", "surface_count_poetry", "surface_count_total"},
			func(yield func([]string)) {
				for _, r := range surfaces {
					ss := s.surfaces[r.surface]
					yield([]string{r.surface, itoa(ss.prose), itoa(ss.poetry), itoa(ss.prose + ss.poetry)})
				}
			})
	}
	sortBySurfaceProseDesc := func(rows []wordlistRow) {
		sort.Slice(rows, func(i, j int) bool {
			si, sj := s.surfaces[rows[i].surface], s.surfaces[rows[j].surface]
			if si.prose != sj.prose {
				return si.prose > sj.prose
			}
			return rows[i].surface < rows[j].surface
		})
	}
	sortBySurfacePoetryDesc := func(rows []wordlistRow) {
		sort.Slice(rows, func(i, j int) bool {
			si, sj := s.surfaces[rows[i].surface], s.surfaces[rows[j].surface]
			if si.poetry != sj.poetry {
				return si.poetry > sj.poetry
			}
			return rows[i].surface < rows[j].surface
		})
	}
	sortBySurfaceProseDesc(s.miningUnresolved)
	sortBySurfacePoetryDesc(s.miningPoetryUnresol)
	sortBySurfaceProseDesc(s.miningAmbiguous)
	if err := miningWriter("unresolved.tsv", s.miningUnresolved); err != nil {
		return err
	}
	if err := miningWriter("poetry-unresolved.tsv", s.miningPoetryUnresol); err != nil {
		return err
	}
	if err := miningWriter("high-frequency-ambiguous.tsv", s.miningAmbiguous); err != nil {
		return err
	}
	sort.Slice(s.miningDisagreements, func(i, j int) bool {
		si, sj := s.surfaces[s.miningDisagreements[i].surface], s.surfaces[s.miningDisagreements[j].surface]
		if si.prose != sj.prose {
			return si.prose > sj.prose
		}
		return s.miningDisagreements[i].surface < s.miningDisagreements[j].surface
	})
	if err := writeTSV(filepath.Join(miningDir, "parser-disagreements.tsv"),
		[]string{"surface", "surface_count_prose", "surface_count_poetry", "basic_lemma", "basic_pos", "basic_feats", "custom_lemma", "custom_pos", "custom_feats"},
		func(yield func([]string)) {
			for _, d := range s.miningDisagreements {
				ss := s.surfaces[d.surface]
				yield([]string{
					d.surface, itoa(ss.prose), itoa(ss.poetry),
					d.basicLemma, d.basicPOS, d.basicFeats,
					d.customLemma, d.customPOS, d.customFeats,
				})
			}
		}); err != nil {
		return err
	}
	sort.Slice(s.miningConsensus, func(i, j int) bool {
		si, sj := s.surfaces[s.miningConsensus[i].surface], s.surfaces[s.miningConsensus[j].surface]
		if si.prose != sj.prose {
			return si.prose > sj.prose
		}
		return s.miningConsensus[i].surface < s.miningConsensus[j].surface
	})
	if err := writeTSV(filepath.Join(miningDir, "internal-consensus-candidates.tsv"),
		[]string{"surface", "surface_count_prose", "surface_count_poetry", "agreed_lemma", "agreed_pos", "agreed_feats", "agreement_kind"},
		func(yield func([]string)) {
			for _, c := range s.miningConsensus {
				ss := s.surfaces[c.surface]
				yield([]string{
					c.surface, itoa(ss.prose), itoa(ss.poetry),
					c.agreedLemma, c.agreedPOS, c.agreedFeats, c.agreementKind,
				})
			}
		}); err != nil {
		return err
	}

	// build_metadata.json + qa-report.json
	consumed := []string{}
	for _, m := range s.manifests {
		// A manifest is "consumed" if it wasn't on the budget-skip list.
		skipped := false
		for _, sk := range s.budgetSourcesSkipped {
			if sk == m.Slug {
				skipped = true
				break
			}
		}
		if !skipped {
			consumed = append(consumed, m.Slug)
		}
	}
	manifestSlugs := make([]string, 0, len(s.manifests))
	for _, m := range s.manifests {
		manifestSlugs = append(manifestSlugs, m.Slug)
	}
	if err := writeJSON(filepath.Join(derived, "build_metadata.json"), map[string]any{
		"lang":             s.langLower,
		"parser_version":   s.parserVersion,
		"fst_tables_sha":   s.fstTablesSHA,
		"dict_fingerprint": s.dictFingerprint,
		"db_path":          s.roots.DBPath,
		"data_root":        s.roots.DataRoot,
		"run_start_utc":    runStart.Format(time.RFC3339),
		"run_end_utc":      time.Now().UTC().Format(time.RFC3339),
		"sources":          s.manifests,
		"scratch_mode":     true,
		"source_order_mode": s.sourceOrderMode,
		"sources_ordered":   manifestSlugs,
		"sources_consumed":  consumed,
		"sources_skipped_by_budget": s.budgetSourcesSkipped,
		"sources_partial_by_budget": s.budgetSourcePartial,
		"user_friendly_budgets": map[string]any{
			"sentences_bytes_budget":  s.ufSentencesBudget,
			"sentences_bytes_actual":  s.auditUFSentencesBytes,
			"sentences_cap_hit":       s.auditUFSentencesCapHit,
			"sentences_bytes_estimate_phase1": s.ufSentencesBytesEstimate,
			"wordlist_bytes_budget":   s.ufWordlistBudget,
			"wordlist_bytes_actual":   s.auditUFWordlistBytes,
			"wordlist_cap_hit":        s.auditUFWordlistCapHit,
		},
	}); err != nil {
		return err
	}
	if err := s.writeQAReportScratch(filepath.Join(derived, "qa-report.json"), runStart, len(fos)); err != nil {
		return err
	}
	return nil
}

// resolveExampleRefFromColumns translates the (example_final_id,
// example_poem_id) pair from tmp_surface_order into the canonical
// example_ref_type / example_ref_id strings that downstream consumers
// expect.
//
// Precedence: a poem ID > 0 wins over a sentence ID. This matches the
// in-memory `exampleRefFor` semantics.
func resolveExampleRefFromColumns(exFinalID sql.NullInt64, exPoemID int) (string, string) {
	if exPoemID > 0 {
		return "poem", itoa(exPoemID)
	}
	if exFinalID.Valid && exFinalID.Int64 > 0 {
		return "sentence", itoa(int(exFinalID.Int64))
	}
	return "", ""
}

// writeQAReportScratch is the same as writeQAReport except sentences_unique
// comes from the SQL count (since s.sentences is empty post-flush).
func (s *state) writeQAReportScratch(path string, runStart time.Time, sentencesUnique int) error {
	tokensProse, tokensPoetry := 0, 0
	uniqProse, uniqPoetry, poetryOnly, proseOnly := 0, 0, 0, 0
	unresolvedProse, unresolvedPoetry := 0, 0
	for _, ss := range s.surfaces {
		tokensProse += ss.prose
		tokensPoetry += ss.poetry
		if ss.prose > 0 && ss.poetry > 0 {
			uniqProse++
			uniqPoetry++
		} else if ss.prose > 0 {
			uniqProse++
			proseOnly++
		} else if ss.poetry > 0 {
			uniqPoetry++
			poetryOnly++
		}
		if !ss.resolved {
			if ss.prose > 0 {
				unresolvedProse++
			}
			if ss.poetry > 0 {
				unresolvedPoetry++
			}
		}
	}
	rateOf := func(num, denom int) float64 {
		if denom == 0 {
			return 0
		}
		return float64(num) / float64(denom)
	}
	var occCount int
	_ = s.scratch.QueryRow(`SELECT COUNT(*) FROM tmp_sentence_occurrences`).Scan(&occCount)
	var docCount int
	_ = s.scratch.QueryRow(`SELECT COUNT(*) FROM tmp_documents`).Scan(&docCount)
	report := map[string]any{
		"lang":             s.langLower,
		"run_start_utc":    runStart.Format(time.RFC3339),
		"run_end_utc":      time.Now().UTC().Format(time.RFC3339),
		"parser_version":   s.parserVersion,
		"fst_tables_sha":   s.fstTablesSHA,
		"dict_fingerprint": s.dictFingerprint,
		"scratch_mode":     true,
		"source_order_mode":         s.sourceOrderMode,
		"sources_skipped_by_budget": s.budgetSourcesSkipped,
		"sources_partial_by_budget": s.budgetSourcePartial,
		"user_friendly_budgets": map[string]any{
			"sentences_bytes_budget": s.ufSentencesBudget,
			"sentences_bytes_actual": s.auditUFSentencesBytes,
			"sentences_cap_hit":      s.auditUFSentencesCapHit,
			"wordlist_bytes_budget":  s.ufWordlistBudget,
			"wordlist_bytes_actual":  s.auditUFWordlistBytes,
			"wordlist_cap_hit":       s.auditUFWordlistCapHit,
		},
		"totals": map[string]any{
			"sources":                     len(s.manifests),
			"documents":                   docCount,
			"sentences_unique":            sentencesUnique,
			"sentences_total_occurrences": occCount,
			"tokens_total":                tokensProse + tokensPoetry,
			"tokens_prose":                tokensProse,
			"tokens_poetry":               tokensPoetry,
			"unique_surfaces":             len(s.surfaces),
			"unique_surfaces_prose":       uniqProse,
			"unique_surfaces_poetry":      uniqPoetry,
			"poetry_only_surfaces":        poetryOnly,
			"prose_only_surfaces":         proseOnly,
			"unresolved_surfaces_total":   unresolvedProse + unresolvedPoetry,
			"unresolved_rate_total":       rateOf(unresolvedProse+unresolvedPoetry, len(s.surfaces)),
			"unresolved_surfaces_prose":   unresolvedProse,
			"unresolved_rate_prose":       rateOf(unresolvedProse, uniqProse),
			"unresolved_surfaces_poetry":  unresolvedPoetry,
			"unresolved_rate_poetry":      rateOf(unresolvedPoetry, uniqPoetry),
			"ambiguous_surfaces":          len(s.miningAmbiguous),
			"poems":                       0,
		},
	}
	return writeJSON(path, report)
}

// streamWriteTSV is a streaming version of writeTSV - the yield function
// pulls rows from a SQL cursor and writes them directly without buffering.
func streamWriteTSV(path string, header []string, stream func(yield func([]string)) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Comma = '\t'
	if err := w.Write(header); err != nil {
		return err
	}
	if err := stream(func(row []string) {
		_ = w.Write(row)
	}); err != nil {
		w.Flush()
		return err
	}
	w.Flush()
	return w.Error()
}
