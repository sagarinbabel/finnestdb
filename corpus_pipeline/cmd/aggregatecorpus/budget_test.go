package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
)

// TestCappedTSVWriter_TabFieldEncodesCorrectly verifies the encode-then-
// measure path correctly handles fields that need csv quoting (tabs and
// newlines). The previous estimate-based path would truncate mid-row
// when the actual quoted output exceeded the estimated 4-byte slack.
func TestCappedTSVWriter_TabFieldEncodesCorrectly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.tsv")
	w, err := newCappedTSVWriter(path, []string{"id", "text"}, 0) // no budget
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Field with embedded tab — csv should quote-wrap it.
	if !w.Write([]string{"1", "hello\tworld"}) {
		t.Fatalf("write rejected unexpectedly")
	}
	if !w.Write([]string{"2", "line1\nline2"}) {
		t.Fatalf("write rejected unexpectedly")
	}
	fileBytes, _, err := w.Close()
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Size() != fileBytes {
		t.Errorf("stat %d != reported %d", st.Size(), fileBytes)
	}
	f, _ := os.Open(path)
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = 2
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("readback parse: %v — TSV malformed", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows (header + 2), got %d", len(rows))
	}
	if rows[1][1] != "hello\tworld" {
		t.Errorf("tab-bearing field round-tripped as %q", rows[1][1])
	}
	if rows[2][1] != "line1\nline2" {
		t.Errorf("newline-bearing field round-tripped as %q", rows[2][1])
	}
}

func TestParseByteSize(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"1024", 1024, false},
		{"1KB", 1000, false},
		{"1MB", 1_000_000, false},
		{"1GB", 1_000_000_000, false},
		{"6GB", 6_000_000_000, false},
		{"1KiB", 1024, false},
		{"1MiB", 1024 * 1024, false},
		{"1GiB", 1024 * 1024 * 1024, false},
		{"4GiB", 4 * 1024 * 1024 * 1024, false},
		{"  6GB  ", 6_000_000_000, false},
		{"1.5GB", 1_500_000_000, false},
		{"-1MB", 0, true},
		{"abc", 0, true},
		{"1QQ", 0, true},
	}
	for _, tc := range cases {
		got, err := parseByteSize(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseByteSize(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("parseByteSize(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestCappedTSVWriter_BudgetCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.tsv")
	// budget = 200 bytes is enough for header + a few rows
	w, err := newCappedTSVWriter(path, []string{"id", "text"}, 200)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	accepted := 0
	rejected := 0
	for i := 0; i < 1000; i++ {
		if w.Write([]string{"row", "0123456789012345"}) {
			accepted++
		} else {
			rejected++
		}
	}
	fileBytes, capHit, err := w.Close()
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if !capHit {
		t.Errorf("expected capHit=true with budget=200B and 1000 large rows")
	}
	if accepted == 0 {
		t.Errorf("expected at least some rows accepted, got 0")
	}
	// Encode-then-measure design: actual file bytes must NEVER exceed
	// the budget (no truncation, no overshoot). Each accepted row's
	// encoded size was checked exactly before commit.
	if fileBytes > 200 {
		t.Errorf("encode-then-measure cap violated: budget=200, file=%d", fileBytes)
	}
	// Sanity: file should round-trip as valid TSV (every row has 2
	// fields, no torn rows). Read it back via csv.
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Size() != fileBytes {
		t.Errorf("stat size %d != reported %d", st.Size(), fileBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = 2
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("readback parse: %v (file truncated mid-row?)", err)
	}
	// header + accepted == total rows
	if len(rows) != accepted+1 {
		t.Errorf("readback row count %d, expected %d (1 header + %d data)", len(rows), accepted+1, accepted)
	}
}

func TestCappedTSVWriter_NoBudget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.tsv")
	w, err := newCappedTSVWriter(path, []string{"id", "text"}, 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 100; i++ {
		if !w.Write([]string{"row", "data"}) {
			t.Fatalf("budget=0 should never reject; got reject at i=%d", i)
		}
	}
	_, capHit, err := w.Close()
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if capHit {
		t.Errorf("expected capHit=false with no budget")
	}
}
