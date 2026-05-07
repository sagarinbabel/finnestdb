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
	if a.PartForm != "Past" {
		t.Errorf("partform=%q want Past", a.PartForm)
	}
	if a.Voice != "Act" {
		t.Errorf("voice=%q want Act", a.Voice)
	}
}

func TestParse_PresentParticiple(t *testing.T) {
	a := Parse("puhua+V+Act+PrsPrc+Sg+Nom")
	if a.VerbForm != "Part" || a.PartForm != "Pres" {
		t.Errorf("verbform=%q partform=%q want Part/Pres", a.VerbForm, a.PartForm)
	}
}

func TestParse_AgentParticiple(t *testing.T) {
	a := Parse("tehdä+V+Act+AgPrc+Sg+Nom")
	if a.VerbForm != "Part" || a.PartForm != "Agt" {
		t.Errorf("verbform=%q partform=%q want Part/Agt", a.VerbForm, a.PartForm)
	}
}

func TestParse_NegParticiple(t *testing.T) {
	a := Parse("tehdä+V+NegPrc+Sg+Par")
	if a.VerbForm != "Part" || a.PartForm != "Neg" {
		t.Errorf("verbform=%q partform=%q want Part/Neg", a.VerbForm, a.PartForm)
	}
}

func TestParse_InfA(t *testing.T) {
	a := Parse("puhua+V+Act+InfA")
	if a.VerbForm != "Inf" || a.InfForm != "1" {
		t.Errorf("verbform=%q infform=%q want Inf/1", a.VerbForm, a.InfForm)
	}
}

func TestParse_InfE(t *testing.T) {
	a := Parse("puhua+V+Act+InfE+Ine")
	if a.VerbForm != "Inf" || a.InfForm != "2" {
		t.Errorf("verbform=%q infform=%q want Inf/2", a.VerbForm, a.InfForm)
	}
	if a.GrammarLabel != "inessive" {
		t.Errorf("grammar=%q want inessive", a.GrammarLabel)
	}
}

func TestParse_InfMA(t *testing.T) {
	a := Parse("puhua+V+Act+InfMA+Ill")
	if a.VerbForm != "Inf" || a.InfForm != "3" {
		t.Errorf("verbform=%q infform=%q want Inf/3", a.VerbForm, a.InfForm)
	}
}

func TestParse_InfMIST(t *testing.T) {
	a := Parse("olla+V+Act+InfMIST")
	if a.VerbForm != "Inf" || a.InfForm != "5" {
		t.Errorf("verbform=%q infform=%q want Inf/5", a.VerbForm, a.InfForm)
	}
}

func TestParse_DegreePositive(t *testing.T) {
	a := Parse("suuri+A+Pos+Sg+Nom")
	if a.Degree != "Pos" {
		t.Errorf("degree=%q want Pos", a.Degree)
	}
	if a.UPOS != "ADJ" {
		t.Errorf("upos=%q want ADJ", a.UPOS)
	}
}

func TestParse_DegreeComparative(t *testing.T) {
	a := Parse("suuri+A+Comp+Sg+Nom")
	if a.Degree != "Cmp" {
		t.Errorf("degree=%q want Cmp", a.Degree)
	}
}

func TestParse_DegreeSuperlative(t *testing.T) {
	a := Parse("suuri+A+Superl+Sg+Nom")
	if a.Degree != "Sup" {
		t.Errorf("degree=%q want Sup", a.Degree)
	}
}

func TestParse_PronTypeDem(t *testing.T) {
	a := Parse("tämä+Pron+Dem+Sg+Nom")
	if a.PronType != "Dem" {
		t.Errorf("prontype=%q want Dem", a.PronType)
	}
	if a.UPOS != "PRON" {
		t.Errorf("upos=%q want PRON", a.UPOS)
	}
}

func TestParse_PronTypeInterr(t *testing.T) {
	a := Parse("kuka+Pron+Interr+Sg+Nom")
	if a.PronType != "Int" {
		t.Errorf("prontype=%q want Int", a.PronType)
	}
}

func TestParse_PronTypeRel(t *testing.T) {
	a := Parse("joka+Pron+Rel+Sg+Nom")
	if a.PronType != "Rel" {
		t.Errorf("prontype=%q want Rel", a.PronType)
	}
}

func TestParse_PronTypePers(t *testing.T) {
	a := Parse("minä+Pron+Pers+Sg+Nom")
	if a.PronType != "Prs" {
		t.Errorf("prontype=%q want Prs", a.PronType)
	}
}

func TestParse_PronTypeIndef(t *testing.T) {
	a := Parse("joku+Pron+Indef+Sg+Nom")
	if a.PronType != "Ind" {
		t.Errorf("prontype=%q want Ind", a.PronType)
	}
}

func TestParse_PronTypeRefl(t *testing.T) {
	a := Parse("itse+Pron+Refl+Sg+Nom")
	if a.PronType != "Rfl" {
		t.Errorf("prontype=%q want Rfl", a.PronType)
	}
}

func TestParse_PronTypeRecipr(t *testing.T) {
	a := Parse("toinen+Pron+Recipr+Pl+Nom")
	if a.PronType != "Rcp" {
		t.Errorf("prontype=%q want Rcp", a.PronType)
	}
}

