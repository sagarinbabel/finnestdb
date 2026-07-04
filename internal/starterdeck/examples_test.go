package starterdeck

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTSV(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "examples.tsv")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write tsv: %v", err)
	}
	return path
}

func TestLoadExamplesFirstPerLemmaWins(t *testing.T) {
	path := writeTSV(t, "lemma\tpos\tform\tsentence\tsource_corpus\n"+
		"olla\tVERB\ton\tTalo on suuri ja vanha.\tfi\n"+
		"olla\tVERB\toli\tKissa oli pöydän alla.\tfi\n"+
		"kissa\tNOUN\tkissan\tNäin kissan pihalla eilen.\tfi\n")
	got, err := LoadExamples(path, "FI")
	if err != nil {
		t.Fatalf("LoadExamples: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 lemmas, got %d", len(got))
	}
	olla := got[LemmaKey{Lemma: "olla", POS: "VERB"}]
	if olla.Form != "on" || olla.Sentence != "Talo on suuri ja vanha." {
		t.Fatalf("first row per lemma should win, got %+v", olla)
	}
}

func TestLoadExamplesRejectsBadHeader(t *testing.T) {
	path := writeTSV(t, "lemma\tpos\tsentence\tsource_corpus\nfoo\tNOUN\tBar.\tfi\n")
	if _, err := LoadExamples(path, "FI"); err == nil {
		t.Fatal("expected error on bad header (missing form column)")
	}
}

func TestLoadExamplesEmptyIsError(t *testing.T) {
	path := writeTSV(t, "lemma\tpos\tform\tsentence\tsource_corpus\n")
	if _, err := LoadExamples(path, "FI"); err == nil {
		t.Fatal("expected error on header-only file")
	}
}

func TestFormTokenIndex(t *testing.T) {
	cases := []struct {
		sentence string
		form     string
		want     int
	}{
		{"Kissa oli pöydän alla.", "oli", 1},
		{"Oli kerran pieni kissa.", "oli", 0}, // case-insensitive, sentence-initial
		{"Kissa istui pöydän alla.", "oli", -1},
		{"Näin kissan, joka nukkui.", "kissan", 1}, // trailing comma trimmed
		{"", "oli", -1},
	}
	for _, tc := range cases {
		if got := FormTokenIndex(tc.sentence, tc.form); got != tc.want {
			t.Errorf("FormTokenIndex(%q,%q)=%d want %d", tc.sentence, tc.form, got, tc.want)
		}
	}
}
