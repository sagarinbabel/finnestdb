package main

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		if s.lemmaPOSCorrect != 8 {
			t.Errorf("case %d lemmaPOS: got %d, want 8", i, s.lemmaPOSCorrect)
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

// TestLemmaPOSDisplay_FallbackForOldBaselines: a report that pre-dates the
// LemmaPOSAccuracy field stores zero in the summary but still has correct
// per-case data. The fallback recomputes from cases so the headline
// before/after diff doesn't read a missing field as a 100-point regression.
func TestLemmaPOSDisplay_FallbackForOldBaselines(t *testing.T) {
	r := fakeReport("custom", 5, 10, 0.8)
	s := r.Summary["custom"]
	s.LemmaPOSAccuracy = 0 // simulate pre-PR baseline
	r.Summary["custom"] = s

	got := lemmaPOSDisplay(s, r, "custom")
	if got < 0.79 || got > 0.81 {
		t.Errorf("fallback recompute: got %.3f, want ~0.8 from per-case data", got)
	}
}

// TestLemmaPOSDisplay_PreservesGenuineZero: when both Lemma and POS marginals
// are zero, the metric is genuinely zero — fallback must not mask this as
// missing data.
func TestLemmaPOSDisplay_PreservesGenuineZero(t *testing.T) {
	r := fakeReport("custom", 5, 10, 0.0) // every token wrong
	s := r.Summary["custom"]
	got := lemmaPOSDisplay(s, r, "custom")
	if got != 0 {
		t.Errorf("genuine zero accuracy: got %.3f, want 0", got)
	}
}

// TestMetricFromReport_LemmaPOSUsesFallback verifies the headline before/after
// table goes through the per-case fallback for Lemma+POS specifically.
func TestMetricFromReport_LemmaPOSUsesFallback(t *testing.T) {
	r := fakeReport("custom", 5, 10, 0.8)
	s := r.Summary["custom"]
	s.LemmaPOSAccuracy = 0
	r.Summary["custom"] = s

	got := metricFromReport(r, "custom", metricLemmaPOS)
	if got < 0.79 || got > 0.81 {
		t.Errorf("Lemma+POS via metricFromReport: got %.3f, want ~0.8", got)
	}

	// Other metrics still read from the summary and are not fallback-recomputed.
	gotLemma := metricFromReport(r, "custom", metricLemma)
	if gotLemma != 0.8 {
		t.Errorf("Lemma metric: got %.3f, want 0.8 (from summary)", gotLemma)
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

func TestLoadBaselineDir_UsesGeneratedAtNotMtime(t *testing.T) {
	dir := t.TempDir()

	oldReport := &eval.Report{
		Generated: "2026-05-06T10:00:00Z",
		Dataset:   eval.ReportDataset{Name: "fi-core-v1", CaseCount: 1},
		Summary: map[string]eval.ParserSummary{
			"custom": {LemmaAccuracy: 0.10},
		},
	}
	newReport := &eval.Report{
		Generated: "2026-05-06T11:00:00Z",
		Dataset:   eval.ReportDataset{Name: "fi-core-v1", CaseCount: 1},
		Summary: map[string]eval.ParserSummary{
			"custom": {LemmaAccuracy: 0.90},
		},
	}

	writeReport(t, filepath.Join(dir, "2026-05-06-newer-generated.json"), newReport)
	writeReport(t, filepath.Join(dir, "2026-05-06-older-generated.json"), oldReport)

	// Make the older generated_at file newer on disk. The selector should
	// ignore filesystem mtime because git checkouts rewrite it.
	oldPath := filepath.Join(dir, "2026-05-06-older-generated.json")
	newPath := filepath.Join(dir, "2026-05-06-newer-generated.json")
	if err := os.Chtimes(newPath, mustParseTime(t, "2026-05-06T10:00:00Z"), mustParseTime(t, "2026-05-06T10:00:00Z")); err != nil {
		t.Fatalf("chtimes newer-generated: %v", err)
	}
	if err := os.Chtimes(oldPath, mustParseTime(t, "2026-05-06T12:00:00Z"), mustParseTime(t, "2026-05-06T12:00:00Z")); err != nil {
		t.Fatalf("chtimes older-generated: %v", err)
	}

	reports, err := loadBaselineDir(dir)
	if err != nil {
		t.Fatalf("loadBaselineDir: %v", err)
	}
	got := reports["fi-core-v1"]
	if got == nil {
		t.Fatal("missing fi-core-v1")
	}
	if got.Generated != newReport.Generated {
		t.Fatalf("selected generated_at=%q, want %q", got.Generated, newReport.Generated)
	}
	if got.Summary["custom"].LemmaAccuracy != 0.90 {
		t.Fatalf("selected wrong report: %#v", got.Summary["custom"])
	}
}

func writeReport(t *testing.T, path string, r *eval.Report) {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustParseTime(t *testing.T, raw string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("parse time %q: %v", raw, err)
	}
	return parsed
}
