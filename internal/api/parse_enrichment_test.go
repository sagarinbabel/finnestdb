package api

// Tests for enrichWords(), collectUniqueForms(), and toParsedSentences().
//
// These tests construct ParsedSentence / ParsedToken data directly, without
// calling parserffi.AnalyzeText — so they run fast and don't depend on the
// Rust parser binary being present.
//
// Data-flow coverage:
//
//	ParsedSentence{Tokens, Text}
//	        │
//	        ▼ enrichWords
//	        │  hit in formResolutions  → real lemma+pos from dict
//	        │  miss in formResolutions → stub lemma+pos from token
//	        │
//	        ▼ aggregate by (lemma, pos)
//	        │  counts, form sets, first-sentence text
//	        │
//	        ▼ gloss lookup from glosses map
//	        │
//	        ▼ []WordEntry sorted by count desc, lemma asc

import (
	"testing"

	"finnestdb/internal/store"
)

// ─── enrichWords tests ────────────────────────────────────────────────────────

// TestEnrichWords_DictHit verifies that when a form appears in formResolutions,
// the WordEntry uses the dictionary lemma+pos (not the stub).
func TestEnrichWords_DictHit(t *testing.T) {
	sentences := []ParsedSentence{
		{
			Text: "Menin pankkiin.",
			Tokens: []ParsedToken{
				{Form: "Menin", StubLemma: "menin", StubPOS: "VERB"},
				{Form: "pankkiin", StubLemma: "pankkiin", StubPOS: "NOUN"},
			},
		},
	}
	formResolutions := map[string][2]string{
		"pankkiin": {"pankki", "NOUN"},
		"Menin":    {"mennä", "VERB"},
	}
	glosses := map[store.LemmaKey]string{
		{Lemma: "pankki", POS: "NOUN"}: "bank (financial institution)",
		{Lemma: "mennä", POS: "VERB"}:  "to go",
	}

	words := enrichWords(sentences, formResolutions, glosses)

	if len(words) != 2 {
		t.Fatalf("expected 2 words, got %d", len(words))
	}
	// Words are sorted by count (both 1), then lemma: mennä < pankki.
	if words[0].Lemma != "mennä" || words[0].POS != "VERB" || words[0].Gloss != "to go" {
		t.Errorf("words[0]: got {%q %q %q}, want {mennä VERB to go}",
			words[0].Lemma, words[0].POS, words[0].Gloss)
	}
	if words[1].Lemma != "pankki" || words[1].POS != "NOUN" || words[1].Gloss != "bank (financial institution)" {
		t.Errorf("words[1]: got {%q %q %q}, want {pankki NOUN bank (financial institution)}",
			words[1].Lemma, words[1].POS, words[1].Gloss)
	}
}

// TestEnrichWords_DictMissUsesStub verifies fallback to stub lemma+pos when a
// form is not in formResolutions (e.g. an out-of-vocabulary word like a proper name).
func TestEnrichWords_DictMissUsesStub(t *testing.T) {
	sentences := []ParsedSentence{
		{
			Text: "Viisutubettaja lauloi.",
			Tokens: []ParsedToken{
				// "Viisutubettaja" is not in any dictionary.
				{Form: "Viisutubettaja", StubLemma: "viisutubettaja", StubPOS: "NOUN"},
			},
		},
	}
	formResolutions := map[string][2]string{} // empty — no dict hits
	glosses := map[store.LemmaKey]string{}    // no glosses

	words := enrichWords(sentences, formResolutions, glosses)

	if len(words) != 1 {
		t.Fatalf("expected 1 word, got %d", len(words))
	}
	got := words[0]
	if got.Lemma != "viisutubettaja" {
		t.Errorf("lemma: got %q, want viisutubettaja", got.Lemma)
	}
	if got.Gloss != "" {
		t.Errorf("gloss: got %q, want empty (no dict entry)", got.Gloss)
	}
	if got.ExampleSentence != "Viisutubettaja lauloi." {
		t.Errorf("example: got %q, want %q", got.ExampleSentence, "Viisutubettaja lauloi.")
	}
}

