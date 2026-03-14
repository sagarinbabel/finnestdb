package api

import (
	"sort"
	"strings"

	"finnestdb/internal/store"
)

// ParsedToken holds the minimal per-token data needed for enrichment.
// Using plain Go types here (no parserffi import) keeps the enrichment
// logic decoupled from the CGO layer, making it easy to unit-test.
type ParsedToken struct {
	Form      string // surface form as it appears in text
	StubLemma string // parser's lemma guess (used as fallback when form not in dictionary)
	StubPOS   string // parser's POS guess (fallback)
}

// ParsedSentence groups a sentence's tokens with its reconstructed text.
type ParsedSentence struct {
	Tokens []ParsedToken
	Text   string // pre-reconstructed text (space-joined forms)
}

// enrichWords aggregates parsed tokens into WordEntry structs enriched with
// real lemmas, POS tags, glosses, and an example sentence from the input text.
//
// This is a pure function — no DB access. All dictionary interaction is done
// upstream (BatchLookupForms + BatchLookupGlosses) and passed in as maps.
//
// Data flow:
//
//	sentences[i].Tokens (PUNCT tokens skipped)
//	      │
//	      ▼ per token: form → formResolutions
//	      │   hit  → FormResolution{Lemma, POS, GrammarLabel, Source}
//	      │   miss → stub (StubLemma lowercased, StubPOS) as fallback
//	      ▼
//	aggregate by (lemma, pos)
//	      │   ├─ count occurrences
//	      │   ├─ collect unique surface forms
//	      │   └─ record first-seen sentence text + grammar label
//	      ▼
//	gloss lookup via glosses map
//	      ▼
//	[]WordEntry sorted by count desc, lemma asc
func enrichWords(
	sentences []ParsedSentence,
	formResolutions map[string]store.FormResolution,
	glosses map[store.LemmaKey]string,
) []WordEntry {
	type key struct{ lemma, pos string }
	counts := make(map[key]int)
	formSets := make(map[key]map[string]struct{})
	firstSentence := make(map[key]string)
	grammarLabels := make(map[key]string) // first non-empty grammar label for this lemma

	for _, sent := range sentences {
		for _, token := range sent.Tokens {
			if strings.TrimSpace(token.Form) == "" {
				continue
			}
			// Skip punctuation tokens — they shouldn't appear in the word list.
			if token.StubPOS == "PUNCT" {
				continue
			}

			var lemma, pos, grammarLabel string
			if resolved, ok := formResolutions[token.Form]; ok {
				lemma, pos = resolved.Lemma, resolved.POS
				grammarLabel = resolved.GrammarLabel
			} else {
				lemma = strings.ToLower(token.StubLemma)
				if lemma == "" {
					lemma = strings.ToLower(token.Form)
				}
				pos = token.StubPOS
			}
			if lemma == "" {
				continue
			}

			k := key{lemma: lemma, pos: pos}
			counts[k]++
			if formSets[k] == nil {
				formSets[k] = make(map[string]struct{})
			}
			formSets[k][token.Form] = struct{}{}
			if _, seen := firstSentence[k]; !seen && sent.Text != "" {
				firstSentence[k] = sent.Text
			}
			// Record the first non-empty grammar label for this lemma.
			if grammarLabel != "" {
				if _, seen := grammarLabels[k]; !seen {
					grammarLabels[k] = grammarLabel
				}
			}
		}
	}

	words := make([]WordEntry, 0, len(counts))
	for k, count := range counts {
		forms := make([]string, 0, len(formSets[k]))
		for f := range formSets[k] {
			forms = append(forms, f)
		}
		sort.Strings(forms)
		lk := store.LemmaKey{Lemma: k.lemma, POS: k.pos}
		words = append(words, WordEntry{
			Lemma:           k.lemma,
			POS:             k.pos,
			Forms:           forms,
			Count:           count,
			Gloss:           glosses[lk],
			GrammarLabel:    grammarLabels[k],
			ExampleSentence: firstSentence[k],
		})
	}

	// Sort by count descending, then lemma alphabetically for stable output.
	sort.Slice(words, func(i, j int) bool {
		if words[i].Count != words[j].Count {
			return words[i].Count > words[j].Count
		}
		return words[i].Lemma < words[j].Lemma
	})
	return words
}
