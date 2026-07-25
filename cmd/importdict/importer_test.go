package main

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"database/sql"
	"fmt"
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
		ChangesNote:   "Normalized to FinnEst lemma/form/POS schema",
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

func TestEkilexEntriesFromDetails(t *testing.T) {
	details := &ekilexWordDetails{
		WordClass: "noomen",
		Paradigms: []struct {
			Forms []struct {
				Value string `json:"value"`
				Mode  string `json:"mode"`
			} `json:"forms"`
		}{
			{Forms: []struct {
				Value string `json:"value"`
				Mode  string `json:"mode"`
			}{
				{Value: "Suuline", Mode: "WORD"},
				{Value: "suulise", Mode: "FORM"},
			}},
		},
		Lexemes: []struct {
			Words       []string `json:"words"`
			WordLang    string   `json:"wordLang"`
			DatasetCode string   `json:"datasetCode"`
			POS         []struct {
				Code  string `json:"code"`
				Value string `json:"value"`
			} `json:"pos"`
			Definitions []struct {
				Value string `json:"value"`
				Lang  string `json:"lang"`
			} `json:"definitions"`
			MeaningWords []struct {
				Value    string `json:"value"`
				Language string `json:"language"`
			} `json:"meaningWords"`
		}{
			{
				Words:    []string{"Suuline"},
				WordLang: "est",
				POS: []struct {
					Code  string `json:"code"`
					Value string `json:"value"`
				}{{Code: "adj", Value: "adjektiiv"}},
				Definitions: []struct {
					Value string `json:"value"`
					Lang  string `json:"lang"`
				}{{Value: "kõneldud kujul olev", Lang: "est"}},
				MeaningWords: []struct {
					Value    string `json:"value"`
					Language string `json:"language"`
				}{{Value: "oral", Language: "eng"}},
			},
		},
	}

	entries := ekilexEntriesFromDetails(details, "")
	if len(entries) != 1 {
		t.Fatalf("len(entries)=%d want 1", len(entries))
	}
	if entries[0].Lemma != "suuline" || entries[0].POS != "ADJ" {
		t.Fatalf("entry lemma/POS = %q/%q, want suuline/ADJ", entries[0].Lemma, entries[0].POS)
	}
	if entries[0].Gloss != "kõneldud kujul olev; eng: oral" {
		t.Fatalf("gloss=%q", entries[0].Gloss)
	}
	if strings.Join(entries[0].Forms, ",") != "suuline,suulise" {
		t.Fatalf("forms=%v", entries[0].Forms)
	}
}

func TestEkilexEntriesFromDetailsSkipsFallbackWhenOnlyNonEstonianLexemes(t *testing.T) {
	details := &ekilexWordDetails{
		WordClass: "subst",
		Lexemes: []struct {
			Words       []string `json:"words"`
			WordLang    string   `json:"wordLang"`
			DatasetCode string   `json:"datasetCode"`
			POS         []struct {
				Code  string `json:"code"`
				Value string `json:"value"`
			} `json:"pos"`
			Definitions []struct {
				Value string `json:"value"`
				Lang  string `json:"lang"`
			} `json:"definitions"`
			MeaningWords []struct {
				Value    string `json:"value"`
				Language string `json:"language"`
			} `json:"meaningWords"`
		}{
			{
				Words:    []string{"house"},
				WordLang: "eng",
			},
		},
	}

	entries := ekilexEntriesFromDetails(details, "maja")
	if len(entries) != 0 {
		t.Fatalf("len(entries)=%d want 0 for non-Estonian-only lexemes: %+v", len(entries), entries)
	}
}

