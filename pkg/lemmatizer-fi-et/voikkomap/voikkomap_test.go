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
	if a.VerbForm != "Fin" {
		t.Errorf("verbform=%q want Fin", a.VerbForm)
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
	if a.VerbForm != "Fin" {
		t.Errorf("verbform=%q want Fin", a.VerbForm)
	}
}

func TestParse_Infinitive(t *testing.T) {
	// First infinitive: [Tn1] should set VerbForm=Inf, not Mood=Inf
	a := Parse("[Lt][Xp]olla[X]ol[Tn1]la")
	if a.VerbForm != "Inf" {
		t.Errorf("verbform=%q want Inf", a.VerbForm)
	}
	if a.Mood != "" {
		t.Errorf("mood=%q want empty (infinitives have no mood)", a.Mood)
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

func TestParse_DegreeComparative(t *testing.T) {
	a := Parse("[Ll][Xp]suuri[X]suure[Sn][Ny][Cc]mpi")
	if a.Degree != "Cmp" {
		t.Errorf("degree=%q want Cmp", a.Degree)
	}
	if a.UPOS != "ADJ" {
		t.Errorf("upos=%q want ADJ", a.UPOS)
	}
}

func TestParse_DegreeSuperlative(t *testing.T) {
	a := Parse("[Ll][Xp]suuri[X]suuri[Sp][Ny][Cs]nta")
	if a.Degree != "Sup" {
		t.Errorf("degree=%q want Sup", a.Degree)
	}
}

func TestParse_InfForm1(t *testing.T) {
	a := Parse("[Lt][Xp]olla[X]ol[Tn1]la")
	if a.VerbForm != "Inf" || a.InfForm != "1" {
		t.Errorf("verbform=%q infform=%q want Inf/1", a.VerbForm, a.InfForm)
	}
}

func TestParse_InfForm3(t *testing.T) {
	a := Parse("[Lt][Xp]puhua[X]puhu[Tn3][Sine]massa")
	if a.VerbForm != "Inf" || a.InfForm != "3" {
		t.Errorf("verbform=%q infform=%q want Inf/3", a.VerbForm, a.InfForm)
	}
	if a.GrammarLabel != "inessive" {
		t.Errorf("grammar=%q want inessive", a.GrammarLabel)
	}
}

func TestParse_PresentParticiple(t *testing.T) {
	a := Parse("[Lt][Rv][Xp]puhua[X]puhu[Sn][Ny]va")
	if a.VerbForm != "Part" || a.PartForm != "Pres" {
		t.Errorf("verbform=%q partform=%q want Part/Pres", a.VerbForm, a.PartForm)
	}
}

func TestParse_PastParticiple(t *testing.T) {
	a := Parse("[Lt][Ru][Xp]puhua[X]puhu[Sn][Ny]nut")
	if a.VerbForm != "Part" || a.PartForm != "Past" {
		t.Errorf("verbform=%q partform=%q want Part/Past", a.VerbForm, a.PartForm)
	}
}

func TestParse_AgentParticiple(t *testing.T) {
	a := Parse("[Lt][Rm][Xp]tehdä[X]teke[Sn][Ny]mä")
	if a.VerbForm != "Part" || a.PartForm != "Agt" {
		t.Errorf("verbform=%q partform=%q want Part/Agt", a.VerbForm, a.PartForm)
	}
}

func TestParse_NegParticiple(t *testing.T) {
	a := Parse("[Lt][Re][Xp]tehdä[X]teke[Sn][Ny]mätön")
	if a.VerbForm != "Part" || a.PartForm != "Neg" {
		t.Errorf("verbform=%q partform=%q want Part/Neg", a.VerbForm, a.PartForm)
	}
}

func TestParse_PassivePastParticiple(t *testing.T) {
	a := Parse("[Lt][Rt][Xp]puhua[X]puhu[Sn][Ny]ttu")
	if a.VerbForm != "Part" || a.PartForm != "Past" {
		t.Errorf("verbform=%q partform=%q want Part/Past", a.VerbForm, a.PartForm)
	}
	if a.Voice != "Pass" {
		t.Errorf("voice=%q want Pass", a.Voice)
	}
}

func TestParse_PassivePresentParticiple(t *testing.T) {
	a := Parse("[Lt][Ra][Xp]puhua[X]puhu[Sn][Ny]ttava")
	if a.VerbForm != "Part" || a.PartForm != "Pres" {
		t.Errorf("verbform=%q partform=%q want Part/Pres", a.VerbForm, a.PartForm)
	}
	if a.Voice != "Pass" {
		t.Errorf("voice=%q want Pass", a.Voice)
	}
}

func TestParse_PossessiveSg1(t *testing.T) {
	a := Parse("[Ln][Xp]talo[X]talo[Sine][Ny][O1y]ssani")
	if a.PersonPsor != "1" || a.NumberPsor != "Sing" {
		t.Errorf("personpsor=%q numberpsor=%q want 1/Sing", a.PersonPsor, a.NumberPsor)
	}
}

func TestParse_PossessivePl2(t *testing.T) {
	a := Parse("[Ln][Xp]talo[X]talo[Sine][Ny][O2m]ssanne")
	if a.PersonPsor != "2" || a.NumberPsor != "Plur" {
		t.Errorf("personpsor=%q numberpsor=%q want 2/Plur", a.PersonPsor, a.NumberPsor)
	}
}

func TestParse_Possessive3(t *testing.T) {
	a := Parse("[Ln][Xp]talo[X]talo[Sg][Ny][O3]nsa")
	if a.PersonPsor != "3" {
		t.Errorf("personpsor=%q want 3", a.PersonPsor)
	}
	if a.NumberPsor != "" {
		t.Errorf("numberpsor=%q want empty (3rd person ambiguous)", a.NumberPsor)
	}
}

func TestParse_CliticKo(t *testing.T) {
	a := Parse("[Lt][Xp]olla[X]o[Tt][Ap][P3][Ny][Fko]nko")
	if a.Clitic != "Ko" {
		t.Errorf("clitic=%q want Ko", a.Clitic)
	}
}

func TestParse_StackedClitics(t *testing.T) {
	a := Parse("[Lt][Xp]olla[X]o[Tt][Ap][P3][Ny][Fko][Fhan]nkohan")
	if a.Clitic != "Han,Ko" {
		t.Errorf("clitic=%q want Han,Ko (sorted)", a.Clitic)
	}
}

func TestParse_Connegative(t *testing.T) {
	a := Parse("[Lt][Xp]puhua[X]puhu[Tt][Cn]")
	if a.Connegative != "Yes" {
		t.Errorf("connegative=%q want Yes", a.Connegative)
	}
}

func TestParse_NumTypeCard(t *testing.T) {
	a := Parse("[Lu][Xp]kolme[X]kolme[Sn][Ny]")
	if a.NumType != "Card" {
		t.Errorf("numtype=%q want Card", a.NumType)
	}
	if a.UPOS != "NUM" {
		t.Errorf("upos=%q want NUM", a.UPOS)
	}
}

func TestParse_NumTypeOrd(t *testing.T) {
	a := Parse("[Lur][Xp]kolmas[X]kolma[Sn][Ny]s")
	if a.NumType != "Ord" {
		t.Errorf("numtype=%q want Ord", a.NumType)
	}
}

func TestParse_FeatsComposition(t *testing.T) {
	cases := []struct {
		name, input, wantFeats string
	}{
		{
			"comparative adjective",
			"[Ll][Xp]suuri[X]suure[Sn][Ny][Cc]mpi",
			"Degree=Cmp|Number=Sing",
		},
		{
			"3rd infinitive inessive",
			"[Lt][Xp]puhua[X]puhu[Tn3][Sine]massa",
			"Case=Ine|InfForm=3|VerbForm=Inf",
		},
		{
			"past participle",
			"[Lt][Ru][Xp]puhua[X]puhu[Sn][Ny]nut",
			"Number=Sing|PartForm=Past|VerbForm=Part",
		},
		{
			"noun with possessive",
			"[Ln][Xp]talo[X]talo[Sine][Ny][O1y]ssani",
			"Case=Ine|Number=Sing|Number[psor]=Sing|Person[psor]=1",
		},
		{
			"verb with clitic",
			"[Lt][Xp]olla[X]o[Tt][Ap][P3][Ny][Fko]nko",
			"Clitic=Ko|Mood=Ind|Number=Sing|Person=3|Tense=Pres|VerbForm=Fin",
		},
		{
			"connegative verb",
			"[Lt][Xp]puhua[X]puhu[Tt][Cn]",
			"Connegative=Yes|Mood=Ind|VerbForm=Fin",
		},
	}
	for _, tc := range cases {
		a := Parse(tc.input)
		if a.Feats != tc.wantFeats {
			t.Errorf("%s: Parse(%q).Feats = %q, want %q", tc.name, tc.input, a.Feats, tc.wantFeats)
		}
	}
}
