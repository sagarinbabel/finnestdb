package eval

// Stratified eval breakdowns. The headline accuracy on a real-world treebank
// like UD-TDT (53.4% lemma) hides a wide spread: ~90%+ open-class buried under
// ~20% PROPN/NUM. Without stratification a regression in one slice (e.g. PROPN
// dropping from 60% → 30%) can be invisible in the rolled-up number when the
// dominant slice (open-class NOUN/VERB) hasn't moved. See
// docs/LEARNINGS.md §2026-05-07 - UD-TDT.
//
// Three independent axes, computed from the existing per-token comparisons
// already stored in CaseReport.Comparisons:
//
//  1. UPOS bucket - open/closed/propn/num/punct/other on the gold POS.
//  2. OOV - in-dict (parser resolved the surface) vs oov (didn't).
//  3. Compoundness - surface contains a hyphen, or parser flagged it as a
//     compound split. Simple = neither.
//
// Stratification is opt-in: cmd/parsertest -stratified attaches it to each
// ParserSummary and writes a sidecar <run>.stratified.md;
// cmd/parser-compare -stratified computes it on the fly from any report JSON
// (so it also works on historical baselines that predate the flag).

import (
	"sort"
	"strings"
)

// Stratification holds per-bucket accuracy breakdowns along three axes. The
// pointer is omitted from JSON when nil so reports without stratification
// stay byte-identical to the legacy schema.
type Stratification struct {
	ByUPOS     []StratifiedMetric `json:"by_upos,omitempty"`
	ByOOV      []StratifiedMetric `json:"by_oov,omitempty"`
	ByCompound []StratifiedMetric `json:"by_compound,omitempty"`
}

// StratifiedMetric is one row in a stratified breakdown. ExpectedTokens is the
// denominator for Lemma/POS/Lemma+POS/Full accuracy; counts are the numerators
// kept so the JSON is auditable without recomputing from cases. LemmaPOS is
// the joint attachment metric (lemma AND POS both correct) that became
// first-class on 2026-05-07; on languages with NOUN/NUM/PROPN homographs it
// is the primary signal for "did this surface land on the right
// (lemma, POS) dictionary entry?"
type StratifiedMetric struct {
	Bucket           string  `json:"bucket"`
	ExpectedTokens   int     `json:"expected_tokens"`
	LemmaCorrect     int     `json:"lemma_correct"`
	POSCorrect       int     `json:"pos_correct"`
	LemmaPOSCorrect  int     `json:"lemma_pos_correct"`
	FullCorrect      int     `json:"full_correct"`
	LemmaAccuracy    float64 `json:"lemma_accuracy"`
	POSAccuracy      float64 `json:"pos_accuracy"`
	LemmaPOSAccuracy float64 `json:"lemma_pos_accuracy"`
	FullAccuracy     float64 `json:"full_accuracy"`
}

// uposBucket maps a UD POS tag to the bucket label used in stratification.
// Open vs closed follows the standard linguistic split; PROPN and NUM are
// pulled out because they are where headline accuracy hides regressions on
// real-world treebanks. PUNCT is included for completeness even though
// compareCase already excludes it.
func uposBucket(pos string) string {
	switch pos {
	case "NOUN", "VERB", "ADJ", "ADV":
		return "open"
	case "DET", "PRON", "CCONJ", "SCONJ", "ADP", "AUX", "PART", "INTJ":
		return "closed"
	case "PROPN":
		return "propn"
	case "NUM":
		return "num"
	case "PUNCT":
		return "punct"
	default:
		return "other"
	}
}

// oovBucket reports whether the parser resolved this surface to a dict entry.
// in-dict means Actual.Resolved == true at compare time; oov includes both
// the unresolved-but-found case and the not-found case.
func oovBucket(cmp TokenCompare) string {
	if cmp.Actual.Found && cmp.Actual.Resolved {
		return "in-dict"
	}
	return "oov"
}

// compoundBucket flags the surface as compound when it contains a hyphen
// ("EU-puhejohtaja", "B1-tase") or when the parser's compound-split rule was
// the resolution source. This is intentionally the cheap version - a future
// pass could call into the FST to recognise compounds whose halves don't
// touch a hyphen, but the current heuristic catches every category we
// currently regress on.
func compoundBucket(cmp TokenCompare) string {
	if strings.Contains(cmp.Surface, "-") {
		return "compound"
	}
	if cmp.Actual.Source == "compound" {
		return "compound"
	}
	return "simple"
}