func TestParse_NumTypeCard(t *testing.T) {
	a := Parse("kolme+Num+Card+Sg+Nom")
	if a.NumType != "Card" {
		t.Errorf("numtype=%q want Card", a.NumType)
	}
	if a.UPOS != "NUM" {
		t.Errorf("upos=%q want NUM", a.UPOS)
	}
}

func TestParse_NumTypeOrd(t *testing.T) {
	a := Parse("kolmas+Num+Ord+Sg+Nom")
	if a.NumType != "Ord" {
		t.Errorf("numtype=%q want Ord", a.NumType)
	}
}

func TestParse_PossessiveSg1(t *testing.T) {
	a := Parse("talo+N+Sg+Nom+PxSg1")
	if a.PersonPsor != "1" || a.NumberPsor != "Sing" {
		t.Errorf("personpsor=%q numberpsor=%q want 1/Sing", a.PersonPsor, a.NumberPsor)
	}
}

func TestParse_PossessivePl3(t *testing.T) {
	a := Parse("talo+N+Sg+Ine+PxPl3")
	if a.PersonPsor != "3" || a.NumberPsor != "Plur" {
		t.Errorf("personpsor=%q numberpsor=%q want 3/Plur", a.PersonPsor, a.NumberPsor)
	}
}

func TestParse_PossessivePx3(t *testing.T) {
	a := Parse("talo+N+Sg+Gen+Px3")
	if a.PersonPsor != "3" {
		t.Errorf("personpsor=%q want 3", a.PersonPsor)
	}
	if a.NumberPsor != "" {
		t.Errorf("numberpsor=%q want empty (Px3 is number-ambiguous)", a.NumberPsor)
	}
}

func TestParse_CliticQst(t *testing.T) {
	a := Parse("puhua+V+Act+Ind+Prs+Sg3+Qst")
	if a.Clitic != "Ko" {
		t.Errorf("clitic=%q want Ko", a.Clitic)
	}
}

func TestParse_CliticFocHan(t *testing.T) {
	a := Parse("olla+V+Act+Ind+Prs+Sg3+Foc/han")
	if a.Clitic != "Han" {
		t.Errorf("clitic=%q want Han", a.Clitic)
	}
}

func TestParse_CliticFocPa(t *testing.T) {
	a := Parse("olla+V+Act+Ind+Prs+Sg3+Foc/pa")
	if a.Clitic != "Pa" {
		t.Errorf("clitic=%q want Pa", a.Clitic)
	}
}

func TestParse_Connegative(t *testing.T) {
	a := Parse("puhua+V+Act+Ind+Prs+ConNeg")
	if a.Connegative != "Yes" {
		t.Errorf("connegative=%q want Yes", a.Connegative)
	}
}

func TestParse_AdpTypePost(t *testing.T) {
	a := Parse("kanssa+Po")
	if a.UPOS != "ADP" || a.AdpType != "Post" {
		t.Errorf("upos=%q adptype=%q want ADP/Post", a.UPOS, a.AdpType)
	}
}

func TestParse_AdpTypePrep(t *testing.T) {
	a := Parse("ennen+Pr")
	if a.UPOS != "ADP" || a.AdpType != "Prep" {
		t.Errorf("upos=%q adptype=%q want ADP/Prep", a.UPOS, a.AdpType)
	}
}

func TestParse_FeatsComposition(t *testing.T) {
	cases := []struct {
		name, input, wantFeats string
	}{
		{
			"comparative adjective",
			"suuri+A+Comp+Sg+Par",
			"Case=Par|Degree=Cmp|Number=Sing",
		},
		{
			"demonstrative pronoun inessive",
			"tämä+Pron+Dem+Sg+Ine",
			"Case=Ine|Number=Sing|PronType=Dem",
		},
		{
			"3rd infinitive inessive",
			"puhua+V+Act+InfMA+Ine",
			"Case=Ine|InfForm=3|VerbForm=Inf|Voice=Act",
		},
		{
			"past participle with PartForm",
			"sanoa+V+Act+PrfPrc+Sg+Nom",
			"Number=Sing|PartForm=Past|VerbForm=Part|Voice=Act",
		},
		{
			"noun with possessive suffix",
			"talo+N+Sg+Ine+PxSg1",
			"Case=Ine|Number=Sing|Number[psor]=Sing|Person[psor]=1",
		},
		{
			"verb with clitic",
			"puhua+V+Act+Ind+Prs+Sg3+Qst",
			"Clitic=Ko|Mood=Ind|Number=Sing|Person=3|Tense=Pres|VerbForm=Fin|Voice=Act",
		},
		{
			"connegative verb",
			"puhua+V+Act+Ind+Prs+ConNeg",
			"Connegative=Yes|Mood=Ind|Tense=Pres|VerbForm=Fin|Voice=Act",
		},
		{
			"ordinal numeral",
			"kolmas+Num+Ord+Sg+Nom",
			"NumType=Ord|Number=Sing",
		},
		{
			"postposition",
			"kanssa+Po",
			"AdpType=Post",
		},
	}
	for _, tc := range cases {
		a := Parse(tc.input)
		if a.Feats != tc.wantFeats {
			t.Errorf("%s: Parse(%q).Feats = %q, want %q", tc.name, tc.input, a.Feats, tc.wantFeats)
		}
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
