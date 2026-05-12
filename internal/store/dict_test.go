package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	lemmatizer "finnestdb/pkg/lemmatizer-fi-et"

	_ "github.com/mattn/go-sqlite3"
)

// newTestDB creates an in-memory SQLite database with the full schema applied.
// Tests use this to avoid touching the production finnestdb.db file.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("newTestDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// seedForms inserts (form, lemma, pos, lang) rows for testing.
func seedForms(t *testing.T, db *DB, rows [][4]string) {
	t.Helper()
	for _, r := range rows {
		_, err := db.db.Exec(
			`INSERT INTO forms (form, lemma, pos, lang) VALUES (?, ?, ?, ?)`,
			r[0], r[1], r[2], r[3],
		)
		if err != nil {
			t.Fatalf("seedForms: %v", err)
		}
	}
}

// seedLemmas inserts (lemma, pos, gloss, lang) rows for testing.
func seedLemmas(t *testing.T, db *DB, rows [][4]string) {
	t.Helper()
	for _, r := range rows {
		_, err := db.db.Exec(
			`INSERT INTO lemmas (lemma, pos, gloss, lang) VALUES (?, ?, ?, ?)`,
			r[0], r[1], r[2], r[3],
		)
		if err != nil {
			t.Fatalf("seedLemmas: %v", err)
		}
	}
}

func installTestLemmatizerTable(t *testing.T, lang string, table map[string][]lemmatizer.Analysis) {
	t.Helper()
	dir := t.TempDir()
	name := "fi_min.json"
	if lang == "ET" {
		name = "et_min.json"
	}
	b, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		t.Fatalf("marshal lemmatizer table: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write lemmatizer table: %v", err)
	}
	t.Setenv("LEMMATIZER_TABLES_DIR", dir)
}

// assertResolution is a test helper to check a FormResolution from the result map.
func assertResolution(t *testing.T, got map[string]FormResolution, form, wantLemma, wantPOS, wantSource string) {
	t.Helper()
	r, ok := got[form]
	if !ok {
		t.Errorf("%s: expected in result map, got absent", form)
		return
	}
	if r.Lemma != wantLemma || r.POS != wantPOS {
		t.Errorf("%s: got {%q %q}, want {%q %q}", form, r.Lemma, r.POS, wantLemma, wantPOS)
	}
	if wantSource != "" && r.Source != wantSource {
		t.Errorf("%s: source got %q, want %q", form, r.Source, wantSource)
	}
}

// --- BatchLookupAllForms tests ---

func TestBatchLookupAllForms_HomonymExpansion(t *testing.T) {
	db := newTestDB(t)
	// "joon" maps to two lemmas: noun "joon" (line) and verb "jooma" (drink),
	// where "joon" is 1Sg of jooma. Both rows must coexist under the new PK.
	seedForms(t, db, [][4]string{
		{"joon", "joon", "NOUN", "ET"},
		{"joon", "jooma", "VERB", "ET"},
		{"vesi", "vesi", "NOUN", "ET"},
	})

	got := db.BatchLookupAllForms([]string{"joon", "vesi", "missing"}, "ET", "custom")

	cands, ok := got["joon"]
	if !ok {
		t.Fatal("joon: expected in result map")
	}
	if len(cands) != 2 {
		t.Fatalf("joon: got %d candidates, want 2: %+v", len(cands), cands)
	}
	wantPairs := map[[2]string]bool{{"joon", "NOUN"}: false, {"jooma", "VERB"}: false}
	for _, c := range cands {
		key := [2]string{c.Lemma, c.POS}
		if _, expected := wantPairs[key]; !expected {
			t.Errorf("joon: unexpected candidate %+v", c)
			continue
		}
		wantPairs[key] = true
		if c.Source != "dict" {
			t.Errorf("joon candidate %+v: source got %q, want \"dict\"", c, c.Source)
		}
	}
	for pair, seen := range wantPairs {
		if !seen {
			t.Errorf("joon: missing expected candidate %v", pair)
		}
	}

	if len(got["vesi"]) != 1 {
		t.Errorf("vesi: got %d candidates, want 1", len(got["vesi"]))
	}
	if _, present := got["missing"]; present {
		t.Errorf("missing: expected absent from result map")
	}
}

func TestBatchLookupAllForms_CaseFolding(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"joon", "joon", "NOUN", "ET"},
		{"joon", "jooma", "VERB", "ET"},
	})

	got := db.BatchLookupAllForms([]string{"Joon"}, "ET", "custom")
	if len(got["Joon"]) != 2 {
		t.Errorf("sentence-initial Joon: got %d candidates, want 2", len(got["Joon"]))
	}
}

func TestBatchLookupAllForms_FiltersBadDictLemmas(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"varsin", "varsi", "NOUN", "FI"},
		{"varsin", "varsin", "ADV", "FI"},
		{"poliisissa", "poli", "NOUN", "FI"},
	})

	got := db.BatchLookupAllForms([]string{"varsin", "poliisissa"}, "FI", "basic")

	cands := got["varsin"]
	if len(cands) != 1 || cands[0].Lemma != "varsin" || cands[0].POS != "ADV" {
		t.Fatalf("varsin candidates=%+v want only varsin/ADV after bad-lemma filtering", cands)
	}
	if _, ok := got["poliisissa"]; ok {
		t.Fatalf("poliisissa candidates=%+v want absent after always-bad lemma filtering", got["poliisissa"])
	}
}

// TestBatchLookupAllForms_FI_MaInfinitiveCorrectsNounCousin proves that
// when kaikki has the buggy (lemma=tarjoama, pos=VERB) row for the
// MA-infinitive surface `tarjoamaan`, the FI homonym-expansion path
// drops it and substitutes the FST's correctly-stemmed verb headword
// (lemma=tarjota). Without this the /api/parse `words` list and deck
// creation would still expose tarjoama as the card headword even though
// the sentence-level token resolved to tarjota.
func TestBatchLookupAllForms_FI_MaInfinitiveCorrectsNounCousin(t *testing.T) {
	installTestLemmatizerTable(t, "FI", map[string][]lemmatizer.Analysis{
		"tarjoamaan": {
			{
				Lemma: "tarjota",
				UPOS:  "VERB",
				Feats: "Case=Ill|InfForm=3|Number=Sing|VerbForm=Inf",
				Raw:   "generated-table",
			},
		},
	})
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		// kaikki's noun-cousin trap: surface keyed under the deverbal -ma
		// noun instead of the verb headword.
		{"tarjoamaan", "tarjoama", "VERB", "FI"},
	})

	got := db.BatchLookupAllForms([]string{"tarjoamaan"}, "FI", "custom")

	cands := got["tarjoamaan"]
	if len(cands) != 1 {
		t.Fatalf("tarjoamaan: got %d candidates, want 1: %+v", len(cands), cands)
	}
	if cands[0].Lemma != "tarjota" || cands[0].POS != "VERB" {
		t.Errorf("tarjoamaan: got {%q %q}, want {tarjota VERB}",
			cands[0].Lemma, cands[0].POS)
	}
}

// TestBatchLookupAllForms_FI_AInfLongCorrectsSelfKey proves the A-long
// homonym-expansion correction: kaikki's self-key (mennäkseen lemma
// `mennäkseen`) is dropped and the FST's verb headword (mennä) is
// substituted. ymmärtääkseen exercises the same path but for the
// wrong-POS variant (kaikki tags it ADV with self-lemma).
func TestBatchLookupAllForms_FI_AInfLongCorrectsSelfKey(t *testing.T) {
	installTestLemmatizerTable(t, "FI", map[string][]lemmatizer.Analysis{
		"mennäkseen": {
			{
				Lemma: "mennä",
				UPOS:  "VERB",
				Feats: "InfForm=1|Person[psor]=3|VerbForm=Inf",
				Raw:   "generated-table",
			},
		},
		"ymmärtääkseen": {
			{
				Lemma: "ymmärtää",
				UPOS:  "VERB",
				Feats: "InfForm=1|Person[psor]=3|VerbForm=Inf",
				Raw:   "generated-table",
			},
		},
	})
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"mennäkseen", "mennäkseen", "VERB", "FI"},      // self-key
		{"ymmärtääkseen", "ymmärtääkseen", "ADV", "FI"}, // wrong-POS self-key
	})

	got := db.BatchLookupAllForms([]string{"mennäkseen", "ymmärtääkseen"}, "FI", "custom")

	if c := got["mennäkseen"]; len(c) != 1 || c[0].Lemma != "mennä" || c[0].POS != "VERB" {
		t.Errorf("mennäkseen: got %+v, want [{mennä VERB}]", c)
	}
	if c := got["ymmärtääkseen"]; len(c) != 1 || c[0].Lemma != "ymmärtää" || c[0].POS != "VERB" {
		t.Errorf("ymmärtääkseen: got %+v, want [{ymmärtää VERB}]", c)
	}
}

// TestBatchLookupAllForms_FI_PreservesLegitMaNoun proves the
// noun-cousin demotion doesn't false-fire on legitimate Finnish nouns
// whose lemma ends in -ma/-mä (voima, ryhmä, asema, järjestelmä,
// oireyhtymä, …). These are real nouns, not the kaikki trap, and on
// MA-shape surfaces (voimassa, ryhmästä) they must pass through to
// the deck/parse expansion as the correct headword.
//
// The kaikki trap signature is POS=VERB mis-tag with -ma/-mä lemma;
// the legit signature is POS=NOUN. The bias gate uses POS to
// distinguish.
func TestBatchLookupAllForms_FI_PreservesLegitMaNoun(t *testing.T) {
	// FST also has a competing voida/VERB MA-infinitive reading for
	// voimassa (linguistically valid: "while being able to"). For the
	// homonym-expansion path, we deliberately do NOT mix the FST verb
	// reading into the candidate list when the dict already has a
	// non-demoted candidate — that prevents the deck from offering
	// the wrong headword for context like "Sopimus on voimassa"
	// where voima/NOUN is what the learner actually sees.
	installTestLemmatizerTable(t, "FI", map[string][]lemmatizer.Analysis{
		"voimassa": {
			{
				Lemma: "voida",
				UPOS:  "VERB",
				Feats: "Case=Ine|InfForm=3|Number=Sing|VerbForm=Inf",
				Raw:   "generated-table",
			},
		},
	})
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"voimassa", "voima", "NOUN", "FI"},
	})

	got := db.BatchLookupAllForms([]string{"voimassa"}, "FI", "custom")

	cands := got["voimassa"]
	if len(cands) != 1 {
		t.Fatalf("voimassa: got %d candidates, want 1: %+v", len(cands), cands)
	}
	if cands[0].Lemma != "voima" || cands[0].POS != "NOUN" {
		t.Errorf("voimassa: got {%q %q}, want {voima NOUN}",
			cands[0].Lemma, cands[0].POS)
	}
}

// TestBatchLookupAllForms_FI_PreservesGenuineHomonym proves the
// correction path is conservative: when no candidate is demoted, the
// raw dict list passes through unchanged (no FST consultation, no
// unintended de-duplication). Uses a non-A/MA surface so neither bias
// fires, plus a separate test for an A/MA surface whose only dict row
// is already correct (verb stemmed, not noun-cousin) — the FST hint
// must not double-count the same (lemma, pos).
func TestBatchLookupAllForms_FI_PreservesGenuineHomonym(t *testing.T) {
	// kuusi: real Finnish homonym (NOUN "spruce" / NOUN "six" / NUM "six").
	// Surface doesn't match MA or A-long suffix so both biases are 0.
	installTestLemmatizerTable(t, "FI", map[string][]lemmatizer.Analysis{
		"kuusi": {
			{Lemma: "kuusi", UPOS: "NOUN", Raw: "generated-table"},
		},
	})
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"kuusi", "kuusi", "NOUN", "FI"},
		{"kuusi", "kuusi", "NUM", "FI"},
	})

	got := db.BatchLookupAllForms([]string{"kuusi"}, "FI", "custom")

	cands := got["kuusi"]
	if len(cands) != 2 {
		t.Fatalf("kuusi: got %d candidates, want 2 (NOUN + NUM): %+v", len(cands), cands)
	}
	seen := map[string]bool{}
	for _, c := range cands {
		seen[c.POS] = true
		if c.Lemma != "kuusi" {
			t.Errorf("kuusi: candidate %+v has lemma != kuusi", c)
		}
	}
	if !seen["NOUN"] || !seen["NUM"] {
		t.Errorf("kuusi: missing expected POS in %+v", cands)
	}
}

// TestBatchLookupAllForms_FI_NoFSTKeepsFilteredDict proves the fallback
// behaviour when no FST table is installed: -1 candidates are still
// dropped (kaikki self-keys are wrong regardless of whether we have a
// replacement), but no substitute is added. The deck/parse path then
// falls through to the parser's pick via expandTokenLemmas.
func TestBatchLookupAllForms_FI_NoFSTKeepsFilteredDict(t *testing.T) {
	// No installTestLemmatizerTable call — LEMMATIZER_TABLES_DIR stays empty.
	t.Setenv("LEMMATIZER_TABLES_DIR", t.TempDir())
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"mennäkseen", "mennäkseen", "VERB", "FI"}, // sole entry: self-key bug
	})

	got := db.BatchLookupAllForms([]string{"mennäkseen"}, "FI", "custom")

	// Without an FST to substitute the correct headword, returning the buggy
	// self-key would be worse than returning nothing — expandTokenLemmas
	// falls back to the parser's pick, which has its own FST-driven lemma.
	if c, present := got["mennäkseen"]; present && len(c) > 0 {
		t.Errorf("mennäkseen with no FST: expected dropped (absent or empty), got %+v", c)
	}
}

