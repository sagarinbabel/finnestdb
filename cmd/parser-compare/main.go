// cmd/parser-compare reads one or more parser-eval report JSON files and emits
// a markdown comparison table to stdout. Reports may be raw .json or compressed
// .json.gz. Designed to be paired with cmd/parsertest to produce side-by-side
// parser comparisons (custom vs analyzer baseline).
//
// Default report shape (no -baseline-dir):
//
//	One row per (dataset, parser) showing absolute lemma/POS/grammar/coverage
//	numbers - the legacy "horizontal" view, useful for "see all parsers at once."
//
// With -baseline-dir <path>, each "now" report is paired with the matching
// prior report (by dataset name) under -baseline-dir, and the headline becomes
// a (custom-prev, custom-now, Δ, analyzer) table - the format the team
// requested 2026-05-07 so reports always answer "did our changes regress
// against the analyzer upper bound?" The legacy view is still printed below
// as an appendix.
//
// With -bootstrap N (default 1000), 95% case-level bootstrap confidence
// intervals are appended to each accuracy cell as ±halfwidth. Set -bootstrap 0
// to disable. CIs prevent reading "82.3% beats 80.1%" as significant when
// it's noise on a 22-case manual set.
//
// With -stratified, a UPOS-bucket / OOV / compoundness breakdown is emitted
// after the headline. Stratification is computed on the fly from the report's
// case-level comparisons, so the flag works on historical baseline JSONs
// regardless of whether they were generated with parsertest's -stratified.
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
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	stratified := flag.Bool("stratified", false, "Emit a per-(dataset, parser, bucket) breakdown by UPOS / OOV / compoundness after the headline. Computed on the fly from the report's case comparisons.")
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "usage: parser-compare [-title T] [-baseline-dir D] [-main-parser custom] [-bootstrap N] report1.json[.gz] [report2.json[.gz] ...]")
		os.Exit(2)
	}

	now, err := loadReports(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sort.Slice(now, func(i, j int) bool {
		return reportDatasetKey(now[i].Dataset) < reportDatasetKey(now[j].Dataset)
	})

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
		fmt.Println("## Appendix - full per-parser table")
		fmt.Println()
	}
	emitLegacyTable(now)
	emitFeatsAttributeTable(now)

	if *stratified {
		fmt.Println()
		emitStratifiedTables(now, prev)
	}
}

// emitStratifiedTables computes and prints the three-axis stratified breakdown
// for each (dataset, parser) pair. Computed on the fly from each report's
// case-level comparisons rather than read from Summary.Stratification, so the
// flag works on historical baseline JSONs that predate the stratified output.
//
// When prev is non-empty (-baseline-dir mode), the matching prior report for
// each dataset is also expanded into rows, with explicit "prev" / "now"
// labels so the two are visually distinct. When prev is nil but multiple now
// reports were supplied, each row carries a short timestamp label derived
// from the report's RunID - without this, two reports for the same dataset
// would render as duplicate-looking rows. Sidecars from cmd/parsertest
// always pass a single report and don't trigger label rendering.
func emitStratifiedTables(now []*eval.Report, prev map[string]*eval.Report) {
	// In baseline-dir mode, force "prev" / "now" labels so the rendered
	// table reads as a comparison rather than two anonymous rows. The
	// short-ID label is reserved for the simpler N-reports-no-baseline
	// case where the user is directly inspecting a handful of JSONs.
	useExplicitPrevNow := len(prev) > 0
	// Detect the case that the user passed multiple JSONs naming the same
	// (dataset, parser) without -baseline-dir. There we still need labels
	// to disambiguate, but the labels should be the short report IDs
	// rather than the binary "prev" / "now" pair.
	useShortIDLabels := !useExplicitPrevNow && hasDuplicateDatasetParser(now)
	nameCounts := datasetNameCounts(now)

	inputs := make([]eval.StratifiedReportInput, 0)
	for _, r := range now {
		datasetName := reportDatasetLabel(r.Dataset, nameCounts)
		for _, parser := range r.Parsers {
			strat := eval.ComputeStratification(parser, r.Cases)
			if strat == nil {
				continue
			}
			label := ""
			if useExplicitPrevNow {
				label = "now"
			} else if useShortIDLabels {
				label = eval.ReportShortLabel(r)
			}
			inputs = append(inputs, eval.StratifiedReportInput{
				DatasetName: datasetName,
				Parser:      parser,
				Label:       label,
				Stratified:  strat,
			})
		}
		if useExplicitPrevNow {
			prevReport, ok := prev[reportDatasetKey(r.Dataset)]
			if !ok {
				continue
			}
			prevDatasetName := reportDatasetLabel(prevReport.Dataset, nameCounts)
			for _, parser := range prevReport.Parsers {
				strat := eval.ComputeStratification(parser, prevReport.Cases)
				if strat == nil {
					// Older summary-only baselines have no cases to
					// re-stratify. Silently skip prev for that dataset;
					// the user still sees the now rows, which is the
					// most informative thing we can render.
					continue
				}
				inputs = append(inputs, eval.StratifiedReportInput{
					DatasetName: prevDatasetName,
					Parser:      parser,
					Label:       "prev",
					Stratified:  strat,
				})
			}
		}
	}
	inputs = eval.SortedStratifiedInputsByDatasetParser(inputs)
	eval.RenderStratifiedMarkdown(os.Stdout, "Stratified accuracy", inputs)
}

