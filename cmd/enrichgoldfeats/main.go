// cmd/enrichgoldfeats seeds UD FEATS into a manual gold JSON by running
// the language-appropriate adapter (omorfi for FI, estnltk for ET) on
// each case's text and overlaying the adapter's per-token FEATS onto
// the gold tokens. The Case= attribute is deterministically anchored to
// the gold token's existing grammar_label so we never inherit a tool
// error on the column we already commit to.
//
// Usage:
//
//	go run ./cmd/enrichgoldfeats \
//	    -in  testdata/parser-eval/fi/gold/fi-grammar-v1.json \
//	    -out testdata/parser-eval/fi/gold/fi-grammar-v1.json \
//	    -diff testdata/parser-eval/fi/gold/fi-grammar-v1.diff.md
//
// Or to enrich all six gold files at once:
//
//	go run ./cmd/enrichgoldfeats -all
//
// Adapters are resolved from the same env vars parsecore uses:
//
//	FINNESTDB_OMORFI_CMD   — full command, e.g.
//	                          "$(pwd)/.venv-omorfi/bin/python $(pwd)/scripts/omorfi_adapter_example.py"
//	FINNESTDB_ESTNLTK_CMD  — same shape; if unset, tool auto-discovers
//	                          .venv-estnltk/bin/python next to the
//	                          repo root (matches parsecore convention).
//
// The diff report flags tokens where:
//   - the adapter returned no analysis for the surface,
//   - the adapter's Case disagreed with grammar_label (gold wins; flagged),
//   - the gold token wasn't found in the adapter's tokenisation.
//
// The user reviews the .diff.md and edits the gold .json directly for
// any corrections — this tool is the seeding pass, not the final say.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"finnestdb/internal/parserules"
	"finnestdb/pkg/lemmatizer-fi-et/udfeats"
)

type goldFile struct {
	Name     string     `json:"name"`
	Version  string     `json:"version"`
	Language string     `json:"language"`
	Cases    []goldCase `json:"cases"`
}

type goldCase struct {
	ID     string      `json:"id"`
	Text   string      `json:"text"`
	Tokens []goldToken `json:"tokens"`
}

type goldToken struct {
	Surface      string `json:"surface"`
	Lemma        string `json:"lemma"`
	POS          string `json:"pos,omitempty"`
	GrammarLabel string `json:"grammar_label,omitempty"`
	Feats        string `json:"feats,omitempty"`
}

type adapterToken struct {
	Form         string         `json:"form"`
	Lemma        string         `json:"lemma"`
	POS          string         `json:"pos"`
	Feats        map[string]any `json:"feats"`
	GrammarLabel string         `json:"grammar_label"`
}

type adapterSentence struct {
	Tokens []adapterToken `json:"tokens"`
}

type adapterOutput struct {
	Sentences []adapterSentence `json:"sentences"`
}

type diffRow struct {
	caseID, surface, kind, detail string
}

var allFiles = []struct {
	path string
	lang string
}{
	{"testdata/parser-eval/fi/gold/fi-core-v1.json", "FI"},
	{"testdata/parser-eval/fi/gold/fi-grammar-v1.json", "FI"},
	{"testdata/parser-eval/fi/gold/fi-manual-v1.json", "FI"},
	{"testdata/parser-eval/fi/gold/fi-manual-v2.json", "FI"},
	{"testdata/parser-eval/et/gold/et-grammar-v1.json", "ET"},
	{"testdata/parser-eval/et/gold/et-manual-v1.json", "ET"},
}

func main() {
	in := flag.String("in", "", "Input gold JSON path (single-file mode)")
	out := flag.String("out", "", "Output gold JSON path (defaults to -in)")
	diffPath := flag.String("diff", "", "Diff report path (Markdown); defaults to <in>.diff.md")
	all := flag.Bool("all", false, "Enrich all 6 manual gold files in-place; -in/-out ignored")
	flag.Parse()

	if *all {
		for _, f := range allFiles {
			diffOut := strings.TrimSuffix(f.path, ".json") + ".diff.md"
			if err := enrichOne(f.path, f.path, f.lang, diffOut); err != nil {
				fatalf("%s: %v", f.path, err)
			}
			fmt.Printf("enriched %s (diff → %s)\n", f.path, diffOut)
		}
		return
	}
	if *in == "" {
		fatalf("either -in or -all is required")
	}
	if *out == "" {
		*out = *in
	}
	if *diffPath == "" {
		*diffPath = strings.TrimSuffix(*in, ".json") + ".diff.md"
	}
	lang := guessLangFromPath(*in)
	if lang == "" {
		fatalf("could not infer language from path %q; rename or pass -all", *in)
	}
	if err := enrichOne(*in, *out, lang, *diffPath); err != nil {
		fatalf("%v", err)
	}
}

