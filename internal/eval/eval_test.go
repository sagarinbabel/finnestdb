package eval

import (
	"math"
	"testing"

	"finnestdb/internal/parsecore"
)

func TestResolveRunIDSlug_OptionWinsAndDistinguishesSameNameFiles(t *testing.T) {
	// Both gold files carry name="fi-manual" by historical accident — they
	// must not collide in the default report path.
	ds := &Dataset{Name: "fi-manual"}

	v1 := resolveRunIDSlug(EvaluateOptions{RunIDSlug: "fi-manual-v1"}, ds)
	v2 := resolveRunIDSlug(EvaluateOptions{RunIDSlug: "fi-manual-v2"}, ds)
	if v1 == v2 || v1 != "fi-manual-v1" || v2 != "fi-manual-v2" {
		t.Fatalf("RunIDSlug must override dataset.Name: v1=%q v2=%q", v1, v2)
	}

	// When the option is empty, fall back to slugify(dataset.Name) so legacy
	// callers that don't know the input file path still work.
	fallback := resolveRunIDSlug(EvaluateOptions{}, ds)
	if fallback != "fi-manual" {
		t.Fatalf("empty RunIDSlug must fall back to slugify(dataset.Name); got %q", fallback)
	}

	// All-whitespace RunIDSlug falls back rather than producing a degenerate
	// slug; slugify() itself never returns empty so the override branch only
	// fires when the caller explicitly provided something.
	dirty := resolveRunIDSlug(EvaluateOptions{RunIDSlug: "   "}, &Dataset{Name: "core"})
	if dirty != "core" {
		t.Fatalf("whitespace RunIDSlug should fall back to dataset.Name; got %q", dirty)
	}
}

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
	if got.LemmaPOSAccuracy != 0.5 {
		t.Fatalf("lemma_pos_accuracy=%v want 0.5", got.LemmaPOSAccuracy)
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

// TestSummaryAccumulator_LemmaPOSIsJoint guards against the easy mistake of
// reading "dictionary entry attachment" as the marginal product of
// LemmaAccuracy and POSAccuracy. With one token having (lemma=ok, POS=wrong)
// and another (lemma=wrong, POS=ok), each marginal is 50% but joint
// attachment is 0% — there is no token where both fields match. A learner
// would land on the right dictionary entry zero times, even though both
// marginals look mediocre-but-passable.
func TestSummaryAccumulator_LemmaPOSIsJoint(t *testing.T) {
	acc := &summaryAccumulator{}
	parsed := &parsecore.ParseResult{
		TotalTokens: 2,
		Sentences: []parsecore.SentenceResult{
			{Tokens: []parsecore.TokenResult{
				{Form: "a", Lemma: "a", POS: "NOUN", Resolved: true},
				{Form: "b", Lemma: "b", POS: "NOUN", Resolved: true},
			}},
		},
	}
	comparisons := []TokenCompare{
		{
			Expected: TokenExpected{Lemma: "a", POS: "NOUN"},
			Match:    TokenMatch{Lemma: true, POS: false},
		},
		{
			Expected: TokenExpected{Lemma: "b", POS: "NOUN"},
			Match:    TokenMatch{Lemma: false, POS: true},
		},
	}
	acc.consume(parsed, comparisons, []int64{1_000_000}, 2)
	got := acc.finish()
	if got.LemmaAccuracy != 0.5 {
		t.Errorf("lemma_accuracy=%v want 0.5", got.LemmaAccuracy)
	}
	if got.POSAccuracy != 0.5 {
		t.Errorf("pos_accuracy=%v want 0.5", got.POSAccuracy)
	}
	if got.LemmaPOSAccuracy != 0 {
		t.Errorf("lemma_pos_accuracy=%v want 0 (no token has BOTH lemma and POS correct)", got.LemmaPOSAccuracy)
	}
}

func TestCompareCase_PerAttributeFeats(t *testing.T) {
	c := DatasetCase{
		ID:   "et-1",
		Text: "Raamatus.",
		Tokens: []ExpectedTokenRef{
			{Surface: "Raamatus", Lemma: "raamat", POS: "NOUN", Feats: "Case=Ine|Number=Sing"},
		},
	}
	parsed := &parsecore.ParseResult{
		Sentences: []parsecore.SentenceResult{
			{
				Text: "Raamatus.",
				Tokens: []parsecore.TokenResult{
					{Form: "Raamatus", Lemma: "raamat", POS: "NOUN", Feats: "Case=Ine|Number=Plur", Resolved: true},
					{Form: ".", Lemma: ".", POS: "PUNCT"},
				},
			},
		},
	}

	got := compareCase(c, parsed)
	if len(got) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(got))
	}
	cmp := got[0]
	if cmp.Match.Feats == nil {
		t.Fatal("expected Match.Feats to be populated")
	}
	if !cmp.Match.Feats["Case"] {
		t.Errorf("Case attribute should match: %+v", cmp.Match.Feats)
	}
	if cmp.Match.Feats["Number"] {
		t.Errorf("Number attribute should NOT match (gold=Sing actual=Plur)")
	}
	if cmp.Actual.Feats != "Case=Ine|Number=Plur" {
		t.Errorf("actual.Feats=%q want Case=Ine|Number=Plur", cmp.Actual.Feats)
	}
	if cmp.Match.Full {
		t.Errorf("Match.Full should be false when a FEATS attribute mismatches: %+v", cmp.Match)
	}
}

