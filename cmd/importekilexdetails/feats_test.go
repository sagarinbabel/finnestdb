package main

import "testing"

func TestEkilexMorphToFeats_Nominal(t *testing.T) {
	cases := map[string]string{
		"SgN":   "Case=Nom|Number=Sing",
		"SgG":   "Case=Gen|Number=Sing",
		"SgP":   "Case=Par|Number=Sing",
		"SgIn":  "Case=Ine|Number=Sing",
		"SgIll": "Case=Ill|Number=Sing",
		"SgAdt": "Case=Ill|Number=Sing", // aditive folds to illative
		"PlKom": "Case=Com|Number=Plur",
		"PlAd":  "Case=Ade|Number=Plur",
		"PlAll": "Case=All|Number=Plur",
	}
	for code, want := range cases {
		got := ekilexMorphToFeats(code)
		if got != want {
			t.Errorf("%s: got %q, want %q", code, got, want)
		}
	}
}

func TestEkilexMorphToFeats_Verbal(t *testing.T) {
	cases := map[string]string{
		"IndPrSg1":  "Mood=Ind|Number=Sing|Person=1|Tense=Pres|VerbForm=Fin|Voice=Act",
		"IndPrSg3":  "Mood=Ind|Number=Sing|Person=3|Tense=Pres|VerbForm=Fin|Voice=Act",
		"IndIpfPl3": "Mood=Ind|Number=Plur|Person=3|Tense=Past|VerbForm=Fin|Voice=Act",
		"KndPrSg1":  "Mood=Cnd|Number=Sing|Person=1|Tense=Pres|VerbForm=Fin|Voice=Act",
		"ImpPrSg2":  "Mood=Imp|Number=Sing|Person=2|Tense=Pres|VerbForm=Fin|Voice=Act",
		"Inf":       "VerbForm=Inf",
		"Sup":       "Case=Ill|VerbForm=Sup",
		"Ger":       "VerbForm=Conv",
		"SupIps":    "Case=Ill|VerbForm=Sup|Voice=Pass",
		"PtsPtIps":  "Tense=Past|VerbForm=Part|Voice=Pass",
	}
	for code, want := range cases {
		got := ekilexMorphToFeats(code)
		if got != want {
			t.Errorf("%s: got %q, want %q", code, got, want)
		}
	}
}

// TestEkilexMorphToFeats_Negative verifies polarity-suffix variants get
// Polarity=Neg appended in alphabetical order.
func TestEkilexMorphToFeats_Negative(t *testing.T) {
	cases := map[string]string{
		"IndPrPsN":    "Mood=Ind|Polarity=Neg|Tense=Pres|VerbForm=Fin|Voice=Act",
		"IndPrIpsN":   "Mood=Ind|Polarity=Neg|Tense=Pres|VerbForm=Fin|Voice=Pass",
		"IndIpfPsN":   "Mood=Ind|Polarity=Neg|Tense=Past|VerbForm=Fin|Voice=Act",
		"PtsPtIpsNeg": "Polarity=Neg|Tense=Past|VerbForm=Part|Voice=Pass",
		"PtsPtPsNeg":  "Polarity=Neg|Tense=Past|VerbForm=Part|Voice=Act",
		"InfN":        "Polarity=Neg|VerbForm=Inf",
	}
	for code, want := range cases {
		got := ekilexMorphToFeats(code)
		if got != want {
			t.Errorf("%s: got %q, want %q", code, got, want)
		}
	}
}

// TestEkilexMorphToFeats_PositiveUnderscore: trailing _ on Ind*Pr*Ps_
// is a positive-form marker; the bare base maps to the same FEATS.
func TestEkilexMorphToFeats_PositiveUnderscore(t *testing.T) {
	cases := map[string]string{
		"IndPrPs_":  "Mood=Ind|Tense=Pres|VerbForm=Fin|Voice=Act",
		"IndPrIps_": "Mood=Ind|Tense=Pres|VerbForm=Fin|Voice=Pass",
	}
	for code, want := range cases {
		got := ekilexMorphToFeats(code)
		if got != want {
			t.Errorf("%s: got %q, want %q", code, got, want)
		}
	}
}

func TestEkilexMorphToFeats_Empty(t *testing.T) {
	cases := []string{"", "ID", "??", "Bogus"}
	for _, code := range cases {
		if got := ekilexMorphToFeats(code); got != "" {
			t.Errorf("%s: expected empty, got %q", code, got)
		}
	}
}

func TestAppendFeat_AlphabeticalOrder(t *testing.T) {
	got := appendFeat("Mood=Ind|Tense=Pres|Voice=Act", "Polarity=Neg")
	want := "Mood=Ind|Polarity=Neg|Tense=Pres|Voice=Act"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAppendFeat_FromEmpty(t *testing.T) {
	got := appendFeat("", "Case=Nom")
	if got != "Case=Nom" {
		t.Errorf("got %q, want Case=Nom", got)
	}
}
