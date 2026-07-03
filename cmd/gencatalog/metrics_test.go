package main

import (
	"testing"

	"finnestdb/internal/catalog"
	"finnestdb/internal/parsecore"
)

// buildResult constructs a synthetic parse result so metric computation can be
// tested deterministically without the FFI parser or a database.
func buildResult() *parsecore.ParseResult {
	return &parsecore.ParseResult{
		Lang:        "FI",
		Parser:      "custom",
		TotalTokens: 6,
		Stats: parsecore.ParseStats{
			TotalSentences:   2,
			ResolvedTokens:   5,
			UnresolvedTokens: 1,
		},
		Words: []parsecore.WordEntry{
			{Lemma: "kissa", POS: "NOUN", Forms: []string{"kissa", "kissan"}, Count: 2, Feats: "Case=Nom|Number=Sing"},
			{Lemma: "juoda", POS: "VERB", Forms: []string{"juo"}, Count: 1},
			{Lemma: "aurinko", POS: "NOUN", Forms: []string{"aurinko"}, Count: 1, Feats: "Case=Nom|Number=Sing"},
			{Lemma: "zzxq", POS: "NOUN", Forms: []string{"zzxq"}, Count: 1}, // unresolved-ish rare form
			{Lemma: "ja", POS: "CCONJ", Forms: []string{"ja"}, Count: 1},
		},
		Sentences: []parsecore.SentenceResult{
			{Text: "Kissa juo.", Tokens: []parsecore.TokenResult{
				{Form: "Kissa", Lemma: "kissa", POS: "NOUN", Resolved: true},
				{Form: "juo", Lemma: "juoda", POS: "VERB", Resolved: true},
				{Form: ".", Lemma: ".", POS: "PUNCT"},
			}},
			{Text: "Aurinko ja kissan zzxq.", Tokens: []parsecore.TokenResult{
				{Form: "Aurinko", Lemma: "aurinko", POS: "NOUN", Resolved: true},
				{Form: "ja", Lemma: "ja", POS: "CCONJ", Resolved: true},
				{Form: "kissan", Lemma: "kissa", POS: "NOUN", Resolved: true},
				{Form: "zzxq", Lemma: "zzxq", POS: "NOUN", Resolved: false},
			}},
		},
	}
}

func TestComputeMetricsNoFrequency(t *testing.T) {
	m := computeMetrics(buildResult(), nil)

	if m.TokenCount != 6 {
		t.Errorf("TokenCount = %d, want 6", m.TokenCount)
	}
	if m.SentenceCount != 2 {
		t.Errorf("SentenceCount = %d, want 2", m.SentenceCount)
	}
	if m.UniqueLemmaCount != 5 {
		t.Errorf("UniqueLemmaCount = %d, want 5", m.UniqueLemmaCount)
	}
	// 1 unresolved / 6 total.
	if got := m.UnresolvedRate; got < 0.166 || got > 0.167 {
		t.Errorf("UnresolvedRate = %.4f, want ~0.1667", got)
	}
	// 6 unique forms (kissa,kissan,juo,aurinko,zzxq,ja) / 6 tokens = 1.0.
	if m.UniqueFormRatio != 1.0 {
		t.Errorf("UniqueFormRatio = %.4f, want 1.0", m.UniqueFormRatio)
	}
	if m.FeatsVariety != 1 { // both "Case=Nom|Number=Sing" collapse to one distinct string
		t.Errorf("FeatsVariety = %d, want 1", m.FeatsVariety)
	}
	if m.MeanSentenceLength != 3.0 {
		t.Errorf("MeanSentenceLength = %.4f, want 3.0", m.MeanSentenceLength)
	}
	if m.MeanFrequencyRank != -1 {
		t.Errorf("MeanFrequencyRank = %.1f, want -1 (no baseline)", m.MeanFrequencyRank)
	}
	// Score must be deterministic and bucketable.
	if m.Score < 0 || m.Score > 1 {
		t.Errorf("Score out of range: %.4f", m.Score)
	}
	// The stored Score must be reproducible from the checked-in (rounded)
	// metrics: recomputing from m yields the same rounded value.
	if _, s := catalog.BucketFromMetrics(m); round4(s) != m.Score {
		t.Errorf("stored Score %.4f not reproducible from metrics: %.4f", m.Score, round4(s))
	}
}

func TestComputeMetricsWithFrequency(t *testing.T) {
	freq := map[string]int{
		"kissa": 100, "kissan": 4000, "juo": 200, "aurinko": 900, "ja": 3,
		// "zzxq" absent -> rare
	}
	m := computeMetrics(buildResult(), freq)
	if m.MeanFrequencyRank <= 0 {
		t.Fatalf("expected a positive mean frequency rank, got %.2f", m.MeanFrequencyRank)
	}
	// rare = forms with rank>6000 or absent. Only "zzxq" is absent; the other
	// ranks are all under 6000. 1 rare / 6 non-punct tokens = 0.1667.
	if got := m.RareFormRate; got < 0.166 || got > 0.167 {
		t.Errorf("RareFormRate = %.4f, want ~0.1667", got)
	}
}

func TestLemmaCountsSortedAndPunctFiltered(t *testing.T) {
	lc := lemmaCounts(buildResult())
	if len(lc) != 5 {
		t.Fatalf("expected 5 lemma entries, got %d", len(lc))
	}
	for i := 1; i < len(lc); i++ {
		if lc[i-1].Lemma > lc[i].Lemma {
			t.Errorf("lemma list not sorted: %q before %q", lc[i-1].Lemma, lc[i].Lemma)
		}
	}
	for _, l := range lc {
		if l.POS == "PUNCT" {
			t.Errorf("punct leaked into lemma list: %+v", l)
		}
	}
}
