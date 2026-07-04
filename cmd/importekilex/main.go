// cmd/importekilex imports a compact Ekilex public word snapshot into the
// finnestdb SQLite dictionary tables.
//
// The input is JSONL produced from Ekilex /api/public_word/{datasetCode}, with
// one object per word:
//
//	{"word_id":183007,"lemma":"koer","lang":"ET","source_dataset":"eki","morph_exists":true}
//
// Ekilex public_word does not include POS, definitions, or full paradigms. This
// importer therefore adds only missing direct headword lookups with POS "X" and
// an empty gloss. Existing dictionary rows are preserved.
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type publicWord struct {
	WordID        int64  `json:"word_id"`
	Lemma         string `json:"lemma"`
	Lang          string `json:"lang"`
	SourceDataset string `json:"source_dataset"`
	MorphExists   bool   `json:"morph_exists"`
}

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	filePath := flag.String("file", "", "Path to Ekilex public-word JSONL snapshot")
	source := flag.String("source", "https://ekilex.ee/api/public_word/eki", "Source identifier for dict_metadata")
	flag.Parse()

	if *filePath == "" {
		log.Fatal("-file is required")
	}

	db, err := sql.Open("sqlite3", *dbPath+"?_busy_timeout=5000")
	if err != nil {
		log.Fatalf("open DB: %v", err)
	}
	defer db.Close()

	if err := ensureSchema(db); err != nil {
		log.Fatalf("schema: %v", err)
	}

	f, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("open file: %v", err)
	}
	defer f.Close()

	imported, skipped, err := importPublicWords(db, f)
	if err != nil {
		log.Fatalf("import: %v", err)
	}

	if _, err := db.Exec(
		`INSERT OR REPLACE INTO dict_metadata (lang, source, imported_at, row_count) VALUES (?, ?, CURRENT_TIMESTAMP, ?)`,
		"ET", *source, imported,
	); err != nil {
		log.Printf("warn: could not update dict_metadata: %v", err)
	}

	fmt.Printf("Done. Imported %d Ekilex public headwords (%d skipped).\n", imported, skipped)
}

func importPublicWords(db *sql.DB, f *os.File) (int, int, error) {
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return 0, 0, err
	}
	if _, err := db.Exec(`PRAGMA synchronous=NORMAL`); err != nil {
		return 0, 0, err
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	stmtLemma, err := tx.Prepare(
		`INSERT OR IGNORE INTO lemmas (lemma, pos, gloss, lang) VALUES (?, 'X', '', ?)`,
	)
	if err != nil {
		return 0, 0, err
	}
	defer stmtLemma.Close()

	stmtForm, err := tx.Prepare(
		`INSERT OR IGNORE INTO forms (form, lemma, pos, lang) VALUES (?, ?, 'X', ?)`,
	)
	if err != nil {
		return 0, 0, err
	}
	defer stmtForm.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	imported := 0
	skipped := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var word publicWord
		if err := json.Unmarshal([]byte(line), &word); err != nil {
			skipped++
			continue
		}
		word.Lemma = strings.TrimSpace(word.Lemma)
		word.Lang = strings.ToUpper(strings.TrimSpace(word.Lang))
		if word.Lemma == "" || word.Lang != "ET" {
			skipped++
			continue
		}

		if _, err := stmtLemma.Exec(word.Lemma, word.Lang); err != nil {
			return imported, skipped, err
		}
		if _, err := stmtForm.Exec(word.Lemma, word.Lemma, word.Lang); err != nil {
			return imported, skipped, err
		}
		imported++
	}
	if err := scanner.Err(); err != nil {
		return imported, skipped, err
	}
	if err := tx.Commit(); err != nil {
		return imported, skipped, err
	}
	return imported, skipped, nil
}

func ensureSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS lemmas (
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			gloss TEXT,
			lang  TEXT NOT NULL,
			PRIMARY KEY(lemma, pos, lang)
		);
		CREATE TABLE IF NOT EXISTS forms (
			form  TEXT NOT NULL,
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			lang  TEXT NOT NULL,
			PRIMARY KEY (form, lang)
		);
		CREATE TABLE IF NOT EXISTS dict_metadata (
			lang        TEXT NOT NULL,
			source      TEXT NOT NULL,
			imported_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			row_count   INTEGER,
			PRIMARY KEY (lang, source)
		);
	`)
	return err
}
