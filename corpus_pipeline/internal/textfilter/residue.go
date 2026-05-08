package textfilter

import (
	"path/filepath"
	"strings"
	"unicode"
)

var resourceSkipNames = map[string]bool{
	"about":           true,
	"colophon":        true,
	"contents":        true,
	"copyright":       true,
	"cover":           true,
	"coverpage":       true,
	"halftitle":       true,
	"imprint":         true,
	"landmarks":       true,
	"legal":           true,
	"legalnotice":     true,
	"nav":             true,
	"navigation":      true,
	"pagelist":        true,
	"sisallys":        true,
	"tableofcontents": true,
	"title":           true,
	"titlepage":       true,
	"toc":             true,
}

var residueFragments = []string{
	"all rights reserved",
	"calibre",
	"copyright",
	"cover design",
	"e-isbn",
	"http://",
	"https://",
	"isbn",
	"www.",
	"©",
	"alkuteos",
	"jacket design",
	"julkaisija",
	"kaikki oikeudet",
	"kansi:",
	"kustantaja",
	"painettu",
	"painopaikka",
	"suomentanut",
	"tekijänoikeus",
}

var headingPrefixes = map[string]bool{
	"chapter":    true,
	"contents":   true,
	"epilogue":   true,
	"esipuhe":    true,
	"jalkisanat": true,
	"luku":       true,
	"osa":        true,
	"prologue":   true,
	"prologi":    true,
	"sisallys":   true,
}

// ShouldSkipEPUBResource returns true for EPUB HTML resources that are very
// likely navigation, cover, title-page, or other front-matter files rather than
// body prose.
func ShouldSkipEPUBResource(name string, raw []byte) bool {
	slashName := filepath.ToSlash(strings.ToLower(name))
	base := strings.TrimSuffix(filepath.Base(slashName), filepath.Ext(slashName))
	baseKey := compactForMatch(base)
	if resourceSkipNames[baseKey] || isNumberedResource(baseKey, "toc") ||
		isNumberedResource(baseKey, "nav") || isNumberedResource(baseKey, "cover") ||
		isNumberedResource(baseKey, "front") || isNumberedResource(baseKey, "frontmatter") ||
		isNumberedResource(baseKey, "fm") {
		return true
	}
	for _, segment := range strings.Split(slashName, "/") {
		key := compactForMatch(segment)
		if key == "frontmatter" || key == "front" || key == "prelims" || key == "preliminary" {
			return true
		}
	}
	if len(raw) == 0 {
		return false
	}
	head := lowerPrefix(raw, 65536)
	if strings.Contains(head, "<nav") &&
		(strings.Contains(head, "epub:type=\"toc\"") ||
			strings.Contains(head, "epub:type='toc'") ||
			strings.Contains(head, "role=\"doc-toc\"") ||
			strings.Contains(head, "role='doc-toc'") ||
			strings.Contains(head, "page-list") ||
			strings.Contains(head, "landmarks")) {
		return true
	}
	return false
}

