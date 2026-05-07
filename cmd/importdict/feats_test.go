package main

import (
	"strings"
	"testing"
)

func TestKaikkiTagsToFeats_NominalCases(t *testing.T) {
	cases := []struct {
		tags []string
		upos string
		want string
	}{
		{[]string{"illative", "singular"}, "NOUN", "Case=Ill|Number=Sing"},
		{[]string{"singular", "illative"}, "NOUN", "Case=Ill|Number=Sing"}, // order-independent
		{[]string{"genitive", "plural"}, "NOUN", "Case=Gen|Number=Plur"},
		{[]string{"partitive"}, "NOUN", "Case=Par"},
		{[]string{"inessive", "singular"}, "NOUN", "Case=Ine|Number=Sing"},
		{[]string{"elative", "plural"}, "NOUN", "Case=Ela|Number=Plur"},
		{[]string{"adessive", "singular"}, "NOUN", "Case=Ade|Number=Sing"},
		{[]string{"ablative", "singular"}, "NOUN", "Case=Abl|Number=Sing"},
		{[]string{"allative", "plural"}, "NOUN", "Case=All|Number=Plur"},
		{[]string{"essive", "singular"}, "NOUN", "Case=Ess|Number=Sing"},
		{[]string{"translative", "plural"}, "NOUN", "Case=Tra|Number=Plur"},
		{[]string{"abessive", "singular"}, "NOUN", "Case=Abe|Number=Sing"},
		{[]string{"comitative", "plural"}, "NOUN", "Case=Com|Number=Plur"},
		{[]string{"instructive", "plural"}, "NOUN", "Case=Ins|Number=Plur"},
		{[]string{"terminative", "singular"}, "NOUN", "Case=Ter|Number=Sing"}, // ET-specific
		{[]string{"aditive", "singular"}, "NOUN", "Case=Ill|Number=Sing"},     // folds to Ill
	}
	for _, tc := range cases {
		got := kaikkiTagsToFeats(tc.tags, tc.upos)
		if got != tc.want {
			t.Errorf("kaikkiTagsToFeats(%v, %q) = %q, want %q", tc.tags, tc.upos, got, tc.want)
		}
	}
}

func TestKaikkiTagsToFeats_VerbForms(t *testing.T) {
	cases := []struct {
		tags []string
		upos string
		want string
	}{
		{
			[]string{"first-person", "present", "indicative", "singular", "active"},
			"VERB",
			"Mood=Ind|Number=Sing|Person=1|Tense=Pres|Voice=Act",
		},
		{
			[]string{"third-person", "past", "indicative", "plural", "active"},
			"VERB",
			"Mood=Ind|Number=Plur|Person=3|Tense=Past|Voice=Act",
		},
		{
			[]string{"present", "passive", "indicative"},
			"VERB",
			"Mood=Ind|Tense=Pres|Voice=Pass",
		},
		{
			[]string{"infinitive"},
			"VERB",
			"VerbForm=Inf",
		},
		{
			[]string{"present-participle", "active"},
			"VERB",
			"VerbForm=Part|Voice=Act",
		},
		{
			[]string{"past-participle", "passive"},
			"VERB",
			"VerbForm=Part|Voice=Pass",
		},
		{
			[]string{"second-person", "imperative", "singular", "active"},
			"VERB",
			"Mood=Imp|Number=Sing|Person=2|Voice=Act",
		},
		{
			[]string{"third-person", "conditional", "singular", "active"},
			"VERB",
			"Mood=Cnd|Number=Sing|Person=3|Voice=Act",
		},
		{
			[]string{"third-person", "potential", "singular", "active"},
			"VERB",
			"Mood=Pot|Number=Sing|Person=3|Voice=Act",
		},
	}
	for _, tc := range cases {
		got := kaikkiTagsToFeats(tc.tags, tc.upos)
		if got != tc.want {
			t.Errorf("kaikkiTagsToFeats(%v, %q) = %q, want %q", tc.tags, tc.upos, got, tc.want)
		}
	}
}

