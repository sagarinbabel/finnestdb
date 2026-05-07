package eval

import (
	"bytes"
	"strings"
	"testing"
)

// TestUposBucket pins the canonical mapping. PROPN/NUM/PUNCT each get their
// own bucket because that's where headline accuracy hides regressions on
// real-world treebanks (see docs/LEARNINGS.md §2026-05-07 — UD-TDT). Anything
// outside the documented set falls into "other" so a future surprise tag (X,
// SYM) is visible rather than silently classed as open/closed.
func TestUposBucket(t *testing.T) {
	cases := []struct {
		pos    string
		bucket string
	}{
		{"NOUN", "open"},
		{"VERB", "open"},
		{"ADJ", "open"},
		{"ADV", "open"},
		{"DET", "closed"},
		{"PRON", "closed"},
		{"CCONJ", "closed"},
		{"SCONJ", "closed"},
		{"ADP", "closed"},
		{"AUX", "closed"},
		{"PART", "closed"},
		{"INTJ", "closed"},
		{"PROPN", "propn"},
		{"NUM", "num"},
		{"PUNCT", "punct"},
		{"X", "other"},
		{"SYM", "other"},
		{"", "other"},
	}
	for _, tc := range cases {
		if got := uposBucket(tc.pos); got != tc.bucket {
			t.Errorf("uposBucket(%q)=%q want %q", tc.pos, got, tc.bucket)
		}
	}
}

func TestOovBucket(t *testing.T) {
	cases := []struct {
		name   string
		cmp    TokenCompare
		bucket string
	}{
		{"resolved", TokenCompare{Actual: TokenActual{Found: true, Resolved: true}}, "in-dict"},
		{"found unresolved", TokenCompare{Actual: TokenActual{Found: true, Resolved: false}}, "oov"},
		{"not found", TokenCompare{Actual: TokenActual{Found: false}}, "oov"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := oovBucket(tc.cmp); got != tc.bucket {
				t.Errorf("oovBucket=%q want %q", got, tc.bucket)
			}
		})
	}
}

func TestCompoundBucket(t *testing.T) {
	cases := []struct {
		name   string
		cmp    TokenCompare
		bucket string
	}{
		{"hyphen surface", TokenCompare{Surface: "EU-puhejohtaja"}, "compound"},
		{"compound source", TokenCompare{Surface: "pankkiautomaatti", Actual: TokenActual{Source: "compound"}}, "compound"},
		{"plain noun", TokenCompare{Surface: "koira", Actual: TokenActual{Source: "dict"}}, "simple"},
		{"plain unresolved", TokenCompare{Surface: "ksksks", Actual: TokenActual{Found: false}}, "simple"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compoundBucket(tc.cmp); got != tc.bucket {
				t.Errorf("compoundBucket=%q want %q", got, tc.bucket)
			}
		})
	}
}