// TestCompareCase_FullRequiresFeatsMatch covers the three FEATS-driven Full
// transitions: a Number-only mismatch flips Full off, a Tense+Mood mismatch
// flips Full off, and a fully-correct verb morphology keeps Full true. Without
// this gating the per-attribute report would surface FEATS errors but
// full_accuracy would still claim 100%.
func TestCompareCase_FullRequiresFeatsMatch(t *testing.T) {
	cases := []struct {
		name      string
		gold      string
		actual    string
		wantFull  bool
		wantMatch map[string]bool
	}{
		{
			name:      "number mismatch only",
			gold:      "Case=Ine|Number=Sing",
			actual:    "Case=Ine|Number=Plur",
			wantFull:  false,
			wantMatch: map[string]bool{"Case": true, "Number": false},
		},
		{
			name:      "tense and mood mismatch",
			gold:      "Mood=Ind|Number=Sing|Person=3|Tense=Pres",
			actual:    "Mood=Imp|Number=Sing|Person=3|Tense=Past",
			wantFull:  false,
			wantMatch: map[string]bool{"Mood": false, "Number": true, "Person": true, "Tense": false},
		},
		{
			name:      "all attributes match",
			gold:      "Case=Ine|Number=Sing",
			actual:    "Case=Ine|Number=Sing",
			wantFull:  true,
			wantMatch: map[string]bool{"Case": true, "Number": true},
		},
		{
			name:      "actual missing feats entirely",
			gold:      "Case=Ine|Number=Sing",
			actual:    "",
			wantFull:  false,
			wantMatch: map[string]bool{"Case": false, "Number": false},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := DatasetCase{
				ID:   "fi-1",
				Text: "x.",
				Tokens: []ExpectedTokenRef{
					{Surface: "x", Lemma: "x", POS: "VERB", Feats: tc.gold},
				},
			}
			parsed := &parsecore.ParseResult{
				Sentences: []parsecore.SentenceResult{
					{Tokens: []parsecore.TokenResult{
						{Form: "x", Lemma: "x", POS: "VERB", Feats: tc.actual, Resolved: true},
					}},
				},
			}
			got := compareCase(c, parsed)
			if len(got) != 1 {
				t.Fatalf("comparisons=%d want 1", len(got))
			}
			cmp := got[0]
			if cmp.Match.Full != tc.wantFull {
				t.Errorf("Match.Full=%v want %v (feats=%+v)", cmp.Match.Full, tc.wantFull, cmp.Match.Feats)
			}
			for attr, want := range tc.wantMatch {
				if got := cmp.Match.Feats[attr]; got != want {
					t.Errorf("Match.Feats[%q]=%v want %v", attr, got, want)
				}
			}
		})
	}
}

