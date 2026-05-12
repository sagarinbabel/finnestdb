package parsecore

import (
	"slices"
	"testing"
)

func TestSupportedParsersList(t *testing.T) {
	got := SupportedParsers()
	want := []string{"basic", "custom"}
	if !slices.Equal(got, want) {
		t.Fatalf("supported parsers=%v want %v", got, want)
	}
}

func TestFeatsFromJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty bytes", "", ""},
		{"empty object", `{}`, ""},
		{"null", `null`, ""},
		{"single attr", `{"Case":"Ine"}`, "Case=Ine"},
		{"sorted keys", `{"Number":"Sing","Case":"Ine"}`, "Case=Ine|Number=Sing"},
		{"array value joined", `{"Case":["Ine","Ade"]}`, "Case=Ine,Ade"},
		{"non-string skipped", `{"Case":42}`, ""},
		{"invalid json", `not-json`, ""},
	}
	for _, tc := range cases {
		got := featsFromJSON([]byte(tc.in))
		if got != tc.want {
			t.Errorf("%s: featsFromJSON(%q)=%q want %q", tc.name, tc.in, got, tc.want)
		}
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
		AnalyzeNs:          4_000_000,
		LookupFormsNs:      3_000_000,
		LookupGlossesNs:    2_000_000,
		ResolveSentencesNs: 1_000_000,
		EnrichWordsNs:      1_000_000,
		TotalNs:            11_000_000,
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
	if stats.Timings.TotalNs != 11_000_000 {
		t.Fatalf("total_ns=%d want 11_000_000", stats.Timings.TotalNs)
	}
}

func TestEnrichWordsSuppressesAmbiguousAggregateMorphology(t *testing.T) {
	words := enrichWords([]SentenceResult{
		{
			Text: "On olemas.",
			Tokens: []TokenResult{
				{Form: "On", Lemma: "olema", POS: "VERB", Feats: "Mood=Ind|Number=Sing|Person=3|Tense=Pres"},
				{Form: "olemas", Lemma: "olema", POS: "VERB", GrammarLabel: "inessive", Feats: "Case=Ine|VerbForm=Sup"},
			},
		},
		{
			Text: "Majas.",
			Tokens: []TokenResult{
				{Form: "Majas", Lemma: "maja", POS: "NOUN", GrammarLabel: "inessive", Feats: "Case=Ine|Number=Sing"},
				{Form: "majas", Lemma: "maja", POS: "NOUN", GrammarLabel: "inessive", Feats: "Case=Ine|Number=Sing"},
			},
		},
	}, nil)

	byLemma := map[string]WordEntry{}
	for _, word := range words {
		byLemma[word.Lemma] = word
	}
	if got := byLemma["olema"]; got.GrammarLabel != "" || got.Feats != "" {
		t.Fatalf("olema aggregate morphology = (%q, %q), want empty for mixed forms", got.GrammarLabel, got.Feats)
	}
	if got := byLemma["maja"]; got.GrammarLabel != "inessive" || got.Feats != "Case=Ine|Number=Sing" {
		t.Fatalf("maja aggregate morphology = (%q, %q), want stable inessive features", got.GrammarLabel, got.Feats)
	}
}
