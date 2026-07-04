package main

import (
	"reflect"
	"testing"
)

func TestCaseResult_CandidateInclusion(t *testing.T) {
	cases := []struct {
		name     string
		result   CaseResult
		expected bool
	}{
		{
			name: "expected sense present in candidate set",
			result: CaseResult{
				Expected:   SenseKey{Lemma: "kuusi", POS: "NOUN"},
				Candidates: []SenseKey{{Lemma: "kuusi", POS: "NOUN"}, {Lemma: "kuusi", POS: "NUM"}},
			},
			expected: true,
		},
		{
			name: "FI kaikki-gap case: NOUN sense absent from candidate set",
			result: CaseResult{
				Expected:   SenseKey{Lemma: "kuusi", POS: "NOUN"},
				Candidates: []SenseKey{{Lemma: "kuusi", POS: "NUM"}},
			},
			expected: false,
		},
		{
			name: "no candidates at all",
			result: CaseResult{
				Expected:   SenseKey{Lemma: "kuusi", POS: "NOUN"},
				Candidates: nil,
			},
			expected: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.result.CandidateInclusion(); got != c.expected {
				t.Errorf("CandidateInclusion() = %v, want %v", got, c.expected)
			}
		})
	}
}

func TestCaseResult_SelectionCorrect(t *testing.T) {
	cases := []struct {
		name     string
		result   CaseResult
		expected bool
	}{
		{
			name: "pick matches expected",
			result: CaseResult{
				Expected:  SenseKey{Lemma: "kuusi", POS: "NOUN"},
				Pick:      SenseKey{Lemma: "kuusi", POS: "NOUN"},
				PickFound: true,
			},
			expected: true,
		},
		{
			name: "pick has right lemma wrong POS - homograph miss",
			result: CaseResult{
				Expected:  SenseKey{Lemma: "kuusi", POS: "NOUN"},
				Pick:      SenseKey{Lemma: "kuusi", POS: "NUM"},
				PickFound: true,
			},
			expected: false,
		},
		{
			name: "target surface not found in parse output",
			result: CaseResult{
				Expected:  SenseKey{Lemma: "kuusi", POS: "NOUN"},
				PickFound: false,
			},
			expected: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.result.SelectionCorrect(); got != c.expected {
				t.Errorf("SelectionCorrect() = %v, want %v", got, c.expected)
			}
		})
	}
}

// TestComputeClassMetrics_FIKaikkiGapFailureMode models the documented FI
// failure mode from docs/PARSER_EVAL_METHODOLOGY.md §3: kaikki.org's forms
// table stores only one reading per surface for "kuusi", so the NOUN
// "spruce" sense never enters BatchLookupAllForms even though two of the
// four gold cases expect it. This verifies candidate inclusion correctly
// tracks reachability independent of whether the parser's pick happens to
// be right, and that a class failing inclusion never meets the threshold
// even when the pick is otherwise picking the majority candidate.
func TestComputeClassMetrics_FIKaikkiGapFailureMode(t *testing.T) {
	spruceExpected := SenseKey{Lemma: "kuusi", POS: "NOUN"}
	sixExpected := SenseKey{Lemma: "kuusi", POS: "NUM"}
	sixOnlyCandidates := []SenseKey{{Lemma: "kuusi", POS: "NUM"}} // NOUN sense missing from dict forms
	sixPick := SenseKey{Lemma: "kuusi", POS: "NUM"}               // parser can only ever pick what's in the candidate set

	results := []CaseResult{
		// Two cases where gold expects NOUN (spruce); dict only offers NUM,
		// so the pick can never be correct and inclusion always fails.
		{CaseID: "fi-amb-kuusi-1", AmbiguityClass: "kuusi", Expected: spruceExpected, Candidates: sixOnlyCandidates, Pick: sixPick, PickFound: true, Proxy: ProxyMulti},
		{CaseID: "fi-amb-kuusi-2", AmbiguityClass: "kuusi", Expected: spruceExpected, Candidates: sixOnlyCandidates, Pick: sixPick, PickFound: true, Proxy: ProxyMulti},
		// Two cases where gold expects NUM (six); dict offers NUM only, so
		// both inclusion and selection succeed.
		{CaseID: "fi-amb-kuusi-3", AmbiguityClass: "kuusi", Expected: sixExpected, Candidates: sixOnlyCandidates, Pick: sixPick, PickFound: true, Proxy: ProxyMulti},
		{CaseID: "fi-amb-kuusi-4", AmbiguityClass: "kuusi", Expected: sixExpected, Candidates: sixOnlyCandidates, Pick: sixPick, PickFound: true, Proxy: ProxyMulti},
	}

	classes := ComputeClassMetrics(results)
	if len(classes) != 1 {
		t.Fatalf("expected 1 class, got %d", len(classes))
	}
	kuusi := classes[0]
	if kuusi.N != 4 {
		t.Errorf("N = %d, want 4", kuusi.N)
	}
	if kuusi.SelectionCorrect != 2 {
		t.Errorf("selection correct = %d, want 2 (only the NUM-expected cases can be right)", kuusi.SelectionCorrect)
	}
	if kuusi.CandidateIncludedCount != 2 {
		t.Errorf("candidate inclusion count = %d, want 2 (only the NUM-expected cases are reachable)", kuusi.CandidateIncludedCount)
	}
	if got, want := kuusi.CandidateInclusionRate(), 0.5; got != want {
		t.Errorf("CandidateInclusionRate() = %v, want %v", got, want)
	}
	if kuusi.MeetsThreshold() {
		t.Error("kuusi should not meet threshold: candidate inclusion is not 100%")
	}
}

