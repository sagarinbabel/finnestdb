package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run with `go test ./cmd/reduceekilex/ -update-golden` to refresh expected
// fixtures after intentional reducer changes. The flag is opt-in so accidental
// runs cannot mask a real regression.
var updateGolden = flag.Bool("update-golden", false, "rewrite expected.compact.json and expected.forms.tsv from current reducer output")

// TestReduceFixtures golden-compares the reducer output against checked-in
// expected files for one fixture per inflection class (plus the invariable
// "_none" bucket). Add a new directory under testdata/by_class to extend
// coverage; the test will pick it up automatically.
func TestReduceFixtures(t *testing.T) {
	classDirs, err := filepath.Glob("testdata/by_class/*/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(classDirs) == 0 {
		t.Fatal("no fixtures under testdata/by_class")
	}

	for _, dir := range classDirs {
		dir := dir
		name := strings.TrimPrefix(dir, "testdata/by_class/")
		t.Run(name, func(t *testing.T) {
			rawPath := filepath.Join(dir, "raw.json.gz")
			raw, err := readRaw(rawPath)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			compact, rows := reduce(raw)

			gotCompact := mustMarshalIndent(t, compact)
			gotForms := mustEncodeTSV(t, rows)

			compactPath := filepath.Join(dir, "expected.compact.json")
			formsPath := filepath.Join(dir, "expected.forms.tsv")

			if *updateGolden {
				writeFile(t, compactPath, gotCompact)
				writeFile(t, formsPath, gotForms)
				t.Logf("wrote %s and %s", compactPath, formsPath)
				return
			}

			wantCompact := readFile(t, compactPath)
			if !bytes.Equal(gotCompact, wantCompact) {
				t.Errorf("compact JSON differs from %s\n--- got ---\n%s\n--- want ---\n%s",
					compactPath, gotCompact, wantCompact)
			}
			wantForms := readFile(t, formsPath)
			if !bytes.Equal(gotForms, wantForms) {
				t.Errorf("forms TSV differs from %s\n--- got (first 600 bytes) ---\n%s\n--- want (first 600 bytes) ---\n%s",
					formsPath, truncateBytes(gotForms, 600), truncateBytes(wantForms, 600))
			}
		})
	}
}

// TestEveryFixtureProducesForms catches the regression where a reducer
// change accidentally drops paradigm forms for a class. Invariable words
// ("_none") legitimately have no forms and are exempt.
func TestEveryFixtureProducesForms(t *testing.T) {
	classDirs, err := filepath.Glob("testdata/by_class/*/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range classDirs {
		raw, err := readRaw(filepath.Join(dir, "raw.json.gz"))
		if err != nil {
			t.Fatal(err)
		}
		_, rows := reduce(raw)
		isInvariable := strings.HasPrefix(strings.TrimPrefix(dir, "testdata/by_class/"), "_none/") ||
			strings.HasPrefix(strings.TrimPrefix(dir, "testdata/by_class/"), "41/")
		if !isInvariable && len(rows) == 0 {
			t.Errorf("%s: expected forms for non-invariable fixture, got 0", dir)
		}
		// Every form must have all three components populated; this is the
		// invariant the parser's forms table relies on.
		for i, r := range rows {
			if r.Lemma == "" || r.Form == "" || r.MorphCode == "" {
				t.Errorf("%s: row %d incomplete: %+v", dir, i, r)
			}
		}
	}
}

// helpers --------------------------------------------------------------------

func mustMarshalIndent(t *testing.T, v any) []byte {
	t.Helper()
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	buf = append(buf, '\n')
	return buf
}

func mustEncodeTSV(t *testing.T, rows []formRow) []byte {
	t.Helper()
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Comma = '\t'
	if err := w.Write([]string{"lemma", "form", "morph_code"}); err != nil {
		t.Fatalf("tsv header: %v", err)
	}
	for _, r := range rows {
		if err := w.Write([]string{r.Lemma, r.Form, r.MorphCode}); err != nil {
			t.Fatalf("tsv row: %v", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("tsv flush: %v", err)
	}
	return b.Bytes()
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v\n(run `go test ./cmd/reduceekilex/ -update-golden` to create)", path, err)
	}
	return data
}

func truncateBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return append(append([]byte{}, b[:n]...), '.', '.', '.')
}
