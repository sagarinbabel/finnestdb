// cmd/parser-compare reads one or more parser-eval report JSON files and emits
// a markdown comparison table to stdout. Designed to be paired with cmd/parsertest
// to produce side-by-side parser comparisons (custom vs analyzer baseline).
//
// Default report shape (no -baseline-dir):
//
//	One row per (dataset, parser) showing absolute lemma/POS/grammar/coverage
//	numbers — the legacy "horizontal" view, useful for "see all parsers at once."
//
// With -baseline-dir <path>, each "now" report is paired with the matching
// prior report (by dataset name) under -baseline-dir, and the headline becomes
// a (custom-prev, custom-now, Δ, analyzer) table — the format the team
// requested 2026-05-07 so reports always answer "did our changes regress
// against the analyzer upper bound?" The legacy view is still printed below
// as an appendix.
//
// With -bootstrap N (default 1000), 95% case-level bootstrap confidence
// intervals are appended to each accuracy cell as ±halfwidth. Set -bootstrap 0
// to disable. CIs prevent reading "82.3% beats 80.1%" as significant when
// it's noise on a 22-case manual set.
//
// Usage:
//
//	# Generate reports first via cmd/parsertest:
//	go run ./cmd/parsertest -dataset DS.json -parsers basic,custom,omorfi
//
//	# Default headline (no baseline):
//	go run ./cmd/parser-compare reports/parser-eval/*-fi-grammar.json > comparison.md
//
//	# 3-column "before / after / analyzer" headline:
//	go run ./cmd/parser-compare \
//	    -baseline-dir docs/baselines/2026-05-06b/ \
//	    reports/parser-eval/20260507T*.json > comparison.md
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"finnestdb/internal/eval"
)

const (
	defaultMainParser     = "custom"
	defaultBaselineParser = "basic"
	bootstrapAlpha        = 0.05 // 95% CI
)

func main() {
	title := flag.String("title", "Parser Comparison", "Markdown H1 title")
	baselineDir := flag.String("baseline-dir", "", "Directory containing prior parsertest JSON reports. When set, each report is paired by dataset name and the headline becomes (custom-prev, custom-now, Δ, analyzer).")
	mainParser := flag.String("main-parser", defaultMainParser, "Parser name treated as 'now' in the headline")
	bootstrapN := flag.Int("bootstrap", 1000, "Bootstrap iterations for 95% case-level CIs. 0 disables CIs.")
	bootstrapSeed := flag.Int64("bootstrap-seed", 0xF1E57DB, "RNG seed for the bootstrap (deterministic by default).")
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "usage: parser-compare [-title T] [-baseline-dir D] [-main-parser custom] [-bootstrap N] report1.json [report2.json ...]")
		os.Exit(2)
	}

	now, err := loadReports(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sort.Slice(now, func(i, j int) bool { return now[i].Dataset.Name < now[j].Dataset.Name })

	var prev map[string]*eval.Report
	if *baselineDir != "" {
		prev, err = loadBaselineDir(*baselineDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load -baseline-dir %s: %v\n", *baselineDir, err)
			os.Exit(1)
		}
	}

	rng := rand.New(rand.NewSource(*bootstrapSeed))

	fmt.Printf("# %s\n\n", *title)

	if *baselineDir != "" {
		emitBeforeAfterTable(now, prev, *mainParser, *bootstrapN, rng)
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
		fmt.Println("## Appendix — full per-parser table")
		fmt.Println()
	}
	emitLegacyTable(now)
}

// loadReports reads each JSON path into an eval.Report.
func loadReports(paths []string) ([]*eval.Report, error) {
	reports := make([]*eval.Report, 0, len(paths))
	for _, p := range paths {
		r, err := loadReport(p)
		if err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, nil
}

// loadReport reads a single JSON path. Errors are wrapped with the path so
// failures point at the offending file.
func loadReport(path string) (*eval.Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var r eval.Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &r, nil
}

// loadBaselineDir scans a directory for JSON reports and returns them keyed
// by dataset name. Multiple files for the same dataset name resolve by the
// report's generated_at timestamp, with filename as a deterministic fallback.
// Avoid filesystem mtimes: git checkouts do not preserve historical report
// creation times.
func loadBaselineDir(dir string) (map[string]*eval.Report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*eval.Report)
	selectedKeys := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		r, err := loadReport(path)
		if err != nil {
			// Tolerate non-report JSON in the same directory (e.g. raw
			// dataset files). Skip silently.
			continue
		}
		if r.Dataset.Name == "" {
			continue
		}
		key := baselineSelectionKey(r, e.Name())
		if existingKey, ok := selectedKeys[r.Dataset.Name]; ok && key <= existingKey {
			continue
		}
		out[r.Dataset.Name] = r
		selectedKeys[r.Dataset.Name] = key
	}
	return out, nil
}

