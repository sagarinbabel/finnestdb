// cmd/enrichcorpus runs external morphological analyzers (omorfi for FI,
// estnltk for ET) on every unique surface in wordlist.tsv and writes a
// sibling wordlist-enriched.tsv with extra columns. Also appends to
// mining/silver-candidates.tsv when external analyzer agrees with
// parser_choice.
//
// Persistent batch adapter — one long-lived subprocess per language,
// JSON-line protocol over stdin/stdout. Avoids per-surface startup cost
// (estnltk init takes ~1 s; per-surface shell-out at 5M surfaces is
// ~58 days).
//
// Graceful degradation: if the external analyzer binary isn't on PATH,
// emits a clear error and exits cleanly (no partial output, no half-
// written enriched.tsv). The pipeline keeps working without enrichment.
//
// Usage:
//
//	go run ./cmd/enrichcorpus -lang fi
//	go run ./cmd/enrichcorpus -lang et
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/cli"
	"finnestdb/corpus_pipeline/internal/sources"
)

func main() {
	var (
		dataRoot = flag.String("data-root", "../localdata", "")
		lang     = flag.String("lang", "fi", "")
		dbPath   = flag.String("db", "", "")
		limit    = flag.Int("limit", 0, "if >0, only process this many unique surfaces (smoke testing)")
	)
	flag.Parse()

	roots, err := cli.Resolve(*dataRoot, *dbPath)
	if err != nil {
		log.Fatalf("resolve: %v", err)
	}
	langLower, langUpper, err := cli.LangCodes(*lang)
	if err != nil {
		log.Fatalf("lang: %v", err)
	}
	derived := sources.DerivedDir(roots.DataRoot, langLower)

	wordlistPath := filepath.Join(derived, "wordlist.tsv")
	if _, err := os.Stat(wordlistPath); err != nil {
		log.Fatalf("wordlist.tsv not found at %s — run aggregatecorpus first", wordlistPath)
	}

	// Detect external analyzer binary. If not present, we emit a
	// helpful error and exit cleanly so the rest of the pipeline keeps
	// working.
	analyzer, err := detectAnalyzer(langUpper)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[enrichcorpus] external analyzer for %s not available: %v\n", langUpper, err)
		fmt.Fprintf(os.Stderr, "[enrichcorpus] To install:\n")
		switch langUpper {
		case "FI":
			fmt.Fprintf(os.Stderr, "  - omorfi:  brew install omorfi  (or pip install omorfi)\n")
			fmt.Fprintf(os.Stderr, "  - or set FINNESTDB_OMORFI_CMD=/path/to/omorfi-disamb-cmdline\n")
		case "ET":
			fmt.Fprintf(os.Stderr, "  - estnltk: pip install estnltk  (then ensure 'python3 -c \"import estnltk\"' works)\n")
		}
		fmt.Fprintf(os.Stderr, "[enrichcorpus] Skipping enrichment. wordlist-enriched.tsv NOT written. silver-candidates.tsv unchanged.\n")
		os.Exit(0)
	}

	// Read unique surfaces from wordlist.tsv (one per surface, dedup
	// across analysis rows).
	surfaces, parserChoiceByS := readSurfaces(wordlistPath, *limit)
	fmt.Fprintf(os.Stderr, "[enrichcorpus] enriching %d unique surfaces for lang=%s via %s\n", len(surfaces), langUpper, analyzer.label)

	// Run analyses. If the persistent subprocess errors, log + exit.
	enriched, err := analyzer.analyzeBatch(surfaces)
	if err != nil {
		log.Fatalf("analyzer batch: %v", err)
	}

	// Write enriched TSV
	enrichedPath := filepath.Join(derived, "wordlist-enriched.tsv")
	if err := writeEnriched(enrichedPath, surfaces, enriched, langLower); err != nil {
		log.Fatalf("write enriched: %v", err)
	}

	// Append to silver-candidates.tsv (the only path that writes this file)
	silverPath := filepath.Join(derived, "mining", "silver-candidates.tsv")
	silverCount, err := writeSilver(silverPath, surfaces, enriched, parserChoiceByS, analyzer.label)
	if err != nil {
		log.Fatalf("write silver: %v", err)
	}

	fmt.Fprintf(os.Stderr, "[enrichcorpus] done: %s + %d silver candidates\n", enrichedPath, silverCount)
}

