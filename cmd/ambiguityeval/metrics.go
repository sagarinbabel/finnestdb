package main

import "sort"

// SenseKey identifies a dictionary sense by (lemma, POS) - the same join key
// dictionary entries are keyed by (see docs/PARSER_EVAL_METHODOLOGY.md's
// Lemma+POS framing). Comparisons are case-sensitive exact matches: gold
// lemma/POS values are curated, and the parser's own casing convention
// (lowercase lemma, uppercase UD POS) is stable, so silently folding case
// would hide real mismatches instead of catching them.
type SenseKey struct {
	Lemma string
	POS   string
}

// Proxy is the structural confidence proxy defined in the methodology doc:
// there is no numeric confidence today, so calibration is measured against
// signals that already exist in BatchLookupAllForms' candidate set and the
// parser's own pick.
type Proxy string

const (
	// ProxySingle: the candidate set has exactly one (lemma, POS) for the
	// surface - treated as high-confidence.
	ProxySingle Proxy = "single"
	// ProxyMultiAgree: two or more distinct candidates, but the pick was
	// corroborated by both a dictionary row and an FST analysis (Source
	// contains "dict" and an "fst_" tag) - raised confidence within the
	// multi bucket.
	ProxyMultiAgree Proxy = "dict_fst_agree"
	// ProxyMulti: two or more distinct candidates with no dict/FST
	// corroboration - low confidence by default, the parser is choosing
	// among genuine homographs.
	ProxyMulti Proxy = "multi"
)

// CaseResult is one case's measured outcome, computed by the runner from a
// live parsecore.Analyze pick and store.BatchLookupAllForms candidate set.
// Kept independent of the DB/parser types so the metrics math (this file) is
// unit-testable without a database.
type CaseResult struct {
	CaseID         string
	AmbiguityClass string
	Expected       SenseKey
	// Candidates is the raw BatchLookupAllForms candidate set for the target
	// surface, deduped to distinct (lemma, POS) pairs.
	Candidates []SenseKey
	// Pick is the parser's single top pick (parsecore.Analyze "custom"
	// mode). PickFound is false if the target surface could not be located
	// in the parse output at all (should not happen for well-formed gold,
	// but kept explicit rather than treating a zero-value SenseKey as a
	// pick of ("", "")).
	Pick      SenseKey
	PickFound bool
	Proxy     Proxy
}

// CandidateInclusion reports whether the expected sense is present in the
// candidate set the product can actually offer. If false, neither the
// single Meaning Check nor Multiple possible meanings can be honestly shown
// for this case - the correct sense is unreachable regardless of ranking.
func (c CaseResult) CandidateInclusion() bool {
	for _, cand := range c.Candidates {
		if cand == c.Expected {
			return true
		}
	}
	return false
}

// SelectionCorrect reports whether the parser's single top pick matches the
// contextually-correct sense.
func (c CaseResult) SelectionCorrect() bool {
	return c.PickFound && c.Pick == c.Expected
}

// ClassMetrics is the per-ambiguity_class rollup the threshold rule (spec
// §4) operates on.
type ClassMetrics struct {
	Class                  string
	N                      int
	SelectionCorrect       int
	CandidateIncludedCount int
	// ProxyCounts / ProxyCorrect key by Proxy bucket for the
	// proxy-stratified accuracy table (spec §1.3).
	ProxyCounts  map[Proxy]int
	ProxyCorrect map[Proxy]int
}

func (m ClassMetrics) SelectionAccuracy() float64 {
	return ratio(m.SelectionCorrect, m.N)
}

func (m ClassMetrics) CandidateInclusionRate() float64 {
	return ratio(m.CandidateIncludedCount, m.N)
}

// MeetsThreshold applies the spec §4 gate: an ambiguity class may use the
// single Meaning Check UI only when selection accuracy >= 90% AND candidate
// inclusion = 100% AND N >= 4 target cases.
func (m ClassMetrics) MeetsThreshold() bool {
	return m.N >= 4 && m.CandidateInclusionRate() >= 1.0 && m.SelectionAccuracy() >= 0.90
}