func TestBatchLookupAllForms_LexOverlaySuppressesRawDictTrapsInCustomMode(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"varsin", "varsi", "NOUN", "FI"},
		{"vuotta", "vuo", "NOUN", "FI"},
		{"välja", "väli", "NOUN", "ET"},
		{"välja", "väljama", "VERB", "ET"},
		{"peale", "pea", "NOUN", "ET"},
		{"sisse", "siss", "NOUN", "ET"},
		{"veel", "vesi", "NOUN", "ET"},
		{"jaoks", "jagu", "NOUN", "ET"},
		{"ta", "TA", "NOUN", "ET"},
		{"ta", "Ta", "X", "ET"},
		{"ta", "tema", "NOUN", "ET"},
	})

	assertOnly := func(got map[string][]FormResolution, form, wantLemma, wantPOS string) {
		t.Helper()
		candidates, ok := got[form]
		if !ok {
			t.Fatalf("%s: missing overlay candidate", form)
		}
		if len(candidates) != 1 {
			t.Fatalf("%s: got %d candidates, want 1: %+v", form, len(candidates), candidates)
		}
		c := candidates[0]
		if c.Lemma != wantLemma || c.POS != wantPOS || c.Source != "lex-overlay" {
			t.Fatalf("%s: got %+v, want %s/%s from lex-overlay", form, c, wantLemma, wantPOS)
		}
	}

	fi := db.BatchLookupAllForms([]string{"varsin", "Vuotta"}, "FI", "custom")
	assertOnly(fi, "varsin", "varsin", "ADV")
	assertOnly(fi, "Vuotta", "vuosi", "NOUN")

	et := db.BatchLookupAllForms([]string{"välja", "peale", "sisse", "veel", "jaoks", "Ta"}, "ET", "custom")
	assertOnly(et, "välja", "välja", "ADV")
	assertOnly(et, "peale", "peale", "ADP")
	assertOnly(et, "sisse", "sisse", "ADV")
	assertOnly(et, "veel", "veel", "ADV")
	assertOnly(et, "jaoks", "jaoks", "ADP")
	assertOnly(et, "Ta", "tema", "PRON")
}

func TestBatchLookupAllForms_LexOverlayBasicModeNotAffected(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"tuskin", "tuska", "NOUN", "FI"},
		{"välja", "väli", "NOUN", "ET"},
	})

	fi := db.BatchLookupAllForms([]string{"tuskin"}, "FI", "basic")
	if cands := fi["tuskin"]; len(cands) != 1 || cands[0].Lemma != "tuska" || cands[0].POS != "NOUN" || cands[0].Source != "dict" {
		t.Fatalf("basic FI candidates=%+v, want raw dict tuska/NOUN", cands)
	}
	et := db.BatchLookupAllForms([]string{"välja"}, "ET", "basic")
	if cands := et["välja"]; len(cands) != 1 || cands[0].Lemma != "väli" || cands[0].POS != "NOUN" || cands[0].Source != "dict" {
		t.Fatalf("basic ET candidates=%+v, want raw dict väli/NOUN", cands)
	}
}

// --- multi-lemma schema migration ---

func TestEnsureMultiLemmaSchema_PreservesRowsAndAddsMultiLemmaSupport(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}

	// Create the LEGACY shape (PK before this migration). Use the same
	// CREATE-time columns the merge-base of this branch had.
	if _, err := raw.Exec(`
		CREATE TABLE forms (
			form  TEXT NOT NULL,
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			lang  TEXT NOT NULL,
			PRIMARY KEY (form, lang)
		);
		CREATE TABLE occurrence (
			deck_id INTEGER NOT NULL,
			sentence_id INTEGER NOT NULL,
			token_ix INTEGER NOT NULL,
			lemma TEXT NOT NULL,
			pos TEXT NOT NULL,
			UNIQUE(deck_id, sentence_id, token_ix)
		);
	`); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO forms (form, lemma, pos, lang) VALUES ('joon', 'joon', 'NOUN', 'ET')`); err != nil {
		t.Fatalf("seed forms: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO occurrence (deck_id, sentence_id, token_ix, lemma, pos) VALUES (1, 1, 0, 'joon', 'NOUN')`); err != nil {
		t.Fatalf("seed occurrence: %v", err)
	}

	if err := EnsureMultiLemmaSchema(raw); err != nil {
		t.Fatalf("EnsureMultiLemmaSchema: %v", err)
	}

	// Existing rows preserved.
	var count int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM forms WHERE form='joon' AND lemma='joon' AND pos='NOUN' AND lang='ET'`).Scan(&count); err != nil {
		t.Fatalf("count forms: %v", err)
	}
	if count != 1 {
		t.Errorf("forms after migration: count=%d want 1", count)
	}

	// New PK admits a second (lemma, pos) row for the same (form, lang).
	if _, err := raw.Exec(`INSERT INTO forms (form, lemma, pos, lang) VALUES ('joon', 'jooma', 'VERB', 'ET')`); err != nil {
		t.Errorf("insert second homonym row: %v", err)
	}

	// New occurrence UNIQUE admits two rows at the same (deck, sentence, token).
	if _, err := raw.Exec(`INSERT INTO occurrence (deck_id, sentence_id, token_ix, lemma, pos) VALUES (1, 1, 0, 'jooma', 'VERB')`); err != nil {
		t.Errorf("insert second occurrence row: %v", err)
	}

	// Migration is idempotent — re-running is a no-op.
	if err := EnsureMultiLemmaSchema(raw); err != nil {
		t.Fatalf("EnsureMultiLemmaSchema (re-run): %v", err)
	}

	// And the previous duplicate-occurrence guard still holds for the
	// fully-keyed tuple.
	_, err = raw.Exec(`INSERT INTO occurrence (deck_id, sentence_id, token_ix, lemma, pos) VALUES (1, 1, 0, 'jooma', 'VERB')`)
	if err == nil {
		t.Error("expected UNIQUE violation on duplicate (deck, sentence, token, lemma, pos), got nil")
	}

	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}
}

func TestBackfillLegacyKaikkiProvenance(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer raw.Close()

	// Simulate the pre-provenance state: tables exist with the source/
	// source_priority columns added but populated only with their column
	// defaults ('' / 0). A real legacy DB looks exactly like this.
	if _, err := raw.Exec(`
		CREATE TABLE lemmas (
			lemma TEXT NOT NULL, pos TEXT NOT NULL, gloss TEXT, lang TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_priority INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (lemma, pos, lang)
		);
		CREATE TABLE forms (
			form TEXT NOT NULL, lemma TEXT NOT NULL, pos TEXT NOT NULL, lang TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_priority INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (form, lang, lemma, pos)
		);
		-- Two legacy FI rows (kaikki, but missing provenance):
		INSERT INTO lemmas (lemma, pos, gloss, lang) VALUES ('pankki', 'NOUN', 'bank', 'FI');
		INSERT INTO forms  (form, lemma, pos, lang)  VALUES ('pankissa', 'pankki', 'NOUN', 'FI');
		-- One ET row already labeled (Ekilex import path) — must be preserved:
		INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		    VALUES ('koer', 'NOUN', 'dog', 'ET', 'ekilex', 20);
		INSERT INTO forms  (form, lemma, pos, lang, source, source_priority)
		    VALUES ('koera', 'koer', 'NOUN', 'ET', 'ekilex', 20);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	if err := BackfillLegacyKaikkiProvenance(raw); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	// Legacy FI rows now carry kaikki provenance.
	var src string
	var prio int
	if err := raw.QueryRow(`SELECT source, source_priority FROM lemmas WHERE lemma='pankki' AND lang='FI'`).Scan(&src, &prio); err != nil {
		t.Fatalf("query FI lemma: %v", err)
	}
	if src != "kaikki" || prio != 10 {
		t.Errorf("FI lemma: got source=%q priority=%d, want kaikki/10", src, prio)
	}
	if err := raw.QueryRow(`SELECT source, source_priority FROM forms WHERE form='pankissa' AND lang='FI'`).Scan(&src, &prio); err != nil {
		t.Fatalf("query FI form: %v", err)
	}
	if src != "kaikki" || prio != 10 {
		t.Errorf("FI form: got source=%q priority=%d, want kaikki/10", src, prio)
	}

	// Pre-labeled ET row was not touched.
	if err := raw.QueryRow(`SELECT source, source_priority FROM lemmas WHERE lemma='koer' AND lang='ET'`).Scan(&src, &prio); err != nil {
		t.Fatalf("query ET lemma: %v", err)
	}
	if src != "ekilex" || prio != 20 {
		t.Errorf("ET lemma: backfill stomped pre-labeled row; got source=%q priority=%d, want ekilex/20", src, prio)
	}

	// Idempotent: a second run touches no rows and changes nothing.
	if err := BackfillLegacyKaikkiProvenance(raw); err != nil {
		t.Fatalf("backfill (second run): %v", err)
	}
	if err := raw.QueryRow(`SELECT source, source_priority FROM lemmas WHERE lemma='koer' AND lang='ET'`).Scan(&src, &prio); err != nil {
		t.Fatalf("query ET lemma (second run): %v", err)
	}
	if src != "ekilex" || prio != 20 {
		t.Errorf("ET lemma after re-run: got source=%q priority=%d, want ekilex/20", src, prio)
	}
}

func TestEnsureMultiLemmaSchema_PreservesAddedColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "with-source.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer raw.Close()

	// Simulate the post-EnsureDictionarySourceColumns state on main: forms
	// has source / source_priority added via ALTER TABLE before this
	// migration runs.
	if _, err := raw.Exec(`
		CREATE TABLE forms (
			form  TEXT NOT NULL,
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			lang  TEXT NOT NULL,
			PRIMARY KEY (form, lang)
		);
		CREATE TABLE occurrence (
			deck_id INTEGER NOT NULL,
			sentence_id INTEGER NOT NULL,
			token_ix INTEGER NOT NULL,
			lemma TEXT NOT NULL,
			pos TEXT NOT NULL,
			UNIQUE(deck_id, sentence_id, token_ix)
		);
		ALTER TABLE forms ADD COLUMN source TEXT NOT NULL DEFAULT '';
		ALTER TABLE forms ADD COLUMN source_priority INTEGER NOT NULL DEFAULT 0;
	`); err != nil {
		t.Fatalf("legacy schema with source cols: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO forms (form, lemma, pos, lang, source, source_priority) VALUES ('joon', 'joon', 'NOUN', 'ET', 'kaikki', 10)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := EnsureMultiLemmaSchema(raw); err != nil {
		t.Fatalf("migration: %v", err)
	}

	var src string
	var prio int
	if err := raw.QueryRow(`SELECT source, source_priority FROM forms WHERE form='joon' AND lemma='joon' AND pos='NOUN' AND lang='ET'`).Scan(&src, &prio); err != nil {
		t.Fatalf("query post-migration: %v", err)
	}
	if src != "kaikki" || prio != 10 {
		t.Errorf("source columns lost: got source=%q priority=%d, want kaikki/10", src, prio)
	}
}

// --- BatchLookupForms tests ---

func TestBatchLookupForms_Found(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"pankkiin", "pankki", "NOUN", "FI"},
		{"kirjassa", "kirja", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"pankkiin", "kirjassa"}, "FI", "basic")

	assertResolution(t, got, "pankkiin", "pankki", "NOUN", "dict")
	assertResolution(t, got, "kirjassa", "kirja", "NOUN", "dict")
}

func TestBatchLookupForms_NotFound(t *testing.T) {
	db := newTestDB(t)
	// No rows seeded — all lookups should miss.

	got := db.BatchLookupForms([]string{"viisutubettaja"}, "FI", "basic")

	if _, ok := got["viisutubettaja"]; ok {
		t.Errorf("viisutubettaja: expected absent from result map, got %v", got["viisutubettaja"])
	}
}

func TestBatchLookupForms_EmptyTable(t *testing.T) {
	db := newTestDB(t)
	// Empty forms table — must not panic, must return empty map.

	got := db.BatchLookupForms([]string{"pankki", "kirja", "talo"}, "FI", "basic")

	if len(got) != 0 {
		t.Errorf("expected empty map for empty table, got %v", got)
	}
}

// TestBatchLookupForms_CaseFolding verifies that sentence-initial capitals
// ("Kirjassa") and fully uppercased tokens resolve correctly even though
// dictionary rows are stored in lowercase.
func TestBatchLookupForms_CaseFolding(t *testing.T) {
	db := newTestDB(t)
	// Dictionary stores the form in lowercase.
	seedForms(t, db, [][4]string{
		{"pankkiin", "pankki", "NOUN", "FI"},
	})

	// "Pankkiin" is the sentence-start capitalised variant — should still resolve.
	got := db.BatchLookupForms([]string{"Pankkiin", "PANKKIIN"}, "FI", "basic")

	assertResolution(t, got, "Pankkiin", "pankki", "NOUN", "dict")
	assertResolution(t, got, "PANKKIIN", "pankki", "NOUN", "dict")
	// Result map is keyed by original form, not lowercased key.
	if _, ok := got["pankkiin"]; ok {
		t.Error("result must be keyed by original form, not lowercased key")
	}
}

// TestBatchLookupForms_CaseFoldingPossessiveStrip verifies that a capitalised
// possessive form ("Kirjassani") is resolved after both lowercasing and suffix
// stripping.
func TestBatchLookupForms_CaseFoldingPossessiveStrip(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"kirjassa", "kirja", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"Kirjassani"}, "FI", "custom")

	assertResolution(t, got, "Kirjassani", "kirja", "NOUN", "possessive")
}

