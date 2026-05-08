package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
	"finnestdb/corpus_pipeline/internal/textfilter"
)

// extractEPUB walks <dir>/raw/*.epub, extracts plain text from each book's
// XHTML content, and writes:
//   - <dir>/per-book/<slug>.txt — one text per book (handy for reading)
//   - <dir>/text.txt — concatenated, one paragraph per line, blank line = book boundary
//   - <dir>/documents.jsonl — one record per book
//
// Idempotent unless force is true: reuses books whose per-book/<slug>.txt
// already exists and is non-empty.
func extractEPUB(dir string, m sources.Manifest, force bool) error {
	rawDir := filepath.Join(dir, "raw")
	perBookDir := filepath.Join(dir, "per-book")
	if err := os.MkdirAll(perBookDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		return err
	}

	textOut, err := os.Create(filepath.Join(dir, "text.txt"))
	if err != nil {
		return err
	}
	defer textOut.Close()
	docsOut, err := os.Create(filepath.Join(dir, "documents.jsonl"))
	if err != nil {
		return err
	}
	defer docsOut.Close()
	textW := bufio.NewWriterSize(textOut, 1<<20)
	defer textW.Flush()
	docsW := bufio.NewWriter(docsOut)
	defer docsW.Flush()

	processed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".epub") {
			continue
		}
		rawPath := filepath.Join(rawDir, name)
		slug := slugify(strings.TrimSuffix(name, filepath.Ext(name)))
		perBookPath := filepath.Join(perBookDir, slug+".txt")

		// Idempotency: reuse books already extracted unless forced. Clean the
		// cached text while rebuilding text.txt so line-level filters still
		// apply after heuristic changes.
		if !force {
			if fi, err := os.Stat(perBookPath); err == nil && fi.Size() > 0 {
				data, _ := os.ReadFile(perBookPath)
				textW.WriteString(textfilter.CleanEPUBText(string(data)))
				textW.WriteByte('\n')
				textW.WriteByte('\n')
				writeDocJSONL(docsW, m.Slug+":"+slug, slug, "", rawPath)
				processed++
				continue
			}
		}

		bookText, err := extractEPUBBook(rawPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[extract_epub] WARN: %s: %v\n", name, err)
			continue
		}
		if strings.TrimSpace(bookText) == "" {
			fmt.Fprintf(os.Stderr, "[extract_epub] WARN: %s: empty after extraction\n", name)
			continue
		}
		if err := os.WriteFile(perBookPath, []byte(bookText), 0o644); err != nil {
			return err
		}
		textW.WriteString(bookText)
		textW.WriteByte('\n')
		textW.WriteByte('\n')
		writeDocJSONL(docsW, m.Slug+":"+slug, slug, "", rawPath)
		processed++
		if processed%25 == 0 {
			fmt.Fprintf(os.Stderr, "[extract_epub] %d books processed\n", processed)
		}
	}
	fmt.Fprintf(os.Stderr, "[extract_epub] done: %d books\n", processed)
	return nil
}

// extractEPUBBook returns the concatenated plain text of all xhtml/html
// files in an EPUB.
func extractEPUBBook(epubPath string) (string, error) {
	r, err := zip.OpenReader(epubPath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	// Sort file names so output is deterministic (per-book/<slug>.txt
	// re-runs produce identical content).
	type entry struct {
		name string
		f    *zip.File
	}
	var docs []entry
	for _, f := range r.File {
		lower := strings.ToLower(f.Name)
		if strings.HasSuffix(lower, ".xhtml") || strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm") {
			docs = append(docs, entry{f.Name, f})
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].name < docs[j].name })

	var sb strings.Builder
	for _, d := range docs {
		if textfilter.ShouldSkipEPUBResource(d.name, nil) {
			continue
		}
		rc, err := d.f.Open()
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		if textfilter.ShouldSkipEPUBResource(d.name, raw) {
			continue
		}
		text := textfilter.CleanEPUBText(stripXHTML(string(raw)))
		if strings.TrimSpace(text) == "" {
			continue
		}
		sb.WriteString(text)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// stripXHTML removes XML/HTML tags and decodes the most common entities.
// Block-level tags become paragraph breaks. Pragmatic — not a full HTML
// parser, but good enough for corpus ingestion.
var (
	// Style/script content must be removed entirely (the text inside is CSS/JS, not prose).
	reStyleBlock    = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`)
	reScriptBlock   = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	reCommentBlock  = regexp.MustCompile(`(?s)<!--.*?-->`)
	reBlockTag      = regexp.MustCompile(`(?i)</?(p|div|br|h[1-6]|li|ul|ol|blockquote|tr|hr|section|article|header|footer|main|aside|nav|figure|figcaption|table|thead|tbody|tfoot|td|th)\b[^>]*/?>`)
	reAnyTag        = regexp.MustCompile(`<[^>]+>`)
	reMultiNewline  = regexp.MustCompile(`\n{3,}`)
	reLineLeadSpace = regexp.MustCompile(`(?m)^[ \t]+`)
	reMultiSpace    = regexp.MustCompile(`[ \t]+`)
)

func stripXHTML(s string) string {
	// Remove style/script/comment blocks entirely BEFORE tag stripping.
	s = reStyleBlock.ReplaceAllString(s, "")
	s = reScriptBlock.ReplaceAllString(s, "")
	s = reCommentBlock.ReplaceAllString(s, "")
	// Replace block-level tags with newlines so paragraphs survive.
	s = reBlockTag.ReplaceAllString(s, "\n")
	// Strip remaining tags.
	s = reAnyTag.ReplaceAllString(s, "")
	// Decode common entities.
	s = strings.NewReplacer(
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
	).Replace(s)
	// Normalize whitespace: collapse 3+ newlines to 2, trim line-leading
	// space, collapse multiple inline spaces.
	s = reLineLeadSpace.ReplaceAllString(s, "")
	s = reMultiSpace.ReplaceAllString(s, " ")
	s = reMultiNewline.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func slugify(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r == ' ' || r == '_' || r == '-' || r == '.':
			b.WriteByte('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return strings.Trim(out, "-")
}

func writeDocJSONL(w *bufio.Writer, docID, title, author, rawPath string) {
	d := docRecord{DocumentID: docID, Title: title, Author: author, RawPath: rawPath}
	_ = writeJSONL(w, d)
}
