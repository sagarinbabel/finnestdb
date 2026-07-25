package main

import (
	"database/sql"
	"io"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newTestDB returns an in-memory SQLite DB with the importer schema applied.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := ensureSchema(db); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func ParseSanalista(r io.Reader) ([]kotusEntry, int, error) {
	var out []kotusEntry
	skipped, err := streamSanalista(r, func(e kotusEntry) error {
		out = append(out, e)
		return nil
	})
	return out, skipped, err
}

func writeKotusEntries(db *sql.DB, entries []kotusEntry) (importStats, error) {
	stats := importStats{}

	tx, err := db.Begin()
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	w, err := newKotusWriter(tx, &stats)
	if err != nil {
		return stats, err
	}
	defer w.Close()

	for _, e := range entries {
		if err := w.write(e); err != nil {
			return stats, err
		}
	}

	return stats, tx.Commit()
}

// --- ParadigmClass formatting ---

func TestParadigmClass_NoGradation(t *testing.T) {
	e := kotusEntry{InflectionType: "38"}
	if got := e.ParadigmClass(); got != "38" {
		t.Errorf("got %q, want %q", got, "38")
	}
}

func TestParadigmClass_WithGradation(t *testing.T) {
	e := kotusEntry{InflectionType: "1", Gradation: "I"}
	if got := e.ParadigmClass(); got != "1-I" {
		t.Errorf("got %q, want %q", got, "1-I")
	}
}

func TestParadigmClass_EmptyTNReturnsEmpty(t *testing.T) {
	e := kotusEntry{Gradation: "I"}
	if got := e.ParadigmClass(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// --- parseTaivutus ---

func TestParseTaivutus(t *testing.T) {
	cases := []struct {
		in, wantTN, wantGR string
	}{
		{"38", "38", ""},
		{"1*I", "1", "I"},
		{"41*A", "41", "A"},
		{"4*A", "4", "A"},
		{"75*I", "75", "I"},
		{"32*C", "32", "C"},
		{"99", "99", ""},
		{"", "", ""},
		{"  38  ", "38", ""}, // whitespace gets trimmed
	}
	for _, c := range cases {
		gotTN, gotGR := parseTaivutus(c.in)
		if gotTN != c.wantTN || gotGR != c.wantGR {
			t.Errorf("parseTaivutus(%q) = (%q, %q), want (%q, %q)",
				c.in, gotTN, gotGR, c.wantTN, c.wantGR)
		}
	}
}

// --- mapWordClass ---

func TestMapWordClass(t *testing.T) {
	cases := []struct {
		in     string
		wantUP string
		wantOK bool
	}{
		{"substantiivi", "NOUN", true},
		{"adjektiivi", "ADJ", true},
		{"verbi", "VERB", true},
		{"adverbi", "ADV", true},
		{"interjektio", "INTJ", true},
		{"pronomini", "PRON", true},
		{"numeraali", "NUM", true},
		{"järjestysluku", "NUM", true},
		{"konjunktio", "CCONJ", true},
		{"rinnastuskonjunktio", "CCONJ", true},
		{"alistuskonjunktio", "SCONJ", true},
		{"postpositio", "ADP", true},
		{"prepositio", "ADP", true},
		{"kieltoverbi", "AUX", true},
		{"partikkeli", "PART", true},
		// Plus-separated takes the leading class
		{"adverbi + kieltoverbi", "ADV", true},
		{"verbi + interjektio", "VERB", true},
		// Whitespace tolerated
		{"  substantiivi  ", "NOUN", true},
		// Unknown labels return ("X", false)
		{"unknown", "X", false},
		{"", "X", false},
	}
	for _, c := range cases {
		gotUP, gotOK := mapWordClass(c.in)
		if gotUP != c.wantUP || gotOK != c.wantOK {
			t.Errorf("mapWordClass(%q) = (%q, %v), want (%q, %v)",
				c.in, gotUP, gotOK, c.wantUP, c.wantOK)
		}
	}
}

// --- ParseSanalista (TSV) ---

func TestParseSanalista_BasicEntries(t *testing.T) {
	tsv := "Hakusana\tHomonymia\tSanaluokka\tTaivutustiedot\n" +
		"aakkonen\t\tsubstantiivi\t38\n" +
		"aalto\t\tsubstantiivi\t1*I\n" +
		"aallottaa\t\tverbi\t53*C\n"

	entries, skipped, err := ParseSanalista(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}

	want := []struct {
		head, hom, tn, gr string
		classes           []string
	}{
		{"aakkonen", "", "38", "", []string{"substantiivi"}},
		{"aalto", "", "1", "I", []string{"substantiivi"}},
		{"aallottaa", "", "53", "C", []string{"verbi"}},
	}
	for i, w := range want {
		got := entries[i]
		if got.Headword != w.head || got.Homonym != w.hom ||
			got.InflectionType != w.tn || got.Gradation != w.gr ||
			len(got.WordClasses) != len(w.classes) || got.WordClasses[0] != w.classes[0] {
			t.Errorf("entry[%d] = %+v, want head=%q hom=%q tn=%q gr=%q classes=%v",
				i, got, w.head, w.hom, w.tn, w.gr, w.classes)
		}
	}
}

func TestParseSanalista_SkipsMalformedRows(t *testing.T) {
	// Rows with too few columns or empty headword get counted as skipped.
	tsv := "Hakusana\tHomonymia\tSanaluokka\tTaivutustiedot\n" +
		"valo\t\tsubstantiivi\t1\n" +
		"\t\tsubstantiivi\t1\n" + // empty headword
		"too-few\tcolumns\n" + // < 4 columns
		"sanoa\t\tverbi\t52\n"

	entries, skipped, err := ParseSanalista(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if skipped != 2 {
		t.Errorf("skipped = %d, want 2", skipped)
	}
	if len(entries) != 2 || entries[0].Headword != "valo" || entries[1].Headword != "sanoa" {
		t.Errorf("entries = %+v, want [valo, sanoa]", entries)
	}
}

func TestParseSanalista_MultiClassSanaluokka(t *testing.T) {
	// "sininen" is both adjective and noun in Kotus.
	tsv := "Hakusana\tHomonymia\tSanaluokka\tTaivutustiedot\n" +
		"sininen\t\tadjektiivi, substantiivi\t38\n"

	entries, _, err := ParseSanalista(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	got := entries[0].WordClasses
	want := []string{"adjektiivi", "substantiivi"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("classes = %v, want %v", got, want)
	}
}

func TestParseSanalista_EmptyTaivutusPreservedAsEmpty(t *testing.T) {
	// Compound nouns like "aaltoallas" have no inflection class in
	// Kotus. The parser must still emit the entry - the writer skips
	// the paradigm-class write but still inserts the dictionary row.
	tsv := "Hakusana\tHomonymia\tSanaluokka\tTaivutustiedot\n" +
		"aaltoallas\t\tsubstantiivi\t\n"

	entries, _, err := ParseSanalista(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 1 || entries[0].Headword != "aaltoallas" || entries[0].InflectionType != "" {
		t.Errorf("entries = %+v", entries)
	}
}

func TestParseSanalista_HomonymPreserved(t *testing.T) {
	tsv := "Hakusana\tHomonymia\tSanaluokka\tTaivutustiedot\n" +
		"kuusi\t1\tsubstantiivi\t27\n" +
		"kuusi\t2\tnumeraali\t27\n"

	entries, _, err := ParseSanalista(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].Homonym != "1" || entries[1].Homonym != "2" {
		t.Errorf("homonyms = %q, %q; want 1, 2", entries[0].Homonym, entries[1].Homonym)
	}
}

// --- writeKotusEntries: conflict policy ---

func TestWriteKotusEntries_UpgradesExistingKaikkiLemmaInPlace(t *testing.T) {
	// Headline policy: when kaikki imported a lemma, Kotus fills
	// paradigm_class but leaves source/source_priority/gloss alone.
	db := newTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('valo', 'NOUN', 'light, lamp', 'FI', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "valo", WordClasses: []string{"substantiivi"}, InflectionType: "1"},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.ParadigmsUpgraded != 1 {
		t.Errorf("paradigms_upgraded = %d, want 1", stats.ParadigmsUpgraded)
	}
	if stats.NewLemmasInserted != 0 {
		t.Errorf("new_lemmas_inserted = %d, want 0", stats.NewLemmasInserted)
	}

	var gloss, source, paradigm string
	var priority int
	if err := db.QueryRow(
		`SELECT gloss, source, source_priority, paradigm_class
		 FROM lemmas WHERE lemma='valo' AND pos='NOUN' AND lang='FI'`,
	).Scan(&gloss, &source, &priority, &paradigm); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gloss != "light, lamp" {
		t.Errorf("gloss preserved: got %q, want %q", gloss, "light, lamp")
	}
	if source != "kaikki" || priority != 10 {
		t.Errorf("source preserved: got %q/%d, want kaikki/10", source, priority)
	}
	if paradigm != "1" {
		t.Errorf("paradigm_class: got %q, want %q", paradigm, "1")
	}
}

func TestWriteKotusEntries_UpgradesAllExistingPOSForLemma(t *testing.T) {
	// kaikki sometimes has the same headword at multiple POS (sininen
	// ADJ + NOUN). Kotus's paradigm_class applies to all.
	db := newTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('sininen', 'ADJ', 'blue', 'FI', 'kaikki', 10),
		        ('sininen', 'NOUN', 'a blue thing', 'FI', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "sininen", WordClasses: []string{"adjektiivi", "substantiivi"}, InflectionType: "38"},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.ParadigmsUpgraded != 2 {
		t.Errorf("paradigms_upgraded = %d, want 2", stats.ParadigmsUpgraded)
	}
	// Regression guard from PR #92 review: one entry → one EntriesProcessed.
	if stats.EntriesProcessed != 1 {
		t.Errorf("entries_processed = %d, want 1 (one row, multiple POS effects)", stats.EntriesProcessed)
	}

	for _, pos := range []string{"ADJ", "NOUN"} {
		var paradigm string
		if err := db.QueryRow(
			`SELECT paradigm_class FROM lemmas WHERE lemma='sininen' AND pos=? AND lang='FI'`, pos,
		).Scan(&paradigm); err != nil {
			t.Fatalf("query %s: %v", pos, err)
		}
		if paradigm != "38" {
			t.Errorf("%s paradigm_class: got %q, want %q", pos, paradigm, "38")
		}
	}
}

func TestWriteKotusEntries_InsertsNewLemmasOnePerWordClass(t *testing.T) {
	// Kotus-only multi-class headword: insert one row per recognised UPOS.
	db := newTestDB(t)

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "uusi-uudempi", WordClasses: []string{"adjektiivi", "substantiivi"}, InflectionType: "16"},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.NewLemmasInserted != 2 {
		t.Errorf("new_lemmas_inserted = %d, want 2 (ADJ + NOUN)", stats.NewLemmasInserted)
	}

	for _, pos := range []string{"ADJ", "NOUN"} {
		var source, paradigm string
		var priority int
		var gloss sql.NullString
		if err := db.QueryRow(
			`SELECT source, source_priority, paradigm_class, gloss
			 FROM lemmas WHERE lemma='uusi-uudempi' AND pos=? AND lang='FI'`, pos,
		).Scan(&source, &priority, &paradigm, &gloss); err != nil {
			t.Fatalf("query %s: %v", pos, err)
		}
		if source != "kotus" || priority != 10 {
			t.Errorf("%s source/priority: got %q/%d, want kotus/10", pos, source, priority)
		}
		if paradigm != "16" {
			t.Errorf("%s paradigm: got %q, want %q", pos, paradigm, "16")
		}
		if gloss.Valid {
			t.Errorf("%s gloss: got %q, want NULL", pos, gloss.String)
		}
	}
}

func TestWriteKotusEntries_EmptyTaivutusInsertsRowWithNullParadigm(t *testing.T) {
	// Compound nouns like "aaltoallas" have no Kotus inflection class.
	// The dictionary row still goes in (so kaikki+ekilex don't lose
	// the headword), but paradigm_class stays NULL.
	db := newTestDB(t)

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "aaltoallas", WordClasses: []string{"substantiivi"}},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.NewLemmasInserted != 1 {
		t.Errorf("new_lemmas_inserted = %d, want 1", stats.NewLemmasInserted)
	}
	if stats.EntriesWithoutParadigm != 1 {
		t.Errorf("entries_without_paradigm = %d, want 1", stats.EntriesWithoutParadigm)
	}

	var paradigm sql.NullString
	if err := db.QueryRow(
		`SELECT paradigm_class FROM lemmas WHERE lemma='aaltoallas' AND lang='FI'`,
	).Scan(&paradigm); err != nil {
		t.Fatalf("query: %v", err)
	}
	if paradigm.Valid {
		t.Errorf("paradigm_class: got %q, want NULL", paradigm.String)
	}
}

func TestWriteKotusEntries_UnknownClassRecordsButDoesNotInsert(t *testing.T) {
	// If every Sanaluokka label is unknown, count it but don't pollute
	// the dictionary with a row tagged "X".
	db := newTestDB(t)

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "weird-word", WordClasses: []string{"unknown-class"}, InflectionType: "1"},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.HeadwordsWithoutClass != 1 {
		t.Errorf("headwords_without_class = %d, want 1", stats.HeadwordsWithoutClass)
	}
	if stats.NewLemmasInserted != 0 {
		t.Errorf("new_lemmas_inserted = %d, want 0", stats.NewLemmasInserted)
	}

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM lemmas WHERE lemma='weird-word'`,
	).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Errorf("lemma count: got %d, want 0", n)
	}
}

func TestWriteKotusEntries_GradationProducesCompoundClass(t *testing.T) {
	db := newTestDB(t)
	if _, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "aalto", WordClasses: []string{"substantiivi"}, InflectionType: "1", Gradation: "I"},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	var paradigm string
	if err := db.QueryRow(
		`SELECT paradigm_class FROM lemmas WHERE lemma='aalto' AND lang='FI'`,
	).Scan(&paradigm); err != nil {
		t.Fatalf("query: %v", err)
	}
	if paradigm != "1-I" {
		t.Errorf("paradigm_class: got %q, want %q (tn-gradation)", paradigm, "1-I")
	}
}

// --- streamSanalista abort ---

func TestStreamSanalista_CallbackErrorAborts(t *testing.T) {
	tsv := "Hakusana\tHomonymia\tSanaluokka\tTaivutustiedot\n" +
		"a\t\tsubstantiivi\t1\n" +
		"b\t\tsubstantiivi\t2\n" +
		"c\t\tsubstantiivi\t3\n"

	seen := 0
	want := errFromString("stop")
	skipped, err := streamSanalista(strings.NewReader(tsv), func(e kotusEntry) error {
		seen++
		if seen == 2 {
			return want
		}
		return nil
	})
	if err != want {
		t.Errorf("err = %v, want %v", err, want)
	}
	if seen != 2 {
		t.Errorf("callback called %d times, want 2", seen)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
}

type errFromString string

func (e errFromString) Error() string { return string(e) }

// --- end-to-end Import ---

func TestImport_EndToEnd(t *testing.T) {
	db := newTestDB(t)

	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('valo', 'NOUN', 'light', 'FI', 'kaikki', 10),
		        ('sanoa', 'VERB', 'to say', 'FI', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tsv := "Hakusana\tHomonymia\tSanaluokka\tTaivutustiedot\n" +
		"valo\t\tsubstantiivi\t1\n" +
		"sanoa\t\tverbi\t52\n" +
		"uusi-uudempi\t\tadjektiivi\t16\n" +
		"compound-noun\t\tsubstantiivi\t\n" // no paradigm

	stats, err := Import(db, strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if stats.EntriesProcessed != 4 {
		t.Errorf("processed = %d, want 4", stats.EntriesProcessed)
	}
	if stats.ParadigmsUpgraded != 2 {
		t.Errorf("upgraded = %d, want 2 (valo + sanoa)", stats.ParadigmsUpgraded)
	}
	if stats.NewLemmasInserted != 2 {
		t.Errorf("new = %d, want 2 (uusi-uudempi + compound-noun)", stats.NewLemmasInserted)
	}
	if stats.EntriesWithoutParadigm != 1 {
		t.Errorf("no-paradigm = %d, want 1 (compound-noun)", stats.EntriesWithoutParadigm)
	}
}

// --- Regression tests from PR #98 review ---

func TestParseTaivutus_MultiValuedAlternatives(t *testing.T) {
	// Real patterns from the 2024 distribution. 152 rows have
	// comma-separated alternatives. Take the primary (first entry,
	// parens stripped); drop alts. Phase 4 consumes paradigm_class as
	// a single key.
	cases := []struct {
		in, wantTN, wantGR string
	}{
		{"73, (77)", "73", ""},        // avata: primary 73, alt (77)
		{"72*D, 74*D", "72", "D"},     // aueta: primary 72*D, alt 74*D
		{"9, 50", "9", ""},            // ahkeraliisa: primary 9, alt 50
		{"7*E, (5)", "7", "E"},        // alpi: primary 7*E, alt (5)
		{"(9)", "9", ""},              // a cappella: parens-only sole class
		{"(1)", "1", ""},              // adagio: parens-only
		{"  73  ,  (77)  ", "73", ""}, // whitespace tolerated
	}
	for _, c := range cases {
		gotTN, gotGR := parseTaivutus(c.in)
		if gotTN != c.wantTN || gotGR != c.wantGR {
			t.Errorf("parseTaivutus(%q) = (%q, %q), want (%q, %q)",
				c.in, gotTN, gotGR, c.wantTN, c.wantGR)
		}
	}
}

func TestWriteKotusEntries_ScopesUpdatesByCurrentRowsWordClasses(t *testing.T) {
	// Codex P1 + sagarinbabel P1: an "update every existing POS"
	// approach corrupts adjacent rows when the TSV has the same
	// headword on multiple rows with different Sanaluokka.
	//
	// Real example from the 2024 dump:
	//   aika  1  substantiivi               9*D
	//   aika  2  adjektiivi, adverbi        99
	//
	// Pre-imports row 1 first (NOUN with class 9-D). When row 2 hits,
	// the writer must NOT touch the NOUN row's paradigm - it must
	// insert ADJ + ADV at class 99 instead.
	db := newTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority, paradigm_class)
		 VALUES ('aika', 'NOUN', 'time', 'FI', 'kaikki', 10, '9-D')`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "aika", WordClasses: []string{"adjektiivi", "adverbi"}, InflectionType: "99"},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.NewLemmasInserted != 2 {
		t.Errorf("new = %d, want 2 (ADJ + ADV)", stats.NewLemmasInserted)
	}
	if stats.ParadigmsUpgraded != 0 {
		t.Errorf("upgraded = %d, want 0 (NOUN must not be touched by an ADJ/ADV row)", stats.ParadigmsUpgraded)
	}

	// NOUN row preserved.
	var nounParadigm string
	if err := db.QueryRow(
		`SELECT paradigm_class FROM lemmas WHERE lemma='aika' AND pos='NOUN' AND lang='FI'`,
	).Scan(&nounParadigm); err != nil {
		t.Fatalf("query NOUN: %v", err)
	}
	if nounParadigm != "9-D" {
		t.Errorf("NOUN paradigm preserved: got %q, want %q", nounParadigm, "9-D")
	}

	// ADJ + ADV inserted at 99.
	for _, pos := range []string{"ADJ", "ADV"} {
		var paradigm string
		if err := db.QueryRow(
			`SELECT paradigm_class FROM lemmas WHERE lemma='aika' AND pos=? AND lang='FI'`, pos,
		).Scan(&paradigm); err != nil {
			t.Fatalf("query %s: %v", pos, err)
		}
		if paradigm != "99" {
			t.Errorf("%s paradigm: got %q, want %q", pos, paradigm, "99")
		}
	}
}

func TestWriteKotusEntries_KuurataInsertsBothNounAndVerb(t *testing.T) {
	// Real homonym: kuurata appears as substantiivi (no class) then
	// verbi (class 73). Processing in TSV order must insert NOUN
	// (with NULL paradigm) AND VERB (with class 73) - the writer
	// must not let the second row's "exists check" find the unrelated
	// NOUN and skip the VERB insert.
	db := newTestDB(t)

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "kuurata", WordClasses: []string{"substantiivi"}}, // empty Taivutus
		{Headword: "kuurata", WordClasses: []string{"verbi"}, InflectionType: "73"},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.NewLemmasInserted != 2 {
		t.Errorf("new = %d, want 2 (NOUN + VERB)", stats.NewLemmasInserted)
	}

	var nounParadigm sql.NullString
	if err := db.QueryRow(
		`SELECT paradigm_class FROM lemmas WHERE lemma='kuurata' AND pos='NOUN' AND lang='FI'`,
	).Scan(&nounParadigm); err != nil {
		t.Fatalf("query NOUN: %v", err)
	}
	if nounParadigm.Valid {
		t.Errorf("NOUN paradigm: got %q, want NULL", nounParadigm.String)
	}

	var verbParadigm string
	if err := db.QueryRow(
		`SELECT paradigm_class FROM lemmas WHERE lemma='kuurata' AND pos='VERB' AND lang='FI'`,
	).Scan(&verbParadigm); err != nil {
		t.Fatalf("query VERB: %v", err)
	}
	if verbParadigm != "73" {
		t.Errorf("VERB paradigm: got %q, want %q", verbParadigm, "73")
	}
}

func TestWriteKotusEntries_EmptyTaivutusDoesNotClobberExistingParadigm(t *testing.T) {
	// Codex P1b: rows with empty Taivutustiedot must not nullify a
	// previously imported paradigm_class. Real example: piilevä#1 is
	// ADJ class 10, piilevä#2 is NOUN with no class. Row 2 must NOT
	// overwrite the ADJ paradigm with NULL.
	db := newTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority, paradigm_class)
		 VALUES ('piilevä', 'ADJ', NULL, 'FI', 'kotus', 10, '10')`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Process the second row's shape: NOUN with empty Taivutustiedot.
	// (Sanaluokka is "substantiivi" only; current row doesn't claim ADJ.)
	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "piilevä", WordClasses: []string{"substantiivi"}},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.NewLemmasInserted != 1 {
		t.Errorf("new = %d, want 1 (NOUN)", stats.NewLemmasInserted)
	}
	if stats.ParadigmsUpgraded != 0 {
		t.Errorf("upgraded = %d, want 0 (ADJ row is out of scope)", stats.ParadigmsUpgraded)
	}

	// ADJ row preserved at class 10.
	var adjParadigm string
	if err := db.QueryRow(
		`SELECT paradigm_class FROM lemmas WHERE lemma='piilevä' AND pos='ADJ' AND lang='FI'`,
	).Scan(&adjParadigm); err != nil {
		t.Fatalf("query ADJ: %v", err)
	}
	if adjParadigm != "10" {
		t.Errorf("ADJ paradigm preserved: got %q, want %q (must not be clobbered by an unrelated row's empty class)",
			adjParadigm, "10")
	}

	// NOUN row inserted with NULL paradigm.
	var nounParadigm sql.NullString
	if err := db.QueryRow(
		`SELECT paradigm_class FROM lemmas WHERE lemma='piilevä' AND pos='NOUN' AND lang='FI'`,
	).Scan(&nounParadigm); err != nil {
		t.Fatalf("query NOUN: %v", err)
	}
	if nounParadigm.Valid {
		t.Errorf("NOUN paradigm: got %q, want NULL", nounParadigm.String)
	}
}

func TestWriteKotusEntries_EmptyTaivutusOnSamePOSPreservesExistingParadigm(t *testing.T) {
	// Similar guard, narrower case: existing kaikki NOUN with paradigm
	// class set; Kotus row says NOUN with empty Taivutustiedot. The
	// existing paradigm must NOT be clobbered with NULL.
	db := newTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority, paradigm_class)
		 VALUES ('foo', 'NOUN', 'foo', 'FI', 'kaikki', 10, '5')`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "foo", WordClasses: []string{"substantiivi"}}, // empty Taivutus
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.ParadigmsUpgraded != 0 {
		t.Errorf("upgraded = %d, want 0 (no class to apply, existing must be preserved)", stats.ParadigmsUpgraded)
	}

	var paradigm string
	if err := db.QueryRow(
		`SELECT paradigm_class FROM lemmas WHERE lemma='foo' AND pos='NOUN' AND lang='FI'`,
	).Scan(&paradigm); err != nil {
		t.Fatalf("query: %v", err)
	}
	if paradigm != "5" {
		t.Errorf("paradigm preserved: got %q, want %q", paradigm, "5")
	}
}
