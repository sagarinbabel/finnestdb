package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestClassifyMorphCode(t *testing.T) {
	cases := []struct {
		code string
		want morphFormClass
	}{
		{"IndPrSg1", morphFormVerbal},
		{"ImpPrPl2", morphFormVerbal},
		{"KndPtIps", morphFormVerbal},
		{"Sup", morphFormVerbal},
		{"SupIn", morphFormVerbal},
		{"PtsPrPs", morphFormVerbal},
		{"PtsPtIpsNeg", morphFormVerbal},
		{"Inf", morphFormVerbal},
		{"Ger", morphFormVerbal},
		{"SgN", morphFormNominal},
		{"SgG", morphFormNominal},
		{"PlAdt", morphFormNominal},
		{"ID", morphFormNominal},
		{"", morphFormUnknown},
		{"WeirdCode", morphFormUnknown},
	}
	for _, tc := range cases {
		got := classifyMorphCode(tc.code)
		if got != tc.want {
			t.Errorf("classifyMorphCode(%q): got %d, want %d", tc.code, got, tc.want)
		}
	}
}

func TestPosMatchesMorphClass(t *testing.T) {
	// Verbal codes match VERB only.
	if !posMatchesMorphClass("VERB", morphFormVerbal) {
		t.Error("VERB should match verbal form")
	}
	if posMatchesMorphClass("NOUN", morphFormVerbal) {
		t.Error("NOUN should not match verbal form")
	}
	// Nominal codes match non-VERB POSes.
	if !posMatchesMorphClass("NOUN", morphFormNominal) {
		t.Error("NOUN should match nominal form")
	}
	if !posMatchesMorphClass("ADJ", morphFormNominal) {
		t.Error("ADJ should match nominal form")
	}
	if posMatchesMorphClass("VERB", morphFormNominal) {
		t.Error("VERB should not match nominal form")
	}
	// Unknown codes are permissive (defensive fallback).
	if !posMatchesMorphClass("VERB", morphFormUnknown) {
		t.Error("VERB should match unknown form (fallback)")
	}
	if !posMatchesMorphClass("NOUN", morphFormUnknown) {
		t.Error("NOUN should match unknown form (fallback)")
	}
}