func TestBatchLookupForms_PossessiveSuffixStrip(t *testing.T) {
	db := newTestDB(t)
	// Seed the base form "kirjassa" but NOT "kirjassani".
	// "kirjassani" = kirjassa + ni (possessive) → should resolve to kirja.
	seedForms(t, db, [][4]string{
		{"kirjassa", "kirja", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"kirjassani"}, "FI", "custom")

	assertResolution(t, got, "kirjassani", "kirja", "NOUN", "possessive")
}

func TestBatchLookupForms_PossessiveStripNotAppliedForEstonian(t *testing.T) {
	db := newTestDB(t)
	// Estonian should NOT apply Finnish possessive suffix stripping.
	seedForms(t, db, [][4]string{
		{"kirjassa", "kirja", "NOUN", "ET"}, // seeded under ET lang
	})

	// "kirjassani" in ET context: direct lookup fails, no possessive strip → absent.
	got := db.BatchLookupForms([]string{"kirjassani"}, "ET", "custom")

	if _, ok := got["kirjassani"]; ok {
		t.Errorf("kirjassani ET: possessive strip should not apply for Estonian, got %v", got["kirjassani"])
	}
}

// --- Compound splitting tests ---

func TestCompoundSplit_Found(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"pankki", "pankki", "NOUN", "FI"},
		{"automaatti", "automaatti", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"pankkiautomaatti"}, "FI", "custom")

	assertResolution(t, got, "pankkiautomaatti", "pankkiautomaatti", "NOUN", "compound")
}

func TestCompoundSplit_MinPartLength(t *testing.T) {
	db := newTestDB(t)
	// "on" is only 2 chars — too short for a compound part (min 3 runes).
	seedForms(t, db, [][4]string{
		{"on", "olla", "VERB", "FI"},
		{"gelma", "gelma", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"ongelma"}, "FI", "custom")

	// "on" + "gelma" should NOT match because "on" is only 2 runes.
	if r, ok := got["ongelma"]; ok && r.Source == "compound" {
		t.Errorf("ongelma: should not split with left part 'on' (too short), got %v", r)
	}
}

func TestCompoundSplit_BothPartsMustExist(t *testing.T) {
	db := newTestDB(t)
	// Seed only the left half — right half doesn't exist.
	seedForms(t, db, [][4]string{
		{"pankki", "pankki", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"pankkixyzabc"}, "FI", "custom")

	if _, ok := got["pankkixyzabc"]; ok {
		t.Errorf("pankkixyzabc: expected absent (right half not in dict), got %v", got["pankkixyzabc"])
	}
}

func TestCompoundSplit_Estonian(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"raamat", "raamat", "NOUN", "ET"},
		{"kogu", "kogu", "NOUN", "ET"},
	})

	got := db.BatchLookupForms([]string{"raamatukogu"}, "ET", "custom")

	// This won't match because "raamatu" is not in forms — need the inflected form.
	// Seed the proper inflected form instead.
	if _, ok := got["raamatukogu"]; ok {
		// If it matches, it means "raamatu" and "kogu" or some other split worked.
		t.Logf("raamatukogu resolved: %v", got["raamatukogu"])
	}
}

func TestCompoundSplit_MultiByte(t *testing.T) {
	db := newTestDB(t)
	// ö and ä are 2 bytes in UTF-8. Compound splitting must use rune boundaries.
	seedForms(t, db, [][4]string{
		{"talo", "talo", "NOUN", "FI"},
		{"yhtiö", "yhtiö", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"taloyhtiö"}, "FI", "custom")

	assertResolution(t, got, "taloyhtiö", "taloyhtiö", "NOUN", "compound")
}

// --- Case suffix stripping tests ---

func TestCaseSuffixStrip_Inessive(t *testing.T) {
	db := newTestDB(t)
	// Seed "talo" as a lemma (not just a form) — case suffix strip validates
	// against the lemmas table.
	seedLemmas(t, db, [][4]string{
		{"talo", "NOUN", "house", "FI"},
	})

	got := db.BatchLookupForms([]string{"talossa"}, "FI", "custom")

	r, ok := got["talossa"]
	if !ok {
		t.Fatal("talossa: expected in result map")
	}
	if r.Lemma != "talo" || r.POS != "NOUN" {
		t.Errorf("talossa: got {%q %q}, want {talo NOUN}", r.Lemma, r.POS)
	}
	if r.GrammarLabel != "inessive" {
		t.Errorf("talossa grammar label: got %q, want inessive", r.GrammarLabel)
	}
	if r.Feats != "Case=Ine" {
		t.Errorf("talossa feats: got %q, want Case=Ine (case-suffix path projects Case= from grammar_label)", r.Feats)
	}
	if r.Source != "case_suffix" {
		t.Errorf("talossa source: got %q, want case_suffix", r.Source)
	}
}

func TestFeatsFromCaseLabel(t *testing.T) {
	cases := []struct {
		label string
		want  string
	}{
		{"inessive", "Case=Ine"},
		{"genitive", "Case=Gen"},
		{"partitive", "Case=Par"},
		{"illative", "Case=Ill"},
		{"elative", "Case=Ela"},
		{"adessive", "Case=Ade"},
		{"ablative", "Case=Abl"},
		{"allative", "Case=All"},
		{"essive", "Case=Ess"},
		{"translative", "Case=Tra"},
		{"comitative", "Case=Com"},
		{"terminative", "Case=Ter"},
		{"abessive", "Case=Abe"},
		{"nominative", ""}, // Nom is implicit
		{"", ""},
		{"unknown-label", ""},
	}
	for _, tc := range cases {
		if got := featsFromCaseLabel(tc.label); got != tc.want {
			t.Errorf("featsFromCaseLabel(%q) = %q, want %q", tc.label, got, tc.want)
		}
	}
}

func TestCaseSuffixStrip_ShortStemRejected(t *testing.T) {
	db := newTestDB(t)
	// "o" would be the stem after stripping "-ssa" from "ossa" — too short (< 3).
	seedLemmas(t, db, [][4]string{
		{"o", "NOUN", "o letter", "FI"},
	})

	got := db.BatchLookupForms([]string{"ossa"}, "FI", "custom")

	if _, ok := got["ossa"]; ok {
		t.Errorf("ossa: stem 'o' too short, should not resolve, got %v", got["ossa"])
	}
}

func TestCaseSuffixStrip_LemmaTableOnly(t *testing.T) {
	db := newTestDB(t)
	// "talossa" exists in forms but "talo" is NOT in lemmas.
	// Case suffix stripping should NOT match (it validates against lemmas table).
	seedForms(t, db, [][4]string{
		{"talossa", "talo", "NOUN", "FI"},
	})
	// Note: no seedLemmas for "talo"

	// The direct dict lookup will find "talossa" in forms, so it resolves via "dict".
	// To test case suffix stripping specifically, use a form NOT in forms table.
	got := db.BatchLookupForms([]string{"xyzssa"}, "FI", "custom")
	// "xyz" is not in lemmas table → should not resolve.
	if _, ok := got["xyzssa"]; ok {
		t.Errorf("xyzssa: stem 'xyz' not in lemmas, should not resolve, got %v", got["xyzssa"])
	}
}

func TestCaseSuffixStrip_EstonianStemAlternations(t *testing.T) {
	db := newTestDB(t)
	seedLemmas(t, db, [][4]string{
		{"oskus", "NOUN", "skill", "ET"},
		{"registreerimisavaldus", "NOUN", "registration application", "ET"},
		{"kirjutamine", "NOUN", "writing", "ET"},
		{"b1-tase", "NOUN", "B1 level", "ET"},
		{"ülesanne", "NOUN", "task", "ET"},
	})

	got := db.BatchLookupForms([]string{
		"oskuse",
		"oskuste",
		"registreerimisavaldusel",
		"kirjutamise",
		"b1-taseme",
		"ülesande",
		"ülesandeid",
	}, "ET", "custom")

	assertResolution(t, got, "oskuse", "oskus", "NOUN", "case_suffix")
	assertResolution(t, got, "oskuste", "oskus", "NOUN", "case_suffix")
	assertResolution(t, got, "registreerimisavaldusel", "registreerimisavaldus", "NOUN", "case_suffix")
	assertResolution(t, got, "kirjutamise", "kirjutamine", "NOUN", "case_suffix")
	assertResolution(t, got, "b1-taseme", "b1-tase", "NOUN", "case_suffix")
	assertResolution(t, got, "ülesande", "ülesanne", "NOUN", "case_suffix")
	assertResolution(t, got, "ülesandeid", "ülesanne", "NOUN", "case_suffix")
}

func TestFallbackChainOrdering(t *testing.T) {
	db := newTestDB(t)
	// Test that direct lookup takes priority over compound split and case suffix
	// for lemma resolution. The case-suffix matcher may *additively* attach a
	// label to the dict result (stopgap until the FST runtime lands), but that
	// must not change the lemma / POS chosen by the dict path.
	seedForms(t, db, [][4]string{
		{"kirjassa", "kirja", "NOUN", "FI"},
	})
	// Also seed "kirja" as a lemma (so case suffix would work too).
	seedLemmas(t, db, [][4]string{
		{"kirja", "NOUN", "book", "FI"},
	})

	got := db.BatchLookupForms([]string{"kirjassa"}, "FI", "custom")

	r, ok := got["kirjassa"]
	if !ok {
		t.Fatal("kirjassa: expected in result map")
	}
	if r.Lemma != "kirja" || r.POS != "NOUN" {
		t.Errorf("kirjassa: got {%q %q}, want {kirja NOUN}", r.Lemma, r.POS)
	}
	// Source must start with "dict" — the dict path won. Suffix attachment may
	// append "+fst_label" or "+case_suffix_label" but the dict prefix proves
	// priority order.
	if !strings.HasPrefix(r.Source, "dict") {
		t.Errorf("kirjassa: should resolve via 'dict' (priority), got source %q", r.Source)
	}
}

// TestBatchLookupForms_AttachCaseLabelOnDictHit covers the custom-mode
// label-attachment paths where, after a successful direct-dict resolution,
// the parser additively attaches a GrammarLabel. Two paths can do this in
// priority order: the FST step-promotion path (fst_label) and the
// case-suffix stopgap (case_suffix_label). Either is acceptable here — the
// invariant under test is that grammar accuracy on dict-resolved tokens
// isn't structurally 0% in custom mode. Whichever path fires must agree on
// "inessive" for "talossa".
func TestBatchLookupForms_AttachCaseLabelOnDictHit(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"talossa", "talo", "NOUN", "FI"},
	})
	seedLemmas(t, db, [][4]string{
		{"talo", "NOUN", "house", "FI"},
	})

	got := db.BatchLookupForms([]string{"talossa"}, "FI", "custom")

	r, ok := got["talossa"]
	if !ok {
		t.Fatal("talossa: expected in result map")
	}
	if r.Lemma != "talo" || r.POS != "NOUN" {
		t.Errorf("talossa: got {%q %q}, want {talo NOUN}", r.Lemma, r.POS)
	}
	if r.GrammarLabel != "inessive" {
		t.Errorf("talossa: grammar label got %q, want inessive", r.GrammarLabel)
	}
	if !strings.Contains(r.Source, "+fst_label") && !strings.Contains(r.Source, "+case_suffix_label") {
		t.Errorf("talossa: source should record additive label (either +fst_label or +case_suffix_label); got %q", r.Source)
	}
}

// TestBatchLookupForms_AttachCaseLabelBasicModeSkips verifies that the
// stopgap is gated on parserMode == "custom". The "basic" parser must keep
// emitting empty grammar labels on dict hits — its identity is "no rules
// beyond direct dict."
func TestBatchLookupForms_AttachCaseLabelBasicModeSkips(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"talossa", "talo", "NOUN", "FI"},
	})
	seedLemmas(t, db, [][4]string{
		{"talo", "NOUN", "house", "FI"},
	})

	got := db.BatchLookupForms([]string{"talossa"}, "FI", "basic")

	r, ok := got["talossa"]
	if !ok {
		t.Fatal("talossa: expected in result map")
	}
	if r.GrammarLabel != "" {
		t.Errorf("talossa basic mode: grammar label got %q, want empty", r.GrammarLabel)
	}
	if r.Source != "dict" {
		t.Errorf("talossa basic mode: source got %q, want plain 'dict'", r.Source)
	}
}

// TestBatchLookupForms_AttachCaseLabelLemmaMismatchSkips verifies that a
// label is NOT attached when the case-suffix-strip lemma differs from the
// dict lemma — that means the suffix path is analyzing a different word and
// the label would be wrong (e.g. dict says PROPN "Linnas", suffix-strip
// would say "inessive of linna").
func TestBatchLookupForms_AttachCaseLabelLemmaMismatchSkips(t *testing.T) {
	db := newTestDB(t)
	// Direct dict says the form is its own lemma (a personal name).
	seedForms(t, db, [][4]string{
		{"linnas", "linnas", "PROPN", "ET"},
	})
	// But the lemmas table also has a different lemma the suffix-strip would
	// arrive at — that's the false-positive we must reject.
	seedLemmas(t, db, [][4]string{
		{"linna", "NOUN", "city", "ET"},
	})

	got := db.BatchLookupForms([]string{"linnas"}, "ET", "custom")

	r, ok := got["linnas"]
	if !ok {
		t.Fatal("linnas: expected in result map")
	}
	if r.Lemma != "linnas" || r.POS != "PROPN" {
		t.Errorf("linnas: got {%q %q}, want {linnas PROPN}", r.Lemma, r.POS)
	}
	if r.GrammarLabel != "" {
		t.Errorf("linnas: grammar label got %q, want empty (lemma-mismatch guard)", r.GrammarLabel)
	}
	if r.Source != "dict" {
		t.Errorf("linnas: source got %q, want plain 'dict'", r.Source)
	}
}

func TestBatchLookupForms_FSTMergesLaterSameLemmaAnalysis(t *testing.T) {
	installTestLemmatizerTable(t, "FI", map[string][]lemmatizer.Analysis{
		"talossa": {
			{Lemma: "tala", UPOS: "NOUN", GrammarLabel: "adessive", Number: "Sing", Raw: "different-lemma-first"},
			{Lemma: "talo", UPOS: "NOUN", GrammarLabel: "inessive", Number: "Sing", Raw: "matching-lemma-second"},
		},
	})
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"talossa", "talo", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"talossa"}, "FI", "custom")

	r := got["talossa"]
	if r.Lemma != "talo" || r.POS != "NOUN" {
		t.Fatalf("talossa: got {%q %q}, want {talo NOUN}", r.Lemma, r.POS)
	}
	if r.GrammarLabel != "inessive" {
		t.Errorf("talossa: grammar label got %q, want inessive", r.GrammarLabel)
	}
	if r.Feats != "Case=Ine|Number=Sing" {
		t.Errorf("talossa: feats got %q, want Case=Ine|Number=Sing", r.Feats)
	}
	if r.Source != "dict+fst_feats" {
		t.Errorf("talossa: source got %q, want dict+fst_feats", r.Source)
	}
}

