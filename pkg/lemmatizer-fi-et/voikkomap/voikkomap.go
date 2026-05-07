// Package voikkomap maps libvoikko's FSTOUTPUT tag stream to the
// finnestdb parser's structured form (Lemma, UPOS, grammar_label,
// Number, Tense, ...). The mapping is derived from libvoikko's
// FinnishVfstAnalyzer.cpp (parseBasicAttributes); see classMap,
// sijamuotoMap, numberMap there for the upstream source of truth.
//
// FSTOUTPUT shape, by example:
//
//	"talossa"           [Ln][Ica][Xp]talo[X]talo[Sine][Ny]ssa
//	"pankkiautomaatista" [Ln][Xp]pankki[X]pankk[Sn][Ny]i[Bh][Bc][Ln][Xp]automaatti[X]automaati[Sela][Ny]sta
//	"olen"              [Lt][Xp]olla[X]o[Tt][Ap][P1][Ny][Ef]len
//
// Tag categories used here:
//   - [L*]              part of speech (last one wins for compound heads)
//   - [Xp]...[X]        a lemma segment; the parsed Lemma is the
//                       concatenation of all such segments (compound-aware)
//   - [S*]              case (sijamuoto)
//   - [N*]              number
//   - [T*]              mood (Tt = indicative, Te = conditional, ...)
//   - [A*]              tense (Ap = present, Ai = past)
//   - [P*]              person (P1/P2/P3)
//   - [B*]              compound boundary (ignored)
//   - everything else   ignored for now
package voikkomap

import (
	"strings"

	"finnestdb/pkg/lemmatizer-fi-et/udfeats"
)

// Analysis is the parser-facing structured view of one FSTOUTPUT line.
type Analysis struct {
	Lemma        string // full lemma; for compounds, all [Xp]...[X] segments concatenated
	UPOS         string // Universal POS tag, e.g. NOUN, VERB, ADJ, ADV, PRON, NUM, ADP, CCONJ, AUX, PROPN, INTJ
	GrammarLabel string // e.g. "inessive", "elative", "partitive"; matches internal/parserules vocabulary
	Number       string // "Sing" / "Plur" (UD), or empty
	Tense        string // "Pres" / "Past" or empty
	Mood         string // "Ind" / "Cnd" / "Imp" / "Pot" or empty
	Person       string // "1" / "2" / "3" / "4" or empty
	Voice        string // "Act" / "Pass" or empty
	VerbForm     string // "Fin" / "Inf" / "Part" or empty
	Degree       string // "Pos" / "Cmp" / "Sup" or empty
	PronType     string // "Dem" / "Int" / "Rel" / "Ind" / "Prs" / "Rfl" / "Rcp" or empty
	PartForm     string // "Pres" / "Past" / "Agt" / "Neg" or empty (Finnish-specific)
	InfForm      string // "1" / "2" / "3" / "5" or empty (Finnish-specific)
	PersonPsor   string // "1" / "2" / "3" or empty — Person[psor]
	NumberPsor   string // "Sing" / "Plur" or empty — Number[psor]
	Clitic       string // "Ko" / "Han" / "Pa" / "Kaan" / "Ka" / "Kin" / "S" or empty
	NumType      string // "Card" / "Ord" or empty
	Connegative  string // "Yes" or empty
	AdpType      string // "Post" / "Prep" or empty
	Feats        string // composed UD FEATS string, e.g. "Case=Ine|Number=Sing"; alphabetically sorted
	Raw          string // the original FSTOUTPUT, for debugging / source provenance
}