func enrichOne(inPath, outPath, lang, diffPath string) error {
	b, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inPath, err)
	}
	var doc goldFile
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", inPath, err)
	}

	cmdParts, err := resolveAdapter(lang)
	if err != nil {
		return err
	}

	var diffs []diffRow

	for ci, c := range doc.Cases {
		analysis, err := runAdapter(cmdParts, lang, c.Text)
		if err != nil {
			return fmt.Errorf("%s adapter on %s: %w", lang, c.ID, err)
		}
		// Flatten adapter tokens across sentences for this case.
		var atoks []adapterToken
		for _, s := range analysis.Sentences {
			atoks = append(atoks, s.Tokens...)
		}

		for ti, gt := range c.Tokens {
			adapter, found := findAdapterToken(atoks, gt.Surface)
			anchorLabel := strings.TrimSpace(gt.GrammarLabel)
			anchorCase, _ := udfeats.LegacyLabelToUDCase[anchorLabel]
			if !found {
				// Adapter didn't tokenise the gold surface. Try in
				// order: gold's grammar_label anchor → suffix-strip
				// fallback. Both can produce Case=, but neither can
				// safely produce Number=.
				switch {
				case anchorCase != "" && anchorCase != "Nom":
					doc.Cases[ci].Tokens[ti].Feats = "Case=" + anchorCase
				case suffixCase(gt.Surface, lang) != "":
					doc.Cases[ci].Tokens[ti].Feats = "Case=" + suffixCase(gt.Surface, lang)
				}
				diffs = append(diffs, diffRow{c.ID, gt.Surface, "no-adapter-token",
					fmt.Sprintf("adapter did not analyse surface %q", gt.Surface)})
				continue
			}
			merged := mergeFeats(adapter.Feats, anchorCase)
			if merged == "" {
				// Adapter ran but emitted nothing useful (often an
				// OOV compound). Fall back to suffix-strip so the
				// token still carries Case= when its surface is
				// recognisably inflected.
				if sc := suffixCase(gt.Surface, lang); sc != "" {
					merged = "Case=" + sc
				}
			}
			doc.Cases[ci].Tokens[ti].Feats = merged

			if anchorCase != "" && anchorCase != "Nom" {
				if adapterCase := caseFromAdapterFeats(adapter.Feats); adapterCase != "" && adapterCase != anchorCase {
					diffs = append(diffs, diffRow{c.ID, gt.Surface, "case-disagreement",
						fmt.Sprintf("adapter said Case=%s; gold grammar_label=%s (Case=%s) — gold wins", adapterCase, anchorLabel, anchorCase)})
				}
			}
			if merged == "" {
				diffs = append(diffs, diffRow{c.ID, gt.Surface, "empty-feats",
					"adapter produced no FEATS and no grammar_label anchor"})
			}
		}
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return err
	}
	if err := writeDiff(diffPath, doc.Name, doc.Version, diffs); err != nil {
		return err
	}
	return nil
}

// resolveAdapter parses the command from FINNESTDB_OMORFI_CMD /
// FINNESTDB_ESTNLTK_CMD or auto-discovers the local venv layout
// (matches parsecore.findSiblingVenvPython for estnltk).
func resolveAdapter(lang string) ([]string, error) {
	switch lang {
	case "FI":
		raw := os.Getenv("FINNESTDB_OMORFI_CMD")
		if raw == "" {
			return nil, fmt.Errorf("FINNESTDB_OMORFI_CMD must be set; e.g. \"$(pwd)/.venv-omorfi/bin/python $(pwd)/scripts/omorfi_adapter_example.py\"")
		}
		return strings.Fields(raw), nil
	case "ET":
		if raw := os.Getenv("FINNESTDB_ESTNLTK_CMD"); raw != "" {
			return strings.Fields(raw), nil
		}
		// Auto-discover: prefer .venv-estnltk in CWD or repo root.
		for _, candidate := range []string{".venv-estnltk/bin/python"} {
			if abs, err := filepath.Abs(candidate); err == nil {
				if _, err := os.Stat(abs); err == nil {
					return []string{abs, "scripts/estnltk_adapter_example.py"}, nil
				}
			}
		}
		return nil, fmt.Errorf("estnltk venv not found; symlink .venv-estnltk into the worktree or set FINNESTDB_ESTNLTK_CMD")
	default:
		return nil, fmt.Errorf("unsupported language %q", lang)
	}
}

