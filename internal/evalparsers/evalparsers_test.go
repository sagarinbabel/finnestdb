package evalparsers

import (
	"slices"
	"strings"
	"testing"
	"time"

	"finnestdb/internal/parsecore"
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

func TestAnalyzeUnsupportedParserKeepsValidationPrecedence(t *testing.T) {
	if _, err := Analyze(nil, "SV", "kissa", "bogus"); err == nil || err.Error() != "language must be FI or ET" {
		t.Fatalf("invalid lang error=%v want language validation", err)
	}

	if _, err := Analyze(nil, "FI", "kissa", "bogus"); err == nil || !strings.Contains(err.Error(), `unsupported parser "bogus"`) {
		t.Fatalf("unsupported parser error=%v", err)
	}
}

func TestOmorfiRulesPreferCustomFallbackAndTraceIt(t *testing.T) {
	token := &parsecore.TokenResult{
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
// shapes of (token has label?, token has feats?) x (custom has label?, custom
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
		token     parsecore.TokenResult
		custom    store.FormResolution
		wantApply bool
		wantLabel string
		wantFeats string
		wantTrace string
	}{
		{
			name:      "feats-only attach when token missing FEATS",
			token:     parsecore.TokenResult{Lemma: "lukea", POS: "VERB"},
			custom:    store.FormResolution{Lemma: "lukea", POS: "VERB", Feats: "Mood=Ind|Number=Sing|Person=3|Tense=Pres"},
			wantApply: true,
			wantLabel: "",
			wantFeats: "Mood=Ind|Number=Sing|Person=3|Tense=Pres",
			wantTrace: "rule:attach_morphology feats=Mood=Ind|Number=Sing|Person=3|Tense=Pres",
		},
		{
			name:      "label-only attach (legacy case-suffix path)",
			token:     parsecore.TokenResult{Lemma: "kirja", POS: "NOUN"},
			custom:    store.FormResolution{Lemma: "kirja", POS: "NOUN", GrammarLabel: "inessive"},
			wantApply: true,
			wantLabel: "inessive",
			wantFeats: "",
			wantTrace: "rule:attach_morphology label=inessive",
		},
		{
			name:      "label and feats both attached when both missing",
			token:     parsecore.TokenResult{Lemma: "kirja", POS: "NOUN"},
			custom:    store.FormResolution{Lemma: "kirja", POS: "NOUN", GrammarLabel: "inessive", Feats: "Case=Ine|Number=Sing"},
			wantApply: true,
			wantLabel: "inessive",
			wantFeats: "Case=Ine|Number=Sing",
			wantTrace: "rule:attach_morphology label=inessive feats=Case=Ine|Number=Sing",
		},
		{
			name:      "no-op when token already has both",
			token:     parsecore.TokenResult{Lemma: "kirja", POS: "NOUN", GrammarLabel: "inessive", Feats: "Case=Ine|Number=Sing"},
			custom:    store.FormResolution{Lemma: "kirja", POS: "NOUN", GrammarLabel: "elative", Feats: "Case=Ela"},
			wantApply: false,
			wantLabel: "inessive",
			wantFeats: "Case=Ine|Number=Sing",
		},
		{
			name:      "no-op when custom is empty",
			token:     parsecore.TokenResult{Lemma: "kirja", POS: "NOUN"},
			custom:    store.FormResolution{Lemma: "kirja", POS: "NOUN"},
			wantApply: false,
			wantLabel: "",
			wantFeats: "",
		},
		{
			name:      "lemma mismatch blocks feats-only attach",
			token:     parsecore.TokenResult{Lemma: "lukea", POS: "VERB"},
			custom:    store.FormResolution{Lemma: "luku", POS: "NOUN", Feats: "Case=Nom|Number=Sing"},
			wantApply: false,
			wantLabel: "",
			wantFeats: "",
		},
		{
			name:      "POS mismatch blocks feats-only attach",
			token:     parsecore.TokenResult{Lemma: "lukea", POS: "NOUN"},
			custom:    store.FormResolution{Lemma: "lukea", POS: "VERB", Feats: "Mood=Ind|Tense=Pres"},
			wantApply: false,
			wantLabel: "",
			wantFeats: "",
		},
		{
			name:      "fills feats but not label when token already has label",
			token:     parsecore.TokenResult{Lemma: "kirja", POS: "NOUN", GrammarLabel: "inessive"},
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

func TestExternalAnalyzerUsesConfiguredSource(t *testing.T) {
	db, err := store.NewDB(":memory:")
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	got, err := parsecore.AnalyzeWithExternalAnalyzer(db, "ET", "Poes", parsecore.ExternalAnalyzerConfig{
		Name:   "estnltk",
		Lang:   "ET",
		Source: "analyzer:estnltk",
		Analyze: func(_, _ string) (*parserffi.AnalysisResult, error) {
			return &parserffi.AnalysisResult{Sentences: []parserffi.Sentence{
				{Tokens: []parserffi.Token{
					{Form: "Poes", Lemma: "pood", POS: "NOUN", GrammarLabel: "inessive"},
				}},
			}}, nil
		},
		Rules: defaultExternalAnalyzerRules(),
	})
	if err != nil {
		t.Fatalf("AnalyzeWithExternalAnalyzer: %v", err)
	}
	token := got.Sentences[0].Tokens[0]
	if token.Source != "analyzer:estnltk" {
		t.Fatalf("source=%q want analyzer:estnltk", token.Source)
	}
	if token.Lemma != "pood" || token.POS != "NOUN" || token.GrammarLabel != "inessive" {
		t.Fatalf("token=%+v", token)
	}
}
