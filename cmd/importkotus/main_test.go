package main

import (
	"database/sql"
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

// --- ParadigmClass formatting ---

func TestParadigmClass_NoGradation(t *testing.T) {
	e := kotusEntry{InflectionType: "32"}
	if got := e.ParadigmClass(); got != "32" {
		t.Errorf("got %q, want %q", got, "32")
	}
}

func TestParadigmClass_WithGradation(t *testing.T) {
	e := kotusEntry{InflectionType: "1", Gradation: "F"}
	if got := e.ParadigmClass(); got != "1-F" {
		t.Errorf("got %q, want %q", got, "1-F")
	}
}

func TestParadigmClass_EmptyTNReturnsEmpty(t *testing.T) {
	e := kotusEntry{Gradation: "F"}
	if got := e.ParadigmClass(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// --- POS inference ---

func TestInferPOSFromInflectionType(t *testing.T) {
	cases := []struct {
		tn   string
		want string
	}{
		{"1", "NOUN"},  // valo
		{"32", "NOUN"}, // sisar
		{"38", "NOUN"}, // nainen / sininen — heuristic
		{"49", "NOUN"}, // askel
		{"50", "NUM"},  // edge of nominal range
		{"51", "NUM"},
		{"52", "VERB"}, // sanoa
		{"78", "VERB"}, // taitaa (last verb class)
		{"79", "X"},    // out of range
		{"0", "X"},
		{"", "X"},    // empty / missing
		{"abc", "X"}, // garbage
	}
	for _, tc := range cases {
		got := inferPOSFromInflectionType(tc.tn)
		if got != tc.want {
			t.Errorf("tn=%q got %q, want %q", tc.tn, got, tc.want)
		}
	}
}

// --- ParseSanalista ---

func TestParseSanalista_BasicEntries(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<kotus-sanalista version="1">
  <st>
    <s>aakkonen</s>
    <t><tn>32</tn></t>
  </st>
  <st>
    <s>aalto</s>
    <t><tn>1</tn><av>F</av></t>
  </st>
  <st>
    <s>sanoa</s>
    <t><tn>52</tn></t>
  </st>
</kotus-sanalista>`

	entries, skipped, err := ParseSanalista(strings.NewReader(xml))
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
		head, tn, av string
	}{
		{"aakkonen", "32", ""},
		{"aalto", "1", "F"},
		{"sanoa", "52", ""},
	}
	for i, w := range want {
		got := entries[i]
		if got.Headword != w.head || got.InflectionType != w.tn || got.Gradation != w.av {
			t.Errorf("entry[%d] = %+v, want head=%q tn=%q av=%q",
				i, got, w.head, w.tn, w.av)
		}
	}
}

func TestParseSanalista_SkipsEmptyHeadword(t *testing.T) {
	xml := `<kotus-sanalista>
  <st><s></s><t><tn>1</tn></t></st>
  <st><s>valo</s><t><tn>1</tn></t></st>
</kotus-sanalista>`
	entries, skipped, err := ParseSanalista(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if len(entries) != 1 || entries[0].Headword != "valo" {
		t.Errorf("entries = %+v, want [valo]", entries)
	}
}

func TestParseSanalista_AllInflectionClassesRoundTrip(t *testing.T) {
	// Smoke test that the XML parser handles all 78 Kotus inflection
	// class numbers without choking. Synthesizes one entry per class
	// with a placeholder headword. Guards against any future change
	// to the parser silently breaking on rare classes.
	var sb strings.Builder
	sb.WriteString(`<kotus-sanalista>`)
	for tn := 1; tn <= 78; tn++ {
		sb.WriteString(`<st><s>w`)
		sb.WriteString(itoa(tn))
		sb.WriteString(`</s><t><tn>`)
		sb.WriteString(itoa(tn))
		sb.WriteString(`</tn></t></st>`)
	}
	sb.WriteString(`</kotus-sanalista>`)

	entries, skipped, err := ParseSanalista(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if len(entries) != 78 {
		t.Fatalf("entries = %d, want 78", len(entries))
	}
	// Spot-check a few classes.
	if entries[0].InflectionType != "1" || entries[77].InflectionType != "78" {
		t.Errorf("first/last tn: %q / %q", entries[0].InflectionType, entries[77].InflectionType)
	}
}

func itoa(n int) string {
	// Tiny helper to avoid pulling strconv into the test for its only use.
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// --- writeKotusEntries ---

func TestWriteKotusEntries_UpgradesExistingKaikkiLemmaInPlace(t *testing.T) {
	// The headline policy: when kaikki already imported a lemma, Kotus
	// fills paradigm_class but leaves source/source_priority/gloss alone.
	// Without this, Kotus would clobber kaikki's gloss with NULL.
	db := newTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('valo', 'NOUN', 'light, lamp', 'FI', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "valo", InflectionType: "1"},
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
		t.Errorf("gloss preserved: got %q, want %q (Kotus must not clobber kaikki gloss)",
			gloss, "light, lamp")
	}
	if source != "kaikki" || priority != 10 {
		t.Errorf("source preserved: got %q/%d, want kaikki/10 (Kotus must not claim the row)",
			source, priority)
	}
	if paradigm != "1" {
		t.Errorf("paradigm_class: got %q, want %q", paradigm, "1")
	}
}

func TestWriteKotusEntries_UpgradesAllExistingPOSForLemma(t *testing.T) {
	// kaikki sometimes has the same headword at multiple POS (e.g. ADJ
	// + NOUN for "sininen"). Kotus's <s>sininen</s><tn>38</tn> doesn't
	// disambiguate which POS — we apply the paradigm_class to BOTH.
	db := newTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('sininen', 'ADJ', 'blue', 'FI', 'kaikki', 10),
		        ('sininen', 'NOUN', 'a blue thing', 'FI', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "sininen", InflectionType: "38"},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.ParadigmsUpgraded != 2 {
		t.Errorf("paradigms_upgraded = %d, want 2 (both ADJ and NOUN)", stats.ParadigmsUpgraded)
	}
	// Regression guard: a single <st> that fans out into multiple row
	// updates must count as ONE entry, not two. Earlier versions derived
	// EntriesProcessed from the row counters, which double-counted here.
	if stats.EntriesProcessed != 1 {
		t.Errorf("entries_processed = %d, want 1 (one <st>, multiple rows)", stats.EntriesProcessed)
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

func TestWriteKotusEntries_InsertsNewLemmaWhenNotPresent(t *testing.T) {
	// Kotus-exclusive headword (kaikki didn't have it). Insert at
	// inferred POS with source='kotus', priority=10.
	db := newTestDB(t)

	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "obscurus", InflectionType: "1"}, // → NOUN
		{Headword: "tehdä", InflectionType: "73"},   // → VERB
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.NewLemmasInserted != 2 {
		t.Errorf("new_lemmas_inserted = %d, want 2", stats.NewLemmasInserted)
	}

	cases := []struct {
		lemma, pos, paradigm string
	}{
		{"obscurus", "NOUN", "1"},
		{"tehdä", "VERB", "73"},
	}
	for _, c := range cases {
		var pos, source, paradigm string
		var priority int
		var gloss sql.NullString
		if err := db.QueryRow(
			`SELECT pos, source, source_priority, paradigm_class, gloss
			 FROM lemmas WHERE lemma=? AND lang='FI'`, c.lemma,
		).Scan(&pos, &source, &priority, &paradigm, &gloss); err != nil {
			t.Fatalf("query %s: %v", c.lemma, err)
		}
		if pos != c.pos {
			t.Errorf("%s POS: got %q, want %q", c.lemma, pos, c.pos)
		}
		if source != "kotus" || priority != 10 {
			t.Errorf("%s source/priority: got %q/%d, want kotus/10", c.lemma, source, priority)
		}
		if paradigm != c.paradigm {
			t.Errorf("%s paradigm: got %q, want %q", c.lemma, paradigm, c.paradigm)
		}
		if gloss.Valid {
			t.Errorf("%s gloss: got %q, want NULL (Kotus has no glosses)", c.lemma, gloss.String)
		}
	}
}

func TestWriteKotusEntries_SkipsEntriesWithEmptyTN(t *testing.T) {
	// Defensive: an entry with no <tn> can't be useful. Count it but
	// don't write a row.
	db := newTestDB(t)
	stats, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "valo", InflectionType: "1"},
		{Headword: "broken"}, // no tn
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if stats.NewLemmasInserted != 1 {
		t.Errorf("new_lemmas_inserted = %d, want 1", stats.NewLemmasInserted)
	}
	if stats.HeadwordsWithNoTN != 1 {
		t.Errorf("skipped_no_tn = %d, want 1", stats.HeadwordsWithNoTN)
	}
}

func TestWriteKotusEntries_WithGradationProducesCompoundClass(t *testing.T) {
	db := newTestDB(t)
	if _, err := writeKotusEntries(db, []kotusEntry{
		{Headword: "aalto", InflectionType: "1", Gradation: "F"},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	var paradigm string
	if err := db.QueryRow(
		`SELECT paradigm_class FROM lemmas WHERE lemma='aalto' AND lang='FI'`,
	).Scan(&paradigm); err != nil {
		t.Fatalf("query: %v", err)
	}
	if paradigm != "1-F" {
		t.Errorf("paradigm_class: got %q, want %q (tn-av compound)", paradigm, "1-F")
	}
}

// --- end-to-end Import ---

func TestImport_EndToEnd(t *testing.T) {
	db := newTestDB(t)

	// Pre-existing kaikki rows for a couple of headwords.
	if _, err := db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('valo', 'NOUN', 'light', 'FI', 'kaikki', 10),
		        ('sanoa', 'VERB', 'to say', 'FI', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	xml := `<kotus-sanalista version="1">
  <st><s>valo</s><t><tn>1</tn></t></st>
  <st><s>sanoa</s><t><tn>52</tn></t></st>
  <st><s>uusi-uudempi</s><t><tn>16</tn></t></st>
</kotus-sanalista>`

	stats, err := Import(db, strings.NewReader(xml))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if stats.EntriesProcessed != 3 {
		t.Errorf("processed = %d, want 3", stats.EntriesProcessed)
	}
	if stats.ParadigmsUpgraded != 2 {
		t.Errorf("upgraded = %d, want 2 (valo + sanoa)", stats.ParadigmsUpgraded)
	}
	if stats.NewLemmasInserted != 1 {
		t.Errorf("new = %d, want 1 (uusi-uudempi)", stats.NewLemmasInserted)
	}
}

func TestStreamSanalista_CallbackErrorAborts(t *testing.T) {
	// streamSanalista must surface the callback's error and stop
	// decoding. The new Import path uses this to abort mid-stream
	// when a write fails, leaving the surrounding tx.Rollback() to
	// undo any partial writes.
	xml := `<kotus-sanalista>
  <st><s>a</s><t><tn>1</tn></t></st>
  <st><s>b</s><t><tn>2</tn></t></st>
  <st><s>c</s><t><tn>3</tn></t></st>
</kotus-sanalista>`

	seen := 0
	want := errStop
	skipped, err := streamSanalista(strings.NewReader(xml), func(e kotusEntry) error {
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
		t.Errorf("callback called %d times, want 2 (should abort after 2nd entry's error)", seen)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
}

var errStop = errFromString("stop")

type errFromString string

func (e errFromString) Error() string { return string(e) }
