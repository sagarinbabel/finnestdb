package main

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/sources"
)

// extractSKVR handles SKVR (Suomen Kansan Vanhat Runot) folk poetry XML.
// Routes to poems.jsonl since manifest.kind == "poetry".
//
// SKVR XML structure varies by collection but typically each <runo>
// element holds the poem, with <s> or <line> sub-elements for lines and
// <text> wrappers. We use a tolerant char-data extractor that captures
// text + line breaks and treats anything inside a "title-like" element
// as title.
func extractSKVR(dir string, m sources.Manifest) error {
	rawDir := filepath.Join(dir, "raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		return err
	}
	poemsOut, err := os.Create(filepath.Join(dir, "poems.jsonl"))
	if err != nil {
		return err
	}
	defer poemsOut.Close()
	docsOut, err := os.Create(filepath.Join(dir, "documents.jsonl"))
	if err != nil {
		return err
	}
	defer docsOut.Close()
	pW := bufio.NewWriter(poemsOut)
	defer pW.Flush()
	docsW := bufio.NewWriter(docsOut)
	defer docsW.Flush()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".xml") {
			continue
		}
		rawPath := filepath.Join(rawDir, name)
		f, err := os.Open(rawPath)
		if err != nil {
			return err
		}
		dec := xml.NewDecoder(f)
		dec.Strict = false
		dec.Entity = xml.HTMLEntity
		var (
			currentPoem *poemBuilder
			depth       int
			poemDepth   int
		)
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}
			switch t := tok.(type) {
			case xml.StartElement:
				depth++
				lname := strings.ToLower(t.Name.Local)
				if currentPoem == nil && (lname == "runo" || lname == "poem" || lname == "song" || lname == "div") {
					currentPoem = &poemBuilder{}
					poemDepth = depth
					for _, attr := range t.Attr {
						aname := strings.ToLower(attr.Name.Local)
						if aname == "id" || aname == "n" {
							currentPoem.id = attr.Value
						}
						if aname == "title" {
							currentPoem.title = attr.Value
						}
					}
				}
				if currentPoem != nil && (lname == "head" || lname == "title") {
					currentPoem.captureTitle = true
				}
				if currentPoem != nil && (lname == "l" || lname == "line" || lname == "s") {
					currentPoem.startLine()
				}
			case xml.EndElement:
				lname := strings.ToLower(t.Name.Local)
				if currentPoem != nil && (lname == "head" || lname == "title") {
					currentPoem.captureTitle = false
				}
				if currentPoem != nil && (lname == "l" || lname == "line" || lname == "s") {
					currentPoem.endLine()
				}
				if currentPoem != nil && depth == poemDepth {
					if poem := currentPoem.finalize(m.Slug, name); poem != nil {
						_ = writeJSONL(pW, poem)
						writeDocJSONL(docsW, poem.DocumentID, poem.Title, poem.Author, rawPath)
					}
					currentPoem = nil
				}
				depth--
			case xml.CharData:
				if currentPoem != nil {
					currentPoem.appendText(string(t))
				}
			}
		}
		f.Close()
	}
	return nil
}

type poemBuilder struct {
	id           string
	title        string
	author       string
	captureTitle bool
	titleBuf     strings.Builder
	lineBuf      strings.Builder
	lines        []string
}

func (p *poemBuilder) startLine() { p.lineBuf.Reset() }
func (p *poemBuilder) endLine() {
	line := strings.TrimSpace(p.lineBuf.String())
	if line != "" {
		p.lines = append(p.lines, line)
	}
	p.lineBuf.Reset()
}
func (p *poemBuilder) appendText(s string) {
	if p.captureTitle {
		p.titleBuf.WriteString(s)
		return
	}
	p.lineBuf.WriteString(s)
}

type poemRecord struct {
	DocumentID string `json:"document_id"`
	Title      string `json:"title,omitempty"`
	Author     string `json:"author,omitempty"`
	LineCount  int    `json:"line_count"`
	Text       string `json:"text"`
}

func (p *poemBuilder) finalize(sourceSlug, fileName string) *poemRecord {
	if leftover := strings.TrimSpace(p.lineBuf.String()); leftover != "" {
		p.lines = append(p.lines, leftover)
	}
	if len(p.lines) == 0 {
		return nil
	}
	if p.title == "" {
		if t := strings.TrimSpace(p.titleBuf.String()); t != "" {
			p.title = t
		}
	}
	id := p.id
	if id == "" {
		id = fmt.Sprintf("%s:auto-%s-%d", sourceSlug, slugify(strings.TrimSuffix(fileName, ".xml")), len(p.lines))
	} else {
		id = sourceSlug + ":" + id
	}
	return &poemRecord{
		DocumentID: id,
		Title:      p.title,
		Author:     p.author,
		LineCount:  len(p.lines),
		Text:       strings.Join(p.lines, "\n"),
	}
}
