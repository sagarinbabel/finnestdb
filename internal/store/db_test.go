package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// columnExists reports whether the given table has the given column.
func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}

// tableExists reports whether the given table is present in sqlite_master.
func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("sqlite_master query: %v", err)
	}
	return true
}

// TestEnsureLexicalEnrichmentColumns_FreshDB verifies a brand-new DB created by
// NewDB has the paradigm_class column on lemmas, the feats column on forms,
// and the translations and definitions tables.
func TestEnsureLexicalEnrichmentColumns_FreshDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fresh.db")
	d, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer d.Close()

	if !columnExists(t, d.db, "lemmas", "paradigm_class") {
		t.Error("lemmas.paradigm_class missing on fresh DB")
	}
	if !columnExists(t, d.db, "forms", "feats") {
		t.Error("forms.feats missing on fresh DB")
	}
	if !columnExists(t, d.db, "occurrence", "surface") {
		t.Error("occurrence.surface missing on fresh DB")
	}
	if !tableExists(t, d.db, "translations") {
		t.Error("translations table missing on fresh DB")
	}
	if !tableExists(t, d.db, "definitions") {
		t.Error("definitions table missing on fresh DB")
	}
	if !tableExists(t, d.db, "learning_targets") {
		t.Error("learning_targets table missing on fresh DB")
	}
	if !tableExists(t, d.db, "correction_overlays") {
		t.Error("correction_overlays table missing on fresh DB")
	}
}

// TestEnsureLexicalEnrichmentColumns_BackfillsOldDB constructs a DB at the
// pre-Finnish-lexical-plan shape (lemmas/forms with source/source_priority but
// no paradigm_class/feats; no translations/definitions tables) and verifies
// the backfill helpers bring it up to current shape, idempotently.
func TestEnsureLexicalEnrichmentColumns_BackfillsOldDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer raw.Close()

	if _, err := raw.Exec(`
		CREATE TABLE lemmas (
			lemma TEXT NOT NULL,
			pos TEXT NOT NULL,
			gloss TEXT,
			lang TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_priority INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(lemma, pos, lang)
		);
		CREATE TABLE forms (
			form  TEXT NOT NULL,
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			lang  TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_priority INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (form, lang)
		);
	`); err != nil {
		t.Fatalf("seed old schema: %v", err)
	}

	if columnExists(t, raw, "lemmas", "paradigm_class") {
		t.Fatal("paradigm_class unexpectedly present in old-shape seed")
	}
	if tableExists(t, raw, "translations") {
		t.Fatal("translations unexpectedly present in old-shape seed")
	}

	if err := EnsureLexicalEnrichmentColumns(raw); err != nil {
		t.Fatalf("EnsureLexicalEnrichmentColumns: %v", err)
	}
	if err := EnsureLexicalEntryTables(raw); err != nil {
		t.Fatalf("EnsureLexicalEntryTables: %v", err)
	}

	if !columnExists(t, raw, "lemmas", "paradigm_class") {
		t.Error("paradigm_class not added by backfill")
	}
	if !columnExists(t, raw, "forms", "feats") {
		t.Error("feats not added by backfill")
	}
	if !tableExists(t, raw, "translations") {
		t.Error("translations not created by backfill")
	}
	if !tableExists(t, raw, "definitions") {
		t.Error("definitions not created by backfill")
	}

	// Idempotent: re-running both helpers must not error.
	if err := EnsureLexicalEnrichmentColumns(raw); err != nil {
		t.Errorf("EnsureLexicalEnrichmentColumns rerun: %v", err)
	}
	if err := EnsureLexicalEntryTables(raw); err != nil {
		t.Errorf("EnsureLexicalEntryTables rerun: %v", err)
	}
}

func TestEnsureOccurrenceSurfaceColumn_BackfillsOldDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old-occurrence.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer raw.Close()

	if _, err := raw.Exec(`
		CREATE TABLE occurrence (
			deck_id INTEGER NOT NULL,
			sentence_id INTEGER NOT NULL,
			token_ix INTEGER NOT NULL,
			lemma TEXT NOT NULL,
			pos TEXT NOT NULL,
			UNIQUE(deck_id, sentence_id, token_ix, lemma, pos)
		);
	`); err != nil {
		t.Fatalf("seed old occurrence schema: %v", err)
	}
	if columnExists(t, raw, "occurrence", "surface") {
		t.Fatal("surface unexpectedly present in old occurrence schema")
	}

	if err := EnsureOccurrenceSurfaceColumn(raw); err != nil {
		t.Fatalf("EnsureOccurrenceSurfaceColumn: %v", err)
	}
	if !columnExists(t, raw, "occurrence", "surface") {
		t.Fatal("surface not added by backfill")
	}
	if err := EnsureOccurrenceSurfaceColumn(raw); err != nil {
		t.Fatalf("EnsureOccurrenceSurfaceColumn rerun: %v", err)
	}
}

