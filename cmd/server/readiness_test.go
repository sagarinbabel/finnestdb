package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestRequireProductionDBReadySkipsOutsideProduction(t *testing.T) {
	t.Setenv(appEnvVar, "development")
	ready, err := requireProductionDBReady(filepath.Join(t.TempDir(), "missing.db"))
	if err != nil {
		t.Fatalf("requireProductionDBReady non-production err=%v, want nil", err)
	}
	if ready != nil {
		t.Fatalf("readiness=%+v, want nil outside production", ready)
	}
}

func TestRequireProductionDBReadyAllowsExplicitDegradedProduction(t *testing.T) {
	t.Setenv(appEnvVar, "production")
	t.Setenv(allowDegradedDBEnvVar, "1")
	ready, err := requireProductionDBReady(filepath.Join(t.TempDir(), "missing.db"))
	if err != nil {
		t.Fatalf("requireProductionDBReady degraded production err=%v, want nil", err)
	}
	if ready != nil {
		t.Fatalf("readiness=%+v, want nil when degraded DB is explicitly allowed", ready)
	}
}

func TestCheckDictionaryDBReadyRejectsMissingEmptyAndStubDB(t *testing.T) {
	reqs := []dictionaryRequirement{{Lang: "FI", MinForms: 1}, {Lang: "ET", MinForms: 1}}

	missing := filepath.Join(t.TempDir(), "missing.db")
	if _, err := checkDictionaryDBReady(missing, reqs); err == nil || !strings.Contains(err.Error(), "required in production") {
		t.Fatalf("missing DB err=%v, want production required error", err)
	}

	empty := filepath.Join(t.TempDir(), "empty.db")
	if err := touch(empty); err != nil {
		t.Fatal(err)
	}
	if _, err := checkDictionaryDBReady(empty, reqs); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty DB err=%v, want empty error", err)
	}

	stub := filepath.Join(t.TempDir(), "stub.db")
	withSQLite(t, stub, `CREATE TABLE users (id INTEGER PRIMARY KEY);`)
	if _, err := checkDictionaryDBReady(stub, reqs); err == nil || !strings.Contains(err.Error(), "missing forms table") {
		t.Fatalf("stub DB err=%v, want missing forms table error", err)
	}
}

func TestCheckDictionaryDBReadyRequiresBothLanguageCounts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "dict.db")
	withSQLite(t, dbPath, `
		CREATE TABLE forms (form TEXT, lemma TEXT, pos TEXT, lang TEXT);
		INSERT INTO forms (form, lemma, pos, lang) VALUES ('kissa', 'kissa', 'NOUN', 'FI');
	`)

	reqs := []dictionaryRequirement{{Lang: "FI", MinForms: 1}, {Lang: "ET", MinForms: 1}}
	_, err := checkDictionaryDBReady(dbPath, reqs)
	if err == nil || !strings.Contains(err.Error(), "0 ET forms") {
		t.Fatalf("missing ET forms err=%v, want ET count error", err)
	}
}

func TestRequireProductionDBReadyUsesConfiguredMinimums(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "dict.db")
	withSQLite(t, dbPath, `
		CREATE TABLE forms (form TEXT, lemma TEXT, pos TEXT, lang TEXT);
		INSERT INTO forms (form, lemma, pos, lang) VALUES ('kissa', 'kissa', 'NOUN', 'FI');
		INSERT INTO forms (form, lemma, pos, lang) VALUES ('koer', 'koer', 'NOUN', 'ET');
	`)

	t.Setenv(appEnvVar, "production")
	t.Setenv(minFormsFIEnvVar, "1")
	t.Setenv(minFormsETEnvVar, "1")
	ready, err := requireProductionDBReady(dbPath)
	if err != nil {
		t.Fatalf("requireProductionDBReady with configured minimums: %v", err)
	}
	if ready == nil || ready.Counts["FI"] != 1 || ready.Counts["ET"] != 1 {
		t.Fatalf("readiness counts=%+v, want FI=1 ET=1", ready)
	}
}

func TestDictionaryFormsCountUsesReadinessCounts(t *testing.T) {
	ready := &dictionaryReadiness{Counts: map[string]int64{"FI": 123}}
	count, err := dictionaryFormsCount(nil, ready, "FI")
	if err != nil {
		t.Fatalf("dictionaryFormsCount: %v", err)
	}
	if count != 123 {
		t.Fatalf("count=%d want 123", count)
	}
}

func TestProductionDictionaryRequirementsRejectBadEnv(t *testing.T) {
	t.Setenv(minFormsFIEnvVar, "not-a-number")
	if _, err := productionDictionaryRequirements(); err == nil || !strings.Contains(err.Error(), minFormsFIEnvVar) {
		t.Fatalf("productionDictionaryRequirements err=%v, want %s parse error", err, minFormsFIEnvVar)
	}
}

func touch(path string) error {
	return os.WriteFile(path, nil, 0o644)
}

func withSQLite(t *testing.T, path string, sqlText string) {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(sqlText); err != nil {
		t.Fatalf("seed sqlite: %v", err)
	}
}
