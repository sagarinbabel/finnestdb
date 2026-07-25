package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture is a tiny realistic CoNLL-U snippet exercising the cases the
// importer has to handle correctly:
//   - sent_id and text comments
//   - inessive case (mapped to "inessive" in legacy grammar_label)
//   - nominative case (left empty in legacy grammar_label by convention)
//   - punctuation (UPOS=PUNCT, dropped because eval drops PUNCT before matching)
//   - multi-word-token range (1-2 ranges should be skipped, children kept)
//   - empty/elliptical node (1.1) skipped
//   - blank line between sentences
const fixture = `# sent_id = train-s1
# text = Koira on kotona.
1	Koira	koira	NOUN	_	Case=Nom|Number=Sing	2	nsubj	_	_
2	on	olla	AUX	_	Mood=Ind|Number=Sing|Person=3|Tense=Pres|VerbForm=Fin|Voice=Act	0	root	_	_
3	kotona	koto	NOUN	_	Case=Ess|Number=Sing	2	advmod	_	_
4	.	.	PUNCT	_	_	2	punct	_	_

# sent_id = train-s2
# text = Talossa asuu mies.
1	Talossa	talo	NOUN	_	Case=Ine|Number=Sing	2	obl	_	_
2	asuu	asua	VERB	_	Mood=Ind|Number=Sing|Person=3|Tense=Pres|VerbForm=Fin|Voice=Act	0	root	_	_
3	mies	mies	NOUN	_	Case=Nom|Number=Sing	2	nsubj	_	_
4	.	.	PUNCT	_	_	2	punct	_	_

# sent_id = train-s3
# text = MWT and elliptical handling.
1-2	don't	_	_	_	_	_	_	_	_
1	do	do	AUX	_	Mood=Ind|VerbForm=Fin	3	aux	_	_
2	n't	not	PART	_	Polarity=Neg	3	advmod	_	_
3	test	test	NOUN	_	Case=Nom|Number=Sing	0	root	_	_
3.1	ellipsis	_	_	_	_	_	_	_	_
4	.	.	PUNCT	_	_	3	punct	_	_
`

func TestParseConllu_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.conllu")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	cases, total, skipped, err := parseConllu(f, "ud-fi-test-")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if skipped != 0 {
		t.Errorf("skipped sentences: got %d, want 0", skipped)
	}
	if got, want := len(cases), 3; got != want {
		t.Fatalf("case count: got %d, want %d", got, want)
	}

	// First sentence: koira / olla / koto (essive) / punct.
	c0 := cases[0]
	if c0.ID != "ud-fi-test-train-s1" {
		t.Errorf("c0 id: got %q", c0.ID)
	}
	if c0.Text != "Koira on kotona." {
		t.Errorf("c0 text: got %q", c0.Text)
	}
	if got, want := len(c0.Tokens), 3; got != want {
		t.Fatalf("c0 token count: got %d, want %d (PUNCT should be dropped)", got, want)
	}
	// Koira: Nom → empty grammar_label, but FEATS preserved.
	if c0.Tokens[0].Surface != "Koira" || c0.Tokens[0].Lemma != "koira" || c0.Tokens[0].POS != "NOUN" {
		t.Errorf("c0[0]: got %+v", c0.Tokens[0])
	}
	if c0.Tokens[0].GrammarLabel != "" {
		t.Errorf("c0[0] nominative grammar_label: got %q, want empty", c0.Tokens[0].GrammarLabel)
	}
	if c0.Tokens[0].Feats != "Case=Nom|Number=Sing" {
		t.Errorf("c0[0] feats: got %q", c0.Tokens[0].Feats)
	}
	// kotona: essive.
	if c0.Tokens[2].GrammarLabel != "essive" {
		t.Errorf("c0[2] grammar_label: got %q, want essive", c0.Tokens[2].GrammarLabel)
	}

	// Second sentence: inessive on first noun.
	c1 := cases[1]
	if c1.Tokens[0].GrammarLabel != "inessive" {
		t.Errorf("c1[0] grammar_label: got %q, want inessive", c1.Tokens[0].GrammarLabel)
	}

	// Third sentence: MWT range row dropped, MWT children present, elliptical
	// node dropped, PUNCT row dropped. Expected tokens: do, n't, test.
	c2 := cases[2]
	if got, want := len(c2.Tokens), 3; got != want {
		t.Fatalf("c2 token count: got %d, want %d (MWT range / elliptical / PUNCT not all skipped?)", got, want)
	}
	wantSurfaces := []string{"do", "n't", "test"}
	for i, want := range wantSurfaces {
		if c2.Tokens[i].Surface != want {
			t.Errorf("c2[%d]: got surface %q, want %q", i, c2.Tokens[i].Surface, want)
		}
	}

	// total = 3 + 3 + 3 = 9 after dropping the trailing "." in each sentence.
	if total != 9 {
		t.Errorf("total tokens: got %d, want 9", total)
	}
}

