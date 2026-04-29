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

func TestComputeParseStatsCountsResolutionSourcesAndTimings(t *testing.T) {
	stats := computeParseStats([]SentenceResult{
		{
			Text: "Kirjassani on pankissa.",
			Tokens: []TokenResult{
				{Form: "Kirjassani", Lemma: "kirja", POS: "NOUN", Source: "possessive", Resolved: true},
				{Form: "on", Lemma: "olla", POS: "VERB", Source: "stub", Resolved: false},
				{Form: "pankissa", Lemma: "pankki", POS: "NOUN", Source: "case_suffix", Resolved: true},
				{Form: ".", Lemma: ".", POS: "PUNCT", Source: "punct"},
			},
		},
	}, 3, ParseTimings{
		AnalyzeMs:          4,
		LookupFormsMs:      3,
		LookupGlossesMs:    2,
		ResolveSentencesMs: 1,
		EnrichWordsMs:      1,
		TotalMs:            11,
	})

	if stats.UniqueForms != 3 {
		t.Fatalf("unique_forms=%d want 3", stats.UniqueForms)
	}
	if stats.TotalSentences != 1 {
		t.Fatalf("total_sentences=%d want 1", stats.TotalSentences)
	}
	if stats.ResolvedTokens != 2 {
		t.Fatalf("resolved_tokens=%d want 2", stats.ResolvedTokens)
	}
	if stats.UnresolvedTokens != 1 {
		t.Fatalf("unresolved_tokens=%d want 1", stats.UnresolvedTokens)
	}
	if stats.PunctTokens != 1 {
		t.Fatalf("punct_tokens=%d want 1", stats.PunctTokens)
	}
	if stats.SourceCounts["possessive"] != 1 || stats.SourceCounts["stub"] != 1 || stats.SourceCounts["punct"] != 1 {
		t.Fatalf("unexpected source counts: %#v", stats.SourceCounts)
	}
	if stats.Timings.TotalMs != 11 {
		t.Fatalf("total_ms=%d want 11", stats.Timings.TotalMs)
	}
}