// ── analyzer interface + detection ────────────────────────────────────

type analyzer struct {
	label       string
	analyzeBatch func(surfaces []string) (map[string]externalAnalysis, error)
}

type externalAnalysis struct {
	Lemma  string
	POS    string
	Feats  string
	Count  int    // number of analyses returned
	Source string // "omorfi" or "estnltk"
}

func detectAnalyzer(langUpper string) (*analyzer, error) {
	// Both analyzers are reached via the local venv's python (created by
	// the user once with `python3 -m venv .venv && pip install omorfi
	// estnltk`). We invoke them via batch scripts in scripts/.
	venvPy := venvPython()
	switch langUpper {
	case "FI":
		// Try venv first, then system PATH.
		py := venvPy
		if py == "" {
			py = "python3"
		}
		check := exec.Command(py, "-c", "import omorfi")
		if out, err := check.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("python3 -c 'import omorfi' failed: %v (output: %s); install: cd corpus_pipeline && python3 -m venv .venv && .venv/bin/pip install omorfi && .venv/bin/omorfi-download.py", err, strings.TrimSpace(string(out)))
		}
		return &analyzer{
			label:        "omorfi",
			analyzeBatch: makePythonBatch(py, "scripts/omorfi-batch.py"),
		}, nil
	case "ET":
		py := venvPy
		if py == "" {
			py = "python3"
		}
		// Prefer vabamorf-direct (parallel to FI's omorfi-direct).
		// Fall back to estnltk's higher-level pipeline if vabamorf
		// import fails.
		if err := exec.Command(py, "-c", "from estnltk.vabamorf.morf import Vabamorf; Vabamorf()").Run(); err == nil {
			return &analyzer{
				label:        "vabamorf",
				analyzeBatch: makePythonBatch(py, "scripts/vabamorf-batch.py"),
			}, nil
		}
		check := exec.Command(py, "-c", "import estnltk")
		if out, err := check.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("python3 -c 'import estnltk' failed: %v (output: %s); install: cd corpus_pipeline && python3 -m venv .venv && .venv/bin/pip install estnltk", err, strings.TrimSpace(string(out)))
		}
		return &analyzer{
			label:        "estnltk",
			analyzeBatch: makePythonBatch(py, "scripts/estnltk-batch.py"),
		}, nil
	default:
		return nil, fmt.Errorf("no external analyzer for lang=%s", langUpper)
	}
}

// venvPython returns the absolute path to corpus_pipeline/.venv/bin/python
// if it exists, else empty string.
func venvPython() string {
	// Try resolving from cwd (Makefile invokes us from corpus_pipeline/)
	candidates := []string{
		".venv/bin/python",
		".venv/bin/python3",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
		}
	}
	return ""
}

// ── batch implementations ─────────────────────────────────────────────

// makePythonBatch invokes a persistent Python subprocess (omorfi-batch.py
// or estnltk-batch.py). Both scripts share the same TSV protocol:
// surface → "surface\tlemma\tPOS\tfeats\tcount\n".
func makePythonBatch(py, scriptRel string) func([]string) (map[string]externalAnalysis, error) {
	return func(surfaces []string) (map[string]externalAnalysis, error) {
		// Resolve script path: try cwd first, then relative to corpus_pipeline.
		scriptPath := scriptRel
		if _, err := os.Stat(scriptPath); err != nil {
			return nil, fmt.Errorf("batch script %q not found from cwd", scriptRel)
		}
		c := exec.Command(py, scriptPath)
		stdin, err := c.StdinPipe()
		if err != nil {
			return nil, err
		}
		stdout, err := c.StdoutPipe()
		if err != nil {
			return nil, err
		}
		c.Stderr = os.Stderr
		if err := c.Start(); err != nil {
			return nil, fmt.Errorf("start %s: %w", scriptPath, err)
		}
		go func() {
			w := bufio.NewWriter(stdin)
			for _, s := range surfaces {
				w.WriteString(s)
				w.WriteByte('\n')
			}
			w.Flush()
			stdin.Close()
		}()
		out := map[string]externalAnalysis{}
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1<<20), 4<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) < 5 {
				continue
			}
			out[fields[0]] = externalAnalysis{
				Lemma:  fields[1],
				POS:    fields[2],
				Feats:  fields[3],
				Source: filepath.Base(scriptPath),
			}
		}
		c.Wait()
		return out, nil
	}
}

