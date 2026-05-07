package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestCheckDB_LegacyKaikkiRowsTriggerProvenanceWarning is the regression
// test for the original bug report: a DB whose lemmas/forms still carry
// empty source / source_priority=0 must produce a clear "provenance
// missing" warning so newcomers know to start the server (which runs the
// backfill migration) before trusting `verify-dict` output.
func TestCheckDB_LegacyKaikkiRowsTriggerProvenanceWarning(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer raw.Close()

	if _, err := raw.Exec(`
		CREATE TABLE forms (
			form TEXT NOT NULL, lemma TEXT NOT NULL, pos TEXT NOT NULL, lang TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_priority INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (form, lang, lemma, pos)
		);
		INSERT INTO forms (form, lemma, pos, lang) VALUES ('pankissa', 'pankki', 'NOUN', 'FI');
		INSERT INTO forms (form, lemma, pos, lang) VALUES ('koeras', 'koer', 'NOUN', 'ET');
		INSERT INTO forms (form, lemma, pos, lang, source, source_priority)
		    VALUES ('koera', 'koer', 'NOUN', 'ET', 'ekilex', 20);
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	main, _, warn := checkDB(dbPath)
	if main.level != levelOK {
		t.Errorf("main check: want OK, got %v (%s)", main.level, main.detail)
	}
	if warn == nil {
		t.Fatalf("expected provenance warning, got none")
	}
	if warn.level != levelWarn {
		t.Errorf("provenance warn: want WARN, got %v", warn.level)
	}
	if !strings.Contains(warn.detail, "1 FI + 1 ET") {
		t.Errorf("provenance warn detail = %q, want '1 FI + 1 ET'", warn.detail)
	}
}

// TestCheckDB_AllRowsLabeledNoWarning confirms the converse: once every
// row has real provenance, the doctor doesn't fire a false positive.
func TestCheckDB_AllRowsLabeledNoWarning(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "labeled.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer raw.Close()

	if _, err := raw.Exec(`
		CREATE TABLE forms (
			form TEXT NOT NULL, lemma TEXT NOT NULL, pos TEXT NOT NULL, lang TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_priority INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (form, lang, lemma, pos)
		);
		INSERT INTO forms (form, lemma, pos, lang, source, source_priority)
		    VALUES ('pankissa', 'pankki', 'NOUN', 'FI', 'kaikki', 10);
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, _, warn := checkDB(dbPath)
	if warn != nil {
		t.Errorf("expected no warning, got %v: %s", warn.level, warn.detail)
	}
}

func TestCheckDB_MissingDBFails(t *testing.T) {
	main, _, _ := checkDB(filepath.Join(t.TempDir(), "does-not-exist.db"))
	if main.level != levelFail {
		t.Errorf("missing DB: want FAIL, got %v", main.level)
	}
	if !strings.Contains(main.hint, "setup-local") && !strings.Contains(main.hint, "import-dict") {
		t.Errorf("missing DB hint should point to setup, got %q", main.hint)
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.txt")
	if fileExists(f) {
		t.Error("nonexistent file: want false")
	}
	if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(f) {
		t.Error("existing file: want true")
	}
	if fileExists(dir) {
		t.Error("directory passed to fileExists: want false")
	}
}

func TestDirNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if dirNonEmpty(dir) {
		t.Error("empty dir: want false")
	}
	// A hidden file shouldn't count.
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if dirNonEmpty(dir) {
		t.Error("only hidden files: want false")
	}
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !dirNonEmpty(dir) {
		t.Error("real file present: want true")
	}
}
