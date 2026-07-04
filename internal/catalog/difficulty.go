package catalog

// This file holds the deterministic difficulty model. It is intentionally
// separate from I/O so cmd/gencatalog and its tests share one implementation:
// the checked-in catalog is exactly what BucketFromMetrics assigns, and the
// tests pin the thresholds so a behavior change fails loudly.
//
// Difficulty is a composite of normalized text-level signals, each clamped to
// [0,1] where higher = harder, then averaged with fixed weights into a Score
// in [0,1]. The Score is bucketed with four fixed cut points into five
// labels (easy, easy-medium, medium, medium-hard, hard). Every constant
// below is documented in docs/GO_LIVE_CHECKLIST.md ("Embedded catalog
// difficulty model") and pinned by difficulty_test.go.
//
// The signals were chosen to be robust on short texts (poems can be ~100
// tokens) and to lean on the parts of the parse we trust most: dictionary
// resolution, frequency of common words, and sentence length. Frequency rank
// is the strongest lexical-difficulty signal when a baseline is available;
// when it is not (MeanFrequencyRank < 0), its weight is redistributed to the
// remaining signals so a missing baseline never silently zeroes difficulty.

// Difficulty model constants. Kept as named values so the doc, the tests, and
// the code cannot drift.
const (
	// Normalization ceilings: the raw value that maps to 1.0 (hardest) for
	// each signal. Values above the ceiling clamp to 1.0.
	unresolvedRateCeiling    = 0.40  // 40% out-of-dictionary tokens -> max
	uniqueFormRatioFloor     = 0.45  // <=45% distinct forms/token -> 0 (very repetitive)
	uniqueFormRatioCeiling   = 0.85  // >=85% distinct forms/token -> 1 (little repetition)
	meanSentenceLenFloor     = 6.0   // <=6 tokens/sentence -> 0
	meanSentenceLenCeiling   = 24.0  // >=24 tokens/sentence -> 1
	frequencyRankCeiling     = 8000.0 // mean OpenSubtitles rank at/above this -> 1
	rareFormRateCeiling      = 0.55  // 55% rare/absent forms -> max
	featsPerTokenCeiling     = 0.28  // distinct FEATS strings per token at/above -> 1

	// Bucket cut points on the composite Score in [0,1]. The boundary bands
	// (easy-medium, medium-hard) are +/-0.05 around the original two-cut
	// model's 0.34 and 0.58; the 2026-07-04 FI human review showed real texts
	// clustering on those boundaries.
	easyEasyMediumCut   = 0.29
	easyMediumMediumCut = 0.39
	mediumMediumHardCut = 0.53
	mediumHardHardCut   = 0.63
)

// Signal weights. They sum to 1.0 across the full set; when the frequency
// signal is unavailable its weight is spread proportionally over the rest.
const (
	wUnresolved      = 0.22
	wUniqueFormRatio = 0.14
	wSentenceLen     = 0.16
	wFrequencyRank   = 0.24
	wRareForm        = 0.14
	wFeats           = 0.10
)

// clamp01 constrains x to [0,1].
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// normLinear maps raw in [lo,hi] to [0,1], clamped. If hi<=lo it returns 0.
func normLinear(raw, lo, hi float64) float64 {
	if hi <= lo {
		return 0
	}
	return clamp01((raw - lo) / (hi - lo))
}

// ScoreFromMetrics computes the composite difficulty score in [0,1] from the
// raw metrics. It is pure and deterministic.
func ScoreFromMetrics(m Metrics) float64 {
	unresolved := normLinear(m.UnresolvedRate, 0, unresolvedRateCeiling)
	variety := normLinear(m.UniqueFormRatio, uniqueFormRatioFloor, uniqueFormRatioCeiling)
	sentence := normLinear(m.MeanSentenceLength, meanSentenceLenFloor, meanSentenceLenCeiling)
	rare := normLinear(m.RareFormRate, 0, rareFormRateCeiling)

	featsPerToken := 0.0
	if m.TokenCount > 0 {
		featsPerToken = float64(m.FeatsVariety) / float64(m.TokenCount)
	}
	feats := normLinear(featsPerToken, 0, featsPerTokenCeiling)

	// Frequency signal is optional; -1 means no baseline was available.
	haveFreq := m.MeanFrequencyRank >= 0
	freq := 0.0
	if haveFreq {
		freq = normLinear(m.MeanFrequencyRank, 0, frequencyRankCeiling)
	}

	type wc struct {
		weight float64
		value  float64
	}
	parts := []wc{
		{wUnresolved, unresolved},
		{wUniqueFormRatio, variety},
		{wSentenceLen, sentence},
		{wRareForm, rare},
		{wFeats, feats},
	}
	if haveFreq {
		parts = append(parts, wc{wFrequencyRank, freq})
	}

	totalWeight := 0.0
	weighted := 0.0
	for _, p := range parts {
		totalWeight += p.weight
		weighted += p.weight * p.value
	}
	if totalWeight == 0 {
		return 0
	}
	// Renormalize so a dropped frequency weight redistributes proportionally.
	return clamp01(weighted / totalWeight)
}

// BucketFromScore maps a composite score to one of the five difficulty
// labels using the fixed cut points.
func BucketFromScore(score float64) Difficulty {
	switch {
	case score < easyEasyMediumCut:
		return DifficultyEasy
	case score < easyMediumMediumCut:
		return DifficultyEasyMedium
	case score < mediumMediumHardCut:
		return DifficultyMedium
	case score < mediumHardHardCut:
		return DifficultyMediumHard
	default:
		return DifficultyHard
	}
}

// BucketFromMetrics is the one-call deterministic classifier used by the
// generator. It returns the bucket and the score it was derived from.
func BucketFromMetrics(m Metrics) (Difficulty, float64) {
	score := ScoreFromMetrics(m)
	return BucketFromScore(score), score
}
