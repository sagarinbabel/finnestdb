package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleConllu = `# sent_id = 1
# text = Koira on kotona.
1	Koira	koira	NOUN	_	Case=Nom|Number=Sing	2	nsubj	_	_
2	on	olla	VERB	_	Mood=Ind|Number=Sing|Person=3|Tense=Pres	0	root	_	_
3	kotona	koti	NOUN	_	Case=Ess|Number=Sing	2	xcomp	_	_
4	.	.	PUNCT	_	_	2	punct	_	_

# sent_id = 2
# text = Koira haukkuu.
1	Koira	koira	NOUN	_	Case=Nom|Number=Sing	2	nsubj	_	_
2	haukkuu	haukkua	VERB	_	Mood=Ind|Number=Sing|Person=3|Tense=Pres	0	root	_	_
3	.	.	PUNCT	_	_	2	punct	_	_

# sent_id = 3
# multi-word token shouldn't double-count
1-2	don't	_	_	_	_	_	_	_	_
1	do	do	AUX	_	_	0	root	_	_
2	n't	not	PART	_	_	1	advmod	_	_

`

func TestDeriveUD(t *testing.T) {
	dir := t.TempDir()
	conllu := filepath.Join(dir, "sample.conllu")
	if err := os.WriteFile(conllu, []byte(sampleConllu), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	formsAll := filepath.Join(dir, "forms.tsv")
	formsWords := filepath.Join(dir, "forms-words.tsv")
	lemmas := filepath.Join(dir, "lemmas.tsv")

	if err := deriveUD(conllu, formsAll, formsWords, lemmas); err != nil {
		t.Fatalf("deriveUD: %v", err)
	}

	all := readTSV(t, formsAll)
	if all["Koira"] != 2 {
		t.Errorf("formsAll: want Koira=2, got %d", all["Koira"])
	}
	if all["."] != 2 {
		t.Errorf("formsAll: want .=2, got %d (punct should appear in formsAll)", all["."])
	}
	if all["don't"] != 0 {
		t.Errorf("formsAll: multi-word token should be skipped, got don't=%d", all["don't"])
	}

	words := readTSV(t, formsWords)
	if words["Koira"] != 2 {
		t.Errorf("formsWords: want Koira=2, got %d", words["Koira"])
	}
	if _, ok := words["."]; ok {
		t.Errorf("formsWords: PUNCT should be filtered out, found .=%d", words["."])
	}

	lems := readTSV(t, lemmas)
	if lems["koira"] != 2 {
		t.Errorf("lemmas: want koira=2, got %d", lems["koira"])
	}
	if lems["olla"] != 1 {
		t.Errorf("lemmas: want olla=1, got %d", lems["olla"])
	}
}

func TestWriteRankedDescByCountThenAsc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.tsv")
	if err := writeRanked(path, map[string]int{
		"alpha": 5,
		"beta":  10,
		"gamma": 5, // tie with alpha; alpha comes first by ascending name
		"delta": 5,
	}); err != nil {
		t.Fatalf("writeRanked: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	want := []string{"beta\t10", "alpha\t5", "delta\t5", "gamma\t5"}
	if len(lines) != len(want) {
		t.Fatalf("line count: got %d want %d (got %v)", len(lines), len(want), lines)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: got %q want %q", i, lines[i], w)
		}
	}
}

func readTSV(t *testing.T, path string) map[string]int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	out := map[string]int{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 2)
		if len(parts) != 2 {
			continue
		}
		var n int
		fmtN := parts[1]
		for _, c := range fmtN {
			if c < '0' || c > '9' {
				t.Fatalf("non-digit in count %q", fmtN)
			}
			n = n*10 + int(c-'0')
		}
		out[parts[0]] = n
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}
