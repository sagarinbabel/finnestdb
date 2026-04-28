// cmd/parser-compare reads one or more parser-eval report JSON files and emits
// a markdown comparison table to stdout. Designed to be paired with cmd/parsertest
// to produce side-by-side parser comparisons (e.g. basic vs custom vs omorfi).
//
// Usage:
//
//	# Generate reports first via cmd/parsertest:
//	go run ./cmd/parsertest -dataset DS.json -parsers basic,custom,omorfi
//	# Then assemble the markdown:
//	go run ./cmd/parser-compare reports/parser-eval/*-fi-grammar.json > comparison.md
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"finnestdb/internal/eval"
)

func main() {
	title := flag.String("title", "Parser Comparison", "Markdown H1 title")
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "usage: parser-compare [-title TITLE] report1.json [report2.json ...]")
		os.Exit(2)
	}

	reports := make([]*eval.Report, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", p, err)
			os.Exit(1)
		}
		var r eval.Report
		if err := json.Unmarshal(data, &r); err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", p, err)
			os.Exit(1)
		}
		reports = append(reports, &r)
	}

	// Stable order: by dataset name.
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Dataset.Name < reports[j].Dataset.Name
	})

	fmt.Printf("# %s\n\n", *title)
	fmt.Println("| Dataset | Cases | Parser | Lemma | POS | Grammar | Full | Coverage | Avg ms |")
	fmt.Println("|---|---:|---|---:|---:|---:|---:|---:|---:|")

	for _, r := range reports {
		// Stable parser order matches the order recorded in the report.
		for _, parser := range r.Parsers {
			s, ok := r.Summary[parser]
			if !ok {
				continue
			}
			fmt.Printf("| %s | %d | %s | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f |\n",
				r.Dataset.Name,
				r.Dataset.CaseCount,
				parser,
				s.LemmaAccuracy*100,
				s.POSAccuracy*100,
				s.GrammarAccuracy*100,
				s.FullAccuracy*100,
				s.ResolvedCoverage*100,
				s.AvgCaseDurationMs,
			)
		}
	}

	// Highlight head-to-head deltas when more than one parser ran on the same dataset.
	fmt.Println()
	fmt.Println("## Head-to-head deltas (parser - first parser, in pts)")
	fmt.Println()
	fmt.Println("| Dataset | Parser | Δ Lemma | Δ POS | Δ Coverage |")
	fmt.Println("|---|---|---:|---:|---:|")
	for _, r := range reports {
		if len(r.Parsers) < 2 {
			continue
		}
		base, ok := r.Summary[r.Parsers[0]]
		if !ok {
			continue
		}
		for _, parser := range r.Parsers[1:] {
			s, ok := r.Summary[parser]
			if !ok {
				continue
			}
			fmt.Printf("| %s | %s vs %s | %+.1f | %+.1f | %+.1f |\n",
				r.Dataset.Name,
				parser, r.Parsers[0],
				(s.LemmaAccuracy-base.LemmaAccuracy)*100,
				(s.POSAccuracy-base.POSAccuracy)*100,
				(s.ResolvedCoverage-base.ResolvedCoverage)*100,
			)
		}
	}
}
