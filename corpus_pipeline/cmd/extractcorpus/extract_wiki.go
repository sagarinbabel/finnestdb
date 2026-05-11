package main

import (
	"bufio"
	"compress/bzip2"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
)

// extractWiki handles MediaWiki XML.bz2 dumps (Wikipedia FI/ET, etc.).
// Streams the bz2 → XML decoder, walks <page> elements, keeps only main
// namespace (ns=0) non-redirect articles, strips MediaWiki markup, and
// writes cleaned paragraphs to text.txt with a blank line between articles.
//
// The markup stripper handles the common cases that account for ~95% of
// surface noise in a real dump: {{templates}}, [[links]], <ref>...</ref>,
// File/Image/Category links, HTML tags, tables, headings, list markers,
// and HTML entities. It is intentionally not a faithful MediaWiki parser
// (those are vastly more complex) — just enough to leave clean prose
// paragraphs that parserffi can tokenize sensibly.
func extractWiki(dir string, m sources.Manifest) error {
	rawDir := filepath.Join(dir, "raw")
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

	totalArticles := 0
	totalSkipped := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".xml.bz2") && !strings.HasSuffix(lower, ".xml") {
			continue
		}
		rawPath := filepath.Join(rawDir, name)
		f, err := os.Open(rawPath)
		if err != nil {
			return err
		}
		var reader io.Reader = f
		if strings.HasSuffix(lower, ".bz2") {
			reader = bzip2.NewReader(f)
		}
		articles, skipped, err := extractWikiStream(reader, m.Slug, rawPath, textW, docsW)
		f.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[extract_wiki] %s/%s: %v\n", m.Slug, name, err)
			continue
		}
		totalArticles += articles
		totalSkipped += skipped
	}
	fmt.Fprintf(os.Stderr, "[extract_wiki] %s: %d articles kept, %d skipped (redirects/non-main-ns)\n",
		m.Slug, totalArticles, totalSkipped)
	return nil
}

// extractWikiStream walks the XML stream and emits each main-namespace
// article's cleaned text. Returns (articles_kept, articles_skipped).
func extractWikiStream(rc io.Reader, slug, rawPath string, textW *bufio.Writer, docsW *bufio.Writer) (int, int, error) {
	dec := xml.NewDecoder(rc)
	dec.Strict = false
	dec.Entity = xml.HTMLEntity

	var (
		inPage, inRevision               bool
		captureTitle, captureText, captureNS bool
		isRedirect                       bool
		titleBuf, textBuf, nsBuf         strings.Builder
		articlesKept, articlesSkipped    int
	)

	flushPage := func() {
		title := strings.TrimSpace(titleBuf.String())
		ns := strings.TrimSpace(nsBuf.String())
		raw := textBuf.String()

		// Reset for next page
		titleBuf.Reset()
		textBuf.Reset()
		nsBuf.Reset()
		isRedirectLocal := isRedirect
		isRedirect = false

		// Skip non-main namespace (only ns=0 is articles)
		if ns != "" && ns != "0" {
			articlesSkipped++
			return
		}
		// Skip redirects
		if isRedirectLocal {
			articlesSkipped++
			return
		}
		// Skip empty
		if raw == "" || title == "" {
			articlesSkipped++
			return
		}
		clean := cleanWikiMarkup(raw)
		clean = strings.TrimSpace(clean)
		if clean == "" {
			articlesSkipped++
			return
		}
		// Emit one paragraph per non-empty line.
		for _, para := range strings.Split(clean, "\n") {
			para = strings.TrimSpace(para)
			if para == "" {
				continue
			}
			// Synthesize sentence-ending punctuation if needed for parserffi.
			if last := para[len(para)-1]; last != '.' && last != '!' && last != '?' && last != ':' {
				para += "."
			}
			textW.WriteString(para)
			textW.WriteByte('\n')
		}
		textW.WriteByte('\n') // article boundary
		docID := slug + ":" + slugify(title)
		writeDocJSONL(docsW, docID, title, "", rawPath)
		articlesKept++
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Tolerate malformed XML; some dumps have stray bytes
			continue
		}
		switch t := tok.(type) {
		case xml.StartElement:
			lname := strings.ToLower(t.Name.Local)
			switch lname {
			case "page":
				inPage = true
			case "title":
				if inPage {
					captureTitle = true
				}
			case "ns":
				if inPage {
					captureNS = true
				}
			case "redirect":
				if inPage {
					isRedirect = true
				}
			case "revision":
				if inPage {
					inRevision = true
				}
			case "text":
				if inRevision {
					captureText = true
				}
			}
		case xml.EndElement:
			lname := strings.ToLower(t.Name.Local)
			switch lname {
			case "title":
				captureTitle = false
			case "ns":
				captureNS = false
			case "text":
				captureText = false
			case "revision":
				inRevision = false
			case "page":
				inPage = false
				flushPage()
			}
		case xml.CharData:
			if captureTitle {
				titleBuf.Write(t)
			} else if captureNS {
				nsBuf.Write(t)
			} else if captureText {
				textBuf.Write(t)
			}
		}
	}
	return articlesKept, articlesSkipped, nil
}

