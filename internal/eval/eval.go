package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/store"
)

type Dataset struct {
	Name     string        `json:"name"`
	Version  string        `json:"version"`
	Language string        `json:"language"`
	Cases    []DatasetCase `json:"cases"`
}

type DatasetCase struct {
	ID     string             `json:"id"`
	Text   string             `json:"text"`
	Tokens []ExpectedTokenRef `json:"tokens"`
}

type ExpectedTokenRef struct {
	Surface      string `json:"surface"`
	Occurrence   int    `json:"occurrence,omitempty"`
	Lemma        string `json:"lemma"`
	POS          string `json:"pos"`
	GrammarLabel string `json:"grammar_label,omitempty"`
}

type EvaluateOptions struct {
	Parsers    []string
	GitCommit  string
	WarmupRuns int
	RepeatRuns int
}

type Report struct {
	RunID               string                   `json:"run_id"`
	Generated           string                   `json:"generated_at"`
	Dataset             ReportDataset            `json:"dataset"`
	Parsers             []string                 `json:"parsers"`
	GitCommit           string                   `json:"git_commit,omitempty"`
	Benchmark           BenchmarkConfig          `json:"benchmark"`
	Summary             map[string]ParserSummary `json:"summary"`
	PriorityRegressions []PriorityRegression     `json:"priority_regressions,omitempty"`
	Cases               []CaseReport             `json:"cases"`
}

type ReportDataset struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Language  string `json:"language"`
	CaseCount int    `json:"case_count"`
}

type BenchmarkConfig struct {
	WarmupRuns int `json:"warmup_runs"`
	RepeatRuns int `json:"repeat_runs"`
}

type ParserSummary struct {
	ExpectedTokens        int     `json:"expected_tokens"`
	LemmaAccuracy         float64 `json:"lemma_accuracy"`
	POSAccuracy           float64 `json:"pos_accuracy"`
	GrammarAccuracy       float64 `json:"grammar_accuracy"`
	FullAccuracy          float64 `json:"full_accuracy"`
	ResolvedCoverage      float64 `json:"resolved_coverage"`
	AvgCaseDurationMs     float64 `json:"avg_case_duration_ms"`
	P50CaseDurationMs     float64 `json:"p50_case_duration_ms"`
	P95CaseDurationMs     float64 `json:"p95_case_duration_ms"`
	WordsPerSecond        float64 `json:"words_per_second"`
	CharsPerSecond        float64 `json:"chars_per_second"`
	AvgUniqueForms        float64 `json:"avg_unique_forms"`
	AvgResolvedTokens     float64 `json:"avg_resolved_tokens"`
	AvgUnresolvedTokens   float64 `json:"avg_unresolved_tokens"`
	AvgAnalyzeMs          float64 `json:"avg_analyze_ms"`
	AvgLookupFormsMs      float64 `json:"avg_lookup_forms_ms"`
	AvgLookupGlossesMs    float64 `json:"avg_lookup_glosses_ms"`
	AvgResolveSentencesMs float64 `json:"avg_resolve_sentences_ms"`
	AvgEnrichWordsMs      float64 `json:"avg_enrich_words_ms"`
}

type CaseReport struct {
	CaseID      string                          `json:"case_id"`
	Text        string                          `json:"text"`
	TokenCount  int                             `json:"token_count"`
	DurationMs  map[string]DurationStats        `json:"duration_ms"`
	Stats       map[string]parsecore.ParseStats `json:"stats,omitempty"`
	Comparisons map[string][]TokenCompare       `json:"comparisons"`
}

type DurationStats struct {
	SamplesNs []int64 `json:"samples_ns"`
	AvgMs     float64 `json:"avg_ms"`
	P50Ms     float64 `json:"p50_ms"`
	P95Ms     float64 `json:"p95_ms"`
}

type TokenCompare struct {
	Surface    string        `json:"surface"`
	Occurrence int           `json:"occurrence"`
	Expected   TokenExpected `json:"expected"`
	Actual     TokenActual   `json:"actual"`
	Match      TokenMatch    `json:"match"`
}

type TokenExpected struct {
	Lemma        string `json:"lemma"`
	POS          string `json:"pos"`
	GrammarLabel string `json:"grammar_label,omitempty"`
}

type TokenActual struct {
	Found        bool     `json:"found"`
	Lemma        string   `json:"lemma,omitempty"`
	POS          string   `json:"pos,omitempty"`
	GrammarLabel string   `json:"grammar_label,omitempty"`
	Source       string   `json:"source,omitempty"`
	Resolved     bool     `json:"resolved"`
	Trace        []string `json:"trace,omitempty"`
	RuleTrace    []string `json:"rule_trace,omitempty"`
}