// Parse turns one FSTOUTPUT line into an Analysis. Returns the zero
// Analysis if fstOutput doesn't look like a Voikko output. Multiple
// readings of one surface form mean the caller has already split into
// lines; this function handles a single line.
func Parse(fstOutput string) Analysis {
	a := Analysis{Raw: fstOutput}
	if fstOutput == "" {
		return a
	}

	var lemmaBuilder strings.Builder
	insideXp := false  // we're between [Xp] and [X]
	xpStart := 0        // start index of current lemma segment

	i := 0
	for i < len(fstOutput) {
		if fstOutput[i] == '[' {
			end := strings.IndexByte(fstOutput[i:], ']')
			if end < 0 {
				break // malformed; bail
			}
			tag := fstOutput[i+1 : i+end]

			if insideXp {
				// We were collecting lemma chars between [Xp] and [X]. The tag
				// we just hit closes that segment.
				lemmaBuilder.WriteString(fstOutput[xpStart : i])
				insideXp = false
			}

			switch {
			case tag == "Xp":
				insideXp = true
				xpStart = i + end + 1 // position right after ']'
			case tag == "X":
				// Already handled above by the insideXp close. Nothing to do.
			case strings.HasPrefix(tag, "L"):
				body := tag[1:]
				if pos := classToUPOS(body); pos != "" {
					a.UPOS = pos
				}
				if nt := numTypeFromClass(body); nt != "" {
					a.NumType = nt
				}
			case strings.HasPrefix(tag, "S"):
				if grammar := sijamuotoToLabel(tag[1:]); grammar != "" {
					a.GrammarLabel = grammar
				}
			case strings.HasPrefix(tag, "N"):
				if num := numberToUD(tag[1:]); num != "" {
					a.Number = num
				}
			case strings.HasPrefix(tag, "T"):
				body := tag[1:]
				if mood := moodToUD(body); mood != "" {
					if mood == "Inf" {
						a.VerbForm = "Inf"
						a.InfForm = infFormFromMood(body)
					} else {
						a.Mood = mood
						a.VerbForm = "Fin"
					}
				}
			case strings.HasPrefix(tag, "A"):
				if tense := tenseToUD(tag[1:]); tense != "" {
					a.Tense = tense
				}
			case strings.HasPrefix(tag, "P"):
				if person := tag[1:]; len(person) == 1 && person[0] >= '1' && person[0] <= '4' {
					a.Person = person
				}
			case strings.HasPrefix(tag, "R"):
				applyParticiple(&a, tag[1:])
			case strings.HasPrefix(tag, "C"):
				applyComparison(&a, tag[1:])
			case strings.HasPrefix(tag, "O"):
				applyPossessive(&a, tag[1:])
			case strings.HasPrefix(tag, "F"):
				if cl := cliticToUD(tag[1:]); cl != "" {
					a.Clitic = udfeats.AppendSortedValue(a.Clitic, cl)
				}
			}

			i += end + 1
			continue
		}
		i++
	}
	if insideXp {
		// FSTOUTPUT ended without closing [X] — take what we have
		lemmaBuilder.WriteString(fstOutput[xpStart:])
	}

	a.Lemma = lemmaBuilder.String()
	if a.UPOS == "ADJ" && a.Degree == "" {
		a.Degree = "Pos"
	}
	a.Feats = composeFeats(&a)
	return a
}

func composeFeats(a *Analysis) string {
	pairs := make(map[string]string, 16)
	if udCase, ok := udfeats.LegacyLabelToUDCase[a.GrammarLabel]; ok && udCase != "Nom" {
		pairs["Case"] = udCase
	}
	pairs["Number"] = a.Number
	pairs["Tense"] = a.Tense
	pairs["Mood"] = a.Mood
	pairs["Person"] = a.Person
	pairs["Voice"] = a.Voice
	pairs["VerbForm"] = a.VerbForm
	pairs["Degree"] = a.Degree
	pairs["PronType"] = a.PronType
	pairs["PartForm"] = a.PartForm
	pairs["InfForm"] = a.InfForm
	pairs["Person[psor]"] = a.PersonPsor
	pairs["Number[psor]"] = a.NumberPsor
	pairs["Clitic"] = a.Clitic
	pairs["NumType"] = a.NumType
	pairs["Connegative"] = a.Connegative
	pairs["AdpType"] = a.AdpType
	return udfeats.ComposeMap(pairs)
}

// classToUPOS maps the body of a [L*] tag (e.g. "n", "t", "ee") to a
// Universal Dependencies POS tag. Source: libvoikko classMap.
func classToUPOS(body string) string {
	switch body {
	case "n":
		return "NOUN" // nimisana
	case "l":
		return "ADJ" // laatusana
	case "nl":
		return "NOUN" // nimisana_laatusana — context-dependent; default NOUN
	case "h":
		return "INTJ" // huudahdussana
	case "ee", "es", "ep", "em":
		return "PROPN" // etunimi/sukunimi/paikannimi/nimi
	case "t":
		return "VERB" // teonsana
	case "a":
		return "NOUN" // lyhenne (abbreviation) — default to NOUN; X is also defensible
	case "s":
		return "ADV" // seikkasana
	case "u", "ur":
		return "NUM" // lukusana
	case "r":
		return "PRON" // asemosana
	case "c":
		return "CCONJ" // sidesana — could also be SCONJ; default CCONJ
	case "d":
		return "ADP" // suhdesana
	case "k":
		return "AUX" // kieltosana (negation aux "ei")
	case "p":
		return "X" // etuliite (prefix); not a normal token
	}
	return ""
}

