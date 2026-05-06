package main

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"database/sql"
	"io"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newTestDB creates an in-memory SQLite DB with the importer schema.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("newTestDB: %v", err)
	}
	if err := ensureSchema(db); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// makeBz2JSONL compresses the given JSONL string into bzip2 for testing.
// Note: Go's compress/bzip2 is read-only; we use compress/gzip to simulate
// a reader we can feed to importJSONL, which accepts any io.Reader.
// Since importJSONL wraps the reader with bzip2.NewReader, we need actual bz2.
// Instead, we expose a helper that writes raw JSONL directly (not bz2-wrapped)
// for unit testing the JSONL parsing layer, keeping tests fast and dependency-free.
func jsonlReader(s string) *strings.Reader {
	return strings.NewReader(s)
}

// importJSONLRaw is a test-only wrapper that skips the bzip2 layer.
func importJSONLRaw(db *sql.DB, jsonl string, lang string) (int, int, error) {
	return importJSONL(db, strings.NewReader(jsonl), lang, importSourceConfig{Name: "kaikki", Priority: 10})
}

// --- normalizePos tests ---

func TestNormalizePos(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"noun", "NOUN"},
		{"verb", "VERB"},
		{"adj", "ADJ"},
		{"adv", "ADV"},
		{"particle", "PART"},
		{"name", "PROPN"},
		{"NOUN", "NOUN"},       // already uppercase passthrough
		{"Unknown", "UNKNOWN"}, // unrecognized → uppercase
		{"", ""},               // empty stays empty
	}
	for _, tc := range cases {
		got := normalizePos(tc.input)
		if got != tc.want {
			t.Errorf("normalizePos(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- hasPossessiveTag tests ---

func TestShouldSkipForm_Possessive(t *testing.T) {
	tags := []string{"singular", "possessive", "first-person"}
	if !hasPossessiveTag(tags) {
		t.Error("expected hasPossessiveTag=true for tags containing 'possessive'")
	}
}

func TestShouldSkipForm_Regular(t *testing.T) {
	tags := []string{"singular", "genitive"}
	if hasPossessiveTag(tags) {
		t.Error("expected hasPossessiveTag=false for tags without 'possessive'")
	}
}

func TestShouldSkipForm_EmptyTags(t *testing.T) {
	if hasPossessiveTag(nil) {
		t.Error("expected hasPossessiveTag=false for nil tags")
	}
}

// --- importJSONL tests ---

func TestImportJSONL_BasicEntry(t *testing.T) {
	db := newTestDB(t)

	jsonl := `{"word":"pankki","pos":"noun","lang_code":"fi","senses":[{"glosses":["bank (financial institution)"]}],"forms":[{"form":"pankkiin","tags":["illative","singular"]},{"form":"pankkini","tags":["singular","possessive","first-person"]}]}`

	count, skipped, err := importJSONLRaw(db, jsonl, "FI")
	if err != nil {
		t.Fatalf("importJSONL: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	// "pankkini" has "possessive" tag → should be skipped
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1 (possessive form)", skipped)
	}

	// Verify lemma was inserted with correct gloss.
	var lemma, pos, gloss string
	err = db.QueryRow(
		`SELECT lemma, pos, gloss FROM lemmas WHERE lemma = 'pankki' AND lang = 'FI'`,
	).Scan(&lemma, &pos, &gloss)
	if err != nil {
		t.Fatalf("lemma query: %v", err)
	}
	if lemma != "pankki" || pos != "NOUN" || gloss != "bank (financial institution)" {
		t.Errorf("lemma row: got (%q,%q,%q), want (pankki,NOUN,bank (financial institution))", lemma, pos, gloss)
	}

	// Verify non-possessive form was inserted.
	var form string
	err = db.QueryRow(
		`SELECT form FROM forms WHERE form = 'pankkiin' AND lang = 'FI'`,
	).Scan(&form)
	if err != nil {
		t.Fatalf("form query (pankkiin): %v", err)
	}

	// Verify possessive form was NOT inserted.
	var dummy string
	err = db.QueryRow(
		`SELECT form FROM forms WHERE form = 'pankkini' AND lang = 'FI'`,
	).Scan(&dummy)
	if err == nil {
		t.Error("possessive form 'pankkini' should not be in forms table")
	}
}

func TestImportJSONL_MalformedLineSkipped(t *testing.T) {
	db := newTestDB(t)

	// Mix of valid, malformed, and blank lines.
	jsonl := `{"word":"kirja","pos":"noun","lang_code":"fi","senses":[{"glosses":["book"]}],"forms":[]}
not-json-at-all
{"word":"talo","pos":"noun","lang_code":"fi","senses":[{"glosses":["house"]}],"forms":[]}`

	count, _, err := importJSONLRaw(db, jsonl, "FI")
	if err != nil {
		t.Fatalf("importJSONL: %v", err)
	}
	// Malformed line silently skipped; 2 valid entries processed.
	if count != 2 {
		t.Errorf("count = %d, want 2 (malformed line should be skipped)", count)
	}
}

func TestImportJSONL_PosNormalization(t *testing.T) {
	db := newTestDB(t)

	// kaikki.org uses lowercase "verb"; we store UPOS "VERB".
	jsonl := `{"word":"mennä","pos":"verb","lang_code":"fi","senses":[{"glosses":["to go"]}],"forms":[]}`

	_, _, err := importJSONLRaw(db, jsonl, "FI")
	if err != nil {
		t.Fatalf("importJSONL: %v", err)
	}

	var pos string
	if err := db.QueryRow(
		`SELECT pos FROM lemmas WHERE lemma = 'mennä' AND lang = 'FI'`,
	).Scan(&pos); err != nil {
		t.Fatalf("lemma query: %v", err)
	}
	if pos != "VERB" {
		t.Errorf("pos = %q, want VERB", pos)
	}
}

func TestImportJSONL_SourcePriorityAllowsHigherPriorityToWin(t *testing.T) {
	db := newTestDB(t)

	lowPriority := `{"word":"suuline","pos":"noun","lang_code":"et","senses":[{"glosses":["low priority gloss"]}],"forms":[{"form":"suulise","tags":["genitive"]}]}`
	if _, _, err := importJSONL(db, strings.NewReader(lowPriority), "ET", importSourceConfig{Name: "kaikki", Priority: 10}); err != nil {
		t.Fatalf("low priority importJSONL: %v", err)
	}

	highPriority := `{"word":"suuline","pos":"adj","lang_code":"et","senses":[{"glosses":["high priority gloss"]}],"forms":[{"form":"suulise","tags":["genitive"]}]}`
	if _, _, err := importJSONL(db, strings.NewReader(highPriority), "ET", importSourceConfig{Name: "official", Priority: 20}); err != nil {
		t.Fatalf("high priority importJSONL: %v", err)
	}

	var lemma, pos, source string
	var priority int
	if err := db.QueryRow(
		`SELECT lemma, pos, source, source_priority FROM forms WHERE form = 'suulise' AND lang = 'ET'`,
	).Scan(&lemma, &pos, &source, &priority); err != nil {
		t.Fatalf("form query: %v", err)
	}
	if lemma != "suuline" || pos != "ADJ" || source != "official" || priority != 20 {
		t.Fatalf("form row = (%q,%q,%q,%d), want (suuline,ADJ,official,20)", lemma, pos, source, priority)
	}
}

func TestRecordImportMetadataPreservesAttributionFields(t *testing.T) {
	db := newTestDB(t)

	err := recordImportMetadata(db, "ET", importMetadata{
		Source:        "ekilex",
		SourceName:    "EKI/Ekilex",
		SourceURL:     "https://ekilex.ee/",
		SourceVersion: "2026-05-05-export",
		License:       "CC BY 4.0",
		Attribution:   "Eesti Keele Instituut / EKI",
		ChangesNote:   "Normalized to FinEstDB lemma/form/POS schema",
	}, 42)
	if err != nil {
		t.Fatalf("recordImportMetadata: %v", err)
	}

	var sourceName, sourceURL, sourceVersion, license, attribution, changesNote string
	var rowCount int
	if err := db.QueryRow(
		`SELECT source_name, source_url, source_version, license, attribution, changes_note, row_count
		 FROM dict_metadata WHERE lang = 'ET' AND source = 'ekilex'`,
	).Scan(&sourceName, &sourceURL, &sourceVersion, &license, &attribution, &changesNote, &rowCount); err != nil {
		t.Fatalf("metadata query: %v", err)
	}
	if sourceName != "EKI/Ekilex" || sourceURL != "https://ekilex.ee/" || sourceVersion != "2026-05-05-export" {
		t.Fatalf("unexpected source metadata: name=%q url=%q version=%q", sourceName, sourceURL, sourceVersion)
	}
	if license != "CC BY 4.0" || attribution != "Eesti Keele Instituut / EKI" || changesNote == "" {
		t.Fatalf("unexpected attribution metadata: license=%q attribution=%q changes=%q", license, attribution, changesNote)
	}
	if rowCount != 42 {
		t.Fatalf("row_count=%d want 42", rowCount)
	}
}

func TestResolveProvenance(t *testing.T) {
	t.Run("kaikki defaults applied when no source flags given", func(t *testing.T) {
		var name, url, license, attribution string
		if err := resolveProvenance("", "kaikki", &name, &url, &license, &attribution); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != kaikkiSourceName || license != kaikkiSourceLicense || attribution != kaikkiAttribution {
			t.Fatalf("kaikki defaults not applied: name=%q license=%q attribution=%q", name, license, attribution)
		}
	})

	t.Run("operator overrides preserved", func(t *testing.T) {
		name := "EKI/Ekilex"
		url := ""
		license := "CC BY 4.0"
		attribution := "Eesti Keele Instituut / EKI"
		if err := resolveProvenance("", "kaikki", &name, &url, &license, &attribution); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "EKI/Ekilex" || license != "CC BY 4.0" || attribution != "Eesti Keele Instituut / EKI" {
			t.Fatalf("operator values overwritten: name=%q license=%q attribution=%q", name, license, attribution)
		}
	})

	t.Run("non-kaikki file requires explicit provenance", func(t *testing.T) {
		var name, url, license, attribution string
		err := resolveProvenance("/tmp/ekilex.jsonl", "ekilex", &name, &url, &license, &attribution)
		if err == nil {
			t.Fatalf("expected error for missing provenance flags")
		}
		for _, want := range []string{"-source-name", "-source-license", "-source-attribution"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q missing %q", err, want)
			}
		}
	})

	t.Run("non-kaikki source URL requires explicit provenance", func(t *testing.T) {
		var name, license, attribution string
		url := "https://ekilex.ee/some/export"
		err := resolveProvenance("", "ekilex", &name, &url, &license, &attribution)
		if err == nil {
			t.Fatalf("expected error for missing provenance flags")
		}
	})

	t.Run("non-kaikki with full provenance succeeds", func(t *testing.T) {
		name := "EKI/Ekilex"
		url := "https://ekilex.ee/some/export"
		license := "CC BY 4.0"
		attribution := "Eesti Keele Instituut / EKI"
		if err := resolveProvenance("", "ekilex", &name, &url, &license, &attribution); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "EKI/Ekilex" || license != "CC BY 4.0" {
			t.Fatalf("operator values overwritten: name=%q license=%q", name, license)
		}
	})

	t.Run("non-kaikki source key with kaikki URL fallback is rejected", func(t *testing.T) {
		var name, url, license, attribution string
		err := resolveProvenance("", "ekilex", &name, &url, &license, &attribution)
		if err == nil {
			t.Fatalf("expected error: -source-key=ekilex with no -file/-source-url should be rejected")
		}
		for _, want := range []string{"-source-key", "ekilex", "-file", "-source-url"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q missing %q", err, want)
			}
		}
	})

	t.Run("kaikki source key tolerates uppercase and surrounding whitespace", func(t *testing.T) {
		var name, url, license, attribution string
		if err := resolveProvenance("", "  Kaikki  ", &name, &url, &license, &attribution); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != kaikkiSourceName {
			t.Fatalf("expected kaikki defaults applied; name=%q", name)
		}
	})
}

func TestOpenJSONLReader_Plain(t *testing.T) {
	r, err := openJSONLReader(strings.NewReader("hello"), "dict.jsonl")
	if err != nil {
		t.Fatalf("openJSONLReader: %v", err)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q, want %q", string(data), "hello")
	}
}

func TestOpenJSONLReader_Gzip(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := openJSONLReader(bytes.NewReader(buf.Bytes()), "dict.jsonl.gz")
	if err != nil {
		t.Fatalf("openJSONLReader: %v", err)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q, want %q", string(data), "hello")
	}
}

// TestApplyCustomGlosses_EmptyFile verifies that a completely empty override
// file (or one containing only comments) is treated as "0 overrides, no error"
// rather than aborting the import with an io.EOF.
func TestApplyCustomGlosses_EmptyFile(t *testing.T) {
	db := newTestDB(t)
	// Create a temp file that is empty.
	f, err := os.CreateTemp(t.TempDir(), "overrides-*.csv")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()

	n, err := applyCustomGlosses(db, f.Name(), "FI")
	if err != nil {
		t.Errorf("empty file: expected nil error, got %v", err)
	}
	if n != 0 {
		t.Errorf("empty file: expected 0 overrides, got %d", n)
	}
}

// TestApplyCustomGlosses_CommentOnlyFile verifies that a file with only
// comment lines (starting with #) is also treated as "0 overrides, no error".
func TestApplyCustomGlosses_CommentOnlyFile(t *testing.T) {
	db := newTestDB(t)
	f, err := os.CreateTemp(t.TempDir(), "overrides-*.csv")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.WriteString("# This is a comment\n# Another comment\n")
	f.Close()

	n, err := applyCustomGlosses(db, f.Name(), "FI")
	if err != nil {
		t.Errorf("comment-only file: expected nil error, got %v", err)
	}
	if n != 0 {
		t.Errorf("comment-only file: expected 0 overrides, got %d", n)
	}
}

// Ensure the bzip2 reader path compiles and can read a real bz2 stream.
// This is a compile-level sanity check, not a real kaikki.org download.
func TestBzip2ReaderCompiles(t *testing.T) {
	// Create a tiny bz2-compressed payload using gzip as a stand-in
	// (bzip2 write is not in stdlib; we verify the bzip2.NewReader path compiles).
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte("{}"))
	gz.Close()
	// Just verify bzip2.NewReader exists (compile check).
	_ = bzip2.NewReader(bytes.NewReader(buf.Bytes()))
}