func TestCollectEkilexWordRefsIgnoresGenericNestedIDs(t *testing.T) {
	raw := map[string]any{
		"results": []any{
			map[string]any{"wordId": float64(42), "value": "maja"},
			map[string]any{
				"id":    float64(9001),
				"value": "metadata row",
				"dataset": map[string]any{
					"id":    float64(123),
					"value": "eki",
				},
			},
		},
	}

	refs := collectEkilexWordRefs(raw)
	if len(refs) != 1 {
		t.Fatalf("len(refs)=%d want 1: %+v", len(refs), refs)
	}
	if refs[0].WordID != 42 || refs[0].Value != "maja" {
		t.Fatalf("refs[0]=%+v want wordId 42 value maja", refs[0])
	}
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

func TestImportJSONL_WritesTranslations(t *testing.T) {
	// Phase 2 PR 1: every sense's every gloss becomes a row in translations
	// with target_lang='EN', flat sense_idx counter, source='kaikki'.
	db := newTestDB(t)

	jsonl := `{"word":"pankki","pos":"noun","lang_code":"fi","senses":[` +
		`{"glosses":["bank (financial institution)","financial institution"]},` +
		`{"glosses":["bank (storage)"]},` +
		`{"glosses":["embankment"]}` +
		`],"forms":[]}`

	if _, _, err := importJSONLRaw(db, jsonl, "FI"); err != nil {
		t.Fatalf("importJSONL: %v", err)
	}

	rows, err := db.Query(
		`SELECT sense_idx, text, target_lang, source FROM translations
		 WHERE lemma='pankki' AND pos='NOUN' AND lang='FI' ORDER BY sense_idx`)
	if err != nil {
		t.Fatalf("query translations: %v", err)
	}
	defer rows.Close()

	var got []struct {
		idx               int
		text, target, src string
	}
	for rows.Next() {
		var r struct {
			idx               int
			text, target, src string
		}
		if err := rows.Scan(&r.idx, &r.text, &r.target, &r.src); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}

	want := []struct {
		idx               int
		text, target, src string
	}{
		{0, "bank (financial institution)", "EN", "kaikki"},
		{1, "financial institution", "EN", "kaikki"},
		{2, "bank (storage)", "EN", "kaikki"},
		{3, "embankment", "EN", "kaikki"},
	}
	if len(got) != len(want) {
		t.Fatalf("translation rows: got %d, want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("translation row %d: got %+v, want %+v", i, got[i], want[i])
		}
	}

	// lemmas.gloss should still cache the first gloss for backwards compat.
	var gloss string
	if err := db.QueryRow(
		`SELECT gloss FROM lemmas WHERE lemma='pankki' AND pos='NOUN' AND lang='FI'`,
	).Scan(&gloss); err != nil {
		t.Fatalf("query lemma gloss: %v", err)
	}
	if gloss != "bank (financial institution)" {
		t.Errorf("lemmas.gloss cache: got %q, want %q", gloss, "bank (financial institution)")
	}
}

func TestImportJSONL_TranslationsIdempotent(t *testing.T) {
	// Re-running the same import must not error or duplicate rows. The
	// translations PK (lemma, pos, lang, target_lang, sense_idx, source)
	// + INSERT OR IGNORE handles this.
	db := newTestDB(t)

	jsonl := `{"word":"talo","pos":"noun","lang_code":"fi","senses":[{"glosses":["house"]},{"glosses":["building"]}],"forms":[]}`

	if _, _, err := importJSONLRaw(db, jsonl, "FI"); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if _, _, err := importJSONLRaw(db, jsonl, "FI"); err != nil {
		t.Fatalf("second import: %v", err)
	}

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM translations WHERE lemma='talo' AND lang='FI'`,
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("translation rows after re-import: got %d, want 2 (no duplicates)", n)
	}
}

func TestImportJSONL_TranslationsSkipEmptyGlosses(t *testing.T) {
	// Whitespace-only or empty gloss strings should not produce rows.
	db := newTestDB(t)

	jsonl := `{"word":"sanaa","pos":"noun","lang_code":"fi","senses":[{"glosses":["word","",""]},{"glosses":["   "]},{"glosses":["term"]}],"forms":[]}`

	if _, _, err := importJSONLRaw(db, jsonl, "FI"); err != nil {
		t.Fatalf("importJSONL: %v", err)
	}

	rows, err := db.Query(
		`SELECT sense_idx, text FROM translations WHERE lemma='sanaa' AND lang='FI' ORDER BY sense_idx`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var idx int
		var text string
		if err := rows.Scan(&idx, &text); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, fmt.Sprintf("%d:%s", idx, text))
	}
	want := []string{"0:word", "1:term"}
	if !equalStringSliceTest(got, want) {
		t.Errorf("translations: got %v, want %v", got, want)
	}
}

// equalStringSliceTest is a small helper since the package's strings_test
// helpers aren't visible here.
func equalStringSliceTest(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestImportJSONL_TranslationsRefreshOnConflict(t *testing.T) {
	// Re-running an import after upstream gloss text changed must update
	// the `text` column, not silently keep the old text. Codex P2.
	db := newTestDB(t)

	v1 := `{"word":"talo","pos":"noun","lang_code":"fi","senses":[{"glosses":["house"]}],"forms":[]}`
	if _, _, err := importJSONLRaw(db, v1, "FI"); err != nil {
		t.Fatalf("v1 import: %v", err)
	}

	v2 := `{"word":"talo","pos":"noun","lang_code":"fi","senses":[{"glosses":["house, dwelling"]}],"forms":[]}`
	if _, _, err := importJSONLRaw(db, v2, "FI"); err != nil {
		t.Fatalf("v2 import: %v", err)
	}

	var text string
	if err := db.QueryRow(
		`SELECT text FROM translations WHERE lemma='talo' AND lang='FI' AND sense_idx=0`,
	).Scan(&text); err != nil {
		t.Fatalf("query: %v", err)
	}
	if text != "house, dwelling" {
		t.Errorf("translation text after re-import: got %q, want %q (refresh on conflict)",
			text, "house, dwelling")
	}
}

func TestReimport_ClearsTranslations(t *testing.T) {
	// -reimport must wipe the translations table for the language too,
	// so removed senses don't leave stale rows behind. Reproduces the
	// scenario from sagarinbabel's review note on the original PR head:
	// import a 2-gloss entry, run reimport, import a 1-gloss version,
	// only the 1-gloss row should remain.
	db := newTestDB(t)

	twoGloss := `{"word":"talo","pos":"noun","lang_code":"fi","senses":[{"glosses":["house"]},{"glosses":["building"]}],"forms":[]}`
	if _, _, err := importJSONLRaw(db, twoGloss, "FI"); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Simulate the -reimport cleanup the way main() does it.
	if _, err := db.Exec(`DELETE FROM forms WHERE lang = ?`, "FI"); err != nil {
		t.Fatalf("drop forms: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM lemmas WHERE lang = ?`, "FI"); err != nil {
		t.Fatalf("drop lemmas: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM translations WHERE lang = ?`, "FI"); err != nil {
		t.Fatalf("drop translations: %v", err)
	}

	oneGloss := `{"word":"talo","pos":"noun","lang_code":"fi","senses":[{"glosses":["house"]}],"forms":[]}`
	if _, _, err := importJSONLRaw(db, oneGloss, "FI"); err != nil {
		t.Fatalf("second import: %v", err)
	}

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM translations WHERE lemma='talo' AND lang='FI'`,
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("translation rows after reimport: got %d, want 1 (stale 2nd-sense row should be gone)", n)
	}

	// And the surviving text should be the new one.
	var text string
	if err := db.QueryRow(
		`SELECT text FROM translations WHERE lemma='talo' AND lang='FI' AND sense_idx=0`,
	).Scan(&text); err != nil {
		t.Fatalf("query text: %v", err)
	}
	if text != "house" {
		t.Errorf("translation text: got %q, want %q", text, "house")
	}
}

func TestImportJSONL_NormalRerunRemovesOrphanedSenses(t *testing.T) {
	// Sagar's blocking scenario on PR #85: re-running an import without
	// -reimport must drop translation rows for senses that disappeared
	// upstream. Without this, a kaikki refresh that prunes a sense
	// leaves its sense_idx row stranded; once the read path queries
	// translations, it would surface the stale gloss.
	db := newTestDB(t)

	twoGloss := `{"word":"talo","pos":"noun","lang_code":"fi","senses":[{"glosses":["house"]},{"glosses":["building"]}],"forms":[]}`
	if _, _, err := importJSONLRaw(db, twoGloss, "FI"); err != nil {
		t.Fatalf("first import: %v", err)
	}

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM translations WHERE lemma='talo' AND lang='FI'`,
	).Scan(&n); err != nil {
		t.Fatalf("count after first import: %v", err)
	}
	if n != 2 {
		t.Fatalf("after 1st import: got %d rows, want 2", n)
	}

	// Re-run with the second sense gone. NO -reimport flag.
	oneGloss := `{"word":"talo","pos":"noun","lang_code":"fi","senses":[{"glosses":["house"]}],"forms":[]}`
	if _, _, err := importJSONLRaw(db, oneGloss, "FI"); err != nil {
		t.Fatalf("second import: %v", err)
	}

	if err := db.QueryRow(
		`SELECT COUNT(*) FROM translations WHERE lemma='talo' AND lang='FI'`,
	).Scan(&n); err != nil {
		t.Fatalf("count after rerun: %v", err)
	}
	if n != 1 {
		t.Errorf("after normal rerun: got %d rows, want 1 (orphaned sense should be removed)", n)
	}

	// And the surviving row is the new one.
	var text string
	if err := db.QueryRow(
		`SELECT text FROM translations WHERE lemma='talo' AND lang='FI' AND sense_idx=0`,
	).Scan(&text); err != nil {
		t.Fatalf("query text: %v", err)
	}
	if text != "house" {
		t.Errorf("surviving text: got %q, want %q", text, "house")
	}
}