func TestEnsureCorrectionOverlaySchema_BackfillsAndConstrains(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old-corrections.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer raw.Close()

	if tableExists(t, raw, "learning_targets") {
		t.Fatal("learning_targets unexpectedly present in old-shape seed")
	}
	if tableExists(t, raw, "correction_overlays") {
		t.Fatal("correction_overlays unexpectedly present in old-shape seed")
	}

	if err := EnsureCorrectionOverlaySchema(raw); err != nil {
		t.Fatalf("EnsureCorrectionOverlaySchema: %v", err)
	}
	if !tableExists(t, raw, "learning_targets") {
		t.Fatal("learning_targets not created by backfill")
	}
	if !tableExists(t, raw, "correction_overlays") {
		t.Fatal("correction_overlays not created by backfill")
	}

	if _, err := raw.Exec(`
		INSERT INTO learning_targets
			(lang, target_kind, target_text, normalized_key, lemma, pos, feats, gloss, cue)
		VALUES
			('FI', 'proper_name', 'Maria', 'maria', 'Maria', 'PROPN', 'Number=Sing', '', 'person name'),
			('ET', 'proper_name', 'Maria', 'maria', 'Maria', 'PROPN', 'Number=Sing', '', 'person name')
	`); err != nil {
		t.Fatalf("insert language-separated targets: %v", err)
	}
	expectExecError(t, raw, `
		INSERT INTO learning_targets
			(lang, target_kind, target_text, normalized_key, lemma, pos, feats)
		VALUES ('FI', 'proper_name', 'Maria', 'maria', 'Maria', 'PROPN', 'Number=Sing')
	`)
	expectExecError(t, raw, `
		INSERT INTO learning_targets
			(lang, target_kind, target_text, normalized_key)
		VALUES ('FI', 'dictionary', 'Maria', 'maria')
	`)

	var targetID int64
	if err := raw.QueryRow(`
		SELECT id FROM learning_targets
		WHERE lang = 'FI' AND target_kind = 'proper_name' AND normalized_key = 'maria'
	`).Scan(&targetID); err != nil {
		t.Fatalf("query target id: %v", err)
	}

	insertOverlay := `
		INSERT INTO correction_overlays
			(lang, correction_type, scope, target_id, surface,
			 original_lemma, original_pos, corrected_lemma, corrected_pos,
			 source_type, source_ref, source_locator, sentence_hash, provenance, note)
		VALUES
			('FI', 'parser_identity', 'source', ?, 'Maria',
			 'mari', 'NOUN', 'Maria', 'PROPN',
			 'paste', 'manual-card-fix', 'sentence:1 token:3', 'sha256:test', 'accepted_feedback', 'proper name')
	`
	if _, err := raw.Exec(insertOverlay, targetID); err != nil {
		t.Fatalf("insert overlay: %v", err)
	}
	expectExecError(t, raw, insertOverlay, targetID)
	expectExecError(t, raw, `
		INSERT INTO correction_overlays (lang, correction_type, scope)
		VALUES ('ET', 'parser_identity', 'paragraph')
	`)
	expectExecError(t, raw, `
		INSERT INTO correction_overlays (lang, correction_type, scope)
		VALUES ('ET', 'translation', 'global')
	`)

	if _, err := raw.Exec(`
		UPDATE correction_overlays SET active = 0
		WHERE lang = 'FI' AND correction_type = 'parser_identity'
	`); err != nil {
		t.Fatalf("deactivate overlay: %v", err)
	}
	if _, err := raw.Exec(insertOverlay, targetID); err != nil {
		t.Fatalf("insert replacement active overlay after deactivation: %v", err)
	}

	if err := EnsureCorrectionOverlaySchema(raw); err != nil {
		t.Fatalf("EnsureCorrectionOverlaySchema rerun: %v", err)
	}
}

func expectExecError(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err == nil {
		t.Fatalf("expected Exec error for query:\n%s", query)
	}
}

