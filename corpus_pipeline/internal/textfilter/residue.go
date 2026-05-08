package textfilter

import (
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
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
	"sisukord":        true,
	"tableofcontents": true,
	"tiitelleht":      true,
	"title":           true,
	"titlepage":       true,
	"toc":             true,
}

var metadataLinePrefixes = []struct {
	words   int
	compact string
}{
	{3, "allrightsreserved"},
	{3, "koikoigusedkaitstud"},
	{2, "kaikkioikeudet"},
	{2, "coverdesign"},
	{2, "jacketdesign"},
	{1, "alkuteos"},
	{1, "autorioigus"},
	{1, "autorioigused"},
	{1, "calibre"},
	{1, "copyright"},
	{1, "julkaisija"},
	{1, "kansi"},
	{1, "kirjastus"},
	{1, "kustantaja"},
	{1, "originaal"},
	{1, "painettu"},
	{1, "painopaikka"},
	{1, "suomentanut"},
	{1, "tekijanoikeus"},
	{1, "tolge"},
	{1, "tolkinud"},
}

var headingPrefixes = map[string]bool{
	"chapter":        true,
	"contents":       true,
	"eessona":        true,
	"epilogue":       true,
	"epiloog":        true,
	"esipuhe":        true,
	"jalkisanat":     true,
	"jarelsana":      true,
	"jarelsona":      true,
	"luku":           true,
	"osa":            true,
	"peatukk":        true,
	"prologue":       true,
	"prologi":        true,
	"proloog":        true,
	"sissejuhatus":   true,
	"sisallys":       true,
	"sisukord":       true,
	"tanuavaldused":  true,
	"tanuavaldus":    true,
	"tunnustussonad": true,
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
	if strings.Contains(text, "....") || strings.Contains(text, "····") {
		return true
	}

	words := wordTokens(text)
	if len(words) == 0 {
		return true
	}
	if hasMetadataResidue(text, lower, words) {
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
	if len(words) <= 8 && startsDigit(text) {
		return true
	}
	if len(words) <= 8 && (isAllCapsText(text) || titleCaseRatio(words) >= 0.5) {
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
		case 'õ':
			r = 'o'
		case 'ü':
			r = 'u'
		case 'š':
			r = 's'
		case 'ž':
			r = 'z'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasSentenceTerminal(s string) bool {
	for len(s) > 0 {
		r, size := utf8.DecodeLastRuneInString(s)
		if r == utf8.RuneError && size == 0 {
			return false
		}
		s = s[:len(s)-size]
		if unicode.IsSpace(r) || strings.ContainsRune("\"'”’»)]}", r) {
			continue
		}
		return r == '.' || r == '!' || r == '?' || r == '…'
	}
	return false
}

func startsDigit(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		return unicode.IsDigit(r)
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

func hasMetadataResidue(text, lower string, words []string) bool {
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "www.") {
		return true
	}
	compact := compactForMatch(text)
	if strings.Contains(compact, "isbn") && containsDigit(text) {
		return true
	}
	if strings.Contains(text, "©") && (containsDigit(text) || len(words) <= 8) {
		return true
	}
	if strings.Contains(compact, "allrightsreserved") ||
		strings.Contains(compact, "kaikkioikeudet") ||
		strings.Contains(compact, "koikoigusedkaitstud") ||
		strings.Contains(compact, "generatedbycalibre") {
		return true
	}
	for _, prefix := range metadataLinePrefixes {
		if len(words) < prefix.words {
			continue
		}
		if leadingCompact(words, prefix.words) != prefix.compact {
			continue
		}
		if metadataLikeLine(text, words) {
			return true
		}
	}
	return false
}

func metadataLikeLine(text string, words []string) bool {
	if strings.ContainsAny(text, ":©") {
		return true
	}
	if !hasSentenceTerminal(text) {
		return true
	}
	return containsDigit(text) && len(words) <= 8
}

func leadingCompact(words []string, n int) string {
	var b strings.Builder
	for i := 0; i < n && i < len(words); i++ {
		b.WriteString(compactForMatch(words[i]))
	}
	return b.String()
}

func containsDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func titleCaseRatio(words []string) float64 {
	if len(words) == 0 {
		return 0
	}
	titleWords := 0
	letterWords := 0
	for _, word := range words {
		if allNumericOrRoman([]string{word}) {
			continue
		}
		letterWords++
		if startsUpper(word) || isInitial(word) {
			titleWords++
		}
	}
	if letterWords == 0 {
		return 0
	}
	return float64(titleWords) / float64(letterWords)
}

func isAllCapsText(s string) bool {
	hasLetter := false
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		hasLetter = true
		if unicode.IsLower(r) {
			return false
		}
	}
	return hasLetter
}
