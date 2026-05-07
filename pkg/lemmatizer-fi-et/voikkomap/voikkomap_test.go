package voikkomap

import "testing"

func TestParse_BasicNoun(t *testing.T) {
	a := Parse("[Ln][Ica][Xp]talo[X]talo[Sn][Ny]")
	if a.Lemma != "talo" {
		t.Errorf("lemma=%q want talo", a.Lemma)
	}
	if a.UPOS != "NOUN" {
		t.Errorf("UPOS=%q want NOUN", a.UPOS)
	}
	if a.GrammarLabel != "nominative" {
		t.Errorf("grammar_label=%q want nominative", a.GrammarLabel)
	}
	if a.Number != "Sing" {
		t.Errorf("number=%q want Sing", a.Number)
	}
}

func TestParse_NounWithCase(t *testing.T) {
	a := Parse("[Ln][Ica][Xp]talo[X]talo[Sine][Ny]ssa")
	if a.Lemma != "talo" {
		t.Errorf("lemma=%q want talo", a.Lemma)
	}
	if a.GrammarLabel != "inessive" {
		t.Errorf("grammar_label=%q want inessive", a.GrammarLabel)
	}
}

func TestParse_Compound(t *testing.T) {
	// pankkiautomaatista = pankki+automaatti, case=elative on the last component
	a := Parse("[Ln][Xp]pankki[X]pankk[Sn][Ny]i[Bh][Bc][Ln][Xp]automaatti[X]automaati[Sela][Ny]sta")
	if a.Lemma != "pankkiautomaatti" {
		t.Errorf("lemma=%q want pankkiautomaatti (compound concat)", a.Lemma)
	}
	if a.UPOS != "NOUN" {
		t.Errorf("UPOS=%q want NOUN", a.UPOS)
	}
	if a.GrammarLabel != "elative" {
		t.Errorf("grammar_label=%q want elative (last component governs)", a.GrammarLabel)
	}
}

func TestParse_Verb(t *testing.T) {
	// olen = olla, present 1sg
	a := Parse("[Lt][Xp]olla[X]o[Tt][Ap][P1][Ny][Ef]len")
	if a.Lemma != "olla" {
		t.Errorf("lemma=%q want olla", a.Lemma)
	}
	if a.UPOS != "VERB" {
		t.Errorf("UPOS=%q want VERB", a.UPOS)
	}
	if a.Tense != "Pres" {
		t.Errorf("tense=%q want Pres", a.Tense)
	}
	if a.Person != "1" {
		t.Errorf("person=%q want 1", a.Person)
	}
	if a.Number != "Sing" {
		t.Errorf("number=%q want Sing", a.Number)
	}
	if a.Mood != "Ind" {
		t.Errorf("mood=%q want Ind", a.Mood)
	}
}

func TestParse_VerbPast(t *testing.T) {
	// kysyit = kysyä, past 2sg
	a := Parse("[Lt][Xp]kysyä[X]kysy[Tt][Ai][P2][Ny][Ef]it")
	if a.Lemma != "kysyä" {
		t.Errorf("lemma=%q want kysyä", a.Lemma)
	}
	if a.Tense != "Past" {
		t.Errorf("tense=%q want Past", a.Tense)
	}
	if a.Person != "2" {
		t.Errorf("person=%q want 2", a.Person)
	}
}

func TestParse_Empty(t *testing.T) {
	if a := Parse(""); a.Lemma != "" || a.UPOS != "" {
		t.Errorf("empty input should give empty Analysis, got %+v", a)
	}
}

func TestParse_Adjective(t *testing.T) {
	// hyvää = hyvä, partitive
	a := Parse("[Ll][Xp]hyvä[X]hyvä[Sp][Ny]ä")
	if a.Lemma != "hyvä" {
		t.Errorf("lemma=%q want hyvä", a.Lemma)
	}
	if a.UPOS != "ADJ" {
		t.Errorf("UPOS=%q want ADJ", a.UPOS)
	}
	if a.GrammarLabel != "partitive" {
		t.Errorf("grammar_label=%q want partitive", a.GrammarLabel)
	}
}
