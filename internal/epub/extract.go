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

// BookMetadata holds the Dublin Core metadata an EPUB advertises in its OPF
// package file. Empty fields mean the EPUB didn't supply them.
type BookMetadata struct {
	Title  string // <dc:title>
	Author string // <dc:creator>
}

// ExtractMetadata returns the book's title/author from the EPUB's OPF
// package, falling back to empty strings when fields are missing. Errors on
// invalid zip but never on missing metadata — a metadata-less EPUB is valid.
func ExtractMetadata(r io.ReaderAt, size int64) (BookMetadata, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return BookMetadata{}, fmt.Errorf("read epub: %w", err)
	}
	return readMetadataFromZip(zr), nil
}

// ExtractMetadataFromBytes is a convenience wrapper around ExtractMetadata.
func ExtractMetadataFromBytes(data []byte) (BookMetadata, error) {
	return ExtractMetadata(bytes.NewReader(data), int64(len(data)))
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

	// Prefer the OPF spine: it gives the publisher's intended reading order
	// AND excludes non-reading-order files like nav documents. Fall back to
	// natural-sorted file walk when the EPUB has no container.xml/OPF or the
	// spine can't be resolved.
	docs := resolveSpineDocs(zr)
	if docs == nil {
		docs = walkContentDocs(zr)
	}
	if len(docs) == 0 {
		return nil, errors.New("no XHTML/HTML content documents found in EPUB")
	}

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

// docEntry is one ordered content document inside the EPUB zip.
type docEntry struct {
	name string
	f    *zip.File
}

// resolveSpineDocs returns content documents in the publisher-declared reading
// order (META-INF/container.xml → OPF rootfile → manifest + spine). Returns
// nil if container.xml or the OPF can't be parsed, signalling the caller to
// fall back to a filename walk.
func resolveSpineDocs(zr *zip.Reader) []docEntry {
	containerXML := readZipFile(zr, "META-INF/container.xml")
	if containerXML == "" {
		return nil
	}
	opfPath := parseContainerRootfile(containerXML)
	if opfPath == "" {
		return nil
	}
	opfXML := readZipFile(zr, opfPath)
	if opfXML == "" {
		return nil
	}
	manifest := parseOPFManifest(opfXML)
	if len(manifest) == 0 {
		return nil
	}
	idrefs := parseOPFSpine(opfXML)
	if len(idrefs) == 0 {
		return nil
	}

	// Resolve hrefs relative to the OPF's directory.
	opfDir := ""
	if i := strings.LastIndex(opfPath, "/"); i >= 0 {
		opfDir = opfPath[:i+1]
	}

	byName := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		byName[f.Name] = f
	}

	docs := make([]docEntry, 0, len(idrefs))
	for _, idref := range idrefs {
		href, ok := manifest[idref]
		if !ok || href == "" {
			continue
		}
		full := joinZipPath(opfDir, href)
		lower := strings.ToLower(full)
		if !strings.HasSuffix(lower, ".xhtml") && !strings.HasSuffix(lower, ".html") && !strings.HasSuffix(lower, ".htm") {
			continue
		}
		if isNonContentEPUBFile(lower) {
			continue
		}
		f, ok := byName[full]
		if !ok {
			continue
		}
		docs = append(docs, docEntry{name: full, f: f})
	}
	if len(docs) == 0 {
		return nil
	}
	return docs
}

// walkContentDocs is the fallback when no spine is available: collect every
// HTML-like file in the zip and natural-sort by path.
func walkContentDocs(zr *zip.Reader) []docEntry {
	var docs []docEntry
	for _, f := range zr.File {
		lower := strings.ToLower(f.Name)
		if !strings.HasSuffix(lower, ".xhtml") && !strings.HasSuffix(lower, ".html") && !strings.HasSuffix(lower, ".htm") {
			continue
		}
		if isNonContentEPUBFile(lower) {
			continue
		}
		docs = append(docs, docEntry{name: f.Name, f: f})
	}
	sort.Slice(docs, func(i, j int) bool { return naturalLess(docs[i].name, docs[j].name) })
	return docs
}

// readZipFile reads a single named file from the archive. Returns "" if the
// file is missing or unreadable.
func readZipFile(zr *zip.Reader, name string) string {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return ""
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return ""
			}
			return string(data)
		}
	}
	return ""
}

// joinZipPath resolves an href like "Text/ch1.xhtml" relative to an OPF
// directory like "OEBPS/". Drops a leading slash on href and collapses ".."
// segments minimally (one-level only — EPUB hrefs almost never go above the
// OPF directory).
func joinZipPath(dir, href string) string {
	href = strings.TrimPrefix(href, "/")
	if dir == "" {
		return href
	}
	// Handle the rare "../foo" case where the OPF lives in a subdirectory.
	for strings.HasPrefix(href, "../") {
		href = strings.TrimPrefix(href, "../")
		if i := strings.LastIndex(strings.TrimSuffix(dir, "/"), "/"); i >= 0 {
			dir = dir[:i+1]
		} else {
			dir = ""
		}
	}
	return dir + href
}