func TestKaikkiTagsToFeats_AdjectiveDegree(t *testing.T) {
	cases := []struct {
		tags []string
		upos string
		want string
	}{
		{[]string{"comparative"}, "ADJ", "Degree=Cmp"},
		{[]string{"superlative"}, "ADJ", "Degree=Sup"},
		{[]string{"positive"}, "ADJ", "Degree=Pos"},
		// Degree gated to ADJ/ADV — same tag on a verb does nothing.
		{[]string{"comparative"}, "VERB", ""},
		// Degree composes with case on inflected adjectives.
		{
			[]string{"comparative", "inessive", "singular"},
			"ADJ",
			"Case=Ine|Degree=Cmp|Number=Sing",
		},
	}
	for _, tc := range cases {
		got := kaikkiTagsToFeats(tc.tags, tc.upos)
		if got != tc.want {
			t.Errorf("kaikkiTagsToFeats(%v, %q) = %q, want %q", tc.tags, tc.upos, got, tc.want)
		}
	}
}

func TestKaikkiTagsToFeats_PronType(t *testing.T) {
	cases := []struct {
		tags []string
		upos string
		want string
	}{
		{[]string{"demonstrative"}, "PRON", "PronType=Dem"},
		{[]string{"interrogative"}, "PRON", "PronType=Int"},
		{[]string{"relative"}, "PRON", "PronType=Rel"},
		{[]string{"personal"}, "PRON", "PronType=Prs"},
		{[]string{"personal-pronoun"}, "PRON", "PronType=Prs"},
		// PronType is gated to PRON — DETs and others stay clean.
		{[]string{"demonstrative"}, "DET", ""},
	}
	for _, tc := range cases {
		got := kaikkiTagsToFeats(tc.tags, tc.upos)
		if got != tc.want {
			t.Errorf("kaikkiTagsToFeats(%v, %q) = %q, want %q", tc.tags, tc.upos, got, tc.want)
		}
	}
}

func TestKaikkiTagsToFeats_EmptyAndUnknown(t *testing.T) {
	if got := kaikkiTagsToFeats(nil, "NOUN"); got != "" {
		t.Errorf("nil tags: got %q, want empty", got)
	}
	if got := kaikkiTagsToFeats([]string{}, "NOUN"); got != "" {
		t.Errorf("empty tags: got %q, want empty", got)
	}
	if got := kaikkiTagsToFeats([]string{"unknown-tag", "another-mystery"}, "NOUN"); got != "" {
		t.Errorf("unknown tags: got %q, want empty", got)
	}
	// Possessive marker is NOT translated to FEATS — caller filters it
	// at hasPossessiveTag time before reaching the mapper.
	if got := kaikkiTagsToFeats([]string{"possessive", "singular", "first-person"}, "NOUN"); got != "Number=Sing|Person=1" {
		t.Errorf("possessive: got %q, want %q", got, "Number=Sing|Person=1")
	}
}

func TestKaikkiTagsToFeats_TagsAreAlphabeticalInOutput(t *testing.T) {
	// Whatever the input tag order, the output must be alphabetical so the
	// FEATS column is canonical and stable across re-imports.
	feats := kaikkiTagsToFeats([]string{"plural", "elative", "third-person"}, "NOUN")
	parts := strings.Split(feats, "|")
	for i := 1; i < len(parts); i++ {
		if parts[i-1] >= parts[i] {
			t.Errorf("FEATS not alphabetical: %q", feats)
		}
	}
}

func TestWithReflex(t *testing.T) {
	cases := []struct {
		feats string
		lemma string
		upos  string
		want  string
	}{
		{"", "itse", "PRON", "Reflex=Yes"},
		{"Case=Ade|Number=Sing", "itse", "PRON", "Case=Ade|Number=Sing|Reflex=Yes"},
		{"", "Itse", "PRON", "Reflex=Yes"}, // case-insensitive lemma match
		{"", "enese", "PRON", "Reflex=Yes"},
		{"", "enda", "PRON", "Reflex=Yes"},
		// Non-PRON: same lemma string is left alone — Reflex= is per-pronoun.
		{"Case=Ine", "itse", "ADV", "Case=Ine"},
		// Unknown lemma: no change.
		{"Case=Ine", "talo", "PRON", "Case=Ine"},
	}
	for _, tc := range cases {
		got := withReflex(tc.feats, tc.lemma, tc.upos)
		if got != tc.want {
			t.Errorf("withReflex(%q, %q, %q) = %q, want %q",
				tc.feats, tc.lemma, tc.upos, got, tc.want)
		}
	}
}