func TestImportJSONL_NormalRerunPreservesOtherSourcesTranslations(t *testing.T) {
	// The pre-import wipe is scoped by (lang, source), so a kaikki
	// rerun must NOT touch translations from other sources for the
	// same language. Guards against the wipe being too broad.
	db := newTestDB(t)

	// Pre-seed an "ekilex" row for the same headword (simulating a
	// future state where Ekilex translations are written too).
	if _, err := db.Exec(
		`INSERT INTO translations (lemma, pos, lang, target_lang, text, sense_idx, source)
		 VALUES ('talo', 'NOUN', 'FI', 'EN', 'ekilex-house', 0, 'ekilex')`,
	); err != nil {
		t.Fatalf("seed ekilex: %v", err)
	}

	jsonl := `{"word":"talo","pos":"noun","lang_code":"fi","senses":[{"glosses":["house"]}],"forms":[]}`
	if _, _, err := importJSONLRaw(db, jsonl, "FI"); err != nil {
		t.Fatalf("kaikki import: %v", err)
	}

	// kaikki row should exist.
	var kaikkiText string
	if err := db.QueryRow(
		`SELECT text FROM translations WHERE lemma='talo' AND source='kaikki' AND lang='FI'`,
	).Scan(&kaikkiText); err != nil {
		t.Fatalf("query kaikki: %v", err)
	}
	if kaikkiText != "house" {
		t.Errorf("kaikki: got %q, want %q", kaikkiText, "house")
	}

	// ekilex row must be untouched.
	var ekilexText string
	if err := db.QueryRow(
		`SELECT text FROM translations WHERE lemma='talo' AND source='ekilex' AND lang='FI'`,
	).Scan(&ekilexText); err != nil {
		t.Fatalf("query ekilex: %v", err)
	}
	if ekilexText != "ekilex-house" {
		t.Errorf("ekilex preserved: got %q, want %q (kaikki wipe should not touch other sources)",
			ekilexText, "ekilex-house")
	}
}

