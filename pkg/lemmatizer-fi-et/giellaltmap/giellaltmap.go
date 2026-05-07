// Package giellaltmap parses Giellalt-style FST output strings into
// the parser's Analysis struct. Giellalt tags look like:
//
//	talo+N+Sg+Nom              (basic noun, nominative singular)
//	olla+V+Act+Ind+Prs+Sg1     (verb, active indicative present 1sg)
//	hyvä+A+Sg+Par              (adjective, partitive singular)
//	pankki+N+Cmp/SgNom+Cmp#automaatti+N+Sg+Ela
//	                           (compound: two stems separated by '#';
//	                           we prefer non-compound analyses if any)
//
// The tag set is documented at https://giellalt.github.io/ (see the
// general docs and lang-fin specifically). For our purposes we map a
// useful subset to UD-style POS / grammar features that match the
// finnestdb parser's vocabulary (NOUN, VERB, ADJ; nominative,
// genitive, inessive, ...).
package giellaltmap

import (
	"strings"

	"finnestdb/pkg/lemmatizer-fi-et/udfeats"
)

// Analysis is the parser-facing structured view of one Giellalt output.
// Mirrors voikkomap.Analysis intentionally so the unified Lemmatize
// layer can compare directly.
type Analysis struct {
	Lemma        string
	UPOS         string
	GrammarLabel string
	Number       string
	Tense        string
	Mood         string
	Person       string
	Feats        string // composed UD FEATS string, e.g. "Case=Ine|Number=Sing"; alphabetically sorted
	IsCompound   bool   // contains '#'; used by the unified merger to deprioritise
	Raw          string
}

// Parse turns one Giellalt FST output line into an Analysis. Returns
// the zero Analysis if the input doesn't look like a Giellalt analysis.
func Parse(out string) Analysis {
	a := Analysis{Raw: out}
	if out == "" {
		return a
	}
	// Compound boundary: "lemma1+TAGS#lemma2+TAGS". We take the LAST
	// segment for inflection tags (Number/Case/etc.) and concatenate
	// all segments' lemma parts.
	if strings.Contains(out, "#") {
		a.IsCompound = true
	}
	segments := strings.Split(out, "#")
	var lemmaBuilder strings.Builder
	var lastSeg string
	for _, seg := range segments {
		// Each segment is "stem+TAG+TAG+...".
		// The stem is everything up to the first '+'.
		if plus := strings.IndexByte(seg, '+'); plus >= 0 {
			lemmaBuilder.WriteString(seg[:plus])
		} else {
			lemmaBuilder.WriteString(seg)
		}
		lastSeg = seg
	}
	a.Lemma = lemmaBuilder.String()

	// Parse tags from the LAST segment — that's where the inflection lives.
	tags := strings.Split(lastSeg, "+")
	if len(tags) > 1 {
		applyTags(&a, tags[1:])
	}
	a.Feats = udfeats.Compose(a.GrammarLabel, a.Number, a.Tense, a.Mood, a.Person)
	return a
}

// applyTags walks the per-tag list (after the lemma) and fills the
// Analysis fields. Unknown tags are ignored.
func applyTags(a *Analysis, tags []string) {
	for _, tag := range tags {
		switch tag {
		// POS
		case "N":
			a.UPOS = "NOUN"
		case "V":
			a.UPOS = "VERB"
		case "A", "Adj":
			a.UPOS = "ADJ"
		case "Adv":
			a.UPOS = "ADV"
		case "Pron":
			a.UPOS = "PRON"
		case "Num":
			a.UPOS = "NUM"
		case "CC":
			a.UPOS = "CCONJ"
		case "CS":
			a.UPOS = "SCONJ"
		case "Po", "Pr", "Adp":
			a.UPOS = "ADP"
		case "Interj":
			a.UPOS = "INTJ"
		case "Pcle", "Part":
			a.UPOS = "PART"
		case "ACR", "Acro":
			a.UPOS = "NOUN"
		case "Prop":
			a.UPOS = "PROPN"

		// Number
		case "Sg":
			a.Number = "Sing"
		case "Pl":
			a.Number = "Plur"

		// Case
		case "Nom":
			a.GrammarLabel = "nominative"
		case "Gen":
			a.GrammarLabel = "genitive"
		case "Par":
			a.GrammarLabel = "partitive"
		case "Ess":
			a.GrammarLabel = "essive"
		case "Tra":
			a.GrammarLabel = "translative"
		case "Ine":
			a.GrammarLabel = "inessive"
		case "Ela":
			a.GrammarLabel = "elative"
		case "Ill":
			a.GrammarLabel = "illative"
		case "Ade":
			a.GrammarLabel = "adessive"
		case "Abl":
			a.GrammarLabel = "ablative"
		case "All":
			a.GrammarLabel = "allative"
		case "Abe":
			a.GrammarLabel = "abessive"
		case "Com":
			a.GrammarLabel = "comitative"
		case "Ins":
			a.GrammarLabel = "instructive"

		// Tense
		case "Prs":
			a.Tense = "Pres"
		case "Prt":
			a.Tense = "Past"

		// Mood
		case "Ind":
			a.Mood = "Ind"
		case "Cnd":
			a.Mood = "Cnd"
		case "Imprt", "Imp":
			a.Mood = "Imp"
		case "Pot":
			a.Mood = "Pot"

		// Person+Number combinations: Sg1, Sg2, Sg3, Pl1, Pl2, Pl3
		case "Sg1":
			a.Number = "Sing"
			a.Person = "1"
		case "Sg2":
			a.Number = "Sing"
			a.Person = "2"
		case "Sg3":
			a.Number = "Sing"
			a.Person = "3"
		case "Pl1":
			a.Number = "Plur"
			a.Person = "1"
		case "Pl2":
			a.Number = "Plur"
			a.Person = "2"
		case "Pl3":
			a.Number = "Plur"
			a.Person = "3"
		}
	}
}
