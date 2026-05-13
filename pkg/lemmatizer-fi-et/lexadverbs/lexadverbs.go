// Package lexadverbs is a small overlay of lexicalised Finnish and
// Estonian forms whose productive morphological analysis misleads the
// learner. When one of these surfaces is looked up, the lemmatizer
// returns the curated analysis here instead of any FST reading.
//
// What belongs here:
//
//   - Adverbs that the FST decomposes as a productive case form whose
//     lemma is etymologically related but pedagogically wrong.
//     `tuskin` is a frozen instructive plural of `tuska` (pain) — it
//     means "barely / hardly" and has nothing to do with pain in
//     modern Finnish. `varsin` is an adverb "quite / rather", not the
//     adessive of `varsi` (stalk). `enemmän` is the comparative
//     adverb "more"; its lemma is `paljon`, not `paljo`.
//   - Lexicalised location/manner adverbs that always carry one
//     meaning regardless of the productive case ending — `perillä`
//     "at destination", `peräisin` "originally from", `tarpeeksi`
//     "enough".
//   - Surface forms whose FST output trips known analyser-trap lemmas
//     (`as`, `varsi`, `sisä-`, `ylä-`, `poli`) — the curated entry
//     ships the correct lemma even when the FST disagrees.
//
// What does NOT belong here:
//
//   - Productive inflections the FST already gets right. Don't shadow
//     the FST when it's accurate; this overlay is only for the cases
//     where the FST's first reading is the wrong one for the learner.
//   - Sentence-context-dependent disambiguations. Those go in the
//     dict layer's contextual gloss override path, not in a
//     surface→analysis lookup that fires for every occurrence.
//
// The overlay was seeded from yle_subs/build_surface_target_decks.py's
// LEXICAL_ANALYSES + PUHEKIELI_ANALYSES tables, where each entry
// already encodes a real-world bug the downstream deck builder had to
// patch with a manual analysis on live cards.
package lexadverbs

import "finnestdb/pkg/lemmatizer-fi-et/voikkomap"

// Analysis is re-exported so callers in the same module don't need a
// separate voikkomap import just to declare a return type.
type Analysis = voikkomap.Analysis

// fiOverlay maps a lowercased Finnish surface form to the curated
// analysis that should be returned in place of any FST reading.
//
// Note on Feats values: we keep Feats explicit (not via udfeats.ComposeMap)
// to make every entry self-describing and grep-friendly. Entries are
// listed alphabetically by surface; keep new ones in order.
//
// The etOverlay below mirrors this structure for Estonian. The two
// maps are independent (different bug catalogues, different
// pedagogical conventions) — keep them separate even when a surface
// shape coincidentally matches across languages.
var fiOverlay = map[string]Analysis{
	"asiaan": {
		Lemma:        "asia",
		UPOS:         "NOUN",
		GrammarLabel: "illative",
		Number:       "Sing",
		Feats:        "Case=Ill|Number=Sing",
	},
	"enemmän": {
		Lemma:  "paljon",
		UPOS:   "ADV",
		Degree: "Cmp",
		Feats:  "Degree=Cmp",
	},
	"kotona": {
		Lemma:        "koti",
		UPOS:         "NOUN",
		GrammarLabel: "essive",
		Number:       "Sing",
		Feats:        "Case=Ess|Number=Sing",
	},
	"muuta": {
		// Kaikki ships `muuta` keyed under the verb `muuttaa` (suspicious
		// per yle_subs SUSPICIOUS_SURFACE_LEMMAS). The dominant reading
		// is the partitive of the pronoun `muu` ("other"). Curate it.
		Lemma:        "muu",
		UPOS:         "PRON",
		GrammarLabel: "partitive",
		Number:       "Sing",
		PronType:     "Ind",
		Feats:        "Case=Par|Number=Sing|PronType=Ind",
	},
	"peräisin": {
		Lemma: "peräisin",
		UPOS:  "ADV",
	},
	"perillä": {
		Lemma: "perillä",
		UPOS:  "ADV",
	},
	"siitä": {
		// Suspicious (siitä, siittää) per yle_subs — the analyzer ranks
		// the verb `siittää` ("conceive") first. The dominant reading
		// is the demonstrative pronoun in the elative.
		Lemma:        "se",
		UPOS:         "PRON",
		GrammarLabel: "elative",
		Number:       "Sing",
		PronType:     "Dem",
		Feats:        "Case=Ela|Number=Sing|PronType=Dem",
	},
	"sisään": {
		Lemma: "sisään",
		UPOS:  "ADV",
	},
	"tarpeeksi": {
		Lemma: "tarpeeksi",
		UPOS:  "ADV",
	},
	"tuskin": {
		Lemma: "tuskin",
		UPOS:  "ADV",
	},
	"vahingossa": {
		Lemma: "vahingossa",
		UPOS:  "ADV",
	},
	"varsin": {
		Lemma: "varsin",
		UPOS:  "ADV",
	},
	"vuotta": {
		// Suspicious (vuotta, vuo) per yle_subs — the analyzer ranks a
		// rare noun `vuo` first. The dominant reading is the partitive
		// of `vuosi` ("year").
		Lemma:        "vuosi",
		UPOS:         "NOUN",
		GrammarLabel: "partitive",
		Number:       "Sing",
		Feats:        "Case=Par|Number=Sing",
	},
	"yleensä": {
		Lemma: "yleensä",
		UPOS:  "ADV",
	},
}