// MediaWiki markup stripping. Order matters — strip nested constructs
// (templates, refs) before splitting on lines.

var (
	reHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reRefSelf     = regexp.MustCompile(`(?i)<ref[^/>]*/>`)
	reRefBlock    = regexp.MustCompile(`(?is)<ref[^>]*>.*?</ref>`)
	reHTMLTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reHeadings    = regexp.MustCompile(`(?m)^=+\s*(.*?)\s*=+\s*$`)
	reLinkPiped   = regexp.MustCompile(`\[\[([^\[\]|]+)\|([^\[\]]+)\]\]`)
	reLinkPlain   = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)
	reExtLink     = regexp.MustCompile(`\[(?:https?|ftp)://[^\s\]]+\s+([^\]]+)\]`)
	reExtLinkBare = regexp.MustCompile(`\[(?:https?|ftp)://[^\s\]]+\]`)
	reBoldItalic  = regexp.MustCompile(`'{2,5}`)
	reMultiNL     = regexp.MustCompile(`\n{3,}`)
	reWikiMultiSpace  = regexp.MustCompile(`[ \t]{2,}`)
)

// cleanWikiMarkup applies the strip pipeline to a raw <text> body.
func cleanWikiMarkup(s string) string {
	s = reHTMLComment.ReplaceAllString(s, "")
	s = stripBraced(s, "{{", "}}") // templates
	s = stripBraced(s, "{|", "|}") // tables
	s = reRefSelf.ReplaceAllString(s, "")
	s = reRefBlock.ReplaceAllString(s, "")

	// File/Image/Category links: drop entire bracketed expression including
	// any trailing | parameters. Walk char-by-char to handle nesting.
	s = stripFileLikeLinks(s)

	// Piped links → second component
	s = reLinkPiped.ReplaceAllString(s, "$2")
	// Plain links → unwrap
	s = reLinkPlain.ReplaceAllString(s, "$1")
	// External links with display text
	s = reExtLink.ReplaceAllString(s, "$1")
	// Bare external links
	s = reExtLinkBare.ReplaceAllString(s, "")

	s = reHeadings.ReplaceAllString(s, "$1")
	s = reBoldItalic.ReplaceAllString(s, "")
	s = reHTMLTag.ReplaceAllString(s, "")

	// Strip leading list/indent markers per line
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, "*#:;> \t")
		lines[i] = trimmed
	}
	s = strings.Join(lines, "\n")

	s = reWikiMultiSpace.ReplaceAllString(s, " ")
	s = reMultiNL.ReplaceAllString(s, "\n\n")
	return s
}

// stripBraced removes substrings starting with `open` and ending with the
// matching `close`, supporting nesting. E.g. stripBraced("{{a{{b}}c}}", "{{", "}}") → "".
func stripBraced(s, open, close string) string {
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		// Look for the next `open`
		j := strings.Index(s[i:], open)
		if j < 0 {
			out.WriteString(s[i:])
			break
		}
		out.WriteString(s[i : i+j])
		// Skip over the matched braced expression with depth counter
		depth := 1
		k := i + j + len(open)
		for k < len(s) && depth > 0 {
			if strings.HasPrefix(s[k:], open) {
				depth++
				k += len(open)
			} else if strings.HasPrefix(s[k:], close) {
				depth--
				k += len(close)
			} else {
				k++
			}
		}
		i = k
	}
	return out.String()
}

// stripFileLikeLinks drops entire [[File:...]], [[Image:...]],
// [[Tiedosto:...]], [[Pilt:...]], [[Category:...]], [[Kategoria:...]],
// [[Kategooria:...]] expressions including any trailing | params (which can
// themselves contain nested [[...]]).
func stripFileLikeLinks(s string) string {
	// File/Image and Category prefixes across English, Finnish, Estonian.
	// Order doesn't matter; first prefix that matches wins.
	prefixes := []string{
		// Files / images / media
		"[[File:", "[[Image:", "[[Media:",
		"[[Tiedosto:", "[[Kuva:", // FI
		"[[Pilt:", "[[Fail:", // ET
		// Categories
		"[[Category:",
		"[[Luokka:", "[[Kategoria:", // FI (modern Luokka, legacy Kategoria)
		"[[Kategooria:", // ET
	}
	var out strings.Builder
	out.Grow(len(s))
	i := 0
outer:
	for i < len(s) {
		// Try each prefix
		for _, p := range prefixes {
			if strings.HasPrefix(s[i:], p) {
				// Skip until matching ]] at depth 0
				depth := 1
				k := i + len(p)
				for k < len(s)-1 && depth > 0 {
					if s[k] == '[' && s[k+1] == '[' {
						depth++
						k += 2
					} else if s[k] == ']' && s[k+1] == ']' {
						depth--
						k += 2
					} else {
						k++
					}
				}
				i = k
				continue outer
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}