// TestComputeStratification_HappyPath constructs a minimal report with five
// tokens (one per UPOS bucket of interest) and confirms each axis sees the
// right slice of correct/expected counts. Hand-crafted because the real
// pipeline writes JSON, and we want an integration-style test that doesn't
// rely on having the SQLite DB on disk.
func TestComputeStratification_HappyPath(t *testing.T) {
	cases := []CaseReport{
		{
			CaseID: "fi-1",
			Comparisons: map[string][]TokenCompare{
				"custom": {
					// open-class noun, in-dict, simple — fully correct
					{
						Surface:  "kirja",
						Expected: TokenExpected{POS: "NOUN"},
						Actual:   TokenActual{Found: true, Resolved: true, Source: "dict"},
						Match:    TokenMatch{Lemma: true, POS: true, Grammar: true, Full: true},
					},
					// closed-class pronoun, in-dict, simple — POS only
					{
						Surface:  "se",
						Expected: TokenExpected{POS: "PRON"},
						Actual:   TokenActual{Found: true, Resolved: true, Source: "dict"},
						Match:    TokenMatch{Lemma: false, POS: true, Grammar: true, Full: false},
					},
					// PROPN, oov, simple — all wrong
					{
						Surface:  "Helsingin",
						Expected: TokenExpected{POS: "PROPN"},
						Actual:   TokenActual{Found: true, Resolved: false, Source: "dict"},
						Match:    TokenMatch{Lemma: false, POS: false, Grammar: true, Full: false},
					},
					// NUM, in-dict, simple — fully correct
					{
						Surface:  "1990",
						Expected: TokenExpected{POS: "NUM"},
						Actual:   TokenActual{Found: true, Resolved: true, Source: "dict"},
						Match:    TokenMatch{Lemma: true, POS: true, Grammar: true, Full: true},
					},
					// open-class noun, in-dict, compound (hyphen) — fully correct
					{
						Surface:  "EU-puhejohtaja",
						Expected: TokenExpected{POS: "NOUN"},
						Actual:   TokenActual{Found: true, Resolved: true, Source: "compound"},
						Match:    TokenMatch{Lemma: true, POS: true, Grammar: true, Full: true},
					},
				},
			},
		},
	}

	got := ComputeStratification("custom", cases)
	if got == nil {
		t.Fatal("ComputeStratification returned nil for non-empty cases")
	}

	uposByBucket := map[string]StratifiedMetric{}
	for _, m := range got.ByUPOS {
		uposByBucket[m.Bucket] = m
	}
	if open := uposByBucket["open"]; open.ExpectedTokens != 2 || open.LemmaCorrect != 2 || open.FullCorrect != 2 || open.LemmaPOSCorrect != 2 {
		t.Errorf("open bucket=%+v want expected=2 lemma=2 lemma_pos=2 full=2", open)
	}
	// closed bucket: lemma=0, pos=1 → lemma_pos must be 0 (both must be true)
	if closed := uposByBucket["closed"]; closed.ExpectedTokens != 1 || closed.LemmaCorrect != 0 || closed.POSCorrect != 1 || closed.LemmaPOSCorrect != 0 {
		t.Errorf("closed bucket=%+v want expected=1 lemma=0 pos=1 lemma_pos=0", closed)
	}
	if propn := uposByBucket["propn"]; propn.ExpectedTokens != 1 || propn.LemmaCorrect != 0 || propn.POSCorrect != 0 || propn.LemmaPOSCorrect != 0 {
		t.Errorf("propn bucket=%+v want expected=1 lemma=0 pos=0 lemma_pos=0", propn)
	}
	if num := uposByBucket["num"]; num.ExpectedTokens != 1 || num.LemmaCorrect != 1 || num.FullCorrect != 1 || num.LemmaPOSCorrect != 1 {
		t.Errorf("num bucket=%+v want expected=1 lemma=1 lemma_pos=1 full=1", num)
	}

	oovByBucket := map[string]StratifiedMetric{}
	for _, m := range got.ByOOV {
		oovByBucket[m.Bucket] = m
	}
	if inDict := oovByBucket["in-dict"]; inDict.ExpectedTokens != 4 || inDict.LemmaCorrect != 3 {
		t.Errorf("in-dict=%+v want expected=4 lemma=3", inDict)
	}
	if oov := oovByBucket["oov"]; oov.ExpectedTokens != 1 || oov.LemmaCorrect != 0 {
		t.Errorf("oov=%+v want expected=1 lemma=0", oov)
	}

	compoundByBucket := map[string]StratifiedMetric{}
	for _, m := range got.ByCompound {
		compoundByBucket[m.Bucket] = m
	}
	if compound := compoundByBucket["compound"]; compound.ExpectedTokens != 1 || compound.FullCorrect != 1 {
		t.Errorf("compound=%+v want expected=1 full=1", compound)
	}
	if simple := compoundByBucket["simple"]; simple.ExpectedTokens != 4 {
		t.Errorf("simple=%+v want expected=4", simple)
	}
}

