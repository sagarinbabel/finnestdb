package main

import (
	"strings"
	"testing"

	"finnestdb/pkg/lemmatizer-fi-et/giellaltmap"
	"finnestdb/pkg/lemmatizer-fi-et/voikkomap"
)

// fakeAnalyzer is a stand-in for vfst/hfstol used by analyzeWord tests.
// It returns a fixed slice of raw analyser outputs, so we can simulate
// the multi-reading scenarios that triggered the PR #148 P1 review
// without committing an actual transducer blob.
type fakeAnalyzer struct {
	raw []string
}

func (f *fakeAnalyzer) Analyze(string) []string { return f.raw }
func (f *fakeAnalyzer) Close() error             { return nil }

// etParse mirrors the production ET parseLine built inside openBackend.
// Duplicating it here lets the test exercise the real giellaltmap
// projection without exporting the closure or restructuring main.
func etParse(raw string) (voikkomap.Analysis, bool) {
	g := giellaltmap.Parse(raw)
	if g.Lemma == "" || g.UPOS == "" {
		return voikkomap.Analysis{}, false
	}
	return voikkomap.Analysis{
		Lemma:        g.Lemma,
		UPOS:         g.UPOS,
		GrammarLabel: g.GrammarLabel,
		Number:       g.Number,
		Tense:        g.Tense,
		Mood:         g.Mood,
		Person:       g.Person,
		Feats:        g.Feats,
		Raw:          g.Raw,
	}, true
}

// openBackend's validation branches are pure (no file I/O), so we can
// exercise them without committing any analyser blobs. The tests assert
// (a) language is required, (b) each language requires its own
// transducer flag, and (c) the wrong-flag combinations fail loudly.
func TestOpenBackend_FlagValidation(t *testing.T) {
	cases := []struct {
		name       string
		lang       string
		vfstPath   string
		hfstolPath string
		wantErrSub string
	}{
		{name: "fi requires vfst", lang: "fi", wantErrSub: "-vfst is required for -lang fi"},
		{name: "fi rejects hfstol", lang: "fi", vfstPath: "ignored", hfstolPath: "x.hfstol", wantErrSub: "-hfstol must not be set for -lang fi"},
		{name: "et requires hfstol", lang: "et", wantErrSub: "-hfstol is required for -lang et"},
		{name: "et rejects vfst", lang: "et", hfstolPath: "ignored", vfstPath: "x.vfst", wantErrSub: "-vfst must not be set for -lang et"},
		{name: "unknown lang", lang: "sv", wantErrSub: `unsupported -lang "sv"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := openBackend(tc.lang, tc.vfstPath, tc.hfstolPath)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrSub)
			}
		})
	}
}

// Regression test for the PR #148 P1 review. Previously the generator
// sort.Slice'd per-word analyses by GrammarLabel, which silently
// realphabetised cases on multi-reading ET surface forms. With the
// analyser emitting "maja" in priority order Nom→Gen→Par, the alphabetic
// sort reordered to Gen→Nom→Par, and downstream merging
// (internal/store::mergeAndRankDictFSTCandidates) - which treats the
// first analysis on ties as priority - would mark nominative base forms
// like "maja" as genitive. analyzeWord must preserve analyser order;
// reproducibility comes from FST determinism, not re-sorting.
func TestAnalyzeWord_PreservesAnalyserOrder(t *testing.T) {
	fake := &fakeAnalyzer{raw: []string{
		"maja+N+Sg+Nom",
		"maja+N+Sg+Gen",
		"maja+N+Sg+Par",
	}}
	got := analyzeWord(fake, etParse, "maja")
	wantCases := []string{"nominative", "genitive", "partitive"}
	if len(got) != len(wantCases) {
		t.Fatalf("got %d analyses, want %d (%v)", len(got), len(wantCases), wantCases)
	}
	for i, a := range got {
		if a.GrammarLabel != wantCases[i] {
			t.Errorf("position %d: GrammarLabel=%q, want %q (case order shuffled - reintroduced sort regression?)", i, a.GrammarLabel, wantCases[i])
		}
	}
}

// Empty / unparseable raw lines from the analyser are dropped, but the
// surviving analyses keep their original relative order.
func TestAnalyzeWord_DropsUnparseableLinesPreservingOrder(t *testing.T) {
	fake := &fakeAnalyzer{raw: []string{
		"maja+N+Sg+Nom",
		"",                  // empty: dropped
		"maja+N+Sg+Gen",
		"+++garbage",        // unparseable lemma: dropped
		"maja+N+Sg+Par",
	}}
	got := analyzeWord(fake, etParse, "maja")
	wantCases := []string{"nominative", "genitive", "partitive"}
	if len(got) != len(wantCases) {
		t.Fatalf("got %d analyses, want %d", len(got), len(wantCases))
	}
	for i, a := range got {
		if a.GrammarLabel != wantCases[i] {
			t.Errorf("position %d: GrammarLabel=%q, want %q", i, a.GrammarLabel, wantCases[i])
		}
	}
}

// Lang dispatch is case-insensitive for both fi and et so the Makefile
// targets, env vars, and ad-hoc CLI invocations all reach the right
// backend regardless of the user's casing convention.
func TestOpenBackend_LangCaseInsensitive(t *testing.T) {
	for _, lang := range []string{"FI", "Fi", "fI"} {
		_, _, err := openBackend(lang, "", "")
		if err == nil || !strings.Contains(err.Error(), "-vfst is required") {
			t.Errorf("lang=%q: expected fi dispatch (vfst-required error), got %v", lang, err)
		}
	}
	for _, lang := range []string{"ET", "Et", "eT"} {
		_, _, err := openBackend(lang, "", "")
		if err == nil || !strings.Contains(err.Error(), "-hfstol is required") {
			t.Errorf("lang=%q: expected et dispatch (hfstol-required error), got %v", lang, err)
		}
	}
}
