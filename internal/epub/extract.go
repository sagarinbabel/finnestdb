// Package epub extracts plain text from a single EPUB file for use in the
// learner-app import flow. It is intentionally pragmatic — not a full HTML
// parser. The logic mirrors corpus_pipeline/cmd/extractcorpus/extract_epub.go
// (which lives in a separate Go module) but is scoped to one in-memory book
// at a time rather than walking a corpus directory.
package epub

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Chapter is one extracted content document from an EPUB.
type Chapter struct {
	// Title is the chapter's display name. Sourced from the first <h1>...</h1>
	// in the document, falling back to <title>...</title>, then to the basename
	// of the zip path (e.g. "OEBPS/ch01.xhtml" → "ch01"). Never empty.
	Title string
	// Text is the chapter's stripped plain-text body.
	Text string
}

// ExtractChapters returns each XHTML/HTML content document in the EPUB as a
// separate Chapter, in alphabetical filename order. Documents whose body is
// empty after stripping are skipped. Returns an error if the bytes are not a
// valid zip or contain no readable content.
func ExtractChapters(r io.ReaderAt, size int64) ([]Chapter, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("read epub: %w", err)
	}

	type entry struct {
		name string
		f    *zip.File
	}
	var docs []entry
	for _, f := range zr.File {
		lower := strings.ToLower(f.Name)
		if !strings.HasSuffix(lower, ".xhtml") && !strings.HasSuffix(lower, ".html") && !strings.HasSuffix(lower, ".htm") {
			continue
		}
		if isNonContentEPUBFile(lower) {
			continue
		}
		docs = append(docs, entry{f.Name, f})
	}
	if len(docs) == 0 {
		return nil, errors.New("no XHTML/HTML content documents found in EPUB")
	}
	// Natural sort: an EPUB whose files are named "ch1.xhtml … ch10.xhtml"
	// sorts wrong lexically ("ch1, ch10, ch11, ch2, …"). Compare prefix +
	// trailing-number tuples so "ch10" follows "ch9".
	sort.Slice(docs, func(i, j int) bool { return naturalLess(docs[i].name, docs[j].name) })

	chapters := make([]Chapter, 0, len(docs))
	for _, d := range docs {
		rc, err := d.f.Open()
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		rawStr := string(raw)
		text := stripXHTML(rawStr)
		if strings.TrimSpace(text) == "" {
			continue
		}
		chapters = append(chapters, Chapter{
			Title: extractChapterTitle(rawStr, d.name),
			Text:  text,
		})
	}
	if len(chapters) == 0 {
		return nil, errors.New("EPUB extracted to empty text")
	}
	return chapters, nil
}

// ExtractChaptersFromBytes is a convenience wrapper around ExtractChapters.
func ExtractChaptersFromBytes(data []byte) ([]Chapter, error) {
	return ExtractChapters(bytes.NewReader(data), int64(len(data)))
}

// ExtractText concatenates the chapter texts into one string with a blank
// line between chapters. Returns an error under the same conditions as
// ExtractChapters.
func ExtractText(r io.ReaderAt, size int64) (string, error) {
	chapters, err := ExtractChapters(r, size)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, ch := range chapters {
		sb.WriteString(ch.Text)
		sb.WriteByte('\n')
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String()), nil
}

// ExtractTextFromBytes is a convenience wrapper around ExtractText.
func ExtractTextFromBytes(data []byte) (string, error) {
	return ExtractText(bytes.NewReader(data), int64(len(data)))
}

