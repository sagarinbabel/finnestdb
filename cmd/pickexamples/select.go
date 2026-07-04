package main

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Quality heuristics for "beautiful evocative" starter-deck examples. These are
// deterministic and documented as named constants so the owner (curator of last
// resort) can see exactly why a sentence was chosen or rejected. Every value
// here is a product judgement, not a tuned parameter.
const (
	// minWords / maxWords bound sentence length. Below the floor a sentence is
	// too fragmentary to teach usage in context; above the ceiling it is too
	// dense for a starter-level learner reading a flashcard.
	minWords = 4
	maxWords = 14

	// allCapsMinLen is the shortest token that counts as a shouted / stylized
	// ALL-CAPS artifact. Two-letter tokens (e.g. an acronym or Roman "II") are
	// too short to reject a whole sentence over.
	allCapsMinLen = 3

	// rareRankFallback is the frequency rank assigned to any word absent from
	// the OpenSubtitles baseline. It sits past the 50k list so unknown words
	// pull a sentence's readability score toward "harder", matching the
	// intuition that an out-of-list word is rare for a beginner.
	rareRankFallback = 60000

	// concreteLenTarget is the character length a "concrete, plain" starter
	// sentence gravitates toward. The final tie-break prefers sentences whose
	// length is closest to this, breaking ties toward the shorter one. It is a
	// soft nudge, applied only after the readability score, never a hard gate.
	concreteLenTarget = 45

	// minInBaselineRate is the fraction of a sentence's words that must appear
	// in the target-language frequency baseline for it to count as being in
	// that language. The corpora contain occasional foreign-language sentences
	// (e.g. an English line whose "me"/"on" matches an Estonian function form);
	// a genuine target-language sentence has most of its everyday words in the
	// 50k baseline, while a foreign one has almost none. 0.4 is deliberately
	// lenient so rich native vocabulary (much of it past the 50k list) is not
	// mistaken for foreign text.
	minInBaselineRate = 0.4
)

var (
	digitRe = regexp.MustCompile(`[0-9]`)
	urlRe   = regexp.MustCompile(`(?i)https?://|www\.|\.com|\.net|\.org`)
	// speakerColonRe catches subtitle speaker labels like "MIES:" or "Anna:"
	// at the start of a sentence — a transcript artifact, not natural prose.
	speakerColonRe = regexp.MustCompile(`^\s*\p{L}[\p{L}\s]*:`)
	// midWordCapRe catches OCR / subtitle noise where an uppercase letter sits
	// inside a word after a lowercase one ("mItä", "aIka", "vaI"). Real Finnish
	// and Estonian words are not camelCased, so this reliably flags garbled
	// tokens (and the equally garbled parser analyses that produced them).
	midWordCapRe = regexp.MustCompile(`\p{Ll}\p{Lu}`)
	// dialogueJoinRe catches subtitle line joins where two lines merged into one
	// "sentence" ("...hautajaisista.- Sinulla on..."): a terminal mark
	// immediately followed by a dialogue dash.
	dialogueJoinRe = regexp.MustCompile(`[.!?]\s*[-–—]`)
)

// terminalPunct are the sentence-ending marks a complete sentence may end on.
const terminalPunct = ".!?…"

// Candidate is one corpus sentence proposed as an example for a lemma, carrying
// the exact inflected surface form of the lemma that appears in it.
type Candidate struct {
	SentenceID   int64
	Text         string
	Form         string // the inflected surface form of the target lemma
	FormCount    int64  // corpus frequency of that form (higher = more canonical)
	SourceCorpus string
}

// scored pairs a candidate with its computed selection score. Higher score is
// better. A rejected candidate is not scored.
type scored struct {
	cand  Candidate
	score float64
}

