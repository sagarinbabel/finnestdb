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

func TestValidateChaptersInputRequiresLangContentAndCap(t *testing.T) {
	cases := []struct {
		name      string
		lang      string
		chapters  []ChapterInput
		wantError string
	}{
		{"bad lang", "DE", []ChapterInput{{Title: "A", Text: "hi"}}, "language must be FI or ET"},
		{"empty chapters", "FI", nil, "chapters is required"},
		{"all chapters whitespace", "FI", []ChapterInput{{Title: "A", Text: "   "}, {Title: "B", Text: "\n\n"}}, "chapters is required"},
		{"ok", "FI", []ChapterInput{{Title: "A", Text: "kissa"}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateChaptersInput(tc.lang, tc.chapters)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantError {
				t.Fatalf("err=%v want %q", err, tc.wantError)
			}
		})
	}
}

func TestComputeChapterResultsBucketsSentencesAndAggregatesWords(t *testing.T) {
	idx := func(i int) *int { return &i }
	sentences := []SentenceResult{
		{
			Text:       "Kissa juoksee.",
			ChapterIdx: idx(0),
			Tokens: []TokenResult{
				{Form: "Kissa", Lemma: "kissa", POS: "NOUN", Resolved: true},
				{Form: "juoksee", Lemma: "juosta", POS: "VERB", Resolved: true},
				{Form: ".", Lemma: ".", POS: "PUNCT"},
			},
		},
		{
			Text:       "Koira haukkuu.",
			ChapterIdx: idx(1),
			Tokens: []TokenResult{
				{Form: "Koira", Lemma: "koira", POS: "NOUN", Resolved: true},
				{Form: "haukkuu", Lemma: "haukkua", POS: "VERB", Resolved: false},
			},
		},
		{
			Text:       "Lisää kissoja.",
			ChapterIdx: idx(0),
			Tokens: []TokenResult{
				{Form: "Lisää", Lemma: "lisää", POS: "ADV", Resolved: true},
				{Form: "kissoja", Lemma: "kissa", POS: "NOUN", Resolved: true},
			},
		},
	}
	chapters := []ChapterInput{
		{Title: "Cat chapter", Text: "Kissa juoksee. Lisää kissoja."},
		{Title: "Dog chapter", Text: "Koira haukkuu."},
		{Title: "Empty", Text: ""},
	}
	got := computeChapterResults(chapters, sentences, nil)
	if len(got) != 3 {
		t.Fatalf("got %d chapter rollups want 3", len(got))
	}
	if got[0].Title != "Cat chapter" || got[0].SentenceCount != 2 || got[0].TokenCount != 4 {
		t.Fatalf("chapter 0 rollup = %+v want sentences=2 tokens=4", got[0])
	}
	if got[0].ResolvedTokens != 4 || got[0].UnresolvedTokens != 0 {
		t.Fatalf("chapter 0 resolved/unresolved = %d/%d want 4/0", got[0].ResolvedTokens, got[0].UnresolvedTokens)
	}
	// kissa appears twice in chapter 0 (different forms) but as one lemma.
	wantLemmas := map[string]int{"kissa": 2, "juosta": 1, "lisää": 1}
	if got[0].LemmaCount != len(wantLemmas) {
		t.Fatalf("chapter 0 lemma_count = %d want %d (kissa+juosta+lisää)", got[0].LemmaCount, len(wantLemmas))
	}
	for _, w := range got[0].Words {
		want, ok := wantLemmas[w.Lemma]
		if !ok {
			t.Fatalf("chapter 0 unexpected lemma %q", w.Lemma)
		}
		if w.Count != want {
			t.Fatalf("chapter 0 lemma %q count = %d want %d", w.Lemma, w.Count, want)
		}
	}
	if got[1].SentenceCount != 1 || got[1].TokenCount != 2 {
		t.Fatalf("chapter 1 rollup = %+v want sentences=1 tokens=2", got[1])
	}
	if got[1].ResolvedTokens != 1 || got[1].UnresolvedTokens != 1 {
		t.Fatalf("chapter 1 resolved/unresolved = %d/%d want 1/1", got[1].ResolvedTokens, got[1].UnresolvedTokens)
	}
	if got[2].Title != "Empty" || got[2].SentenceCount != 0 || got[2].LemmaCount != 0 || len(got[2].Words) != 0 {
		t.Fatalf("empty chapter rollup = %+v want zeroed", got[2])
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
		{
			Text: "Joon vett.",
			Tokens: []TokenResult{
				{Form: "Joon", Lemma: "jooma", POS: "VERB", Feats: "Mood=Ind|Number=Sing|Person=1|Tense=Pres"},
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
	if got := byLemma["jooma"]; got.GrammarLabel != "" || got.Feats != "Mood=Ind|Number=Sing|Person=1|Tense=Pres" {
		t.Fatalf("jooma aggregate morphology = (%q, %q), want verb FEATS without a grammar label", got.GrammarLabel, got.Feats)
	}
}
