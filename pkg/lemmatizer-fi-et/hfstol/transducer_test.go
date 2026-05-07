package hfstol

import (
	"os"
	"strings"
	"testing"
)

// Local-only test path. The repository does not vendor .hfstol artifacts;
// if the file is absent, these integration tests are skipped.
const fiAnalyserPath = "../data/fi/analyser-gt-desc.hfstol"

func loadFi(t *testing.T) *Transducer {
	t.Helper()
	if _, err := os.Stat(fiAnalyserPath); err != nil {
		t.Skipf("skipping: %s not present (%v)", fiAnalyserPath, err)
	}
	tr, err := Open(fiAnalyserPath)
	if err != nil {
		t.Fatalf("open %s: %v", fiAnalyserPath, err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	return tr
}

func TestOpen_HeaderAndAlphabet(t *testing.T) {
	tr := loadFi(t)
	if tr.indexTableSize == 0 {
		t.Fatal("indexTableSize == 0; header didn't parse")
	}
	if tr.transTableSize == 0 {
		t.Fatal("transTableSize == 0; header didn't parse")
	}
	if !tr.weighted {
		t.Logf("note: FST is unweighted (lang-fin is usually built weighted)")
	}
	// Sanity: we should have a basic Latin alphabet plus Finnish vowels.
	for _, r := range []rune{'a', 'e', 'i', 'o', 'u', 'ä', 'ö'} {
		if _, ok := tr.stringToSymbol[r]; !ok {
			t.Errorf("expected rune %q in alphabet", r)
		}
	}
}

func TestAnalyze_KnownSurfaceForms(t *testing.T) {
	tr := loadFi(t)
	cases := []struct {
		surface string
		want    string // an analysis (concatenated output symbols) we expect to find
	}{
		{"talo", "talo+N+Sg+Nom"},
		{"talossa", "talo+N+Sg+Ine"},
		{"käden", "käsi+N+Sg+Gen"},
		{"naisen", "nainen+N+Sg+Gen"},
		{"miestä", "mies+N+Sg+Par"},
		{"olen", "olla+V+Act+Ind+Prs+Sg1"},
		{"kysyit", "kysyä+V+Act+Ind+Prt+Sg2"},
	}
	for _, tc := range cases {
		t.Run(tc.surface, func(t *testing.T) {
			got := tr.Analyze(tc.surface)
			if len(got) == 0 {
				t.Fatalf("Analyze(%q) returned no analyses", tc.surface)
			}
			matched := false
			for _, a := range got {
				if strings.Contains(a, tc.want) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("Analyze(%q) did not produce an analysis containing %q.\nGot: %v",
					tc.surface, tc.want, got)
			}
		})
	}
}

func TestAnalyze_UnknownInputReturnsEmpty(t *testing.T) {
	tr := loadFi(t)
	if got := tr.Analyze("xyzzy123"); len(got) != 0 {
		t.Errorf("expected no analyses, got %d: %v", len(got), got)
	}
}
