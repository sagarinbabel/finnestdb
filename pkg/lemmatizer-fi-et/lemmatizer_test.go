package lemmatizer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fixturesDir resolves the hand-authored test fixtures under
// testdata/lemmatizer/ at the repo root. `go test` runs from the
// package directory, so we walk up two levels.
func fixturesDir() string {
	return filepath.Join("..", "..", "testdata", "lemmatizer")
}

func newTestLemmatizer(t *testing.T) *Lemmatizer {
	t.Helper()
	l, err := NewFromDir(fixturesDir())
	if err != nil {
		t.Fatalf("NewFromDir(%s): %v", fixturesDir(), err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func TestNewAndLemmatize_Finnish(t *testing.T) {
	l := newTestLemmatizer(t)

	cases := []struct {
		surface  string
		wantLem  string
		wantPOS  string
		wantGram string
	}{
		{"talo", "talo", "NOUN", "nominative"},
		{"talossa", "talo", "NOUN", "inessive"},
		{"taloja", "talo", "NOUN", "partitive"},
		{"käden", "käsi", "NOUN", "genitive"},
		{"naisen", "nainen", "NOUN", "genitive"},
		{"olen", "olla", "VERB", ""}, // verbs don't carry sijamuoto
		{"kysyit", "kysyä", "VERB", ""},
		{"hyvää", "hyvä", "ADJ", "partitive"},
		{"pankkiautomaatista", "pankkiautomaatti", "NOUN", "elative"},
	}

	for _, tc := range cases {
		t.Run(tc.surface, func(t *testing.T) {
			analyses := l.Lemmatize("FI", tc.surface)
			if len(analyses) == 0 {
				t.Fatalf("Lemmatize(%q): no analyses", tc.surface)
			}
			matched := false
			for _, a := range analyses {
				if a.Lemma == tc.wantLem && a.UPOS == tc.wantPOS {
					if tc.wantGram == "" || a.GrammarLabel == tc.wantGram {
						matched = true
						break
					}
				}
			}
			if !matched {
				t.Errorf("Lemmatize(%q) had no analysis matching {lemma=%q UPOS=%q grammar=%q}.\nAll analyses: %+v",
					tc.surface, tc.wantLem, tc.wantPOS, tc.wantGram, analyses)
			}
		})
	}
}

func TestNewAndLemmatize_Estonian(t *testing.T) {
	l := newTestLemmatizer(t)

	cases := []struct {
		surface string
		wantLem string
		wantPOS string
	}{
		{"maja", "maja", "NOUN"},   // house
		{"majas", "maja", "NOUN"},  // house+inessive
		{"lapse", "laps", "NOUN"},  // child+genitive (consonant gradation ps→ps)
		{"kooli", "kool", "NOUN"},  // school+genitive (multiple readings)
		{"oli", "olema", "VERB"},   // was (3sg past of olema)
		{"teen", "tegema", "VERB"}, // I do (1sg present)
		{"tegin", "tegema", "VERB"}, // I did (1sg past)
	}

	for _, tc := range cases {
		t.Run(tc.surface, func(t *testing.T) {
			analyses := l.Lemmatize("ET", tc.surface)
			if len(analyses) == 0 {
				t.Fatalf("Lemmatize(ET, %q): no analyses", tc.surface)
			}
			matched := false
			for _, a := range analyses {
				if a.Lemma == tc.wantLem && a.UPOS == tc.wantPOS {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("Lemmatize(ET, %q) had no analysis matching {lemma=%q UPOS=%q}.\nAll: %+v",
					tc.surface, tc.wantLem, tc.wantPOS, analyses)
			}
		})
	}
}

func TestLemmatize_UnknownLanguage(t *testing.T) {
	l := newTestLemmatizer(t)
	if got := l.Lemmatize("DE", "haus"); len(got) != 0 {
		t.Errorf("expected empty for unsupported lang, got %d analyses", len(got))
	}
}

func TestLemmatize_UnknownWord(t *testing.T) {
	l := newTestLemmatizer(t)
	if got := l.Lemmatize("FI", "xyzzy123"); len(got) != 0 {
		t.Errorf("expected empty for unknown word, got %d analyses: %+v", len(got), got)
	}
}

func TestNewFromDir_MissingFiles(t *testing.T) {
	_, err := NewFromDir(t.TempDir())
	if err == nil {
		t.Fatal("NewFromDir on empty dir: expected error, got nil")
	}
	if !errors.Is(err, ErrNoTables) {
		t.Errorf("expected error to wrap ErrNoTables, got %v", err)
	}
}

// TestNewFromDir_PartialCoverage covers the per-language-independent
// loader behaviour: if only fi_min.json exists, FI works and ET
// returns no analyses (no error).
func TestNewFromDir_PartialCoverage(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(fixturesDir(), "fi_min.json")
	dst := filepath.Join(dir, "fi_min.json")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatalf("write fi_min.json: %v", err)
	}
	l, err := NewFromDir(dir)
	if err != nil {
		t.Fatalf("NewFromDir: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	if got := l.Lemmatize("FI", "talo"); len(got) == 0 {
		t.Errorf("FI partial: expected analyses for 'talo', got none")
	}
	if got := l.Lemmatize("ET", "maja"); len(got) != 0 {
		t.Errorf("ET partial: expected no analyses (no et_min.json), got %d", len(got))
	}
}

// TestLemmatize_FI_LexicalOverlayShadowsFST asserts that the
// lexicalised-adverbs overlay short-circuits the FST lookup. The
// surfaces below are not present in the smoke fi_min.json fixture, so
// without the overlay Lemmatize would return an empty slice. With the
// overlay we get the curated analysis even on a degraded table.
func TestLemmatize_FI_LexicalOverlayShadowsFST(t *testing.T) {
	l := newTestLemmatizer(t)
	cases := []struct {
		surface, wantLemma, wantPOS string
	}{
		{"tuskin", "tuskin", "ADV"},
		{"Tuskin", "tuskin", "ADV"},
		{"varsin", "varsin", "ADV"},
		{"enemmän", "paljon", "ADV"},
		{"vahingossa", "vahingossa", "ADV"},
		{"asiaan", "asia", "NOUN"},
	}
	for _, tc := range cases {
		t.Run(tc.surface, func(t *testing.T) {
			analyses := l.Lemmatize("FI", tc.surface)
			if len(analyses) != 1 {
				t.Fatalf("Lemmatize(%q): want 1 overlay analysis, got %d (%+v)", tc.surface, len(analyses), analyses)
			}
			a := analyses[0]
			if a.Lemma != tc.wantLemma || a.UPOS != tc.wantPOS {
				t.Errorf("Lemmatize(%q): got lemma=%q upos=%q, want lemma=%q upos=%q",
					tc.surface, a.Lemma, a.UPOS, tc.wantLemma, tc.wantPOS)
			}
		})
	}
}

// TestLemmatize_ET_LexicalOverlayShadowsFST is the Estonian counterpart
// to TestLemmatize_FI_LexicalOverlayShadowsFST. The surfaces below
// are absent from the smoke et_min.json fixture but each one is a
// catalogued productive-case-on-closed-class trap (välja read as
// `väli`/Case=Ill, seal as `siga`/Case=Ade, etc.). With the overlay
// they resolve to the curated ADV/ADP analysis regardless of the
// underlying table.
func TestLemmatize_ET_LexicalOverlayShadowsFST(t *testing.T) {
	l := newTestLemmatizer(t)
	cases := []struct {
		surface, wantLemma, wantPOS string
	}{
		{"välja", "välja", "ADV"},
		{"Välja", "välja", "ADV"},
		{"seal", "seal", "ADV"},
		{"peale", "peale", "ADP"},
		{"jaoks", "jaoks", "ADP"},
		{"tegelikult", "tegelikult", "ADV"},
	}
	for _, tc := range cases {
		t.Run(tc.surface, func(t *testing.T) {
			analyses := l.Lemmatize("ET", tc.surface)
			if len(analyses) != 1 {
				t.Fatalf("Lemmatize(ET, %q): want 1 overlay analysis, got %d (%+v)",
					tc.surface, len(analyses), analyses)
			}
			a := analyses[0]
			if a.Lemma != tc.wantLemma || a.UPOS != tc.wantPOS {
				t.Errorf("Lemmatize(ET, %q): got lemma=%q upos=%q, want lemma=%q upos=%q",
					tc.surface, a.Lemma, a.UPOS, tc.wantLemma, tc.wantPOS)
			}
		})
	}
}

func TestResolveTablesDir_DefaultAndOverride(t *testing.T) {
	t.Setenv("LEMMATIZER_TABLES_DIR", "")
	if got := resolveTablesDir(); got != DefaultTablesDir {
		t.Errorf("default: got %q, want %q", got, DefaultTablesDir)
	}
	t.Setenv("LEMMATIZER_TABLES_DIR", "/tmp/custom")
	if got := resolveTablesDir(); got != "/tmp/custom" {
		t.Errorf("override: got %q, want /tmp/custom", got)
	}
}
