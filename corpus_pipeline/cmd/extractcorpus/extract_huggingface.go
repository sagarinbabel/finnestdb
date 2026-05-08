package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
)

// extractHuggingFace handles HuggingFace dataset dumps. Two formats
// supported:
//   - JSONL (.jsonl, .jsonl.gz) — one record per line
//   - Plain text (.txt, .txt.gz)
//
// For JSONL, the manifest's notes can specify which fields to use:
//   text_fields=heading,lead-in,text
// (concatenated with newlines)
//
// If notes don't specify, we pick the longest string field on the first
// row.
func extractHuggingFace(dir string, m sources.Manifest) error {
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

	textFieldsHint := extractHint(m.Notes, "text_fields")
	textFields := []string{}
	if textFieldsHint != "" {
		for _, f := range strings.Split(textFieldsHint, ",") {
			textFields = append(textFields, strings.TrimSpace(f))
		}
	}

	docIdx := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		var reader io.Reader
		f, err := os.Open(filepath.Join(rawDir, name))
		if err != nil {
			return err
		}
		if strings.HasSuffix(lower, ".gz") {
			gz, err := gzip.NewReader(f)
			if err != nil {
				f.Close()
				continue
			}
			reader = gz
			defer gz.Close()
		} else {
			reader = f
		}
		// Treat as JSONL if .jsonl extension
		isJSONL := strings.Contains(lower, ".jsonl")
		linesInDoc := 0
		if isJSONL {
			scanner := bufio.NewScanner(reader)
			scanner.Buffer(make([]byte, 1<<20), 64<<20)
			for scanner.Scan() {
				line := scanner.Bytes()
				if len(line) == 0 {
					continue
				}
				var obj map[string]any
				if err := json.Unmarshal(line, &obj); err != nil {
					continue
				}
				text := assembleHFText(obj, textFields)
				if text == "" {
					continue
				}
				if last := text[len(text)-1]; last != '.' && last != '!' && last != '?' {
					text += "."
				}
				textW.WriteString(text)
				textW.WriteByte('\n')
				linesInDoc++
				if linesInDoc >= 200 {
					textW.WriteByte('\n')
					docID := fmt.Sprintf("%s:%s:doc-%d", m.Slug, slugify(strings.TrimSuffix(name, filepath.Ext(name))), docIdx)
					writeDocJSONL(docsW, docID, "", "", filepath.Join(rawDir, name))
					docIdx++
					linesInDoc = 0
				}
			}
		} else {
			scanner := bufio.NewScanner(reader)
			scanner.Buffer(make([]byte, 1<<20), 64<<20)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				if last := line[len(line)-1]; last != '.' && last != '!' && last != '?' {
					line += "."
				}
				textW.WriteString(line)
				textW.WriteByte('\n')
				linesInDoc++
				if linesInDoc >= 500 {
					textW.WriteByte('\n')
					docID := fmt.Sprintf("%s:%s:doc-%d", m.Slug, slugify(strings.TrimSuffix(name, filepath.Ext(name))), docIdx)
					writeDocJSONL(docsW, docID, "", "", filepath.Join(rawDir, name))
					docIdx++
					linesInDoc = 0
				}
			}
		}
		if linesInDoc > 0 {
			textW.WriteByte('\n')
			docID := fmt.Sprintf("%s:%s:doc-%d", m.Slug, slugify(strings.TrimSuffix(name, filepath.Ext(name))), docIdx)
			writeDocJSONL(docsW, docID, "", "", filepath.Join(rawDir, name))
			docIdx++
		}
		f.Close()
	}
	return nil
}

func assembleHFText(obj map[string]any, fields []string) string {
	if len(fields) > 0 {
		var parts []string
		for _, f := range fields {
			if v, ok := obj[f]; ok {
				if s, ok := v.(string); ok && s != "" {
					parts = append(parts, s)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	// No hint — pick longest string field
	var bestK string
	var bestV string
	for k, v := range obj {
		if s, ok := v.(string); ok {
			if len(s) > len(bestV) {
				bestK, bestV = k, s
			}
		}
	}
	_ = bestK
	return bestV
}
