package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/store"
)

type tokenSummary struct {
	Lemma        string `json:"lemma"`
	POS          string `json:"pos"`
	GrammarLabel string `json:"grammar_label,omitempty"`
	Source       string `json:"source"`
	Resolved     bool   `json:"resolved"`
}

type tokenDiff struct {
	Index  int          `json:"index"`
	Form   string       `json:"form"`
	Basic  tokenSummary `json:"basic"`
	Custom tokenSummary `json:"custom"`
	Reason []string     `json:"reason"`
}

type candidate struct {
	File             string      `json:"file"`
	SentenceIndex    int         `json:"sentence_index"`
	Sentence         string      `json:"sentence"`
	Score            int         `json:"score"`
	DiffCount        int         `json:"diff_count"`
	ImprovementCount int         `json:"improvement_count"`
	RegressionCount  int         `json:"regression_count"`
	Differences      []tokenDiff `json:"differences"`
}

type report struct {
	InputDir   string      `json:"input_dir"`
	Language   string      `json:"language"`
	FileCount  int         `json:"file_count"`
	Candidates []candidate `json:"candidates"`
}

func main() {
	inputDir := flag.String("input-dir", "", "Directory of cleaned corpus .txt files")
	lang := flag.String("lang", "FI", "Language code")
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	outJSON := flag.String("out-json", "", "Path to write JSON candidate report")
	outTSV := flag.String("out-tsv", "", "Path to write TSV summary")
	topN := flag.Int("top", 200, "Maximum number of top candidates to keep")
	flag.Parse()

	if *inputDir == "" {
		log.Fatal("input-dir is required")
	}

	files, err := filepath.Glob(filepath.Join(*inputDir, "*.txt"))
	if err != nil {
		log.Fatalf("glob input files: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		log.Fatalf("no .txt files found in %s", *inputDir)
	}

	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var candidates []candidate
	for _, path := range files {
		textBytes, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("read %s: %v", path, err)
		}
		text := strings.TrimSpace(string(textBytes))
		if text == "" {
			continue
		}

		basic, err := parsecore.Analyze(db, *lang, text, "basic")
		if err != nil {
			log.Fatalf("parse basic %s: %v", path, err)
		}
		custom, err := parsecore.Analyze(db, *lang, text, "custom")
		if err != nil {
			log.Fatalf("parse custom %s: %v", path, err)
		}

		candidates = append(candidates, compareFile(path, basic, custom)...)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].ImprovementCount != candidates[j].ImprovementCount {
			return candidates[i].ImprovementCount > candidates[j].ImprovementCount
		}
		if candidates[i].DiffCount != candidates[j].DiffCount {
			return candidates[i].DiffCount > candidates[j].DiffCount
		}
		if candidates[i].File != candidates[j].File {
			return candidates[i].File < candidates[j].File
		}
		return candidates[i].SentenceIndex < candidates[j].SentenceIndex
	})

	if *topN > 0 && len(candidates) > *topN {
		candidates = candidates[:*topN]
	}

	rep := report{
		InputDir:   *inputDir,
		Language:   *lang,
		FileCount:  len(files),
		Candidates: candidates,
	}

	if *outJSON != "" {
		if err := writeJSON(*outJSON, rep); err != nil {
			log.Fatalf("write json: %v", err)
		}
	}
	if *outTSV != "" {
		if err := writeTSV(*outTSV, candidates); err != nil {
			log.Fatalf("write tsv: %v", err)
		}
	}

	fmt.Printf("Files: %d\n", len(files))
	fmt.Printf("Candidates: %d\n", len(candidates))
	if *outJSON != "" {
		fmt.Printf("JSON: %s\n", *outJSON)
	}
	if *outTSV != "" {
		fmt.Printf("TSV: %s\n", *outTSV)
	}
}

func compareFile(path string, basic, custom *parsecore.ParseResult) []candidate {
	count := len(basic.Sentences)
	if len(custom.Sentences) < count {
		count = len(custom.Sentences)
	}

	out := make([]candidate, 0, count)
	for i := 0; i < count; i++ {
		diffs, score, improvements, regressions := compareSentence(basic.Sentences[i], custom.Sentences[i])
		if len(diffs) == 0 {
			continue
		}
		out = append(out, candidate{
			File:             path,
			SentenceIndex:    i,
			Sentence:         strings.TrimSpace(custom.Sentences[i].Text),
			Score:            score,
			DiffCount:        len(diffs),
			ImprovementCount: improvements,
			RegressionCount:  regressions,
			Differences:      diffs,
		})
	}
	return out
}

func compareSentence(basic, custom parsecore.SentenceResult) ([]tokenDiff, int, int, int) {
	count := len(basic.Tokens)
	if len(custom.Tokens) < count {
		count = len(custom.Tokens)
	}

	diffs := make([]tokenDiff, 0)
	score := 0
	improvements := 0
	regressions := 0

	for i := 0; i < count; i++ {
		bt := basic.Tokens[i]
		ct := custom.Tokens[i]
		if bt.POS == "PUNCT" && ct.POS == "PUNCT" {
			continue
		}

		var reasons []string
		tokenScore := 0
		if !bt.Resolved && ct.Resolved {
			reasons = append(reasons, "custom_resolved_basic_unresolved")
			tokenScore += 3
			improvements++
		}
		if bt.Resolved && !ct.Resolved {
			reasons = append(reasons, "basic_resolved_custom_unresolved")
			tokenScore += 3
			regressions++
		}
		if bt.Lemma != ct.Lemma {
			reasons = append(reasons, "lemma")
			tokenScore += 2
		}
		if bt.POS != ct.POS {
			reasons = append(reasons, "pos")
			tokenScore += 2
		}
		if bt.GrammarLabel != ct.GrammarLabel {
			reasons = append(reasons, "grammar")
			tokenScore++
		}
		if bt.Source != ct.Source {
			reasons = append(reasons, "source")
			tokenScore++
		}
		if len(reasons) == 0 {
			continue
		}

		score += tokenScore
		diffs = append(diffs, tokenDiff{
			Index: i,
			Form:  ct.Form,
			Basic: tokenSummary{
				Lemma:        bt.Lemma,
				POS:          bt.POS,
				GrammarLabel: bt.GrammarLabel,
				Source:       bt.Source,
				Resolved:     bt.Resolved,
			},
			Custom: tokenSummary{
				Lemma:        ct.Lemma,
				POS:          ct.POS,
				GrammarLabel: ct.GrammarLabel,
				Source:       ct.Source,
				Resolved:     ct.Resolved,
			},
			Reason: reasons,
		})
	}
	return diffs, score, improvements, regressions
}

func writeJSON(path string, rep report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func writeTSV(path string, candidates []candidate) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.Comma = '\t'
	if err := w.Write([]string{
		"file", "sentence_index", "score", "diff_count", "improvement_count", "regression_count", "sentence", "difference_forms",
	}); err != nil {
		return err
	}
	for _, c := range candidates {
		forms := make([]string, 0, len(c.Differences))
		for _, diff := range c.Differences {
			forms = append(forms, diff.Form)
		}
		if err := w.Write([]string{
			c.File,
			fmt.Sprintf("%d", c.SentenceIndex),
			fmt.Sprintf("%d", c.Score),
			fmt.Sprintf("%d", c.DiffCount),
			fmt.Sprintf("%d", c.ImprovementCount),
			fmt.Sprintf("%d", c.RegressionCount),
			c.Sentence,
			strings.Join(forms, ", "),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
