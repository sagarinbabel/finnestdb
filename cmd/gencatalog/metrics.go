package main

import (
	"sort"
	"strings"

	"finnestdb/internal/catalog"
	"finnestdb/internal/parsecore"
)

// rareFormRankThreshold is the frequency rank beyond which a form counts as
// "rare" for the rare-form-rate signal. A form absent from the baseline is
// also rare. 6000 sits inside the 50k list but well past everyday vocabulary.
const rareFormRankThreshold = 6000

// computeMetrics derives the text-level difficulty metrics from a real parse
// result and an optional frequency-rank map (nil when no baseline is
// available). It is pure so difficulty is reproducible and unit-testable
// without the FFI parser or a database.
//
// Frequency signals are computed over resolved non-punct tokens, keyed by
// lowercased surface form. When freqRanks is nil, MeanFrequencyRank is set to
// -1 (the "no baseline" sentinel the difficulty model redistributes weight
// around) and RareFormRate is computed structurally as the unresolved rate's
// companion is not - instead RareFormRate is left at 0 so it does not
// double-count with the unresolved signal.
func computeMetrics(res *parsecore.ParseResult, freqRanks map[string]int) catalog.Metrics {
	m := catalog.Metrics{
		TokenCount:       res.TotalTokens,
		SentenceCount:    res.Stats.TotalSentences,
		UniqueLemmaCount: len(res.Words),
	}

	totalTokens := res.Stats.ResolvedTokens + res.Stats.UnresolvedTokens
	if totalTokens > 0 {
		m.UnresolvedRate = float64(res.Stats.UnresolvedTokens) / float64(totalTokens)
	}
	if res.Stats.TotalSentences > 0 {
		m.MeanSentenceLength = float64(totalTokens) / float64(res.Stats.TotalSentences)
	}

	// Unique surface forms over non-punct tokens, and FEATS variety, come from
	// the aggregated word list (Forms + Feats per (lemma,pos) entry). We also
	// walk sentences for the frequency signal because Words drop per-token
	// form frequency but keep the form set.
	uniqueForms := map[string]struct{}{}
	featsSet := map[string]struct{}{}
	for _, w := range res.Words {
		for _, f := range w.Forms {
			uniqueForms[strings.ToLower(f)] = struct{}{}
		}
		if w.Feats != "" {
			featsSet[w.Feats] = struct{}{}
		}
	}
	m.FeatsVariety = len(featsSet)
	if totalTokens > 0 {
		m.UniqueFormRatio = float64(len(uniqueForms)) / float64(totalTokens)
	}

	// Frequency signals over resolved tokens.
	if freqRanks != nil {
		var rankSum, rankedCount, rareCount, freqEligible float64
		for _, sent := range res.Sentences {
			for _, tok := range sent.Tokens {
				if tok.POS == "PUNCT" {
					continue
				}
				freqEligible++
				form := strings.ToLower(tok.Form)
				if r, ok := freqRanks[form]; ok {
					rankSum += float64(r)
					rankedCount++
					if r > rareFormRankThreshold {
						rareCount++
					}
				} else {
					// Absent from the top-50k baseline: treat as rare.
					rareCount++
				}
			}
		}
		if rankedCount > 0 {
			m.MeanFrequencyRank = rankSum / rankedCount
		} else {
			m.MeanFrequencyRank = -1
		}
		if freqEligible > 0 {
			m.RareFormRate = rareCount / freqEligible
		}
	} else {
		m.MeanFrequencyRank = -1
	}

	// Round the derived metric fields BEFORE scoring so the stored Score is
	// exactly what BucketFromMetrics recomputes from the checked-in (rounded)
	// metrics - otherwise a re-derivation from the JSON would drift in the
	// last digit and break the reproducibility guarantee.
	m.UnresolvedRate = round4(m.UnresolvedRate)
	m.UniqueFormRatio = round4(m.UniqueFormRatio)
	m.RareFormRate = round4(m.RareFormRate)
	m.MeanSentenceLength = round4(m.MeanSentenceLength)
	if m.MeanFrequencyRank >= 0 {
		m.MeanFrequencyRank = round4(m.MeanFrequencyRank)
	}
	m.Score = round4(catalog.ScoreFromMetrics(m))
	return m
}

// lemmaCounts converts the parse result's word list into the precomputed
// (lemma, pos, count) list stored per catalog entry, sorted for stable output.
func lemmaCounts(res *parsecore.ParseResult) []catalog.LemmaCount {
	out := make([]catalog.LemmaCount, 0, len(res.Words))
	for _, w := range res.Words {
		if w.POS == "PUNCT" || strings.TrimSpace(w.Lemma) == "" {
			continue
		}
		out = append(out, catalog.LemmaCount{Lemma: w.Lemma, POS: w.POS, Count: w.Count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Lemma != out[j].Lemma {
			return out[i].Lemma < out[j].Lemma
		}
		return out[i].POS < out[j].POS
	})
	return out
}

func round4(x float64) float64 {
	return float64(int64(x*10000+sign(x)*0.5)) / 10000
}

func sign(x float64) float64 {
	if x < 0 {
		return -1
	}
	return 1
}
