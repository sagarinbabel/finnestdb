// cmd/autoresearch is an automated rule-tuning loop for the parser. It
// iteratively proposes mutations to the rule data in internal/parserules/,
// runs the parser-eval against a target gold dataset, and accepts mutations
// that don't regress the chosen accuracy metric.
//
// The MVP mutation strategy is "ablation": comment out each individual suffix
// entry one at a time and see if accuracy holds. Suffixes whose removal does
// not regress accuracy are candidates for cleanup or further investigation
// (likely covered by other paths or never firing).
//
// Each experiment is written to a JSONL log so a human or another tool can
// review the run later.
//
// Usage:
//
//	go run ./cmd/autoresearch \
//	    -dataset testdata/parser-eval/fi/gold/fi-manual-v1.json \
//	    -rules   internal/parserules/finnish.go \
//	    -metric  lemma \
//	    -log     experiments/autoresearch-FI.jsonl
//
// Safety:
//   - Each mutation is applied to a single line in the rule file
//   - The original file bytes are saved in memory; if the eval crashes or
//     the user Ctrl-Cs, a deferred restore puts the file back
//   - All experiments are logged with the exact line modified, the metric
//     before/after, and the verdict
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"finnestdb/internal/eval"
	"finnestdb/internal/store"
)

type Experiment struct {
	Iteration  int               `json:"iteration"`
	Timestamp  string            `json:"timestamp"`
	Mutation   MutationDetails   `json:"mutation"`
	Baseline   map[string]float64 `json:"baseline_metric"`
	Candidate  map[string]float64 `json:"candidate_metric"`
	Delta      float64           `json:"delta"`
	Metric     string            `json:"metric"`
	Verdict    string            `json:"verdict"` // "kept" | "reverted" | "skipped"
	Reason     string            `json:"reason,omitempty"`
}