// --- end-to-end: importJSONL writes feats column ---

func TestImportJSONL_WritesFeats(t *testing.T) {
	db := newTestDB(t)

	jsonl := `{"word":"pankki","pos":"noun","lang_code":"fi","senses":[{"glosses":["bank"]}],"forms":[` +
		`{"form":"pankin","tags":["genitive","singular"]},` +
		`{"form":"pankissa","tags":["inessive","singular"]},` +
		`{"form":"pankkeja","tags":["partitive","plural"]}` +
		`]}`

	if _, _, err := importJSONLRaw(db, jsonl, "FI"); err != nil {
		t.Fatalf("importJSONL: %v", err)
	}

	cases := []struct {
		form string
		want string
	}{
		{"pankki", ""}, // canonical headword: no feats
		{"pankin", "Case=Gen|Number=Sing"},
		{"pankissa", "Case=Ine|Number=Sing"},
		{"pankkeja", "Case=Par|Number=Plur"},
	}
	for _, tc := range cases {
		var feats string
		err := db.QueryRow(
			`SELECT COALESCE(feats, '') FROM forms WHERE form = ? AND lang = 'FI'`, tc.form,
		).Scan(&feats)
		if err != nil {
			t.Fatalf("query %q: %v", tc.form, err)
		}
		if feats != tc.want {
			t.Errorf("form %q: feats = %q, want %q", tc.form, feats, tc.want)
		}
	}
}

func TestImportJSONL_WritesFeats_VerbForms(t *testing.T) {
	db := newTestDB(t)

	jsonl := `{"word":"mennä","pos":"verb","lang_code":"fi","senses":[{"glosses":["to go"]}],"forms":[` +
		`{"form":"menen","tags":["first-person","present","indicative","singular","active"]},` +
		`{"form":"menivät","tags":["third-person","past","indicative","plural","active"]}` +
		`]}`

	if _, _, err := importJSONLRaw(db, jsonl, "FI"); err != nil {
		t.Fatalf("importJSONL: %v", err)
	}

	var feats string
	if err := db.QueryRow(
		`SELECT COALESCE(feats, '') FROM forms WHERE form = 'menen' AND lang = 'FI'`,
	).Scan(&feats); err != nil {
		t.Fatalf("menen query: %v", err)
	}
	if feats != "Mood=Ind|Number=Sing|Person=1|Tense=Pres|Voice=Act" {
		t.Errorf("menen feats = %q", feats)
	}

	if err := db.QueryRow(
		`SELECT COALESCE(feats, '') FROM forms WHERE form = 'menivät' AND lang = 'FI'`,
	).Scan(&feats); err != nil {
		t.Fatalf("menivät query: %v", err)
	}
	if feats != "Mood=Ind|Number=Plur|Person=3|Tense=Past|Voice=Act" {
		t.Errorf("menivät feats = %q", feats)
	}
}

func TestImportJSONL_WritesFeats_AdjectiveDegree(t *testing.T) {
	db := newTestDB(t)

	jsonl := `{"word":"hyvä","pos":"adj","lang_code":"fi","senses":[{"glosses":["good"]}],"forms":[` +
		`{"form":"parempi","tags":["comparative"]},` +
		`{"form":"paras","tags":["superlative"]},` +
		`{"form":"paremmassa","tags":["comparative","inessive","singular"]}` +
		`]}`

	if _, _, err := importJSONLRaw(db, jsonl, "FI"); err != nil {
		t.Fatalf("importJSONL: %v", err)
	}

	cases := map[string]string{
		"parempi":       "Degree=Cmp",
		"paras":         "Degree=Sup",
		"paremmassa":    "Case=Ine|Degree=Cmp|Number=Sing",
	}
	for form, want := range cases {
		var feats string
		if err := db.QueryRow(
			`SELECT COALESCE(feats, '') FROM forms WHERE form = ? AND lang = 'FI'`, form,
		).Scan(&feats); err != nil {
			t.Fatalf("%q query: %v", form, err)
		}
		if feats != want {
			t.Errorf("%q: feats = %q, want %q", form, feats, want)
		}
	}
}