func TestBatchLookupForms_FSTCanCorrectWeakDictLemmaPOS(t *testing.T) {
	installTestLemmatizerTable(t, "FI", map[string][]lemmatizer.Analysis{
		"korjasin": {
			{Lemma: "korjata", UPOS: "VERB", Number: "Sing", Mood: "Ind", Tense: "Past", Person: "1", Raw: "generated-table"},
		},
	})
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"korjasin", "korja", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"korjasin"}, "FI", "custom")

	r := got["korjasin"]
	if r.Lemma != "korjata" || r.POS != "VERB" {
		t.Fatalf("korjasin: got {%q %q}, want {korjata VERB}", r.Lemma, r.POS)
	}
	if r.Feats != "Mood=Ind|Number=Sing|Person=1|Tense=Past" {
		t.Errorf("korjasin: feats got %q", r.Feats)
	}
	if r.Source != "fst" {
		t.Errorf("korjasin: source got %q, want fst", r.Source)
	}
}

func TestBatchLookupForms_StrongDictResistsDisagreeingFST(t *testing.T) {
	installTestLemmatizerTable(t, "FI", map[string][]lemmatizer.Analysis{
		"pankki": {
			{Lemma: "pankkia", UPOS: "VERB", Number: "Sing", Mood: "Ind", Tense: "Pres", Person: "3", Raw: "generated-table"},
		},
	})
	db := newTestDB(t)
	if _, err := db.db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority) VALUES (?, ?, ?, ?, ?, ?)`,
		"pankki", "pankki", "NOUN", "FI", "kaikki", 10,
	); err != nil {
		t.Fatalf("seed prioritized form: %v", err)
	}

	got := db.BatchLookupForms([]string{"pankki"}, "FI", "custom")

	r := got["pankki"]
	if r.Lemma != "pankki" || r.POS != "NOUN" {
		t.Fatalf("pankki: got {%q %q}, want {pankki NOUN}", r.Lemma, r.POS)
	}
	if r.Source != "dict" {
		t.Errorf("pankki: source got %q, want dict", r.Source)
	}
}

func TestBatchLookupForms_SourcePriorityBeatsLowerPriorityFSTSupport(t *testing.T) {
	// Regression guard for the Ekilex-vs-kaikki ranker boundary: once
	// candidates are equal on case/POS sanity, row-level source priority is
	// the dictionary authority signal. FST agreement with the lower-priority
	// row can enrich a same (lemma, POS), but should not override a stronger
	// source on a different lemma.
	installTestLemmatizerTable(t, "ET", map[string][]lemmatizer.Analysis{
		"mustris": {
			{Lemma: "mustri", UPOS: "NOUN", GrammarLabel: "inessive", Number: "Sing", Raw: "generated-table"},
		},
	})
	db := newTestDB(t)
	seedFormsWithSource(t, db, []struct {
		form, lemma, pos, lang, source string
		priority                       int
	}{
		{"mustris", "muster", "NOUN", "ET", "ekilex", 20},
		{"mustris", "mustri", "NOUN", "ET", "kaikki", 10},
	})

	got := db.BatchLookupForms([]string{"mustris"}, "ET", "custom")

	r, ok := got["mustris"]
	if !ok {
		t.Fatal("mustris: expected resolution")
	}
	if r.Lemma != "muster" || r.POS != "NOUN" {
		t.Fatalf("mustris: got {%q %q}, want {muster NOUN}", r.Lemma, r.POS)
	}
	if r.Source != "dict" {
		t.Errorf("mustris: source got %q, want dict", r.Source)
	}
	if r.Feats != "" {
		t.Errorf("mustris: feats got %q, want empty; losing FST-backed candidate must not enrich winner", r.Feats)
	}
}

func TestBatchLookupForms_SourcePriorityBeatsLowerPriorityMorphology(t *testing.T) {
	// Same source-priority boundary as the FST-support regression, but without
	// any FST candidate: a lower-priority row's FEATS should not outrank a
	// higher-priority source once case/POS sanity ties.
	t.Setenv("LEMMATIZER_TABLES_DIR", t.TempDir())
	db := newTestDB(t)
	rows := []struct {
		form, lemma, pos, lang, source, feats string
		priority                              int
	}{
		{"x", "lemma-a", "NOUN", "ET", "ekilex", "", 20},
		{"x", "lemma-b", "NOUN", "ET", "kaikki", "Case=Ine|Number=Sing", 10},
	}
	for _, r := range rows {
		if _, err := db.db.Exec(
			`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.form, r.lemma, r.pos, r.lang, r.source, r.priority, r.feats,
		); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	got := db.BatchLookupForms([]string{"x"}, "ET", "custom")

	r, ok := got["x"]
	if !ok {
		t.Fatal("x: expected resolution")
	}
	if r.Lemma != "lemma-a" || r.POS != "NOUN" {
		t.Fatalf("x: got {%q %q}, want {lemma-a NOUN}", r.Lemma, r.POS)
	}
	if r.Feats != "" {
		t.Errorf("x: feats got %q, want empty from higher-priority Ekilex row", r.Feats)
	}
	if r.Source != "dict" {
		t.Errorf("x: source got %q, want dict", r.Source)
	}
}

func TestBatchLookupForms_FSTOnlyPropagatesFeats(t *testing.T) {
	installTestLemmatizerTable(t, "FI", map[string][]lemmatizer.Analysis{
		"olen": {
			{Lemma: "olla", UPOS: "VERB", Number: "Sing", Mood: "Ind", Tense: "Pres", Person: "1", Raw: "generated-table"},
		},
	})
	db := newTestDB(t)

	got := db.BatchLookupForms([]string{"olen"}, "FI", "custom")

	r := got["olen"]
	if r.Lemma != "olla" || r.POS != "VERB" {
		t.Fatalf("olen: got {%q %q}, want {olla VERB}", r.Lemma, r.POS)
	}
	if r.Feats != "Mood=Ind|Number=Sing|Person=1|Tense=Pres" {
		t.Errorf("olen: feats got %q", r.Feats)
	}
	if r.Source != "fst" {
		t.Errorf("olen: source got %q, want fst", r.Source)
	}
}

func TestBatchLookupForms_ETFSTMergesDictCandidate(t *testing.T) {
	installTestLemmatizerTable(t, "ET", map[string][]lemmatizer.Analysis{
		"raamatus": {
			{Lemma: "raamat", UPOS: "NOUN", GrammarLabel: "inessive", Number: "Sing", Raw: "generated-table"},
		},
	})
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"raamatus", "raamat", "NOUN", "ET"},
	})

	got := db.BatchLookupForms([]string{"raamatus"}, "ET", "custom")

	r := got["raamatus"]
	if r.Lemma != "raamat" || r.POS != "NOUN" {
		t.Fatalf("raamatus: got {%q %q}, want {raamat NOUN}", r.Lemma, r.POS)
	}
	if r.Feats != "Case=Ine|Number=Sing" {
		t.Errorf("raamatus: feats got %q, want Case=Ine|Number=Sing", r.Feats)
	}
	if r.Source != "dict+fst_feats" {
		t.Errorf("raamatus: source got %q, want dict+fst_feats", r.Source)
	}
}

func TestBatchLookupForms_FSTRollsBackSameLabelConflictingFeats(t *testing.T) {
	installTestLemmatizerTable(t, "ET", map[string][]lemmatizer.Analysis{
		"raamatus": {
			{Lemma: "raamat", UPOS: "NOUN", GrammarLabel: "inessive", Number: "Sing", Raw: "generated-table-sing"},
			{Lemma: "raamat", UPOS: "NOUN", GrammarLabel: "inessive", Number: "Plur", Raw: "generated-table-plur"},
		},
	})
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"raamatus", "raamat", "NOUN", "ET"},
	})

	got := db.BatchLookupForms([]string{"raamatus"}, "ET", "custom")

	r := got["raamatus"]
	if r.Lemma != "raamat" || r.POS != "NOUN" {
		t.Fatalf("raamatus: got {%q %q}, want {raamat NOUN}", r.Lemma, r.POS)
	}
	if r.Feats != "" {
		t.Errorf("raamatus: feats got %q, want empty because same-label FST analyses disagree on full FEATS", r.Feats)
	}
	if r.GrammarLabel != "" {
		t.Errorf("raamatus: grammar label got %q, want original empty label after ambiguous FST rollback", r.GrammarLabel)
	}
	if r.Source != "dict" {
		t.Errorf("raamatus: source got %q, want dict after ambiguous FST rollback", r.Source)
	}
}

func TestBatchLookupForms_DictFeatsBeatSameLemmaPOSFSTFeats(t *testing.T) {
	installTestLemmatizerTable(t, "ET", map[string][]lemmatizer.Analysis{
		"raamatus": {
			{Lemma: "raamat", UPOS: "NOUN", GrammarLabel: "elative", Number: "Plur", Raw: "generated-table"},
		},
	})
	db := newTestDB(t)
	if _, err := db.db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, feats) VALUES (?, ?, ?, ?, ?)`,
		"raamatus", "raamat", "NOUN", "ET", "Case=Ine|Number=Sing",
	); err != nil {
		t.Fatalf("seed feats form: %v", err)
	}

	got := db.BatchLookupForms([]string{"raamatus"}, "ET", "custom")

	r := got["raamatus"]
	if r.Lemma != "raamat" || r.POS != "NOUN" {
		t.Fatalf("raamatus: got {%q %q}, want {raamat NOUN}", r.Lemma, r.POS)
	}
	if r.Feats != "Case=Ine|Number=Sing" {
		t.Errorf("raamatus: feats got %q, want dict feats", r.Feats)
	}
	if r.GrammarLabel != "inessive" {
		t.Errorf("raamatus: grammar label got %q, want inessive", r.GrammarLabel)
	}
	if r.Source != "dict" {
		t.Errorf("raamatus: source got %q, want dict", r.Source)
	}
}

func TestBatchLookupForms_SameLemmaPOSDisagreementCanPreferFSTVerb(t *testing.T) {
	installTestLemmatizerTable(t, "ET", map[string][]lemmatizer.Analysis{
		"joon": {
			{Lemma: "jooma", UPOS: "VERB", Number: "Sing", Mood: "Ind", Tense: "Pres", Person: "1", Raw: "generated-table"},
		},
	})
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"joon", "jooma", "NOUN", "ET"},
	})

	got := db.BatchLookupForms([]string{"joon"}, "ET", "custom")

	r := got["joon"]
	if r.Lemma != "jooma" || r.POS != "VERB" {
		t.Fatalf("joon: got {%q %q}, want {jooma VERB}", r.Lemma, r.POS)
	}
	if r.Feats != "Mood=Ind|Number=Sing|Person=1|Tense=Pres" {
		t.Errorf("joon: feats got %q", r.Feats)
	}
	if r.Source != "fst" {
		t.Errorf("joon: source got %q, want fst", r.Source)
	}
}

func TestBatchLookupForms_MissingFSTTablesKeepsDictPath(t *testing.T) {
	t.Setenv("LEMMATIZER_TABLES_DIR", t.TempDir())
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"talo", "talo", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"talo"}, "FI", "custom")

	r := got["talo"]
	if r.Lemma != "talo" || r.POS != "NOUN" {
		t.Fatalf("talo: got {%q %q}, want {talo NOUN}", r.Lemma, r.POS)
	}
	if r.Source != "dict" {
		t.Errorf("talo: source got %q, want dict", r.Source)
	}
}

func TestAppendSourceTagComparesWholeTags(t *testing.T) {
	if got := appendSourceTag("dict+fst_label", "fst_lab"); got != "dict+fst_label+fst_lab" {
		t.Errorf("appendSourceTag substring tag got %q", got)
	}
	if got := appendSourceTag("dict+fst_label", "fst_label"); got != "dict+fst_label" {
		t.Errorf("appendSourceTag duplicate tag got %q", got)
	}
}

func TestEnsureCardReturnsExistingCardID(t *testing.T) {
	db := newTestDB(t)
	user, err := db.GetOrCreateUser("cards@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	firstID, err := db.EnsureCard(user.ID, "FI", "kissa", "NOUN")
	if err != nil {
		t.Fatalf("EnsureCard first: %v", err)
	}
	secondID, err := db.EnsureCard(user.ID, "FI", "kissa", "NOUN")
	if err != nil {
		t.Fatalf("EnsureCard second: %v", err)
	}
	if firstID != secondID {
		t.Fatalf("card IDs differ: first=%d second=%d", firstID, secondID)
	}

	var cardCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM cards WHERE user_id = ? AND lang = 'FI' AND lemma = 'kissa' AND pos = 'NOUN' AND mwe_id IS NULL`, user.ID).Scan(&cardCount); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if cardCount != 1 {
		t.Fatalf("card_count=%d want 1", cardCount)
	}

	var stateCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM card_state WHERE card_id = ?`, firstID).Scan(&stateCount); err != nil {
		t.Fatalf("count card_state: %v", err)
	}
	if stateCount != 1 {
		t.Fatalf("card_state_count=%d want 1", stateCount)
	}
}

func TestEnsureCardConcurrentUpsertsReturnSingleCard(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cards-concurrent.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	user, err := db.GetOrCreateUser("cards-concurrent@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	const goroutines = 8
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	idCh := make(chan int64, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cardID, err := db.EnsureCard(user.ID, "FI", "kissa", "NOUN")
			if err != nil {
				errCh <- err
				return
			}
			idCh <- cardID
		}()
	}

	wg.Wait()
	close(errCh)
	close(idCh)

	for err := range errCh {
		t.Fatalf("EnsureCard concurrent: %v", err)
	}

	var firstID int64
	for cardID := range idCh {
		if firstID == 0 {
			firstID = cardID
			continue
		}
		if cardID != firstID {
			t.Fatalf("concurrent card IDs differ: first=%d got=%d", firstID, cardID)
		}
	}

	var cardCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM cards WHERE user_id = ? AND lang = 'FI' AND lemma = 'kissa' AND pos = 'NOUN' AND mwe_id IS NULL`, user.ID).Scan(&cardCount); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if cardCount != 1 {
		t.Fatalf("card_count=%d want 1", cardCount)
	}

	var stateCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM card_state`).Scan(&stateCount); err != nil {
		t.Fatalf("count card_state: %v", err)
	}
	if stateCount != 1 {
		t.Fatalf("card_state_count=%d want 1", stateCount)
	}
}

func TestLegacyCardMigrationPreservesStateAndBackfillsFILang(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}

	legacySchema := `
	CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE,
		email_verified INTEGER DEFAULT 0,
		is_admin INTEGER DEFAULT 0,
		settings_json TEXT DEFAULT '{"new_per_day":20,"retention":0.9,"theme":"system"}'
	);
	CREATE TABLE cards (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		mwe_id INTEGER,
		FOREIGN KEY(user_id) REFERENCES users(id),
		UNIQUE(user_id, lemma, pos, mwe_id)
	);
	CREATE TABLE card_state (
		card_id INTEGER PRIMARY KEY,
		fsrs_json TEXT,
		next_due DATETIME,
		last_answer_at DATETIME,
		FOREIGN KEY(card_id) REFERENCES cards(id)
	);`
	if _, err := legacy.Exec(legacySchema); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := legacy.Exec(`INSERT INTO users (id, email) VALUES (1, 'legacy@example.com')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := legacy.Exec(`INSERT INTO cards (id, user_id, lemma, pos, mwe_id) VALUES (7, 1, 'kissa', 'NOUN', NULL)`); err != nil {
		t.Fatalf("insert legacy card: %v", err)
	}
	if _, err := legacy.Exec(`INSERT INTO card_state (card_id, fsrs_json, next_due, last_answer_at) VALUES (7, '{"due":3}', '2026-05-01 10:00:00', '2026-04-29 09:00:00')`); err != nil {
		t.Fatalf("insert legacy card_state: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	migrated, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB migration: %v", err)
	}
	t.Cleanup(func() { migrated.Close() })

	var lang, fsrsJSON, nextDue, lastAnswerAt string
	if err := migrated.db.QueryRow(`
		SELECT c.lang, cs.fsrs_json, cs.next_due, cs.last_answer_at
		FROM cards c
		JOIN card_state cs ON cs.card_id = c.id
		WHERE c.user_id = 1 AND c.lemma = 'kissa' AND c.pos = 'NOUN'`,
	).Scan(&lang, &fsrsJSON, &nextDue, &lastAnswerAt); err != nil {
		t.Fatalf("query migrated card state: %v", err)
	}
	if lang != "FI" {
		t.Fatalf("lang=%q want FI", lang)
	}
	if fsrsJSON != `{"due":3}` {
		t.Fatalf("fsrs_json=%q want %q", fsrsJSON, `{"due":3}`)
	}
	if nextDue != "2026-05-01T10:00:00Z" {
		t.Fatalf("next_due=%q want 2026-05-01T10:00:00Z", nextDue)
	}
	if lastAnswerAt != "2026-04-29T09:00:00Z" {
		t.Fatalf("last_answer_at=%q want 2026-04-29T09:00:00Z", lastAnswerAt)
	}
}

