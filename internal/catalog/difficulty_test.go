package catalog

import "testing"

// These tests pin the deterministic difficulty model so a threshold or weight
// change fails loudly. They encode WHY each bucket is assigned, not just the
// arithmetic: an all-common, short-sentence, fully-resolved text must be easy;
// a rare-vocabulary, high-unresolved, long-sentence text must be hard.

func TestBucketFromScoreCutPoints(t *testing.T) {
	cases := []struct {
		score float64
		want  Difficulty
	}{
		{0.0, DifficultyEasy},
		{easyMediumCut - 0.001, DifficultyEasy},
		{easyMediumCut, DifficultyMedium},
		{mediumHardCut - 0.001, DifficultyMedium},
		{mediumHardCut, DifficultyHard},
		{1.0, DifficultyHard},
	}
	for _, c := range cases {
		if got := BucketFromScore(c.score); got != c.want {
			t.Errorf("BucketFromScore(%.3f) = %q, want %q", c.score, got, c.want)
		}
	}
}

func TestEasyTextIsEasy(t *testing.T) {
	// A beginner text: very common words (low mean rank), few rare forms,
	// short sentences, everything resolved, little inflectional variety.
	m := Metrics{
		TokenCount:         150,
		SentenceCount:      25,
		UnresolvedRate:     0.0,
		UniqueFormRatio:    0.50,
		MeanFrequencyRank:  1500,
		RareFormRate:       0.08,
		FeatsVariety:       10,
		MeanSentenceLength: 6.0,
	}
	diff, score := BucketFromMetrics(m)
	if diff != DifficultyEasy {
		t.Fatalf("expected easy, got %q (score=%.3f)", diff, score)
	}
}

func TestHardTextIsHard(t *testing.T) {
	// A dense literary/poetic text: rare vocabulary, high unresolved rate,
	// long sentences, high lexical variety, rich morphology.
	m := Metrics{
		TokenCount:         150,
		SentenceCount:      6,
		UnresolvedRate:     0.30,
		UniqueFormRatio:    0.90,
		MeanFrequencyRank:  8500,
		RareFormRate:       0.60,
		FeatsVariety:       45,
		MeanSentenceLength: 25.0,
	}
	diff, score := BucketFromMetrics(m)
	if diff != DifficultyHard {
		t.Fatalf("expected hard, got %q (score=%.3f)", diff, score)
	}
}

// A missing frequency baseline must not silently collapse difficulty toward
// easy: the frequency weight is redistributed over the remaining signals, so
// an otherwise-hard text stays hard even with no frequency data.
func TestMissingFrequencyBaselineRedistributes(t *testing.T) {
	withFreq := Metrics{
		TokenCount:         150,
		SentenceCount:      6,
		UnresolvedRate:     0.30,
		UniqueFormRatio:    0.90,
		MeanFrequencyRank:  8500,
		RareFormRate:       0.60,
		FeatsVariety:       45,
		MeanSentenceLength: 25.0,
	}
	noFreq := withFreq
	noFreq.MeanFrequencyRank = -1
	noFreq.RareFormRate = 0 // rare-form also depends on the baseline

	if got := BucketFromScore(ScoreFromMetrics(noFreq)); got != DifficultyHard {
		t.Fatalf("no-baseline hard text should stay hard, got %q (score=%.3f)",
			got, ScoreFromMetrics(noFreq))
	}
	// Score with the baseline should be at least as high as without it here,
	// since the baseline adds a strong hard signal.
	if ScoreFromMetrics(withFreq) < ScoreFromMetrics(noFreq)-0.2 {
		t.Errorf("dropping the frequency baseline changed score too much: with=%.3f without=%.3f",
			ScoreFromMetrics(withFreq), ScoreFromMetrics(noFreq))
	}
}

func TestScoreIsDeterministicAndBounded(t *testing.T) {
	m := Metrics{
		TokenCount: 100, SentenceCount: 10, UnresolvedRate: 0.5,
		UniqueFormRatio: 1.0, MeanFrequencyRank: 12000, RareFormRate: 1.0,
		FeatsVariety: 100, MeanSentenceLength: 40,
	}
	s1 := ScoreFromMetrics(m)
	s2 := ScoreFromMetrics(m)
	if s1 != s2 {
		t.Fatalf("ScoreFromMetrics not deterministic: %.6f vs %.6f", s1, s2)
	}
	if s1 < 0 || s1 > 1 {
		t.Fatalf("score out of [0,1]: %.6f", s1)
	}
}