// etOverlay maps a lowercased Estonian surface form to the curated
// analysis. Seeded from a corpus sweep that identified 3,137 ET
// surfaces where an ADV reading exists but the parser's chosen
// analysis is a case-marked non-ADV (~38M aggregate occurrences in
// the working corpus). The 12 entries here are the highest-impact
// failures; the productive case reading for each is essentially
// never the intended meaning in modern Estonian prose.
//
// Closed-class adpositions (ADP), adverbs (ADV), particles, and pronouns
// sit side-by-side because the bug being patched is the same: the parser
// fires a productive case or noun reading on a high-frequency function
// word. We pick the most informative single tag and let the contextual
// gloss layer (TODO from yle_subs) handle finer disambiguation later.
var etOverlay = map[string]Analysis{
	"ei": {
		Lemma: "ei",
		UPOS:  "ADV",
	},
	"eest": {
		Lemma: "eest",
		UPOS:  "ADP",
	},
	"jaoks": {
		Lemma: "jaoks",
		UPOS:  "ADP",
	},
	"lihtsalt": {
		Lemma: "lihtsalt",
		UPOS:  "ADV",
	},
	"ma": {
		Lemma:  "ma",
		UPOS:   "PRON",
		Number: "Sing",
		Person: "1",
		Feats:  "Case=Nom|Number=Sing|Person=1|PronType=Prs",
	},
	"peale": {
		Lemma: "peale",
		UPOS:  "ADP",
	},
	"pärast": {
		Lemma: "pärast",
		UPOS:  "ADP",
	},
	"seal": {
		Lemma: "seal",
		UPOS:  "ADV",
	},
	"sisse": {
		Lemma: "sisse",
		UPOS:  "ADV",
	},
	"sina": {
		Lemma:  "sina",
		UPOS:   "PRON",
		Number: "Sing",
		Person: "2",
		Feats:  "Case=Nom|Number=Sing|Person=2|PronType=Prs",
	},
	"ta": {
		// Lowercase `ta` and sentence-initial `Ta` are the
		// high-frequency personal pronoun. The store layer deliberately
		// bypasses this overlay for exact all-caps `TA` so source
		// abbreviation entries can still be reached.
		Lemma: "tema",
		UPOS:  "PRON",
	},
	"taga": {
		Lemma: "taga",
		UPOS:  "ADP",
	},
	"tegelikult": {
		Lemma: "tegelikult",
		UPOS:  "ADV",
	},
	"veel": {
		Lemma: "veel",
		UPOS:  "ADV",
	},
	"välja": {
		Lemma: "välja",
		UPOS:  "ADV",
	},
}

// LookupFI returns the curated overlay analysis for a Finnish surface
// form. The first return is a single-element slice (cloned, so callers
// can mutate without affecting the shared table); ok is false when no
// overlay exists, in which case the caller falls through to the FST.
//
// Surface matching is case-folded against the keys, so "Tuskin" at
// sentence-initial position hits the same entry as "tuskin".
func LookupFI(surface string) ([]Analysis, bool) {
	return lookup(fiOverlay, surface)
}

// HasFI reports whether a Finnish surface has an overlay entry,
// without allocating the analysis slice. Useful for callers that
// only need to know "should I bypass the FST."
func HasFI(surface string) bool {
	return has(fiOverlay, surface)
}

// LookupET is the Estonian counterpart to LookupFI. Behaviour is
// identical (case-fold, single-element slice, cloned-by-construction).
// See etOverlay for the catalogued entries and the bug class.
func LookupET(surface string) ([]Analysis, bool) {
	return lookup(etOverlay, surface)
}

// HasET is the Estonian counterpart to HasFI.
func HasET(surface string) bool {
	return has(etOverlay, surface)
}

// lookup is the shared implementation. The single-element slice is
// freshly allocated on every call so a caller mutating it cannot
// corrupt the shared map's Analysis value (cloned-by-construction).
func lookup(table map[string]Analysis, surface string) ([]Analysis, bool) {
	if surface == "" {
		return nil, false
	}
	a, ok := table[toLowerASCII(surface)]
	if !ok {
		return nil, false
	}
	return []Analysis{a}, true
}

func has(table map[string]Analysis, surface string) bool {
	if surface == "" {
		return false
	}
	_, ok := table[toLowerASCII(surface)]
	return ok
}

// toLowerASCII downcases ASCII letters and the Finnish/Estonian umlaut
// pair Ä/Ö (and the Estonian Õ/Ü) without pulling in unicode/strings.
// The overlay table contains only lowercase keys, so the lookup must
// canonicalise the input on the same axis. Local copy of the same
// helper in udfeats to keep packages independent.
func toLowerASCII(s string) string {
	needsCopy := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			needsCopy = true
			break
		}
		if c == 0xC3 && i+1 < len(s) {
			next := s[i+1]
			if next == 0x84 || next == 0x96 || next == 0x95 || next == 0x9C {
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
			case 0x84: // Ä → ä
				b[i+1] = 0xA4
			case 0x96: // Ö → ö
				b[i+1] = 0xB6
			case 0x95: // Õ → õ
				b[i+1] = 0xB5
			case 0x9C: // Ü → ü
				b[i+1] = 0xBC
			}
		}
	}
	return string(b)
}
