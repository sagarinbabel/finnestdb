package parsecore

import (
	"slices"
	"testing"
	"time"

	"finnestdb/internal/parserffi"
	"finnestdb/internal/store"
)

func TestAnalyzerTimeoutFallsBackOnUnsetMalformedAndNonPositive(t *testing.T) {
	const env = "FINNESTDB_TEST_TIMEOUT"

	t.Setenv(env, "")
	if got := analyzerTimeout(env, 7*time.Second); got != 7*time.Second {
		t.Fatalf("empty env: got %v want 7s", got)
	}

	t.Setenv(env, "not-a-duration")
	if got := analyzerTimeout(env, 7*time.Second); got != 7*time.Second {
		t.Fatalf("malformed env: got %v want 7s", got)
	}

	t.Setenv(env, "0s")
	if got := analyzerTimeout(env, 7*time.Second); got != 7*time.Second {
		t.Fatalf("zero env: got %v want 7s", got)
	}

	t.Setenv(env, "-5s")
	if got := analyzerTimeout(env, 7*time.Second); got != 7*time.Second {
		t.Fatalf("negative env: got %v want 7s", got)
	}

	t.Setenv(env, "45s")
	if got := analyzerTimeout(env, 7*time.Second); got != 45*time.Second {
		t.Fatalf("override env: got %v want 45s", got)
	}

	t.Setenv(env, "1m30s")
	if got := analyzerTimeout(env, 7*time.Second); got != 90*time.Second {
		t.Fatalf("compound env: got %v want 90s", got)
	}
}

func TestSupportedParsersList(t *testing.T) {
	got := SupportedParsers()
	want := []string{"basic", "custom", "estnltk", "omorfi"}
	if !slices.Equal(got, want) {
		t.Fatalf("supported parsers=%v want %v", got, want)
	}
}

func TestOmorfiRulesPreferCustomFallbackAndTraceIt(t *testing.T) {
	token := &TokenResult{
		Form:     "kirjassani",
		Lemma:    "kirjassani",
		POS:      "X",
		Resolved: false,
		Source:   "analyzer:omorfi",
	}
	direct := store.FormResolution{}
	custom := store.FormResolution{
		Lemma:        "kirja",
		POS:          "NOUN",
		GrammarLabel: "inessive",
		Source:       "possessive",
	}

	applied := defaultExternalAnalyzerRules()[1].Apply("FI", token, direct, custom)
	if !applied {
		t.Fatal("expected custom fallback rule to apply")
	}
	if token.Lemma != "kirja" || token.POS != "NOUN" {
		t.Fatalf("got %s/%s want kirja/NOUN", token.Lemma, token.POS)
	}
	if token.Source != "override:possessive" {
		t.Fatalf("source=%q want override:possessive", token.Source)
	}
	if token.GrammarLabel != "inessive" {
		t.Fatalf("grammar_label=%q want inessive", token.GrammarLabel)
	}
	if len(token.Trace) == 0 {
		t.Fatal("expected trace entry to be recorded")
	}
}

