package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"finnestdb/internal/parsecore"
)

// Report is the JSON report schema for one ambiguityeval run, covering
// possibly multiple gold files. Field naming follows internal/eval.Report's
// conventions (run_id, generated_at, git_commit, parser_version) so the
// report is self-describing the same way parsertest reports are, but the
// body is metrics-shaped rather than token-accuracy-shaped since ambiguity
// metrics (candidate inclusion, per-class stratification) don't fit that
// schema (see docs/PARSER_EVAL_METHODOLOGY.md §6).
type Report struct {
	RunID         string          `json:"run_id"`
	Generated     string          `json:"generated_at"`
	GitCommit     string          `json:"git_commit,omitempty"`
	ParserVersion string          `json:"parser_version,omitempty"`
	Datasets      []DatasetReport `json:"datasets"`
}

type DatasetReport struct {
	Name       string         `json:"name"`
	Version    string         `json:"version"`
	Language   string         `json:"language"`
	SourceFile string         `json:"source_file"`
	CaseCount  int            `json:"case_count"`
	Overall    OverallMetrics `json:"overall"`
	Classes    []ClassMetrics `json:"classes"`
	Proxy      []ProxyMetrics `json:"proxy_stratified"`
	Eligible   []string       `json:"threshold_eligible_classes"`
}

func BuildReport(datasetReports []DatasetReport, gitCommit string) *Report {
	return &Report{
		RunID:         time.Now().UTC().Format("20060102T150405Z"),
		Generated:     time.Now().UTC().Format(time.RFC3339),
		GitCommit:     gitCommit,
		ParserVersion: parsecore.ParserVersion,
		Datasets:      datasetReports,
	}
}

func WriteReport(path string, report *Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func DefaultReportPath(rootDir string, report *Report) string {
	return filepath.Join(rootDir, "reports", "parser-eval", "ambiguity-"+report.RunID+".json")
}

// PrintTable writes the human-readable summary table to w: per-dataset
// headline, per-class selection/inclusion table, and the proxy-stratified
// accuracy table. This is the same table shape used in the PR body per the
// task's reporting convention.
func PrintTable(w io.Writer, report *Report) {
	for _, ds := range report.Datasets {
		fmt.Fprintf(w, "## %s (%s, %s) — %d cases\n\n", ds.Name, ds.Language, ds.SourceFile, ds.CaseCount)
		fmt.Fprintf(w, "Headline: selection accuracy %d/%d = %.1f%%; candidate inclusion %d/%d = %.1f%%\n\n",
			ds.Overall.SelectionCorrect, ds.Overall.N, ds.Overall.SelectionAccuracy()*100,
			ds.Overall.CandidateIncludedCount, ds.Overall.N, ds.Overall.CandidateInclusionRate()*100,
		)

		fmt.Fprintln(w, "| class | N | sel. acc | candidate inclusion | threshold-eligible |")
		fmt.Fprintln(w, "|---|---:|---:|---:|:---:|")
		eligible := make(map[string]bool, len(ds.Eligible))
		for _, c := range ds.Eligible {
			eligible[c] = true
		}
		for _, c := range ds.Classes {
			mark := ""
			if eligible[c.Class] {
				mark = "yes"
			}
			fmt.Fprintf(w, "| %s | %d | %d/%d (%.1f%%) | %d/%d (%.1f%%) | %s |\n",
				c.Class, c.N,
				c.SelectionCorrect, c.N, c.SelectionAccuracy()*100,
				c.CandidateIncludedCount, c.N, c.CandidateInclusionRate()*100,
				mark,
			)
		}
		fmt.Fprintln(w)

		fmt.Fprintln(w, "| proxy | N | selection accuracy |")
		fmt.Fprintln(w, "|---|---:|---:|")
		for _, p := range ds.Proxy {
			fmt.Fprintf(w, "| %s | %d | %d/%d (%.1f%%) |\n", p.Proxy, p.N, p.Correct, p.N, p.Accuracy()*100)
		}
		fmt.Fprintln(w)

		if len(ds.Eligible) == 0 {
			fmt.Fprintln(w, "No classes meet the threshold rule (selection >= 90%, inclusion = 100%, N >= 4) yet.")
		} else {
			fmt.Fprintf(w, "Threshold-eligible classes: %v\n", ds.Eligible)
		}
		fmt.Fprintln(w)
	}
}
