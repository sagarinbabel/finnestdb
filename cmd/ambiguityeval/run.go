package main

import (
	"fmt"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/store"
)

// EvaluateDataset runs one ambiguity gold dataset against the "custom"
// parser and returns one CaseResult per case, in gold-file order. For each
// case it runs parsecore.Analyze for the single pick and
// store.BatchLookupAllFormsWithOptions (MergeFSTReadings) for the candidate
// set on the target surface - the dict candidates the Multi-Lemma Surface
// expansion offers, plus the FST-known homograph readings kaikki's `forms`
// table omits, which is the candidate set the "Multiple possible meanings"
// meaning-check UI needs (see PARSER_EVAL_METHODOLOGY.md §Ambiguity).
func EvaluateDataset(db *store.DB, dataset *Dataset) ([]CaseResult, error) {
	results := make([]CaseResult, 0, len(dataset.Cases))
	for _, c := range dataset.Cases {
		result, err := evaluateCase(db, dataset.Language, c)
		if err != nil {
			return nil, fmt.Errorf("case %s: %w", c.ID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func evaluateCase(db *store.DB, lang string, c Case) (CaseResult, error) {
	target, err := targetToken(c)
	if err != nil {
		return CaseResult{}, err
	}

	parsed, err := parsecore.Analyze(db, lang, c.Text, "custom")
	if err != nil {
		return CaseResult{}, fmt.Errorf("analyze: %w", err)
	}

	occurrence := target.Occurrence
	if occurrence <= 0 {
		occurrence = 1
	}
	pick, found := findOccurrence(parsed.Sentences, target.Surface, occurrence)

	// Measure the ambiguity / meaning-check candidate set: dict candidates
	// merged with FST-known homograph readings the `forms` table omits. This
	// is the inclusion lever the "Multiple possible meanings" UI depends on;
	// the deck-expansion path deliberately stays dict-only (see
	// store.AllFormsOptions.MergeFSTReadings).
	candSet := db.BatchLookupAllFormsWithOptions([]string{target.Surface}, lang, "custom", store.AllFormsOptions{MergeFSTReadings: true})
	candidates := dedupeCandidates(candSet[target.Surface])

	result := CaseResult{
		CaseID:         c.ID,
		AmbiguityClass: c.AmbiguityClass,
		Expected:       SenseKey{Lemma: target.Lemma, POS: target.POS},
		Candidates:     candidates,
		PickFound:      found,
		Proxy:          ClassifyProxy(len(candidates), pick.Source),
	}
	if found {
		result.Pick = SenseKey{Lemma: pick.Lemma, POS: pick.POS}
	}
	return result, nil
}

func dedupeCandidates(resolutions []store.FormResolution) []SenseKey {
	if len(resolutions) == 0 {
		return nil
	}
	seen := make(map[SenseKey]struct{}, len(resolutions))
	out := make([]SenseKey, 0, len(resolutions))
	for _, r := range resolutions {
		if r.Lemma == "" || r.POS == "" {
			continue
		}
		key := SenseKey{Lemma: r.Lemma, POS: r.POS}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

// findOccurrence locates the Nth non-PUNCT occurrence of surface within the
// parsed sentences, mirroring internal/eval.findOccurrence's matching rule
// (exact Form match, 1-based occurrence, PUNCT excluded from the count).
func findOccurrence(sentences []parsecore.SentenceResult, surface string, occurrence int) (parsecore.TokenResult, bool) {
	count := 0
	for _, sentence := range sentences {
		for _, token := range sentence.Tokens {
			if token.POS == "PUNCT" {
				continue
			}
			if token.Form != surface {
				continue
			}
			count++
			if count == occurrence {
				return token, true
			}
		}
	}
	return parsecore.TokenResult{}, false
}
