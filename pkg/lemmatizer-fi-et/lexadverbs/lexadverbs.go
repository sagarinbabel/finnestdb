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
var fiOverlay = map[string]Analysis{
	"asiaan": {
		Lemma:        "asia",
		UPOS:         "NOUN",
		GrammarLabel: "illative",
		Number:       "Sing",
		Feats:        "Case=Ill|Number=Sing",
	},
	"enemmän": {
		Lemma:    "paljon",
		UPOS:     "ADV",
		Degree:   "Cmp",
		Feats:    "Degree=Cmp",
	},
	"kotona": {
		Lemma:        "koti",
		UPOS:         "NOUN",
		GrammarLabel: "essive",
		Number:       "Sing",
		Feats:        "Case=Ess|Number=Sing",
	},
	"peräisin": {
		Lemma: "peräisin",
		UPOS:  "ADV",
	},
	"perillä": {
		Lemma: "perillä",
		UPOS:  "ADV",
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
	"yleensä": {
		Lemma: "yleensä",
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
	if surface == "" {
		return nil, false
	}
	a, ok := fiOverlay[toLowerASCII(surface)]
	if !ok {
		return nil, false
	}
	out := []Analysis{a}
	return out, true
}

// HasFI reports whether a Finnish surface has an overlay entry,
// without allocating the analysis slice. Useful for callers that
// only need to know "should I bypass the FST."
func HasFI(surface string) bool {
	if surface == "" {
		return false
	}
	_, ok := fiOverlay[toLowerASCII(surface)]
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
