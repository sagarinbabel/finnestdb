package udfeats

import "testing"

func TestCaseFromFeats(t *testing.T) {
	cases := []struct {
		feats, want string
	}{
		{"", ""},
		{"Case=Ine|Number=Sing", "inessive"},
		{"Number=Sing|Case=Par", "partitive"},
		{"Case=Nom|Number=Sing", ""}, // Nom implicit
		{"Number=Sing", ""},
		{"Case=Frabjous", ""}, // unknown
	}
	for _, tc := range cases {
		got := CaseFromFeats(tc.feats)
		if got != tc.want {
			t.Errorf("CaseFromFeats(%q) = %q, want %q", tc.feats, got, tc.want)
		}
	}
}

func TestComposeMap(t *testing.T) {
	cases := []struct {
		name  string
		pairs map[string]string
		want  string
	}{
		{"nil map", nil, ""},
		{"empty map", map[string]string{}, ""},
		{"all empty values", map[string]string{"Case": "", "Number": ""}, ""},
		{"single pair", map[string]string{"Degree": "Cmp"}, "Degree=Cmp"},
		{"mixed empty and set", map[string]string{"Degree": "Sup", "Case": "", "Number": "Sing"}, "Degree=Sup|Number=Sing"},
		{"layered features", map[string]string{"Person[psor]": "3", "Number[psor]": "Sing", "Case": "Ine"}, "Case=Ine|Number[psor]=Sing|Person[psor]=3"},
		{"full Finnish verb", map[string]string{
			"Mood": "Ind", "Number": "Sing", "Person": "1",
			"Tense": "Pres", "VerbForm": "Fin", "Voice": "Act",
		}, "Mood=Ind|Number=Sing|Person=1|Tense=Pres|VerbForm=Fin|Voice=Act"},
		{"participle with PartForm", map[string]string{
			"VerbForm": "Part", "PartForm": "Past", "Voice": "Act",
		}, "PartForm=Past|VerbForm=Part|Voice=Act"},
		{"infinitive with InfForm", map[string]string{
			"VerbForm": "Inf", "InfForm": "3", "Voice": "Act", "Case": "Ine",
		}, "Case=Ine|InfForm=3|VerbForm=Inf|Voice=Act"},
		{"pronoun with PronType", map[string]string{
			"PronType": "Dem", "Case": "Ine", "Number": "Sing",
		}, "Case=Ine|Number=Sing|PronType=Dem"},
		{"clitic + connegative", map[string]string{
			"Clitic": "Ko", "Connegative": "Yes", "VerbForm": "Fin",
		}, "Clitic=Ko|Connegative=Yes|VerbForm=Fin"},
	}
	for _, tc := range cases {
		got := ComposeMap(tc.pairs)
		if got != tc.want {
			t.Errorf("%s: ComposeMap = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAppendSortedValue(t *testing.T) {
	cases := []struct {
		name, existing, value, want string
	}{
		{"empty + value", "", "Ko", "Ko"},
		{"single + later", "Han", "Ko", "Han,Ko"},
		{"single + earlier", "Ko", "Han", "Han,Ko"},
		{"two + middle", "Han,Pa", "Ko", "Han,Ko,Pa"},
		{"duplicate", "Han,Ko", "Ko", "Han,Ko"},
		{"three stacked", AppendSortedValue(AppendSortedValue("", "Ko"), "Han"), "S", "Han,Ko,S"},
	}
	for _, tc := range cases {
		got := AppendSortedValue(tc.existing, tc.value)
		if got != tc.want {
			t.Errorf("%s: AppendSortedValue(%q, %q) = %q, want %q", tc.name, tc.existing, tc.value, got, tc.want)
		}
	}
}

func TestNormalizeMaInfinitive(t *testing.T) {
	cases := []struct {
		name, surface, pos, in, want string
	}{
		{
			"illative MA noun-cousin trap (tarjoamaan)",
			"tarjoamaan", "VERB",
			"Case=Ill|Number=Sing|Person=3",
			"Case=Ill|InfForm=Ma|VerbForm=Inf",
		},
		{
			"inessive MA in-progress (juomassa)",
			"juomassa", "VERB",
			"Case=Ine|Number=Sing|Person=3",
			"Case=Ine|InfForm=Ma|VerbForm=Inf",
		},
		{
			"elative MA from-doing (juomasta)",
			"juomasta", "VERB",
			"Case=Ela|Number=Sing|Person=3",
			"Case=Ela|InfForm=Ma|VerbForm=Inf",
		},
		{
			"adessive MA by-doing (tekemällä)",
			"Tekemällä", "VERB",
			"Case=Ade|Number=Sing|Person=3",
			"Case=Ade|InfForm=Ma|VerbForm=Inf",
		},
		{
			"abessive MA without-doing (puhumatta)",
			"puhumatta", "VERB",
			"Case=Abe|Number=Sing|Person=3",
			"Case=Abe|InfForm=Ma|VerbForm=Inf",
		},
		{
			"front-vowel harmony preserved (lähtemään)",
			"lähtemään", "VERB",
			"Case=Ill|Number=Sing|Person=3",
			"Case=Ill|InfForm=Ma|VerbForm=Inf",
		},
		{
			"sentence-initial capital with umlaut (Lähtemään)",
			"Lähtemään", "VERB",
			"Case=Ill|Number=Sing|Person=3",
			"Case=Ill|InfForm=Ma|VerbForm=Inf",
		},
		{
			"preserves Voice and other unrelated features",
			"tarjoamaan", "VERB",
			"Case=Ill|Number=Sing|Person=3|Voice=Act",
			"Case=Ill|InfForm=Ma|VerbForm=Inf|Voice=Act",
		},
		{
			"already correctly marked MA-infinitive: unchanged",
			"tarjoamaan", "VERB",
			"Case=Ill|InfForm=Ma|VerbForm=Inf",
			"Case=Ill|InfForm=Ma|VerbForm=Inf",
		},
		{
			"non-VERB pos: unchanged",
			"tarjoamaan", "NOUN",
			"Case=Ill|Number=Sing",
			"Case=Ill|Number=Sing",
		},
		{
			"verb but surface doesn't end in MA-suffix: unchanged",
			"menin", "VERB",
			"Number=Sing|Person=1|Tense=Past",
			"Number=Sing|Person=1|Tense=Past",
		},
		{
			"verb with case but wrong suffix for case: unchanged",
			"talossa", "VERB",
			"Case=Ine|Number=Sing|Person=3",
			"Case=Ine|Number=Sing|Person=3",
		},
		{
			"empty feats: unchanged",
			"tarjoamaan", "VERB",
			"",
			"",
		},
		{
			"verb without Case feature: unchanged",
			"tarjoamaan", "VERB",
			"Number=Sing|Person=3",
			"Number=Sing|Person=3",
		},
		{
			"VERB illative on Number=Plur surface: still normalises (no Sing to strip)",
			"tarjoamaan", "VERB",
			"Case=Ill|Number=Plur|Person=3",
			"Case=Ill|InfForm=Ma|Number=Plur|VerbForm=Inf",
		},
	}
	for _, tc := range cases {
		got := NormalizeMaInfinitive(tc.surface, tc.pos, tc.in)
		if got != tc.want {
			t.Errorf("%s: NormalizeMaInfinitive(%q, %q, %q) = %q, want %q",
				tc.name, tc.surface, tc.pos, tc.in, got, tc.want)
		}
	}
}

func TestParseFeatsRoundTrip(t *testing.T) {
	// ComposeMap(parseFeats(x)) must be canonical (alphabetised, deduped).
	inputs := []string{
		"Case=Ine|Number=Sing",
		"Number=Sing|Case=Par",
		"Person[psor]=3|Number[psor]=Sing|Case=Ine",
		"VerbForm=Fin|Voice=Act|Tense=Pres|Person=1|Number=Sing|Mood=Ind",
	}
	for _, in := range inputs {
		got := ComposeMap(parseFeats(in))
		// Reparse to ignore key order in the input.
		if !sameFeats(in, got) {
			t.Errorf("ComposeMap(parseFeats(%q)) = %q; not equivalent", in, got)
		}
	}
}

func sameFeats(a, b string) bool {
	pa := parseFeats(a)
	pb := parseFeats(b)
	if len(pa) != len(pb) {
		return false
	}
	for k, v := range pa {
		if pb[k] != v {
			return false
		}
	}
	return true
}

func TestRoundTrip(t *testing.T) {
	// ComposeMap then CaseFromFeats should round-trip the case label.
	for label := range LegacyLabelToUDCase {
		udCase := LegacyLabelToUDCase[label]
		feats := ""
		if udCase != "Nom" {
			feats = ComposeMap(map[string]string{"Case": udCase})
		}
		got := CaseFromFeats(feats)
		want := label
		if label == "nominative" {
			want = "" // implicit
		}
		if got != want {
			t.Errorf("round-trip %q: feats=%q caseFromFeats=%q want %q", label, feats, got, want)
		}
	}
}
