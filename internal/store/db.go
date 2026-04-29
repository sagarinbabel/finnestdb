package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	db *sql.DB
}

type User struct {
	ID            int64
	Email         string
	EmailVerified bool
	IsAdmin       bool
	Settings      map[string]interface{}
}

type Deck struct {
	ID        int64
	UserID    int64
	Title     string
	Lang      string
	CreatedAt time.Time
}

type Sentence struct {
	ID     int64
	DeckID int64
	Text   string
	Lang   string
}

type Card struct {
	ID     int64
	UserID int64
	Lang   string
	Lemma  string
	POS    string
	MWEID  *int64
}

type KnownLemma struct {
	Lemma string `json:"lemma"`
	POS   string `json:"pos"`
	Lang  string `json:"lang"`
}

func NewDB(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	store := &DB{db: db}
	if err := store.initSchema(); err != nil {
		return nil, err
	}

	return store, nil
}

func (d *DB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE,
		email_verified INTEGER DEFAULT 0,
		is_admin INTEGER DEFAULT 0,
		settings_json TEXT DEFAULT '{"new_per_day":20,"retention":0.9,"theme":"system"}'
	);

	CREATE TABLE IF NOT EXISTS decks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		lang TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS sentences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		deck_id INTEGER NOT NULL,
		text TEXT NOT NULL,
		lang TEXT NOT NULL,
		FOREIGN KEY(deck_id) REFERENCES decks(id)
	);

	CREATE TABLE IF NOT EXISTS lemmas (
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		gloss TEXT,
		lang TEXT NOT NULL,
		PRIMARY KEY(lemma, pos, lang)
	);

	CREATE TABLE IF NOT EXISTS occurrence (
		deck_id INTEGER NOT NULL,
		sentence_id INTEGER NOT NULL,
		token_ix INTEGER NOT NULL,
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		FOREIGN KEY(deck_id) REFERENCES decks(id),
		FOREIGN KEY(sentence_id) REFERENCES sentences(id),
		UNIQUE(deck_id, sentence_id, token_ix)
	);

	CREATE TABLE IF NOT EXISTS cards (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		lang TEXT NOT NULL DEFAULT '',
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		mwe_id INTEGER,
		FOREIGN KEY(user_id) REFERENCES users(id),
		UNIQUE(user_id, lang, lemma, pos, mwe_id)
	);

	CREATE TABLE IF NOT EXISTS card_state (
		card_id INTEGER PRIMARY KEY,
		fsrs_json TEXT,
		next_due DATETIME,
		last_answer_at DATETIME,
		FOREIGN KEY(card_id) REFERENCES cards(id)
	);

	CREATE TABLE IF NOT EXISTS user_known_lemmas (
		user_id INTEGER NOT NULL,
		lang TEXT NOT NULL DEFAULT '',
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		PRIMARY KEY(user_id, lang, lemma, pos),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS user_ignored_lemmas (
		user_id INTEGER NOT NULL,
		lang TEXT NOT NULL DEFAULT '',
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		PRIMARY KEY(user_id, lang, lemma, pos),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	-- Dictionary: maps inflected surface forms to their canonical lemma + POS.
	-- PRIMARY KEY (form, lang) = one lemma per surface form per language (first-import wins).
	-- Finnish possessive forms (e.g. "kirjassani") are NOT imported here; they are handled
	-- at enrichment time via suffix stripping (see internal/store/dict.go).
	CREATE TABLE IF NOT EXISTS forms (
		form  TEXT NOT NULL,
		lemma TEXT NOT NULL,
		pos   TEXT NOT NULL,
		lang  TEXT NOT NULL,
		PRIMARY KEY (form, lang)
	);

	-- Tracks when dictionary data was imported and from which source.
	CREATE TABLE IF NOT EXISTS dict_metadata (
		lang        TEXT NOT NULL,
		source      TEXT NOT NULL,
		imported_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		row_count   INTEGER,
		PRIMARY KEY (lang, source)
	);
	`

	_, err := d.db.Exec(schema)
	if err != nil {
		return err
	}

	if _, err := d.db.Exec(`ALTER TABLE users ADD COLUMN is_admin INTEGER DEFAULT 0`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	if _, err := d.db.Exec(`ALTER TABLE cards ADD COLUMN lang TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	if _, err := d.db.Exec(`ALTER TABLE user_known_lemmas ADD COLUMN lang TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	if _, err := d.db.Exec(`ALTER TABLE user_ignored_lemmas ADD COLUMN lang TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}

	return nil
}

func (d *DB) GetOrCreateUser(email string) (*User, error) {
	var user User
	var settingsJSON string
	email = normalizeEmail(email)
	isAdmin := isAdminEmail(email)

	err := d.db.QueryRow(
		"SELECT id, email, email_verified, is_admin, settings_json FROM users WHERE email = ?",
		email,
	).Scan(&user.ID, &user.Email, &user.EmailVerified, &user.IsAdmin, &settingsJSON)

	if err == sql.ErrNoRows {
		// Create new user
		settingsJSON = `{"new_per_day":20,"retention":0.9,"theme":"system"}`
		result, err := d.db.Exec(
			"INSERT INTO users (email, email_verified, is_admin, settings_json) VALUES (?, 1, ?, ?)",
			email, boolToInt(isAdmin), settingsJSON,
		)
		if err != nil {
			return nil, err
		}
		user.ID, _ = result.LastInsertId()
		user.Email = email
		user.EmailVerified = true
		user.IsAdmin = isAdmin
		json.Unmarshal([]byte(settingsJSON), &user.Settings)
		return &user, nil
	}
	if err != nil {
		return nil, err
	}

	if user.IsAdmin != isAdmin {
		if _, err := d.db.Exec(`UPDATE users SET is_admin = ? WHERE id = ?`, boolToInt(isAdmin), user.ID); err != nil {
			return nil, err
		}
		user.IsAdmin = isAdmin
	}

	json.Unmarshal([]byte(settingsJSON), &user.Settings)
	return &user, nil
}

func (d *DB) GetUserByID(userID int64) (*User, error) {
	var user User
	var settingsJSON string

	err := d.db.QueryRow(
		`SELECT id, email, email_verified, is_admin, settings_json FROM users WHERE id = ?`,
		userID,
	).Scan(&user.ID, &user.Email, &user.EmailVerified, &user.IsAdmin, &settingsJSON)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(settingsJSON), &user.Settings)
	return &user, nil
}