// runAdapter spawns the adapter, sends text on stdin, parses the JSON
// reply.
func runAdapter(cmdParts []string, lang, text string) (*adapterOutput, error) {
	if len(cmdParts) == 0 {
		return nil, fmt.Errorf("empty adapter command")
	}
	cmd := exec.Command(cmdParts[0], append(cmdParts[1:], "--lang", lang)...)
	cmd.Stdin = strings.NewReader(text)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w; stderr: %s", err, stderr.String())
	}
	var out adapterOutput
	if err := json.NewDecoder(&stdout).Decode(&out); err != nil {
		if errors := strings.TrimSpace(stderr.String()); errors != "" {
			return nil, fmt.Errorf("decode adapter JSON: %w; stderr: %s", err, errors)
		}
		return nil, fmt.Errorf("decode adapter JSON: %w", err)
	}
	return &out, nil
}

// findAdapterToken locates the adapter token whose form matches the
// gold surface (case-insensitive). Returns (token, true) on match.
func findAdapterToken(atoks []adapterToken, surface string) (adapterToken, bool) {
	for _, t := range atoks {
		if strings.EqualFold(t.Form, surface) {
			return t, true
		}
	}
	return adapterToken{}, false
}

// mergeFeats turns the adapter's feats dict into a UD-canonical string
// and overlays anchorCase (from gold grammar_label) onto Case=. Returns
// "" when nothing maps.
func mergeFeats(adapterFeats map[string]any, anchorCase string) string {
	pairs := map[string]string{}
	for k, v := range adapterFeats {
		s := stringifyFeatsValue(v)
		if s == "" {
			continue
		}
		// Skip the adapter's debugging breadcrumb if present.
		if k == "vabamorf_form" {
			continue
		}
		pairs[k] = s
	}
	// Anchor Case from the gold grammar_label if present and not Nom.
	if anchorCase != "" && anchorCase != "Nom" {
		pairs["Case"] = anchorCase
	} else if anchorCase == "Nom" {
		// Gold says nominative — UD convention is to leave Case unset.
		delete(pairs, "Case")
	}
	if len(pairs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(pairs[k])
	}
	return b.String()
}

func caseFromAdapterFeats(feats map[string]any) string {
	if v, ok := feats["Case"]; ok {
		return stringifyFeatsValue(v)
	}
	return ""
}

func stringifyFeatsValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	default:
		return ""
	}
}

func writeDiff(path, name, version string, rows []diffRow) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# enrichgoldfeats diff — %s %s\n\n", name, version)
	if len(rows) == 0 {
		fmt.Fprintln(&b, "No issues — every gold token received FEATS from the adapter (with gold's Case anchor where applicable).")
		return os.WriteFile(path, []byte(b.String()), 0o644)
	}
	fmt.Fprintf(&b, "%d token(s) need a manual look:\n\n", len(rows))
	fmt.Fprintln(&b, "| case | surface | issue | detail |")
	fmt.Fprintln(&b, "|---|---|---|---|")
	for _, r := range rows {
		// Escape pipes in the detail column.
		detail := strings.ReplaceAll(r.detail, "|", `\|`)
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s |\n", r.caseID, r.surface, r.kind, detail)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// suffixCase walks the longest-first case-suffix table for the language
// and returns the UD Case= value of the first suffix that matches with
// a stem of at least 3 chars. Returns "" when no suffix matches or the
// suffix maps to nominative (Nom is implicit per UD convention).
//
// This is the same heuristic the dict layer's tryCaseSuffixStrip uses;
// shared via the parserules tables to avoid drift.
func suffixCase(surface, lang string) string {
	low := strings.ToLower(surface)
	suffixes := parserules.FinnishCaseSuffixes
	if lang == "ET" {
		suffixes = parserules.EstonianCaseSuffixes
	}
	for _, cs := range suffixes {
		if !strings.HasSuffix(low, cs.Suffix) {
			continue
		}
		stem := low[:len(low)-len(cs.Suffix)]
		if len(stem) < 3 {
			continue
		}
		ud, ok := udfeats.LegacyLabelToUDCase[cs.Label]
		if !ok || ud == "Nom" {
			return ""
		}
		return ud
	}
	return ""
}

func guessLangFromPath(p string) string {
	low := strings.ToLower(p)
	if strings.Contains(low, "/fi/") || strings.Contains(low, "fi-") {
		return "FI"
	}
	if strings.Contains(low, "/et/") || strings.Contains(low, "et-") {
		return "ET"
	}
	return ""
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	io.WriteString(os.Stderr, "\n")
	os.Exit(2)
}
