// Package udfeats centralises the FI/ET grammar_label ↔ UD Case mapping
// and the FEATS-string composer used by every FST analyzer
// (voikkomap, giellaltmap) and by the dict layer's runtime composer.
//
// Two callers, one source of truth:
//
//   - voikkomap.Analysis and giellaltmap.Analysis fill their morphological
//     fields and call udfeats.ComposeMap once at Parse() time, persisting
//     the FEATS string in their Feats field. Generated table JSONs are then
//     self-describing.
//   - internal/store/dict.go calls ComposeMap for analyses that arrive
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

// ComposeMap builds a UD-canonical FEATS string from an arbitrary map of
// attribute→value pairs. Empty values are skipped. Keys are sorted
// alphabetically per UD convention.
func ComposeMap(pairs map[string]string) string {
	n := 0
	for _, v := range pairs {
		if v != "" {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	keys := make([]string, 0, n)
	for k, v := range pairs {
		if v != "" {
			keys = append(keys, k)
		}
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	out := make([]byte, 0, 64)
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

// AppendSortedValue inserts value into a comma-separated sorted string
// (UD convention for multi-valued features like Clitic=Han,Kin). Returns
// value unchanged if existing is empty. Deduplicates.
func AppendSortedValue(existing, value string) string {
	if existing == "" {
		return value
	}
	vals := splitComma(existing)
	pos := len(vals)
	for i, v := range vals {
		if value == v {
			return existing
		}
		if value < v {
			pos = i
			break
		}
	}
	vals = append(vals, "")
	copy(vals[pos+1:], vals[pos:])
	vals[pos] = value
	n := 0
	for _, v := range vals {
		if n > 0 {
			n++ // comma
		}
		n += len(v)
	}
	out := make([]byte, 0, n)
	for i, v := range vals {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, v...)
	}
	return string(out)
}

func splitComma(s string) []string {
	n := 1
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			n++
		}
	}
	out := make([]string, 0, n)
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
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

// maInfinitiveEndings maps a UD Case= value to the surface suffix pairs
// (back/front harmony) that signal a Finnish MA-infinitive (the
// "third infinitive") in that case. A noun-cousin like `tarjoama` will
// emit an FST reading with Case=Ill|Person=3|Number=Sing for the
// surface `tarjoamaan` even though the form is the MA-infinitive
// illative of `tarjota`. NormalizeMaInfinitive detects that signature
// from the surface+case agreement and rewrites the FEATS.
//
// The five cases here are the inflectional slots the MA-infinitive
// productively occupies: illative ("for V-ing"), inessive ("while
// V-ing"), elative ("from V-ing"), adessive ("by V-ing"), abessive
// ("without V-ing"). Translative MA-infinitives don't exist in modern
// Finnish, so we deliberately don't list it.
var maInfinitiveEndings = map[string][2]string{
	"Ill": {"maan", "mään"},
	"Ine": {"massa", "mässä"},
	"Ela": {"masta", "mästä"},
	"Ade": {"malla", "mällä"},
	"Abe": {"matta", "mättä"},
}

// IsMaInfinitiveSurface reports whether a Finnish surface matches the
// MA-infinitive case-marked suffix pattern (maan/mään/massa/mässä/
// masta/mästä/malla/mällä/matta/mättä). Used by the dict-layer
// ranker to demote the analyzer noun-cousin trap (e.g. `lähtemään`
// emitted as `lähtemä`/NOUN/Case=Ill instead of the verb's
// MA-infinitive illative) against any candidate VERB reading.
//
// This is a surface-only check: it does NOT confirm a verb reading
// exists. Callers compose this with POS/feats checks on each
// candidate to decide.
func IsMaInfinitiveSurface(surface string) bool {
	if surface == "" {
		return false
	}
	lower := toLowerASCII(surface)
	for _, pair := range maInfinitiveEndings {
		if hasSuffix(lower, pair[0]) || hasSuffix(lower, pair[1]) {
			return true
		}
	}
	return false
}

// NormalizeMaInfinitive rewrites FEATS when (pos, surface, feats) match
// the analyzer-noun-cousin trap for Finnish MA-infinitives.
//
// Concretely: if pos is VERB, surface ends in -maan/-mään (etc.) for a
// case slot the MA-infinitive can occupy, the Case= already matches,
// and the FEATS does NOT already declare VerbForm=Inf|InfForm=Ma, this
// function strips spurious Person=3 and Number=Sing (the noun-cousin's
// signature) and adds VerbForm=Inf|InfForm=Ma. The returned FEATS is
// canonicalised via ComposeMap.
//
// If pos is not VERB, the surface doesn't match a MA-suffix for its
// declared case, or the FEATS is already correctly marked, the input
// is returned unchanged. This is a no-op for the common case so it's
// safe to call on every analysis at lookup time.
//
// Seeded from yle_subs/build_surface_target_decks.py's
// probable_ma_infinitive_case + normalize_feats_for_surface, where the
// downstream deck builder applied the same fix to live cards before
// shipping them. Moving the rule into the parser closes it for every
// consumer.
func NormalizeMaInfinitive(surface, pos, feats string) string {
	if pos != "VERB" || feats == "" {
		return feats
	}
	pairs := parseFeats(feats)
	if pairs["VerbForm"] == "Inf" && pairs["InfForm"] == "Ma" {
		return feats
	}
	caseVal, ok := pairs["Case"]
	if !ok || caseVal == "" {
		return feats
	}
	suffixes, ok := maInfinitiveEndings[caseVal]
	if !ok {
		return feats
	}
	lower := toLowerASCII(surface)
	if !hasSuffix(lower, suffixes[0]) && !hasSuffix(lower, suffixes[1]) {
		return feats
	}
	// Strip the noun-cousin signature (Person=3, Number=Sing) and assert
	// the MA-infinitive markers. Other features (Clitic, etc.) are
	// preserved. Voice=Act is added when no Voice attribute is present
	// — MA-infinitives default to active voice in UD; passive MA
	// forms would have arrived with Voice=Pass already.
	if pairs["Person"] == "3" {
		delete(pairs, "Person")
	}
	if pairs["Number"] == "Sing" {
		delete(pairs, "Number")
	}
	pairs["VerbForm"] = "Inf"
	pairs["InfForm"] = "Ma"
	if pairs["Voice"] == "" {
		pairs["Voice"] = "Act"
	}
	return ComposeMap(pairs)
}

// parseFeats splits a UD FEATS string back into an attribute map. The
// inverse of ComposeMap. Empty input returns an empty (non-nil) map so
// callers can write into it.
func parseFeats(feats string) map[string]string {
	out := make(map[string]string, 8)
	if feats == "" {
		return out
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
			if eq > 0 {
				out[pair[:eq]] = pair[eq+1:]
			}
			start = i + 1
		}
	}
	return out
}

// hasSuffix reports whether s ends in suffix. Local helper to avoid an
// import-strings dependency in a hot path (Lemmatize calls this for
// every verb analysis).
func hasSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

// toLowerASCII downcases ASCII letters in-place. Finnish MA-suffixes
// contain only ASCII a/m/n/s/t/ä/ö; for ä/ö (multi-byte UTF-8) the
// uppercase bytes (Ä=0xC3 0x84, Ö=0xC3 0x96) differ from lowercase
// (ä=0xC3 0xA4, ö=0xC3 0xB6) only in the trailing byte, which we
// handle by hand. Keeps the function dependency-free.
func toLowerASCII(s string) string {
	needsCopy := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			needsCopy = true
			break
		}
		// Capital Ä (0xC3 0x84) and Ö (0xC3 0x96) are the only
		// non-ASCII uppercase chars that appear in MA-infinitive
		// surfaces. Detect via the C3 prefix.
		if c == 0xC3 && i+1 < len(s) {
			next := s[i+1]
			if next == 0x84 || next == 0x96 {
				needsCopy = true
				break
			}
		}
	}
	if !needsCopy {
		return s
	}
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
			continue
		}
		if c == 0xC3 && i+1 < len(b) {
			switch b[i+1] {
			case 0x84:
				b[i+1] = 0xA4
			case 0x96:
				b[i+1] = 0xB6
			}
		}
	}
	return string(b)
}
