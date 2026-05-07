package udfeats

import "testing"

func TestCompose(t *testing.T) {
	cases := []struct {
		name                                                      string
		label, number, tense, mood, person, voice, verbForm, want string
	}{
		{"all empty", "", "", "", "", "", "", "", ""},
		{"case only", "inessive", "", "", "", "", "", "", "Case=Ine"},
		{"nom dropped", "nominative", "", "", "", "", "", "", ""},
		{"nom + number kept on Number", "nominative", "Sing", "", "", "", "", "", "Number=Sing"},
		{"verb finite", "", "Sing", "Pres", "Ind", "1", "", "", "Mood=Ind|Number=Sing|Person=1|Tense=Pres"},
		{"unknown label dropped", "frabjous", "Sing", "", "", "", "", "", "Number=Sing"},
		{"alphabetical order", "elative", "Plur", "Past", "Cnd", "3", "", "", "Case=Ela|Mood=Cnd|Number=Plur|Person=3|Tense=Past"},
		{"voice only", "", "", "", "", "", "Act", "", "Voice=Act"},
		{"verbform only", "", "", "", "", "", "", "Inf", "VerbForm=Inf"},
		{"full verb", "", "Sing", "Pres", "Ind", "1", "Act", "Fin", "Mood=Ind|Number=Sing|Person=1|Tense=Pres|VerbForm=Fin|Voice=Act"},
		{"passive participle", "", "Sing", "", "", "", "Pass", "Part", "Number=Sing|VerbForm=Part|Voice=Pass"},
	}
	for _, tc := range cases {
		got := Compose(tc.label, tc.number, tc.tense, tc.mood, tc.person, tc.voice, tc.verbForm)
		if got != tc.want {
			t.Errorf("%s: Compose(%q,%q,%q,%q,%q,%q,%q) = %q, want %q", tc.name,
				tc.label, tc.number, tc.tense, tc.mood, tc.person, tc.voice, tc.verbForm, got, tc.want)
		}
	}
}

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

func TestRoundTrip(t *testing.T) {
	// Compose then CaseFromFeats should round-trip the case label.
	for label := range LegacyLabelToUDCase {
		feats := Compose(label, "", "", "", "", "", "")
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