func baselineSelectionKey(r *eval.Report, filename string) string {
	if r.Generated != "" {
		return r.Generated + "\x00" + filename
	}
	return "\x00" + filename
}

// emitBeforeAfterTable renders the headline (Dataset, Metric, custom-prev,
// custom-now, Δ, analyzer) table, plus an entry-marker for missing baselines
// or analyzers.
func emitBeforeAfterTable(now []*eval.Report, prev map[string]*eval.Report, mainParser string, bootstrap int, rng *rand.Rand) {
	fmt.Printf("## Headline — %s before/after, with analyzer upper bound\n\n", mainParser)
	fmt.Println("Δ shows percentage-point change in the main parser since the prior report.")
	if bootstrap > 0 {
		fmt.Printf("Accuracy cells include 95%% case-level bootstrap CIs (B=%d, deterministic seed).\n", bootstrap)
	}
	fmt.Println()
	fmt.Printf("| Dataset | Cases | Metric | %s-prev | %s-now | Δ | analyzer |\n", mainParser, mainParser)
	fmt.Println("|---|---:|---|---:|---:|---:|---:|")

	for _, nowR := range now {
		analyzer := analyzerParserFor(nowR)
		ds := nowR.Dataset.Name
		cases := nowR.Dataset.CaseCount
		prevR := prev[ds]

		for _, m := range []metric{metricLemma, metricPOS, metricGrammar, metricCoverage} {
			nowVal := summaryMetric(nowR.Summary[mainParser], m)
			prevVal := math.NaN()
			if prevR != nil {
				prevVal = summaryMetric(prevR.Summary[mainParser], m)
			}
			analyzerVal := math.NaN()
			if analyzer != "" {
				analyzerVal = summaryMetric(nowR.Summary[analyzer], m)
			}

			nowCell := formatAccuracyWithCI(nowVal, nowR, mainParser, m, bootstrap, rng)
			prevCell := "—"
			if prevR != nil {
				prevCell = formatAccuracyWithCI(prevVal, prevR, mainParser, m, bootstrap, rng)
			}
			delta := "—"
			if !math.IsNaN(nowVal) && !math.IsNaN(prevVal) {
				delta = fmt.Sprintf("%+.1f", (nowVal-prevVal)*100)
			}
			analyzerCell := "—"
			if !math.IsNaN(analyzerVal) {
				analyzerCell = formatAccuracyWithCI(analyzerVal, nowR, analyzer, m, bootstrap, rng)
			}

			fmt.Printf("| %s | %d | %s | %s | %s | %s | %s |\n",
				ds, cases, m.label, prevCell, nowCell, delta, analyzerCell)
		}
	}
}

// emitLegacyTable is the original "all parsers across all datasets" table,
// retained as an appendix when -baseline-dir is set, and as the default
// output when -baseline-dir is not set (back-compat).
func emitLegacyTable(reports []*eval.Report) {
	fmt.Println("| Dataset | Cases | Parser | Lemma | POS | Grammar | Full | Coverage | Avg ms |")
	fmt.Println("|---|---:|---|---:|---:|---:|---:|---:|---:|")

	for _, r := range reports {
		for _, parser := range r.Parsers {
			s, ok := r.Summary[parser]
			if !ok {
				continue
			}
			fmt.Printf("| %s | %d | %s | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f |\n",
				r.Dataset.Name, r.Dataset.CaseCount, parser,
				s.LemmaAccuracy*100, s.POSAccuracy*100, s.GrammarAccuracy*100,
				s.FullAccuracy*100, s.ResolvedCoverage*100, s.AvgCaseDurationMs)
		}
	}

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
				r.Dataset.Name, parser, r.Parsers[0],
				(s.LemmaAccuracy-base.LemmaAccuracy)*100,
				(s.POSAccuracy-base.POSAccuracy)*100,
				(s.ResolvedCoverage-base.ResolvedCoverage)*100,
			)
		}
	}
}

// analyzerParserFor returns the name of the analyzer baseline parser in this
// report (omorfi for FI, estnltk for ET), or "" if neither is present.
func analyzerParserFor(r *eval.Report) string {
	for _, p := range r.Parsers {
		if p == "omorfi" || p == "estnltk" {
			return p
		}
	}
	return ""
}

// metric is a small value-typed handle for "which accuracy field am I
// looking at." Lets the bootstrap loop iterate metric kinds without a switch
// at every call site.
type metric struct {
	label string
	read  func(eval.ParserSummary) float64
}