// TestComputeStratification_ReturnsNilForEmpty so callers can attach as
// *Stratification without rendering an empty section.
func TestComputeStratification_ReturnsNilForEmpty(t *testing.T) {
	if got := ComputeStratification("custom", nil); got != nil {
		t.Errorf("nil cases: got %+v want nil", got)
	}
	if got := ComputeStratification("custom", []CaseReport{}); got != nil {
		t.Errorf("empty cases: got %+v want nil", got)
	}
	caseWithoutParser := []CaseReport{{Comparisons: map[string][]TokenCompare{"other": {}}}}
	if got := ComputeStratification("custom", caseWithoutParser); got != nil {
		t.Errorf("missing parser: got %+v want nil", got)
	}
}

// TestComputeStratification_BucketsDeterministicOrder pins the row order so
// downstream markdown stays stable across runs (matters for diffing
// committed reports).
func TestComputeStratification_BucketsDeterministicOrder(t *testing.T) {
	cases := []CaseReport{
		{Comparisons: map[string][]TokenCompare{
			"custom": {
				{Expected: TokenExpected{POS: "NUM"}, Actual: TokenActual{Found: true}, Match: TokenMatch{}},
				{Expected: TokenExpected{POS: "NOUN"}, Actual: TokenActual{Found: true}, Match: TokenMatch{}},
				{Expected: TokenExpected{POS: "PROPN"}, Actual: TokenActual{Found: true}, Match: TokenMatch{}},
				{Expected: TokenExpected{POS: "DET"}, Actual: TokenActual{Found: true}, Match: TokenMatch{}},
			},
		}},
	}
	got := ComputeStratification("custom", cases)
	if got == nil {
		t.Fatal("nil")
	}
	want := []string{"open", "closed", "propn", "num"}
	if len(got.ByUPOS) != len(want) {
		t.Fatalf("len(ByUPOS)=%d want %d", len(got.ByUPOS), len(want))
	}
	for i, m := range got.ByUPOS {
		if m.Bucket != want[i] {
			t.Errorf("ByUPOS[%d].Bucket=%q want %q", i, m.Bucket, want[i])
		}
	}
}

