package main

import (
	"math"
	"math/rand"
	"testing"

	"finnestdb/internal/eval"
)

// fakeReport builds a Report with N cases, each having tokensPerCase tokens
// of which lemmaCorrectFraction are marked correct. Used to exercise the
// per-case statistics extractor and bootstrap loop without standing up the
// full eval pipeline.
func fakeReport(parser string, n, tokensPerCase int, lemmaCorrectFraction float64) *eval.Report {
	cases := make([]eval.CaseReport, n)
	for i := 0; i < n; i++ {
		cmps := make([]eval.TokenCompare, tokensPerCase)
		correct := int(float64(tokensPerCase) * lemmaCorrectFraction)
		for j := 0; j < tokensPerCase; j++ {
			cmps[j] = eval.TokenCompare{
				Surface: "x",
				Expected: eval.TokenExpected{
					Lemma:        "x",
					POS:          "NOUN",
					GrammarLabel: "inessive",
				},
				Actual: eval.TokenActual{Found: true, Resolved: true},
				Match: eval.TokenMatch{
					Lemma:   j < correct,
					POS:     true,
					Grammar: j < correct,
					Full:    j < correct,
				},
			}
		}
		cases[i] = eval.CaseReport{
			CaseID: "c",
			Comparisons: map[string][]eval.TokenCompare{
				parser: cmps,
			},
		}
	}
	return &eval.Report{
		Dataset: eval.ReportDataset{Name: "fake", CaseCount: n},
		Parsers: []string{parser},
		Summary: map[string]eval.ParserSummary{
			parser: {
				LemmaAccuracy:    lemmaCorrectFraction,
				POSAccuracy:      1.0,
				GrammarAccuracy:  lemmaCorrectFraction,
				FullAccuracy:     lemmaCorrectFraction,
				ResolvedCoverage: 1.0,
			},
		},
		Cases: cases,
	}
}

// TestPerCaseStats verifies that the importer of token-level matches into
// per-case counts matches the expected denominators / numerators.
func TestPerCaseStats(t *testing.T) {
	r := fakeReport("custom", 5, 10, 0.8)
	stats := perCaseStats(r, "custom")
	if got, want := len(stats), 5; got != want {
		t.Fatalf("len(stats): got %d, want %d", got, want)
	}
	for i, s := range stats {
		if s.expectedTokens != 10 {
			t.Errorf("case %d expected: got %d, want 10", i, s.expectedTokens)
		}
		if s.lemmaCorrect != 8 {
			t.Errorf("case %d lemma: got %d, want 8", i, s.lemmaCorrect)
		}
		if s.grammarEligible != 10 {
			t.Errorf("case %d grammarEligible: got %d, want 10", i, s.grammarEligible)
		}
		if s.grammarCorrect != 8 {
			t.Errorf("case %d grammarCorrect: got %d, want 8", i, s.grammarCorrect)
		}
		if s.totalTokens != 10 {
			t.Errorf("case %d total: got %d, want 10", i, s.totalTokens)
		}
		if s.resolvedTokens != 10 {
			t.Errorf("case %d resolved: got %d, want 10", i, s.resolvedTokens)
		}
	}
}

// TestBootstrapHalfWidth: the half-width on a uniform-accuracy report (every
// case is exactly 80% correct on the lemma metric) should be effectively zero.
// Resampling the same case repeatedly gives the same accuracy every time.
func TestBootstrapHalfWidth_UniformIsZero(t *testing.T) {
	r := fakeReport("custom", 30, 10, 0.8)
	rng := rand.New(rand.NewSource(1))
	half := bootstrapHalfWidth(r, "custom", metricLemma, 200, rng)
	if half > 1e-9 {
		t.Errorf("half-width on uniform-accuracy fake: got %g, want ≈0", half)
	}
}

// TestBootstrapHalfWidth_VarianceShrinksWithN: doubling the case count
// should shrink the bootstrap CI half-width. Heterogeneous per-case
// accuracy is required to see any width at all — half the cases are 100%
// correct, the other half 0%.
func TestBootstrapHalfWidth_VarianceShrinksWithN(t *testing.T) {
	mkHetReport := func(nCases int) *eval.Report {
		// Half the cases all-correct, half all-wrong on Lemma.
		// All cases identical token count = 10.
		cases := make([]eval.CaseReport, nCases)
		for i := 0; i < nCases; i++ {
			cmps := make([]eval.TokenCompare, 10)
			correct := i%2 == 0
			for j := range cmps {
				cmps[j] = eval.TokenCompare{
					Expected: eval.TokenExpected{Lemma: "x", POS: "NOUN"},
					Match:    eval.TokenMatch{Lemma: correct, POS: true, Grammar: correct},
				}
			}
			cases[i] = eval.CaseReport{
				Comparisons: map[string][]eval.TokenCompare{"custom": cmps},
			}
		}
		return &eval.Report{
			Dataset: eval.ReportDataset{Name: "het", CaseCount: nCases},
			Parsers: []string{"custom"},
			Cases:   cases,
		}
	}

	rng := rand.New(rand.NewSource(2))
	smaller := bootstrapHalfWidth(mkHetReport(20), "custom", metricLemma, 500, rng)
	rng = rand.New(rand.NewSource(2))
	larger := bootstrapHalfWidth(mkHetReport(200), "custom", metricLemma, 500, rng)

	if math.IsNaN(smaller) || math.IsNaN(larger) {
		t.Fatalf("got NaN: smaller=%v larger=%v", smaller, larger)
	}
	if larger >= smaller {
		t.Errorf("CI should shrink with N: smaller=%g (n=20) larger=%g (n=200)", smaller, larger)
	}
}

// TestAnalyzerParserFor recognises omorfi and estnltk; everything else returns "".
func TestAnalyzerParserFor(t *testing.T) {
	cases := []struct {
		parsers []string
		want    string
	}{
		{[]string{"basic", "custom", "omorfi"}, "omorfi"},
		{[]string{"basic", "custom", "estnltk"}, "estnltk"},
		{[]string{"basic", "custom"}, ""},
		{[]string{}, ""},
	}
	for _, tc := range cases {
		got := analyzerParserFor(&eval.Report{Parsers: tc.parsers})
		if got != tc.want {
			t.Errorf("analyzerParserFor(%v): got %q, want %q", tc.parsers, got, tc.want)
		}
	}
}
