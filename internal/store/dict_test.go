package store

import (
	"testing"
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

// --- BatchLookupForms tests ---

func TestBatchLookupForms_Found(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"pankkiin", "pankki", "NOUN", "FI"},
		{"kirjassa", "kirja", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"pankkiin", "kirjassa"}, "FI")

	if got["pankkiin"] != [2]string{"pankki", "NOUN"} {
		t.Errorf("pankkiin: got %v, want {pankki NOUN}", got["pankkiin"])
	}
	if got["kirjassa"] != [2]string{"kirja", "NOUN"} {
		t.Errorf("kirjassa: got %v, want {kirja NOUN}", got["kirjassa"])
	}
}

func TestBatchLookupForms_NotFound(t *testing.T) {
	db := newTestDB(t)
	// No rows seeded — all lookups should miss.

	got := db.BatchLookupForms([]string{"viisutubettaja"}, "FI")

	if _, ok := got["viisutubettaja"]; ok {
		t.Errorf("viisutubettaja: expected absent from result map, got %v", got["viisutubettaja"])
	}
}

func TestBatchLookupForms_EmptyTable(t *testing.T) {
	db := newTestDB(t)
	// Empty forms table — must not panic, must return empty map.

	got := db.BatchLookupForms([]string{"pankki", "kirja", "talo"}, "FI")

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
	got := db.BatchLookupForms([]string{"Pankkiin", "PANKKIIN"}, "FI")

	if got["Pankkiin"] != [2]string{"pankki", "NOUN"} {
		t.Errorf("Pankkiin: got %v, want {pankki NOUN}", got["Pankkiin"])
	}
	if got["PANKKIIN"] != [2]string{"pankki", "NOUN"} {
		t.Errorf("PANKKIIN: got %v, want {pankki NOUN}", got["PANKKIIN"])
	}
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

	got := db.BatchLookupForms([]string{"Kirjassani"}, "FI")

	if got["Kirjassani"] != [2]string{"kirja", "NOUN"} {
		t.Errorf("Kirjassani: got %v, want {kirja NOUN}", got["Kirjassani"])
	}
}

func TestBatchLookupForms_PossessiveSuffixStrip(t *testing.T) {
	db := newTestDB(t)
	// Seed the base form "kirjassa" but NOT "kirjassani".
	// "kirjassani" = kirjassa + ni (possessive) → should resolve to kirja.
	seedForms(t, db, [][4]string{
		{"kirjassa", "kirja", "NOUN", "FI"},
	})

	got := db.BatchLookupForms([]string{"kirjassani"}, "FI")

	if got["kirjassani"] != [2]string{"kirja", "NOUN"} {
		t.Errorf("kirjassani (possessive strip): got %v, want {kirja NOUN}", got["kirjassani"])
	}
}

func TestBatchLookupForms_PossessiveStripNotAppliedForEstonian(t *testing.T) {
	db := newTestDB(t)
	// Estonian should NOT apply Finnish possessive suffix stripping.
	seedForms(t, db, [][4]string{
		{"kirjassa", "kirja", "NOUN", "ET"}, // seeded under ET lang
	})

	// "kirjassani" in ET context: direct lookup fails, no possessive strip → absent.
	got := db.BatchLookupForms([]string{"kirjassani"}, "ET")

	if _, ok := got["kirjassani"]; ok {
		t.Errorf("kirjassani ET: possessive strip should not apply for Estonian, got %v", got["kirjassani"])
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