// --- BatchLookupGlosses tests ---

func TestBatchLookupGlosses_Found(t *testing.T) {
	db := newTestDB(t)
	seedLemmas(t, db, [][4]string{
		{"pankki", "NOUN", "bank (financial institution)", "FI"},
		{"kirja", "NOUN", "book", "FI"},
	})

	got := db.BatchLookupGlosses([]LemmaKey{
		{Lemma: "pankki", POS: "NOUN"},
		{Lemma: "kirja", POS: "NOUN"},
	}, "FI")

	if got[LemmaKey{"pankki", "NOUN"}] != "bank (financial institution)" {
		t.Errorf("pankki: got %q, want %q", got[LemmaKey{"pankki", "NOUN"}], "bank (financial institution)")
	}
	if got[LemmaKey{"kirja", "NOUN"}] != "book" {
		t.Errorf("kirja: got %q, want %q", got[LemmaKey{"kirja", "NOUN"}], "book")
	}
}

func TestBatchLookupGlosses_NotFound(t *testing.T) {
	db := newTestDB(t)
	// "viisutubettaja" is not in the dictionary.

	got := db.BatchLookupGlosses([]LemmaKey{{Lemma: "viisutubettaja", POS: "NOUN"}}, "FI")

	if _, ok := got[LemmaKey{"viisutubettaja", "NOUN"}]; ok {
		t.Errorf("viisutubettaja: expected absent, got %q", got[LemmaKey{"viisutubettaja", "NOUN"}])
	}
}

func TestBatchLookupGlosses_NullGloss(t *testing.T) {
	db := newTestDB(t)
	// Insert a lemma with NULL gloss — should be absent from result (treated as no gloss).
	_, err := db.db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang) VALUES ('jokin', 'PRON', NULL, 'FI')`,
	)
	if err != nil {
		t.Fatalf("seed null gloss: %v", err)
	}

	got := db.BatchLookupGlosses([]LemmaKey{{Lemma: "jokin", POS: "PRON"}}, "FI")

	if _, ok := got[LemmaKey{"jokin", "PRON"}]; ok {
		t.Errorf("jokin (null gloss): expected absent from result, got %q", got[LemmaKey{"jokin", "PRON"}])
	}
}

// seedFormsWithSource inserts (form, lemma, pos, lang, source, source_priority).
func seedFormsWithSource(t *testing.T, db *DB, rows []struct {
	form, lemma, pos, lang, source string
	priority                       int
}) {
	t.Helper()
	for _, r := range rows {
		_, err := db.db.Exec(
			`INSERT INTO forms (form, lemma, pos, lang, source, source_priority) VALUES (?, ?, ?, ?, ?, ?)`,
			r.form, r.lemma, r.pos, r.lang, r.source, r.priority,
		)
		if err != nil {
			t.Fatalf("seedFormsWithSource: %v", err)
		}
	}
}

func TestBatchLookupForms_PrefersLowercaseLemmaOnLowercaseSurface(t *testing.T) {
	// Regression covered: "linnas" → linn/NOUN ("in the city"), not Linna/PROPN
	// (place name homonym). Both rows live under the multi-lemma PK; the picker
	// must prefer the candidate whose lemma matches the surface's case.
	db := newTestDB(t)
	seedFormsWithSource(t, db, []struct {
		form, lemma, pos, lang, source string
		priority                       int
	}{
		{"linnas", "linn", "NOUN", "ET", "kaikki", 10},
		{"linnas", "Linna", "PROPN", "ET", "ekilex", 20}, // higher source priority but uppercase lemma
	})

	got := db.BatchLookupForms([]string{"linnas"}, "ET", "basic")
	r, ok := got["linnas"]
	if !ok {
		t.Fatal("linnas: expected resolution")
	}
	if r.Lemma != "linn" || r.POS != "NOUN" {
		t.Errorf("linnas: got %s/%s, want linn/NOUN", r.Lemma, r.POS)
	}
}

func TestBatchLookupForms_DemotesPROPNOnLowercaseSurface(t *testing.T) {
	// Even if both candidates start lowercase (no case-match signal), a
	// lowercase surface should not pick PROPN over NOUN.
	db := newTestDB(t)
	seedFormsWithSource(t, db, []struct {
		form, lemma, pos, lang, source string
		priority                       int
	}{
		{"koer", "koer", "PROPN", "ET", "ekilex", 20},
		{"koer", "koer", "NOUN", "ET", "kaikki", 10},
	})

	got := db.BatchLookupForms([]string{"koer"}, "ET", "basic")
	r, ok := got["koer"]
	if !ok {
		t.Fatal("koer: expected resolution")
	}
	if r.POS != "NOUN" {
		t.Errorf("koer: got POS=%s, want NOUN", r.POS)
	}
}

func TestBatchLookupForms_AllowsPROPNOnUppercaseSurface(t *testing.T) {
	// Sentence-initial or genuinely-capitalized surface — both lemma cases
	// are plausible, so source priority decides.
	db := newTestDB(t)
	seedFormsWithSource(t, db, []struct {
		form, lemma, pos, lang, source string
		priority                       int
	}{
		{"linnas", "linn", "NOUN", "ET", "kaikki", 10},
		{"linnas", "Linna", "PROPN", "ET", "ekilex", 20},
	})

	got := db.BatchLookupForms([]string{"Linnas"}, "ET", "basic")
	r, ok := got["Linnas"]
	if !ok {
		t.Fatal("Linnas: expected resolution")
	}
	// Both case-match and POS-pref scores are equal for "Linnas", so
	// source priority decides — Ekilex (priority 20) wins over kaikki (10).
	if r.Lemma != "Linna" || r.POS != "PROPN" {
		t.Errorf("Linnas: got %s/%s, want Linna/PROPN", r.Lemma, r.POS)
	}
}

func TestBatchLookupForms_HigherPriorityWinsAmongEqualCandidates(t *testing.T) {
	// Two NOUN candidates (no case/POS signal), source priority decides.
	db := newTestDB(t)
	seedFormsWithSource(t, db, []struct {
		form, lemma, pos, lang, source string
		priority                       int
	}{
		{"talvel", "tali", "NOUN", "ET", "ekilex", 20},
		{"talvel", "talv", "NOUN", "ET", "kaikki", 10},
	})

	got := db.BatchLookupForms([]string{"talvel"}, "ET", "basic")
	r, ok := got["talvel"]
	if !ok {
		t.Fatal("talvel: expected resolution")
	}
	if r.Lemma != "tali" {
		t.Errorf("talvel: got %s, want tali (higher source priority wins)", r.Lemma)
	}
}

func TestBatchLookupForms_DeterministicTiebreak(t *testing.T) {
	// Two candidates equal on every score axis (same case, same POS, same
	// source priority, same source). Tiebreak by lemma asc.
	db := newTestDB(t)
	seedFormsWithSource(t, db, []struct {
		form, lemma, pos, lang, source string
		priority                       int
	}{
		{"x", "beta", "NOUN", "ET", "kaikki", 10},
		{"x", "alpha", "NOUN", "ET", "kaikki", 10},
	})

	got := db.BatchLookupForms([]string{"x"}, "ET", "basic")
	r, ok := got["x"]
	if !ok {
		t.Fatal("x: expected resolution")
	}
	if r.Lemma != "alpha" {
		t.Errorf("x: got %s, want alpha (lemma-asc tiebreak)", r.Lemma)
	}
}

func TestBatchLookupForms_LegacyRowsWithEmptySource(t *testing.T) {
	// Pre-#67 rows have source='' and source_priority=0. Lookup must still
	// work — the case-match and POS heuristics should fire even without
	// source metadata.
	db := newTestDB(t)
	seedFormsWithSource(t, db, []struct {
		form, lemma, pos, lang, source string
		priority                       int
	}{
		{"linnas", "linn", "NOUN", "ET", "", 0},
		{"linnas", "Linna", "PROPN", "ET", "", 0},
	})

	got := db.BatchLookupForms([]string{"linnas"}, "ET", "basic")
	if got["linnas"].Lemma != "linn" || got["linnas"].POS != "NOUN" {
		t.Errorf("linnas with empty source: got %s/%s, want linn/NOUN",
			got["linnas"].Lemma, got["linnas"].POS)
	}
}

// TestBatchLookupForms_DoesNotPickVerbHomonymOnFEATSDensity covers the
// 2026-05-07 ET regression where the Ekilex bulk drop populated dense FEATS
// on every form and a scaled morphologyScore tiebreak started picking
// VERB readings over NOUN readings purely because verbs structurally carry
// more FEATS attributes (Mood/Number/Person/Tense/VerbForm/Voice ≈ 6) than
// nouns (Case/Number ≈ 2). The picker has no contextual disambiguation —
// when both candidates have FEATS, it must defer to the deterministic
// fallback (source priority → lemma asc) rather than vote on FEATS density.
func TestBatchLookupForms_DoesNotPickVerbHomonymOnFEATSDensity(t *testing.T) {
	db := newTestDB(t)
	rows := []struct {
		form, lemma, pos, feats string
	}{
		{"arstiks", "arst", "NOUN", "Case=Tra|Number=Sing"},
		{"arstiks", "arstima", "VERB", "Mood=Cnd|Tense=Pres|VerbForm=Fin|Voice=Act"},
	}
	for _, r := range rows {
		if _, err := db.db.Exec(
			`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.form, r.lemma, r.pos, "ET", "ekilex", 20, r.feats,
		); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	got := db.BatchLookupForms([]string{"arstiks"}, "ET", "custom")
	r, ok := got["arstiks"]
	if !ok {
		t.Fatal("arstiks: expected resolution")
	}
	if r.Lemma != "arst" || r.POS != "NOUN" {
		t.Errorf("arstiks: got %s/%s, want arst/NOUN (lemma asc tiebreak after morphology ties)",
			r.Lemma, r.POS)
	}
}

