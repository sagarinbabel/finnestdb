package store

import (
	"regexp"
	"strings"
	"unicode"
)

// MaxDerivedTitleLen is the hard cap on a derived title, matching the
// deck-save modal's suggested-title UX (see CreateDeckRequest.Title).
const MaxDerivedTitleLen = 60

// reURL matches a bare URL, used to detect URL-only input.
var reURL = regexp.MustCompile(`^https?://\S+$`)

// reMarkdownPrefix strips a leading markdown heading/list/quote marker
// ("# ", "## ", "- ", "* ", "> ") from the start of a line.
var reMarkdownPrefix = regexp.MustCompile(`^(#{1,6}\s+|[-*+]\s+|>\s+)`)

// reWhitespace collapses any run of whitespace (including newlines) to a
// single space.
var reWhitespace = regexp.MustCompile(`\s+`)

// reSentenceEnd matches a real sentence-ending punctuation mark: one of . ! ?
// (optionally followed by a closing quote) followed by end-of-string or
// whitespace. Requiring a following space/EOS (rather than matching any
// ".!?" byte) avoids false "sentence ends" inside URLs ("example.com"),
// abbreviations, and decimals ("3.14"), where the punctuation is immediately
// followed by another non-space character.
var reSentenceEnd = regexp.MustCompile(`[.!?]["'”’]?(\s|$)`)

// DefaultTitleForLang returns the language-appropriate fallback title used
// when source text has no usable content to derive a title from. UI copy is
// English regardless of the learner's target language (see CONTEXT.md), so
// this names the language rather than switching UI language.
func DefaultTitleForLang(lang string) string {
	if lang == "ET" {
		return "Untitled Estonian text"
	}
	return "Untitled Finnish text"
}

// DeriveTitle produces a short, human-readable display title from pasted
// source text: the first clause or sentence, cleaned of surrounding
// whitespace/quotes/markdown artifacts, cut at the sentence end or clause
// boundary (comma/semicolon/colon) closest under MaxDerivedTitleLen. An
// ellipsis is appended only when the result was truncated mid-clause.
//
// Degenerate input (a single word, a bare URL, digits-only) falls back to the
// first 2-4 words instead of an arbitrary character cut. Empty or
// whitespace-only input falls back to a language-appropriate default title.
//
// This is deterministic text processing, not an LLM call: the title only
// needs to look reasonable at a glance, and a fixed rule set is auditable and
// free, unlike an LLM call for every pasted paragraph.
func DeriveTitle(sourceText, lang string) string {
	cleaned := cleanTitleSource(sourceText)
	if cleaned == "" {
		return DefaultTitleForLang(lang)
	}

	if isDegenerateTitleSource(cleaned) {
		return truncateToWords(cleaned, 4)
	}

	return truncateAtClause(cleaned)
}

// cleanTitleSource trims whitespace, strips a leading markdown marker, strips
// surrounding quote characters, and collapses internal whitespace (including
// newlines) to single spaces so multi-line pastes read as one line.
func cleanTitleSource(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}

	// Only the first line drives the title; a pasted paragraph's later lines
	// aren't part of "the first clause/sentence".
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	s = reMarkdownPrefix.ReplaceAllString(s, "")
	s = reWhitespace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'“”«»*_`)
	return strings.TrimSpace(s)
}

// isDegenerateTitleSource reports whether text has no real clause structure
// to cut at: a single word, a bare URL, or digits-only content.
func isDegenerateTitleSource(text string) bool {
	if reURL.MatchString(text) {
		return true
	}
	if isDigitsOnly(text) {
		return true
	}
	return len(strings.Fields(text)) <= 1
}

func isDigitsOnly(text string) bool {
	seenDigit := false
	for _, r := range text {
		switch {
		case unicode.IsDigit(r):
			seenDigit = true
		case unicode.IsSpace(r), r == '.', r == ',', r == '-', r == ':':
			// Punctuation commonly found in dates/numbers is not disqualifying.
		default:
			return false
		}
	}
	return seenDigit
}

// truncateToWords joins up to n whitespace-separated words from text, then
// applies the same hard character cap as truncateAtClause.
func truncateToWords(text string, n int) string {
	fields := strings.Fields(text)
	if len(fields) > n {
		fields = fields[:n]
	}
	joined := strings.Join(fields, " ")
	return hardTruncate(joined)
}

// truncateAtClause returns the first clause/sentence of text, cut at the
// closest sentence-ending (. ! ?) or clause-boundary (, ; :) punctuation at or
// under MaxDerivedTitleLen. Falls back to a word-boundary hard cut with an
// ellipsis when no such boundary exists within the limit.
func truncateAtClause(text string) string {
	if len([]rune(text)) <= MaxDerivedTitleLen {
		// Still prefer stopping at the first sentence end even when the whole
		// text already fits, so "One sentence. Another sentence." becomes
		// "One sentence." rather than the concatenation of both.
		if loc := reSentenceEnd.FindStringIndex(text); loc != nil {
			return strings.TrimSpace(text[:loc[0]+1])
		}
		return text
	}

	runes := []rune(text)
	window := string(runes[:MaxDerivedTitleLen])

	// Prefer the first sentence end within the window: the result is a strict
	// prefix of the window (so it already fits) and needs no ellipsis.
	if loc := reSentenceEnd.FindStringIndex(window); loc != nil {
		return strings.TrimSpace(window[:loc[0]+1])
	}

	// Every other path below appends an ellipsis, so cut from a window that's
	// one rune shorter to keep the final result, including "…", within
	// MaxDerivedTitleLen.
	ellipsisWindow := string(runes[:MaxDerivedTitleLen-1])

	// Next, a clause boundary (comma/semicolon/colon) within the window.
	if end := lastBoundary(ellipsisWindow, ",;:"); end >= 0 {
		return strings.TrimSpace(ellipsisWindow[:end]) + "…"
	}
	// No punctuation boundary at all: cut at the last full word within the
	// limit so we don't split a word in half.
	if sp := strings.LastIndexByte(ellipsisWindow, ' '); sp > 0 {
		return strings.TrimSpace(ellipsisWindow[:sp]) + "…"
	}
	// Single very long "word" with no space: hard character cut.
	return strings.TrimSpace(ellipsisWindow) + "…"
}

// lastBoundary returns the byte index of the last rune in text that is one of
// chars, or -1 if none is present.
func lastBoundary(text, chars string) int {
	return strings.LastIndexAny(text, chars)
}

// hardTruncate enforces MaxDerivedTitleLen as an absolute character cap,
// rune-safe, appending an ellipsis when truncation occurred.
func hardTruncate(text string) string {
	runes := []rune(text)
	if len(runes) <= MaxDerivedTitleLen {
		return text
	}
	return strings.TrimSpace(string(runes[:MaxDerivedTitleLen-1])) + "…"
}