type MutationDetails struct {
	Strategy string `json:"strategy"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Before   string `json:"before"`
	After    string `json:"after"`
}

func main() {
	dataset := flag.String("dataset", "testdata/parser-eval/fi/gold/fi-manual-v1.json", "Gold dataset to evaluate against")
	rulesFile := flag.String("rules", "internal/parserules/finnish.go", "Rule file to mutate")
	parser := flag.String("parser", "custom", "Parser mode to evaluate (basic|custom)")
	dbPath := flag.String("db", "finnestdb.db", "SQLite database path")
	metric := flag.String("metric", "lemma", "Metric to optimise: lemma|pos|grammar|full|coverage")
	minDelta := flag.Float64("min-delta", 0.0, "Min Δ (in pts) to accept a mutation. 0 = accept ties.")
	maxIters := flag.Int("max", 0, "Max iterations (0 = all candidates)")
	logPath := flag.String("log", "experiments/autoresearch.jsonl", "Path to JSONL experiment log")
	dryRun := flag.Bool("dry-run", false, "Don't actually mutate; only report which lines would be tried")
	flag.Parse()

	if err := run(*dataset, *rulesFile, *parser, *dbPath, *metric, *minDelta, *maxIters, *logPath, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "autoresearch:", err)
		os.Exit(1)
	}
}

func run(datasetPath, rulesPath, parserMode, dbPath, metricName string, minDelta float64, maxIters int, logPath string, dryRun bool) error {
	originalBytes, err := os.ReadFile(rulesPath)
	if err != nil {
		return fmt.Errorf("read rules: %w", err)
	}

	// Make sure we restore the file no matter how we exit.
	defer func() {
		if err := os.WriteFile(rulesPath, originalBytes, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "WARNING: failed to restore", rulesPath, ":", err)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	candidates, err := findCandidateLines(originalBytes)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return errors.New("no candidate lines found in rule file")
	}

	if dryRun {
		fmt.Printf("Would try %d candidate mutations:\n", len(candidates))
		for i, c := range candidates {
			fmt.Printf("  %2d. line %d: %s\n", i+1, c.LineNumber, strings.TrimSpace(c.Original))
		}
		return nil
	}

	if maxIters > 0 && maxIters < len(candidates) {
		candidates = candidates[:maxIters]
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("mkdir logs: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()

	// 0. Baseline measurement.
	fmt.Println("== baseline ==")
	baselineMetrics, err := evaluate(datasetPath, dbPath, parserMode)
	if err != nil {
		return fmt.Errorf("baseline eval: %w", err)
	}
	baselineValue := baselineMetrics[metricName]
	fmt.Printf("baseline %s = %.4f\n", metricName, baselineValue)

	// 1. Iterate over candidates.
	for i, cand := range candidates {
		if ctx.Err() != nil {
			fmt.Println("interrupted")
			break
		}
		fmt.Printf("\n== iter %d/%d : line %d : %s ==\n", i+1, len(candidates), cand.LineNumber, strings.TrimSpace(cand.Original))

		// Apply mutation: comment out the line.
		mutated := commentOutLine(originalBytes, cand)
		if err := os.WriteFile(rulesPath, mutated, 0o644); err != nil {
			return fmt.Errorf("write mutation: %w", err)
		}

		exp := Experiment{
			Iteration: i + 1,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Mutation: MutationDetails{
				Strategy: "comment-out-suffix",
				File:     rulesPath,
				Line:     cand.LineNumber,
				Before:   strings.TrimSpace(cand.Original),
				After:    "// " + strings.TrimSpace(cand.Original),
			},
			Baseline: baselineMetrics,
			Metric:   metricName,
		}

		candidateMetrics, err := evaluate(datasetPath, dbPath, parserMode)
		if err != nil {
			exp.Verdict = "skipped"
			exp.Reason = "eval failed: " + err.Error()
			writeExperiment(logFile, exp)
			// Restore for the next iteration.
			if writeErr := os.WriteFile(rulesPath, originalBytes, 0o644); writeErr != nil {
				return fmt.Errorf("restore after eval failure: %w", writeErr)
			}
			continue
		}
		exp.Candidate = candidateMetrics
		candidateValue := candidateMetrics[metricName]
		exp.Delta = (candidateValue - baselineValue) * 100 // in pts

		if exp.Delta >= -minDelta {
			exp.Verdict = "kept"
			fmt.Printf("KEPT (Δ %s = %+.4f pts)\n", metricName, exp.Delta)
			// In MVP we revert anyway — we're just measuring impact, not
			// permanently mutating. A future "greedy" mode could keep accepted
			// changes by updating originalBytes.
		} else {
			exp.Verdict = "reverted"
			fmt.Printf("REVERTED (Δ %s = %+.4f pts < -%.4f)\n", metricName, exp.Delta, minDelta)
		}
		writeExperiment(logFile, exp)

		// Always restore for the next iteration in MVP mode.
		if err := os.WriteFile(rulesPath, originalBytes, 0o644); err != nil {
			return fmt.Errorf("restore between iterations: %w", err)
		}
	}

	fmt.Printf("\nDone. Log: %s\n", logPath)
	return nil
}

type lineCandidate struct {
	LineNumber int
	Original   string
}

// suffixEntryRE matches lines like: `\t{"ssa", "inessive"}, {"ssä", "inessive"},`
// or:                                `\t"nsa", "nsä", // 3rd person…`
// We're conservative — only lines whose first non-comment, non-whitespace token
// is a `"…"` string literal are considered candidate suffix entries.
var suffixEntryRE = regexp.MustCompile(`^(\s*)(?:\{?"|")[^"]+"`)

func findCandidateLines(src []byte) ([]lineCandidate, error) {
	var out []lineCandidate
	lines := strings.Split(string(src), "\n")
	for i, raw := range lines {
		trimmed := strings.TrimLeft(raw, " \t")
		// Only consider entries that look like suffix-table rows.
		if !suffixEntryRE.MatchString(raw) {
			continue
		}
		// Skip lines inside doc comments (they often contain quoted text).
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		out = append(out, lineCandidate{
			LineNumber: i + 1,
			Original:   raw,
		})
	}
	return out, nil
}

func commentOutLine(src []byte, c lineCandidate) []byte {
	lines := strings.Split(string(src), "\n")
	if c.LineNumber < 1 || c.LineNumber > len(lines) {
		return src
	}
	idx := c.LineNumber - 1
	indent := lines[idx][:len(lines[idx])-len(strings.TrimLeft(lines[idx], " \t"))]
	lines[idx] = indent + "// " + strings.TrimLeft(lines[idx], " \t")
	return []byte(strings.Join(lines, "\n"))
}

// evaluate runs the parsertest CLI as a subprocess and returns the headline
// metrics for the chosen parser. Using a subprocess (rather than calling
// internal/eval directly) gives us isolation: if the parser has been put
// in a bad state by a mutation we want a clean process boundary.
func evaluate(datasetPath, dbPath, parserMode string) (map[string]float64, error) {
	tmp, err := os.CreateTemp("", "autoresearch-eval-*.json")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	cmd := exec.Command("go", "run", "./cmd/parsertest",
		"-dataset", datasetPath,
		"-db", dbPath,
		"-parsers", parserMode,
		"-warmup", "0",
		"-repeat", "1",
		"-out", tmp.Name(),
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("parsertest: %w", err)
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return nil, err
	}
	var report eval.Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	s, ok := report.Summary[parserMode]
	if !ok {
		return nil, fmt.Errorf("no summary for parser %q", parserMode)
	}
	return map[string]float64{
		"lemma":    s.LemmaAccuracy,
		"pos":      s.POSAccuracy,
		"grammar":  s.GrammarAccuracy,
		"full":     s.FullAccuracy,
		"coverage": s.ResolvedCoverage,
	}, nil
}

func writeExperiment(f *os.File, exp Experiment) {
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(exp); err != nil {
		fmt.Fprintln(os.Stderr, "log write:", err)
	}
}

// Compile-time guard: keep the store package referenced so the cmd binary
// links the same way as cmd/parsertest (avoids surprise on future refactors).
var _ = store.LemmaKey{}