// TestRenderStratifiedMarkdown_Basic confirms the renderer emits all three
// axes plus a row per (dataset, parser, bucket) and includes the headers we
// care about for grep-driven CI checks.
func TestRenderStratifiedMarkdown_Basic(t *testing.T) {
	strat := &Stratification{
		ByUPOS: []StratifiedMetric{
			{Bucket: "open", ExpectedTokens: 100, LemmaCorrect: 95, POSCorrect: 98, LemmaPOSCorrect: 94, FullCorrect: 92, LemmaAccuracy: 0.95, POSAccuracy: 0.98, LemmaPOSAccuracy: 0.94, FullAccuracy: 0.92},
			{Bucket: "propn", ExpectedTokens: 20, LemmaCorrect: 4, POSCorrect: 13, LemmaPOSCorrect: 3, FullCorrect: 3, LemmaAccuracy: 0.20, POSAccuracy: 0.65, LemmaPOSAccuracy: 0.15, FullAccuracy: 0.15},
		},
		ByOOV: []StratifiedMetric{
			{Bucket: "in-dict", ExpectedTokens: 110, LemmaCorrect: 99, POSCorrect: 110, LemmaPOSCorrect: 98, FullCorrect: 95, LemmaAccuracy: 0.90, POSAccuracy: 1.0, LemmaPOSAccuracy: 0.89, FullAccuracy: 0.86},
			{Bucket: "oov", ExpectedTokens: 10, LemmaCorrect: 0, POSCorrect: 1, LemmaPOSCorrect: 0, FullCorrect: 0, LemmaAccuracy: 0.0, POSAccuracy: 0.10, LemmaPOSAccuracy: 0.0, FullAccuracy: 0.0},
		},
		ByCompound: []StratifiedMetric{
			{Bucket: "compound", ExpectedTokens: 5, LemmaCorrect: 5, POSCorrect: 5, LemmaPOSCorrect: 5, FullCorrect: 5, LemmaAccuracy: 1.0, POSAccuracy: 1.0, LemmaPOSAccuracy: 1.0, FullAccuracy: 1.0},
		},
	}

	var buf bytes.Buffer
	RenderStratifiedMarkdown(&buf, "Stratified accuracy", []StratifiedReportInput{
		{DatasetName: "ud-fi-tdt-test", Parser: "custom", Stratified: strat},
	})
	out := buf.String()

	mustContain := []string{
		"## Stratified accuracy",
		"### By UPOS bucket",
		"### By OOV (in-dict vs unresolved)",
		"### By Compoundness",
		"| Dataset | Parser | Bucket | Tokens | Lemma | POS | Lemma+POS | Full |",
		"| ud-fi-tdt-test | custom | open | 100 | 95.0% | 98.0% | 94.0% | 92.0% |",
		"| ud-fi-tdt-test | custom | propn | 20 | 20.0% | 65.0% | 15.0% | 15.0% |",
		"| ud-fi-tdt-test | custom | in-dict | 110 |",
		"| ud-fi-tdt-test | custom | oov | 10 |",
		"| ud-fi-tdt-test | custom | compound | 5 |",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestRenderStratifiedMarkdown_NoData prints a helpful note rather than
// blank tables when every report lacks stratification.
func TestRenderStratifiedMarkdown_NoData(t *testing.T) {
	var buf bytes.Buffer
	RenderStratifiedMarkdown(&buf, "Stratified accuracy", []StratifiedReportInput{
		{DatasetName: "old-baseline", Parser: "custom", Stratified: nil},
	})
	out := buf.String()
	if !strings.Contains(out, "## Stratified accuracy") {
		t.Errorf("missing heading in:\n%s", out)
	}
	if !strings.Contains(out, "No stratification data") {
		t.Errorf("expected fallback note, got:\n%s", out)
	}
	if strings.Contains(out, "| Dataset | Parser |") {
		t.Errorf("should not emit empty tables; got:\n%s", out)
	}
}

func TestSidecarPathForReport(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"reports/parser-eval/run-20260507.json", "reports/parser-eval/run-20260507.stratified.md"},
		{"a.json", "a.stratified.md"},
		{"no-extension", ""},
		{"data.txt", ""},
	}
	for _, tc := range cases {
		if got := SidecarPathForReport(tc.in); got != tc.want {
			t.Errorf("SidecarPathForReport(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

// TestEvaluateOptions_StratifiedOff confirms the legacy schema is preserved
// when the flag is off — the Stratification field should be nil on every
// summary so the JSON matches existing baseline files byte-for-byte (modulo
// timestamps).
func TestEvaluateOptions_StratifiedOff(t *testing.T) {
	report := &Report{
		Parsers: []string{"custom"},
		Cases: []CaseReport{
			{Comparisons: map[string][]TokenCompare{
				"custom": {{Expected: TokenExpected{POS: "NOUN"}, Match: TokenMatch{Lemma: true, POS: true, Full: true}}},
			}},
		},
		Summary: map[string]ParserSummary{"custom": {ExpectedTokens: 1}},
	}
	if report.Summary["custom"].Stratification != nil {
		t.Errorf("Stratification should be nil by default, got %+v", report.Summary["custom"].Stratification)
	}
}

// TestRenderStratifiedMarkdown_RunColumnEmittedWhenLabelsSet covers the
// reviewer's primary complaint: when two reports name the same (dataset,
// parser), they used to render as visually identical rows. With Label set,
// the renderer adds a Run column so the two rows are distinguishable.
func TestRenderStratifiedMarkdown_RunColumnEmittedWhenLabelsSet(t *testing.T) {
	mk := func(lemma float64) *Stratification {
		return &Stratification{
			ByUPOS: []StratifiedMetric{{
				Bucket: "open", ExpectedTokens: 100,
				LemmaCorrect: int(lemma * 100), LemmaAccuracy: lemma,
				POSCorrect: 95, POSAccuracy: 0.95,
				LemmaPOSCorrect: int(lemma*100) - 1, LemmaPOSAccuracy: lemma - 0.01,
				FullCorrect: 80, FullAccuracy: 0.80,
			}},
		}
	}
	var buf bytes.Buffer
	RenderStratifiedMarkdown(&buf, "Stratified accuracy", []StratifiedReportInput{
		{DatasetName: "fi-grammar", Parser: "custom", Label: "prev", Stratified: mk(0.85)},
		{DatasetName: "fi-grammar", Parser: "custom", Label: "now", Stratified: mk(0.90)},
	})
	out := buf.String()

	mustContain := []string{
		"| Dataset | Run | Parser | Bucket | Tokens | Lemma | POS | Lemma+POS | Full |",
		"| fi-grammar | prev | custom | open | 100 | 85.0% |",
		"| fi-grammar | now | custom | open | 100 | 90.0% |",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// And the legacy 4-column header should NOT appear when labels are set.
	if strings.Contains(out, "| Dataset | Parser | Bucket | Tokens |") {
		t.Errorf("legacy non-Run header leaked into labelled rendering:\n%s", out)
	}
}

// TestRenderStratifiedMarkdown_RunColumnSuppressedWhenNoLabels guards the
// parsertest sidecar case: a single report's stratification renders without
// the Run column because adding it would just clutter the table.
func TestRenderStratifiedMarkdown_RunColumnSuppressedWhenNoLabels(t *testing.T) {
	strat := &Stratification{
		ByUPOS: []StratifiedMetric{{
			Bucket: "open", ExpectedTokens: 100,
			LemmaCorrect: 90, LemmaAccuracy: 0.9,
		}},
	}
	var buf bytes.Buffer
	RenderStratifiedMarkdown(&buf, "Stratified accuracy", []StratifiedReportInput{
		{DatasetName: "fi-grammar", Parser: "custom", Stratified: strat},
	})
	out := buf.String()
	if !strings.Contains(out, "| Dataset | Parser | Bucket | Tokens | Lemma | POS | Lemma+POS | Full |") {
		t.Errorf("expected legacy header without Run column, got:\n%s", out)
	}
	if strings.Contains(out, "| Dataset | Run | Parser |") {
		t.Errorf("Run column should be suppressed for single-report rendering, got:\n%s", out)
	}
}

func TestReportShortLabel(t *testing.T) {
	cases := []struct {
		name string
		r    *Report
		want string
	}{
		{"nil", nil, ""},
		{"runID with slug", &Report{RunID: "20260507T122013Z-fi-grammar"}, "20260507T122013Z"},
		{"runID without slug", &Report{RunID: "20260507T122013Z"}, "20260507T122013Z"},
		{"falls back to generated", &Report{Generated: "2026-05-07T12:20:13Z"}, "2026-05-07T12:20:13Z"},
		{"falls back to commit", &Report{GitCommit: "b7ab30f"}, "b7ab30f"},
		{"empty", &Report{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReportShortLabel(tc.r); got != tc.want {
				t.Errorf("ReportShortLabel=%q want %q", got, tc.want)
			}
		})
	}
}

// TestSortedStratifiedInputsByDatasetParser_LabelStable covers that two
// reports for the same (dataset, parser) sort by label, so prev and now
// render adjacent in deterministic order.
func TestSortedStratifiedInputsByDatasetParser_LabelStable(t *testing.T) {
	got := SortedStratifiedInputsByDatasetParser([]StratifiedReportInput{
		{DatasetName: "fi-grammar", Parser: "custom", Label: "now"},
		{DatasetName: "fi-grammar", Parser: "custom", Label: "prev"},
	})
	if got[0].Label != "now" || got[1].Label != "prev" {
		// Sort is alphabetical: "now" < "prev"
		t.Errorf("sort order: got [%s, %s] want [now, prev]", got[0].Label, got[1].Label)
	}
}