func TestImportJSONL_WritesFeats_ReflexivePronoun(t *testing.T) {
	db := newTestDB(t)

	jsonl := `{"word":"itse","pos":"pron","lang_code":"fi","senses":[{"glosses":["self"]}],"forms":[` +
		`{"form":"itseään","tags":["partitive","singular"]}` +
		`]}`

	if _, _, err := importJSONLRaw(db, jsonl, "FI"); err != nil {
		t.Fatalf("importJSONL: %v", err)
	}

	// Lemma form gets Reflex=Yes even with no other tags.
	var feats string
	if err := db.QueryRow(
		`SELECT COALESCE(feats, '') FROM forms WHERE form = 'itse' AND lang = 'FI'`,
	).Scan(&feats); err != nil {
		t.Fatalf("itse query: %v", err)
	}
	if feats != "Reflex=Yes" {
		t.Errorf("itse feats = %q", feats)
	}

	// Inflected form composes Reflex=Yes with case+number.
	if err := db.QueryRow(
		`SELECT COALESCE(feats, '') FROM forms WHERE form = 'itseään' AND lang = 'FI'`,
	).Scan(&feats); err != nil {
		t.Fatalf("itseään query: %v", err)
	}
	if feats != "Case=Par|Number=Sing|Reflex=Yes" {
		t.Errorf("itseään feats = %q", feats)
	}
}

func TestImportJSONL_PossessivesStillSkipped(t *testing.T) {
	// Regression: the FEATS pipeline must not weaken the possessive-skip
	// behavior. Possessive forms still don't land in `forms` at all.
	db := newTestDB(t)

	jsonl := `{"word":"pankki","pos":"noun","lang_code":"fi","senses":[{"glosses":["bank"]}],"forms":[` +
		`{"form":"pankkini","tags":["singular","possessive","first-person"]}` +
		`]}`

	_, skipped, err := importJSONLRaw(db, jsonl, "FI")
	if err != nil {
		t.Fatalf("importJSONL: %v", err)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	var dummy string
	if err := db.QueryRow(
		`SELECT form FROM forms WHERE form = 'pankkini' AND lang = 'FI'`,
	).Scan(&dummy); err == nil {
		t.Error("possessive form 'pankkini' should not be in forms table")
	}
}

func TestBackfillFeatsJSONL(t *testing.T) {
	// Pre-seed forms with NULL feats to simulate an existing DB imported
	// before the FEATS mapper landed. Backfill should populate them.
	db := newTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
		 VALUES ('pankissa', 'pankki', 'NOUN', 'FI', 'kaikki', 10, NULL),
		        ('pankkeja', 'pankki', 'NOUN', 'FI', 'kaikki', 10, NULL),
		        ('hardcoded', 'foo',     'NOUN', 'FI', 'manual', 50, 'Case=Nom|Number=Sing')`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	jsonl := `{"word":"pankki","pos":"noun","lang_code":"fi","senses":[{"glosses":["bank"]}],"forms":[` +
		`{"form":"pankissa","tags":["inessive","singular"]},` +
		`{"form":"pankkeja","tags":["partitive","plural"]}` +
		`]}`

	updated, scanned, err := backfillFeatsJSONL(db, strings.NewReader(jsonl), "FI")
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if scanned < 2 {
		t.Errorf("scanned = %d, want >= 2", scanned)
	}
	if updated != 2 {
		t.Errorf("updated = %d, want 2", updated)
	}

	cases := map[string]string{
		"pankissa":  "Case=Ine|Number=Sing",
		"pankkeja":  "Case=Par|Number=Plur",
		"hardcoded": "Case=Nom|Number=Sing", // pre-existing feats untouched
	}
	for form, want := range cases {
		var feats string
		if err := db.QueryRow(
			`SELECT COALESCE(feats, '') FROM forms WHERE form = ? AND lang = 'FI'`, form,
		).Scan(&feats); err != nil {
			t.Fatalf("%q query: %v", form, err)
		}
		if feats != want {
			t.Errorf("%q: feats = %q, want %q", form, feats, want)
		}
	}
}