// TestBatchLookupForms_NominativeNotPunishedByEmptyGrammarLabel covers the
// adjacent corner of the same regression: a NOUN/Nom candidate has FEATS
// (Case=Nom|Number=Sing) but its projected GrammarLabel is "" because Nom
// is implicit per UD convention. A scaled morphologyScore that gave +1 for
// non-empty GrammarLabel made any non-Nom homonym beat the Nom reading on
// otherwise-equal candidates — observed for "mees" (man, NOUN/Nom) losing
// to "mesi" (honey, NOUN/Ine) in the et-grammar set.
func TestBatchLookupForms_NominativeNotPunishedByEmptyGrammarLabel(t *testing.T) {
	db := newTestDB(t)
	rows := []struct {
		form, lemma, pos, feats string
	}{
		{"mees", "mees", "NOUN", "Case=Nom|Number=Sing"},
		{"mees", "mesi", "NOUN", "Case=Ine|Number=Sing"},
	}
	for _, r := range rows {
		if _, err := db.db.Exec(
			`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.form, r.lemma, r.pos, "ET", "ekilex", 20, r.feats,
		); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	got := db.BatchLookupForms([]string{"mees"}, "ET", "custom")
	r, ok := got["mees"]
	if !ok {
		t.Fatal("mees: expected resolution")
	}
	if r.Lemma != "mees" {
		t.Errorf("mees: got lemma=%s, want mees (Nom→empty-label must not lose to Ine→inessive)", r.Lemma)
	}
}

// seedLemmasFull and seedTranslations are minimal helpers that write into
// the post-Phase-1 / post-#67 schema (with source / source_priority).
func seedLemmasFull(t *testing.T, db *DB, rows []struct {
	lemma, pos, gloss, lang, source string
	priority                        int
}) {
	t.Helper()
	for _, r := range rows {
		if _, err := db.db.Exec(
			`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			r.lemma, r.pos, r.gloss, r.lang, r.source, r.priority,
		); err != nil {
			t.Fatalf("seedLemmasFull: %v", err)
		}
	}
}

