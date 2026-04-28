package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	"finnestdb/internal/eval"
	"finnestdb/internal/store"
)

func main() {
	datasetPath := flag.String("dataset", "", "Path to parser evaluation dataset JSON")
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	parserList := flag.String("parsers", "basic,custom", "Comma-separated parser modes to evaluate")
	outPath := flag.String("out", "", "Path to write JSON report (default: reports/parser-eval/<run>.json)")
	warmupRuns := flag.Int("warmup", 1, "Warmup runs per case before timing")
	repeatRuns := flag.Int("repeat", 5, "Timed runs per case")
	flag.Parse()

	if *datasetPath == "" {
		log.Fatal("dataset is required")
	}

	dataset, err := eval.LoadDataset(*datasetPath)
	if err != nil {
		log.Fatalf("load dataset: %v", err)
	}

	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	parsers := parseParsers(*parserList)
	report, err := eval.Evaluate(db, dataset, eval.EvaluateOptions{
		Parsers:    parsers,
		GitCommit:  gitCommit(),
		WarmupRuns: *warmupRuns,
		RepeatRuns: *repeatRuns,
	})
	if err != nil {
		log.Fatalf("evaluate: %v", err)
	}

	reportPath := *outPath
	if reportPath == "" {
		cwd, err := filepath.Abs(".")
		if err != nil {
			log.Fatalf("resolve cwd: %v", err)
		}
		reportPath = eval.DefaultReportPath(cwd, report)
	}
	if err := eval.WriteReport(reportPath, report); err != nil {
		log.Fatalf("write report: %v", err)
	}

	fmt.Printf("Dataset: %s (%s)\n", report.Dataset.Name, report.Dataset.Language)
	fmt.Printf("Cases: %d\n", report.Dataset.CaseCount)
	fmt.Printf("Report: %s\n\n", reportPath)
	for _, parser := range report.Parsers {
		summary := report.Summary[parser]
		fmt.Printf(
			"%-6s lemma=%5.1f%% pos=%5.1f%% grammar=%5.1f%% full=%5.1f%% coverage=%5.1f%% avg=%4.1fms p50=%4.1fms p95=%4.1fms\n",
			parser,
			summary.LemmaAccuracy*100,
			summary.POSAccuracy*100,
			summary.GrammarAccuracy*100,
			summary.FullAccuracy*100,
			summary.ResolvedCoverage*100,
			summary.AvgCaseDurationMs,
			summary.P50CaseDurationMs,
			summary.P95CaseDurationMs,
		)
	}

	if len(report.PriorityRegressions) > 0 {
		fmt.Printf("\nPriority regressions (%s fails while earlier parsers pass):\n", report.Parsers[len(report.Parsers)-1])
		for _, regression := range report.PriorityRegressions {
			fmt.Printf(
				"- %s %s/%d expected=%s/%s got=%s/%s source=%s refs=%s\n",
				regression.CaseID,
				regression.Surface,
				regression.Occurrence,
				regression.Expected.Lemma,
				regression.Expected.POS,
				regression.Actual.Lemma,
				regression.Actual.POS,
				regression.Actual.Source,
				strings.Join(regression.PassingRefs, ","),
			)
		}
	}
}

func parseParsers(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
