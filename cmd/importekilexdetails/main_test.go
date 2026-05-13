package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"finnestdb/internal/glossfallback"
	"finnestdb/internal/store"

	_ "github.com/mattn/go-sqlite3"
)

func joinTranslations(entry definitionEntry) string {
	tmp := lemmaPOSMap{}
	for _, m := range entry.Meanings {
		tmp.add("_", "_", m.TranslationsEN, m.DefinitionsET)
	}
	if d, ok := tmp["_"]["_"]; ok {
		return joinTranslationData(d)
	}
	return ""
}

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
	// "Line" wins the dedup tiebreak over "line" (chooseCasing prefers the
	// uppercase-leading variant), but the original meaning order is preserved.
	want := "Line; stripe; feature; shape"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJoinTranslations_PreservesMeaningOrderForPrimaryGloss(t *testing.T) {
	// Learner-facing primary glosses must follow Ekilex meaning order, not
	// alphabetic order. Otherwise common verbs like olema can surface a rare
	// alphabetically-earlier sense such as "accompany" before "be".
	entry := definitionEntry{Meanings: []struct {
		POS            []string `json:"pos"`
		DefinitionsET  []string `json:"definitions_et"`
		TranslationsEN []string `json:"translations_en"`
		UsagesET       []string `json:"usages_et"`
	}{
		{TranslationsEN: []string{"be"}},
		{TranslationsEN: []string{"accompany"}},
	}}
	got := joinTranslations(entry)
	want := "be; accompany"
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
	a.add("kalvados", "NOUN", []string{"calvados", "Calvados"}, nil)
	b := lemmaPOSMap{}
	b.add("kalvados", "NOUN", []string{"Calvados", "calvados"}, nil)

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
			"jooma\tjuua\tInf\n"+
			"jooma\tjuues\tGer\n"+
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

	for _, tc := range []struct {
		form string
		want string
	}{
		{"jooma", "Case=Ill|VerbForm=Sup"},
		{"juua", "VerbForm=Inf"},
		{"juues", "VerbForm=Conv"},
	} {
		var feats string
		if err := db.QueryRow(`SELECT COALESCE(feats, '') FROM forms WHERE form=? AND lemma='jooma' AND pos='VERB' AND lang='ET'`, tc.form).Scan(&feats); err != nil {
			t.Fatalf("query %s feats: %v", tc.form, err)
		}
		if feats != tc.want {
			t.Errorf("%s feats = %q, want %q", tc.form, feats, tc.want)
		}
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
	want := "step; stepping; degree; rank"
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
// dict_metadata only, leaving inserted lemmas/forms with the source
// column at the empty string and source_priority at 0. Without the
// row-level tag, the lookup ranker has nothing to rank by when
// distinguishing kaikki from Ekilex candidates.
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

func TestImporter_ReimportRepairsStaleSameSourceFeats(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	formsDir := filepath.Join(tmp, "forms")
	if err := writeAll(defDir, "j.jsonl",
		`{"word_id":1,"lemma":"jooma","homonym_nr":1,"word_class":"verb","meanings":[{"pos":["v"],"translations_en":["drink"]}]}`+"\n"+
			`{"word_id":2,"lemma":"guarded","homonym_nr":1,"word_class":"verb","meanings":[{"pos":["v"],"translations_en":["guard"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write definitions: %v", err)
	}
	if err := writeAll(formsDir, "j.tsv",
		"lemma\tform\tmorph_code\n"+
			"jooma\tjooma\tSup\n"+
			"jooma\tjuua\tInf\n"+
			"jooma\tjuues\tGer\n"+
			"jooma\tjoodama\tSupIps\n"+
			"guarded\tguarded\tSup\n",
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

	if _, err := db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
		 VALUES ('jooma', 'jooma', 'VERB', 'ET', 'ekilex', 20, 'VerbForm=Inf')`,
	); err != nil {
		t.Fatalf("seed stale ekilex form: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
		 VALUES ('juua', 'jooma', 'VERB', 'ET', 'ekilex', 20, 'VerbForm=Sup')`,
	); err != nil {
		t.Fatalf("seed stale infinitive form: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
		 VALUES ('juues', 'jooma', 'VERB', 'ET', 'ekilex', 20, '')`,
	); err != nil {
		t.Fatalf("seed empty gerund form: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
		 VALUES ('joodama', 'jooma', 'VERB', 'ET', 'ekilex', 20, 'VerbForm=Sup|Voice=Pass')`,
	); err != nil {
		t.Fatalf("seed stale passive supine form: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
		 VALUES ('guarded', 'guarded', 'VERB', 'ET', 'custom_overrides', 100, 'VerbForm=Inf')`,
	); err != nil {
		t.Fatalf("seed custom form: %v", err)
	}

	if _, err := importForms(db, formsDir, lemmaPOS); err != nil {
		t.Fatalf("importForms: %v", err)
	}

	var feats string
	for _, tc := range []struct {
		form string
		want string
	}{
		{"jooma", "Case=Ill|VerbForm=Sup"},
		{"juua", "VerbForm=Inf"},
		{"juues", "VerbForm=Conv"},
		{"joodama", "Case=Ill|VerbForm=Sup|Voice=Pass"},
	} {
		if err := db.QueryRow(
			`SELECT feats FROM forms WHERE form=? AND lemma='jooma' AND pos='VERB' AND lang='ET'`,
			tc.form,
		).Scan(&feats); err != nil {
			t.Fatalf("query repaired %s form: %v", tc.form, err)
		}
		if feats != tc.want {
			t.Errorf("%s repaired feats = %q, want %q", tc.form, feats, tc.want)
		}
	}

	if err := db.QueryRow(
		`SELECT feats FROM forms WHERE form='guarded' AND lemma='guarded' AND pos='VERB' AND lang='ET'`,
	).Scan(&feats); err != nil {
		t.Fatalf("query custom form: %v", err)
	}
	if feats != "VerbForm=Inf" {
		t.Errorf("custom feats overwritten: got %q, want original VerbForm=Inf", feats)
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
		t.Errorf("translations written = %d, want 3 (dog+hound+bad after dedup)", written)
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
	// orderedTranslations preserves Ekilex meaning order: ["dog", "hound", "bad"].
	want := []string{"0:dog/EN/ekilex", "1:hound/EN/ekilex", "2:bad/EN/ekilex"}
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

// TestJoinTranslationData_FallsBackToETDefinition guards the new fallback
// path — when no EN translations are present for a (lemma, pos), the gloss
// column gets a `[ET] `-prefixed Estonian definition instead of an empty
// string. The user-friendly wordlist export depends on this so its
// `meaning` column is non-empty for the ~4.6% of ET tokens that ekilex
// covers with a definition but no translation.
func TestJoinTranslationData_FallsBackToETDefinition(t *testing.T) {
	tmp := lemmaPOSMap{}
	tmp.add("aabitsakukk", "NOUN",
		nil,
		[]string{"kuke kujutis varasema aja aabitsa kaanel või esilehel"})
	gloss := joinTranslationData(tmp["aabitsakukk"]["NOUN"])
	want := "[ET] kuke kujutis varasema aja aabitsa kaanel või esilehel"
	if gloss != want {
		t.Errorf("ET fallback gloss: got %q, want %q", gloss, want)
	}
}

// TestJoinTranslationData_PrefersENTranslationOverETDefinition guards
// against the fallback firing when an EN translation IS available — the
// user-friendly path only wants `[ET] ` glosses as a last resort.
func TestJoinTranslationData_PrefersENTranslationOverETDefinition(t *testing.T) {
	tmp := lemmaPOSMap{}
	tmp.add("koer", "NOUN",
		[]string{"dog"},
		[]string{"koduloom"})
	gloss := joinTranslationData(tmp["koer"]["NOUN"])
	want := "dog"
	if gloss != want {
		t.Errorf("EN preferred over ET: got %q, want %q", gloss, want)
	}
}

// TestJoinTranslationData_TruncatesLongETDefinition guards against
// pathologically long EKI definitions bloating the gloss column. The full
// definition still lands in the definitions table.
func TestJoinTranslationData_TruncatesLongETDefinition(t *testing.T) {
	long := strings.Repeat("õ", glossFallbackMaxLen+50)
	tmp := lemmaPOSMap{}
	tmp.add("x", "NOUN", nil, []string{long})
	gloss := joinTranslationData(tmp["x"]["NOUN"])
	if !strings.HasPrefix(gloss, glossfallback.ETPrefix) {
		t.Errorf("missing ET prefix: %q", gloss[:20])
	}
	if !strings.HasSuffix(gloss, "…") {
		t.Errorf("missing truncation marker: %q", gloss[len(gloss)-10:])
	}
	// Prefix + glossFallbackMaxLen runes + ellipsis. Counting runes since
	// each "õ" is 2 bytes in UTF-8.
	got := utf8RuneCount(gloss)
	want := utf8RuneCount(glossfallback.ETPrefix) + glossFallbackMaxLen + 1
	if got != want {
		t.Errorf("rune count: got %d, want %d", got, want)
	}
}

// TestWriteDefinitions_BasicWrite guards the new definitions-table writer
// path — every Estonian-language definition lands as one row per sense,
// in the order they arrive from the JSONL.
func TestWriteDefinitions_BasicWrite(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "a.jsonl",
		`{"word_id":1,"lemma":"aade","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"definitions_et":["mõte","plaan"],"translations_en":["idea"]},{"pos":["s"],"definitions_et":["unistus"]}]}`+"\n",
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

	written, err := writeDefinitions(db, lemmaPOS)
	if err != nil {
		t.Fatalf("writeDefinitions: %v", err)
	}
	if written != 3 {
		t.Errorf("definitions written = %d, want 3", written)
	}

	rows, err := db.Query(
		`SELECT sense_idx, text, source FROM definitions
		 WHERE lemma='aade' AND pos='NOUN' AND lang='ET' ORDER BY sense_idx`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	got := []string{}
	for rows.Next() {
		var idx int
		var text, src string
		if err := rows.Scan(&idx, &text, &src); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, fmt.Sprintf("%d:%s/%s", idx, text, src))
	}
	want := []string{"0:mõte/ekilex", "1:plaan/ekilex", "2:unistus/ekilex"}
	if !equalStringSlices(got, want) {
		t.Errorf("definitions: got %v, want %v", got, want)
	}
}

// TestWriteDefinitions_DedupesAcrossHomonyms guards that two Ekilex
// homonyms that collapse into the same (lemma, pos) (e.g. aste#1 + aste#2,
// both NOUN) merge their definition sets case-sensitively without dropping
// distinct senses or duplicating identical phrases.
func TestWriteDefinitions_DedupesAcrossHomonyms(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "a.jsonl",
		`{"word_id":1,"lemma":"aste","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"definitions_et":["samm","aste1-def"]}]}`+"\n"+
			`{"word_id":2,"lemma":"aste","homonym_nr":2,"word_class":"noomen","meanings":[{"pos":["s"],"definitions_et":["samm","aste2-def"]}]}`+"\n",
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
		t.Fatalf("writeLemmas: %v", err)
	}
	if _, err := writeDefinitions(db, lemmaPOS); err != nil {
		t.Fatalf("writeDefinitions: %v", err)
	}

	rows, err := db.Query(
		`SELECT text FROM definitions
		 WHERE lemma='aste' AND pos='NOUN' AND lang='ET' ORDER BY sense_idx`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	got := []string{}
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, text)
	}
	// "samm" appears once (deduped); "aste1-def" and "aste2-def" both
	// preserved. Order is insertion order across the homonym entries.
	want := []string{"samm", "aste1-def", "aste2-def"}
	if !equalStringSlices(got, want) {
		t.Errorf("definitions: got %v, want %v", got, want)
	}
}

// TestWriteDefinitions_RewriteRemovesOrphanedSenses mirrors
// TestWriteTranslations_RewriteRemovesOrphanedSenses. Shrinking a
// definition list upstream must not leave stale rows.
func TestWriteDefinitions_RewriteRemovesOrphanedSenses(t *testing.T) {
	tmp := t.TempDir()
	defDirA := filepath.Join(tmp, "defs-a")
	defDirB := filepath.Join(tmp, "defs-b")
	if err := writeAll(defDirA, "a.jsonl",
		`{"word_id":1,"lemma":"x","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"definitions_et":["a","b","c"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write A: %v", err)
	}
	if err := writeAll(defDirB, "a.jsonl",
		`{"word_id":1,"lemma":"x","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"definitions_et":["a"]}]}`+"\n",
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

	lemmaA, _, err := aggregateDefinitions(defDirA)
	if err != nil {
		t.Fatalf("aggregate A: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaA); err != nil {
		t.Fatalf("writeLemmas A: %v", err)
	}
	if _, err := writeDefinitions(db, lemmaA); err != nil {
		t.Fatalf("writeDefinitions A: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM definitions WHERE lemma='x'`).Scan(&n); err != nil {
		t.Fatalf("count A: %v", err)
	}
	if n != 3 {
		t.Fatalf("after A: got %d, want 3", n)
	}

	lemmaB, _, err := aggregateDefinitions(defDirB)
	if err != nil {
		t.Fatalf("aggregate B: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaB); err != nil {
		t.Fatalf("writeLemmas B: %v", err)
	}
	if _, err := writeDefinitions(db, lemmaB); err != nil {
		t.Fatalf("writeDefinitions B: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM definitions WHERE lemma='x'`).Scan(&n); err != nil {
		t.Fatalf("count B: %v", err)
	}
	if n != 1 {
		t.Errorf("after B: got %d, want 1 (orphan senses should be removed)", n)
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

// TestAggregateDefinitions_AttributesMeaningsPerPOS guards against the codex
// finding: when one Ekilex entry carries meanings under multiple POS
// (e.g. aastatagune has an ADJ meaning followed by NOUN meanings),
// translations and definitions must be attributed to the meaning's own POS,
// not flattened across the whole entry. Cross-POS leakage would write ADJ
// content into the NOUN row and vice versa.
func TestAggregateDefinitions_AttributesMeaningsPerPOS(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "a.jsonl",
		`{"word_id":1,"lemma":"aastatagune","homonym_nr":1,"word_class":"noomen",`+
			`"meanings":[`+
			`{"pos":["adj"],"definitions_et":["adj-def"],"translations_en":["yearly"]},`+
			`{"pos":["s"],"definitions_et":["noun-def-1"],"translations_en":["yearling"]},`+
			`{"pos":["s"],"definitions_et":["noun-def-2"]}`+
			`]}`+"\n",
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	adj, ok := lemmaPOS["aastatagune"]["ADJ"]
	if !ok {
		t.Fatalf("expected ADJ entry for aastatagune")
	}
	if !equalStringSlices(adj.definitionsET, []string{"adj-def"}) {
		t.Errorf("ADJ definitionsET = %v, want [adj-def] (NOUN defs leaked in)",
			adj.definitionsET)
	}
	adjT := orderedTranslations(adj)
	if !equalStringSlices(adjT, []string{"yearly"}) {
		t.Errorf("ADJ translations = %v, want [yearly] (NOUN translations leaked in)", adjT)
	}

	noun, ok := lemmaPOS["aastatagune"]["NOUN"]
	if !ok {
		t.Fatalf("expected NOUN entry for aastatagune")
	}
	if !equalStringSlices(noun.definitionsET, []string{"noun-def-1", "noun-def-2"}) {
		t.Errorf("NOUN definitionsET = %v, want [noun-def-1 noun-def-2] (ADJ defs leaked in or order wrong)",
			noun.definitionsET)
	}
	nounT := orderedTranslations(noun)
	if !equalStringSlices(nounT, []string{"yearling"}) {
		t.Errorf("NOUN translations = %v, want [yearling] (ADJ translations leaked in)", nounT)
	}
}

// TestAggregateDefinitions_PerMeaningWordClassFallback guards the per-meaning
// fallback path: a meaning whose own pos is unmappable should fall back to
// the entry-level word_class, but only for that meaning. A sibling meaning
// with a mappable pos must still land on its own POS, not on the fallback.
func TestAggregateDefinitions_PerMeaningWordClassFallback(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "x.jsonl",
		`{"word_id":1,"lemma":"x","homonym_nr":1,"word_class":"noomen",`+
			`"meanings":[`+
			`{"pos":["adj"],"definitions_et":["adj-def"]},`+
			`{"pos":["WEIRD"],"definitions_et":["fallback-def"]}`+
			`]}`+"\n",
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if adj, ok := lemmaPOS["x"]["ADJ"]; !ok {
		t.Errorf("expected ADJ entry from mappable meaning")
	} else if !equalStringSlices(adj.definitionsET, []string{"adj-def"}) {
		t.Errorf("ADJ definitionsET = %v, want [adj-def]", adj.definitionsET)
	}

	if noun, ok := lemmaPOS["x"]["NOUN"]; !ok {
		t.Errorf("expected NOUN entry from word_class fallback")
	} else if !equalStringSlices(noun.definitionsET, []string{"fallback-def"}) {
		t.Errorf("NOUN definitionsET = %v, want [fallback-def] (cross-meaning leak?)",
			noun.definitionsET)
	}
}

// TestWriteLemmas_RefreshesETFallbackOnReimport guards against the codex
// finding: when the first import wrote an `[ET] ` fallback into lemmas.gloss
// because no EN translation was available, a subsequent import that DOES
// have an EN translation (or a different ET definition) must replace the
// fallback. Without the refresh, lemmas.gloss stays stale even though the
// translations and definitions tables were rebuilt by Pass 2.5/2.6.
func TestWriteLemmas_RefreshesETFallbackOnReimport(t *testing.T) {
	tmp := t.TempDir()
	defDir1 := filepath.Join(tmp, "defs-1")
	defDir2 := filepath.Join(tmp, "defs-2")
	defDir3 := filepath.Join(tmp, "defs-3")
	// Pass 1: only ET definition. Gloss should be `[ET] old-def`.
	if err := writeAll(defDir1, "a.jsonl",
		`{"word_id":1,"lemma":"aabits","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"definitions_et":["old-def"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	// Pass 2: EN translation now available. Gloss should refresh to "primer".
	if err := writeAll(defDir2, "a.jsonl",
		`{"word_id":1,"lemma":"aabits","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["primer"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	// Pass 3: still no EN, but ET definition changed upstream. Gloss should
	// refresh from `[ET] old-def` to `[ET] new-def`.
	if err := writeAll(defDir3, "a.jsonl",
		`{"word_id":1,"lemma":"refresh-et","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"definitions_et":["new-def"]}]}`+"\n",
	); err != nil {
		t.Fatalf("write 3: %v", err)
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

	// Pre-seed a separate (lemma, pos) with an `[ET] new-def` row so
	// pass-3's import path exercises the refresh against an existing
	// `[ET] old-def` row.
	lp1, _, err := aggregateDefinitions(defDir1)
	if err != nil {
		t.Fatalf("aggregate 1: %v", err)
	}
	if _, _, err := writeLemmas(db, lp1); err != nil {
		t.Fatalf("writeLemmas 1: %v", err)
	}
	var gloss1 string
	if err := db.QueryRow(
		`SELECT gloss FROM lemmas WHERE lemma='aabits' AND pos='NOUN' AND lang='ET'`,
	).Scan(&gloss1); err != nil {
		t.Fatalf("query 1: %v", err)
	}
	if gloss1 != "[ET] old-def" {
		t.Fatalf("after import 1: gloss = %q, want %q", gloss1, "[ET] old-def")
	}

	// Reimport with EN translation: gloss must refresh, not stay stale.
	lp2, _, err := aggregateDefinitions(defDir2)
	if err != nil {
		t.Fatalf("aggregate 2: %v", err)
	}
	if _, _, err := writeLemmas(db, lp2); err != nil {
		t.Fatalf("writeLemmas 2: %v", err)
	}
	var gloss2 string
	if err := db.QueryRow(
		`SELECT gloss FROM lemmas WHERE lemma='aabits' AND pos='NOUN' AND lang='ET'`,
	).Scan(&gloss2); err != nil {
		t.Fatalf("query 2: %v", err)
	}
	if gloss2 != "primer" {
		t.Errorf("after EN reimport: gloss = %q, want %q (stale [ET] fallback was not refreshed)",
			gloss2, "primer")
	}

	// Now also exercise the `[ET] x` → `[ET] y` refresh path. Seed a row
	// directly with an old `[ET] old-def` gloss tagged source=ekilex,
	// then import a different ET definition.
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('refresh-et', 'NOUN', '[ET] old-def', 'ET', 'ekilex', 20)`,
	); err != nil {
		t.Fatalf("seed [ET] row: %v", err)
	}
	lp3, _, err := aggregateDefinitions(defDir3)
	if err != nil {
		t.Fatalf("aggregate 3: %v", err)
	}
	if _, _, err := writeLemmas(db, lp3); err != nil {
		t.Fatalf("writeLemmas 3: %v", err)
	}
	var gloss3 string
	if err := db.QueryRow(
		`SELECT gloss FROM lemmas WHERE lemma='refresh-et' AND pos='NOUN' AND lang='ET'`,
	).Scan(&gloss3); err != nil {
		t.Fatalf("query 3: %v", err)
	}
	if gloss3 != "[ET] new-def" {
		t.Errorf("after [ET] reimport: gloss = %q, want %q (changed ET definition was not refreshed)",
			gloss3, "[ET] new-def")
	}
}

// TestAggregateDefinitions_EntryWithoutMeaningsUsesWordClass guards the
// entry-level word_class fallback for entries that arrive with zero
// resolvable meanings — real Ekilex carries ~19k such fallback-importable
// lemmas (e.g. verbs like alajahtuma) whose definitions live in a paired
// entry while forms still need a target (lemma, pos) to attach to. Without
// this fallback the form-import path drops every form for the lemma.
func TestAggregateDefinitions_EntryWithoutMeaningsUsesWordClass(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "a.jsonl",
		// Entry with zero meanings + verb word_class.
		`{"word_id":1,"lemma":"alajahtuma","homonym_nr":1,"word_class":"verb","meanings":[]}`+"\n"+
			// Entry with one meaning whose pos is unmappable + noomen word_class.
			// Should still use the per-meaning fallback (already covered) and
			// land on NOUN.
			`{"word_id":2,"lemma":"weirdpos","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["WEIRD"]}]}`+"\n"+
			// Entry with no meanings AND no resolvable word_class — should
			// be counted as noPOS, not silently dropped without a counter.
			`{"word_id":3,"lemma":"unknown","homonym_nr":1,"word_class":"alien","meanings":[]}`+"\n",
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	lemmaPOS, stats, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, ok := lemmaPOS["alajahtuma"]["VERB"]; !ok {
		t.Errorf("expected VERB entry for alajahtuma via word_class fallback")
	}
	if _, ok := lemmaPOS["weirdpos"]["NOUN"]; !ok {
		t.Errorf("expected NOUN entry for weirdpos via per-meaning fallback")
	}
	if _, ok := lemmaPOS["unknown"]; ok {
		t.Errorf("unknown should not have an entry — word_class is unmappable")
	}
	if stats.noPOS != 1 {
		t.Errorf("noPOS = %d, want 1 (the unknown/alien entry)", stats.noPOS)
	}
}

// TestImporter_AttachesFormsToWordClassFallbackLemmas guards the integration
// path: an entry whose definitions JSONL has zero meanings still needs to
// land in lemmaPOS so that importForms can attribute its forms. A regression
// in the per-meaning attribution rewrite would silently drop ~19k FI-side
// forms; this catches the case end-to-end.
func TestImporter_AttachesFormsToWordClassFallbackLemmas(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	formsDir := filepath.Join(tmp, "forms")
	if err := writeAll(defDir, "a.jsonl",
		`{"word_id":1,"lemma":"alajahtuma","homonym_nr":1,"word_class":"verb","meanings":[]}`+"\n",
	); err != nil {
		t.Fatalf("write defs: %v", err)
	}
	if err := writeAll(formsDir, "a.tsv",
		"lemma\tform\tmorph_code\n"+
			"alajahtuma\talajahtub\tIndPrSg3\n"+
			"alajahtuma\talajahtus\tIndImPs\n",
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
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}
	formInserted, err := importForms(db, formsDir, lemmaPOS)
	if err != nil {
		t.Fatalf("importForms: %v", err)
	}
	if formInserted != 2 {
		t.Errorf("forms inserted = %d, want 2 (both verbal forms attributed via word_class)",
			formInserted)
	}

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM forms WHERE lemma='alajahtuma' AND pos='VERB' AND lang='ET'`,
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("alajahtuma VERB forms = %d, want 2", n)
	}
}

// TestWriteLemmas_RefreshesEkilexENGlossOnReimport guards the codex-flagged
// regression: an existing source='ekilex' row whose EN translation set
// changed upstream (e.g. "dog" → "puppy") must refresh lemmas.gloss to
// match. The translations table is rebuilt fresh per source, so failing
// to refresh would let the cached gloss diverge from the per-meaning
// translations table that wordlist consumers also see.
func TestWriteLemmas_RefreshesEkilexENGlossOnReimport(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "p.jsonl",
		`{"word_id":1,"lemma":"puppy","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["puppy","whelp"]}]}`+"\n",
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

	// Pre-seed an ekilex-owned row whose EN translation has since changed.
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('puppy', 'NOUN', 'old-en-gloss', 'ET', 'ekilex', 20)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}

	var gloss string
	if err := db.QueryRow(
		`SELECT gloss FROM lemmas WHERE lemma='puppy' AND pos='NOUN' AND lang='ET'`,
	).Scan(&gloss); err != nil {
		t.Fatalf("query: %v", err)
	}
	want := "puppy; whelp"
	if gloss != want {
		t.Errorf("ekilex EN gloss not refreshed on reimport: got %q, want %q", gloss, want)
	}
}

// TestWriteLemmas_RefreshesEkilexENtoETFallbackOnReimport guards the path
// where the upstream EN translation is removed entirely and only an ET
// definition remains. Without the same-source refresh, lemmas.gloss would
// keep showing the stale "old-en" while the translations table no longer
// has any EN entry for the (lemma, pos).
func TestWriteLemmas_RefreshesEkilexENtoETFallbackOnReimport(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "p.jsonl",
		// Upstream removed the EN translation; only ET def remains.
		`{"word_id":1,"lemma":"orphan","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"definitions_et":["fallback-def"]}]}`+"\n",
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
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('orphan', 'NOUN', 'old-en', 'ET', 'ekilex', 20)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}

	var gloss string
	if err := db.QueryRow(
		`SELECT gloss FROM lemmas WHERE lemma='orphan' AND pos='NOUN' AND lang='ET'`,
	).Scan(&gloss); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gloss != "[ET] fallback-def" {
		t.Errorf("ekilex EN→[ET] gloss not refreshed: got %q, want %q",
			gloss, "[ET] fallback-def")
	}
}

// TestWriteLemmas_ClearsEkilexGlossWhenUpstreamWentEmpty guards the codex
// finding: when upstream Ekilex no longer has any EN translation OR ET
// definition for an existing source='ekilex' row (so the (lemma, pos) is
// reached only via the word_class fallback path, with `gloss = ""`),
// lemmas.gloss must be cleared to match. Otherwise Pass 2.5 / 2.6 wipe
// the now-empty translations and definitions tables but lemmas.gloss
// keeps showing the stale EN/ET text from the previous run.
func TestWriteLemmas_ClearsEkilexGlossWhenUpstreamWentEmpty(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "g.jsonl",
		// Empty meanings + verb word_class — exercises the word_class
		// fallback path where joinTranslationData returns "".
		`{"word_id":1,"lemma":"ghost","homonym_nr":1,"word_class":"verb","meanings":[]}`+"\n",
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

	// Pre-seed an ekilex-owned row whose previous run left a stale gloss.
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('ghost', 'VERB', 'old-stale-en', 'ET', 'ekilex', 20)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}

	var gloss string
	if err := db.QueryRow(
		`SELECT COALESCE(gloss, '') FROM lemmas WHERE lemma='ghost' AND pos='VERB' AND lang='ET'`,
	).Scan(&gloss); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gloss != "" {
		t.Errorf("ekilex gloss not cleared on empty reimport: got %q, want \"\" (translations/definitions tables were wiped, lemmas.gloss must follow)",
			gloss)
	}
}

// TestWriteLemmas_DoesNotClearNonEkilexGlossOnEmptyReimport guards the
// flip side of the above: an empty Ekilex reimport for a (lemma, pos)
// where the row is owned by a different source (kaikki, custom_overrides)
// must NOT clear that row's gloss. The pre-INSERT refresh is keyed on
// `source = 'ekilex'` precisely so non-ekilex rows are unaffected.
func TestWriteLemmas_DoesNotClearNonEkilexGlossOnEmptyReimport(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "g.jsonl",
		`{"word_id":1,"lemma":"ghost","homonym_nr":1,"word_class":"verb","meanings":[]}`+"\n",
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

	// Pre-seed a custom_overrides row that must survive an empty Ekilex run.
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('ghost', 'VERB', 'custom-keep-this', 'ET', 'custom_overrides', 100)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}

	var gloss, source string
	if err := db.QueryRow(
		`SELECT gloss, source FROM lemmas WHERE lemma='ghost' AND pos='VERB' AND lang='ET'`,
	).Scan(&gloss, &source); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gloss != "custom-keep-this" {
		t.Errorf("non-ekilex gloss clobbered by empty Ekilex run: got %q, want %q",
			gloss, "custom-keep-this")
	}
	if source != "custom_overrides" {
		t.Errorf("source unexpectedly changed: got %q, want custom_overrides", source)
	}
}

// TestWriteLemmas_PreservesKaikkiGlossOnFirstEkilexImport guards the
// first-run preservation contract: a non-ekilex row (kaikki bootstrap or
// pre-source-tracking row) should keep its English gloss across the very
// first Ekilex import. The pre-INSERT refresh is keyed on source='ekilex'
// so it skips rows with any other source; the INSERT path only upgrades
// source/priority, leaving the gloss untouched. Same-source reimports
// (after the upgrade) DO refresh — the trade-off price for keeping
// translations and lemmas.gloss consistent — but that's a different test.
func TestWriteLemmas_PreservesKaikkiGlossOnFirstEkilexImport(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "k.jsonl",
		`{"word_id":1,"lemma":"koer","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["dog"]}]}`+"\n",
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
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('koer', 'NOUN', 'rich kaikki gloss', 'ET', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}

	var gloss, source string
	var priority int
	if err := db.QueryRow(
		`SELECT gloss, source, source_priority FROM lemmas WHERE lemma='koer' AND pos='NOUN' AND lang='ET'`,
	).Scan(&gloss, &source, &priority); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gloss != "rich kaikki gloss" {
		t.Errorf("kaikki gloss clobbered on first Ekilex import: got %q, want %q",
			gloss, "rich kaikki gloss")
	}
	if source != "ekilex" || priority != 20 {
		t.Errorf("source not upgraded: got %q/%d, want ekilex/20", source, priority)
	}
}

// TestWriteLemmas_DoesNotRefreshNonEkilexETPrefixedGloss guards against an
// over-eager refresh: a `[ET] ...` gloss carried by a higher-priority source
// (e.g. someone bulk-edited custom_overrides with an Estonian definition)
// must not be replaced by the Ekilex importer. The source-guard on the
// fallback-refresh statement binds the refresh to ekilex-owned rows only.
func TestWriteLemmas_DoesNotRefreshNonEkilexETPrefixedGloss(t *testing.T) {
	tmp := t.TempDir()
	defDir := filepath.Join(tmp, "definitions")
	if err := writeAll(defDir, "x.jsonl",
		`{"word_id":1,"lemma":"override","homonym_nr":1,"word_class":"noomen","meanings":[{"pos":["s"],"translations_en":["new-en"]}]}`+"\n",
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

	// Pre-seed a custom-overrides row that happens to have an `[ET] ...`
	// gloss. The Ekilex import must not overwrite it.
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('override', 'NOUN', '[ET] custom-et-text', 'ET', 'custom_overrides', 100)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	lemmaPOS, _, err := aggregateDefinitions(defDir)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if _, _, err := writeLemmas(db, lemmaPOS); err != nil {
		t.Fatalf("writeLemmas: %v", err)
	}

	var gloss, source string
	if err := db.QueryRow(
		`SELECT gloss, source FROM lemmas WHERE lemma='override' AND pos='NOUN' AND lang='ET'`,
	).Scan(&gloss, &source); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gloss != "[ET] custom-et-text" {
		t.Errorf("custom_overrides [ET] gloss clobbered: got %q, want %q",
			gloss, "[ET] custom-et-text")
	}
	if source != "custom_overrides" {
		t.Errorf("custom_overrides source downgraded: got %q, want custom_overrides", source)
	}
}
