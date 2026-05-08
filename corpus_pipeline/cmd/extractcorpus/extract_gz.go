package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
)

// extractGZ handles plain text gzip files (the OPUS .txt.gz format).
// One sentence per line in the input. We pass them through to text.txt
// with blank lines marking document boundaries — for OPUS we don't have
// document structure, so we group into pseudo-documents of N lines each
// for sentence_ix locality.
func extractGZ(dir string, m sources.Manifest) error {
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

	const linesPerDoc = 500 // pseudo-document size

	docIdx := 0
	totalLines := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".gz") {
			continue
		}
		rawPath := filepath.Join(rawDir, name)
		f, err := os.Open(rawPath)
		if err != nil {
			return err
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return fmt.Errorf("gzip %s: %w", name, err)
		}
		scanner := bufio.NewScanner(gz)
		scanner.Buffer(make([]byte, 1<<20), 16<<20) // up to 16MB lines
		linesInDoc := 0
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			// Synthesize a sentence-ending period if line lacks one.
			// Without this, parserffi can't sentence-split EU legal chemistry
			// text and emits one 100KB "sentence" per pseudo-doc.
			if last := line[len(line)-1]; last != '.' && last != '!' && last != '?' && last != ':' {
				line += "."
			}
			textW.WriteString(line)
			textW.WriteByte('\n')
			linesInDoc++
			totalLines++
			if linesInDoc >= linesPerDoc {
				textW.WriteByte('\n') // doc boundary
				docID := fmt.Sprintf("%s:%s:doc-%d", m.Slug, slugify(strings.TrimSuffix(name, ".gz")), docIdx)
				writeDocJSONL(docsW, docID, "", "", rawPath)
				docIdx++
				linesInDoc = 0
			}
		}
		// Flush partial doc
		if linesInDoc > 0 {
			textW.WriteByte('\n')
			docID := fmt.Sprintf("%s:%s:doc-%d", m.Slug, slugify(strings.TrimSuffix(name, ".gz")), docIdx)
			writeDocJSONL(docsW, docID, "", "", rawPath)
			docIdx++
		}
		gz.Close()
		f.Close()
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("scan %s: %w", name, err)
		}
	}
	fmt.Fprintf(os.Stderr, "[extract_gz] %s: %d lines across %d pseudo-docs\n", m.Slug, totalLines, docIdx)
	return nil
}