// TestParseConllu_RepeatedSurfacesGetOccurrence regression-tests the codex
// P2 finding on PR #113: when a sentence has the same surface form twice,
// the importer must emit Occurrence>=2 on the second and later instances
// so eval.findOccurrence matches them against the right parsed token.
func TestParseConllu_RepeatedSurfacesGetOccurrence(t *testing.T) {
	const repeated = `# sent_id = s1
# text = the dog ate the bone .
1	the	the	DET	_	Definite=Def	2	det	_	_
2	dog	dog	NOUN	_	Case=Nom	3	nsubj	_	_
3	ate	eat	VERB	_	Tense=Past	0	root	_	_
4	the	the	DET	_	Definite=Def	5	det	_	_
5	bone	bone	NOUN	_	Case=Nom	3	obj	_	_
6	.	.	PUNCT	_	_	3	punct	_	_

# sent_id = s2
# text = a a a
1	a	a	DET	_	_	0	root	_	_
2	a	a	DET	_	_	1	conj	_	_
3	a	a	DET	_	_	1	conj	_	_
`
	dir := t.TempDir()
	path := filepath.Join(dir, "repeated.conllu")
	if err := os.WriteFile(path, []byte(repeated), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	cases, _, _, err := parseConllu(f, "x-")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(cases))
	}

	// First sentence after PUNCT drop: the(1) dog ate the(2) bone.
	got := map[string]int{}
	for _, tok := range cases[0].Tokens {
		key := tok.Surface
		if tok.Occurrence > 0 {
			key = fmt.Sprintf("%s#%d", tok.Surface, tok.Occurrence)
		}
		got[key]++
	}
	if got["the"] != 1 {
		t.Errorf("first 'the' should be implicit (Occurrence=0/omitted): got %+v", got)
	}
	if got["the#2"] != 1 {
		t.Errorf("second 'the' should be Occurrence=2: got %+v", got)
	}

	// Per-sentence reset: second sentence "a a a" resets the counter.
	if cases[1].Tokens[0].Occurrence != 0 {
		t.Errorf("s2 first 'a' Occurrence: got %d, want 0 (per-sentence reset)", cases[1].Tokens[0].Occurrence)
	}
	if cases[1].Tokens[1].Occurrence != 2 {
		t.Errorf("s2 second 'a' Occurrence: got %d, want 2", cases[1].Tokens[1].Occurrence)
	}
	if cases[1].Tokens[2].Occurrence != 3 {
		t.Errorf("s2 third 'a' Occurrence: got %d, want 3", cases[1].Tokens[2].Occurrence)
	}
}

func TestCaseLabelFromFeats(t *testing.T) {
	cases := []struct {
		feats string
		want  string
	}{
		{"", ""},
		{"_", ""}, // shouldn't happen in real input but defensive
		{"Number=Sing", ""},
		{"Case=Nom|Number=Sing", ""}, // nominative left empty
		{"Case=Ine|Number=Sing", "inessive"},
		{"Case=Par|Number=Plur", "partitive"},
		{"Case=Gen", "genitive"},
		{"Number=Plur|Case=All", "allative"}, // not first attribute
		{"Case=Bogus", ""},                   // unknown case left empty
	}
	for _, tc := range cases {
		// "_" is a CoNLL-U sentinel; the importer normalizes it before calling
		// caseLabelFromFeats, but be defensive - empty result is fine.
		feats := tc.feats
		if feats == "_" {
			feats = ""
		}
		got := caseLabelFromFeats(feats)
		if got != tc.want {
			t.Errorf("caseLabelFromFeats(%q): got %q, want %q", tc.feats, got, tc.want)
		}
	}
}

func TestParseConllu_MissingTextReconstructs(t *testing.T) {
	const noText = `# sent_id = s1
1	a	a	NOUN	_	Case=Nom	0	root	_	_
2	b	b	NOUN	_	Case=Nom	1	obl	_	_

`
	dir := t.TempDir()
	path := filepath.Join(dir, "no-text.conllu")
	if err := os.WriteFile(path, []byte(noText), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	cases, _, _, err := parseConllu(f, "x-")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	if !strings.Contains(cases[0].Text, "a") || !strings.Contains(cases[0].Text, "b") {
		t.Errorf("reconstructed text: got %q", cases[0].Text)
	}
}