// TestAttachMorphologyRule_AttachesOnLemmaPOSAgreement covers all four
// shapes of (token has label?, token has feats?) × (custom has label?, custom
// has feats?) the attach rule cares about, plus the lemma/POS guards.
//
// The FEATS-only case is the one #129 cares about: the FST emits verb
// morphology like "Mood=Ind|Number=Sing|Person=3|Tense=Pres" with no
// case-derived GrammarLabel. The earlier rule gated on custom.GrammarLabel
// and dropped these on the floor when omorfi already had lemma+POS.
func TestAttachMorphologyRule_AttachesOnLemmaPOSAgreement(t *testing.T) {
	rule := externalAttachMorphologyRule{}
	cases := []struct {
		name      string
		token     TokenResult
		custom    store.FormResolution
		wantApply bool
		wantLabel string
		wantFeats string
		wantTrace string
	}{
		{
			name:      "feats-only attach when token missing FEATS",
			token:     TokenResult{Lemma: "lukea", POS: "VERB"},
			custom:    store.FormResolution{Lemma: "lukea", POS: "VERB", Feats: "Mood=Ind|Number=Sing|Person=3|Tense=Pres"},
			wantApply: true,
			wantLabel: "",
			wantFeats: "Mood=Ind|Number=Sing|Person=3|Tense=Pres",
			wantTrace: "rule:attach_morphology feats=Mood=Ind|Number=Sing|Person=3|Tense=Pres",
		},
		{
			name:      "label-only attach (legacy case-suffix path)",
			token:     TokenResult{Lemma: "kirja", POS: "NOUN"},
			custom:    store.FormResolution{Lemma: "kirja", POS: "NOUN", GrammarLabel: "inessive"},
			wantApply: true,
			wantLabel: "inessive",
			wantFeats: "",
			wantTrace: "rule:attach_morphology label=inessive",
		},
		{
			name:      "label and feats both attached when both missing",
			token:     TokenResult{Lemma: "kirja", POS: "NOUN"},
			custom:    store.FormResolution{Lemma: "kirja", POS: "NOUN", GrammarLabel: "inessive", Feats: "Case=Ine|Number=Sing"},
			wantApply: true,
			wantLabel: "inessive",
			wantFeats: "Case=Ine|Number=Sing",
			wantTrace: "rule:attach_morphology label=inessive feats=Case=Ine|Number=Sing",
		},
		{
			name:      "no-op when token already has both",
			token:     TokenResult{Lemma: "kirja", POS: "NOUN", GrammarLabel: "inessive", Feats: "Case=Ine|Number=Sing"},
			custom:    store.FormResolution{Lemma: "kirja", POS: "NOUN", GrammarLabel: "elative", Feats: "Case=Ela"},
			wantApply: false,
			wantLabel: "inessive",
			wantFeats: "Case=Ine|Number=Sing",
		},
		{
			name:      "no-op when custom is empty",
			token:     TokenResult{Lemma: "kirja", POS: "NOUN"},
			custom:    store.FormResolution{Lemma: "kirja", POS: "NOUN"},
			wantApply: false,
			wantLabel: "",
			wantFeats: "",
		},
		{
			name:      "lemma mismatch blocks feats-only attach",
			token:     TokenResult{Lemma: "lukea", POS: "VERB"},
			custom:    store.FormResolution{Lemma: "luku", POS: "NOUN", Feats: "Case=Nom|Number=Sing"},
			wantApply: false,
			wantLabel: "",
			wantFeats: "",
		},
		{
			name:      "POS mismatch blocks feats-only attach",
			token:     TokenResult{Lemma: "lukea", POS: "NOUN"},
			custom:    store.FormResolution{Lemma: "lukea", POS: "VERB", Feats: "Mood=Ind|Tense=Pres"},
			wantApply: false,
			wantLabel: "",
			wantFeats: "",
		},
		{
			name:      "fills feats but not label when token already has label",
			token:     TokenResult{Lemma: "kirja", POS: "NOUN", GrammarLabel: "inessive"},
			custom:    store.FormResolution{Lemma: "kirja", POS: "NOUN", GrammarLabel: "elative", Feats: "Case=Ine|Number=Sing"},
			wantApply: true,
			wantLabel: "inessive",
			wantFeats: "Case=Ine|Number=Sing",
			wantTrace: "rule:attach_morphology feats=Case=Ine|Number=Sing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := tc.token
			got := rule.Apply("FI", &tok, store.FormResolution{}, tc.custom)
			if got != tc.wantApply {
				t.Fatalf("Apply()=%v want %v", got, tc.wantApply)
			}
			if tok.GrammarLabel != tc.wantLabel {
				t.Errorf("GrammarLabel=%q want %q", tok.GrammarLabel, tc.wantLabel)
			}
			if tok.Feats != tc.wantFeats {
				t.Errorf("Feats=%q want %q", tok.Feats, tc.wantFeats)
			}
			if tc.wantTrace != "" {
				if len(tok.Trace) == 0 || tok.Trace[len(tok.Trace)-1] != tc.wantTrace {
					t.Errorf("trace=%v want last entry %q", tok.Trace, tc.wantTrace)
				}
			}
		})
	}
}

func TestFeatsFromJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty bytes", "", ""},
		{"empty object", `{}`, ""},
		{"null", `null`, ""},
		{"single attr", `{"Case":"Ine"}`, "Case=Ine"},
		{"sorted keys", `{"Number":"Sing","Case":"Ine"}`, "Case=Ine|Number=Sing"},
		{"array value joined", `{"Case":["Ine","Ade"]}`, "Case=Ine,Ade"},
		{"non-string skipped", `{"Case":42}`, ""},
		{"invalid json", `not-json`, ""},
	}
	for _, tc := range cases {
		got := featsFromJSON([]byte(tc.in))
		if got != tc.want {
			t.Errorf("%s: featsFromJSON(%q)=%q want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestExternalAnalyzerParserUsesConfiguredSource(t *testing.T) {
	db, err := store.NewDB(":memory:")
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	parser := externalAnalyzerParser{
		name:   "estnltk",
		lang:   "ET",
		source: "analyzer:estnltk",
		analyzer: func(_, _ string) (*parserffi.AnalysisResult, error) {
			return &parserffi.AnalysisResult{Sentences: []parserffi.Sentence{
				{Tokens: []parserffi.Token{
					{Form: "Poes", Lemma: "pood", POS: "NOUN", GrammarLabel: "inessive"},
				}},
			}}, nil
		},
		overrideSet: defaultExternalAnalyzerRules,
	}

	got, err := parser.Parse(db, "ET", "Poes")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	token := got.Sentences[0].Tokens[0]
	if token.Source != "analyzer:estnltk" {
		t.Fatalf("source=%q want analyzer:estnltk", token.Source)
	}
	if token.Lemma != "pood" || token.POS != "NOUN" || token.GrammarLabel != "inessive" {
		t.Fatalf("token=%+v", token)
	}
}

func TestComputeParseStatsCountsResolutionSourcesAndTimings(t *testing.T) {
	stats := computeParseStats([]SentenceResult{
		{
			Text: "Kirjassani on pankissa.",
			Tokens: []TokenResult{
				{Form: "Kirjassani", Lemma: "kirja", POS: "NOUN", Source: "possessive", Resolved: true},
				{Form: "on", Lemma: "olla", POS: "VERB", Source: "stub", Resolved: false},
				{Form: "pankissa", Lemma: "pankki", POS: "NOUN", Source: "case_suffix", Resolved: true},
				{Form: ".", Lemma: ".", POS: "PUNCT", Source: "punct"},
			},
		},
	}, 3, ParseTimings{
		AnalyzeNs:          4_000_000,
		LookupFormsNs:      3_000_000,
		LookupGlossesNs:    2_000_000,
		ResolveSentencesNs: 1_000_000,
		EnrichWordsNs:      1_000_000,
		TotalNs:            11_000_000,
	})

	if stats.UniqueForms != 3 {
		t.Fatalf("unique_forms=%d want 3", stats.UniqueForms)
	}
	if stats.TotalSentences != 1 {
		t.Fatalf("total_sentences=%d want 1", stats.TotalSentences)
	}
	if stats.ResolvedTokens != 2 {
		t.Fatalf("resolved_tokens=%d want 2", stats.ResolvedTokens)
	}
	if stats.UnresolvedTokens != 1 {
		t.Fatalf("unresolved_tokens=%d want 1", stats.UnresolvedTokens)
	}
	if stats.PunctTokens != 1 {
		t.Fatalf("punct_tokens=%d want 1", stats.PunctTokens)
	}
	if stats.SourceCounts["possessive"] != 1 || stats.SourceCounts["stub"] != 1 || stats.SourceCounts["punct"] != 1 {
		t.Fatalf("unexpected source counts: %#v", stats.SourceCounts)
	}
	if stats.Timings.TotalNs != 11_000_000 {
		t.Fatalf("total_ns=%d want 11_000_000", stats.Timings.TotalNs)
	}
}
