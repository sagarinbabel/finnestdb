package starterdeck

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// LemmaKey identifies a curated example by its (lemma, pos) sense, matching
// the deck card identity that seedcolddeck seeds.
type LemmaKey struct {
	Lemma string
	POS   string
}

// Example is one curated corpus sentence for a starter-deck card: the sentence
// text plus the exact inflected surface form of the lemma that appears in it.
// Form is what seedcolddeck stores as the highlighted occurrence.
type Example struct {
	Form         string
	Sentence     string
	SourceCorpus string
}

// exampleColumns is the fixed column order of the starter-examples TSV emitted
// by cmd/pickexamples. The loader validates the header against this so a
// schema change fails loudly instead of silently mismapping columns.
var exampleColumns = []string{"lemma", "pos", "form", "sentence", "source_corpus"}

// LoadExamples reads a starter-examples TSV (see cmd/pickexamples and
// testdata/starter-examples/) and returns the first curated example per
// (lemma, pos). Rows whose lang column mismatches lang are ignored - the TSV
// has no lang column of its own, so lang is only used to build a helpful error
// if the file is empty for the requested language; callers pass the matching
// per-language artifact. When a (lemma, pos) has more than one row, the first
// (highest-ranked) example wins, which is the sentence seedcolddeck attaches.
func LoadExamples(path, lang string) (map[LemmaKey]Example, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[LemmaKey]Example)
	scanner := bufio.NewScanner(f)
	// Corpus sentences can be long; raise the line cap well above the default
	// 64 KiB so a legitimate long example is never truncated mid-row.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		fields := strings.Split(line, "\t")
		if lineNo == 1 {
			if err := validateHeader(fields); err != nil {
				return nil, err
			}
			continue
		}
		if len(fields) != len(exampleColumns) {
			return nil, fmt.Errorf("%s line %d: got %d columns, want %d", path, lineNo, len(fields), len(exampleColumns))
		}
		key := LemmaKey{Lemma: fields[0], POS: fields[1]}
		if key.Lemma == "" || key.POS == "" {
			continue
		}
		if _, exists := out[key]; exists {
			// First row per lemma wins (the artifact is emitted best-first).
			continue
		}
		out[key] = Example{Form: fields[2], Sentence: fields[3], SourceCorpus: fields[4]}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no examples for lang %s (empty or header-only)", path, lang)
	}
	return out, nil
}

func validateHeader(fields []string) error {
	if len(fields) != len(exampleColumns) {
		return fmt.Errorf("bad header: got %d columns, want %d (%s)", len(fields), len(exampleColumns), strings.Join(exampleColumns, ", "))
	}
	for i, want := range exampleColumns {
		if fields[i] != want {
			return fmt.Errorf("bad header column %d: got %q, want %q", i, fields[i], want)
		}
	}
	return nil
}

// FormTokenIndex returns the 0-based word position of form within sentence,
// or -1 if it is not present as a whole word. "Word" splitting mirrors the
// deck occurrence model: whitespace-delimited tokens with leading/trailing
// punctuation trimmed, compared case-insensitively so a sentence-capitalized
// form still matches its lowercase surface. The index is used only to give the
// occurrence a stable, unique token_ix; the review highlight matches by
// surface string, so exactness of the index does not affect rendering.
func FormTokenIndex(sentence, form string) int {
	target := strings.ToLower(strings.TrimFunc(form, isTrimPunct))
	if target == "" {
		return -1
	}
	ix := 0
	for _, tok := range strings.Fields(sentence) {
		word := strings.ToLower(strings.TrimFunc(tok, isTrimPunct))
		if word == target {
			return ix
		}
		ix++
	}
	return -1
}

func isTrimPunct(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}
