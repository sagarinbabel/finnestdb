package main

import (
	"regexp"
	"strings"
)

// structuralGlossPatterns matches kaikki/Wiktionary "form-of" definitions
// that restate metadata the learner already sees on the card
// (case/number/voice/person) instead of giving a meaning. A gloss
// like "partitive singular of vuosi" tells you the form's morphology,
// not what `vuotta` means. The learner-facing surface for `vuotta`
// should resolve to `vuosi → "year"`, never "partitive singular of vuosi".
//
// The pattern set was seeded from
// yle_subs/build_surface_target_decks.py:_STRUCTURAL_GLOSS_PATTERNS,
// where the same regexes drove a pre-build audit that fails the deck
// build when any structural gloss leaks into the override file.
// Catching them at kaikki ingest closes the leak one layer earlier -
// every downstream consumer (deck builder, API, web UI) gets the
// meaning gloss by default.
//
// The patterns are intentionally anchor-strict (^...): a gloss
// containing the phrase "inflection of" in the middle of a real
// definition ("a regional inflection of the verb 'to be'") must not
// be filtered out.
//
// Note on Unicode: Go's RE2 \w is ASCII-only - a naïve port of the
// Python original would miss "partitive singular of ääni" or
// "genitive singular of õun" because `ä`/`õ` are not in \w. The
// case-of-headword pattern below uses \pL (Unicode letter class)
// instead. The other patterns end in \b, where the boundary fires
// between the ASCII letter `f` of "of" and the following space -
// regardless of what letter starts the headword that follows - so
// they handle umlauts correctly already.
var structuralGlossPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^inflection of\b`),
	regexp.MustCompile(`(?i)^(first|second|third)[-\s]?person\b.*\b(of|indicative)\b`),
	regexp.MustCompile(`(?i)^(past|present|future)\s+(active|passive)?\s*participle\s+of\b`),
	regexp.MustCompile(`(?i)^(alternative|obsolete|archaic|dated|colloquial|informal|formal)\s+(form|spelling)\s+of\b`),
	regexp.MustCompile(`(?i)^(partitive|genitive|illative|inessive|elative|adessive|allative|ablative|essive|translative|nominative|accusative|abessive|comitative|instructive)\s+(singular|plural)?\s*of\s+\pL`),
	regexp.MustCompile(`(?i)^synonym of\b`),
	regexp.MustCompile(`(?i)^(form|conjugation|declension)\s+of\b`),
	regexp.MustCompile(`(?i)^plural\s+of\b`),
	regexp.MustCompile(`(?i)^singular\s+of\b`),
}

// isStructuralGloss reports whether a Wiktionary gloss is a form-of /
// metadata restatement that should not be surfaced as the primary
// learner-facing translation. The matcher is whitespace-tolerant at
// the leading edge so kaikki's varying capitalisation doesn't matter.
//
// Empty strings return false: an empty gloss is filtered separately at
// the caller (`strings.TrimSpace(g) == ""` skip), and treating "" as
// structural would mask that distinct failure mode.
func isStructuralGloss(g string) bool {
	g = strings.TrimSpace(g)
	if g == "" {
		return false
	}
	for _, re := range structuralGlossPatterns {
		if re.MatchString(g) {
			return true
		}
	}
	return false
}

// pickPrimaryGloss returns the first gloss across all senses that is
// NOT a structural form-of restatement, falling back to the first
// gloss overall when every reading is structural. The fallback keeps
// "form-of-only" entries (legitimate Wiktionary content for purely
// inflected headwords) from blanking the `lemmas.gloss` cache; the
// translations table filter will still drop them from the per-sense
// rows so they never beat a real definition at lookup time.
//
// `senses` is the same shape as kaikkiEntry.Senses; we pass an
// interface-free slice signature so the helper can be exercised in
// tests without constructing a full kaikkiEntry.
func pickPrimaryGloss(senses [][]string) string {
	fallback := ""
	for _, glosses := range senses {
		for _, g := range glosses {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}
			if fallback == "" {
				fallback = g
			}
			if !isStructuralGloss(g) {
				return g
			}
		}
	}
	return fallback
}
