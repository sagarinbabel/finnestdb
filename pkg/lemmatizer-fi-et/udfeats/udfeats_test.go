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