var (
	metricLemma    = metric{label: "Lemma", read: func(s eval.ParserSummary) float64 { return s.LemmaAccuracy }}
	metricPOS      = metric{label: "POS", read: func(s eval.ParserSummary) float64 { return s.POSAccuracy }}
	metricGrammar  = metric{label: "Grammar", read: func(s eval.ParserSummary) float64 { return s.GrammarAccuracy }}
	metricCoverage = metric{label: "Coverage", read: func(s eval.ParserSummary) float64 { return s.ResolvedCoverage }}
)

func summaryMetric(s eval.ParserSummary, m metric) float64 {
	v := m.read(s)
	if math.IsNaN(v) {
		return math.NaN()
	}
	return v
}

// formatAccuracyWithCI returns "82.3% ±0.4" when bootstrap > 0, else "82.3%".
// Returns "—" for NaN. CIs are computed from per-case bootstrap resampling
// of the report's case-level outcomes.
func formatAccuracyWithCI(value float64, r *eval.Report, parser string, m metric, bootstrap int, rng *rand.Rand) string {
	if math.IsNaN(value) {
		return "—"
	}
	if bootstrap <= 0 || r == nil || parser == "" {
		return fmt.Sprintf("%.1f%%", value*100)
	}
	half := bootstrapHalfWidth(r, parser, m, bootstrap, rng)
	if math.IsNaN(half) {
		return fmt.Sprintf("%.1f%%", value*100)
	}
	return fmt.Sprintf("%.1f%% ±%.1f", value*100, half*100)
}

// bootstrapHalfWidth resamples cases with replacement and returns half the
// 95% CI width on the requested metric. Returns NaN when the report has no
// usable cases for the metric (e.g. grammarEligible == 0).
func bootstrapHalfWidth(r *eval.Report, parser string, m metric, B int, rng *rand.Rand) float64 {
	stats := perCaseStats(r, parser)
	n := len(stats)
	if n == 0 {
		return math.NaN()
	}
	values := make([]float64, 0, B)
	for b := 0; b < B; b++ {
		var num, den int
		for k := 0; k < n; k++ {
			i := rng.Intn(n)
			n2, d2 := metricNumDen(stats[i], m)
			num += n2
			den += d2
		}
		if den == 0 {
			continue
		}
		values = append(values, float64(num)/float64(den))
	}
	if len(values) < 2 {
		return math.NaN()
	}
	sort.Float64s(values)
	lo := values[int(float64(len(values))*bootstrapAlpha/2)]
	hi := values[int(float64(len(values))*(1-bootstrapAlpha/2))-1]
	return (hi - lo) / 2
}

// caseStats holds per-case correct/eligible counts for each metric. Pulled
// out of the bootstrap loop so we don't re-scan token comparisons B times.
type caseStats struct {
	lemmaCorrect    int
	posCorrect      int
	grammarCorrect  int
	grammarEligible int
	fullCorrect     int
	expectedTokens  int
	resolvedTokens  int
	totalTokens     int
}

// perCaseStats walks one parser's comparisons and aggregates per-case counts
// the bootstrap loop will resample over.
func perCaseStats(r *eval.Report, parser string) []caseStats {
	out := make([]caseStats, 0, len(r.Cases))
	for _, c := range r.Cases {
		cmps, ok := c.Comparisons[parser]
		if !ok {
			continue
		}
		var s caseStats
		for _, cmp := range cmps {
			s.expectedTokens++
			if cmp.Match.Lemma {
				s.lemmaCorrect++
			}
			if cmp.Match.POS {
				s.posCorrect++
			}
			if cmp.Expected.GrammarLabel != "" {
				s.grammarEligible++
				if cmp.Match.Grammar {
					s.grammarCorrect++
				}
			}
			if cmp.Match.Full {
				s.fullCorrect++
			}
			if cmp.Actual.Resolved {
				s.resolvedTokens++
			}
			s.totalTokens++
		}
		out = append(out, s)
	}
	return out
}

// metricNumDen returns (numerator, denominator) for the given metric on
// one case's stats. Used by the bootstrap to compute resampled accuracy.
func metricNumDen(s caseStats, m metric) (int, int) {
	switch m.label {
	case "Lemma":
		return s.lemmaCorrect, s.expectedTokens
	case "POS":
		return s.posCorrect, s.expectedTokens
	case "Grammar":
		return s.grammarCorrect, s.grammarEligible
	case "Full":
		return s.fullCorrect, s.expectedTokens
	case "Coverage":
		return s.resolvedTokens, s.totalTokens
	}
	return 0, 0
}
