// cmd/corpusverify is the promotion-gate verifier. Reads derived TSVs,
// applies hard/soft gates, writes qa-gate.json, exits nonzero on hard
// failure. Appends to errors/error-history.jsonl when it fails.
//
// Usage:
//
//	go run ./cmd/corpusverify -lang fi [-data-root ../localdata] [-profile smoke|pilot|full]
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"finnestdb/corpus_pipeline/internal/cli"
	"finnestdb/corpus_pipeline/internal/sources"
)

type gateResult struct {
	Lang         string         `json:"lang"`
	Profile      string         `json:"profile"`
	UTC          string         `json:"utc"`
	HardFailures []string       `json:"hard_failures"`
	SoftWarnings []string       `json:"soft_warnings"`
	Stats        map[string]any `json:"stats,omitempty"`
}

func main() {
	var (
		dataRoot = flag.String("data-root", "../localdata", "")
		lang     = flag.String("lang", "fi", "")
		profile  = flag.String("profile", "smoke", "smoke | pilot | full")
		dbPath   = flag.String("db", "", "")
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

	res := gateResult{
		Lang:    langLower,
		Profile: *profile,
		UTC:     time.Now().UTC().Format(time.RFC3339),
		Stats:   map[string]any{},
	}

	// Hard gate: preflight (FST + DB)
	if err := cli.PreflightTables(roots.TablesDir); err != nil {
		res.HardFailures = append(res.HardFailures, "preflight.fst: "+err.Error())
	}
	if err := cli.PreflightDB(roots.DBPath, langUpper); err != nil {
		res.HardFailures = append(res.HardFailures, "preflight.dict: "+err.Error())
	}

	// Hard gate: required output files
	derived := sources.DerivedDir(roots.DataRoot, langLower)
	res.HardFailures = append(res.HardFailures, checkRequiredFiles(derived)...)

	// silver-candidates.tsv: presence + content is fine when enrichcorpus
	// has run. The anti-circular concern (aggregator writing silver) is
	// statically guaranteed: aggregatecorpus source has zero references
	// to writing this file. We surface presence as a stat, not a gate.
	silverPath := filepath.Join(derived, "mining", "silver-candidates.tsv")
	if data, err := os.ReadFile(silverPath); err == nil {
		lines := strings.Count(string(data), "\n")
		res.Stats["silver_candidates_rows"] = max(0, lines-1)
	}

	// Hard gate: TSVs parse cleanly + sentences ≤ occurrences
	sentCount := countTSVRows(filepath.Join(derived, "sentences.tsv"))
	occCount := countTSVRows(filepath.Join(derived, "sentence_occurrences.tsv"))
	if occCount < sentCount {
		res.HardFailures = append(res.HardFailures, fmt.Sprintf("sentence_occurrences (%d) < sentences (%d)", occCount, sentCount))
	}
	res.Stats["sentences"] = sentCount
	res.Stats["sentences_user_friendly"] = countTSVRows(filepath.Join(derived, "sentences_user_friendly.tsv"))
	res.Stats["sentence_occurrences"] = occCount
	res.Stats["wordlist_rows"] = countTSVRows(filepath.Join(derived, "wordlist.tsv"))
	res.Stats["wordlist_user_friendly_rows"] = countTSVRows(filepath.Join(derived, "wordlist_user_friendly.tsv"))

	// Hard gate: fixture probes (smoke profile only)
	if *profile == "smoke" {
		if err := smokeFixtureProbes(derived, langLower); err != nil {
			res.HardFailures = append(res.HardFailures, "fixture_probe: "+err.Error())
		}
	}

	// Soft gate: unresolved rate
	if rate := readQARate(filepath.Join(derived, "qa-report.json"), "unresolved_rate_prose"); rate >= 0 {
		res.Stats["unresolved_rate_prose"] = rate
		threshold := 0.10
		if *profile == "smoke" {
			threshold = 0.15
		}
		if rate > threshold {
			res.SoftWarnings = append(res.SoftWarnings, fmt.Sprintf("unresolved_rate_prose=%.3f > %.2f", rate, threshold))
		}
	}

	// Write qa-gate.json
	gatePath := filepath.Join(derived, "qa-gate.json")
	if err := writeJSON(gatePath, res); err != nil {
		log.Fatalf("write qa-gate.json: %v", err)
	}

	if len(res.HardFailures) > 0 {
		// Append to error-history.jsonl + write error_report.txt
		appendErrorHistory(derived, res)
		writeErrorReport(derived, res)
		fmt.Fprintf(os.Stderr, "VERIFY FAIL (lang=%s profile=%s): %d hard failures\n",
			langLower, *profile, len(res.HardFailures))
		for _, hf := range res.HardFailures {
			fmt.Fprintf(os.Stderr, "  - %s\n", hf)
		}
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "VERIFY PASS (lang=%s profile=%s)", langLower, *profile)
	if len(res.SoftWarnings) > 0 {
		fmt.Fprintf(os.Stderr, " — %d soft warning(s)\n", len(res.SoftWarnings))
	} else {
		fmt.Fprintln(os.Stderr)
	}
}

func requiredDerivedFiles() []string {
	return []string{
		"wordlist.tsv", "wordlist_user_friendly.tsv",
		"sentences.tsv", "sentences_user_friendly.tsv", "sentence_occurrences.tsv",
		"poems.tsv", "documents.tsv", "manifest.tsv",
		"build_metadata.json", "qa-report.json",
		"mining/unresolved.tsv", "mining/poetry-unresolved.tsv",
		"mining/parser-disagreements.tsv", "mining/high-frequency-ambiguous.tsv",
		"mining/internal-consensus-candidates.tsv",
	}
}

func checkRequiredFiles(derived string) []string {
	var failures []string
	for _, f := range requiredDerivedFiles() {
		p := filepath.Join(derived, f)
		fi, err := os.Stat(p)
		if err != nil {
			failures = append(failures, "missing_required: "+f)
			continue
		}
		if fi.Size() == 0 {
			failures = append(failures, "empty_required: "+f)
		}
	}
	return failures
}

func countTSVRows(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	n := -1 // discount header
	for {
		_, err := r.Read()
		if err != nil {
			break
		}
		n++
	}
	return n
}

func readQARate(path, key string) float64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	var qa struct {
		Totals map[string]any `json:"totals"`
	}
	if err := json.Unmarshal(data, &qa); err != nil {
		return -1
	}
	v, ok := qa.Totals[key]
	if !ok {
		return -1
	}
	switch x := v.(type) {
	case float64:
		return x
	default:
		return -1
	}
}

