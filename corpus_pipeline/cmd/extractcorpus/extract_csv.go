package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
)

// extractCSV handles tabular CSV/TSV sources where one column holds the
// text we want. Reads <source>/raw/*.csv (or .tsv). The manifest's notes
// can specify column name via "text_column=NAME" hint, else heuristic
// picks the longest-text column on the first data row.
//
// Used for Eduskunta parliament transcripts (speech column) etc.
func extractCSV(dir string, m sources.Manifest) error {
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

	textColHint := extractHint(m.Notes, "text_column")

	docIdx := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".csv") && !strings.HasSuffix(lower, ".tsv") {
			continue
		}
		rawPath := filepath.Join(rawDir, name)
		f, err := os.Open(rawPath)
		if err != nil {
			return err
		}
		r := csv.NewReader(f)
		if strings.HasSuffix(lower, ".tsv") {
			r.Comma = '\t'
		}
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		header, err := r.Read()
		if err != nil {
			f.Close()
			return fmt.Errorf("read header %s: %w", name, err)
		}
		colIx := pickTextColumn(header, textColHint, r)
		if colIx < 0 {
			fmt.Fprintf(os.Stderr, "[extract_csv] %s: no text column found, skipping\n", name)
			f.Close()
			continue
		}
		// Re-open since pickTextColumn consumed a row
		f.Close()
		f2, err := os.Open(rawPath)
		if err != nil {
			return err
		}
		r2 := csv.NewReader(f2)
		if strings.HasSuffix(lower, ".tsv") {
			r2.Comma = '\t'
		}
		r2.FieldsPerRecord = -1
		r2.LazyQuotes = true
		_, _ = r2.Read() // discard header
		linesInDoc := 0
		for {
			rec, err := r2.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}
			if colIx >= len(rec) {
				continue
			}
			text := strings.TrimSpace(rec[colIx])
			if text == "" {
				continue
			}
			// Synthetic period if missing
			if last := text[len(text)-1]; last != '.' && last != '!' && last != '?' {
				text += "."
			}
			textW.WriteString(text)
			textW.WriteByte('\n')
			linesInDoc++
			if linesInDoc >= 200 {
				textW.WriteByte('\n')
				docID := fmt.Sprintf("%s:%s:doc-%d", m.Slug, slugify(strings.TrimSuffix(name, filepath.Ext(name))), docIdx)
				writeDocJSONL(docsW, docID, "", "", rawPath)
				docIdx++
				linesInDoc = 0
			}
		}
		if linesInDoc > 0 {
			textW.WriteByte('\n')
			docID := fmt.Sprintf("%s:%s:doc-%d", m.Slug, slugify(strings.TrimSuffix(name, filepath.Ext(name))), docIdx)
			writeDocJSONL(docsW, docID, "", "", rawPath)
			docIdx++
		}
		f2.Close()
	}
	return nil
}

// pickTextColumn returns the column index of the most likely "text" column.
// Uses hint if provided (case-insensitive name match), else picks the
// column with the longest string in the first ~50 sample rows.
func pickTextColumn(header []string, hint string, r *csv.Reader) int {
	if hint != "" {
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), hint) {
				return i
			}
		}
	}
	// Common name match
	for i, h := range header {
		lh := strings.ToLower(strings.TrimSpace(h))
		if lh == "text" || lh == "speech" || lh == "body" || lh == "content" || lh == "puhe" || lh == "kone" {
			return i
		}
	}
	// Heuristic: sample rows, pick column with longest avg length
	avgLen := make([]int, len(header))
	for i := 0; i < 50; i++ {
		rec, err := r.Read()
		if err != nil {
			break
		}
		for j := 0; j < len(rec) && j < len(avgLen); j++ {
			avgLen[j] += len(rec[j])
		}
	}
	best, bestI := -1, -1
	for i, l := range avgLen {
		if l > best {
			best, bestI = l, i
		}
	}
	if best < 50 {
		return -1
	}
	return bestI
}

func extractHint(notes, key string) string {
	for _, line := range strings.Split(notes, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+"="))
		}
	}
	return ""
}
