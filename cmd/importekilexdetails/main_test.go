package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"finnestdb/internal/store"

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
	// "Line" wins the dedup tiebreak over "line" (chooseCasing prefers
	// the uppercase-leading variant). Lexicographic sort places "Line"
	// before "feature" because uppercase ASCII sorts before lowercase.
	want := "Line; feature; shape; stripe"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestChooseCasing(t *testing.T) {
	cases := []struct {
		a, b, want string
	}{
		// Real samples from k.jsonl: proper-noun cased version wins.
		{"Calvinism", "calvinism", "Calvinism"},
		{"calvinism", "Calvinism", "Calvinism"},
		{"Turbot", "turbot", "Turbot"},
		// Two title-case forms — lexicographic tiebreak.
		{"Calvinism", "CALVINISM", "CALVINISM"},
		// Two lowercase forms — lexicographic tiebreak.
		{"line", "Line", "Line"},
		{"line", "lyne", "line"},
		// Empty edge case: "" never has uppercase, the other wins.
		{"", "Calvinism", "Calvinism"},
		{"calvinism", "", "calvinism"},
	}
	for _, tc := range cases {
		if got := chooseCasing(tc.a, tc.b); got != tc.want {
			t.Errorf("chooseCasing(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestLemmaPOSMap_CaseTiebreakDeterministic(t *testing.T) {
	// Same dedup result regardless of input order — guards against
	// last-write-wins regressions.
	a := lemmaPOSMap{}
	a.add("kalvados", "NOUN", []string{"calvados", "Calvados"})
	b := lemmaPOSMap{}
	b.add("kalvados", "NOUN", []string{"Calvados", "calvados"})

	if joinTranslationData(a["kalvados"]["NOUN"]) != "Calvados" {
		t.Errorf("a: got %q, want \"Calvados\"", joinTranslationData(a["kalvados"]["NOUN"]))
	}
	if joinTranslationData(b["kalvados"]["NOUN"]) != "Calvados" {
		t.Errorf("b: got %q, want \"Calvados\"", joinTranslationData(b["kalvados"]["NOUN"]))
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

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregateDefinitions: %v", err)
	}
	if countLemmaPOS(lemmaPOS) == 0 {
		t.Error("expected non-zero (lemma, pos) count")
	}

	inserted, glossFilled, err := writeLemmas(db, lemmaPOS)
	if err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}
	if inserted == 0 && glossFilled == 0 {
		t.Error("expected non-zero inserted+glossFilled")
	}

	formInserted, err := importForms(db, formsDir, lemmaPOS)
	if err != nil {
		t.Fatalf("importForms: %v", err)
	}
	if formInserted == 0 {
		t.Error("expected non-zero form rows inserted")
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

// TestImporter_MergesGlossesAcrossHomonyms guards against the P2 regression
// codex flagged: two Ekilex entries that collapse into the same
// (lemma, pos, lang) row (e.g. aste#1 "step" + aste#2 "degree", both NOUN)
// must merge their EN translations into one gloss instead of dropping the
// second insert.
func TestImporter_MergesGlossesAcrossHomonyms(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "a.jsonl",
		`{"word_id":1,"lemma":"aste","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["step","stepping"]}]}`+"\n"+
			`{"word_id":2,"lemma":"aste","homonym_nr":2,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["degree","rank"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := ensureSchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("write: %v", err)
	}

	var gloss string
	if err := db.QueryRow(`SELECT gloss FROM lemmas WHERE lemma='aste' AND pos='NOUN' AND lang='ET'`).Scan(&gloss); err != nil {
		t.Fatalf("query: %v", err)
	}
	want := "degree; rank; step; stepping"
	if gloss != want {
		t.Errorf("merged gloss = %q, want %q", gloss, want)
	}
}

// TestImporter_RunsMultiLemmaMigration guards against the P1 regression
// codex flagged: an existing finnestdb.db imported with the legacy
// (form, lang) PK must be migrated to the multi-lemma PK before the
// importer's INSERT OR IGNORE runs, otherwise ambiguous Ekilex rows like
// joon → joon/NOUN + joon → jooma/VERB collapse to whichever was
// inserted first.
func TestImporter_RunsMultiLemmaMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// Create the LEGACY single-lemma schema explicitly — what cmd/importdict
	// would have left behind on a finnestdb.db built before PR #77.
	if _, err := db.Exec(`
		CREATE TABLE lemmas (
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			gloss TEXT,
			lang  TEXT NOT NULL,
			PRIMARY KEY(lemma, pos, lang)
		);
		CREATE TABLE forms (
			form  TEXT NOT NULL,
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			lang  TEXT NOT NULL,
			PRIMARY KEY (form, lang)
		);
	`); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}

	// Apply the same migration the main flow now runs before any inserts.
	if err := store.EnsureMultiLemmaSchema(db); err != nil {
		t.Fatalf("EnsureMultiLemmaSchema: %v", err)
	}

	// New PK admits two rows for the same (form, lang).
	if _, err := db.Exec(`INSERT INTO forms (form, lemma, pos, lang) VALUES ('joon', 'joon', 'NOUN', 'ET')`); err != nil {
		t.Fatalf("insert noun: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO forms (form, lemma, pos, lang) VALUES ('joon', 'jooma', 'VERB', 'ET')`); err != nil {
		t.Errorf("insert verb candidate: %v (multi-lemma migration didn't apply)", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM forms WHERE form='joon' AND lang='ET'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("forms[joon] count = %d, want 2 (both homonyms preserved)", n)
	}
}

// TestImporter_ReportsActualRowsInserted guards against the P3 regression
// codex flagged: importer counters used to track INSERT attempts even when
// the row was ignored. With RowsAffected they reflect actual writes.
func TestImporter_ReportsActualRowsInserted(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	formsDir := filepath.Join(tmp, "forms")
	if err := writeAll(defDir, "k.jsonl",
		// Two homonym entries that collapse to a single (kass, NOUN) row —
		// only one row is actually written.
		`{"word_id":1,"lemma":"kass","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["cat"]}]}`+"\n"+
			`{"word_id":2,"lemma":"kass","homonym_nr":2,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["pussy"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write defs: %v", err)
	}
	if err := writeAll(formsDir, "k.tsv",
		"lemma\tform\tmorph_code\n"+
			// Same canonical form emitted twice (e.g. PtsPtIps + PtsPtIpsNeg
			// often share a surface) — should still INSERT only once.
			"kass\tkassi\tSgG\n"+
			"kass\tkassi\tSgP\n"+
			"kass\tkass\tSgN\n",
	); err != nil {
		t.Fatalf("write forms: %v", err)
	}

	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := ensureSchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	inserted, _, err := writeLemmas(db, lemmaPOS)
	if err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}
	if inserted != 1 {
		t.Errorf("lemma rows inserted = %d, want 1 (kass#1 + kass#2 collapse to one row)", inserted)
	}

	formInserted, err := importForms(db, formsDir, lemmaPOS)
	if err != nil {
		t.Fatalf("importForms: %v", err)
	}
	// Three TSV rows but only two unique (form, lemma, pos): kassi/SgG and
	// kassi/SgP both yield (kassi, kass, NOUN); kass/SgN yields (kass, kass, NOUN).
	if formInserted != 2 {
		t.Errorf("form rows inserted = %d, want 2 (one duplicate ignored)", formInserted)
	}
}

// TestAggregateDefinitions_TracksBadLines guards against silently swallowing
// JSONL parse errors — a counter and capped sample collection surface
// upstream schema drift in the run summary instead of failing quietly.
func TestAggregateDefinitions_TracksBadLines(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "x.jsonl",
		`{"word_id":1,"lemma":"good","word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["good"]}]}`+"\n"+
			`{this is not valid json`+"\n"+
			`{"word_id":2,"lemma":"","word_class":"noomen","meanings":[{"pos":["s"]}]}`+"\n"+
			// Valid JSON, lemma present, but every meaning's POS is unmappable.
			`{"word_id":3,"lemma":"weirdpos","word_class":"","meanings":[{"pos":["WEIRD"]}]}`+"\n"+
			`{another bad line`+"\n",
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, stats, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if stats.badLines != 2 {
		t.Errorf("badLines = %d, want 2", stats.badLines)
	}
	if stats.emptyLemma != 1 {
		t.Errorf("emptyLemma = %d, want 1", stats.emptyLemma)
	}
	if stats.noPOS != 1 {
		t.Errorf("noPOS = %d, want 1", stats.noPOS)
	}
	if len(stats.badSamples) != 2 {
		t.Errorf("badSamples = %d, want 2 (under maxBadLineSamples cap)", len(stats.badSamples))
	}
	for _, s := range stats.badSamples {
		if !strings.Contains(s, "x.jsonl:") {
			t.Errorf("sample %q missing file:line prefix", s)
		}
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

// TestImporter_TagsRowsWithSourceAndPriority guards against the regression
// found in PR #83's baseline: cmd/importekilexdetails set source on
// dict_metadata only, leaving inserted lemmas/forms with source='' and
// source_priority=0. Without the row-level tag, the lookup ranker has
// nothing to rank by when distinguishing kaikki from Ekilex candidates.
func TestImporter_TagsRowsWithSourceAndPriority(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	formsDir := filepath.Join(tmp, "forms")
	if err := writeAll(defDir, "k.jsonl",
		`{"word_id":1,"lemma":"koer","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["dog"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write definitions: %v", err)
	}
	if err := writeAll(formsDir, "k.tsv",
		"lemma\tform\tmorph_code\nkoer\tkoerad\tPlN\n",
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

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregateDefinitions: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}
	if _, err := importForms(db, formsDir, lemmaPOS); err != nil {
		t.Fatalf("importForms: %v", err)
	}

	var lemmaSource string
	var lemmaPriority int
	if err := db.QueryRow(
		`SELECT source, source_priority FROM lemmas WHERE lemma='koer' AND pos='NOUN' AND lang='ET'`,
	).Scan(&lemmaSource, &lemmaPriority); err != nil {
		t.Fatalf("query lemma: %v", err)
	}
	if lemmaSource != "ekilex" || lemmaPriority != 20 {
		t.Errorf("lemma row: got source=%q priority=%d, want source=\"ekilex\" priority=20",
			lemmaSource, lemmaPriority)
	}

	var formSource string
	var formPriority int
	if err := db.QueryRow(
		`SELECT source, source_priority FROM forms WHERE form='koerad' AND lang='ET'`,
	).Scan(&formSource, &formPriority); err != nil {
		t.Fatalf("query form: %v", err)
	}
	if formSource != "ekilex" || formPriority != 20 {
		t.Errorf("form row: got source=%q priority=%d, want source=\"ekilex\" priority=20",
			formSource, formPriority)
	}
}

// TestImporter_UpgradesSourceOnConflictWhenAtLeastAsAuthoritative guards
// the upsert behavior. A pre-existing kaikki-tagged row (priority 10)
// should get upgraded to ekilex (priority 20) when this importer runs;
// a pre-existing custom-overrides row (priority 100) should not.
func TestImporter_UpgradesSourceOnConflictWhenAtLeastAsAuthoritative(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	formsDir := filepath.Join(tmp, "forms")
	if err := writeAll(defDir, "k.jsonl",
		`{"word_id":1,"lemma":"koer","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["dog"]}]}`+"\n"+
			`{"word_id":2,"lemma":"override","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["override-en"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write definitions: %v", err)
	}
	if err := writeAll(formsDir, "k.tsv",
		"lemma\tform\tmorph_code\nkoer\tkoerad\tPlN\noverride\toverrided\tPlN\n",
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

	// Pre-seed a kaikki-tagged row (priority 10) — should be upgraded.
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('koer', 'NOUN', 'kaikki-gloss', 'ET', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed kaikki lemma: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority)
		 VALUES ('koerad', 'koer', 'NOUN', 'ET', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed kaikki form: %v", err)
	}

	// Pre-seed a custom-overrides row (priority 100) — must NOT be downgraded.
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('override', 'NOUN', 'custom-gloss', 'ET', 'custom_overrides', 100)`,
	); err != nil {
		t.Fatalf("seed custom lemma: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority)
		 VALUES ('overrided', 'override', 'NOUN', 'ET', 'custom_overrides', 100)`,
	); err != nil {
		t.Fatalf("seed custom form: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregateDefinitions: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}
	if _, err := importForms(db, formsDir, lemmaPOS); err != nil {
		t.Fatalf("importForms: %v", err)
	}

	// Existing kaikki row should be upgraded to ekilex/20.
	var src string
	var prio int
	if err := db.QueryRow(
		`SELECT source, source_priority FROM lemmas WHERE lemma='koer' AND pos='NOUN' AND lang='ET'`,
	).Scan(&src, &prio); err != nil {
		t.Fatalf("query upgraded lemma: %v", err)
	}
	if src != "ekilex" || prio != 20 {
		t.Errorf("kaikki→ekilex lemma upgrade: got source=%q priority=%d, want ekilex/20",
			src, prio)
	}
	if err := db.QueryRow(
		`SELECT source, source_priority FROM forms WHERE form='koerad' AND lang='ET'`,
	).Scan(&src, &prio); err != nil {
		t.Fatalf("query upgraded form: %v", err)
	}
	if src != "ekilex" || prio != 20 {
		t.Errorf("kaikki→ekilex form upgrade: got source=%q priority=%d, want ekilex/20",
			src, prio)
	}

	// Existing custom row must NOT be downgraded.
	if err := db.QueryRow(
		`SELECT source, source_priority FROM lemmas WHERE lemma='override' AND pos='NOUN' AND lang='ET'`,
	).Scan(&src, &prio); err != nil {
		t.Fatalf("query custom lemma: %v", err)
	}
	if src != "custom_overrides" || prio != 100 {
		t.Errorf("custom downgrade leak: got source=%q priority=%d, want custom_overrides/100",
			src, prio)
	}
	if err := db.QueryRow(
		`SELECT source, source_priority FROM forms WHERE form='overrided' AND lang='ET'`,
	).Scan(&src, &prio); err != nil {
		t.Fatalf("query custom form: %v", err)
	}
	if src != "custom_overrides" || prio != 100 {
		t.Errorf("custom form downgrade leak: got source=%q priority=%d, want custom_overrides/100",
			src, prio)
	}
}

func TestWriteTranslations_BasicWrite(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "k.jsonl",
		`{"word_id":1,"lemma":"koer","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["dog","hound"]},{"pos":["s"],"translations_en":["bad"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write defs: %v", err)
	}

	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := ensureSchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}

	written, err := writeTranslations(db, lemmaPOS)
	if err != nil {
		t.Fatalf("writeTranslations: %v", err)
	}
	if written != 3 {
		t.Errorf("translations written = %d, want 3 (bad+dog+hound after dedup)", written)
	}

	rows, err := db.Query(
		`SELECT sense_idx, text, target_lang, source FROM translations
		 WHERE lemma='koer' AND pos='NOUN' AND lang='ET' ORDER BY sense_idx`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	got := []string{}
	for rows.Next() {
		var idx int
		var text, target, src string
		if err := rows.Scan(&idx, &text, &target, &src); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, fmt.Sprintf("%d:%s/%s/%s", idx, text, target, src))
	}
	// sortedTranslations sorts the deduplicated set: ["bad", "dog", "hound"].
	want := []string{"0:bad/EN/ekilex", "1:dog/EN/ekilex", "2:hound/EN/ekilex"}
	if !equalStringSlices(got, want) {
		t.Errorf("translations: got %v, want %v", got, want)
	}
}

func TestWriteTranslations_RewriteRemovesOrphanedSenses(t *testing.T) {
	// Mirrors PR #85's TestImportJSONL_NormalRerunRemovesOrphanedSenses:
	// when an entry's translation list shrinks upstream, the obsolete
	// sense_idx rows must be removed on the next run. The pre-write
	// DELETE + rebuild handles this.
	tmp := t.TempDir()
	defDirA := filepath.Join(tmp, "defs-a")
	defDirB := filepath.Join(tmp, "defs-b")
	if err := writeAll(defDirA, "k.jsonl",
		`{"word_id":1,"lemma":"koer","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["dog","hound","cur"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write A: %v", err)
	}
	if err := writeAll(defDirB, "k.jsonl",
		`{"word_id":1,"lemma":"koer","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["dog"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write B: %v", err)
	}

	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := ensureSchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}

	// First import: 3 translations.
	lemmaA, _, err := aggregateDefinitions(defDirA)
	if err != nil {
		t.Fatalf("aggregate A: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaA); err != nil {
		t.Fatalf("writeLemmas A: %v", err)
	}
	if _, err := writeTranslations(db, lemmaA); err != nil {
		t.Fatalf("writeTranslations A: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM translations WHERE lemma='koer'`).Scan(&n); err != nil {
		t.Fatalf("count A: %v", err)
	}
	if n != 3 {
		t.Fatalf("after A: got %d rows, want 3", n)
	}

	// Second import: only 1 translation. Stale sense_idx 1, 2 should go.
	lemmaB, _, err := aggregateDefinitions(defDirB)
	if err != nil {
		t.Fatalf("aggregate B: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaB); err != nil {
		t.Fatalf("writeLemmas B: %v", err)
	}
	if _, err := writeTranslations(db, lemmaB); err != nil {
		t.Fatalf("writeTranslations B: %v", err)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM translations WHERE lemma='koer'`).Scan(&n); err != nil {
		t.Fatalf("count B: %v", err)
	}
	if n != 1 {
		t.Errorf("after B (shrunk): got %d rows, want 1 (orphans should be removed)", n)
	}
}

func TestWriteTranslations_PreservesOtherSourcesTranslations(t *testing.T) {
	// The pre-import DELETE is scoped by (lang='ET', source='ekilex').
	// A pre-existing kaikki translation row for the same lemma must
	// survive an Ekilex re-run.
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "k.jsonl",
		`{"word_id":1,"lemma":"koer","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["dog"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write defs: %v", err)
	}

	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := ensureSchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}

	// Pre-seed a kaikki translation row.
	if _, err := db.Exec(
		`INSERT INTO translations (lemma, pos, lang, target_lang, text, sense_idx, source)
		 VALUES ('koer', 'NOUN', 'ET', 'EN', 'kaikki-dog', 0, 'kaikki')`,
	); err != nil {
		t.Fatalf("seed kaikki: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}
	if _, err := writeTranslations(db, lemmaPOS); err != nil {
		t.Fatalf("writeTranslations: %v", err)
	}

	// kaikki row preserved.
	var kaikkiText string
	if err := db.QueryRow(
		`SELECT text FROM translations WHERE lemma='koer' AND source='kaikki' AND lang='ET'`,
	).Scan(&kaikkiText); err != nil {
		t.Fatalf("query kaikki: %v", err)
	}
	if kaikkiText != "kaikki-dog" {
		t.Errorf("kaikki preserved: got %q, want %q (Ekilex wipe scope was too broad)", kaikkiText, "kaikki-dog")
	}

	// ekilex row written.
	var ekilexText string
	if err := db.QueryRow(
		`SELECT text FROM translations WHERE lemma='koer' AND source='ekilex' AND lang='ET'`,
	).Scan(&ekilexText); err != nil {
		t.Fatalf("query ekilex: %v", err)
	}
	if ekilexText != "dog" {
		t.Errorf("ekilex written: got %q, want %q", ekilexText, "dog")
	}
}

func TestWriteTranslations_NoTranslationsETOnlyEntry(t *testing.T) {
	// Some Ekilex entries have only ET definitions and no EN
	// translations (the "muutumatu"-with-no-meanings tail). They
	// should produce zero translation rows, not crash.
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "k.jsonl",
		// definitions only, no translations_en
		`{"word_id":1,"lemma":"koer","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"definitions_et":["loom"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write defs: %v", err)
	}

	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := ensureSchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}

	written, err := writeTranslations(db, lemmaPOS)
	if err != nil {
		t.Fatalf("writeTranslations: %v", err)
	}
	if written != 0 {
		t.Errorf("ET-only entry: got %d rows, want 0", written)
	}
}
