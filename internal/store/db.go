package store

import (
	"database/sql"
	"encoding/json"
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
	Lemma  string
	POS    string
	MWEID  *int64
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
		FOREIGN KEY(sentence_id) REFERENCES sentences(id)
	);

	CREATE TABLE IF NOT EXISTS cards (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		mwe_id INTEGER,
		FOREIGN KEY(user_id) REFERENCES users(id),
		UNIQUE(user_id, lemma, pos, mwe_id)
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
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		PRIMARY KEY(user_id, lemma, pos),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS user_ignored_lemmas (
		user_id INTEGER NOT NULL,
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		PRIMARY KEY(user_id, lemma, pos),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	`

	_, err := d.db.Exec(schema)
	return err
}

func (d *DB) GetOrCreateUser(email string) (*User, error) {
	var user User
	var settingsJSON string

	err := d.db.QueryRow(
		"SELECT id, email, email_verified, settings_json FROM users WHERE email = ?",
		email,
	).Scan(&user.ID, &user.Email, &user.EmailVerified, &settingsJSON)

	if err == sql.ErrNoRows {
		// Create new user
		settingsJSON = `{"new_per_day":20,"retention":0.9,"theme":"system"}`
		result, err := d.db.Exec(
			"INSERT INTO users (email, email_verified, settings_json) VALUES (?, 1, ?)",
			email, settingsJSON,
		)
		if err != nil {
			return nil, err
		}
		user.ID, _ = result.LastInsertId()
		user.Email = email
		user.EmailVerified = true
		json.Unmarshal([]byte(settingsJSON), &user.Settings)
		return &user, nil
	}
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

func (d *DB) Close() error {
	return d.db.Close()
}

