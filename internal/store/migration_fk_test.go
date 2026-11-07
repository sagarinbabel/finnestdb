package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// Legacy table rebuilds must survive foreign-key enforcement.
//
// Why it matters: enabling foreign_keys in the DSN made NewDB fail outright
// with "FOREIGN KEY constraint failed" on any database old enough to still
// need the surface-card or language-scoping migration AND carrying review
// history - the app would not open the file at all. DROP TABLE on a parent
// does an implicit delete, so dropping `cards` while review_log.card_id
// references it is a violation, even though an identical table is renamed back
// into place two statements later.
//
// The fixture deliberately declares the review_log -> cards foreign key. The
// pre-existing surface-migration fixtures omit it, which is exactly why they
// did not catch this.
func TestLegacyCardsRebuildSurvivesForeignKeyEnforcement(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Legacy schema: cards WITHOUT surface_norm, plus review_log with its FK.
	for _, stmt := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT UNIQUE,
		   email_verified INTEGER DEFAULT 0, is_admin INTEGER DEFAULT 0,
		   password_hash TEXT NOT NULL DEFAULT '', settings_json TEXT DEFAULT '{}')`,
		`CREATE TABLE cards (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL,
		   lang TEXT NOT NULL DEFAULT '', lemma TEXT NOT NULL, pos TEXT NOT NULL, mwe_id INTEGER,
		   FOREIGN KEY(user_id) REFERENCES users(id),
		   UNIQUE(user_id, lang, lemma, pos, mwe_id))`,
		`CREATE TABLE card_state (card_id INTEGER PRIMARY KEY, fsrs_json TEXT, next_due DATETIME,
		   last_answer_at DATETIME, introduced_at DATETIME,
		   FOREIGN KEY(card_id) REFERENCES cards(id))`,
		`CREATE TABLE review_log (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL,
		   card_id INTEGER NOT NULL, lang TEXT NOT NULL DEFAULT '', rating TEXT NOT NULL,
		   reviewed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		   FOREIGN KEY(user_id) REFERENCES users(id),
		   FOREIGN KEY(card_id) REFERENCES cards(id))`,
		`INSERT INTO users (id, email) VALUES (1, 'legacy@example.com')`,
		`INSERT INTO cards (id, user_id, lang, lemma, pos) VALUES (1, 1, 'FI', 'kissa', 'NOUN')`,
		`INSERT INTO card_state (card_id, next_due, last_answer_at) VALUES (1, '2026-01-01 00:00:00', '2025-12-31 00:00:00')`,
		`INSERT INTO review_log (user_id, card_id, lang, rating) VALUES (1, 1, 'FI', 'good')`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	raw.Close()

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB on a legacy DB with review history failed: %v", err)
	}
	defer db.Close()

	// The migration must have preserved the review history and the card.
	var cards, states, logs int
	db.db.QueryRow(`SELECT COUNT(*) FROM cards`).Scan(&cards)
	db.db.QueryRow(`SELECT COUNT(*) FROM card_state`).Scan(&states)
	db.db.QueryRow(`SELECT COUNT(*) FROM review_log`).Scan(&logs)
	t.Logf("after migration: cards=%d card_state=%d review_log=%d", cards, states, logs)
	if cards != 1 || states != 1 || logs != 1 {
		t.Errorf("migration lost data: cards=%d card_state=%d review_log=%d, want 1/1/1", cards, states, logs)
	}
	var surfaceNorm string
	if err := db.db.QueryRow(`SELECT surface_norm FROM cards WHERE id = 1`).Scan(&surfaceNorm); err != nil {
		t.Errorf("surface_norm missing after migration: %v", err)
	}
}
