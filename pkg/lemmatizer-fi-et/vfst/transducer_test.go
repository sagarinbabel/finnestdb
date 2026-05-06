package vfst

import (
	"strings"
	"testing"
)

const morVfstPath = "../data/fi/mor.vfst"

// loadFi opens the vendored Finnish morphology FST. Test helper.
func loadFi(t *testing.T) *Transducer {
	t.Helper()
	tr, err := Open(morVfstPath)
	if err != nil {
		t.Fatalf("open %s: %v", morVfstPath, err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	return tr
}

func TestOpen_ParsesHeaderAndSymbolTable(t *testing.T) {
	tr := loadFi(t)
	if tr.firstNormalChar == 0 {
		t.Fatal("firstNormalChar is 0; symbol table did not parse flag-diacritic region")
	}
	if tr.firstMultiChar == 0 {
		t.Fatal("firstMultiChar is 0; symbol table did not detect any multi-char tag (looking for '[...]' entries)")
	}
	if tr.firstNormalChar >= tr.firstMultiChar {
		t.Fatalf("expected firstNormalChar (%d) < firstMultiChar (%d)", tr.firstNormalChar, tr.firstMultiChar)
	}
	if tr.flagDiacriticFeatureCount == 0 {
		t.Fatal("flagDiacriticFeatureCount is 0; no flag features parsed")
	}
	if tr.transitionStart == 0 {
		t.Fatal("transitionStart is 0; symbol table parsing didn't advance the cursor")
	}
	// The Finnish FST should have a Latin letter and the morphology bracket
	// vocabulary at minimum.
	if _, ok := tr.stringToSymbol['a']; !ok {
		t.Error("expected 'a' to be in the input alphabet")
	}
	if _, ok := tr.stringToSymbol['ä']; !ok {
		t.Error("expected 'ä' (front vowel) to be in the input alphabet")
	}
}

// Smoke-test that known Finnish surface forms produce at least one
// analysis containing the expected lemma. Exact analysis tag strings
// are an implementation detail of the FST so we check substrings only.
func TestAnalyze_KnownSurfaceForms(t *testing.T) {
	tr := loadFi(t)

	cases := []struct {
		surface  string
		wantLemma string
	}{
		{"talo", "talo"},          // basic noun
		{"talon", "talo"},          // genitive
		{"talossa", "talo"},        // inessive
		{"taloja", "talo"},         // partitive plural (vowel harmony 'a')
		{"käden", "käsi"},          // consonant gradation t→d (käsi → käden)
		{"naisen", "nainen"},       // -nen class genitive
		{"naiset", "nainen"},       // -nen class plural
		{"miestä", "mies"},         // mies + partitive
		{"olen", "olla"},           // verb 'olla', 1sg present
		{"kysyn", "kysyä"},         // verb 'kysyä', 1sg present
		{"hyvää", "hyvä"},          // adjective + partitive
	}

	for _, tc := range cases {
		t.Run(tc.surface, func(t *testing.T) {
			analyses := tr.Analyze(tc.surface)
			if len(analyses) == 0 {
				t.Fatalf("Analyze(%q) returned no analyses", tc.surface)
			}
			matched := false
			for _, a := range analyses {
				if strings.Contains(a, tc.wantLemma) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("Analyze(%q) produced %d analyses, none containing lemma %q.\nFirst analysis: %s",
					tc.surface, len(analyses), tc.wantLemma, analyses[0])
			}
		})
	}
}

func TestAnalyze_UnknownInputReturnsEmpty(t *testing.T) {
	tr := loadFi(t)
	if got := tr.Analyze("xyzzy123"); len(got) != 0 {
		t.Errorf("expected no analyses for non-Finnish input, got %d: %v", len(got), got)
	}
}

func TestAnalyze_EmptyInput(t *testing.T) {
	tr := loadFi(t)
	// Empty input: behavior is implementation-defined. We just want it not
	// to panic. Either no analyses or one degenerate analysis is fine.
	_ = tr.Analyze("")
}