var (
	reH1Tag    = regexp.MustCompile(`(?is)<h1\b[^>]*>(.*?)</h1\s*>`)
	reTitleTag = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title\s*>`)
)

// isNonContentEPUBFile skips files that are HTML by extension but aren't
// reading content — currently the iBooks DRM marker. Conservative: only known
// patterns are filtered.
func isNonContentEPUBFile(lowerName string) bool {
	return strings.Contains(lowerName, "rrownerinfo")
}

// naturalLess compares two paths so embedded integer runs sort numerically.
// "ch2.xhtml" < "ch10.xhtml" rather than the lexical opposite. Compares
// segment-by-segment over the whole path. Case-insensitive.
func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	la, lb := strings.ToLower(a), strings.ToLower(b)
	for ai < len(la) && bi < len(lb) {
		ad := isDigit(la[ai])
		bd := isDigit(lb[bi])
		if ad && bd {
			// Read full integer runs from both sides and compare numerically.
			as, ae := ai, ai
			for ae < len(la) && isDigit(la[ae]) {
				ae++
			}
			bs, be := bi, bi
			for be < len(lb) && isDigit(lb[be]) {
				be++
			}
			an, _ := strconv.Atoi(la[as:ae])
			bn, _ := strconv.Atoi(lb[bs:be])
			if an != bn {
				return an < bn
			}
			ai, bi = ae, be
			continue
		}
		if la[ai] != lb[bi] {
			return la[ai] < lb[bi]
		}
		ai++
		bi++
	}
	return len(la) < len(lb)
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// extractChapterTitle picks a human-readable title for one chapter. Looks at
// the first <h1>, then <title>, then falls back to the basename of the zip
// path. Always returns a non-empty string.
func extractChapterTitle(rawHTML, zipPath string) string {
	if m := reH1Tag.FindStringSubmatch(rawHTML); len(m) == 2 {
		if t := strings.TrimSpace(stripXHTML(m[1])); t != "" {
			return t
		}
	}
	if m := reTitleTag.FindStringSubmatch(rawHTML); len(m) == 2 {
		if t := strings.TrimSpace(stripXHTML(m[1])); t != "" {
			return t
		}
	}
	base := zipPath
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	if base == "" {
		return "Untitled"
	}
	return base
}

var (
	reHeadBlock     = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head\s*>`)
	reStyleBlock    = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`)
	reScriptBlock   = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	reCommentBlock  = regexp.MustCompile(`(?s)<!--.*?-->`)
	reBlockTag      = regexp.MustCompile(`(?i)</?(p|div|br|h[1-6]|li|ul|ol|blockquote|tr|hr|section|article|header|footer|main|aside|nav|figure|figcaption|table|thead|tbody|tfoot|td|th)\b[^>]*/?>`)
	reAnyTag        = regexp.MustCompile(`<[^>]+>`)
	reMultiNewline  = regexp.MustCompile(`\n{3,}`)
	reLineLeadSpace = regexp.MustCompile(`(?m)^[ \t]+`)
	reMultiSpace    = regexp.MustCompile(`[ \t]+`)
)

var entityReplacer = strings.NewReplacer(
	"&amp;", "&",
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", "\"",
	"&apos;", "'",
	"&nbsp;", " ",
	"&#160;", " ",
	"&hellip;", "…",
	"&mdash;", "—",
	"&ndash;", "–",
	"&rsquo;", "'",
	"&lsquo;", "'",
	"&rdquo;", "\"",
	"&ldquo;", "\"",
)

// stripXHTML removes XML/HTML tags and decodes the most common entities.
// Block-level tags become paragraph breaks so reading structure survives.
func stripXHTML(s string) string {
	// Drop <head> entirely so <title>'s content doesn't leak into the body
	// text. Same reasoning for <style> and <script>.
	s = reHeadBlock.ReplaceAllString(s, "")
	s = reStyleBlock.ReplaceAllString(s, "")
	s = reScriptBlock.ReplaceAllString(s, "")
	s = reCommentBlock.ReplaceAllString(s, "")
	s = reBlockTag.ReplaceAllString(s, "\n")
	s = reAnyTag.ReplaceAllString(s, "")
	s = entityReplacer.Replace(s)
	s = reLineLeadSpace.ReplaceAllString(s, "")
	s = reMultiSpace.ReplaceAllString(s, " ")
	s = reMultiNewline.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
