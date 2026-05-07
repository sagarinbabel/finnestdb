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
	Iteration int                `json:"iteration"`
	Timestamp string             `json:"timestamp"`
	Mutation  MutationDetails    `json:"mutation"`
	Baseline  map[string]float64 `json:"baseline_metric"`
	Candidate map[string]float64 `json:"candidate_metric"`
	Delta     float64            `json:"delta"`
	Metric    string             `json:"metric"`
	Verdict   string             `json:"verdict"` // "kept" | "reverted" | "skipped"
	Reason    string             `json:"reason,omitempty"`
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

// supportedMetrics lists the keys that evaluate() emits. Validation is done
// against this set so a typo (e.g. -metric "lemmaa") fails fast rather than
// silently producing 0/0/0 deltas.
var supportedMetrics = map[string]struct{}{
	"lemma":    {},
	"pos":      {},
	"grammar":  {},
	"full":     {},
	"coverage": {},
}

func run(datasetPath, rulesPath, parserMode, dbPath, metricName string, minDelta float64, maxIters int, logPath string, dryRun bool) error {
	if _, ok := supportedMetrics[metricName]; !ok {
		return fmt.Errorf("unsupported metric %q (supported: lemma, pos, grammar, full, coverage)", metricName)
	}

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

	candidates, err := findCandidates(originalBytes)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return errors.New("no candidate mutations found in rule file")
	}

	if dryRun {
		fmt.Printf("Would try %d candidate mutations:\n", len(candidates))
		for i, c := range candidates {
			fmt.Printf("  %2d. line %d: %s\n", i+1, c.LineNumber, c.Original)
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
		fmt.Printf("\n== iter %d/%d : line %d : %s ==\n", i+1, len(candidates), cand.LineNumber, cand.Original)

		// Apply mutation: comment out only this candidate's byte range.
		mutated := commentOutCandidate(originalBytes, cand)
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
				Before:   cand.Original,
				After:    "/* " + cand.Original + " */",
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

// candidate identifies a single suffix entry (one possessive string literal,
// or one CaseSuffix struct literal) at a precise byte range in the rule file.
// Mutating just that range — instead of the whole line — keeps each
// experiment attributable to exactly one rule, even when the source line
// packs multiple entries together (e.g. `{"ssa", "inessive"}, {"ssä", "inessive"},`).
type candidate struct {
	LineNumber int
	StartByte  int
	EndByte    int
	Original   string // the exact text being mutated
}

// caseSuffixRE matches a single `{"<suffix>", "<label>"}` struct literal and
// captures its start/end. This is the form used by FinnishCaseSuffixes and
// EstonianCaseSuffixes in internal/parserules/.
var caseSuffixRE = regexp.MustCompile(`\{\s*"[^"]+"\s*,\s*"[^"]+"\s*\}`)

// possessiveSuffixRE matches a single bare string literal in the
// FinnishPossessiveSuffixes slice. The literal must follow the slice
// opening `{`, a comma, or the start of the line — this prevents matching
// strings inside doc comments. The trailing comma is intentionally NOT
// consumed so the next iteration can re-anchor on it.
var possessiveSuffixRE = regexp.MustCompile(`(?m)(?:^|,|\{)\s*("[^"]+")`)

func findCandidates(src []byte) ([]candidate, error) {
	var out []candidate
	text := string(src)

	// Walk source line by line; produce candidates only inside slice
	// literals, not inside comments.
	lineStart := 0
	lineNum := 1
	for lineStart < len(text) {
		nl := strings.Index(text[lineStart:], "\n")
		end := lineStart + nl
		if nl < 0 {
			end = len(text)
		}
		line := text[lineStart:end]
		trimmed := strings.TrimLeft(line, " \t")

		// Skip whole-line comments (rule docs contain example "..." literals).
		if strings.HasPrefix(trimmed, "//") {
			lineStart = end + 1
			lineNum++
			continue
		}

		// Strip any trailing line comment so its quoted strings don't match.
		codeOnly := line
		if c := strings.Index(line, "//"); c >= 0 {
			codeOnly = line[:c]
		}

		// Pass 1: case-suffix struct literals on this line.
		for _, m := range caseSuffixRE.FindAllStringIndex(codeOnly, -1) {
			out = append(out, candidate{
				LineNumber: lineNum,
				StartByte:  lineStart + m[0],
				EndByte:    lineStart + m[1],
				Original:   line[m[0]:m[1]],
			})
		}

		// Pass 2: bare possessive-suffix string literals on this line, but
		// only if the line had no case-suffix matches (the two formats don't
		// mix in the same slice).
		if len(caseSuffixRE.FindAllStringIndex(codeOnly, -1)) == 0 {
			for _, m := range possessiveSuffixRE.FindAllStringSubmatchIndex(codeOnly, -1) {
				// m[2:4] is the captured literal range
				out = append(out, candidate{
					LineNumber: lineNum,
					StartByte:  lineStart + m[2],
					EndByte:    lineStart + m[3],
					Original:   line[m[2]:m[3]],
				})
			}
		}

		lineStart = end + 1
		lineNum++
	}
	return out, nil
}

// commentOutCandidate removes the candidate's entry from the slice literal
// while keeping the source syntactically valid. The entry text itself is
// preserved inside an adjacent block comment so the experiment log can show
// exactly what was removed; the surrounding comma is also consumed so the
// slice doesn't end up with a stray `,` (which would be a parse error).
func commentOutCandidate(src []byte, c candidate) []byte {
	if c.StartByte < 0 || c.EndByte > len(src) || c.StartByte >= c.EndByte {
		return src
	}

	// Expand the range to swallow whichever side has a comma so the
	// slice literal remains valid. Prefer trailing comma (the normal case).
	start, end := c.StartByte, c.EndByte
	for end < len(src) && (src[end] == ' ' || src[end] == '\t') {
		end++
	}
	if end < len(src) && src[end] == ',' {
		end++ // consume trailing comma
	} else {
		// No trailing comma; try to consume a leading one.
		for start > 0 && (src[start-1] == ' ' || src[start-1] == '\t') {
			start--
		}
		if start > 0 && src[start-1] == ',' {
			start--
		}
	}

	commented := []byte("/* removed: " + string(src[c.StartByte:c.EndByte]) + " */")
	out := make([]byte, 0, len(src))
	out = append(out, src[:start]...)
	out = append(out, commented...)
	out = append(out, src[end:]...)
	return out
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
		"lemma":     s.LemmaAccuracy,
		"pos":       s.POSAccuracy,
		"lemma_pos": s.LemmaPOSAccuracy,
		"grammar":   s.GrammarAccuracy,
		"full":      s.FullAccuracy,
		"coverage":  s.ResolvedCoverage,
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