// sijamuotoToLabel maps the body of a [S*] tag to the grammar_label
// vocabulary used by parserules. Source: libvoikko sijamuotoMap.
func sijamuotoToLabel(body string) string {
	switch body {
	case "n":
		return "nominative"
	case "g":
		return "genitive"
	case "p":
		return "partitive"
	case "es":
		return "essive"
	case "tr":
		return "translative"
	case "ine":
		return "inessive"
	case "ela":
		return "elative"
	case "ill":
		return "illative"
	case "ade":
		return "adessive"
	case "abl":
		return "ablative"
	case "all":
		return "allative"
	case "ab":
		return "abessive"
	case "ko":
		return "comitative"
	case "in":
		return "instructive"
	case "ak":
		return "accusative"
	case "sti":
		return "" // kerrontosti — no UD case; treated as adverb suffix
	}
	return ""
}

func numberToUD(body string) string {
	switch body {
	case "y":
		return "Sing"
	case "m":
		return "Plur"
	}
	return ""
}

func moodToUD(body string) string {
	switch body {
	case "t":
		return "Ind"
	case "e":
		return "Cnd"
	case "k":
		return "Imp"
	case "m":
		return "Pot"
	case "n1", "n2", "n3", "n4", "n5":
		return "Inf" // infinitive forms; UD lumps as VerbForm=Inf rather than Mood
	}
	return ""
}

func tenseToUD(body string) string {
	switch body {
	case "p":
		return "Pres"
	case "i":
		return "Past"
	}
	return ""
}

func infFormFromMood(body string) string {
	switch body {
	case "n1":
		return "1"
	case "n2":
		return "2"
	case "n3":
		return "3"
	case "n4":
		return "4"
	case "n5":
		return "5"
	}
	return ""
}

// applyParticiple handles [R*] tags — Voikko's participle/derivation
// family. Sets VerbForm, PartForm, and Voice where the tag implies
// active vs. passive.
func applyParticiple(a *Analysis, body string) {
	switch body {
	case "v": // VA-participle (active present)
		a.VerbForm = "Part"
		a.PartForm = "Pres"
	case "u": // NUT-participle (active past)
		a.VerbForm = "Part"
		a.PartForm = "Past"
	case "t": // TU-participle (passive past)
		a.VerbForm = "Part"
		a.PartForm = "Past"
		a.Voice = "Pass"
	case "a": // TAVA-participle (passive present)
		a.VerbForm = "Part"
		a.PartForm = "Pres"
		a.Voice = "Pass"
	case "m": // MA-participle (agent)
		a.VerbForm = "Part"
		a.PartForm = "Agt"
	case "e": // MATON-participle (negative/caritive)
		a.VerbForm = "Part"
		a.PartForm = "Neg"
	}
}

// applyComparison handles [C*] tags — degree of comparison AND
// connegative. In Voikko FSTOUTPUT, [Cc]=comparative, [Cs]=superlative,
// [Cn]=connegative. Positive degree is unmarked (no tag emitted).
func applyComparison(a *Analysis, body string) {
	switch body {
	case "c":
		a.Degree = "Cmp"
	case "s":
		a.Degree = "Sup"
	case "n":
		a.Connegative = "Yes"
	}
}

func numTypeFromClass(body string) string {
	switch body {
	case "u":
		return "Card"
	case "ur":
		return "Ord"
	}
	return ""
}

func applyPossessive(a *Analysis, body string) {
	switch body {
	case "1y":
		a.PersonPsor = "1"
		a.NumberPsor = "Sing"
	case "2y":
		a.PersonPsor = "2"
		a.NumberPsor = "Sing"
	case "1m":
		a.PersonPsor = "1"
		a.NumberPsor = "Plur"
	case "2m":
		a.PersonPsor = "2"
		a.NumberPsor = "Plur"
	case "3":
		a.PersonPsor = "3"
	}
}

func cliticToUD(body string) string {
	switch body {
	case "ko":
		return "Ko"
	case "han":
		return "Han"
	case "pa":
		return "Pa"
	case "kaan":
		return "Kaan"
	case "ka":
		return "Ka"
	case "kin":
		return "Kin"
	case "s":
		return "S"
	}
	return ""
}