// OverallMetrics is the headline rollup across every case in the dataset(s)
// evaluated, mirroring the "FI headline: selection accuracy X%; candidate
// inclusion Y%" framing in the methodology doc.
type OverallMetrics struct {
	N                      int
	SelectionCorrect       int
	CandidateIncludedCount int
}

func (m OverallMetrics) SelectionAccuracy() float64 {
	return ratio(m.SelectionCorrect, m.N)
}

func (m OverallMetrics) CandidateInclusionRate() float64 {
	return ratio(m.CandidateIncludedCount, m.N)
}

// ProxyMetrics is one row of the proxy-stratified accuracy table: of the
// cases whose structural proxy fell in this bucket, what fraction did
// selection get right. This is what tells us whether "single" is actually a
// trustworthy high-confidence signal.
type ProxyMetrics struct {
	Proxy   Proxy
	N       int
	Correct int
}

func (m ProxyMetrics) Accuracy() float64 {
	return ratio(m.Correct, m.N)
}

// ComputeClassMetrics groups case results by ambiguity_class and computes
// per-class metrics. Returns classes sorted by name for deterministic
// output ordering.
func ComputeClassMetrics(results []CaseResult) []ClassMetrics {
	byClass := make(map[string]*ClassMetrics)
	var order []string
	for _, r := range results {
		cm, ok := byClass[r.AmbiguityClass]
		if !ok {
			cm = &ClassMetrics{
				Class:        r.AmbiguityClass,
				ProxyCounts:  make(map[Proxy]int),
				ProxyCorrect: make(map[Proxy]int),
			}
			byClass[r.AmbiguityClass] = cm
			order = append(order, r.AmbiguityClass)
		}
		cm.N++
		if r.SelectionCorrect() {
			cm.SelectionCorrect++
		}
		if r.CandidateInclusion() {
			cm.CandidateIncludedCount++
		}
		cm.ProxyCounts[r.Proxy]++
		if r.SelectionCorrect() {
			cm.ProxyCorrect[r.Proxy]++
		}
	}
	sort.Strings(order)
	out := make([]ClassMetrics, 0, len(order))
	for _, name := range order {
		out = append(out, *byClass[name])
	}
	return out
}

// ComputeOverallMetrics rolls every case up into the headline numbers.
func ComputeOverallMetrics(results []CaseResult) OverallMetrics {
	var m OverallMetrics
	for _, r := range results {
		m.N++
		if r.SelectionCorrect() {
			m.SelectionCorrect++
		}
		if r.CandidateInclusion() {
			m.CandidateIncludedCount++
		}
	}
	return m
}

// ComputeProxyMetrics stratifies selection accuracy by structural confidence
// proxy bucket, sorted single, dict_fst_agree, multi for deterministic
// output (the natural confidence ordering, not alphabetical).
func ComputeProxyMetrics(results []CaseResult) []ProxyMetrics {
	counts := make(map[Proxy]int)
	correct := make(map[Proxy]int)
	for _, r := range results {
		counts[r.Proxy]++
		if r.SelectionCorrect() {
			correct[r.Proxy]++
		}
	}
	order := []Proxy{ProxySingle, ProxyMultiAgree, ProxyMulti}
	out := make([]ProxyMetrics, 0, len(order))
	for _, p := range order {
		if counts[p] == 0 {
			continue
		}
		out = append(out, ProxyMetrics{Proxy: p, N: counts[p], Correct: correct[p]})
	}
	return out
}

// ThresholdEligibleClasses returns the names of classes that meet the spec
// §4 gate, sorted for deterministic output.
func ThresholdEligibleClasses(classes []ClassMetrics) []string {
	var out []string
	for _, c := range classes {
		if c.MeetsThreshold() {
			out = append(out, c.Class)
		}
	}
	sort.Strings(out)
	return out
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