// TestEnrichWords_CountSortingAndFormAggregation verifies that:
//   - multiple occurrences of the same lemma are counted correctly
//   - different surface forms of the same lemma are collected into Forms
//   - output is sorted by count descending, then lemma ascending
func TestEnrichWords_CountSortingAndFormAggregation(t *testing.T) {
	sentences := []ParsedSentence{
		{
			Text: "Kirja on hyvä.",
			Tokens: []ParsedToken{
				{Form: "Kirja", StubLemma: "kirja", StubPOS: "NOUN"},
				{Form: "on", StubLemma: "olla", StubPOS: "VERB"},
				{Form: "hyvä", StubLemma: "hyvä", StubPOS: "ADJ"},
			},
		},
		{
			Text: "Kirjassa on viisaus.",
			Tokens: []ParsedToken{
				{Form: "Kirjassa", StubLemma: "kirja", StubPOS: "NOUN"}, // second occurrence of kirja
				{Form: "on", StubLemma: "olla", StubPOS: "VERB"},        // second occurrence of olla
				{Form: "viisaus", StubLemma: "viisaus", StubPOS: "NOUN"},
			},
		},
	}
	formResolutions := map[string][2]string{
		"Kirja":    {"kirja", "NOUN"},
		"Kirjassa": {"kirja", "NOUN"}, // both map to same lemma
		"on":       {"olla", "VERB"},
		"hyvä":     {"hyvä", "ADJ"},
		"viisaus":  {"viisaus", "NOUN"},
	}
	glosses := map[store.LemmaKey]string{
		{Lemma: "kirja", POS: "NOUN"}: "book",
		{Lemma: "olla", POS: "VERB"}:  "to be",
	}

	words := enrichWords(sentences, formResolutions, glosses)

	// kirja: count 2, olla: count 2, hyvä: count 1, viisaus: count 1
	// Sorted: count-2 first (kirja < olla), then count-1 (hyvä < viisaus)
	if len(words) != 4 {
		t.Fatalf("expected 4 words, got %d: %v", len(words), words)
	}
	if words[0].Lemma != "kirja" || words[0].Count != 2 {
		t.Errorf("words[0]: got {%q count=%d}, want {kirja count=2}", words[0].Lemma, words[0].Count)
	}
	if words[1].Lemma != "olla" || words[1].Count != 2 {
		t.Errorf("words[1]: got {%q count=%d}, want {olla count=2}", words[1].Lemma, words[1].Count)
	}
	// kirja should have both surface forms collected.
	kirjaForms := words[0].Forms
	if len(kirjaForms) != 2 {
		t.Errorf("kirja forms: got %v, want [Kirja Kirjassa]", kirjaForms)
	}
	// ExampleSentence should be the FIRST sentence the lemma appeared in.
	if words[0].ExampleSentence != "Kirja on hyvä." {
		t.Errorf("kirja example: got %q, want %q", words[0].ExampleSentence, "Kirja on hyvä.")
	}
}

// TestEnrichWords_EmptyInput verifies that an empty sentences slice returns
// an empty (not nil) word list.
func TestEnrichWords_EmptyInput(t *testing.T) {
	words := enrichWords(nil, nil, nil)
	if words == nil {
		t.Error("expected non-nil slice for nil input, got nil")
	}
	if len(words) != 0 {
		t.Errorf("expected 0 words for nil input, got %d", len(words))
	}
}

// ─── collectUniqueForms tests ─────────────────────────────────────────────────

func TestCollectUniqueForms_Deduplication(t *testing.T) {
	sentences := []ParsedSentence{
		{Tokens: []ParsedToken{{Form: "kirja"}, {Form: "on"}}},
		{Tokens: []ParsedToken{{Form: "kirja"}, {Form: "hyvä"}}}, // "kirja" repeated
	}
	forms := collectUniqueForms(sentences)
	// Should return exactly 3 unique forms.
	seen := make(map[string]struct{})
	for _, f := range forms {
		seen[f] = struct{}{}
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 unique forms, got %d: %v", len(seen), forms)
	}
}