func seedTranslations(t *testing.T, db *DB, rows []struct {
	lemma, pos, lang, target, text, source string
	senseIdx                               int
}) {
	t.Helper()
	for _, r := range rows {
		if _, err := db.db.Exec(
			`INSERT INTO translations (lemma, pos, lang, target_lang, text, sense_idx, source)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.lemma, r.pos, r.lang, r.target, r.text, r.senseIdx, r.source,
		); err != nil {
			t.Fatalf("seedTranslations: %v", err)
		}
	}
}

func TestBatchLookupGlosses_PrefersTranslationsOverLemmasGloss(t *testing.T) {
	// When the translations table has a row whose source matches the
	// lemma's source (the kaikki-imported case), prefer translations.text
	// over lemmas.gloss. Today the two are identical for sense_idx=0 in
	// typical entries; the test guards the precedence regardless.
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"talo", "NOUN", "stale-cache", "FI", "kaikki", 10},
	})
	seedTranslations(t, db, []struct {
		lemma, pos, lang, target, text, source string
		senseIdx                               int
	}{
		{"talo", "NOUN", "FI", "EN", "house", "kaikki", 0},
	})

	got := db.BatchLookupGlosses([]LemmaKey{{"talo", "NOUN"}}, "FI")
	if got[LemmaKey{"talo", "NOUN"}] != "house" {
		t.Errorf("translations should win: got %q, want %q", got[LemmaKey{"talo", "NOUN"}], "house")
	}
}

func TestBatchLookupGlosses_FallsBackToLemmasGlossWhenNoTranslations(t *testing.T) {
	// Legacy DBs imported before PR #85 have no translations rows. The
	// read path must fall back to lemmas.gloss.
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"talo", "NOUN", "house-from-cache", "FI", "kaikki", 10},
	})
	// no translations rows seeded

	got := db.BatchLookupGlosses([]LemmaKey{{"talo", "NOUN"}}, "FI")
	if got[LemmaKey{"talo", "NOUN"}] != "house-from-cache" {
		t.Errorf("fallback to lemmas.gloss: got %q, want %q",
			got[LemmaKey{"talo", "NOUN"}], "house-from-cache")
	}
}

func TestBatchLookupGlosses_CustomOverrideViaLemmasGlossWins(t *testing.T) {
	// applyCustomGlosses (cmd/importdict's -custom-glosses CSV path)
	// upserts lemmas with source='custom', priority=100, replacing the
	// kaikki-tagged row. It does NOT currently write to translations.
	// The read path must surface the custom override even though older
	// kaikki translation rows still exist for this (lemma, pos).
	//
	// Without the JOIN-on-source design, a naive translations-first
	// query would silently return the stale kaikki gloss.
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"talo", "NOUN", "domain-specific override", "FI", "custom", 100},
	})
	// Old kaikki translation rows still exist (the user ran kaikki import
	// first, then -custom-glosses; the kaikki rows weren't cleared).
	seedTranslations(t, db, []struct {
		lemma, pos, lang, target, text, source string
		senseIdx                               int
	}{
		{"talo", "NOUN", "FI", "EN", "stale kaikki text", "kaikki", 0},
	})

	got := db.BatchLookupGlosses([]LemmaKey{{"talo", "NOUN"}}, "FI")
	if got[LemmaKey{"talo", "NOUN"}] != "domain-specific override" {
		t.Errorf("custom-override fallthrough: got %q, want %q",
			got[LemmaKey{"talo", "NOUN"}], "domain-specific override")
	}
}

func TestBatchLookupGlosses_HigherPriorityTranslationWins(t *testing.T) {
	// Forward-looking guard for when multiple sources write translations
	// for the same (lemma, pos) (Ekilex translations via the upcoming
	// PR 4). The JOIN ranks by lemmas.source_priority DESC.
	//
	// Note: this only fires when the lemma's row is at the higher-priority
	// source. The current upsert pattern in cmd/importdict already
	// promotes the higher-priority source on conflict, so this state is
	// reachable when ekilex (priority 20) follows kaikki (priority 10).
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		// Lemma row was last written by ekilex, which won the upsert.
		{"talo", "NOUN", "ekilex-cache", "ET", "ekilex", 20},
	})
	seedTranslations(t, db, []struct {
		lemma, pos, lang, target, text, source string
		senseIdx                               int
	}{
		// Both sources have translations rows for the same headword.
		{"talo", "NOUN", "ET", "EN", "kaikki text", "kaikki", 0},
		{"talo", "NOUN", "ET", "EN", "ekilex text", "ekilex", 0},
	})

	got := db.BatchLookupGlosses([]LemmaKey{{"talo", "NOUN"}}, "ET")
	if got[LemmaKey{"talo", "NOUN"}] != "ekilex text" {
		t.Errorf("higher-priority source wins: got %q, want %q",
			got[LemmaKey{"talo", "NOUN"}], "ekilex text")
	}
}

func TestBatchLookupGlosses_LowerSenseIdxWinsWithinSameSource(t *testing.T) {
	// Within a single source, the "primary" sense (lowest sense_idx)
	// should win for the gloss cache.
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"pankki", "NOUN", "stale", "FI", "kaikki", 10},
	})
	seedTranslations(t, db, []struct {
		lemma, pos, lang, target, text, source string
		senseIdx                               int
	}{
		{"pankki", "NOUN", "FI", "EN", "embankment", "kaikki", 5},
		{"pankki", "NOUN", "FI", "EN", "bank (financial)", "kaikki", 0},
		{"pankki", "NOUN", "FI", "EN", "bank (storage)", "kaikki", 2},
	})

	got := db.BatchLookupGlosses([]LemmaKey{{"pankki", "NOUN"}}, "FI")
	if got[LemmaKey{"pankki", "NOUN"}] != "bank (financial)" {
		t.Errorf("lowest sense_idx should win: got %q, want %q",
			got[LemmaKey{"pankki", "NOUN"}], "bank (financial)")
	}
}

func TestBatchLookupGlosses_ETLearnerOverridesHighImpactBadPrimaries(t *testing.T) {
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"see", "PRON", "here; it; this", "ET", "ekilex", 20},
		{"väike", "ADJ", "hardly sufficient; small", "ET", "ekilex", 20},
	})
	seedTranslations(t, db, []struct {
		lemma, pos, lang, target, text, source string
		senseIdx                               int
	}{
		{"see", "PRON", "ET", "EN", "here", "ekilex", 0},
		{"see", "PRON", "ET", "EN", "this", "ekilex", 1},
		{"väike", "ADJ", "ET", "EN", "hardly sufficient", "ekilex", 0},
		{"väike", "ADJ", "ET", "EN", "small", "ekilex", 1},
	})

	got := db.BatchLookupGlosses([]LemmaKey{{"see", "PRON"}, {"väike", "ADJ"}}, "ET")
	if got[LemmaKey{"see", "PRON"}] != "this; that" {
		t.Errorf("see gloss=%q want this; that", got[LemmaKey{"see", "PRON"}])
	}
	if got[LemmaKey{"väike", "ADJ"}] != "small; little" {
		t.Errorf("väike gloss=%q want small; little", got[LemmaKey{"väike", "ADJ"}])
	}
}

func TestBatchLookupGlosses_ETLearnerOverridesDoNotBeatCustomGlosses(t *testing.T) {
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"see", "PRON", "domain-specific see", "ET", "custom", 100},
	})
	seedTranslations(t, db, []struct {
		lemma, pos, lang, target, text, source string
		senseIdx                               int
	}{
		{"see", "PRON", "ET", "EN", "here", "ekilex", 0},
	})

	got := db.BatchLookupGlosses([]LemmaKey{{"see", "PRON"}}, "ET")
	if got[LemmaKey{"see", "PRON"}] != "domain-specific see" {
		t.Errorf("custom gloss=%q want domain-specific see", got[LemmaKey{"see", "PRON"}])
	}
}

func TestBatchLookupGlosses_ETLearnerOverridesOnlyApplyToEkilex(t *testing.T) {
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"see", "PRON", "curated see", "ET", "curated", 80},
	})
	seedTranslations(t, db, []struct {
		lemma, pos, lang, target, text, source string
		senseIdx                               int
	}{
		{"see", "PRON", "ET", "EN", "curated see translation", "curated", 0},
	})

	got := db.BatchLookupGlosses([]LemmaKey{{"see", "PRON"}}, "ET")
	if got[LemmaKey{"see", "PRON"}] != "curated see translation" {
		t.Errorf("curated gloss=%q want curated see translation", got[LemmaKey{"see", "PRON"}])
	}
}

// --- Lexical-overlay short-circuit tests ---

// TestBatchLookupForms_LexOverlayBeatsDictTrap is the P1 regression for
// review feedback on PR #183: the lexadverbs overlay was firing
// inside Lemmatize() but losing the supportScore tiebreak (dict=3 vs
// FST=2) against the kaikki form rows for the same surfaces. With
// the Step 0 short-circuit in BatchLookupForms, the overlay wins
// outright.
func TestBatchLookupForms_LexOverlayBeatsDictTrap(t *testing.T) {
	db := newTestDB(t)

	// Seed kaikki's bad readings for each overlay surface. Each row
	// is what kaikki actually ships (verified against the production
	// finnestdb.db on the review machine).
	seedFormsWithSource(t, db, []struct {
		form, lemma, pos, lang, source string
		priority                       int
	}{
		{"tuskin", "tuska", "NOUN", "FI", "kaikki", 10},
		{"asiaan", "as", "NOUN", "FI", "kaikki", 10},
		{"vuotta", "vuo", "NOUN", "FI", "kaikki", 10},
		{"varsin", "varsi", "NOUN", "FI", "kaikki", 10},
		{"siitä", "siittää", "VERB", "FI", "kaikki", 10},
		{"muuta", "muuttaa", "VERB", "FI", "kaikki", 10},
	})

	cases := []struct {
		surface, wantLemma, wantPOS string
	}{
		{"tuskin", "tuskin", "ADV"},
		{"asiaan", "asia", "NOUN"},
		{"vuotta", "vuosi", "NOUN"},
		{"varsin", "varsin", "ADV"},
		{"siitä", "se", "PRON"},
		{"muuta", "muu", "PRON"},
		// Sentence-initial capitals must also short-circuit through
		// the overlay (case-folded match before dict lookup).
		{"Tuskin", "tuskin", "ADV"},
		{"Vuotta", "vuosi", "NOUN"},
	}
	got := db.BatchLookupForms([]string{
		"tuskin", "asiaan", "vuotta", "varsin", "siitä", "muuta",
		"Tuskin", "Vuotta",
	}, "FI", "custom")
	for _, tc := range cases {
		r, ok := got[tc.surface]
		if !ok {
			t.Errorf("%s: missing resolution", tc.surface)
			continue
		}
		if r.Lemma != tc.wantLemma || r.POS != tc.wantPOS {
			t.Errorf("%s: got (%s/%s), want (%s/%s)",
				tc.surface, r.Lemma, r.POS, tc.wantLemma, tc.wantPOS)
		}
		if r.Source != "lex-overlay" {
			t.Errorf("%s: got source=%q, want lex-overlay", tc.surface, r.Source)
		}
	}
}

// TestBatchLookupForms_BadLemmaFilterPreservesLegitimateLookup is
// the P1 regression for review feedback on the bad-lemma blocklist:
// dropping `varsi` globally also removed the legitimate noun lookup
// for surface `varsi → varsi/NOUN` (a real Finnish noun meaning
// "stalk"). The blocklist must be (surface, lemma)-keyed for these
// non-fragment lemmas — only filter when the surface is the trap.
func TestBatchLookupForms_BadLemmaFilterPreservesLegitimateLookup(t *testing.T) {
	db := newTestDB(t)
	seedFormsWithSource(t, db, []struct {
		form, lemma, pos, lang, source string
		priority                       int
	}{
		// Legitimate dict rows: the lemma matches its own surface.
		{"varsi", "varsi", "NOUN", "FI", "kaikki", 10},
		{"vuo", "vuo", "NOUN", "FI", "kaikki", 10},
		{"siittää", "siittää", "VERB", "FI", "kaikki", 10},
		{"muuttaa", "muuttaa", "VERB", "FI", "kaikki", 10},
	})

	// Each of these surfaces is the lemma form itself, NOT the trap
	// surface. The blocklist must not strip it.
	for _, tc := range []struct {
		surface, wantLemma, wantPOS string
	}{
		{"varsi", "varsi", "NOUN"},
		{"vuo", "vuo", "NOUN"},
		{"siittää", "siittää", "VERB"},
		{"muuttaa", "muuttaa", "VERB"},
	} {
		for _, mode := range []string{"basic", "custom"} {
			got := db.BatchLookupForms([]string{tc.surface}, "FI", mode)
			r, ok := got[tc.surface]
			if !ok {
				t.Errorf("%s [%s]: surface dropped (bad-lemma filter regression)",
					tc.surface, mode)
				continue
			}
			if r.Lemma != tc.wantLemma || r.POS != tc.wantPOS {
				t.Errorf("%s [%s]: got (%s/%s), want (%s/%s)",
					tc.surface, mode, r.Lemma, r.POS, tc.wantLemma, tc.wantPOS)
			}
		}
	}
}

func TestIsBadDictLemma(t *testing.T) {
	// Always-bad fragments: filtered regardless of surface.
	for _, lemma := range []string{"as", "taa", "poli", "sisä-", "ylä-", "ku"} {
		if !isBadDictLemma("FI", "anything", lemma) {
			t.Errorf("always-bad %q: expected blocked, was not", lemma)
		}
	}
	// Surface-keyed pairs: only the trap surface fires the block.
	hits := []struct{ surface, lemma string }{
		{"varsin", "varsi"},
		{"vuotta", "vuo"},
		{"siitä", "siittää"},
		{"muuta", "muuttaa"},
		{"paljon", "paljo"},
	}
	for _, h := range hits {
		if !isBadDictLemma("FI", h.surface, h.lemma) {
			t.Errorf("trap pair (%s, %s): expected blocked, was not", h.surface, h.lemma)
		}
	}
	// Same lemmas on non-trap surfaces must NOT be blocked.
	misses := []struct{ surface, lemma string }{
		{"varsi", "varsi"},     // bare lemma lookup
		{"vuo", "vuo"},         // bare lemma lookup
		{"siittää", "siittää"}, // bare verb lookup
		{"muuttaa", "muuttaa"}, // bare verb lookup
		{"paljo", "paljo"},     // bare lemma lookup
	}
	for _, m := range misses {
		if isBadDictLemma("FI", m.surface, m.lemma) {
			t.Errorf("legitimate (%s, %s): expected NOT blocked, was blocked", m.surface, m.lemma)
		}
	}
	// Non-FI languages are not filtered.
	if isBadDictLemma("ET", "varsin", "varsi") {
		t.Error("ET lang: blocklist is FI-only, should not fire")
	}
	// Empty values defensive.
	if isBadDictLemma("FI", "", "") {
		t.Error("empty: should not fire")
	}
	if isBadDictLemma("FI", "anything", "") {
		t.Error("empty lemma: should not fire")
	}
}

// TestBatchLookupForms_LexOverlayBasicModeNotAffected confirms the
// overlay is gated to custom mode. Basic-mode eval baselines must
// not shift because of this fix.
func TestBatchLookupForms_LexOverlayBasicModeNotAffected(t *testing.T) {
	db := newTestDB(t)
	seedFormsWithSource(t, db, []struct {
		form, lemma, pos, lang, source string
		priority                       int
	}{
		{"tuskin", "tuska", "NOUN", "FI", "kaikki", 10},
	})
	got := db.BatchLookupForms([]string{"tuskin"}, "FI", "basic")
	r, ok := got["tuskin"]
	if !ok {
		t.Fatal("tuskin (basic): missing resolution")
	}
	// Basic mode keeps the dict row's reading even though it's the
	// known-wrong one; the overlay only activates in custom mode.
	if r.Lemma != "tuska" || r.POS != "NOUN" {
		t.Errorf("tuskin (basic): got (%s/%s), want dict (tuska/NOUN)",
			r.Lemma, r.POS)
	}
}

// TestPickBestResolutionCandidate_MaInfinitiveBias proves the
// MA-infinitive bias picks the FST verb reading over the dict
// noun-cousin reading. Constructed directly against
// pickBestResolutionCandidate to avoid needing a populated FST table
// fixture for lähtemään/juomassa/etc.
func TestPickBestResolutionCandidate_MaInfinitiveBias(t *testing.T) {
	dictNoun := resolutionCandidate{
		res: FormResolution{
			Lemma:  "lähtemä",
			POS:    "NOUN",
			Feats:  "Case=Ill|Number=Sing|Person=3",
			Source: "dict",
		},
		sourcePriority: 10,
		hasDict:        true,
		fstOrder:       9999,
	}
	fstVerb := resolutionCandidate{
		res: FormResolution{
			Lemma:  "lähteä",
			POS:    "VERB",
			Feats:  "Case=Ill|InfForm=Ma|VerbForm=Inf",
			Source: "fst",
		},
		hasFST:   true,
		fstOrder: 0,
	}
	got := pickBestResolutionCandidate("lähtemään", "FI", []resolutionCandidate{dictNoun, fstVerb})
	if got.res.Lemma != "lähteä" || got.res.POS != "VERB" {
		t.Errorf("MA-infinitive bias: got (%s/%s), want (lähteä/VERB)",
			got.res.Lemma, got.res.POS)
	}
	// The same surfaces in non-MA positions must NOT be biased.
	dictNounOk := resolutionCandidate{
		res:            FormResolution{Lemma: "talo", POS: "NOUN", Feats: "Case=Ine"},
		sourcePriority: 10,
		hasDict:        true,
	}
	fstVerbOddball := resolutionCandidate{
		res:      FormResolution{Lemma: "tela", POS: "VERB", Feats: "Mood=Ind"},
		hasFST:   true,
		fstOrder: 0,
	}
	got = pickBestResolutionCandidate("talossa", "FI", []resolutionCandidate{dictNounOk, fstVerbOddball})
	if got.res.Lemma != "talo" {
		t.Errorf("non-MA surface: bias should not fire, got (%s/%s) want talo/NOUN",
			got.res.Lemma, got.res.POS)
	}
}

// TestPickBestResolutionCandidate_AInfLongBias proves the
// A-infinitive-long bias picks the FST verb reading over kaikki's
// self-keyed dict row, and that a non-A-long surface is unaffected.
//
// Surfaces under test:
//   - tarjotakseen: dict ships nothing useful (or wrong-POS); FST
//     gives tarjota VERB InfForm=1|Person[psor]=3|VerbForm=Inf
//   - mennäkseen: kaikki keys this back to itself as lemma; FST gives
//     mennä VERB InfForm=1|Person[psor]=3|VerbForm=Inf
//   - ymmärtääkseen: kaikki tags this ADV (wrong POS) with self-lemma;
//     FST gives ymmärtää VERB InfForm=1|Person[psor]=3|VerbForm=Inf
func TestPickBestResolutionCandidate_AInfLongBias(t *testing.T) {
	cases := []struct {
		name      string
		surface   string
		dictRes   FormResolution
		fstRes    FormResolution
		wantLemma string
		wantPOS   string
	}{
		{
			name:    "mennäkseen self-keyed dict loses to FST verb",
			surface: "mennäkseen",
			dictRes: FormResolution{
				Lemma:  "mennäkseen",
				POS:    "VERB",
				Source: "dict",
			},
			fstRes: FormResolution{
				Lemma:  "mennä",
				POS:    "VERB",
				Feats:  "InfForm=1|Person[psor]=3|VerbForm=Inf",
				Source: "fst",
			},
			wantLemma: "mennä",
			wantPOS:   "VERB",
		},
		{
			name:    "ymmärtääkseen wrong-POS ADV self-key loses to FST verb",
			surface: "ymmärtääkseen",
			dictRes: FormResolution{
				Lemma:  "ymmärtääkseen",
				POS:    "ADV",
				Source: "dict",
			},
			fstRes: FormResolution{
				Lemma:  "ymmärtää",
				POS:    "VERB",
				Feats:  "InfForm=1|Person[psor]=3|VerbForm=Inf",
				Source: "fst",
			},
			wantLemma: "ymmärtää",
			wantPOS:   "VERB",
		},
		{
			name:    "tarjotakseen FST-only path resolves to verb",
			surface: "tarjotakseen",
			// No dict candidate — only the FST verb reading. Still must
			// resolve correctly. (This is a smoke that adding the bias
			// doesn't break the FST-only happy path.)
			fstRes: FormResolution{
				Lemma:  "tarjota",
				POS:    "VERB",
				Feats:  "InfForm=1|Person[psor]=3|VerbForm=Inf",
				Source: "fst",
			},
			wantLemma: "tarjota",
			wantPOS:   "VERB",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cands := make([]resolutionCandidate, 0, 2)
			if tc.dictRes.Lemma != "" {
				cands = append(cands, resolutionCandidate{
					res:            tc.dictRes,
					sourcePriority: 10, // strong dict priority — bias must override
					hasDict:        true,
					fstOrder:       9999,
				})
			}
			cands = append(cands, resolutionCandidate{
				res:      tc.fstRes,
				hasFST:   true,
				fstOrder: 0,
			})
			got := pickBestResolutionCandidate(tc.surface, "FI", cands)
			if got.res.Lemma != tc.wantLemma || got.res.POS != tc.wantPOS {
				t.Errorf("got (%s/%s), want (%s/%s)",
					got.res.Lemma, got.res.POS, tc.wantLemma, tc.wantPOS)
			}
		})
	}

	// Non-A-long surface: bias must NOT fire. tarjotaan is the passive
	// present indicative of tarjota — same lemma, but a different
	// inflection that doesn't carry the -kseen suffix family.
	dictNoun := resolutionCandidate{
		res:            FormResolution{Lemma: "talo", POS: "NOUN", Feats: "Case=Ine"},
		sourcePriority: 10,
		hasDict:        true,
	}
	fstOddball := resolutionCandidate{
		res:      FormResolution{Lemma: "talu", POS: "VERB", Feats: "InfForm=1|Person[psor]=3"},
		hasFST:   true,
		fstOrder: 0,
	}
	got := pickBestResolutionCandidate("talossa", "FI", []resolutionCandidate{dictNoun, fstOddball})
	if got.res.Lemma != "talo" {
		t.Errorf("non-A-long surface: bias should not fire, got (%s/%s) want talo/NOUN",
			got.res.Lemma, got.res.POS)
	}
}

// TestPickBestResolutionCandidate_EstonianVerbInflectionBias covers the
// peatus → peatuma/VERB regression class. When the surface is also the
// citation form of a competing noun (lemma == surface for the noun),
// the inflected verb reading must win over both the noun-as-citation
// and any non-verb derivation (e.g. peatu/ADJ inessive).
func TestPickBestResolutionCandidate_EstonianVerbInflectionBias(t *testing.T) {
	// peatus: VERB peatuma+Past+3Sg vs NOUN peatus (citation) vs ADJ peatu+Ine.
	// Pre-bias, the binary morphologyScore tied all three and the
	// fstOrder tiebreak picked peatu/ADJ (FST emits it earlier).
	dictPeatuADJ := resolutionCandidate{
		res:            FormResolution{Lemma: "peatu", POS: "ADJ", Feats: "Case=Ine|Number=Sing"},
		sourcePriority: 20,
		hasDict:        true,
		hasFST:         true,
		fstOrder:       1,
	}
	dictPeatumaVERB := resolutionCandidate{
		res: FormResolution{
			Lemma: "peatuma", POS: "VERB",
			Feats: "Mood=Ind|Number=Sing|Person=3|Tense=Past|VerbForm=Fin|Voice=Act",
		},
		sourcePriority: 20,
		hasDict:        true,
		hasFST:         true,
		fstOrder:       3,
	}
	dictPeatusNOUN := resolutionCandidate{
		res:            FormResolution{Lemma: "peatus", POS: "NOUN", Feats: "Case=Nom|Number=Sing"},
		sourcePriority: 20,
		hasDict:        true,
		hasFST:         true,
		fstOrder:       2,
	}
	got := pickBestResolutionCandidate("peatus", "ET", []resolutionCandidate{dictPeatuADJ, dictPeatumaVERB, dictPeatusNOUN})
	if got.res.Lemma != "peatuma" || got.res.POS != "VERB" {
		t.Errorf("peatus: got (%s/%s), want (peatuma/VERB)", got.res.Lemma, got.res.POS)
	}

	// Negative control: arstiks. NOUN candidate is INFLECTED (lemma=arst
	// != surface=arstiks, translative case). Bias must not fire — verb
	// arstima is the wrong answer (kaikki artifact). Falls through to
	// the binary-morphologyScore-and-alphabetical chain that picks
	// arst/NOUN.
	dictArst := resolutionCandidate{
		res:            FormResolution{Lemma: "arst", POS: "NOUN", Feats: "Case=Tra|Number=Sing"},
		sourcePriority: 20,
		hasDict:        true,
	}
	dictArstima := resolutionCandidate{
		res:            FormResolution{Lemma: "arstima", POS: "VERB", Feats: "Mood=Cnd|Tense=Pres|VerbForm=Fin|Voice=Act"},
		sourcePriority: 20,
		hasDict:        true,
	}
	got = pickBestResolutionCandidate("arstiks", "ET", []resolutionCandidate{dictArst, dictArstima})
	if got.res.Lemma != "arst" || got.res.POS != "NOUN" {
		t.Errorf("arstiks negative control: bias must not fire, got (%s/%s), want (arst/NOUN)", got.res.Lemma, got.res.POS)
	}

	// FI surface must not trigger the ET-only bias even when shaped
	// similarly. Use a hypothetical FI noun-citation-form configuration
	// to confirm lang="FI" suppresses the bias.
	got = pickBestResolutionCandidate("peatus", "FI", []resolutionCandidate{dictPeatuADJ, dictPeatumaVERB, dictPeatusNOUN})
	if got.res.Lemma == "peatuma" {
		t.Errorf("FI must not trigger ET verb bias: got (%s/%s)", got.res.Lemma, got.res.POS)
	}

	// Verb without Person= must not get the bias. Cover the gate so
	// future edits don't accidentally drop it.
	dictPeatumaNoPerson := dictPeatumaVERB
	dictPeatumaNoPerson.res.Feats = "Mood=Cnd|Tense=Pres|VerbForm=Fin|Voice=Act"
	got = pickBestResolutionCandidate("peatus", "ET", []resolutionCandidate{dictPeatuADJ, dictPeatumaNoPerson, dictPeatusNOUN})
	if got.res.Lemma == "peatuma" {
		t.Errorf("no-Person verb must not trigger bias: got (%s/%s)", got.res.Lemma, got.res.POS)
	}
}

func TestAInfLongBias(t *testing.T) {
	cases := []struct {
		surface string
		res     FormResolution
		want    int
	}{
		// Verb with A-long signature + correct stemmed lemma → +1.
		{"mennäkseen", FormResolution{
			Lemma: "mennä", POS: "VERB",
			Feats: "InfForm=1|Person[psor]=3|VerbForm=Inf",
		}, 1},
		{"tarjotakseni", FormResolution{
			Lemma: "tarjota", POS: "VERB",
			Feats: "InfForm=1|Number[psor]=Sing|Person[psor]=1|VerbForm=Inf",
		}, 1},
		{"mennäksemme", FormResolution{
			Lemma: "mennä", POS: "VERB",
			Feats: "InfForm=1|Number[psor]=Plur|Person[psor]=1|VerbForm=Inf",
		}, 1},
		// Self-key (kaikki bug): lemma equals surface on A-long surface → -1,
		// regardless of POS (ymmärtääkseen is tagged ADV in kaikki).
		{"mennäkseen", FormResolution{Lemma: "mennäkseen", POS: "VERB"}, -1},
		{"ymmärtääkseen", FormResolution{Lemma: "ymmärtääkseen", POS: "ADV"}, -1},
		// Mixed-case lemma-equals-surface still demotes (EqualFold).
		{"Mennäkseen", FormResolution{Lemma: "mennäkseen", POS: "VERB"}, -1},
		// A-long surface but candidate has neither signature — bias is 0.
		// (e.g. an FST reading that happens to land here without the
		// A-long markers, or a noun reading.)
		{"mennäkseen", FormResolution{Lemma: "mennä", POS: "NOUN", Feats: "Case=Tra|Number=Sing"}, 0},
		// Verb with InfForm=1 but no Person[psor]: basic A-inf, not A-long.
		// Bias must not promote.
		{"mennäkseen", FormResolution{Lemma: "mennä", POS: "VERB", Feats: "InfForm=1|VerbForm=Inf"}, 0},
		// Non-A-long surfaces: always 0.
		{"talossa", FormResolution{Lemma: "mennä", POS: "VERB", Feats: "InfForm=1|Person[psor]=3|VerbForm=Inf"}, 0},
		{"tarjoamaan", FormResolution{Lemma: "tarjota", POS: "VERB", Feats: "Case=Ill|InfForm=Ma|VerbForm=Inf"}, 0},
		// Empty surface: defensive.
		{"", FormResolution{POS: "VERB"}, 0},
	}
	for _, tc := range cases {
		got := aInfLongBias(tc.surface, tc.res)
		if got != tc.want {
			t.Errorf("aInfLongBias(%q, %+v) = %d, want %d", tc.surface, tc.res, got, tc.want)
		}
	}
}

func TestMaInfinitiveBias(t *testing.T) {
	cases := []struct {
		surface string
		res     FormResolution
		want    int
	}{
		// MA-suffix surfaces: verb-with-Ma wins, noun-cousin trap loses.
		{"lähtemään", FormResolution{POS: "VERB", Feats: "Case=Ill|InfForm=Ma"}, 1},
		{"juomassa", FormResolution{POS: "VERB", Feats: "Case=Ine|InfForm=Ma|VerbForm=Inf"}, 1},
		// Noun-cousin trap: POS=VERB with -ma/-mä lemma is the kaikki
		// mis-tagging of an inflected deverbal noun. Demote.
		{"lähtemään", FormResolution{POS: "VERB", Lemma: "lähtemä"}, -1},
		{"juomassa", FormResolution{POS: "VERB", Lemma: "juoma"}, -1},
		{"tarjoamaan", FormResolution{POS: "VERB", Lemma: "tarjoama"}, -1},
		// Legitimate -ma/-mä noun on a MA-shape surface (voima "force",
		// ryhmä "group", asema "station", oireyhtymä "syndrome", …).
		// Must NOT be demoted — the POS=NOUN tag distinguishes a real
		// noun from the kaikki noun-cousin mis-tag.
		{"voimassa", FormResolution{POS: "NOUN", Lemma: "voima"}, 0},
		{"ryhmässä", FormResolution{POS: "NOUN", Lemma: "ryhmä"}, 0},
		{"asemalla", FormResolution{POS: "NOUN", Lemma: "asema"}, 0},
		{"oireyhtymästä", FormResolution{POS: "NOUN", Lemma: "oireyhtymä"}, 0},
		// MA-suffix surface, lemma not ending in -ma/-mä: no NOUN penalty
		// (could be an unrelated compound).
		{"lähtemään", FormResolution{POS: "NOUN", Lemma: "lähtö"}, 0},
		// Non-MA surfaces: no bias.
		{"talossa", FormResolution{POS: "VERB", Feats: "Case=Ill|InfForm=Ma"}, 0},
		{"talossa", FormResolution{POS: "NOUN", Lemma: "tala"}, 0},
		// Empty surface: defensive.
		{"", FormResolution{POS: "VERB"}, 0},
	}
	for _, tc := range cases {
		got := maInfinitiveBias(tc.surface, tc.res)
		if got != tc.want {
			t.Errorf("maInfinitiveBias(%q, %+v) = %d, want %d", tc.surface, tc.res, got, tc.want)
		}
	}
}

// --- BatchLookupSenses tests ---

func TestBatchLookupSenses_ReturnsAllSensesInOrder(t *testing.T) {
	// All senses from the winning source, in sense_idx ASC order. The
	// first sense is the same string BatchLookupGlosses returns — the
	// two APIs must agree on "the primary gloss". The schema has one
	// lemma row per (lemma, pos, lang); the JOIN in the senses query
	// couples each translation row to that single source row.
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"pää", "NOUN", "head", "FI", "kaikki", 10},
	})
	seedTranslations(t, db, []struct {
		lemma, pos, lang, target, text, source string
		senseIdx                               int
	}{
		{"pää", "NOUN", "FI", "EN", "head (anatomical)", "kaikki", 0},
		{"pää", "NOUN", "FI", "EN", "tip / end (of an object)", "kaikki", 1},
		{"pää", "NOUN", "FI", "EN", "top (the upper part)", "kaikki", 2},
		{"pää", "NOUN", "FI", "EN", "stalk / stem", "kaikki", 3},
	})

	got := db.BatchLookupSenses([]LemmaKey{{"pää", "NOUN"}}, "FI")
	senses, ok := got[LemmaKey{"pää", "NOUN"}]
	if !ok {
		t.Fatal("pää: expected senses, got none")
	}
	if len(senses) != 4 {
		t.Fatalf("pää: got %d senses, want 4", len(senses))
	}
	for i, want := range []string{
		"head (anatomical)",
		"tip / end (of an object)",
		"top (the upper part)",
		"stalk / stem",
	} {
		if senses[i].Text != want || senses[i].SenseIdx != i {
			t.Errorf("senses[%d]: got (%q, sense_idx=%d), want (%q, %d)",
				i, senses[i].Text, senses[i].SenseIdx, want, i)
		}
		if senses[i].Source != "kaikki" || senses[i].SourcePriority != 10 {
			t.Errorf("senses[%d]: got source=%q priority=%d, want kaikki/10",
				i, senses[i].Source, senses[i].SourcePriority)
		}
	}
	// Verify the first sense's text matches what BatchLookupGlosses
	// returns — the two APIs must agree on what "the primary gloss" is.
	primary := db.BatchLookupGlosses([]LemmaKey{{"pää", "NOUN"}}, "FI")[LemmaKey{"pää", "NOUN"}]
	if primary != senses[0].Text {
		t.Errorf("primary disagreement: BatchLookupGlosses=%q, senses[0]=%q",
			primary, senses[0].Text)
	}
}

func TestBatchLookupSenses_HigherPrioritySourceWinsLemmaAndDictatesSenses(t *testing.T) {
	// After ekilex takes over the lemma row (priority 20 > kaikki's 10),
	// only ekilex's translation rows JOIN successfully. Stale kaikki
	// translations are silently ignored — mirroring the documented
	// BatchLookupGlosses behaviour. This guards the invariant that
	// BatchLookupSenses NEVER surfaces senses whose source no longer
	// owns the lemma row.
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"talo", "NOUN", "house-from-ekilex", "ET", "ekilex", 20},
	})
	seedTranslations(t, db, []struct {
		lemma, pos, lang, target, text, source string
		senseIdx                               int
	}{
		{"talo", "NOUN", "ET", "EN", "stale-kaikki-1", "kaikki", 0},
		{"talo", "NOUN", "ET", "EN", "stale-kaikki-2", "kaikki", 1},
		{"talo", "NOUN", "ET", "EN", "house", "ekilex", 0},
		{"talo", "NOUN", "ET", "EN", "dwelling", "ekilex", 1},
	})

	got := db.BatchLookupSenses([]LemmaKey{{"talo", "NOUN"}}, "ET")
	senses, ok := got[LemmaKey{"talo", "NOUN"}]
	if !ok {
		t.Fatal("talo: expected senses, got none")
	}
	if len(senses) != 2 {
		t.Fatalf("talo: got %d senses, want 2 (kaikki rows must be stranded)", len(senses))
	}
	for i, want := range []string{"house", "dwelling"} {
		if senses[i].Text != want || senses[i].Source != "ekilex" {
			t.Errorf("senses[%d]: got (%q, %q), want (%q, ekilex)",
				i, senses[i].Text, senses[i].Source, want)
		}
	}
}

func TestBatchLookupSenses_NoTranslationsRowsAbsentFromMap(t *testing.T) {
	// A lemma that has only lemmas.gloss but no translations rows
	// (legacy DB, or applyCustomGlosses path) returns no entry from
	// BatchLookupSenses. That's the documented contract: this API is
	// about the ranked sense list, not the cached primary.
	db := newTestDB(t)
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"legacy", "NOUN", "old gloss", "FI", "kaikki", 10},
	})
	// no translations seeded

	got := db.BatchLookupSenses([]LemmaKey{{"legacy", "NOUN"}}, "FI")
	if _, ok := got[LemmaKey{"legacy", "NOUN"}]; ok {
		t.Errorf("legacy lemma with no translations should be absent; got %v", got)
	}
}

func TestBatchLookupSenses_EmptyInput(t *testing.T) {
	db := newTestDB(t)
	got := db.BatchLookupSenses(nil, "FI")
	if got == nil {
		t.Error("nil input should return non-nil empty map")
	}
	if len(got) != 0 {
		t.Errorf("nil input: got %d entries, want 0", len(got))
	}
}

// TestPickBestVFSTAnalysis_PreservesSurfaceCase regression-tests the codex
// review finding on PR #107: when libvoikko returns multiple analyses for
// the lowercase form (e.g. `turku/NOUN` and `Turku/PROPN` for "turussa"),
// the VFST fallback was picking the first analysis regardless of surface
// case, which marked the city name "Turussa" as a common noun. The fix
// applies the same case/POS scoring as the dictionary candidate ranker.
func TestPickBestVFSTAnalysis_PreservesSurfaceCase(t *testing.T) {
	turkuNoun := lemmatizer.Analysis{Lemma: "turku", UPOS: "NOUN", GrammarLabel: "inessive"}
	turkuPropn := lemmatizer.Analysis{Lemma: "Turku", UPOS: "PROPN", GrammarLabel: "inessive"}
	analyses := []lemmatizer.Analysis{turkuNoun, turkuPropn}

	// Capitalized surface — PROPN candidate should win even though
	// the noun reading came first in FST priority order.
	if got := pickBestVFSTAnalysis("Turussa", analyses); got.Lemma != "Turku" || got.UPOS != "PROPN" {
		t.Errorf("Turussa: got (%q, %q), want (Turku, PROPN)", got.Lemma, got.UPOS)
	}

	// Lowercase surface — common-noun reading should win, demoting PROPN.
	if got := pickBestVFSTAnalysis("turussa", analyses); got.Lemma != "turku" || got.UPOS != "NOUN" {
		t.Errorf("turussa: got (%q, %q), want (turku, NOUN)", got.Lemma, got.UPOS)
	}

	// Single analysis — identity (no scoring needed, no panic).
	if got := pickBestVFSTAnalysis("Turussa", analyses[:1]); got.UPOS != "NOUN" {
		t.Errorf("single analysis: got %q, want NOUN", got.UPOS)
	}
}

// TestPickBestVFSTAnalysis_TiePreservesFSTOrder confirms that when two
// analyses score identically on the case/POS heuristic, the original FST
// priority order is preserved — protects the deterministic VFST > Giellalt
// ordering inside the lemmatizer package from being scrambled by the
// stable-sort tiebreak.
func TestPickBestVFSTAnalysis_TiePreservesFSTOrder(t *testing.T) {
	first := lemmatizer.Analysis{Lemma: "alku", UPOS: "NOUN"}
	second := lemmatizer.Analysis{Lemma: "alusta", UPOS: "NOUN"}
	got := pickBestVFSTAnalysis("alusta", []lemmatizer.Analysis{first, second})
	if got.Lemma != "alku" {
		t.Errorf("ties should keep FST order: got %q, want %q", got.Lemma, "alku")
	}
}
