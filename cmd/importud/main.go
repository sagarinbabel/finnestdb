// cmd/importud reads a CoNLL-U file from a Universal Dependencies treebank
// and writes our parser-eval gold JSON format. This is Plan C / PR 1: lift
// gold-set scale from ~166 cases to ~14k cases by ingesting the published
// UD treebanks for Finnish and Estonian.
//
// Source treebanks (all are CoNLL-U files from the UniversalDependencies
// org on GitHub):
//
//	UD_Finnish-TDT  — Turku Dependency Treebank.       CC BY-SA 4.0
//	UD_Finnish-FTB  — FinnTreeBank from Helsinki.      CC BY 4.0
//	UD_Finnish-PUD  — Parallel UD test set.            CC BY-SA 3.0
//	UD_Finnish-OOD  — Out-of-domain (poetry/dialogue). CC BY-SA 4.0
//	UD_Estonian-EDT — Estonian Dependency Treebank.    CC BY-NC-SA 4.0
//	UD_Estonian-EWT — Estonian Web Treebank.           CC BY-NC-SA 4.0
//
// The Estonian treebanks are NC-licensed; output JSON for those is
// gitignored (see .gitignore). Finnish treebanks are CC-BY / CC-BY-SA;
// their derived gold JSON is committed to the repo.
//
// Usage:
//
//	go run ./cmd/importud \
//	    -input data/ud-cache/UD_Finnish-TDT/fi_tdt-ud-test.conllu \
//	    -lang FI \
//	    -name ud-fi-tdt-test-v1 \
//	    -version v1 \
//	    -output testdata/parser-eval/fi/gold/ud-fi-tdt-test-v1.json
//
// CoNLL-U format reference:
// https://universaldependencies.org/format.html
//
// Each non-comment line is one token with 10 tab-separated fields:
//
//	1. ID       — "1", "2", ... or "1-2" for multi-word tokens or "1.1" for empty nodes
//	2. FORM     — surface form
//	3. LEMMA    — canonical form
//	4. UPOS     — universal POS tag
//	5. XPOS     — language-specific POS (we ignore this — UPOS is sufficient)
//	6. FEATS    — morphological features as Attr=Val|Attr=Val (or "_" for none)
//	7-10        — dependency parse fields (we ignore these)
//
// We emit one gold case per CoNLL-U sentence, skipping:
//
//   - MWT rows (ID with "-"). MWT *child* rows that follow are kept.
//   - Empty/elliptical nodes (ID with ".").
//   - Sentences whose # text = line is missing AND whose tokens reconstruct
//     to a degenerate text (rare; survives as a sanity check).
//
// FEATS handling:
//
//   - The full FEATS string is preserved in a "feats" field on each token
//     for forward-compat with the planned per-attribute eval. Existing
//     eval code (internal/eval/eval.go::ExpectedTokenRef) ignores unknown
//     JSON fields, so this is a non-breaking addition.
//   - For back-compat with the existing case-only grammar metric, we also
//     project Case=Xxx into the legacy "grammar_label" field using the
//     udCaseToLabel map below. Tokens with no Case attribute (or Nominative
//     — left implicit per existing convention) get an empty grammar_label.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	var (
		inputPath  = flag.String("input", "", "Path to CoNLL-U file")
		outputPath = flag.String("output", "", "Path to write gold JSON to")
		datasetID  = flag.String("name", "", "Dataset name for the JSON header (e.g. ud-fi-tdt-test-v1)")
		version    = flag.String("version", "v1", "Dataset version string")
		lang       = flag.String("lang", "", "Language code: FI or ET")
		idPrefix   = flag.String("id-prefix", "", "Prefix for case IDs (defaults to dataset name + '-')")
	)
	flag.Parse()

	if *inputPath == "" || *outputPath == "" || *datasetID == "" || *lang == "" {
		log.Fatal("usage: importud -input <conllu> -output <json> -name <id> -lang FI|ET [-version v1] [-id-prefix prefix]")
	}
	if *lang != "FI" && *lang != "ET" {
		log.Fatalf("lang must be FI or ET, got %q", *lang)
	}

	prefix := *idPrefix
	if prefix == "" {
		prefix = *datasetID + "-"
	}

	in, err := os.Open(*inputPath)
	if err != nil {
		log.Fatalf("open %s: %v", *inputPath, err)
	}
	defer in.Close()

	cases, totalTokens, skippedSentences, err := parseConllu(in, prefix)
	if err != nil {
		log.Fatalf("parse %s: %v", *inputPath, err)
	}

	out := struct {
		Name     string   `json:"name"`
		Version  string   `json:"version"`
		Language string   `json:"language"`
		Cases    []udCase `json:"cases"`
	}{
		Name:     *datasetID,
		Version:  *version,
		Language: *lang,
		Cases:    cases,
	}

	if err := writeJSON(*outputPath, out); err != nil {
		log.Fatalf("write %s: %v", *outputPath, err)
	}

	log.Printf("wrote %s — %d cases, %d tokens (skipped %d malformed sentences)",
		*outputPath, len(cases), totalTokens, skippedSentences)
}

type udCase struct {
	ID     string    `json:"id"`
	Text   string    `json:"text"`
	Tokens []udToken `json:"tokens"`
}

type udToken struct {
	Surface      string `json:"surface"`
	Lemma        string `json:"lemma"`
	POS          string `json:"pos"`
	GrammarLabel string `json:"grammar_label,omitempty"`
	Feats        string `json:"feats,omitempty"`
}

