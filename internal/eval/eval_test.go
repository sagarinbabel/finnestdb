package eval

import (
	"math"
	"testing"

	"finnestdb/internal/parsecore"
)

func TestCompareCase_MatchesOccurrenceAndGrammar(t *testing.T) {
	c := DatasetCase{
		ID:   "fi-1",
		Text: "Talossa talossa.",
		Tokens: []ExpectedTokenRef{
			{Surface: "Talossa", Lemma: "talo", POS: "NOUN", GrammarLabel: "inessive"},
			{Surface: "talossa", Occurrence: 1, Lemma: "talo", POS: "NOUN", GrammarLabel: "inessive"},
		},
	}
	parsed := &parsecore.ParseResult{
		Sentences: []parsecore.SentenceResult{
			{
				Text: "Talossa talossa.",
				Tokens: []parsecore.TokenResult{
					{Form: "Talossa", Lemma: "talo", POS: "NOUN", GrammarLabel: "inessive", Source: "case_suffix", Resolved: true},
					{Form: "talossa", Lemma: "talo", POS: "NOUN", GrammarLabel: "inessive", Source: "case_suffix", Resolved: true},
					{Form: ".", Lemma: ".", POS: "PUNCT", Source: "punct"},
				},
			},
		},
	}

	got := compareCase(c, parsed)
	if len(got) != 2 {
		t.Fatalf("expected 2 comparisons, got %d", len(got))
	}
	for i, cmp := range got {
		if !cmp.Actual.Found {
			t.Fatalf("comparison %d: expected token to be found", i)
		}
		if !cmp.Match.Full {
			t.Fatalf("comparison %d: expected full match, got %+v", i, cmp.Match)
		}
		if cmp.Actual.Source != "case_suffix" {
			t.Fatalf("comparison %d: source=%q want case_suffix", i, cmp.Actual.Source)
		}
	}
}