func TestJoinTranslations_DedupesCaseInsensitive(t *testing.T) {
	entry := definitionEntry{Meanings: []struct {
		POS            []string `json:"pos"`
		DefinitionsET  []string `json:"definitions_et"`
		TranslationsEN []string `json:"translations_en"`
		UsagesET       []string `json:"usages_et"`
	}{
		{TranslationsEN: []string{"line", "stripe", " line ", ""}},
		{TranslationsEN: []string{"Line", "feature", "shape"}},
	}}
	got := joinTranslations(entry)
	want := "feature; line; shape; stripe"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJoinTranslations_Empty(t *testing.T) {
	if got := joinTranslations(definitionEntry{}); got != "" {
		t.Errorf("expected empty for no meanings, got %q", got)
	}
}

func TestImporter_EndToEnd_HandlesHomonymsAndEmptyGlossGuard(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	formsDir := filepath.Join(tmp, "forms")
	if err := writeAll(defDir, "j.jsonl",
		// jooma#1 — verb (drink), with EN translations
		`{"word_id":1,"lemma":"jooma","homonym_nr":1,"word_class":"verb","meanings":[{"pos":["v"],"translations_en":["drink"]}]}`+"\n"+
			// jooma#2 — noun (drinking party), Estonian def only, no EN translations
			`{"word_id":2,"lemma":"jooma","homonym_nr":2,"word_class":"noomen","meanings":[{"pos":["s"],"definitions_et":["joomine"]}]}`+"\n"+
			// joon — noun (line), with EN translations
			`{"word_id":3,"lemma":"joon","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["line","stripe"]}]}`+"\n"+
			// invariant — falls through wordClassFallback
			`{"word_id":4,"lemma":"24/7","homonym_nr":1,"word_class":"muutumatu","meanings":[{"pos":["adv"],"translations_en":["all the time"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write definitions: %v", err)
	}
	if err := writeAll(formsDir, "j.tsv",
		"lemma\tform\tmorph_code\n"+
			// jooma's verbal forms
			"jooma\tjoon\tIndPrSg1\n"+
			"jooma\tjooma\tSup\n"+
			// jooma's nominal forms
			"jooma\tjooma\tSgN\n"+
			"jooma\tjoomad\tPlN\n"+
			// joon's nominal forms
			"joon\tjoon\tSgN\n"+
			"joon\tjoone\tSgG\n",
	); err != nil {
		t.Fatalf("write forms: %v", err)
	}

	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := ensureSchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}

	// Pre-existing kaikki gloss for jooma/NOUN — must NOT be overwritten by
	// Ekilex's empty-gloss case (jooma#2 has only ET definitions, no EN).
	if _, err := db.Exec(`INSERT INTO lemmas (lemma, pos, gloss, lang) VALUES ('jooma', 'NOUN', 'kaikki gloss for noun', 'ET')`); err != nil {
		t.Fatalf("seed kaikki: %v", err)
	}

	lemmaPOS, lemmaCount, err := importDefinitions(db, defDir)
	if err != nil {
		t.Fatalf("importDefinitions: %v", err)
	}
	if lemmaCount == 0 {
		t.Error("expected non-zero lemma count")
	}

	formCount, err := importForms(db, formsDir, lemmaPOS)
	if err != nil {
		t.Fatalf("importForms: %v", err)
	}
	if formCount == 0 {
		t.Error("expected non-zero form count")
	}

	// jooma should have both VERB and NOUN entries.
	var verbGloss, nounGloss string
	if err := db.QueryRow(`SELECT gloss FROM lemmas WHERE lemma='jooma' AND pos='VERB' AND lang='ET'`).Scan(&verbGloss); err != nil {
		t.Fatalf("query jooma VERB: %v", err)
	}
	if verbGloss != "drink" {
		t.Errorf("jooma VERB gloss = %q, want %q", verbGloss, "drink")
	}
	if err := db.QueryRow(`SELECT gloss FROM lemmas WHERE lemma='jooma' AND pos='NOUN' AND lang='ET'`).Scan(&nounGloss); err != nil {
		t.Fatalf("query jooma NOUN: %v", err)
	}
	// Empty-gloss guard preserved the kaikki gloss.
	if nounGloss != "kaikki gloss for noun" {
		t.Errorf("empty-gloss guard failed: jooma NOUN gloss = %q, want kaikki preserved", nounGloss)
	}

	// "joon" form maps to BOTH joon/NOUN and jooma/VERB — the homonym case.
	rows, err := db.Query(`SELECT lemma, pos FROM forms WHERE form='joon' AND lang='ET' ORDER BY lemma, pos`)
	if err != nil {
		t.Fatalf("query joon forms: %v", err)
	}
	defer rows.Close()
	got := []string{}
	for rows.Next() {
		var l, p string
		if err := rows.Scan(&l, &p); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, l+"/"+p)
	}
	want := []string{"jooma/VERB", "joon/NOUN"}
	if !equalStringSlices(got, want) {
		t.Errorf("forms[joon] = %v, want %v", got, want)
	}

	// jooma's verbal form (Sup, which makes "jooma") must NOT be attributed
	// to NOUN — that's the morph-class disambiguation.
	var nounSup int
	if err := db.QueryRow(`SELECT COUNT(*) FROM forms WHERE form='jooma' AND lemma='jooma' AND pos='NOUN' AND lang='ET'`).Scan(&nounSup); err != nil {
		t.Fatalf("query jooma noun jooma: %v", err)
	}
	// SgN of jooma is "jooma" (nominal) — that should be present as NOUN.
	if nounSup != 1 {
		t.Errorf("jooma SgN row missing under NOUN: count=%d", nounSup)
	}

	// 24/7's "all the time" went to ADV (via the prop→PROPN, adv→ADV mapping).
	var advGloss string
	if err := db.QueryRow(`SELECT gloss FROM lemmas WHERE lemma='24/7' AND pos='ADV' AND lang='ET'`).Scan(&advGloss); err != nil {
		t.Fatalf("query 24/7 ADV: %v", err)
	}
	if advGloss != "all the time" {
		t.Errorf("24/7 ADV gloss = %q, want %q", advGloss, "all the time")
	}
}

func writeAll(dir, name, content string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