func TestClassMetrics_MeetsThreshold(t *testing.T) {
	cases := []struct {
		name     string
		metrics  ClassMetrics
		expected bool
	}{
		{
			name:     "meets all three gates",
			metrics:  ClassMetrics{N: 4, SelectionCorrect: 4, CandidateIncludedCount: 4},
			expected: true,
		},
		{
			name:     "fails N floor even at perfect accuracy",
			metrics:  ClassMetrics{N: 2, SelectionCorrect: 2, CandidateIncludedCount: 2},
			expected: false,
		},
		{
			name:     "fails candidate inclusion despite good selection",
			metrics:  ClassMetrics{N: 4, SelectionCorrect: 4, CandidateIncludedCount: 3},
			expected: false,
		},
		{
			name:     "fails selection threshold (89% < 90%)",
			metrics:  ClassMetrics{N: 100, SelectionCorrect: 89, CandidateIncludedCount: 100},
			expected: false,
		},
		{
			name:     "exactly at selection threshold (90%)",
			metrics:  ClassMetrics{N: 100, SelectionCorrect: 90, CandidateIncludedCount: 100},
			expected: true,
		},
		{
			name:     "N below floor with perfect scores still fails",
			metrics:  ClassMetrics{N: 3, SelectionCorrect: 3, CandidateIncludedCount: 3},
			expected: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.metrics.MeetsThreshold(); got != c.expected {
				t.Errorf("MeetsThreshold() = %v, want %v", got, c.expected)
			}
		})
	}
}

func TestThresholdEligibleClasses_SortedAndFiltered(t *testing.T) {
	classes := []ClassMetrics{
		{Class: "voi", N: 4, SelectionCorrect: 4, CandidateIncludedCount: 4},    // eligible
		{Class: "alusta", N: 4, SelectionCorrect: 0, CandidateIncludedCount: 2}, // not eligible
		{Class: "ilta", N: 4, SelectionCorrect: 4, CandidateIncludedCount: 4},   // eligible
	}
	got := ThresholdEligibleClasses(classes)
	want := []string{"ilta", "voi"} // sorted, "alusta" excluded
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ThresholdEligibleClasses() = %v, want %v", got, want)
	}
}

func TestComputeClassMetrics_DeterministicSortOrder(t *testing.T) {
	results := []CaseResult{
		{AmbiguityClass: "voi", Expected: SenseKey{Lemma: "voi", POS: "NOUN"}, Candidates: []SenseKey{{Lemma: "voi", POS: "NOUN"}}, Pick: SenseKey{Lemma: "voi", POS: "NOUN"}, PickFound: true},
		{AmbiguityClass: "alusta", Expected: SenseKey{Lemma: "alusta", POS: "NOUN"}, Candidates: []SenseKey{{Lemma: "alus", POS: "NOUN"}}, Pick: SenseKey{Lemma: "alus", POS: "NOUN"}, PickFound: true},
		{AmbiguityClass: "kuusi", Expected: SenseKey{Lemma: "kuusi", POS: "NOUN"}, Candidates: []SenseKey{{Lemma: "kuusi", POS: "NOUN"}}, Pick: SenseKey{Lemma: "kuusi", POS: "NOUN"}, PickFound: true},
	}
	classes := ComputeClassMetrics(results)
	var order []string
	for _, c := range classes {
		order = append(order, c.Class)
	}
	want := []string{"alusta", "kuusi", "voi"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("class order = %v, want %v (sorted)", order, want)
	}
}