// acceptable applies the hard "is this even a usable starter sentence" gates.
// It returns false for anything that is not a clean, complete, readable
// sentence containing the target form. freqRanks is the target-language
// baseline used for the foreign-language gate; a nil map disables that one gate
// (all other gates still apply). The reason string is for logging/debug.
func acceptable(c Candidate, freqRanks map[string]int) (bool, string) {
	text := strings.TrimSpace(c.Text)
	if text == "" {
		return false, "empty"
	}

	// Subtitle artifact: leading dash (dialogue dash). Checked before the
	// capitalization gate so the more specific reason is reported.
	if strings.HasPrefix(text, "-") || strings.HasPrefix(text, "–") || strings.HasPrefix(text, "—") {
		return false, "leading-dash"
	}

	// Subtitle artifact: speaker label ("NAME: ...").
	if speakerColonRe.MatchString(text) {
		return false, "speaker-colon"
	}

	// Complete sentence: capitalized start.
	first, _ := utf8.DecodeRuneInString(text)
	if !unicode.IsUpper(first) && !unicode.IsTitle(first) {
		return false, "not-capitalized"
	}

	// Complete sentence: terminal punctuation.
	last, _ := utf8.DecodeLastRuneInString(text)
	if !strings.ContainsRune(terminalPunct, last) {
		return false, "no-terminal-punct"
	}

	// Quote fragment: an unbalanced quote mark means we likely captured half
	// of a quoted exchange.
	if hasUnbalancedQuote(text) {
		return false, "unbalanced-quote"
	}

	// No digits or URLs — both read as noise on a vocabulary card.
	if digitRe.MatchString(text) {
		return false, "has-digit"
	}
	if urlRe.MatchString(text) {
		return false, "has-url"
	}

	// Subtitle line join: "...sentence.- Next line...". Two merged dialogue
	// lines are not one natural sentence.
	if dialogueJoinRe.MatchString(text) {
		return false, "dialogue-join"
	}

	// OCR / subtitle garble: a mid-word capital ("mItä", "aIka"). Such tokens
	// are noise, and the parser analysis that mapped them to a lemma is wrong,
	// so any sentence carrying one is untrustworthy as an example.
	if midWordCapRe.MatchString(text) {
		return false, "mid-word-cap"
	}

	// The target form itself must be a plausible word, not a garbled parser
	// analysis ("einsä"/"eimme" for the lemma "en"): reject a mixed-interior-
	// case form outright.
	if midWordCapRe.MatchString(c.Form) {
		return false, "garbled-form"
	}

	words := strings.Fields(text)
	if len(words) < minWords || len(words) > maxWords {
		return false, "word-count"
	}

	// No ALL-CAPS token (shouting / stylized headings).
	for _, w := range words {
		if isAllCaps(w) {
			return false, "all-caps"
		}
	}

	// The target form must actually appear as a whole word.
	if formTokenIndex(words, c.Form) < 0 {
		return false, "form-absent"
	}

	// Foreign-language guard: a target-language sentence has most of its
	// everyday words in the baseline; a foreign line (whose short function
	// word happened to match the target form) has almost none.
	if freqRanks != nil && baselineRate(words, freqRanks) < minInBaselineRate {
		return false, "foreign-language"
	}
	return true, ""
}

// baselineRate is the fraction of a sentence's alphabetic words present in the
// target-language frequency baseline. Used only as a coarse foreign-language
// detector, so it is intentionally simple.
func baselineRate(words []string, freqRanks map[string]int) float64 {
	var in, total float64
	for _, w := range words {
		key := strings.ToLower(strings.TrimFunc(w, isTrimPunct))
		if key == "" {
			continue
		}
		total++
		if _, ok := freqRanks[key]; ok {
			in++
		}
	}
	if total == 0 {
		return 0
	}
	return in / total
}