func (d *DB) CreateDeck(userID int64, title, lang string) (int64, error) {
	result, err := d.db.Exec(
		"INSERT INTO decks (user_id, title, lang) VALUES (?, ?, ?)",
		userID, title, lang,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) CreateSentence(deckID int64, text, lang string) (int64, error) {
	result, err := d.db.Exec(
		"INSERT INTO sentences (deck_id, text, lang) VALUES (?, ?, ?)",
		deckID, text, lang,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) GetUserDecks(userID int64) ([]Deck, error) {
	rows, err := d.db.Query(
		"SELECT id, user_id, title, lang, created_at FROM decks WHERE user_id = ? ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decks []Deck
	for rows.Next() {
		var deck Deck
		if err := rows.Scan(&deck.ID, &deck.UserID, &deck.Title, &deck.Lang, &deck.CreatedAt); err != nil {
			return nil, err
		}
		decks = append(decks, deck)
	}
	return decks, rows.Err()
}

// CreateOccurrence records that a lemma+pos token appeared at position tokenIx
// within the given sentence (which belongs to the given deck).
func (d *DB) CreateOccurrence(deckID, sentenceID int64, tokenIx int, lemma, pos string) error {
	_, err := d.db.Exec(
		`INSERT OR IGNORE INTO occurrence (deck_id, sentence_id, token_ix, lemma, pos)
		 VALUES (?, ?, ?, ?, ?)`,
		deckID, sentenceID, tokenIx, lemma, pos,
	)
	return err
}

// FormsCount returns the number of rows in the forms table for the given lang.
// Used at startup to detect whether the dictionary has been imported.
func (d *DB) FormsCount(lang string) (int, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM forms WHERE lang = ?`, lang,
	).Scan(&count)
	return count, err
}

func (d *DB) UpsertLemma(lemma, pos, gloss, lang string) error {
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO lemmas (lemma, pos, gloss, lang) VALUES (?, ?, ?, ?)`,
		lemma, pos, gloss, lang,
	)
	return err
}

func (d *DB) UpsertForm(form, lemma, pos, lang string) error {
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO forms (form, lemma, pos, lang) VALUES (?, ?, ?, ?)`,
		strings.ToLower(strings.TrimSpace(form)), lemma, pos, lang,
	)
	return err
}

func (d *DB) ImportKnownWords(userID int64, lang string, words []string) ([]KnownLemma, []string, error) {
	normalized := make([]string, 0, len(words))
	seen := make(map[string]struct{}, len(words))
	for _, word := range words {
		trimmed := strings.TrimSpace(strings.ToLower(word))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	resolutions := d.BatchLookupForms(normalized, lang, "custom")
	imported := make([]KnownLemma, 0, len(resolutions))
	unresolved := make([]string, 0)
	importedSeen := make(map[string]struct{}, len(resolutions))
	for _, word := range normalized {
		resolution, ok := resolutions[word]
		if !ok || resolution.Lemma == "" || resolution.POS == "" {
			unresolved = append(unresolved, word)
			continue
		}
		if _, err := d.db.Exec(
			`INSERT OR IGNORE INTO user_known_lemmas (user_id, lang, lemma, pos) VALUES (?, ?, ?, ?)`,
			userID, lang, resolution.Lemma, resolution.POS,
		); err != nil {
			return nil, nil, err
		}
		key := resolution.Lemma + "\x00" + resolution.POS
		if _, ok := importedSeen[key]; ok {
			continue
		}
		importedSeen[key] = struct{}{}
		imported = append(imported, KnownLemma{
			Lemma: resolution.Lemma,
			POS:   resolution.POS,
			Lang:  lang,
		})
	}

	return imported, unresolved, nil
}

func (d *DB) ListKnownWords(userID int64, lang string) ([]KnownLemma, error) {
	rows, err := d.db.Query(
		`SELECT lemma, pos, lang
		 FROM user_known_lemmas
		 WHERE user_id = ? AND lang = ?
		 ORDER BY lemma ASC, pos ASC`,
		userID, lang,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lemmas []KnownLemma
	for rows.Next() {
		var lemma KnownLemma
		if err := rows.Scan(&lemma.Lemma, &lemma.POS, &lemma.Lang); err != nil {
			return nil, err
		}
		lemmas = append(lemmas, lemma)
	}
	return lemmas, rows.Err()
}

func (d *DB) DeleteKnownWord(userID int64, lang, lemma, pos string) error {
	_, err := d.db.Exec(
		`DELETE FROM user_known_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?`,
		userID, lang, lemma, pos,
	)
	return err
}

func (d *DB) IsKnownOrIgnored(userID int64, lang, lemma, pos string) (bool, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM (
			SELECT 1 FROM user_known_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?
			UNION ALL
			SELECT 1 FROM user_ignored_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?
		)`,
		userID, lang, lemma, pos,
		userID, lang, lemma, pos,
	).Scan(&count)
	return count > 0, err
}

func (d *DB) EnsureCard(userID int64, lang, lemma, pos string) (int64, error) {
	_, err := d.db.Exec(
		`INSERT OR IGNORE INTO cards (user_id, lang, lemma, pos) VALUES (?, ?, ?, ?)`,
		userID, lang, lemma, pos,
	)
	if err != nil {
		return 0, err
	}

	var cardID int64
	err = d.db.QueryRow(
		`SELECT id FROM cards WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ? AND mwe_id IS NULL`,
		userID, lang, lemma, pos,
	).Scan(&cardID)
	if err != nil {
		return 0, err
	}

	if _, err := d.db.Exec(
		`INSERT OR IGNORE INTO card_state (card_id, fsrs_json, next_due, last_answer_at) VALUES (?, NULL, NULL, NULL)`,
		cardID,
	); err != nil {
		return 0, err
	}

	return cardID, nil
}

func (d *DB) CountCards(userID int64, lang string) (int, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM cards WHERE user_id = ? AND lang = ?`,
		userID, lang,
	).Scan(&count)
	return count, err
}

func (d *DB) Close() error {
	return d.db.Close()
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isAdminEmail(email string) bool {
	email = normalizeEmail(email)
	if email == "" {
		return false
	}
	for _, candidate := range strings.Split(os.Getenv("FINNESTDB_ADMIN_EMAILS"), ",") {
		if normalizeEmail(candidate) == email {
			return true
		}
	}
	return false
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
