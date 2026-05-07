// Package udfeats centralises the FI/ET grammar_label ↔ UD Case mapping
// and the FEATS-string composer used by every FST analyzer
// (voikkomap, giellaltmap) and by the dict layer's runtime composer.
//
// Two callers, one source of truth:
//
//   - voikkomap.Analysis and giellaltmap.Analysis fill their morphological
//     fields (Number, Tense, Mood, Person, GrammarLabel) and call
//     udfeats.Compose(...) once at Parse() time, persisting the FEATS
//     string in their Feats field. Generated table JSONs are then
//     self-describing.
//   - internal/store/dict.go calls Compose for analyses that arrive
//     without a pre-built Feats (older table files, FST results that
//     bypass the table cache).
//
// The `legacyLabel` vocabulary is the lowercase English case names
// finnestdb has used since the case-only parser ("inessive", "elative",
// ...). Adding a new case here, in voikkomap.sijamuotoToLabel, and in
// giellaltmap.applyTags is what it takes to support a new case
// end-to-end.
package udfeats

// LegacyLabelToUDCase maps the parser's lowercase English case name to
// the UD Case= value. The reverse map is UDCaseToLegacyLabel.
//
// "Nom" deliberately maps because some analyzers emit it explicitly,
// but Compose treats it as implicit per UD convention (returns no
// Case= attribute when Nom is the only signal).
var LegacyLabelToUDCase = map[string]string{
	"nominative":  "Nom",
	"genitive":    "Gen",
	"partitive":   "Par",
	"illative":    "Ill",
	"inessive":    "Ine",
	"elative":     "Ela",
	"allative":    "All",
	"adessive":    "Ade",
	"ablative":    "Abl",
	"essive":      "Ess",
	"translative": "Tra",
	"instructive": "Ins",
	"abessive":    "Abe",
	"comitative":  "Com",
	"terminative": "Ter",
	"accusative":  "Acc",
	"vocative":    "Voc",
}

// UDCaseToLegacyLabel is the inverse of LegacyLabelToUDCase, except
// "Nom" maps to "" because the parser's existing convention is to
// leave nominative tokens with an empty grammar_label.
var UDCaseToLegacyLabel = map[string]string{
	"Gen": "genitive",
	"Par": "partitive",
	"Ill": "illative",
	"Ine": "inessive",
	"Ela": "elative",
	"All": "allative",
	"Ade": "adessive",
	"Abl": "ablative",
	"Ess": "essive",
	"Tra": "translative",
	"Ins": "instructive",
	"Abe": "abessive",
	"Com": "comitative",
	"Ter": "terminative",
	"Acc": "accusative",
	"Voc": "vocative",
}

// Compose builds a UD-canonical FEATS string from the structured
// fields an FST analyzer fills. Returns "" when nothing maps.
//
// Attribute order is alphabetical (UD canonical). Empty inputs are
// skipped. "Nom" is treated as implicit and omitted.
//
// The signature is deliberately positional rather than struct-based so
// both voikkomap and giellaltmap can call it without taking a dependency
// on each other or on a shared Analysis type.
func Compose(grammarLabel, number, tense, mood, person string) string {
	pairs := make(map[string]string, 5)
	if udCase, ok := LegacyLabelToUDCase[grammarLabel]; ok && udCase != "Nom" {
		pairs["Case"] = udCase
	}
	if number != "" {
		pairs["Number"] = number
	}
	if mood != "" {
		pairs["Mood"] = mood
	}
	if tense != "" {
		pairs["Tense"] = tense
	}
	if person != "" {
		pairs["Person"] = person
	}
	if len(pairs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	out := make([]byte, 0, 32)
	for i, k := range keys {
		if i > 0 {
			out = append(out, '|')
		}
		out = append(out, k...)
		out = append(out, '=')
		out = append(out, pairs[k]...)
	}
	return string(out)
}

// CaseFromFeats returns the lowercase English case label for the
// Case= attribute in feats, or "" if FEATS has no Case= or the value
// is Nom. Mirrors the dict-layer caseFromFeats; centralised here so
// both halves of the FEATS round-trip live next to the table.
func CaseFromFeats(feats string) string {
	if feats == "" {
		return ""
	}
	start := 0
	for i := 0; i <= len(feats); i++ {
		if i == len(feats) || feats[i] == '|' {
			pair := feats[start:i]
			eq := -1
			for k := 0; k < len(pair); k++ {
				if pair[k] == '=' {
					eq = k
					break
				}
			}
			if eq >= 0 && pair[:eq] == "Case" {
				val := pair[eq+1:]
				if val == "Nom" {
					return ""
				}
				return UDCaseToLegacyLabel[val]
			}
			start = i + 1
		}
	}
	return ""
}