// score ranks an acceptable candidate. Two signals, in priority order:
//  1. the target form is NOT sentence-initial (a non-initial form shows real
//     inflection in running context, not just a dictionary headword);
//  2. the other words are high-frequency (readable at a beginner's level),
//     scored as the negative mean OpenSubtitles rank of the non-target words.
//
// freqRanks maps lowercased form -> 1-based rank (rank 1 = most common); a nil
// map disables the readability signal (every sentence scores equally on it).
func score(c Candidate, freqRanks map[string]int) float64 {
	words := strings.Fields(c.Text)
	formIx := formTokenIndex(words, c.Form)

	var s float64

	// (1) Reward a non-initial target form. Weighted high so it dominates the
	// readability nudge: a sentence that actually demonstrates inflection in
	// context beats a marginally more common-worded one that opens on the form.
	if formIx > 0 {
		s += 1000
	}

	// (2) Readability: lower mean rank of the *other* words is better. We add
	// the negated, scaled mean rank so more common surrounding vocabulary
	// scores higher. Missing words fall back to rareRankFallback.
	var sum, n float64
	for i, w := range words {
		if i == formIx {
			continue
		}
		key := strings.ToLower(strings.TrimFunc(w, isTrimPunct))
		if key == "" {
			continue
		}
		rank := rareRankFallback
		if r, ok := freqRanks[key]; ok {
			rank = r
		}
		sum += float64(rank)
		n++
	}
	if n > 0 {
		meanRank := sum / n
		// Scale so readability contributes on the order of a few hundred
		// points at most — below the non-initial reward, above the tie-break.
		s += 500 * (1 - meanRank/rareRankFallback)
	}

	// Tie-break nudge toward concrete/shorter: closeness to the target length.
	lenPenalty := float64(len(c.Text) - concreteLenTarget)
	if lenPenalty < 0 {
		lenPenalty = -lenPenalty
	}
	s -= lenPenalty / 100

	return s
}

// pickBest scores all candidates, drops the unacceptable ones, and returns up
// to want best sentences, highest score first. Ties break deterministically by
// the more canonical (higher-count) form, then by sentence id, so a rerun on
// the same corpus yields the same artifact.
func pickBest(cands []Candidate, freqRanks map[string]int, want int) []Candidate {
	var pool []scored
	for _, c := range cands {
		if ok, _ := acceptable(c, freqRanks); !ok {
			continue
		}
		pool = append(pool, scored{cand: c, score: score(c, freqRanks)})
	}
	// Deterministic sort: score desc, then form-count desc, then id asc.
	sortScored(pool)

	// Avoid two near-identical sentences: dedup on normalized text.
	seen := make(map[string]struct{})
	var out []Candidate
	for _, sc := range pool {
		norm := normText(sc.cand.Text)
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, sc.cand)
		if len(out) >= want {
			break
		}
	}
	return out
}

// sortScored orders best-first by score, breaking ties by the more canonical
// (higher-count) form and then by sentence id, so a rerun on the same corpus
// yields the same artifact.
func sortScored(pool []scored) {
	sort.Slice(pool, func(i, j int) bool {
		a, b := pool[i], pool[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.cand.FormCount != b.cand.FormCount {
			return a.cand.FormCount > b.cand.FormCount
		}
		return a.cand.SentenceID < b.cand.SentenceID
	})
}

// formTokenIndex returns the 0-based index of form among words (whole-word,
// case-insensitive, punctuation-trimmed), or -1.
func formTokenIndex(words []string, form string) int {
	target := strings.ToLower(strings.TrimFunc(form, isTrimPunct))
	if target == "" {
		return -1
	}
	for i, w := range words {
		if strings.ToLower(strings.TrimFunc(w, isTrimPunct)) == target {
			return i
		}
	}
	return -1
}

func isAllCaps(w string) bool {
	trimmed := strings.TrimFunc(w, isTrimPunct)
	letters := 0
	for _, r := range trimmed {
		if unicode.IsLetter(r) {
			letters++
			if !unicode.IsUpper(r) && !unicode.IsTitle(r) {
				return false
			}
		}
	}
	return letters >= allCapsMinLen
}

func hasUnbalancedQuote(text string) bool {
	count := 0
	for _, r := range text {
		switch r {
		case '"', '\'', '«', '»', '“', '”', '‘', '’':
			count++
		}
	}
	return count%2 != 0
}

func isTrimPunct(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// normText collapses whitespace and lowercases so near-identical sentences
// (differing only in spacing or leading capitalization) dedup to one pick.
func normText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}
