// cmd/silvertag tags a raw-text corpus with morphological analyses
// from pkg/lemmatizer-fi-et and emits gold-format JSON suitable for
// the parser-eval pipeline. Plan C / PR 6 — see docs/CHANGELOG.md.
//
// Source labels are "silver" (not "gold") because the annotations are
// auto-produced rather than human-checked. The lemmatizer for FI
// internally merges Voikko VFST + Giellalt HFSTOL with VFST priority;
// ET is Giellalt-only. Tokens with no FST analysis are emitted with
// empty lemma/pos/feats — the eval will count them as missed by the
// silver annotator.
//
// Usage:
//
//	go run ./cmd/silvertag \
//	    -lang FI \
//	    -input data/silver-fi/raw \
//	    -output testdata/parser-eval/fi/silver/gutenberg-fi-v1.json \
//	    -name gutenberg-fi-v1 \
//	    -version v1
//
// Output shape mirrors cmd/importud:
//
//   { "name": "...", "language": "FI", "version": "v1",
//     "cases": [
//       { "id": "gutenberg-fi-v1-7000-s1",
//         "text": "Pohjola, kotini Pohjolan...",
//         "tokens": [ { "surface": "Pohjola", "lemma": "Pohjola",
//                        "pos": "PROPN", "feats": "Case=Nom|Number=Sing" }, ...
//   ]}]}
//
// "Cases" here are sentence-sized for compatibility with the existing
// eval harness. Sentence boundaries: `.`, `!`, `?` followed by
// whitespace, or end-of-paragraph.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	lemmatizer "finnestdb/pkg/lemmatizer-fi-et"
)

func main() {
	var (
		inputDir   = flag.String("input", "data/silver-fi/raw", "Directory of raw .txt files")
		outputPath = flag.String("output", "", "Path to write the silver gold JSON")
		datasetID  = flag.String("name", "", "Dataset name for the JSON header (e.g. gutenberg-fi-silver-v1)")
		version    = flag.String("version", "v1", "Dataset version")
		lang       = flag.String("lang", "FI", "Language: FI or ET")
		maxCases   = flag.Int("max-cases", 0, "Stop after this many sentence-cases (0 = unlimited)")
	)
	flag.Parse()

	if *outputPath == "" || *datasetID == "" {
		log.Fatal("usage: silvertag -input <dir> -output <json> -name <id> [-lang FI|ET] [-version v1] [-max-cases N]")
	}
	if *lang != "FI" && *lang != "ET" {
		log.Fatalf("lang must be FI or ET, got %q", *lang)
	}

	lem, err := lemmatizer.New()
	if err != nil {
		log.Fatalf("lemmatizer: %v", err)
	}
	defer lem.Close()

	files, err := listTextFiles(*inputDir)
	if err != nil {
		log.Fatalf("list inputs: %v", err)
	}
	if len(files) == 0 {
		log.Fatalf("no .txt files under %s", *inputDir)
	}

	type silverCase struct {
		ID     string        `json:"id"`
		Text   string        `json:"text"`
		Tokens []silverToken `json:"tokens"`
	}
	var cases []silverCase
	totalTokens, taggedTokens := 0, 0

	for _, path := range files {
		bookID := strings.TrimSuffix(filepath.Base(path), ".txt")
		text, err := os.ReadFile(path)
		if err != nil {
			log.Printf("skip %s: %v", path, err)
			continue
		}
		seqIdx := 0
		for _, sentence := range splitSentences(string(text)) {
			tokens := tagSentence(lem, *lang, sentence)
			if len(tokens) == 0 {
				continue
			}
			seqIdx++
			cases = append(cases, silverCase{
				ID:     fmt.Sprintf("%s-%s-s%d", *datasetID, bookID, seqIdx),
				Text:   sentence,
				Tokens: tokens,
			})
			totalTokens += len(tokens)
			for _, t := range tokens {
				if t.Lemma != "" {
					taggedTokens++
				}
			}
			if *maxCases > 0 && len(cases) >= *maxCases {
				break
			}
		}
		if *maxCases > 0 && len(cases) >= *maxCases {
			break
		}
	}

	out := struct {
		Name     string       `json:"name"`
		Version  string       `json:"version"`
		Language string       `json:"language"`
		Cases    []silverCase `json:"cases"`
	}{
		Name:     *datasetID,
		Version:  *version,
		Language: *lang,
		Cases:    cases,
	}

	if err := writeJSON(*outputPath, out); err != nil {
		log.Fatalf("write %s: %v", *outputPath, err)
	}
	tagPct := 0.0
	if totalTokens > 0 {
		tagPct = 100 * float64(taggedTokens) / float64(totalTokens)
	}
	log.Printf("wrote %s — %d cases, %d tokens (%.1f%% tagged)",
		*outputPath, len(cases), totalTokens, tagPct)
}

// silverToken is one token row in the output. The surface always has
// a value; lemma / pos / feats may be empty if the FST didn't return
// an analysis (eval will count those as missed).
type silverToken struct {
	Surface      string `json:"surface"`
	Lemma        string `json:"lemma,omitempty"`
	POS          string `json:"pos,omitempty"`
	GrammarLabel string `json:"grammar_label,omitempty"`
	Feats        string `json:"feats,omitempty"`
}

