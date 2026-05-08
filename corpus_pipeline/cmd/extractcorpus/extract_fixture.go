package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
)

type docRecord struct {
	DocumentID  string `json:"document_id"`
	Title       string `json:"title,omitempty"`
	Author      string `json:"author,omitempty"`
	RawPath     string `json:"raw_path"`
	ByteOffset  int64  `json:"byte_offset"`
}

// extractFixture is the simplest extractor — passthrough. Reads each
// raw/*.txt file and concatenates non-empty lines into text.txt with
// blank-line document boundaries. One document per raw file.
func extractFixture(dir string, m sources.Manifest) error {
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
	textW := bufio.NewWriter(textOut)
	defer textW.Flush()
	docsW := bufio.NewWriter(docsOut)
	defer docsW.Flush()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".txt") {
			continue
		}
		rawPath := filepath.Join(rawDir, name)
		data, err := os.ReadFile(rawPath)
		if err != nil {
			return err
		}
		// Document boundary: blank line. Each fixture file is one doc.
		if _, err := textW.Write(data); err != nil {
			return err
		}
		if !strings.HasSuffix(string(data), "\n") {
			textW.WriteByte('\n')
		}
		textW.WriteByte('\n') // blank line = doc boundary
		doc := docRecord{
			DocumentID: m.Slug + ":" + strings.TrimSuffix(name, filepath.Ext(name)),
			Title:      strings.TrimSuffix(name, filepath.Ext(name)),
			RawPath:    rawPath,
		}
		if err := writeJSONL(docsW, doc); err != nil {
			return err
		}
	}
	return nil
}

// extractText is the same as extractFixture for now (manual .txt sources).
func extractText(dir string, m sources.Manifest) error {
	return extractFixture(dir, m)
}

func writeJSONL(w *bufio.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	return w.WriteByte('\n')
}
