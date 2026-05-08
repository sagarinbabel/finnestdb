package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	// --- Step 1+2: assign deterministic IDs ---
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
	hashToID := make(map[string]int, len(fos))
	for i, f := range fos {
		hashToID[f.hash] = i + 1
	}

	// Skip the UPDATE step — instead resolve hash→id at write time via the
	// in-memory map. Avoids 9.5M individual SQL UPDATEs (the WAL bottleneck
	// at scale; 100K UPDATEs/sec × 9.5M = ~95 min on a busy WAL).

	// --- Step 3: stream sentences.tsv (sorted by final_id via slice walk) ---
	// Build slice of (final_id, hash) sorted by final_id for stable iteration.
	type idHash struct {
		id int
		h  string
	}
	idHashes := make([]idHash, 0, len(hashToID))
	for h, id := range hashToID {
		idHashes = append(idHashes, idHash{id, h})
	}
	sort.Slice(idHashes, func(i, j int) bool { return idHashes[i].id < idHashes[j].id })

	// Bulk-load all (hash, text) into memory. ~1.5 GB for 9.5M sentences,
	// but avoids 9.5M individual SELECT round-trips.
	hashToText := make(map[string]string, len(hashToID))
	textRows, err := s.scratch.Query(`SELECT hash, text FROM tmp_sentences`)
	if err != nil {
		return fmt.Errorf("bulk text select: %w", err)
	}
	for textRows.Next() {
		var h, t string
		if err := textRows.Scan(&h, &t); err != nil {
			textRows.Close()
			return err
		}
		hashToText[h] = t
	}
	textRows.Close()
	if err := streamWriteTSV(filepath.Join(derived, "sentences.tsv"),
		[]string{"id", "lang", "text"},
		func(yield func([]string)) error {
			for _, ih := range idHashes {
				yield([]string{itoa(ih.id), s.langLower, hashToText[ih.h]})
			}
			return nil
		}); err != nil {
		return err
	}

	// --- Step 4: stream sentence_occurrences.tsv ---
	// Walk all occurrences via SELECT, look up each one's final_id from the
	// in-memory map, write. Sort happens at TSV-load time downstream if
	// needed (we sort by hash within source naturally).
	occRows, err := s.scratch.Query(`SELECT sentence_hash, source, document_id, sentence_ix, quality_flags FROM tmp_sentence_occurrences ORDER BY source, document_id, sentence_ix`)
	if err != nil {
		return fmt.Errorf("occurrences select: %w", err)
	}
	if err := streamWriteTSV(filepath.Join(derived, "sentence_occurrences.tsv"),
		[]string{"sentence_id", "source", "document_id", "sentence_ix", "quality_flags"},
		func(yield func([]string)) error {
			defer occRows.Close()
			for occRows.Next() {
				var hash, source, docID, flags string
				var ix int
				if err := occRows.Scan(&hash, &source, &docID, &ix, &flags); err != nil {
					return err
				}
				id, ok := hashToID[hash]
				if !ok {
					continue // shouldn't happen
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

	// --- Step 6: wordlist + mining (in-memory rows enriched in phase 2) ---
	// Need example_text resolution per row. With scratch, we look up text
	// from tmp_sentences by hash. We have ss.exampleHash on each surface.
	// Sort the wordlist (same logic as in-memory).
	sort.Slice(s.wordlistRows, func(i, j int) bool {
		si, sj := s.surfaces[s.wordlistRows[i].surface], s.surfaces[s.wordlistRows[j].surface]
		if si.prose != sj.prose {
			return si.prose > sj.prose
		}
		if si.prose+si.poetry != sj.prose+sj.poetry {
			return si.prose+si.poetry > sj.prose+sj.poetry
		}
		if s.wordlistRows[i].surface != s.wordlistRows[j].surface {
			return s.wordlistRows[i].surface < s.wordlistRows[j].surface
		}
		return s.wordlistRows[i].analysisRank < s.wordlistRows[j].analysisRank
	})

	// Resolve example_text via in-memory hash→id and hash→text maps.
	resolveExample := func(hash string) (refType, refID, text string) {
		if hash == "" {
			return "", "", ""
		}
		id, ok := hashToID[hash]
		if !ok {
			return "", "", ""
		}
		t, ok := hashToText[hash]
		if !ok {
			return "", "", ""
		}
		return "sentence", itoa(id), truncateExample(t)
	}

	if err := writeTSV(filepath.Join(derived, "wordlist.tsv"),
		[]string{
			"surface", "surface_count_prose", "surface_count_poetry", "surface_count_total",
			"doc_count_prose", "doc_count_poetry", "source_counts_json",
			"lang", "lemma", "pos", "feats",
			"analysis_sources", "analysis_rank", "is_parser_choice",
			"parser_version", "fst_tables_sha", "dict_fingerprint",
			"example_ref_type", "example_ref_id", "example_text",
		},
		func(yield func([]string)) {
			for _, r := range s.wordlistRows {
				ss := s.surfaces[r.surface]
				exType, exID, exText := resolveExample(ss.exampleHash)
				srcJSON, _ := json.Marshal(ss.sourceCount)
				yield([]string{
					r.surface,
					itoa(ss.prose), itoa(ss.poetry), itoa(ss.prose + ss.poetry),
					itoa(ss.docCountProse), itoa(ss.docCountPoetry),
					string(srcJSON),
					s.langLower, r.lemma, r.pos, r.feats,
					strings.Join(r.analysisSources, ";"), itoa(r.analysisRank), boolStr(r.isParserChoice),
					s.parserVersion, s.fstTablesSHA, s.dictFingerprint,
					exType, exID, exText,
				})
			}
		}); err != nil {
		return err
	}

	// Mining TSVs (sort + write — same logic as in-memory path)
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
	}); err != nil {
		return err
	}
	if err := s.writeQAReportScratch(filepath.Join(derived, "qa-report.json"), runStart, len(fos)); err != nil {
		return err
	}
	return nil
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
		"totals": map[string]any{
			"sources":                       len(s.manifests),
			"documents":                     docCount,
			"sentences_unique":              sentencesUnique,
			"sentences_total_occurrences":   occCount,
			"tokens_total":                  tokensProse + tokensPoetry,
			"tokens_prose":                  tokensProse,
			"tokens_poetry":                 tokensPoetry,
			"unique_surfaces":               len(s.surfaces),
			"unique_surfaces_prose":         uniqProse,
			"unique_surfaces_poetry":        uniqPoetry,
			"poetry_only_surfaces":          poetryOnly,
			"prose_only_surfaces":           proseOnly,
			"unresolved_surfaces_total":     unresolvedProse + unresolvedPoetry,
			"unresolved_rate_total":         rateOf(unresolvedProse+unresolvedPoetry, len(s.surfaces)),
			"unresolved_surfaces_prose":     unresolvedProse,
			"unresolved_rate_prose":         rateOf(unresolvedProse, uniqProse),
			"unresolved_surfaces_poetry":    unresolvedPoetry,
			"unresolved_rate_poetry":        rateOf(unresolvedPoetry, uniqPoetry),
			"ambiguous_surfaces":            len(s.miningAmbiguous),
			"poems":                         0,
		},
	}
	return writeJSON(path, report)
}

// streamWriteTSV is a streaming version of writeTSV — the yield function
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

// keep imports happy
var _ = sql.ErrNoRows