// makeOmorfiBatch returns a closure that pipes surfaces through a
// long-lived omorfi-disamb-cmdline subprocess.
func makeOmorfiBatch(cmd string) func([]string) (map[string]externalAnalysis, error) {
	return func(surfaces []string) (map[string]externalAnalysis, error) {
		c := exec.Command(cmd)
		stdin, err := c.StdinPipe()
		if err != nil {
			return nil, err
		}
		stdout, err := c.StdoutPipe()
		if err != nil {
			return nil, err
		}
		c.Stderr = os.Stderr
		if err := c.Start(); err != nil {
			return nil, fmt.Errorf("start %s: %w", cmd, err)
		}
		// Feed surfaces, read output. omorfi-disamb-cmdline expects
		// whitespace-tokenized input on stdin, emits analyses on stdout
		// in CONLLU-ish format. Implementation: just feed each surface
		// on its own line, parse output back. This is best-effort —
		// omorfi versions vary.
		go func() {
			w := bufio.NewWriter(stdin)
			for _, s := range surfaces {
				w.WriteString(s)
				w.WriteByte('\n')
			}
			w.Flush()
			stdin.Close()
		}()
		out := map[string]externalAnalysis{}
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1<<20), 4<<20)
		var current string
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			// Heuristic: if line has 10 columns, it's CONLLU. Column 1
			// is index, column 2 is form, column 3 lemma, column 4 UPOS,
			// column 6 feats.
			if len(fields) >= 6 {
				surface := fields[1]
				if existing, ok := out[surface]; !ok {
					out[surface] = externalAnalysis{
						Lemma:  fields[2],
						POS:    fields[3],
						Feats:  fields[5],
						Count:  1,
						Source: "omorfi",
					}
				} else {
					existing.Count++
					out[surface] = existing
				}
				current = surface
			}
		}
		_ = current
		if err := c.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "[omorfi] subprocess exited with: %v (partial results kept)\n", err)
		}
		return out, nil
	}
}

// makeEstnltkBatch invokes a persistent Python subprocess using a small
// embedded script that loads estnltk once and reads/writes JSON-line.
func makeEstnltkBatch() func([]string) (map[string]externalAnalysis, error) {
	return func(surfaces []string) (map[string]externalAnalysis, error) {
		scriptPath := filepath.Join(filepath.Dir(os.Args[0]), "..", "..", "scripts", "estnltk-batch.py")
		// Try a few path resolutions
		if _, err := os.Stat(scriptPath); err != nil {
			// Look from cwd
			scriptPath = "scripts/estnltk-batch.py"
			if _, err := os.Stat(scriptPath); err != nil {
				return nil, fmt.Errorf("estnltk batch script not found at %s", scriptPath)
			}
		}
		c := exec.Command("python3", scriptPath)
		stdin, err := c.StdinPipe()
		if err != nil {
			return nil, err
		}
		stdout, err := c.StdoutPipe()
		if err != nil {
			return nil, err
		}
		c.Stderr = os.Stderr
		if err := c.Start(); err != nil {
			return nil, fmt.Errorf("start estnltk: %w", err)
		}
		go func() {
			w := bufio.NewWriter(stdin)
			for _, s := range surfaces {
				w.WriteString(s)
				w.WriteByte('\n')
			}
			w.Flush()
			stdin.Close()
		}()
		out := map[string]externalAnalysis{}
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1<<20), 4<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			// Format: surface\tlemma\tPOS\tfeats\tcount
			fields := strings.Split(line, "\t")
			if len(fields) < 5 {
				continue
			}
			out[fields[0]] = externalAnalysis{
				Lemma:  fields[1],
				POS:    fields[2],
				Feats:  fields[3],
				Source: "estnltk",
			}
		}
		c.Wait()
		return out, nil
	}
}

