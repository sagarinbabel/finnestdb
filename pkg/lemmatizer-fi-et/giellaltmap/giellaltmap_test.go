package giellaltmap

import "testing"

func TestParse_BasicNoun(t *testing.T) {
	a := Parse("talo+N+Sg+Nom")
	if a.Lemma != "talo" || a.UPOS != "NOUN" || a.GrammarLabel != "nominative" || a.Number != "Sing" {
		t.Errorf("got %+v", a)
	}
}

func TestParse_NounIne(t *testing.T) {
	a := Parse("talo+N+Sg+Ine")
	if a.GrammarLabel != "inessive" {
		t.Errorf("grammar=%q want inessive", a.GrammarLabel)
	}
}

func TestParse_Verb1Sg(t *testing.T) {
	a := Parse("olla+V+Act+Ind+Prs+Sg1")
	if a.Lemma != "olla" || a.UPOS != "VERB" || a.Tense != "Pres" || a.Mood != "Ind" ||
		a.Number != "Sing" || a.Person != "1" {
		t.Errorf("got %+v", a)
	}
	if a.Voice != "Act" {
		t.Errorf("voice=%q want Act", a.Voice)
	}
	if a.VerbForm != "Fin" {
		t.Errorf("verbform=%q want Fin", a.VerbForm)
	}
}

func TestParse_VerbPast2Sg(t *testing.T) {
	a := Parse("kysyä+V+Act+Ind+Prt+Sg2")
	if a.Tense != "Past" || a.Person != "2" {
		t.Errorf("got %+v", a)
	}
	if a.Voice != "Act" {
		t.Errorf("voice=%q want Act", a.Voice)
	}
}

func TestParse_VerbPassive(t *testing.T) {
	a := Parse("sanoa+V+Pass+Ind+Prs")
	if a.Voice != "Pass" {
		t.Errorf("voice=%q want Pass", a.Voice)
	}
	if a.Mood != "Ind" || a.Tense != "Pres" {
		t.Errorf("mood=%q tense=%q want Ind/Pres", a.Mood, a.Tense)
	}
}

func TestParse_Infinitive(t *testing.T) {
	a := Parse("olla+V+Act+Inf")
	if a.VerbForm != "Inf" {
		t.Errorf("verbform=%q want Inf", a.VerbForm)
	}
	if a.Voice != "Act" {
		t.Errorf("voice=%q want Act", a.Voice)
	}
	if a.Mood != "" {
		t.Errorf("mood=%q want empty (infinitives have no mood)", a.Mood)
	}
}

func TestParse_Participle(t *testing.T) {
	a := Parse("sanoa+V+Act+PrfPrc+Sg+Nom")
	if a.VerbForm != "Part" {
		t.Errorf("verbform=%q want Part", a.VerbForm)
	}
	if a.Voice != "Act" {
		t.Errorf("voice=%q want Act", a.Voice)
	}
}

func TestParse_Compound(t *testing.T) {
	// Two-segment compound: lemma should be concatenated; case from last segment.
	a := Parse("pankki+N+Cmp/SgNom+Cmp#automaatti+N+Sg+Ela")
	if a.Lemma != "pankkiautomaatti" {
		t.Errorf("lemma=%q want pankkiautomaatti", a.Lemma)
	}
	if a.UPOS != "NOUN" || a.GrammarLabel != "elative" {
		t.Errorf("UPOS=%q grammar=%q want NOUN/elative", a.UPOS, a.GrammarLabel)
	}
	if !a.IsCompound {
		t.Error("IsCompound should be true")
	}
}

func TestParse_Adjective(t *testing.T) {
	a := Parse("hyvä+A+Sg+Par")
	if a.UPOS != "ADJ" || a.GrammarLabel != "partitive" {
		t.Errorf("got %+v", a)
	}
}

func TestParse_Empty(t *testing.T) {
	if a := Parse(""); a.Lemma != "" {
		t.Errorf("empty input should give zero analysis, got %+v", a)
	}
}