type TokenMatch struct {
	Lemma   bool `json:"lemma"`
	POS     bool `json:"pos"`
	Grammar bool `json:"grammar"`
	Full    bool `json:"full"`
}

type PriorityRegression struct {
	Parser      string        `json:"parser"`
	CaseID      string        `json:"case_id"`
	Surface     string        `json:"surface"`
	Occurrence  int           `json:"occurrence"`
	Expected    TokenExpected `json:"expected"`
	Actual      TokenActual   `json:"actual"`
	PassingRefs []string      `json:"passing_refs"`
}

func LoadDataset(path string) (*Dataset, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var dataset Dataset
	if err := json.Unmarshal(b, &dataset); err != nil {
		return nil, err
	}
	if dataset.Name == "" {
		return nil, fmt.Errorf("dataset name is required")
	}
	if dataset.Language != "FI" && dataset.Language != "ET" {
		return nil, fmt.Errorf("dataset language must be FI or ET")
	}
	if len(dataset.Cases) == 0 {
		return nil, fmt.Errorf("dataset must contain at least one case")
	}
	for i, c := range dataset.Cases {
		if c.ID == "" {
			return nil, fmt.Errorf("case %d: id is required", i)
		}
		if c.Text == "" {
			return nil, fmt.Errorf("case %s: text is required", c.ID)
		}
		if len(c.Tokens) == 0 {
			return nil, fmt.Errorf("case %s: tokens are required", c.ID)
		}
	}
	return &dataset, nil
}

func Evaluate(db *store.DB, dataset *Dataset, options EvaluateOptions) (*Report, error) {
	parsers := slices.Clone(options.Parsers)
	if len(parsers) == 0 {
		parsers = []string{"basic", "custom"}
	}
	supported := make(map[string]struct{})
	for _, name := range parsecore.SupportedParsers() {
		supported[name] = struct{}{}
	}
	for _, parser := range parsers {
		if _, ok := supported[parser]; !ok {
			return nil, fmt.Errorf("unsupported parser %q", parser)
		}
	}
	if options.RepeatRuns <= 0 {
		options.RepeatRuns = 1
	}
	if options.WarmupRuns < 0 {
		options.WarmupRuns = 0
	}

	runID := fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405Z"), slugify(dataset.Name))
	report := &Report{
		RunID:     runID,
		Generated: time.Now().UTC().Format(time.RFC3339),
		Dataset: ReportDataset{
			Name:      dataset.Name,
			Version:   dataset.Version,
			Language:  dataset.Language,
			CaseCount: len(dataset.Cases),
		},
		Parsers:   parsers,
		GitCommit: options.GitCommit,
		Benchmark: BenchmarkConfig{
			WarmupRuns: options.WarmupRuns,
			RepeatRuns: options.RepeatRuns,
		},
		Summary: make(map[string]ParserSummary, len(parsers)),
		Cases:   make([]CaseReport, 0, len(dataset.Cases)),
	}

	summaries := make(map[string]*summaryAccumulator, len(parsers))
	for _, parser := range parsers {
		summaries[parser] = &summaryAccumulator{}
	}

	for _, c := range dataset.Cases {
		caseReport := CaseReport{
			CaseID:      c.ID,
			Text:        c.Text,
			DurationMs:  make(map[string]DurationStats, len(parsers)),
			Stats:       make(map[string]parsecore.ParseStats, len(parsers)),
			Comparisons: make(map[string][]TokenCompare, len(parsers)),
		}
		for _, parser := range parsers {
			for i := 0; i < options.WarmupRuns; i++ {
				if _, err := parsecore.Analyze(db, dataset.Language, c.Text, parser); err != nil {
					return nil, fmt.Errorf("case %s parser %s warmup: %w", c.ID, parser, err)
				}
			}

			var last *parsecore.ParseResult
			samples := make([]int64, 0, options.RepeatRuns)
			for i := 0; i < options.RepeatRuns; i++ {
				parsed, err := parsecore.Analyze(db, dataset.Language, c.Text, parser)
				if err != nil {
					return nil, fmt.Errorf("case %s parser %s: %w", c.ID, parser, err)
				}
				last = parsed
				samples = append(samples, parsed.ParseDurationNs)
			}
			if last == nil {
				return nil, fmt.Errorf("case %s parser %s: no result", c.ID, parser)
			}
			caseReport.TokenCount = last.TotalTokens
			caseReport.DurationMs[parser] = summarizeDurations(samples)
			caseReport.Stats[parser] = last.Stats
			comparisons := compareCase(c, last)
			caseReport.Comparisons[parser] = comparisons
			summaries[parser].consume(last, comparisons, samples, utf8.RuneCountInString(c.Text))
		}
		report.Cases = append(report.Cases, caseReport)
	}

	for _, parser := range parsers {
		report.Summary[parser] = summaries[parser].finish()
	}
	report.PriorityRegressions = computePriorityRegressions(report)
	return report, nil
}