// listTextFiles returns sorted absolute paths of .txt files under dir.
// Stable order so re-runs of silvertag produce identical output.
func listTextFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".txt") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// splitSentences breaks raw text into sentences. The split rule is the
// minimum that produces eval-shaped cases: . ! ? followed by whitespace
// or end-of-input, plus paragraph breaks (\n\n). Skips zero-length
// results.
//
// This is approximate. Real Finnish sentence boundaries include
// abbreviations like "esim." and number+period that this rule treats
// as sentence-final. Acceptable for silver because the eval treats
// each case independently — a too-aggressive split just produces
// extra short cases, not wrong tokens.
func splitSentences(text string) []string {
	if text == "" {
		return nil
	}
	var out []string
	var current strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		current.WriteRune(r)
		boundary := false
		switch r {
		case '.', '!', '?':
			next := i + 1
			for next < len(runes) && runes[next] == r {
				next++
			}
			// followed by whitespace or end-of-input → sentence break
			if next == len(runes) || unicode.IsSpace(runes[next]) {
				boundary = true
			}
		case '\n':
			// double newline = paragraph break
			if i+1 < len(runes) && runes[i+1] == '\n' {
				boundary = true
				i++ // skip the second newline
			}
		}
		if boundary {
			s := strings.TrimSpace(current.String())
			if s != "" {
				out = append(out, s)
			}
			current.Reset()
		}
	}
	tail := strings.TrimSpace(current.String())
	if tail != "" {
		out = append(out, tail)
	}
	return out
}

// tagSentence tokenizes the sentence and runs the FST on each word.
// Returns one silverToken per word-shaped run; punctuation is skipped
// (consistent with the eval harness which excludes punct from
// comparison).
func tagSentence(lem *lemmatizer.Lemmatizer, lang, sentence string) []silverToken {
	words := tokenizeWords(sentence)
	out := make([]silverToken, 0, len(words))
	for _, w := range words {
		tok := silverToken{Surface: w}
		// FST is case-sensitive; analyzers built on case-folded data
		// generally match better with lowercase input.
		analyses := lem.Lemmatize(lang, strings.ToLower(w))
		if a, ok := pickFirstNonEmpty(analyses); ok {
			tok.Lemma = a.Lemma
			tok.POS = a.UPOS
			tok.GrammarLabel = a.GrammarLabel
			tok.Feats = featsString(a)
		}
		out = append(out, tok)
	}
	return out
}

// tokenizeWords splits a sentence on whitespace and standalone
// punctuation. Hyphens inside words are preserved (e.g. "B1-tase");
// quotes / parens / brackets are dropped. Numbers stay as their own
// tokens. Returns lowercase word forms unmodified — the FST does its
// own normalization.
func tokenizeWords(text string) []string {
	var out []string
	var current strings.Builder
	flush := func() {
		w := strings.TrimSpace(current.String())
		w = trimQuotesAndPunct(w)
		if w != "" {
			out = append(out, w)
		}
		current.Reset()
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '\'' {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// trimQuotesAndPunct removes wrapping ASCII/Unicode quote and bracket
// characters that survive the letter-or-digit filter (e.g. when a
// hyphen attaches them to the word). Trim is iterative.
func trimQuotesAndPunct(w string) string {
	for {
		s := strings.Trim(w, `"'-«»‹›„""''`)
		if s == w {
			return s
		}
		w = s
	}
}

// pickFirstNonEmpty returns the first analysis with both Lemma and
// UPOS set. Same selection rule as internal/store/dict.go uses for
// the FST step — keeps silver and the live parser using the same
// criterion.
func pickFirstNonEmpty(analyses []lemmatizer.Analysis) (lemmatizer.Analysis, bool) {
	for _, a := range analyses {
		if a.Lemma == "" || a.UPOS == "" {
			continue
		}
		return a, true
	}
	return lemmatizer.Analysis{}, false
}

// featsString assembles a UD FEATS string from the lemmatizer's
// structured Analysis fields. Mirrors
// internal/store/dict.go::featsFromFSTAnalysis so silver and live
// parser FEATS are byte-identical for the same surface form.
func featsString(a lemmatizer.Analysis) string {
	var pairs []string
	if a.GrammarLabel != "" {
		if udCase, ok := caseLegacyToUD[a.GrammarLabel]; ok {
			pairs = append(pairs, "Case="+udCase)
		}
	}
	if a.Number != "" {
		pairs = append(pairs, "Number="+a.Number)
	}
	if a.Person != "" {
		pairs = append(pairs, "Person="+a.Person)
	}
	if a.Tense != "" {
		pairs = append(pairs, "Tense="+a.Tense)
	}
	if a.Mood != "" {
		pairs = append(pairs, "Mood="+a.Mood)
	}
	if len(pairs) == 0 {
		return ""
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "|")
}

// caseLegacyToUD inverts the lowercase-name → UD-code mapping.
// Duplicated here from internal/store/dict.go because cmd/silvertag
// is a separate binary and importing internal/store would pull a lot
// of database dependencies for no benefit.
var caseLegacyToUD = map[string]string{
	"nominative":  "Nom",
	"genitive":    "Gen",
	"partitive":   "Par",
	"illative":    "Ill",
	"inessive":    "Ine",
	"elative":     "Ela",
	"allative":    "All",
	"adessive":    "Ade",
	"ablative":    "Abl",
	"essive":      "Ess",
	"translative": "Tra",
	"instructive": "Ins",
	"abessive":    "Abe",
	"comitative":  "Com",
	"terminative": "Ter",
	"accusative":  "Acc",
	"vocative":    "Voc",
}

// writeJSON marshals v with two-space indentation and atomically writes
// it to path (write to .tmp, close, rename).
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