func TestComputeOverallMetrics(t *testing.T) {
	results := []CaseResult{
		{Expected: SenseKey{Lemma: "a", POS: "NOUN"}, Pick: SenseKey{Lemma: "a", POS: "NOUN"}, PickFound: true, Candidates: []SenseKey{{Lemma: "a", POS: "NOUN"}}},
		{Expected: SenseKey{Lemma: "b", POS: "NOUN"}, Pick: SenseKey{Lemma: "c", POS: "NOUN"}, PickFound: true, Candidates: []SenseKey{{Lemma: "c", POS: "NOUN"}}},                            // wrong pick, inclusion also fails
		{Expected: SenseKey{Lemma: "d", POS: "NOUN"}, Pick: SenseKey{Lemma: "e", POS: "NOUN"}, PickFound: true, Candidates: []SenseKey{{Lemma: "d", POS: "NOUN"}, {Lemma: "e", POS: "NOUN"}}}, // wrong pick, inclusion succeeds
	}
	overall := ComputeOverallMetrics(results)
	if overall.N != 3 {
		t.Errorf("N = %d, want 3", overall.N)
	}
	if overall.SelectionCorrect != 1 {
		t.Errorf("SelectionCorrect = %d, want 1", overall.SelectionCorrect)
	}
	if overall.CandidateIncludedCount != 2 {
		t.Errorf("CandidateIncludedCount = %d, want 2", overall.CandidateIncludedCount)
	}
	if got, want := overall.SelectionAccuracy(), 1.0/3.0; got != want {
		t.Errorf("SelectionAccuracy() = %v, want %v", got, want)
	}
}

func TestComputeProxyMetrics_StratifiesAndOrdersByConfidence(t *testing.T) {
	results := []CaseResult{
		{Proxy: ProxyMulti, Expected: SenseKey{Lemma: "a", POS: "NOUN"}, Pick: SenseKey{Lemma: "a", POS: "NOUN"}, PickFound: true},
		{Proxy: ProxyMulti, Expected: SenseKey{Lemma: "b", POS: "NOUN"}, Pick: SenseKey{Lemma: "x", POS: "NOUN"}, PickFound: true},
		{Proxy: ProxySingle, Expected: SenseKey{Lemma: "c", POS: "NOUN"}, Pick: SenseKey{Lemma: "c", POS: "NOUN"}, PickFound: true},
		{Proxy: ProxyMultiAgree, Expected: SenseKey{Lemma: "d", POS: "NOUN"}, Pick: SenseKey{Lemma: "d", POS: "NOUN"}, PickFound: true},
	}
	got := ComputeProxyMetrics(results)
	if len(got) != 3 {
		t.Fatalf("expected 3 proxy buckets, got %d: %+v", len(got), got)
	}
	// Order must be single, dict_fst_agree, multi (confidence order), not
	// insertion or alphabetical order.
	wantOrder := []Proxy{ProxySingle, ProxyMultiAgree, ProxyMulti}
	for i, p := range got {
		if p.Proxy != wantOrder[i] {
			t.Errorf("proxy[%d] = %s, want %s", i, p.Proxy, wantOrder[i])
		}
	}
	for _, p := range got {
		switch p.Proxy {
		case ProxySingle:
			if p.N != 1 || p.Correct != 1 {
				t.Errorf("single bucket = %+v, want N=1 Correct=1", p)
			}
		case ProxyMultiAgree:
			if p.N != 1 || p.Correct != 1 {
				t.Errorf("dict_fst_agree bucket = %+v, want N=1 Correct=1", p)
			}
		case ProxyMulti:
			if p.N != 2 || p.Correct != 1 {
				t.Errorf("multi bucket = %+v, want N=2 Correct=1", p)
			}
		}
	}
}

func TestComputeProxyMetrics_OmitsEmptyBuckets(t *testing.T) {
	results := []CaseResult{
		{Proxy: ProxySingle, Expected: SenseKey{Lemma: "a", POS: "NOUN"}, Pick: SenseKey{Lemma: "a", POS: "NOUN"}, PickFound: true},
	}
	got := ComputeProxyMetrics(results)
	if len(got) != 1 {
		t.Fatalf("expected 1 populated bucket, got %d: %+v", len(got), got)
	}
}

func TestRatio_ZeroDenominator(t *testing.T) {
	if got := ratio(5, 0); got != 0 {
		t.Errorf("ratio(5, 0) = %v, want 0", got)
	}
}