func DefaultReportPath(rootDir string, report *Report) string {
	filename := report.RunID + ".json"
	return filepath.Join(rootDir, "reports", "parser-eval", filename)
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

type summaryAccumulator struct {
	expectedTokens          int
	lemmaCorrect            int
	posCorrect              int
	grammarEligible         int
	grammarCorrect          int
	fullCorrect             int
	totalTokens             int
	resolvedTokens          int
	allDurationsNs          []int64
	caseCount               int
	uniqueFormsTotal        int
	resolvedTokensTotal     int
	unresolvedTokensTotal   int
	analyzeNsTotal          int64
	lookupFormsNsTotal      int64
	lookupGlossesNsTotal    int64
	resolveSentencesNsTotal int64
	enrichWordsNsTotal      int64
	throughputCharsTotal    int64
	throughputWordsTotal    int64
}

func (s *summaryAccumulator) consume(parsed *parsecore.ParseResult, comparisons []TokenCompare, durations []int64, charsPerRun int) {
	s.allDurationsNs = append(s.allDurationsNs, durations...)
	s.caseCount++
	s.uniqueFormsTotal += parsed.Stats.UniqueForms
	s.resolvedTokensTotal += parsed.Stats.ResolvedTokens
	s.unresolvedTokensTotal += parsed.Stats.UnresolvedTokens
	s.analyzeNsTotal += parsed.Stats.Timings.AnalyzeNs
	s.lookupFormsNsTotal += parsed.Stats.Timings.LookupFormsNs
	s.lookupGlossesNsTotal += parsed.Stats.Timings.LookupGlossesNs
	s.resolveSentencesNsTotal += parsed.Stats.Timings.ResolveSentencesNs
	s.enrichWordsNsTotal += parsed.Stats.Timings.EnrichWordsNs
	s.throughputCharsTotal += int64(charsPerRun) * int64(len(durations))
	s.throughputWordsTotal += int64(parsed.TotalTokens) * int64(len(durations))
	for _, sentence := range parsed.Sentences {
		for _, token := range sentence.Tokens {
			if token.POS == "PUNCT" {
				continue
			}
			s.totalTokens++
			if token.Resolved {
				s.resolvedTokens++
			}
		}
	}
	for _, cmp := range comparisons {
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
	}
}

func (s *summaryAccumulator) finish() ParserSummary {
	stats := summarizeDurations(s.allDurationsNs)
	var totalNs int64
	for _, sample := range s.allDurationsNs {
		totalNs += sample
	}
	return ParserSummary{
		ExpectedTokens:        s.expectedTokens,
		LemmaAccuracy:         ratio(s.lemmaCorrect, s.expectedTokens),
		POSAccuracy:           ratio(s.posCorrect, s.expectedTokens),
		GrammarAccuracy:       ratio(s.grammarCorrect, s.grammarEligible),
		FullAccuracy:          ratio(s.fullCorrect, s.expectedTokens),
		ResolvedCoverage:      ratio(s.resolvedTokens, s.totalTokens),
		AvgCaseDurationMs:     stats.AvgMs,
		P50CaseDurationMs:     stats.P50Ms,
		P95CaseDurationMs:     stats.P95Ms,
		WordsPerSecond:        throughputPerSecond(s.throughputWordsTotal, totalNs),
		CharsPerSecond:        throughputPerSecond(s.throughputCharsTotal, totalNs),
		AvgUniqueForms:        averageInt(s.uniqueFormsTotal, s.caseCount),
		AvgResolvedTokens:     averageInt(s.resolvedTokensTotal, s.caseCount),
		AvgUnresolvedTokens:   averageInt(s.unresolvedTokensTotal, s.caseCount),
		AvgAnalyzeMs:          nsAverageMs(s.analyzeNsTotal, s.caseCount),
		AvgLookupFormsMs:      nsAverageMs(s.lookupFormsNsTotal, s.caseCount),
		AvgLookupGlossesMs:    nsAverageMs(s.lookupGlossesNsTotal, s.caseCount),
		AvgResolveSentencesMs: nsAverageMs(s.resolveSentencesNsTotal, s.caseCount),
		AvgEnrichWordsMs:      nsAverageMs(s.enrichWordsNsTotal, s.caseCount),
	}
}

func throughputPerSecond(total, totalNs int64) float64 {
	if totalNs <= 0 {
		return 0
	}
	return float64(total) / float64(totalNs) * 1e9
}

func compareCase(c DatasetCase, parsed *parsecore.ParseResult) []TokenCompare {
	actual := flattenNonPunct(parsed.Sentences)
	comparisons := make([]TokenCompare, 0, len(c.Tokens))
	for _, expected := range c.Tokens {
		occurrence := expected.Occurrence
		if occurrence <= 0 {
			occurrence = 1
		}
		token, found := findOccurrence(actual, expected.Surface, occurrence)
		cmp := TokenCompare{
			Surface:    expected.Surface,
			Occurrence: occurrence,
			Expected: TokenExpected{
				Lemma:        expected.Lemma,
				POS:          expected.POS,
				GrammarLabel: expected.GrammarLabel,
			},
			Actual: TokenActual{Found: found},
		}
		if found {
			cmp.Actual = TokenActual{
				Found:        true,
				Lemma:        token.Lemma,
				POS:          token.POS,
				GrammarLabel: token.GrammarLabel,
				Source:       token.Source,
				Resolved:     token.Resolved,
				Trace:        slices.Clone(token.Trace),
				RuleTrace:    slices.Clone(token.RuleTrace),
			}
		}
		cmp.Match.Lemma = found && token.Lemma == expected.Lemma
		cmp.Match.POS = found && token.POS == expected.POS
		if expected.GrammarLabel == "" {
			cmp.Match.Grammar = true
		} else {
			cmp.Match.Grammar = found && token.GrammarLabel == expected.GrammarLabel
		}
		cmp.Match.Full = cmp.Match.Lemma && cmp.Match.POS && cmp.Match.Grammar
		comparisons = append(comparisons, cmp)
	}
	return comparisons
}

func flattenNonPunct(sentences []parsecore.SentenceResult) []parsecore.TokenResult {
	var out []parsecore.TokenResult
	for _, sentence := range sentences {
		for _, token := range sentence.Tokens {
			if token.POS == "PUNCT" {
				continue
			}
			out = append(out, token)
		}
	}
	return out
}

func findOccurrence(tokens []parsecore.TokenResult, surface string, occurrence int) (parsecore.TokenResult, bool) {
	count := 0
	for _, token := range tokens {
		if token.Form != surface {
			continue
		}
		count++
		if count == occurrence {
			return token, true
		}
	}
	return parsecore.TokenResult{}, false
}

func summarizeDurations(samplesNs []int64) DurationStats {
	stats := DurationStats{SamplesNs: slices.Clone(samplesNs)}
	if len(samplesNs) == 0 {
		return stats
	}
	var totalNs int64
	for _, sample := range samplesNs {
		totalNs += sample
	}
	stats.AvgMs = float64(totalNs) / float64(len(samplesNs)) / 1e6
	stats.P50Ms = percentileNsToMs(samplesNs, 0.50)
	stats.P95Ms = percentileNsToMs(samplesNs, 0.95)
	return stats
}

func percentileNsToMs(samplesNs []int64, p float64) float64 {
	if len(samplesNs) == 0 {
		return 0
	}
	cloned := slices.Clone(samplesNs)
	sort.Slice(cloned, func(i, j int) bool { return cloned[i] < cloned[j] })
	idx := int(float64(len(cloned)-1) * p)
	return float64(cloned[idx]) / 1e6
}

func computePriorityRegressions(report *Report) []PriorityRegression {
	if len(report.Parsers) < 3 {
		return nil
	}
	focus := report.Parsers[len(report.Parsers)-1]
	references := report.Parsers[:len(report.Parsers)-1]
	var out []PriorityRegression
	for _, c := range report.Cases {
		focusComparisons := c.Comparisons[focus]
		for idx, focusCmp := range focusComparisons {
			if focusCmp.Match.Full {
				continue
			}
			allRefsPass := true
			for _, ref := range references {
				refComparisons := c.Comparisons[ref]
				if idx >= len(refComparisons) || !refComparisons[idx].Match.Full {
					allRefsPass = false
					break
				}
			}
			if !allRefsPass {
				continue
			}
			out = append(out, PriorityRegression{
				Parser:      focus,
				CaseID:      c.CaseID,
				Surface:     focusCmp.Surface,
				Occurrence:  focusCmp.Occurrence,
				Expected:    focusCmp.Expected,
				Actual:      focusCmp.Actual,
				PassingRefs: slices.Clone(references),
			})
		}
	}
	return out
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func averageInt(total, count int) float64 {
	if count == 0 {
		return 0
	}
	return float64(total) / float64(count)
}

func nsAverageMs(totalNs int64, count int) float64 {
	if count == 0 {
		return 0
	}
	return float64(totalNs) / float64(count) / 1e6
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-")
	s = replacer.Replace(s)
	if s == "" {
		return "dataset"
	}
	return s
}
