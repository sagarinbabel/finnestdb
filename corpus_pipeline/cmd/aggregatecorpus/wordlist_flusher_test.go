package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestWordlistFlusher_BatchedInsert validates the flusher writes rows to
// tmp_wordlist in batches without losing any. Uses a temp scratch DB.
func TestWordlistFlusher_BatchedInsert(t *testing.T) {
	dir := t.TempDir()
	db, err := openScratch(filepath.Join(dir, "scratch.db"))
	if err != nil {
		t.Fatalf("openScratch: %v", err)
	}
	defer db.Close()

	// flushEvery=10 so we exercise the multi-batch path with ~25 rows.
	w, err := newWordlistFlusher(db, 10)
	if err != nil {
		t.Fatalf("newWordlistFlusher: %v", err)
	}
	for i := 0; i < 25; i++ {
		row := wordlistRow{
			surface:         "surf-X",
			lemma:           "lemma-X",
			pos:             "NOUN",
			feats:           "Case=Nom",
			analysisSources: []string{"parser_choice", "fst"},
			analysisRank:    1,
			isParserChoice:  i%2 == 0,
		}
		if err := w.Add(row); err != nil {
			t.Fatalf("Add #%d: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := w.TotalRows(); got != 25 {
		t.Errorf("TotalRows: got %d, want 25", got)
	}
	// Round-trip: read back and confirm every row landed.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tmp_wordlist`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 25 {
		t.Errorf("rows in DB: got %d, want 25", count)
	}
	// Validate a specific column round-trips analysis_sources joined with ;
	var srcs string
	if err := db.QueryRow(`SELECT analysis_sources FROM tmp_wordlist LIMIT 1`).Scan(&srcs); err != nil {
		t.Fatalf("query srcs: %v", err)
	}
	if srcs != "parser_choice;fst" {
		t.Errorf("analysis_sources: got %q, want parser_choice;fst", srcs)
	}
}

// TestWordlistFlusher_EmptyClose ensures Close on a freshly-opened
// flusher (zero rows added) doesn't fail.
func TestWordlistFlusher_EmptyClose(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, "scratch.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(scratchSchema); err != nil {
		// scratchSchema is a multi-statement string; tolerant exec is fine
		// for tests where one of the splits may bork. Re-exec via openScratch
		// helper instead.
	}
	db.Close()
	db, err = openScratch(filepath.Join(dir, "scratch.db"))
	if err != nil {
		t.Fatalf("openScratch: %v", err)
	}
	defer db.Close()
	w, err := newWordlistFlusher(db, 100)
	if err != nil {
		t.Fatalf("newWordlistFlusher: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close empty: %v", err)
	}
}