// parseConllu reads CoNLL-U from r, splits into sentences, and emits
// gold cases for our parser-eval format. Returns (cases, total_tokens,
// skipped_sentences, error).
func parseConllu(r *os.File, idPrefix string) ([]udCase, int, int, error) {
	scanner := bufio.NewScanner(r)
	// Some UD files have very long sentences; bump the buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		cases       []udCase
		totalTokens int
		skipped     int
	)

	var (
		sentText   string
		sentID     string
		sentTokens []udToken
		sawError   bool
		seqIndex   int
	)
	flush := func() {
		defer func() {
			sentText = ""
			sentID = ""
			sentTokens = nil
			sawError = false
		}()
		if len(sentTokens) == 0 {
			return
		}
		if sawError {
			skipped++
			return
		}
		seqIndex++
		caseID := sentID
		if caseID == "" {
			caseID = fmt.Sprintf("%d", seqIndex)
		}
		caseID = idPrefix + caseID
		text := sentText
		if text == "" {
			text = reconstructText(sentTokens)
		}
		cases = append(cases, udCase{
			ID:     caseID,
			Text:   text,
			Tokens: sentTokens,
		})
		totalTokens += len(sentTokens)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "#") {
			parseSentenceComment(line, &sentID, &sentText)
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 6 {
			sawError = true
			continue
		}
		id := fields[0]
		// Skip multi-word-token range rows (e.g. "1-2 don't") — the *child*
		// rows that follow carry the morphology we want.
		if strings.Contains(id, "-") {
			continue
		}
		// Skip empty/elliptical nodes (e.g. "1.1") — they have no surface.
		if strings.Contains(id, ".") {
			continue
		}
		surface := fields[1]
		lemma := fields[2]
		upos := fields[3]
		feats := fields[5]
		if surface == "_" || lemma == "_" || upos == "_" {
			sawError = true
			continue
		}
		featsClean := ""
		if feats != "_" && feats != "" {
			featsClean = feats
		}
		tok := udToken{
			Surface:      surface,
			Lemma:        lemma,
			POS:          upos,
			GrammarLabel: caseLabelFromFeats(featsClean),
			Feats:        featsClean,
		}
		sentTokens = append(sentTokens, tok)
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, 0, fmt.Errorf("scan: %w", err)
	}
	flush()
	return cases, totalTokens, skipped, nil
}

// parseSentenceComment extracts sent_id and text from a "# key = value"
// comment line. Mutates the receiving variables in place; lines that
// don't match a known key are ignored.
func parseSentenceComment(line string, sentID, sentText *string) {
	// Strip "#" and surrounding whitespace.
	body := strings.TrimSpace(strings.TrimPrefix(line, "#"))
	eq := strings.Index(body, "=")
	if eq < 0 {
		return
	}
	key := strings.TrimSpace(body[:eq])
	val := strings.TrimSpace(body[eq+1:])
	switch key {
	case "sent_id":
		*sentID = val
	case "text":
		*sentText = val
	}
}

// reconstructText concatenates token surfaces with spaces, used as a fallback
// when "# text =" is missing. Real UD files almost always supply text
// directly; this exists for resilience.
func reconstructText(tokens []udToken) string {
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = t.Surface
	}
	return strings.Join(parts, " ")
}

// udCaseToLabel maps UD Case= values to the lowercase English case names
// used in our existing gold sets. Nominative is intentionally absent — the
// existing convention is to leave grammar_label empty for nominative
// (e.g. fi-grammar-v1 has many bare-noun cases without grammar_label).
//
// Nominal cases listed here cover everything UD uses for FI + ET. If a Case
// value not in this map appears, we leave grammar_label empty rather than
// invent a label.
var udCaseToLabel = map[string]string{
	"Gen": "genitive",
	"Par": "partitive",
	"Ill": "illative",
	"Ine": "inessive",
	"Ela": "elative",
	"All": "allative",
	"Ade": "adessive",
	"Abl": "ablative",
	"Ess": "essive",
	"Tra": "translative",
	"Ins": "instructive",
	"Abe": "abessive",
	"Com": "comitative",
	"Ter": "terminative",
	"Acc": "accusative",
	"Voc": "vocative",
}

// caseLabelFromFeats projects a UD FEATS string to a single grammar_label
// string using the legacy case-only convention. Returns "" when:
//   - FEATS is empty, or
//   - FEATS has no Case= attribute, or
//   - The Case= value is Nominative (left implicit per existing convention),
//   - The Case= value is unknown to udCaseToLabel.
func caseLabelFromFeats(feats string) string {
	if feats == "" {
		return ""
	}
	for _, pair := range strings.Split(feats, "|") {
		eq := strings.Index(pair, "=")
		if eq < 0 {
			continue
		}
		if pair[:eq] != "Case" {
			continue
		}
		val := pair[eq+1:]
		// Nominative left as empty grammar_label per existing gold convention.
		if val == "Nom" {
			return ""
		}
		if label, ok := udCaseToLabel[val]; ok {
			return label
		}
		return ""
	}
	return ""
}

// writeJSON marshals v with two-space indentation and atomically writes it
// to path (write to .tmp, fsync-equivalent close, rename) so a crash midway
// through doesn't leave a half-written gold file lying around.
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(parentDir(path), 0o755); err != nil {
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

func parentDir(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "."
	}
	return path[:i]
}
