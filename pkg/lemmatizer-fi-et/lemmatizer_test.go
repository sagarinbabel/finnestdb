package lemmatizer

import "testing"

func TestNewAndLemmatize_Finnish(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

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
	l, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	cases := []struct {
		surface  string
		wantLem  string
		wantPOS  string
	}{
		{"maja", "maja", "NOUN"},                    // house
		{"majas", "maja", "NOUN"},                   // house+inessive
		{"lapse", "laps", "NOUN"},                    // child+genitive (consonant gradation ps→ps)
		{"kooli", "kool", "NOUN"},                    // school+genitive (multiple readings)
		{"oli", "olema", "VERB"},                     // was (3sg past of olema)
		{"teen", "tegema", "VERB"},                   // I do (1sg present)
		{"tegin", "tegema", "VERB"},                  // I did (1sg past)
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
	l, _ := New()
	defer l.Close()
	if got := l.Lemmatize("DE", "haus"); len(got) != 0 {
		t.Errorf("expected empty for unsupported lang, got %d analyses", len(got))
	}
}

func TestLemmatize_UnknownWord(t *testing.T) {
	l, _ := New()
	defer l.Close()
	if got := l.Lemmatize("FI", "xyzzy123"); len(got) != 0 {
		t.Errorf("expected empty for unknown word, got %d analyses: %+v", len(got), got)
	}
}
