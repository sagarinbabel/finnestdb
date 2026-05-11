package main

import (
	"database/sql"
	"fmt"
	"strings"

	"finnestdb/corpus_pipeline/internal/textfilter"
)

// scratch_io.go: SQLite I/O for the aggregator's streaming mode.
// When state.scratch != nil, sentences/occurrences/documents/wordlist
// flow through SQLite instead of in-memory maps/slices.

// flushSentencesToScratch persists the current in-memory sentences,
// occurrences, and documents to the scratch DB, then clears the
// in-memory copies. Surfaces stay in memory (compact, hot in phase 2).
func (s *state) flushSentencesToScratch() error {
	if s.scratch == nil {
		return nil
	}
	tx, err := s.scratch.Begin()
	if err != nil {
		return fmt.Errorf("begin scratch tx: %w", err)
	}
	defer tx.Rollback()

	// Sentences: INSERT OR IGNORE. We track RowsAffected per insert so we
	// can update the user-friendly-sentences byte estimate ONLY for rows
	// that were actually new (i.e. globally unique across all previously-
	// flushed sources). Phase 1 reads this estimate to decide when to stop
	// ingesting additional sources.
	stmtSent, err := tx.Prepare(`INSERT OR IGNORE INTO tmp_sentences(hash, text) VALUES(?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare sentences: %w", err)
	}
	defer stmtSent.Close()
	for _, hash := range s.sentenceOrder {
		rec := s.sentences[hash]
		res, err := stmtSent.Exec(rec.hash, rec.text)
		if err != nil {
			return fmt.Errorf("insert sentence: %w", err)
		}
		// RowsAffected==1 → the INSERT actually wrote. RowsAffected==0
		// means a duplicate hash already exists in scratch and the row
		// was ignored. Only count newly-inserted rows toward the
		// budget estimate.
		if n, err := res.RowsAffected(); err == nil && n == 1 {
			if textfilter.IsUserFriendlySentence(rec.text) {
				// Two-column TSV row: id\ttext\n + 4-byte slack.
				// id digits ≤ 12 for any plausible corpus.
				s.ufSentencesBytesEstimate += int64(len(rec.text)) + 12 + 4
			}
		}
	}

	// Occurrences: append
	stmtOcc, err := tx.Prepare(`INSERT INTO tmp_sentence_occurrences(sentence_hash, source, document_id, sentence_ix, quality_flags) VALUES(?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare occurrences: %w", err)
	}
	defer stmtOcc.Close()
	for _, occ := range s.occurrences {
		if _, err := stmtOcc.Exec(occ.hash, occ.source, occ.documentID, occ.sentenceIx, occ.qualityFlags); err != nil {
			return fmt.Errorf("insert occurrence: %w", err)
		}
	}

	// Documents: INSERT OR REPLACE
	stmtDoc, err := tx.Prepare(`INSERT OR REPLACE INTO tmp_documents(document_id, source, title, author, raw_path) VALUES(?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare documents: %w", err)
	}
	defer stmtDoc.Close()
	for _, docID := range s.docOrder {
		d := s.documents[docID]
		if _, err := stmtDoc.Exec(d.id, d.source, d.title, d.author, d.rawPath); err != nil {
			return fmt.Errorf("insert document: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Clear in-memory now that scratch has it.
	s.sentences = map[string]*sentenceRec{}
	s.sentenceOrder = nil
	s.occurrences = nil
	// Documents stay in-memory too (small, accumulating). Clearing them
	// would force phase 4 to re-read from SQL; cheap to keep.
	return nil
}

// scratchInsertWordlistRow inserts one wordlist row to scratch (used by
// phase 2 when scratch is active).
func (s *state) scratchInsertWordlistRow(tx *sql.Tx, r wordlistRow) error {
	_, err := tx.Exec(`INSERT INTO tmp_wordlist(surface, lemma, pos, feats, analysis_sources, analysis_rank, is_parser_choice) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		r.surface, r.lemma, r.pos, r.feats,
		strings.Join(r.analysisSources, ";"),
		r.analysisRank,
		boolStrInt(r.isParserChoice),
	)
	return err
}

func boolStrInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