var (
	reContainerRootfile = regexp.MustCompile(`(?is)<rootfile\b[^>]*\bfull-path="([^"]+)"`)
	reOPFManifestItem   = regexp.MustCompile(`(?is)<item\b([^>]*)/?>`)
	reOPFItemID         = regexp.MustCompile(`(?is)\bid="([^"]+)"`)
	reOPFItemHref       = regexp.MustCompile(`(?is)\bhref="([^"]+)"`)
	reOPFSpineBlock     = regexp.MustCompile(`(?is)<spine\b[^>]*>(.*?)</spine\s*>`)
	reOPFItemref        = regexp.MustCompile(`(?is)<itemref\b([^>]*)/?>`)
	reOPFIdref          = regexp.MustCompile(`(?is)\bidref="([^"]+)"`)
)

// parseContainerRootfile returns the full-path of the first <rootfile/> in
// META-INF/container.xml, or "" if not found.
func parseContainerRootfile(xml string) string {
	if m := reContainerRootfile.FindStringSubmatch(xml); len(m) == 2 {
		return m[1]
	}
	return ""
}

var reOPFItemProperties = regexp.MustCompile(`(?is)\bproperties="([^"]+)"`)

// parseOPFManifest returns an id → href map of every <item> in the manifest.
// Items marked with `properties="nav"` (EPUB3 navigation documents) are
// skipped — they're the table-of-contents, not learner reading content.
func parseOPFManifest(opf string) map[string]string {
	manifestStart := strings.Index(opf, "<manifest")
	manifestEnd := strings.Index(opf, "</manifest")
	if manifestStart < 0 || manifestEnd < 0 || manifestEnd <= manifestStart {
		return nil
	}
	body := opf[manifestStart:manifestEnd]
	out := make(map[string]string)
	for _, m := range reOPFManifestItem.FindAllStringSubmatch(body, -1) {
		attrs := m[1]
		idMatch := reOPFItemID.FindStringSubmatch(attrs)
		hrefMatch := reOPFItemHref.FindStringSubmatch(attrs)
		if len(idMatch) != 2 || len(hrefMatch) != 2 {
			continue
		}
		if propMatch := reOPFItemProperties.FindStringSubmatch(attrs); len(propMatch) == 2 {
			props := strings.Fields(propMatch[1])
			isNav := false
			for _, p := range props {
				if p == "nav" {
					isNav = true
					break
				}
			}
			if isNav {
				continue
			}
		}
		out[idMatch[1]] = hrefMatch[1]
	}
	return out
}

// parseOPFSpine returns the ordered list of idref values from <spine>.
func parseOPFSpine(opf string) []string {
	block := reOPFSpineBlock.FindStringSubmatch(opf)
	if len(block) != 2 {
		return nil
	}
	var out []string
	for _, m := range reOPFItemref.FindAllStringSubmatch(block[1], -1) {
		if id := reOPFIdref.FindStringSubmatch(m[1]); len(id) == 2 {
			out = append(out, id[1])
		}
	}
	return out
}

// readMetadataFromZip walks the zip for an OPF package file and pulls
// <dc:title> / <dc:creator> out of it. Returns zero-value metadata if no OPF
// is present or its dc fields are empty.
func readMetadataFromZip(zr *zip.Reader) BookMetadata {
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".opf") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		return parseOPFMetadata(string(raw))
	}
	return BookMetadata{}
}

var (
	// `[^/>]*` in the open-tag attribute slot excludes self-closing forms
	// (`<dc:title/>`) so the non-greedy body doesn't span across the empty
	// element into the next element's content.
	reDCTitle   = regexp.MustCompile(`(?is)<dc:title\b[^/>]*>(.*?)</dc:title\s*>`)
	reDCCreator = regexp.MustCompile(`(?is)<dc:creator\b[^/>]*>(.*?)</dc:creator\s*>`)
)

func parseOPFMetadata(opf string) BookMetadata {
	out := BookMetadata{}
	if m := reDCTitle.FindStringSubmatch(opf); len(m) == 2 {
		out.Title = strings.TrimSpace(stripXHTML(m[1]))
	}
	// First non-empty <dc:creator> wins. EPUBs sometimes declare an empty
	// placeholder before the real one.
	for _, m := range reDCCreator.FindAllStringSubmatch(opf, -1) {
		if name := strings.TrimSpace(stripXHTML(m[1])); name != "" {
			out.Author = name
			break
		}
	}
	return out
}

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
