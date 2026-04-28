package parsecore

import (
	"slices"
	"testing"

	"finnestdb/internal/store"
)

func TestSupportedParsersIncludesOmorfi(t *testing.T) {
	got := SupportedParsers()
	want := []string{"basic", "custom", "omorfi"}
	if !slices.Equal(got, want) {
		t.Fatalf("supported parsers=%v want %v", got, want)
	}
}

func TestOmorfiRulesPreferCustomFallbackAndTraceIt(t *testing.T) {
	token := &TokenResult{
		Form:     "kirjassani",
		Lemma:    "kirjassani",
		POS:      "X",
		Resolved: false,
		Source:   "analyzer:omorfi",
	}
	direct := store.FormResolution{}
	custom := store.FormResolution{
		Lemma:        "kirja",
		POS:          "NOUN",
		GrammarLabel: "inessive",
		Source:       "possessive",
	}

	applied := defaultOmorfiRules()[1].Apply("FI", token, direct, custom)
	if !applied {
		t.Fatal("expected custom fallback rule to apply")
	}
	if token.Lemma != "kirja" || token.POS != "NOUN" {
		t.Fatalf("got %s/%s want kirja/NOUN", token.Lemma, token.POS)
	}
	if token.Source != "override:possessive" {
		t.Fatalf("source=%q want override:possessive", token.Source)
	}
	if token.GrammarLabel != "inessive" {
		t.Fatalf("grammar_label=%q want inessive", token.GrammarLabel)
	}
	if len(token.Trace) == 0 {
		t.Fatal("expected trace entry to be recorded")
	}
}
