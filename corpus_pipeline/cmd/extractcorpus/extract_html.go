package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
)

// extractHTML strips HTML files in <source>/raw/*.html into clean
// paragraph-per-line text. Reuses the regex tag stripper from
// extract_epub. For folder-driven sources where someone has already
// downloaded HTML pages.
//
// Polite scrapers should write their output here (one .html per page).
func extractHTML(dir string, m sources.Manifest) error {
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

	processed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".html") && !strings.HasSuffix(lower, ".htm") && !strings.HasSuffix(lower, ".xhtml") {
			continue
		}
		rawPath := filepath.Join(rawDir, name)
		raw, err := os.ReadFile(rawPath)
		if err != nil {
			return err
		}
		text := stripXHTML(string(raw))
		if strings.TrimSpace(text) == "" {
			continue
		}
		textW.WriteString(text)
		textW.WriteByte('\n')
		textW.WriteByte('\n')
		slug := slugify(strings.TrimSuffix(name, filepath.Ext(name)))
		writeDocJSONL(docsW, m.Slug+":"+slug, slug, "", rawPath)
		processed++
	}
	fmt.Fprintf(os.Stderr, "[extract_html] %s: %d pages\n", m.Slug, processed)
	return nil
}