// uposBucketOrder is the canonical row order for the by-UPOS table.
// Sorted "most-informative-first" rather than alphabetical: open dominates
// volume, closed is the second-largest slice, then propn / num / punct /
// other in descending order of how often regressions show up there.
var uposBucketOrder = []string{"open", "closed", "propn", "num", "punct", "other"}

var oovBucketOrder = []string{"in-dict", "oov"}

var compoundBucketOrder = []string{"compound", "simple"}

// ComputeStratification walks one parser's comparisons across all cases and
// returns the three-axis breakdown. The argument is the slice of cases from
// a Report - works on freshly-evaluated reports and historical baseline
// JSONs alike.
//
// Returns nil if the report has zero comparisons for the parser, so callers
// can attach it via *Stratification without fabricating empty rows.
func ComputeStratification(parser string, cases []CaseReport) *Stratification {
	byUPOS := make(map[string]*stratCounters, len(uposBucketOrder))
	byOOV := make(map[string]*stratCounters, len(oovBucketOrder))
	byCompound := make(map[string]*stratCounters, len(compoundBucketOrder))

	any := false
	for _, c := range cases {
		cmps, ok := c.Comparisons[parser]
		if !ok {
			continue
		}
		for _, cmp := range cmps {
			any = true
			incrementBucket(byUPOS, uposBucket(cmp.Expected.POS), cmp)
			incrementBucket(byOOV, oovBucket(cmp), cmp)
			incrementBucket(byCompound, compoundBucket(cmp), cmp)
		}
	}
	if !any {
		return nil
	}
	return &Stratification{
		ByUPOS:     finalizeBuckets(byUPOS, uposBucketOrder),
		ByOOV:     finalizeBuckets(byOOV, oovBucketOrder),
		ByCompound: finalizeBuckets(byCompound, compoundBucketOrder),
	}
}

// stratCounters is the per-bucket accumulator for a single axis. Kept
// internal because the public surface is the StratifiedMetric value type.
type stratCounters struct {
	expected        int
	lemmaCorrect    int
	posCorrect      int
	lemmaPOSCorrect int
	fullCorrect     int
}

func incrementBucket(buckets map[string]*stratCounters, name string, cmp TokenCompare) {
	c, ok := buckets[name]
	if !ok {
		c = &stratCounters{}
		buckets[name] = c
	}
	c.expected++
	if cmp.Match.Lemma {
		c.lemmaCorrect++
	}
	if cmp.Match.POS {
		c.posCorrect++
	}
	if cmp.Match.Lemma && cmp.Match.POS {
		c.lemmaPOSCorrect++
	}
	if cmp.Match.Full {
		c.fullCorrect++
	}
}

// finalizeBuckets renders the accumulated counters into StratifiedMetric
// rows in a deterministic order. Empty buckets (zero expected tokens) are
// omitted so a dataset without PROPN/NUM doesn't print a row of zeros that
// invites the wrong reading. Buckets not in the canonical order are
// appended alphabetically - defensive guard for future POS tags.
func finalizeBuckets(buckets map[string]*stratCounters, order []string) []StratifiedMetric {
	out := make([]StratifiedMetric, 0, len(buckets))
	seen := make(map[string]bool, len(order))
	for _, name := range order {
		seen[name] = true
		c, ok := buckets[name]
		if !ok || c.expected == 0 {
			continue
		}
		out = append(out, metricFromCounters(name, c))
	}
	extras := make([]string, 0)
	for name, c := range buckets {
		if seen[name] {
			continue
		}
		if c.expected == 0 {
			continue
		}
		extras = append(extras, name)
	}
	sort.Strings(extras)
	for _, name := range extras {
		out = append(out, metricFromCounters(name, buckets[name]))
	}
	return out
}

func metricFromCounters(name string, c *stratCounters) StratifiedMetric {
	return StratifiedMetric{
		Bucket:           name,
		ExpectedTokens:   c.expected,
		LemmaCorrect:     c.lemmaCorrect,
		POSCorrect:       c.posCorrect,
		LemmaPOSCorrect:  c.lemmaPOSCorrect,
		FullCorrect:      c.fullCorrect,
		LemmaAccuracy:    ratio(c.lemmaCorrect, c.expected),
		POSAccuracy:      ratio(c.posCorrect, c.expected),
		LemmaPOSAccuracy: ratio(c.lemmaPOSCorrect, c.expected),
		FullAccuracy:     ratio(c.fullCorrect, c.expected),
	}
}
