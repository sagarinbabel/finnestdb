// Package cli holds shared startup helpers for every corpus-pipeline command.
//
// Per Decisions #20-23 of the plan: data-root resolution, language
// normalization at the boundary, lemmatizer-tables-dir env wiring, and
// dictionary DB preflight all happen here so each command does it the same
// way and we don't drift.
package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/internal/store"
)

// Roots holds the resolved absolute paths a command operates against.
type Roots struct {
	DataRoot  string // e.g. /Users/sagar/Downloads/projects/finnestdb/localdata
	RepoRoot  string // dataRoot/..  (main finnestdb repo root)
	TablesDir string // dataRoot/lemmatizer-fi-et/tables
	DBPath    string // repoRoot/finnestdb.db (overridable)
}

// Resolve takes a -data-root flag value and a -db flag value and produces
// absolute, clean paths. Sets LEMMATIZER_TABLES_DIR so parsecore + store +
// lemmatizer pick up the right tables (Decision #20).
func Resolve(dataRoot, dbPath string) (Roots, error) {
	if dataRoot == "" {
		dataRoot = "../localdata"
	}
	abs, err := filepath.Abs(dataRoot)
	if err != nil {
		return Roots{}, fmt.Errorf("resolve data-root: %w", err)
	}
	clean := filepath.Clean(abs)
	repoRoot := filepath.Clean(filepath.Join(clean, ".."))
	tables := filepath.Join(clean, "lemmatizer-fi-et", "tables")
	if dbPath == "" {
		dbPath = filepath.Join(repoRoot, "finnestdb.db")
	}
	dbAbs, err := filepath.Abs(dbPath)
	if err != nil {
		return Roots{}, fmt.Errorf("resolve db path: %w", err)
	}
	if err := os.Setenv("LEMMATIZER_TABLES_DIR", tables); err != nil {
		return Roots{}, fmt.Errorf("set LEMMATIZER_TABLES_DIR: %w", err)
	}
	return Roots{
		DataRoot:  clean,
		RepoRoot:  repoRoot,
		TablesDir: tables,
		DBPath:    dbAbs,
	}, nil
}

// LangCodes returns the lowercase + uppercase pair from a user-supplied
// -lang flag. Returns an error for anything other than fi/et.
func LangCodes(lang string) (lower, upper string, err error) {
	lower = strings.ToLower(strings.TrimSpace(lang))
	switch lower {
	case "fi", "et":
		return lower, strings.ToUpper(lower), nil
	default:
		return "", "", fmt.Errorf("unsupported lang %q (want fi or et)", lang)
	}
}

// PreflightTables asserts the FST table files exist and are non-empty.
// Decision #20 hard preflight.
func PreflightTables(tablesDir string) error {
	for _, fname := range []string{"fi_min.json", "et_min.json"} {
		p := filepath.Join(tablesDir, fname)
		fi, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("preflight.fst.missing: %s: %w", p, err)
		}
		if fi.Size() == 0 {
			return fmt.Errorf("preflight.fst.empty: %s", p)
		}
	}
	return nil
}

// PreflightDB asserts the dictionary DB exists, is non-empty, has the
// required tables, and has rows for the target language.
// Decision #23 hard preflight (real schema: forms, lemmas, dict_metadata).
func PreflightDB(dbPath, langUpper string) error {
	fi, err := os.Stat(dbPath)
	if err != nil {
		return fmt.Errorf("preflight.dict.missing_or_empty: %s: %w", dbPath, err)
	}
	if fi.Size() == 0 {
		return fmt.Errorf("preflight.dict.missing_or_empty: empty file: %s", dbPath)
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("preflight.dict.open: %w", err)
	}
	defer db.Close()
	for _, table := range []string{"forms", "lemmas", "dict_metadata"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("preflight.dict.missing_or_empty: table %q absent in %s", table, dbPath)
			}
			return fmt.Errorf("preflight.dict.schema_query: %w", err)
		}
	}
	for _, table := range []string{"forms", "lemmas"} {
		var n int
		if err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE lang = ?`, table), langUpper).Scan(&n); err != nil {
			return fmt.Errorf("preflight.dict.count_%s: %w", table, err)
		}
		if n == 0 {
			return fmt.Errorf("preflight.dict.missing_or_empty: %s has 0 rows for lang=%s", table, langUpper)
		}
	}
	return nil
}

// OpenDB opens the dictionary store after preflight passes. Returns the
// store.DB ready for parsecore.Analyze calls.
func OpenDB(dbPath string) (*store.DB, error) {
	return store.NewDB(dbPath)
}