// TestSummaryAccumulator_FullAccuracyRespectsFeats verifies the accumulator
// rolls per-token Full into FullAccuracy, so a Number/Tense FEATS divergence
// (which the legacy GrammarAccuracy can't see) actually lowers
// full_accuracy on the parser-compare report.
func TestSummaryAccumulator_FullAccuracyRespectsFeats(t *testing.T) {
	acc := &summaryAccumulator{}
	parsed := &parsecore.ParseResult{
		TotalTokens: 2,
		Sentences: []parsecore.SentenceResult{
			{Tokens: []parsecore.TokenResult{
				{Form: "a", Lemma: "a", POS: "NOUN", Resolved: true},
				{Form: "b", Lemma: "b", POS: "NOUN", Resolved: true},
			}},
		},
	}
	c := DatasetCase{
		Tokens: []ExpectedTokenRef{
			{Surface: "a", Lemma: "a", POS: "NOUN", Feats: "Case=Ine|Number=Sing"},
			{Surface: "b", Lemma: "b", POS: "NOUN", Feats: "Case=Ine|Number=Sing"},
		},
	}
	parsedRun := &parsecore.ParseResult{
		Sentences: []parsecore.SentenceResult{
			{Tokens: []parsecore.TokenResult{
				{Form: "a", Lemma: "a", POS: "NOUN", Feats: "Case=Ine|Number=Sing", Resolved: true},
				{Form: "b", Lemma: "b", POS: "NOUN", Feats: "Case=Ine|Number=Plur", Resolved: true},
			}},
		},
	}
	comparisons := compareCase(c, parsedRun)
	acc.consume(parsed, comparisons, []int64{1_000_000}, 2)
	got := acc.finish()
	if got.FullAccuracy != 0.5 {
		t.Errorf("full_accuracy=%v want 0.5 (one of two tokens has FEATS mismatch)", got.FullAccuracy)
	}
	if got.LemmaAccuracy != 1.0 || got.POSAccuracy != 1.0 {
		t.Errorf("lemma=%v pos=%v want both 1.0", got.LemmaAccuracy, got.POSAccuracy)
	}
}

func TestSummaryAccumulator_FeatsAttributeBreakdown(t *testing.T) {
	acc := &summaryAccumulator{}
	parsed := &parsecore.ParseResult{
		TotalTokens: 2,
		Sentences: []parsecore.SentenceResult{
			{Tokens: []parsecore.TokenResult{
				{Form: "a", Lemma: "a", POS: "NOUN", Resolved: true},
				{Form: "b", Lemma: "b", POS: "NOUN", Resolved: true},
			}},
		},
	}
	comparisons := []TokenCompare{
		{
			Expected: TokenExpected{Lemma: "a", POS: "NOUN", Feats: "Case=Ine|Number=Sing"},
			Match:    TokenMatch{Lemma: true, POS: true, Grammar: true, Feats: map[string]bool{"Case": true, "Number": true}, Full: true},
		},
		{
			Expected: TokenExpected{Lemma: "b", POS: "NOUN", Feats: "Case=Ine|Number=Plur"},
			Match:    TokenMatch{Lemma: true, POS: true, Grammar: true, Feats: map[string]bool{"Case": false, "Number": true}, Full: false},
		},
	}
	acc.consume(parsed, comparisons, []int64{1_000_000}, 2)
	got := acc.finish()

	if len(got.FeatsAttributes) != 2 {
		t.Fatalf("FeatsAttributes len=%d want 2: %+v", len(got.FeatsAttributes), got.FeatsAttributes)
	}
	if got.FeatsAttributes[0].Attribute != "Case" || got.FeatsAttributes[1].Attribute != "Number" {
		t.Fatalf("expected sorted [Case, Number], got %+v", got.FeatsAttributes)
	}
	caseMetric := got.FeatsAttributes[0]
	if caseMetric.Eligible != 2 || caseMetric.Correct != 1 || caseMetric.Accuracy != 0.5 {
		t.Errorf("Case metric=%+v want eligible=2 correct=1 accuracy=0.5", caseMetric)
	}
	numberMetric := got.FeatsAttributes[1]
	if numberMetric.Eligible != 2 || numberMetric.Correct != 2 || numberMetric.Accuracy != 1.0 {
		t.Errorf("Number metric=%+v want eligible=2 correct=2 accuracy=1.0", numberMetric)
	}
}

func TestParseFeats(t *testing.T) {
	cases := []struct {
		in   string
		want map[string]string
	}{
		{"", map[string]string{}},
		{"Case=Ine", map[string]string{"Case": "Ine"}},
		{"Case=Ine|Number=Sing", map[string]string{"Case": "Ine", "Number": "Sing"}},
		{"Case=Ine|garbage|Number=Sing", map[string]string{"Case": "Ine", "Number": "Sing"}},
		{"=Ine|Case=", map[string]string{}},
	}
	for _, tc := range cases {
		got := parseFeats(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("parseFeats(%q)=%v want %v", tc.in, got, tc.want)
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Errorf("parseFeats(%q)[%q]=%q want %q", tc.in, k, got[k], v)
			}
		}
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
