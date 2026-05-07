package main

import "strings"

// kaikkiTagsToFeats projects a kaikki.org form's tag list (Wiktionary-derived)
// into a UD-FEATS string. Returns "" when no productive morphology can be
// inferred (lemma forms, untagged senses, idiomatic templates).
//
// kaikki tags are lowercase English words: ["illative","singular"],
// ["comparative"], ["first-person","present","indicative","singular"], etc.
// The vocabulary is small and stable across both Finnish and Estonian
// extracts — they share the same Wiktionary tag dictionary.
//
// Coverage philosophy mirrors cmd/importekilexdetails::ekilexMorphToFeats:
//   - Map every tag with a clear UD FEATS equivalent.
//   - Use upos to disambiguate where the same tag has different meanings
//     across categories (e.g. "passive" on a verb is Voice=Pass; on an
//     adjective participle it stays Voice=Pass too — same here).
//   - Polarity is NOT inferred — Polarity=Neg in FI/ET is a sentence-level
//     signal driven by the auxiliary "ei" particle, not a per-form lookup.
//     Adapter pipelines (omorfi, estnltk) emit it from full-sentence context;
//     dict layer correctly leaves it empty.
//   - Empty result is a fine signal — caller stores NULL, and FST/adapter
//     paths can still backfill at parse time.
//
// Reflexive pronouns are recognized lexically by the caller via
// kaikkiReflexiveLemmas; this function handles only tag-driven attributes.
func kaikkiTagsToFeats(tags []string, upos string) string {
	if len(tags) == 0 {
		return ""
	}
	pairs := make(map[string]string, 8)
	for _, raw := range tags {
		t := strings.ToLower(strings.TrimSpace(raw))
		if t == "" {
			continue
		}
		if c, ok := kaikkiCaseTags[t]; ok {
			pairs["Case"] = c
			continue
		}
		if n, ok := kaikkiNumberTags[t]; ok {
			pairs["Number"] = n
			continue
		}
		if p, ok := kaikkiPersonTags[t]; ok {
			pairs["Person"] = p
			continue
		}
		if v, ok := kaikkiTenseTags[t]; ok {
			pairs["Tense"] = v
			continue
		}
		if v, ok := kaikkiMoodTags[t]; ok {
			pairs["Mood"] = v
			continue
		}
		if v, ok := kaikkiVoiceTags[t]; ok {
			pairs["Voice"] = v
			continue
		}
		if v, ok := kaikkiVerbFormTags[t]; ok {
			pairs["VerbForm"] = v
			continue
		}
		if v, ok := kaikkiDegreeTags[t]; ok && (upos == "ADJ" || upos == "ADV") {
			pairs["Degree"] = v
			continue
		}
		if v, ok := kaikkiPronTypeTags[t]; ok && upos == "PRON" {
			pairs["PronType"] = v
			continue
		}
	}
	return joinSortedFeats(pairs)
}

// joinSortedFeats turns a {attr:value} map into a UD-canonical pipe-separated
// string with attributes in alphabetical order ("Case=Ine|Number=Sing").
func joinSortedFeats(pairs map[string]string) string {
	if len(pairs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	// Cheap insertion sort — len is at most ~8.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(pairs[k])
	}
	return b.String()
}

// kaikkiCaseTags covers the Finnish 14-case set + Estonian additions
// (terminative, comitative, abessive). Aditive folds to Ill (UD-EDT
// convention; matches cmd/importekilexdetails::ekilexMorphToFeats).
var kaikkiCaseTags = map[string]string{
	"nominative":   "Nom",
	"genitive":     "Gen",
	"accusative":   "Acc",
	"partitive":    "Par",
	"inessive":     "Ine",
	"elative":      "Ela",
	"illative":     "Ill",
	"adessive":     "Ade",
	"ablative":     "Abl",
	"allative":     "All",
	"essive":       "Ess",
	"translative":  "Tra",
	"abessive":     "Abe",
	"comitative":   "Com",
	"instructive":  "Ins",
	"terminative":  "Ter",
	"aditive":      "Ill",
}

var kaikkiNumberTags = map[string]string{
	"singular": "Sing",
	"plural":   "Plur",
}

var kaikkiPersonTags = map[string]string{
	"first-person":  "1",
	"second-person": "2",
	"third-person":  "3",
}

var kaikkiTenseTags = map[string]string{
	"present": "Pres",
	"past":    "Past",
}

var kaikkiMoodTags = map[string]string{
	"indicative":  "Ind",
	"conditional": "Cnd",
	"imperative":  "Imp",
	"potential":   "Pot",
}

var kaikkiVoiceTags = map[string]string{
	"active":  "Act",
	"passive": "Pass",
}

// kaikkiVerbFormTags covers non-finite verbal forms tagged in kaikki.
// "supine" and "infinitive" exist for both languages; participle naming
// is inconsistent across kaikki entries, so all four common spellings
// land on VerbForm=Part.
var kaikkiVerbFormTags = map[string]string{
	"infinitive":          "Inf",
	"participle":          "Part",
	"present-participle":  "Part",
	"past-participle":     "Part",
	"agent-participle":    "Part",
	"negative-participle": "Part",
	"supine":              "Sup",
	"connegative":         "Fin",
}

var kaikkiDegreeTags = map[string]string{
	"comparative": "Cmp",
	"superlative": "Sup",
	"positive":    "Pos",
}

// kaikkiPronTypeTags is restricted to PRON because words like "this" carry
// "demonstrative" elsewhere (DET) where UD's PronType doesn't apply the
// same way. The PRON gate in kaikkiTagsToFeats keeps it conservative.
var kaikkiPronTypeTags = map[string]string{
	"demonstrative":     "Dem",
	"interrogative":     "Int",
	"relative":          "Rel",
	"indefinite":        "Ind",
	"personal":          "Prs",
	"personal-pronoun":  "Prs",
	"reciprocal":        "Rcp",
}

// kaikkiReflexiveLemmas is the static set of FI/ET reflexive-pronoun
// lemmas. Reflex=Yes is lemma-level, not per-form-tag — kaikki doesn't
// emit a "reflexive" tag on individual forms, so we identify them by
// headword. Caller passes the lemma; if it matches, append Reflex=Yes
// to whatever the tags produced.
//
// Finnish: itse "self" + its case-form headwords sometimes registered
// separately. The lemma is "itse"; case-marked forms of "itse" inherit
// reflexivity through standard inflection.
//
// Estonian: enese / enda — both are dictionary lemmas of the same
// reflexive paradigm (one suppletive lemma family).
var kaikkiReflexiveLemmas = map[string]bool{
	"itse":  true, // FI
	"enese": true, // ET (genitive-stem lemma)
	"enda":  true, // ET (genitive-stem variant lemma)
}

// withReflex appends Reflex=Yes to feats if lemma is a reflexive pronoun.
// Keeps alphabetical attribute order. Cheap because feats are short.
func withReflex(feats, lemma, upos string) string {
	if upos != "PRON" {
		return feats
	}
	if !kaikkiReflexiveLemmas[strings.ToLower(lemma)] {
		return feats
	}
	if feats == "" {
		return "Reflex=Yes"
	}
	parts := strings.Split(feats, "|")
	parts = append(parts, "Reflex=Yes")
	for i := 1; i < len(parts); i++ {
		for j := i; j > 0 && parts[j-1] > parts[j]; j-- {
			parts[j-1], parts[j] = parts[j], parts[j-1]
		}
	}
	return strings.Join(parts, "|")
}
