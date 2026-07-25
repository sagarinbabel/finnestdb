package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
)

// extractVRT handles VRT format (Vertical Text - used by IMS Corpus
// Workbench, Yle Kielipankki, etc.). Format:
//
//   <text id="..." date="..." ...>
//   <paragraph>
//   <sentence>
//   form	lemma	POS	morpho	...    ← TAB-separated, one token per line
//   form	lemma	POS	morpho	...
//   </sentence>
//   <sentence>
//   ...
//   </paragraph>
//   </text>
//
// Blank lines may also separate sentences.
//
// Yle Kielipankki ships VRT inside a .zip. We walk the zip, extract
// each .vrt file, parse, and emit reconstructed sentences (joined
// from the FORM column).
func extractVRT(dir string, m sources.Manifest) error {
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

	totalSent := 0
	totalDoc := 0

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".zip") {
			continue
		}
		rawPath := filepath.Join(rawDir, name)
		zr, err := zip.OpenReader(rawPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[extract_vrt] %s: %v\n", name, err)
			continue
		}
		for _, zf := range zr.File {
			zlower := strings.ToLower(zf.Name)
			if !strings.HasSuffix(zlower, ".vrt") {
				continue
			}
			rc, err := zf.Open()
			if err != nil {
				continue
			}
			sentCount, docCount, err := extractVRTStream(rc, m.Slug, zf.Name, rawPath, textW, docsW)
			rc.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[extract_vrt] %s/%s: %v\n", name, zf.Name, err)
				continue
			}
			totalSent += sentCount
			totalDoc += docCount
		}
		zr.Close()
	}
	fmt.Fprintf(os.Stderr, "[extract_vrt] %s: %d sentences across %d docs\n", m.Slug, totalSent, totalDoc)
	return nil
}

// extractVRTStream parses a VRT file from rc and writes reconstructed
// sentences to textW + per-doc records to docsW. Returns counts.
func extractVRTStream(rc io.Reader, slug, zipEntryName, rawPath string, textW *bufio.Writer, docsW *bufio.Writer) (int, int, error) {
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 1<<20), 16<<20)

	var (
		currentDoc        string
		currentSentTokens []string
		inSentence        bool
		sentCount         int
		docCount          int
	)
	flushSentence := func() {
		if !inSentence || len(currentSentTokens) == 0 {
			return
		}
		// Reconstruct: join tokens with spaces, but no leading space before
		// pure-punctuation tokens.
		var sb strings.Builder
		for i, t := range currentSentTokens {
			if i > 0 && !isPunctOnly(t) {
				sb.WriteByte(' ')
			}
			sb.WriteString(t)
		}
		text := sb.String()
		if last := text[len(text)-1]; last != '.' && last != '!' && last != '?' {
			text += "."
		}
		textW.WriteString(text)
		textW.WriteByte('\n')
		sentCount++
	}
	flushDoc := func() {
		if currentDoc == "" {
			return
		}
		textW.WriteByte('\n') // doc boundary
		docID := slug + ":" + currentDoc
		writeDocJSONL(docsW, docID, currentDoc, "", rawPath)
		docCount++
		currentDoc = ""
	}

	for sc.Scan() {
		line := sc.Text()
		// Tags: <text id="...">, </text>, <sentence>, </sentence>, <paragraph>, etc.
		if strings.HasPrefix(line, "<") {
			ll := strings.ToLower(line)
			if strings.HasPrefix(ll, "<text") {
				flushSentence()
				inSentence = false
				currentSentTokens = nil
				flushDoc()
				// Try to extract id="..." or just use a counter
				currentDoc = parseAttr(line, "id")
				if currentDoc == "" {
					currentDoc = fmt.Sprintf("%s-doc-%d", zipEntryName, docCount)
				}
				continue
			}
			if strings.HasPrefix(ll, "</text") {
				flushSentence()
				inSentence = false
				currentSentTokens = nil
				flushDoc()
				continue
			}
			if strings.HasPrefix(ll, "<sentence") || strings.HasPrefix(ll, "<s ") || ll == "<s>" {
				flushSentence()
				inSentence = true
				currentSentTokens = currentSentTokens[:0]
				continue
			}
			if strings.HasPrefix(ll, "</sentence") || strings.HasPrefix(ll, "</s>") {
				flushSentence()
				inSentence = false
				currentSentTokens = currentSentTokens[:0]
				continue
			}
			// Other tags (paragraph, head, etc.) - ignore
			continue
		}
		// Blank line: end of sentence
		if line == "" {
			flushSentence()
			inSentence = false
			currentSentTokens = currentSentTokens[:0]
			continue
		}
		// Token line: TAB-sep, first column is FORM
		fields := strings.Split(line, "\t")
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		// If we never saw <sentence>, treat each block as its own sentence
		// (some VRT files don't wrap each sentence in <s>).
		if !inSentence {
			inSentence = true
			currentSentTokens = currentSentTokens[:0]
		}
		currentSentTokens = append(currentSentTokens, fields[0])
	}
	// Flush trailing
	flushSentence()
	flushDoc()
	if err := sc.Err(); err != nil {
		return sentCount, docCount, err
	}
	return sentCount, docCount, nil
}

// parseAttr extracts attribute value from a tag line like
//   <text id="74-20223215" date="2022-09-01" ...>
// returns "74-20223215" for parseAttr(line, "id").
func parseAttr(line, attr string) string {
	needle := attr + "=\""
	i := strings.Index(line, needle)
	if i < 0 {
		return ""
	}
	rest := line[i+len(needle):]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func isPunctOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r == '\'' || r == '"' {
			continue
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			return false
		}
	}
	return true
}