// hasDuplicateDatasetParser reports whether any (dataset, parser) pair appears
// more than once across the supplied reports - the signal that a user passed
// e.g. an old and new run for the same gold set without -baseline-dir, and
// the rendered rows must carry labels to stay distinguishable.
func hasDuplicateDatasetParser(reports []*eval.Report) bool {
	seen := make(map[string]struct{})
	for _, r := range reports {
		for _, parser := range r.Parsers {
			key := reportDatasetKey(r.Dataset) + "\x00" + parser
			if _, ok := seen[key]; ok {
				return true
			}
			seen[key] = struct{}{}
		}
	}
	return false
}

// loadReports reads each JSON or JSON.GZ path into an eval.Report.
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

// loadReport reads a single JSON or JSON.GZ path. Errors are wrapped with the path so
// failures point at the offending file.
func loadReport(path string) (*eval.Report, error) {
	data, err := readReportBytes(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var r eval.Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &r, nil
}

func readReportBytes(path string) ([]byte, error) {
	if !strings.HasSuffix(path, ".gz") {
		return os.ReadFile(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

// loadBaselineDir scans a directory for JSON reports and returns them keyed
// by dataset name + version. Multiple files for the same dataset identity
// resolve by the report's generated_at timestamp, with filename as a
// deterministic fallback.
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
		if e.IsDir() || !isReportJSONName(e.Name()) {
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
		datasetKey := reportDatasetKey(r.Dataset)
		if existingKey, ok := selectedKeys[datasetKey]; ok && key <= existingKey {
			continue
		}
		out[datasetKey] = r
		selectedKeys[datasetKey] = key
	}
	return out, nil
}

func reportDatasetKey(d eval.ReportDataset) string {
	if d.Version == "" {
		return d.Name
	}
	return d.Name + "\x00" + d.Version
}

func datasetNameCounts(reports []*eval.Report) map[string]int {
	counts := make(map[string]int)
	for _, r := range reports {
		counts[r.Dataset.Name]++
	}
	return counts
}

func reportDatasetLabel(d eval.ReportDataset, nameCounts map[string]int) string {
	if d.Version == "" || nameCounts[d.Name] <= 1 || strings.HasSuffix(d.Name, "-"+d.Version) {
		return d.Name
	}
	return d.Name + "-" + d.Version
}

func isReportJSONName(name string) bool {
	return strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.gz")
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
	nameCounts := datasetNameCounts(now)
	fmt.Printf("## Headline - %s before/after, with analyzer upper bound\n\n", mainParser)
	fmt.Println("Δ shows percentage-point change in the main parser since the prior report.")
	if bootstrap > 0 {
		fmt.Printf("Accuracy cells include 95%% case-level bootstrap CIs (B=%d, deterministic seed).\n", bootstrap)
	}
	fmt.Println()
	fmt.Printf("| Dataset | Cases | Metric | %s-prev | %s-now | Δ | analyzer |\n", mainParser, mainParser)
	fmt.Println("|---|---:|---|---:|---:|---:|---:|")

	for _, nowR := range now {
		analyzer := analyzerParserFor(nowR)
		ds := reportDatasetLabel(nowR.Dataset, nameCounts)
		cases := nowR.Dataset.CaseCount
		prevR := prev[reportDatasetKey(nowR.Dataset)]

		for _, m := range []metric{metricLemma, metricPOS, metricLemmaPOS, metricGrammar, metricCoverage} {
			nowVal := metricFromReport(nowR, mainParser, m)
			prevVal := math.NaN()
			if prevR != nil {
				prevVal = metricFromReport(prevR, mainParser, m)
			}
			analyzerVal := math.NaN()
			if analyzer != "" {
				analyzerVal = metricFromReport(nowR, analyzer, m)
			}

			nowCell := formatAccuracyWithCI(nowVal, nowR, mainParser, m, bootstrap, rng)
			prevCell := "-"
			if prevR != nil {
				prevCell = formatAccuracyWithCI(prevVal, prevR, mainParser, m, bootstrap, rng)
			}
			delta := "-"
			if !math.IsNaN(nowVal) && !math.IsNaN(prevVal) {
				delta = fmt.Sprintf("%+.1f", (nowVal-prevVal)*100)
			}
			analyzerCell := "-"
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
	nameCounts := datasetNameCounts(reports)
	fmt.Println("| Dataset | Cases | Parser | Lemma | POS | Lemma+POS | Grammar | Full | Coverage | Avg ms |")
	fmt.Println("|---|---:|---|---:|---:|---:|---:|---:|---:|---:|")

	for _, r := range reports {
		for _, parser := range r.Parsers {
			s, ok := r.Summary[parser]
			if !ok {
				continue
			}
			fmt.Printf("| %s | %d | %s | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f |\n",
				reportDatasetLabel(r.Dataset, nameCounts), r.Dataset.CaseCount, parser,
				s.LemmaAccuracy*100, s.POSAccuracy*100,
				lemmaPOSDisplay(s, r, parser)*100,
				s.GrammarAccuracy*100,
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
				reportDatasetLabel(r.Dataset, nameCounts), parser, r.Parsers[0],
				(s.LemmaAccuracy-base.LemmaAccuracy)*100,
				(s.POSAccuracy-base.POSAccuracy)*100,
				(s.ResolvedCoverage-base.ResolvedCoverage)*100,
			)
		}
	}
}

// emitFeatsAttributeTable prints per-UD-FEATS-attribute accuracy for each
// (dataset, parser) pair. Only datasets with at least one parser reporting
// per-attribute data are emitted - older gold sets that only carry
// GrammarLabel produce no rows. Each row covers one attribute (Case,
// Number, Tense, …); the accuracy is computed against the gold tokens
// whose FEATS contained that attribute.
func emitFeatsAttributeTable(reports []*eval.Report) {
	nameCounts := datasetNameCounts(reports)
	type row struct {
		dataset  string
		parser   string
		attr     string
		eligible int
		correct  int
		accuracy float64
	}
	var rows []row
	for _, r := range reports {
		for _, parser := range r.Parsers {
			s, ok := r.Summary[parser]
			if !ok {
				continue
			}
			for _, m := range s.FeatsAttributes {
				rows = append(rows, row{
					dataset:  reportDatasetLabel(r.Dataset, nameCounts),
					parser:   parser,
					attr:     m.Attribute,
					eligible: m.Eligible,
					correct:  m.Correct,
					accuracy: m.Accuracy,
				})
			}
		}
	}
	if len(rows) == 0 {
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].dataset != rows[j].dataset {
			return rows[i].dataset < rows[j].dataset
		}
		if rows[i].attr != rows[j].attr {
			return rows[i].attr < rows[j].attr
		}
		return rows[i].parser < rows[j].parser
	})
	fmt.Println()
	fmt.Println("## Per-FEATS-attribute accuracy")
	fmt.Println()
	fmt.Println("Each row scores one UD FEATS attribute (Case, Number, Tense, ...) on the")
	fmt.Println("subset of gold tokens whose FEATS contained that attribute. Useful for")
	fmt.Println("seeing where the parser is silent vs. wrong on richer morphology than the")
	fmt.Println("single-attribute Grammar metric covers.")
	fmt.Println()
	fmt.Println("| Dataset | Attribute | Parser | Eligible | Correct | Accuracy |")
	fmt.Println("|---|---|---|---:|---:|---:|")
	for _, r := range rows {
		fmt.Printf("| %s | %s | %s | %d | %d | %.1f%% |\n",
			r.dataset, r.attr, r.parser, r.eligible, r.correct, r.accuracy*100)
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
	metricLemma = metric{label: "Lemma", read: func(s eval.ParserSummary) float64 { return s.LemmaAccuracy }}
	metricPOS   = metric{label: "POS", read: func(s eval.ParserSummary) float64 { return s.POSAccuracy }}
	// metricLemmaPOS is the dictionary-entry attachment metric: did the
	// surface form land on the right (lemma, POS) entry? First-class for
	// language-learning quality alongside Grammar - see
	// docs/PARSER_EVAL_METHODOLOGY.md.
	metricLemmaPOS = metric{label: "Lemma+POS", read: func(s eval.ParserSummary) float64 { return s.LemmaPOSAccuracy }}
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

// metricFromReport reads m for parser from r.Summary, falling back to per-case
// recomputation for Lemma+POS on baselines that pre-date the field. The
// fallback only triggers when the summary value is 0 yet the surrounding
// Lemma/POS values prove the parser ran on the data - this preserves
// "0% because we got nothing right" while fixing "0% because the field
// didn't exist when the JSON was written."
func metricFromReport(r *eval.Report, parser string, m metric) float64 {
	s, ok := r.Summary[parser]
	if !ok {
		return math.NaN()
	}
	if m.label == "Lemma+POS" {
		return lemmaPOSDisplay(s, r, parser)
	}
	return summaryMetric(s, m)
}

// lemmaPOSDisplay returns LemmaPOSAccuracy from the summary, or - if absent
// (old baselines pre-dating this metric) - recomputes it from the report's
// per-case Comparisons. Without this fallback, comparing a fresh report
// against a pre-PR baseline directory would show "0.0%" for the prev cell
// and read as a catastrophic regression rather than "metric not present in
// the baseline."
func lemmaPOSDisplay(s eval.ParserSummary, r *eval.Report, parser string) float64 {
	if s.LemmaPOSAccuracy > 0 || s.LemmaAccuracy == 0 || s.POSAccuracy == 0 {
		return s.LemmaPOSAccuracy
	}
	stats := perCaseStats(r, parser)
	var num, den int
	for _, st := range stats {
		num += st.lemmaPOSCorrect
		den += st.expectedTokens
	}
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

// formatAccuracyWithCI returns "82.3% ±0.4" when bootstrap > 0, else "82.3%".
// Returns "-" for NaN. CIs are computed from per-case bootstrap resampling
// of the report's case-level outcomes.
func formatAccuracyWithCI(value float64, r *eval.Report, parser string, m metric, bootstrap int, rng *rand.Rand) string {
	if math.IsNaN(value) {
		return "-"
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
	lemmaPOSCorrect int
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
			if cmp.Match.Lemma && cmp.Match.POS {
				s.lemmaPOSCorrect++
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
	case "Lemma+POS":
		return s.lemmaPOSCorrect, s.expectedTokens
	case "Grammar":
		return s.grammarCorrect, s.grammarEligible
	case "Full":
		return s.fullCorrect, s.expectedTokens
	case "Coverage":
		return s.resolvedTokens, s.totalTokens
	}
	return 0, 0
}