// errReader returns its data once then errors. Used to simulate a
// mid-stream reader failure (truncated download, network blip, etc.).
type errReader struct {
	data []byte
	pos  int
	err  error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func TestImportJSONL_StreamFailurePreservesExistingTranslations(t *testing.T) {
	// Sagar's blocking scenario on PR #85: when importJSONL's stream
	// returns an error before any batch commits, the pre-import
	// translations DELETE must roll back along with the partial
	// inserts. Otherwise a refresh that fails mid-stream leaves the
	// dictionary with no translations for this source - strictly
	// worse than the pre-import state. Mirrors the existing
	// "fail without leaving the dictionary incomplete" guarantee for
	// forms/lemmas.
	db := newTestDB(t)

	// Pre-seed a kaikki translation as the "existing" state.
	if _, err := db.Exec(
		`INSERT INTO translations (lemma, pos, lang, target_lang, text, sense_idx, source)
		 VALUES ('talo', 'NOUN', 'FI', 'EN', 'house', 0, 'kaikki')`,
	); err != nil {
		t.Fatalf("seed translation: %v", err)
	}

	var before int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM translations WHERE lang='FI' AND source='kaikki'`,
	).Scan(&before); err != nil {
		t.Fatalf("count before: %v", err)
	}
	if before != 1 {
		t.Fatalf("seed: got %d rows, want 1", before)
	}

	// Reader returns part of a JSONL line then errors mid-stream. The
	// partial line never produces a complete entry; scanner.Err()
	// returns the read error.
	streamErr := &errReader{
		data: []byte(`{"word":"some","pos":"noun","lang_code":"fi","senses"`),
		err:  io.ErrUnexpectedEOF,
	}

	_, _, err := importJSONL(db, streamErr, "FI", importSourceConfig{Name: "kaikki", Priority: 10})
	if err == nil {
		t.Fatal("expected stream error, got nil")
	}

	// The pre-existing translation must survive the failed import.
	var after int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM translations WHERE lang='FI' AND source='kaikki'`,
	).Scan(&after); err != nil {
		t.Fatalf("count after: %v", err)
	}
	if after != 1 {
		t.Errorf("after stream failure: got %d rows, want 1 (DELETE should have rolled back with the failed transaction)", after)
	}

	// And the surviving content is the original.
	var text string
	if err := db.QueryRow(
		`SELECT text FROM translations WHERE lemma='talo' AND lang='FI' AND source='kaikki'`,
	).Scan(&text); err != nil {
		t.Fatalf("query text: %v", err)
	}
	if text != "house" {
		t.Errorf("surviving text: got %q, want %q (original)", text, "house")
	}
}