// ── readers / writers ─────────────────────────────────────────────────

type parserChoice struct {
	lemma string
	pos   string
	feats string
}

func readSurfaces(wordlistPath string, limit int) ([]string, map[string]parserChoice) {
	f, err := os.Open(wordlistPath)
	if err != nil {
		log.Fatalf("open wordlist: %v", err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		log.Fatalf("read wordlist header: %v", err)
	}
	col := func(name string) int {
		for i, h := range header {
			if h == name {
				return i
			}
		}
		return -1
	}
	idxSurface := col("surface")
	idxLemma := col("lemma")
	idxPOS := col("pos")
	idxFeats := col("feats")
	idxIsParserChoice := col("is_parser_choice")
	if idxSurface < 0 || idxLemma < 0 || idxPOS < 0 || idxIsParserChoice < 0 {
		log.Fatalf("wordlist.tsv missing required columns; header: %v", header)
	}

	var surfaces []string
	choice := map[string]parserChoice{}
	seen := map[string]bool{}
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if idxSurface >= len(rec) {
			continue
		}
		surface := rec[idxSurface]
		if seen[surface] {
			// Capture parser_choice rows even when we've already added
			// the surface to the queue.
			if idxIsParserChoice < len(rec) && rec[idxIsParserChoice] == "1" {
				choice[surface] = parserChoice{
					lemma: rec[idxLemma], pos: rec[idxPOS], feats: rec[idxFeats],
				}
			}
			continue
		}
		seen[surface] = true
		surfaces = append(surfaces, surface)
		if idxIsParserChoice < len(rec) && rec[idxIsParserChoice] == "1" {
			choice[surface] = parserChoice{
				lemma: rec[idxLemma], pos: rec[idxPOS], feats: rec[idxFeats],
			}
		}
		if limit > 0 && len(surfaces) >= limit {
			break
		}
	}
	return surfaces, choice
}

func writeEnriched(path string, surfaces []string, enriched map[string]externalAnalysis, langLower string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Comma = '\t'
	if err := w.Write([]string{"surface", "lang", "external_lemma", "external_pos", "external_feats", "external_analysis_count", "external_source"}); err != nil {
		return err
	}
	for _, s := range surfaces {
		ea, ok := enriched[s]
		if !ok {
			_ = w.Write([]string{s, langLower, "", "", "", "0", ""})
			continue
		}
		_ = w.Write([]string{s, langLower, ea.Lemma, ea.POS, ea.Feats, fmt.Sprintf("%d", ea.Count), ea.Source})
	}
	w.Flush()
	return w.Error()
}

func writeSilver(path string, surfaces []string, enriched map[string]externalAnalysis, parserChoice map[string]parserChoice, analyzerLabel string) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Comma = '\t'
	if err := w.Write([]string{"surface", "agreed_lemma", "agreed_pos", "external_analyzer"}); err != nil {
		return 0, err
	}
	count := 0
	for _, s := range surfaces {
		ea, ok := enriched[s]
		if !ok {
			continue
		}
		pc, ok := parserChoice[s]
		if !ok {
			continue
		}
		// Silver = parser_choice agrees with external analyzer on (lemma, pos)
		if ea.Lemma == pc.lemma && ea.POS == pc.pos {
			_ = w.Write([]string{s, pc.lemma, pc.pos, analyzerLabel})
			count++
		}
	}
	w.Flush()
	return count, w.Error()
}
