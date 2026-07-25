package eval

// Markdown rendering for stratified breakdowns. Pulled out of stratify.go so
// the data layer stays tool-agnostic: cmd/parsertest writes a sidecar
// `<run>.stratified.md` after the JSON report, and cmd/parser-compare emits
// the same tables inline next to the headline. Both routes go through these
// helpers so the formatting stays consistent.
//
// Design choice: each axis is its own H2 with one row per (dataset, parser,
// bucket). Aggregating across datasets would obscure the very thing
// stratification exists to surface - that PROPN regresses on UD-TDT but not
// on fi-grammar, for example.

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// StratifiedReportInput is the per-dataset data the renderer needs. One
// instance per .json file passed to parser-compare; cmd/parsertest builds one
// from its own freshly-evaluated Report.
//
// Label disambiguates rows when multiple reports share the same
// (dataset, parser) - e.g. before/after comparisons or runs from two
// different commits. When set on any input, the renderer emits an extra
// "Run" column carrying the label. Leave empty when only one report per
// (dataset, parser) is being rendered (the parsertest sidecar case) and
// the column is suppressed for readability.
type StratifiedReportInput struct {
	DatasetName string
	Parser      string
	Label       string
	Stratified  *Stratification
}

// RenderStratifiedMarkdown writes the three-axis breakdown tables to w.
// Heading is the H2 title used as a prefix (e.g. "Stratified accuracy"); the
// caller is responsible for any preceding context (H1, headline table, etc.).
// Inputs are emitted in the order given so callers control row ordering.
//
// Empty Stratification slices are skipped. If every input lacks
// stratification, RenderStratifiedMarkdown emits a single-line note instead
// of empty tables.
func RenderStratifiedMarkdown(w io.Writer, heading string, inputs []StratifiedReportInput) {
	type rowKey struct{ axis, name string }
	axes := []struct {
		key    string
		title  string
		extract func(*Stratification) []StratifiedMetric
	}{
		{key: "upos", title: "UPOS bucket", extract: func(s *Stratification) []StratifiedMetric { return s.ByUPOS }},
		{key: "oov", title: "OOV (in-dict vs unresolved)", extract: func(s *Stratification) []StratifiedMetric { return s.ByOOV }},
		{key: "compound", title: "Compoundness", extract: func(s *Stratification) []StratifiedMetric { return s.ByCompound }},
	}

	any := false
	for _, in := range inputs {
		if in.Stratified != nil {
			any = true
			break
		}
	}
	if !any {
		fmt.Fprintf(w, "## %s\n\n", heading)
		fmt.Fprintln(w, "No stratification data attached to the supplied reports.")
		fmt.Fprintln(w, "Run cmd/parsertest with -stratified to embed it, or pass JSONs whose")
		fmt.Fprintln(w, "case-level comparisons are still present (older summary-only baselines")
		fmt.Fprintln(w, "cannot be re-stratified).")
		fmt.Fprintln(w)
		return
	}

	fmt.Fprintf(w, "## %s\n\n", heading)
	fmt.Fprintln(w, "Each row is one (dataset, parser, bucket) slice. ExpectedTokens is the")
	fmt.Fprintln(w, "denominator for accuracy; empty buckets are omitted. See stratify.go for")
	fmt.Fprintln(w, "the bucket definitions.")
	fmt.Fprintln(w)

	// Render the Run column only when at least one input has a Label.
	// Single-report sidecars (parsertest) leave Label empty and skip the
	// column for readability; multi-report comparisons (parser-compare with
	// two JSONs, or -baseline-dir mode) populate Label so prev and now stay
	// distinguishable in the rendered tables.
	hasLabels := false
	for _, in := range inputs {
		if in.Label != "" {
			hasLabels = true
			break
		}
	}

	for _, axis := range axes {
		fmt.Fprintf(w, "### By %s\n\n", axis.title)
		if hasLabels {
			fmt.Fprintln(w, "| Dataset | Run | Parser | Bucket | Tokens | Lemma | POS | Lemma+POS | Full |")
			fmt.Fprintln(w, "|---|---|---|---|---:|---:|---:|---:|---:|")
		} else {
			fmt.Fprintln(w, "| Dataset | Parser | Bucket | Tokens | Lemma | POS | Lemma+POS | Full |")
			fmt.Fprintln(w, "|---|---|---|---:|---:|---:|---:|---:|")
		}
		for _, in := range inputs {
			if in.Stratified == nil {
				continue
			}
			rows := axis.extract(in.Stratified)
			for _, row := range rows {
				if hasLabels {
					fmt.Fprintf(w, "| %s | %s | %s | %s | %d | %.1f%% | %.1f%% | %.1f%% | %.1f%% |\n",
						in.DatasetName, in.Label, in.Parser, row.Bucket,
						row.ExpectedTokens,
						row.LemmaAccuracy*100, row.POSAccuracy*100,
						row.LemmaPOSAccuracy*100, row.FullAccuracy*100)
				} else {
					fmt.Fprintf(w, "| %s | %s | %s | %d | %.1f%% | %.1f%% | %.1f%% | %.1f%% |\n",
						in.DatasetName, in.Parser, row.Bucket,
						row.ExpectedTokens,
						row.LemmaAccuracy*100, row.POSAccuracy*100,
						row.LemmaPOSAccuracy*100, row.FullAccuracy*100)
				}
			}
		}
		fmt.Fprintln(w)
	}
}

// SortedStratifiedInputsByDatasetParser returns the inputs sorted by
// (dataset, parser, label) so two reports for the same dataset stay
// grouped and prev/now (or any same-(dataset, parser) pair) stay adjacent
// in the rendered table. Used by cmd/parser-compare so multi-dataset runs
// print readable tables.
func SortedStratifiedInputsByDatasetParser(inputs []StratifiedReportInput) []StratifiedReportInput {
	out := append([]StratifiedReportInput(nil), inputs...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].DatasetName != out[j].DatasetName {
			return out[i].DatasetName < out[j].DatasetName
		}
		if out[i].Parser != out[j].Parser {
			return out[i].Parser < out[j].Parser
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// ReportShortLabel derives a short, human-readable label from a Report,
// suitable for the "Run" column. Picks the first non-empty value among:
// timestamp prefix from RunID (e.g. "20260507T122013Z" from
// "20260507T122013Z-fi-grammar"), the full RunID, or the generated_at
// timestamp re-rendered without colons. Returns "" only when every option
// is empty - callers should treat that as "label this report as best you
// can but don't crash."
func ReportShortLabel(r *Report) string {
	if r == nil {
		return ""
	}
	if r.RunID != "" {
		// RunID format is "YYYYMMDDTHHmmssZ-slug"; strip the slug so the
		// label fits in one column without echoing the dataset name (which
		// is already a separate column).
		if i := strings.IndexByte(r.RunID, '-'); i > 0 {
			return r.RunID[:i]
		}
		return r.RunID
	}
	if r.Generated != "" {
		return r.Generated
	}
	if r.GitCommit != "" {
		return r.GitCommit
	}
	return ""
}

// SidecarPathForReport returns the path of the stratified markdown sidecar
// for a JSON report at jsonPath ("foo/bar/run-20260507.json" →
// "foo/bar/run-20260507.stratified.md"). Used by cmd/parsertest. A trailing
// .json is required; otherwise returns "" so callers can detect the misuse.
func SidecarPathForReport(jsonPath string) string {
	if !strings.HasSuffix(jsonPath, ".json") {
		return ""
	}
	return strings.TrimSuffix(jsonPath, ".json") + ".stratified.md"
}