func TestSummaryAccumulator_FullAndCoverage(t *testing.T) {
	acc := &summaryAccumulator{}
	parsed := &parsecore.ParseResult{
		TotalTokens: 2,
		Stats: parsecore.ParseStats{
			UniqueForms:      2,
			ResolvedTokens:   1,
			UnresolvedTokens: 1,
			Timings: parsecore.ParseTimings{
				AnalyzeNs:          4_000_000,
				LookupFormsNs:      3_000_000,
				LookupGlossesNs:    2_000_000,
				ResolveSentencesNs: 1_000_000,
				EnrichWordsNs:      1_000_000,
			},
		},
		Sentences: []parsecore.SentenceResult{
			{
				Text: "Kirjassani on.",
				Tokens: []parsecore.TokenResult{
					{Form: "Kirjassani", Lemma: "kirja", POS: "NOUN", Resolved: true},
					{Form: "on", Lemma: "olla", POS: "VERB", Resolved: false},
					{Form: ".", Lemma: ".", POS: "PUNCT"},
				},
			},
		},
	}
	comparisons := []TokenCompare{
		{
			Expected: TokenExpected{Lemma: "kirja", POS: "NOUN"},
			Actual:   TokenActual{Found: true, Lemma: "kirja", POS: "NOUN", Resolved: true},
			Match:    TokenMatch{Lemma: true, POS: true, Grammar: true, Full: true},
		},
		{
			Expected: TokenExpected{Lemma: "olla", POS: "VERB"},
			Actual:   TokenActual{Found: true, Lemma: "on", POS: "VERB", Resolved: false},
			Match:    TokenMatch{Lemma: false, POS: true, Grammar: true, Full: false},
		},
	}

	const charsPerRun = 14 // utf8 rune count of "Kirjassani on."
	acc.consume(parsed, comparisons, []int64{8_000_000, 10_000_000}, charsPerRun)
	got := acc.finish()

	if got.ExpectedTokens != 2 {
		t.Fatalf("expected_tokens=%d want 2", got.ExpectedTokens)
	}
	if got.LemmaAccuracy != 0.5 {
		t.Fatalf("lemma_accuracy=%v want 0.5", got.LemmaAccuracy)
	}
	if got.POSAccuracy != 1.0 {
		t.Fatalf("pos_accuracy=%v want 1.0", got.POSAccuracy)
	}
	if got.FullAccuracy != 0.5 {
		t.Fatalf("full_accuracy=%v want 0.5", got.FullAccuracy)
	}
	if got.ResolvedCoverage != 0.5 {
		t.Fatalf("resolved_coverage=%v want 0.5", got.ResolvedCoverage)
	}
	if got.AvgCaseDurationMs != 9 {
		t.Fatalf("avg_case_duration_ms=%v want 9", got.AvgCaseDurationMs)
	}
	if got.P50CaseDurationMs != 8 {
		t.Fatalf("p50_case_duration_ms=%v want 8", got.P50CaseDurationMs)
	}
	if got.P95CaseDurationMs != 8 {
		t.Fatalf("p95_case_duration_ms=%v want 8", got.P95CaseDurationMs)
	}
	if got.AvgUniqueForms != 2 {
		t.Fatalf("avg_unique_forms=%v want 2", got.AvgUniqueForms)
	}
	if got.AvgResolvedTokens != 1 {
		t.Fatalf("avg_resolved_tokens=%v want 1", got.AvgResolvedTokens)
	}
	if got.AvgUnresolvedTokens != 1 {
		t.Fatalf("avg_unresolved_tokens=%v want 1", got.AvgUnresolvedTokens)
	}
	if got.AvgAnalyzeMs != 4 {
		t.Fatalf("avg_analyze_ms=%v want 4", got.AvgAnalyzeMs)
	}
	if got.AvgLookupFormsMs != 3 {
		t.Fatalf("avg_lookup_forms_ms=%v want 3", got.AvgLookupFormsMs)
	}
	if got.AvgLookupGlossesMs != 2 {
		t.Fatalf("avg_lookup_glosses_ms=%v want 2", got.AvgLookupGlossesMs)
	}
	if got.AvgResolveSentencesMs != 1 {
		t.Fatalf("avg_resolve_sentences_ms=%v want 1", got.AvgResolveSentencesMs)
	}
	if got.AvgEnrichWordsMs != 1 {
		t.Fatalf("avg_enrich_words_ms=%v want 1", got.AvgEnrichWordsMs)
	}

	const totalNs = 8_000_000 + 10_000_000
	wantWordsPerSec := float64(2*2) / float64(totalNs) * 1e9
	wantCharsPerSec := float64(charsPerRun*2) / float64(totalNs) * 1e9
	if math.Abs(got.WordsPerSecond-wantWordsPerSec) > 1e-6 {
		t.Fatalf("words_per_second=%v want %v", got.WordsPerSecond, wantWordsPerSec)
	}
	if math.Abs(got.CharsPerSecond-wantCharsPerSec) > 1e-6 {
		t.Fatalf("chars_per_second=%v want %v", got.CharsPerSecond, wantCharsPerSec)
	}
}

func TestComputePriorityRegressions_FocusParserLoses(t *testing.T) {
	report := &Report{
		Parsers: []string{"basic", "custom", "omorfi"},
		Cases: []CaseReport{
			{
				CaseID: "fi-1",
				Comparisons: map[string][]TokenCompare{
					"basic":  {{Surface: "kirjassani", Occurrence: 1, Match: TokenMatch{Full: true}}},
					"custom": {{Surface: "kirjassani", Occurrence: 1, Match: TokenMatch{Full: true}}},
					"omorfi": {{
						Surface:    "kirjassani",
						Occurrence: 1,
						Expected:   TokenExpected{Lemma: "kirja", POS: "NOUN"},
						Actual:     TokenActual{Found: true, Lemma: "kirjassani", POS: "X", Source: "analyzer:omorfi"},
						Match:      TokenMatch{Full: false},
					}},
				},
			},
		},
	}

	got := computePriorityRegressions(report)
	if len(got) != 1 {
		t.Fatalf("expected 1 priority regression, got %d", len(got))
	}
	if got[0].Parser != "omorfi" {
		t.Fatalf("parser=%q want omorfi", got[0].Parser)
	}
	if got[0].Surface != "kirjassani" {
		t.Fatalf("surface=%q want kirjassani", got[0].Surface)
	}
}
