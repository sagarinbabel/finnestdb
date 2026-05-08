package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
)

// extractLeipzig handles Leipzig Corpora Collection .tar.gz archives.
// Each archive contains a *-sentences.txt file with one row per line
// formatted as `id<TAB>sentence`. We strip the id prefix and emit the
// sentence portion.
func extractLeipzig(dir string, m sources.Manifest) error {
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

	docIdx := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".tar.gz") && !strings.HasSuffix(lower, ".tgz") {
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
		tr := tar.NewReader(gz)
		linesInDoc := 0
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				gz.Close()
				f.Close()
				return fmt.Errorf("tar %s: %w", name, err)
			}
			if !strings.Contains(hdr.Name, "-sentences.txt") {
				continue
			}
			scanner := bufio.NewScanner(tr)
			scanner.Buffer(make([]byte, 1<<20), 16<<20)
			for scanner.Scan() {
				line := scanner.Text()
				// Strip "id\tsentence" prefix
				if tab := strings.IndexByte(line, '\t'); tab >= 0 {
					line = line[tab+1:]
				}
				line = strings.TrimSpace(line)
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
					docID := fmt.Sprintf("%s:%s:doc-%d", m.Slug, slugify(strings.TrimSuffix(name, ".tar.gz")), docIdx)
					writeDocJSONL(docsW, docID, "", "", rawPath)
					docIdx++
					linesInDoc = 0
				}
			}
		}
		if linesInDoc > 0 {
			textW.WriteByte('\n')
			docID := fmt.Sprintf("%s:%s:doc-%d", m.Slug, slugify(strings.TrimSuffix(name, ".tar.gz")), docIdx)
			writeDocJSONL(docsW, docID, "", "", rawPath)
			docIdx++
		}
		gz.Close()
		f.Close()
	}
	return nil
}