// CleanEPUBText removes obvious EPUB residue and short heading-only rows from
// extracted plain text while preserving paragraph breaks for aggregation.
func CleanEPUBText(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blankPending := false
	for _, line := range lines {
		line = normalizeInlineSpace(strings.TrimSpace(line))
		if line == "" {
			if len(out) > 0 && !blankPending {
				out = append(out, "")
				blankPending = true
			}
			continue
		}
		if ShouldSkipEPUBLine(line) {
			continue
		}
		out = append(out, line)
		blankPending = false
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// ShouldSkipEPUBLine catches line-level front matter and heading residue before
// it reaches the sentence aggregator.
func ShouldSkipEPUBLine(line string) bool {
	return shouldSkipSentenceLikeText(line)
}

// IsUserFriendlySentence reports whether a canonical sentence is suitable for
// learner-facing sentence exports.
func IsUserFriendlySentence(text string) bool {
	return !shouldSkipSentenceLikeText(text)
}

func shouldSkipSentenceLikeText(text string) bool {
	text = normalizeInlineSpace(strings.TrimSpace(text))
	if text == "" {
		return true
	}
	if strings.Contains(text, "<") && strings.Contains(text, ">") {
		return true
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "&nbsp;") || strings.Contains(lower, "&amp;") {
		return true
	}
	for _, fragment := range residueFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	if strings.Contains(text, "....") || strings.Contains(text, "····") {
		return true
	}

	words := wordTokens(text)
	if len(words) == 0 {
		return true
	}
	if len(words) <= 3 && allNumericOrRoman(words) {
		return true
	}
	if len(words) <= 6 && digitHeavy(text) {
		return true
	}
	if looksHeadingLike(text, words) || looksNameOnly(words) {
		return true
	}
	return false
}

func looksHeadingLike(text string, words []string) bool {
	if len(words) == 0 {
		return false
	}
	first := compactForMatch(words[0])
	if headingPrefixes[first] {
		return true
	}
	if hasSentenceTerminal(text) {
		return false
	}
	if len(words) <= 8 && startsUpperOrDigit(text) {
		return true
	}
	return len(words) <= 4 && !hasLowercase(text)
}

func looksNameOnly(words []string) bool {
	if len(words) < 2 || len(words) > 6 {
		return false
	}
	for _, word := range words {
		if allNumericOrRoman([]string{word}) {
			continue
		}
		if !startsUpper(word) && !isInitial(word) {
			return false
		}
	}
	return true
}

func lowerPrefix(raw []byte, limit int) string {
	if len(raw) > limit {
		raw = raw[:limit]
	}
	return strings.ToLower(string(raw))
}

func isNumberedResource(key, prefix string) bool {
	if key == prefix {
		return true
	}
	if !strings.HasPrefix(key, prefix) {
		return false
	}
	for _, r := range key[len(prefix):] {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func normalizeInlineSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func wordTokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func compactForMatch(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'ä':
			r = 'a'
		case 'ö':
			r = 'o'
		case 'å':
			r = 'a'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasSentenceTerminal(s string) bool {
	for i := len(s); i > 0; {
		r, size := rune(s[i-1]), 1
		if r >= 0x80 {
			r, size = lastRune(s[:i])
		}
		i -= size
		if unicode.IsSpace(r) || strings.ContainsRune("\"'”’»)]}", r) {
			continue
		}
		return r == '.' || r == '!' || r == '?' || r == '…'
	}
	return false
}

func lastRune(s string) (rune, int) {
	var last rune
	var size int
	for i, r := range s {
		last = r
		size = len(s) - i
	}
	return last, size
}

func startsUpperOrDigit(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		return unicode.IsUpper(r) || unicode.IsDigit(r)
	}
	return false
}

func startsUpper(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return unicode.IsUpper(r)
		}
	}
	return false
}

func hasLowercase(s string) bool {
	for _, r := range s {
		if unicode.IsLower(r) {
			return true
		}
	}
	return false
}

func isInitial(s string) bool {
	rs := []rune(s)
	return len(rs) == 1 && unicode.IsUpper(rs[0])
}

func allNumericOrRoman(words []string) bool {
	for _, word := range words {
		key := compactForMatch(word)
		if key == "" {
			return false
		}
		allDigit := true
		for _, r := range key {
			if !unicode.IsDigit(r) {
				allDigit = false
				break
			}
		}
		if allDigit {
			continue
		}
		if !isRomanNumeral(key) {
			return false
		}
	}
	return true
}

func isRomanNumeral(s string) bool {
	if s == "" || len(s) > 12 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("ivxlcdm", r) {
			return false
		}
	}
	return true
}

func digitHeavy(s string) bool {
	digits, letters := 0, 0
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			digits++
		case unicode.IsLetter(r):
			letters++
		}
	}
	return digits > 0 && digits >= letters
}