func TestGetNextReviewCardRespectsDailyNewCardLimit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	user, err := db.GetOrCreateUser("limit@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	if _, err := db.db.Exec(
		`UPDATE users SET settings_json = ? WHERE id = ?`,
		`{"new_per_day":1,"retention":0.9,"theme":"system"}`,
		user.ID,
	); err != nil {
		t.Fatalf("update user settings: %v", err)
	}

	if _, err := db.EnsureCard(user.ID, "FI", "kissa", "NOUN"); err != nil {
		t.Fatalf("EnsureCard(kissa): %v", err)
	}
	if _, err := db.EnsureCard(user.ID, "FI", "koira", "NOUN"); err != nil {
		t.Fatalf("EnsureCard(koira): %v", err)
	}

	card, err := db.GetNextReviewCard(user.ID, nil, "")
	if err != nil {
		t.Fatalf("GetNextReviewCard(before answer): %v", err)
	}
	if card == nil {
		t.Fatal("expected first new card before limit is exhausted")
	}

	if err := db.RecordReviewAnswer(user.ID, card.CardID, "good"); err != nil {
		t.Fatalf("RecordReviewAnswer: %v", err)
	}

	remaining, err := db.CountNewCards(user.ID)
	if err != nil {
		t.Fatalf("CountNewCards: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining new cards=%d want 0 after hitting daily limit", remaining)
	}

	next, err := db.GetNextReviewCard(user.ID, nil, "")
	if err != nil {
		t.Fatalf("GetNextReviewCard(after answer): %v", err)
	}
	if next != nil {
		t.Fatalf("expected no additional new card after daily limit, got %+v", next)
	}
}

func TestGetDeckDetailsUsesBatchGlossEnrichment(t *testing.T) {
	t.Run("ekilex override", func(t *testing.T) {
		db := newTestDB(t)
		user := createTestUser(t, db, "deck-ekilex@example.com")
		seedBadEkilexSeeGloss(t, db)

		deckID := createSingleTokenDeck(t, db, user.ID, "ET", "See.", "See", "see", "PRON")
		details, err := db.GetDeckDetails(user.ID, deckID)
		if err != nil {
			t.Fatalf("GetDeckDetails: %v", err)
		}

		if gloss := deckGloss(details, "see", "PRON"); gloss != "this; that" {
			t.Fatalf("deck see/PRON gloss=%q want this; that", gloss)
		}
	})

	t.Run("custom gloss", func(t *testing.T) {
		db := newTestDB(t)
		user := createTestUser(t, db, "deck-custom@example.com")
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

		deckID := createSingleTokenDeck(t, db, user.ID, "ET", "See.", "See", "see", "PRON")
		details, err := db.GetDeckDetails(user.ID, deckID)
		if err != nil {
			t.Fatalf("GetDeckDetails: %v", err)
		}

		if gloss := deckGloss(details, "see", "PRON"); gloss != "domain-specific see" {
			t.Fatalf("custom deck see/PRON gloss=%q want domain-specific see", gloss)
		}
	})
}

func TestGetNextReviewCardUsesBatchGlossEnrichment(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "review-ekilex@example.com")
	seedBadEkilexSeeGloss(t, db)
	createSingleTokenDeck(t, db, user.ID, "ET", "See.", "See", "see", "PRON")

	card, err := db.GetNextReviewCard(user.ID, nil, "")
	if err != nil {
		t.Fatalf("GetNextReviewCard: %v", err)
	}
	if card == nil {
		t.Fatal("GetNextReviewCard returned nil")
	}
	if card.Gloss != "this; that" {
		t.Fatalf("review see/PRON gloss=%q want this; that", card.Gloss)
	}
}

func createTestUser(t *testing.T, db *DB, email string) *User {
	t.Helper()
	user, err := db.GetOrCreateUser(email)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	return user
}

func seedBadEkilexSeeGloss(t *testing.T, db *DB) {
	t.Helper()
	seedLemmasFull(t, db, []struct {
		lemma, pos, gloss, lang, source string
		priority                        int
	}{
		{"see", "PRON", "here; it; this", "ET", "ekilex", 20},
	})
	seedTranslations(t, db, []struct {
		lemma, pos, lang, target, text, source string
		senseIdx                               int
	}{
		{"see", "PRON", "ET", "EN", "here", "ekilex", 0},
		{"see", "PRON", "ET", "EN", "this", "ekilex", 1},
	})
}

func createSingleTokenDeck(t *testing.T, db *DB, userID int64, lang, text, form, lemma, pos string) int64 {
	t.Helper()
	deckID, err := db.CreateDeckWithSentences(userID, "Test deck", lang, []DeckSentenceInput{
		{
			Text: text,
			Tokens: []DeckTokenInput{
				{TokenIx: 0, Form: form, Lemma: lemma, POS: pos},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateDeckWithSentences: %v", err)
	}
	return deckID
}

func deckGloss(details *DeckDetails, lemma, pos string) string {
	for _, item := range details.Lemmas {
		if item.Lemma == lemma && item.POS == pos {
			return item.Gloss
		}
	}
	return ""
}