func smokeFixtureProbes(derived, lang string) error {
	// Read wordlist.tsv and assert known ambiguous forms produce expected
	// behavior. Per-language probes.
	rows, err := readWordlist(filepath.Join(derived, "wordlist.tsv"))
	if err != nil {
		return err
	}
	bySurface := map[string][]map[string]string{}
	for _, r := range rows {
		surface := r["surface"]
		bySurface[surface] = append(bySurface[surface], r)
	}
	require := func(surface string, minRows int) error {
		got := len(bySurface[surface])
		if got < minRows {
			return fmt.Errorf("expected ≥%d rows for %q, got %d", minRows, surface, got)
		}
		return nil
	}
	requireResolves := func(surface, lemma, pos string) error {
		for _, r := range bySurface[surface] {
			if r["lemma"] == lemma && r["pos"] == pos {
				return nil
			}
		}
		return fmt.Errorf("expected %q to resolve to lemma=%s pos=%s; got %d rows: %v", surface, lemma, pos, len(bySurface[surface]), bySurface[surface])
	}
	switch lang {
	case "fi":
		// Fixture sentences are capitalized at sentence start. Check the
		// capitalized forms.
		if err := require("Tulen", 1); err != nil {
			return err
		}
		if err := requireResolves("Talossa", "talo", "NOUN"); err != nil {
			return err
		}
		if err := requireResolves("Kuusen", "kuusi", "NOUN"); err != nil {
			return err
		}
	case "et":
		if err := require("joon", 2); err != nil {
			return err
		}
		// At least one analysis must be the verb jooma
		var ok bool
		for _, r := range bySurface["joon"] {
			if r["lemma"] == "jooma" {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("expected ET joon to have a jooma/VERB analysis")
		}
	}
	return nil
}

func readWordlist(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	var out []map[string]string
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		row := make(map[string]string, len(header))
		for i, h := range header {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func appendErrorHistory(derived string, res gateResult) {
	dir := filepath.Join(derived, "errors")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "error-history.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	for _, hf := range res.HardFailures {
		entry := map[string]any{
			"utc":         res.UTC,
			"profile":     res.Profile,
			"stage":       "verify",
			"error_class": classifyError(hf),
			"message":     hf,
			"file_refs":   []string{"qa-gate.json"},
		}
		b, _ := json.Marshal(entry)
		f.Write(b)
		f.Write([]byte("\n"))
	}
}

func classifyError(msg string) string {
	if strings.HasPrefix(msg, "preflight.") {
		i := strings.IndexByte(msg, ':')
		if i > 0 {
			return msg[:i]
		}
	}
	if strings.HasPrefix(msg, "missing_required:") {
		return "verify.missing_required"
	}
	if strings.HasPrefix(msg, "empty_required:") {
		return "verify.empty_required"
	}
	if strings.HasPrefix(msg, "fixture_probe:") {
		return "verify.fixture_probe"
	}
	return "verify.unknown"
}

func writeErrorReport(derived string, res gateResult) {
	dir := filepath.Join(derived, "errors")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "error_report.txt")
	var b strings.Builder
	fmt.Fprintf(&b, "Corpus verify failed\n\n")
	fmt.Fprintf(&b, "Language: %s\n", res.Lang)
	fmt.Fprintf(&b, "Profile: %s\n", res.Profile)
	fmt.Fprintf(&b, "UTC: %s\n\n", res.UTC)
	fmt.Fprintf(&b, "Hard gate failures:\n")
	for _, hf := range res.HardFailures {
		fmt.Fprintf(&b, "- %s\n", hf)
	}
	if len(res.SoftWarnings) > 0 {
		fmt.Fprintf(&b, "\nSoft gate warnings:\n")
		for _, sw := range res.SoftWarnings {
			fmt.Fprintf(&b, "- %s\n", sw)
		}
	}
	fmt.Fprintf(&b, "\nRelevant files:\n")
	fmt.Fprintf(&b, "- %s\n", filepath.Join(derived, "qa-gate.json"))
	fmt.Fprintf(&b, "- %s\n", filepath.Join(derived, "qa-report.json"))
	fmt.Fprintf(&b, "- %s\n", filepath.Join(derived, "errors", "error-history.jsonl"))
	_ = os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
