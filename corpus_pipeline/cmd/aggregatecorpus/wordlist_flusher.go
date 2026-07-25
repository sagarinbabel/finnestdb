package main

import (
	"database/sql"
	"fmt"
	"strings"
)

// wordlistFlusher batches inserts into the scratch DB's tmp_wordlist
// table so Phase 2 doesn't accumulate millions of wordlistRows in
// memory. The previous design held everything in s.wordlistRows, which
// at FI scale (~15M rows × ~200 bytes per row) was ~3 GB of pure heap on
// top of the surfaces map and the parsecore working set. Combined with
// macOS swap pressure that turned a 30-minute Phase 2 into a 5-hour
// page-thrash crawl.
//
// Strategy:
//   - Open a single transaction with a single prepared INSERT.
//   - Buffer rows in memory until either the buffer count or pending
//     byte size exceeds a threshold, then commit + open a fresh tx.
//   - Periodic commits keep WAL bounded; at default flushEvery=50k the
//     WAL stays well under 1 GB during phase 2.
//
// The flusher does not hold open the connection - caller passes in
// s.scratch and is responsible for closing it.

type wordlistFlusher struct {
	db         *sql.DB
	tx         *sql.Tx
	stmt       *sql.Stmt
	flushEvery int
	pending    int
	totalRows  int64
}

func newWordlistFlusher(db *sql.DB, flushEvery int) (*wordlistFlusher, error) {
	if flushEvery <= 0 {
		flushEvery = 50_000
	}
	w := &wordlistFlusher{db: db, flushEvery: flushEvery}
	if err := w.openTx(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *wordlistFlusher) openTx() error {
	tx, err := w.db.Begin()
	if err != nil {
		return fmt.Errorf("begin wordlist tx: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO tmp_wordlist(surface, lemma, pos, feats, analysis_sources, analysis_rank, is_parser_choice) VALUES(?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare wordlist insert: %w", err)
	}
	w.tx = tx
	w.stmt = stmt
	w.pending = 0
	return nil
}

func (w *wordlistFlusher) Add(r wordlistRow) error {
	if w.stmt == nil {
		return fmt.Errorf("wordlistFlusher: closed")
	}
	if _, err := w.stmt.Exec(
		r.surface, r.lemma, r.pos, r.feats,
		strings.Join(r.analysisSources, ";"),
		r.analysisRank,
		boolStrInt(r.isParserChoice),
	); err != nil {
		return fmt.Errorf("insert wordlist row: %w", err)
	}
	w.pending++
	w.totalRows++
	if w.pending >= w.flushEvery {
		return w.flush()
	}
	return nil
}

// flush commits the current tx and opens a fresh one. Public so phase 2
// can force a flush at end-of-loop without going through Close().
func (w *wordlistFlusher) flush() error {
	if w.stmt != nil {
		_ = w.stmt.Close()
		w.stmt = nil
	}
	if w.tx != nil {
		if err := w.tx.Commit(); err != nil {
			return fmt.Errorf("commit wordlist tx: %w", err)
		}
		w.tx = nil
	}
	return w.openTx()
}

// Close commits the final tx and releases resources. Safe to call twice.
func (w *wordlistFlusher) Close() error {
	if w.stmt != nil {
		_ = w.stmt.Close()
		w.stmt = nil
	}
	if w.tx != nil {
		err := w.tx.Commit()
		w.tx = nil
		if err != nil {
			return fmt.Errorf("commit final wordlist tx: %w", err)
		}
	}
	return nil
}

// TotalRows returns how many rows the flusher has accepted across all
// transactions. Useful for progress logging.
func (w *wordlistFlusher) TotalRows() int64 { return w.totalRows }
