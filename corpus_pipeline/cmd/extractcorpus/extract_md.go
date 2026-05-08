package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
)

// extractMDLingQ parses LingQ parallel-text Markdown files. The format
// alternates English and Estonian sentences (often with one blank line
// between EN and ET). Heuristic: detect Estonian sentences via Estonian-
// specific characters (õ, ä, ö, ü) AND/OR by parsing the typical LingQ
// pattern of "EN sentence" → blank → "ET sentence" → blank.
//
// For v1 we use a tolerant heuristic: keep lines that contain at least one
// of {õ, ä, ö, ü} OR end with characters likely Estonian. Drop lines that
// look like pure English (no diacritics + only ASCII) and Markdown
// headers/horizontal rules.
//
// This is best-effort; if it leaves some English in, the language-ID quality
// flag in the QA report will surface it. Future: a real Estonian detector.
func extractMDLingQ(dir string, m sources.Manifest) error {
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
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, ".markdown") {
			continue
		}
		rawPath := filepath.Join(rawDir, name)
		data, err := os.ReadFile(rawPath)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		var kept []string
		for _, line := range lines {
			s := strings.TrimSpace(line)
			if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, "---") || strings.HasPrefix(s, "```") {
				continue
			}
			if !looksEstonian(s) {
				continue
			}
			kept = append(kept, s)
		}
		if len(kept) == 0 {
			continue
		}
		for _, k := range kept {
			textW.WriteString(k)
			textW.WriteByte('\n')
		}
		textW.WriteByte('\n') // doc boundary
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

// looksEstonian returns true if the line has any of the Estonian-marker
// characters or has a high enough fraction of non-ASCII letters to be
// likely-Estonian. Tolerant — better to keep an English line that slipped
// through than drop a real Estonian sentence.
func looksEstonian(s string) bool {
	for _, r := range s {
		switch r {
		case 'õ', 'Õ', 'ä', 'Ä', 'ö', 'Ö', 'ü', 'Ü':
			return true
		}
	}
	// Fallback: check for common Estonian short words. Crude but cheap.
	lower := strings.ToLower(s)
	for _, marker := range []string{" on ", " ja ", " ei ", " ma ", " ta ", " et ", " kui "} {
		if strings.Contains(" "+lower+" ", marker) {
			return true
		}
	}
	return false
}
