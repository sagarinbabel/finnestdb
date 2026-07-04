// cmd/ambiguityeval runs the ambiguity and meaning-check calibration eval
// slice defined in docs/PARSER_EVAL_METHODOLOGY.md §"Ambiguity and
// meaning-check calibration". For each case in a slice:"ambiguity" gold
// file, it runs parsecore.Analyze(..., "custom") for the parser's single
// pick and store.BatchLookupAllForms for the candidate set on the target
// surface, then reports candidate inclusion, selection accuracy, and
// proxy-stratified accuracy, keyed by ambiguity_class.
//
// Usage:
//
//	go run ./cmd/ambiguityeval -db finnestdb.db testdata/parser-eval/fi-ambiguity/fi-ambiguity-v1.json
//	go run ./cmd/ambiguityeval -db finnestdb.db   # discovers testdata/parser-eval/*-ambiguity/*.json
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"finnestdb/internal/store"
)

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	outPath := flag.String("out", "", "Path to write JSON report (default: reports/parser-eval/ambiguity-<run>.json)")
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		discovered, err := discoverGoldFiles(".")
		if err != nil {
			log.Fatalf("discover gold files: %v", err)
		}
		if len(discovered) == 0 {
			log.Fatal("no ambiguity gold files found under testdata/parser-eval/*-ambiguity/*.json; pass file paths explicitly")
		}
		paths = discovered
	}

	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var datasetReports []DatasetReport
	for _, path := range paths {
		dataset, err := LoadDataset(path)
		if err != nil {
			log.Fatalf("load dataset %s: %v", path, err)
		}
		results, err := EvaluateDataset(db, dataset)
		if err != nil {
			log.Fatalf("evaluate dataset %s: %v", path, err)
		}
		classes := ComputeClassMetrics(results)
		datasetReports = append(datasetReports, DatasetReport{
			Name:       dataset.Name,
			Version:    dataset.Version,
			Language:   dataset.Language,
			SourceFile: path,
			CaseCount:  len(dataset.Cases),
			Overall:    ComputeOverallMetrics(results),
			Classes:    classes,
			Proxy:      ComputeProxyMetrics(results),
			Eligible:   ThresholdEligibleClasses(classes),
		})
	}

	report := BuildReport(datasetReports, gitCommit())

	reportPath := *outPath
	if reportPath == "" {
		cwd, err := filepath.Abs(".")
		if err != nil {
			log.Fatalf("resolve cwd: %v", err)
		}
		reportPath = DefaultReportPath(cwd, report)
	}
	if err := WriteReport(reportPath, report); err != nil {
		log.Fatalf("write report: %v", err)
	}

	fmt.Printf("Report: %s\n\n", reportPath)
	PrintTable(os.Stdout, report)
}

// discoverGoldFiles globs testdata/parser-eval/*-ambiguity/*.json under
// rootDir, matching the layout described in TODO.md's "Wire
// make compare-ambiguity" item. Sorted for deterministic run order.
func discoverGoldFiles(rootDir string) ([]string, error) {
	pattern := filepath.Join(rootDir, "testdata", "parser-eval", "*-ambiguity", "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
