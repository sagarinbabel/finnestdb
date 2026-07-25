package main

import (
	"testing"
)

func TestIsStructuralGloss(t *testing.T) {
	structural := []string{
		"inflection of vuosi",
		"Inflection of vuosi", // capitalised
		"partitive singular of vuosi",
		"Partitive plural of asia",
		"genitive singular of pankki",
		// Umlauted / non-ASCII headwords: Go's regexp \w is ASCII-only,
		// so the case-of-X pattern uses \pL instead. These cases would
		// silently miss with a naive \w port from the Python pattern.
		"partitive singular of ääni",
		"genitive singular of õun",
		"essive singular of pää",
		"illative singular of käsi",
		"adessive plural of käet",
		"elative singular of väri",
		"first-person singular indicative of olla",
		"second person plural indicative of mennä",
		"third-person singular present indicative of tehdä",
		"past active participle of olla",
		"present passive participle of antaa",
		"alternative form of väri",
		"obsolete spelling of värilliset",
		"colloquial form of minä",
		"informal form of sinä",
		"plural of lapsi",
		"singular of lapset",
		"synonym of hyvä",
		"form of antaa",
		"conjugation of mennä",
		"declension of talo",
	}
	for _, g := range structural {
		if !isStructuralGloss(g) {
			t.Errorf("isStructuralGloss(%q) = false; want true", g)
		}
	}

	meaningful := []string{
		"year",
		"bank (financial institution)",
		"a regional inflection of the verb to be", // contains 'inflection of' but not anchored
		"the matter at hand; issue",
		"good",
		"to go",
		"barely, hardly, scarcely",
		"on top of",
		"",   // empty must not be treated as structural - separate skip path
		"  ", // whitespace-only - same
	}
	for _, g := range meaningful {
		if isStructuralGloss(g) {
			t.Errorf("isStructuralGloss(%q) = true; want false", g)
		}
	}
}

// TestImportJSONL_StructuralFilterEndToEnd asserts the filter is wired
// through the full importJSONL path: a kaikki entry whose first sense
// is a "form-of" restatement must land with the meaning gloss in
// lemmas.gloss, and the structural row must never appear in the
// translations table (where it would otherwise win the
// ORDER BY sense_idx ASC LIMIT 1 lookup in BatchLookupGlosses).
func TestImportJSONL_StructuralFilterEndToEnd(t *testing.T) {
	db := newTestDB(t)

	// `vuotta` is a Wiktionary "form-of" entry whose first sense is
	// the metadata restatement; the entry may carry a real definition
	// in a later sense or via the headword `vuosi`. We assert that
	// the meaning gloss wins lemmas.gloss and that the structural row
	// is dropped from translations.
	jsonl := `{"word":"vuotta","pos":"noun","lang_code":"fi","senses":[{"glosses":["partitive singular of vuosi"]},{"glosses":["year (duration; partitive context)"]}],"forms":[]}`

	if _, _, err := importJSONLRaw(db, jsonl, "FI"); err != nil {
		t.Fatalf("importJSONL: %v", err)
	}

	var gloss string
	if err := db.QueryRow(
		`SELECT gloss FROM lemmas WHERE lemma = 'vuotta' AND lang = 'FI'`,
	).Scan(&gloss); err != nil {
		t.Fatalf("lemma query: %v", err)
	}
	if gloss != "year (duration; partitive context)" {
		t.Errorf("lemmas.gloss = %q, want the meaning gloss; structural row should not win", gloss)
	}

	// Confirm the structural row was dropped from translations.
	var structuralCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM translations WHERE lemma = 'vuotta' AND lang = 'FI' AND text LIKE 'partitive singular of%'`,
	).Scan(&structuralCount); err != nil {
		t.Fatalf("translations count: %v", err)
	}
	if structuralCount != 0 {
		t.Errorf("translations: %d structural rows; want 0", structuralCount)
	}

	// Confirm the meaning row IS present and at sense_idx 0 (the
	// promoted slot, where BatchLookupGlosses will read it).
	var firstText string
	var firstIdx int
	if err := db.QueryRow(
		`SELECT text, sense_idx FROM translations WHERE lemma = 'vuotta' AND lang = 'FI' ORDER BY sense_idx ASC LIMIT 1`,
	).Scan(&firstText, &firstIdx); err != nil {
		t.Fatalf("first translation query: %v", err)
	}
	if firstText != "year (duration; partitive context)" || firstIdx != 0 {
		t.Errorf("first translation = (%q, %d); want (meaning gloss, 0)", firstText, firstIdx)
	}
}

func TestPickPrimaryGloss(t *testing.T) {
	cases := []struct {
		name   string
		senses [][]string
		want   string
	}{
		{"empty", nil, ""},
		{"empty senses", [][]string{{}, {}}, ""},
		{"single meaning", [][]string{{"year"}}, "year"},
		{
			"structural first, meaning second",
			[][]string{{"partitive singular of vuosi"}, {"year (as a duration)"}},
			"year (as a duration)",
		},
		{
			"structural first, meaning in same sense",
			[][]string{{"inflection of vuosi", "year"}},
			"year",
		},
		{
			"all structural: fall back to first",
			[][]string{{"partitive singular of vuosi"}, {"plural of vuosi"}},
			"partitive singular of vuosi",
		},
		{
			"meaning interleaved with empty",
			[][]string{{""}, {"  "}, {"the matter"}},
			"the matter",
		},
		{
			"first sense's second gloss is meaning",
			[][]string{{"form of pankki", "bank"}, {"piggy bank"}},
			"bank",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pickPrimaryGloss(tc.senses)
			if got != tc.want {
				t.Errorf("pickPrimaryGloss = %q, want %q", got, tc.want)
			}
		})
	}
}
