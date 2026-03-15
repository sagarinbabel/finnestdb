package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const defaultSettingsJSON = `{"new_per_day":20,"retention":0.9,"theme":"system"}`

var ErrNotFound = errors.New("not found")

type DB struct {
	db *sql.DB
}

type User struct {
	ID            int64
	Email         string
	EmailVerified bool
	PasswordHash  string
	IsAdmin       bool
	Plan          string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Profile       UserProfile
}

type UserProfile struct {
	UserID      int64
	Settings    map[string]interface{}
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
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

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE,
		email_verified INTEGER DEFAULT 0,
		password_hash TEXT NOT NULL DEFAULT '',
		is_admin INTEGER NOT NULL DEFAULT 0,
		plan TEXT NOT NULL DEFAULT 'free',
		settings_json TEXT DEFAULT '` + defaultSettingsJSON + `',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS user_profiles (
		user_id INTEGER PRIMARY KEY,
		settings_json TEXT NOT NULL DEFAULT '` + defaultSettingsJSON + `',
		display_name TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS user_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		revoked_at DATETIME,
		last_seen_at DATETIME,
		ip_address TEXT,
		user_agent TEXT,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS password_reset_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		used_at DATETIME,
		FOREIGN KEY(user_id) REFERENCES users(id)
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

	CREATE TABLE IF NOT EXISTS forms (
		form  TEXT NOT NULL,
		lemma TEXT NOT NULL,
		pos   TEXT NOT NULL,
		lang  TEXT NOT NULL,
		PRIMARY KEY (form, lang)
	);

	CREATE TABLE IF NOT EXISTS dict_metadata (
		lang        TEXT NOT NULL,
		source      TEXT NOT NULL,
		imported_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		row_count   INTEGER,
		PRIMARY KEY (lang, source)
	);
	`

	if _, err := d.db.Exec(schema); err != nil {
		return err
	}

	if err := d.migrateUserSchema(); err != nil {
		return err
	}

	return d.backfillUserProfiles()
}

func (d *DB) migrateUserSchema() error {
	userColumns := []struct {
		name       string
		definition string
	}{
		{"password_hash", "TEXT NOT NULL DEFAULT ''"},
		{"is_admin", "INTEGER NOT NULL DEFAULT 0"},
		{"plan", "TEXT NOT NULL DEFAULT 'free'"},
		{"created_at", "DATETIME"},
		{"updated_at", "DATETIME"},
	}

	for _, col := range userColumns {
		if err := d.ensureColumn("users", col.name, col.definition); err != nil {
			return err
		}
	}

	if _, err := d.db.Exec(`UPDATE users SET created_at = COALESCE(created_at, CURRENT_TIMESTAMP), updated_at = COALESCE(updated_at, CURRENT_TIMESTAMP)`); err != nil {
		return err
	}

	return nil
}

func (d *DB) ensureColumn(table, name, definition string) error {
	exists, err := d.columnExists(table, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = d.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + name + ` ` + definition)
	return err
}

func (d *DB) columnExists(table, name string) (bool, error) {
	rows, err := d.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			colName   string
			colType   string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if colName == name {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (d *DB) backfillUserProfiles() error {
	_, err := d.db.Exec(`
		INSERT INTO user_profiles (user_id, settings_json)
		SELECT u.id, COALESCE(u.settings_json, ?)
		FROM users u
		LEFT JOIN user_profiles p ON p.user_id = u.id
		WHERE p.user_id IS NULL
	`, defaultSettingsJSON)
	return err
}

func (d *DB) CreateUser(email, passwordHash string) (*User, error) {
	now := time.Now().UTC()

	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO users (email, email_verified, password_hash, is_admin, plan, settings_json, created_at, updated_at)
		 VALUES (?, 0, ?, 0, 'free', ?, ?, ?)`,
		email, passwordHash, defaultSettingsJSON, now, now,
	)
	if err != nil {
		return nil, err
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(
		`INSERT INTO user_profiles (user_id, settings_json, display_name, created_at, updated_at)
		 VALUES (?, ?, '', ?, ?)`,
		userID, defaultSettingsJSON, now, now,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return d.GetUserByID(userID)
}

func (d *DB) GetUserByEmail(email string) (*User, error) {
	return d.getUserByQuery(`
		SELECT
			u.id,
			u.email,
			u.email_verified,
			u.password_hash,
			u.is_admin,
			u.plan,
			u.created_at,
			u.updated_at,
			COALESCE(p.settings_json, u.settings_json, ?) AS settings_json,
			COALESCE(p.display_name, '') AS display_name,
			COALESCE(p.created_at, u.created_at) AS profile_created_at,
			COALESCE(p.updated_at, u.updated_at) AS profile_updated_at
		FROM users u
		LEFT JOIN user_profiles p ON p.user_id = u.id
		WHERE u.email = ?
	`, defaultSettingsJSON, email)
}

func (d *DB) GetUserByID(userID int64) (*User, error) {
	return d.getUserByQuery(`
		SELECT
			u.id,
			u.email,
			u.email_verified,
			u.password_hash,
			u.is_admin,
			u.plan,
			u.created_at,
			u.updated_at,
			COALESCE(p.settings_json, u.settings_json, ?) AS settings_json,
			COALESCE(p.display_name, '') AS display_name,
			COALESCE(p.created_at, u.created_at) AS profile_created_at,
			COALESCE(p.updated_at, u.updated_at) AS profile_updated_at
		FROM users u
		LEFT JOIN user_profiles p ON p.user_id = u.id
		WHERE u.id = ?
	`, defaultSettingsJSON, userID)
}

func (d *DB) getUserByQuery(query string, args ...interface{}) (*User, error) {
	var (
		user         User
		settingsJSON string
		profile      UserProfile
		userCreated  sql.NullString
		userUpdated  sql.NullString
		profCreated  sql.NullString
		profUpdated  sql.NullString
	)

	err := d.db.QueryRow(query, args...).Scan(
		&user.ID,
		&user.Email,
		&user.EmailVerified,
		&user.PasswordHash,
		&user.IsAdmin,
		&user.Plan,
		&userCreated,
		&userUpdated,
		&settingsJSON,
		&profile.DisplayName,
		&profCreated,
		&profUpdated,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	profile.UserID = user.ID
	if err := json.Unmarshal([]byte(settingsJSON), &profile.Settings); err != nil {
		return nil, err
	}
	if user.CreatedAt, err = parseSQLiteTime(userCreated); err != nil {
		return nil, err
	}
	if user.UpdatedAt, err = parseSQLiteTime(userUpdated); err != nil {
		return nil, err
	}
	if profile.CreatedAt, err = parseSQLiteTime(profCreated); err != nil {
		return nil, err
	}
	if profile.UpdatedAt, err = parseSQLiteTime(profUpdated); err != nil {
		return nil, err
	}
	user.Profile = profile
	return &user, nil
}

func parseSQLiteTime(value sql.NullString) (time.Time, error) {
	if !value.Valid || value.String == "" {
		return time.Time{}, nil
	}

	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, value.String)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, errors.New("unable to parse sqlite time: " + value.String)
}

func (d *DB) CreateSession(userID int64, tokenHash string, expiresAt time.Time, ipAddress, userAgent string) error {
	now := time.Now().UTC()
	_, err := d.db.Exec(
		`INSERT INTO user_sessions (user_id, token_hash, created_at, expires_at, last_seen_at, ip_address, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, tokenHash, now, expiresAt.UTC(), now, ipAddress, userAgent,
	)
	return err
}

func (d *DB) GetUserBySessionTokenHash(tokenHash string) (*User, error) {
	var userID int64
	err := d.db.QueryRow(
		`SELECT user_id
		 FROM user_sessions
		 WHERE token_hash = ?
		   AND revoked_at IS NULL
		   AND expires_at > CURRENT_TIMESTAMP
		 ORDER BY id DESC
		 LIMIT 1`,
		tokenHash,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if _, err := d.db.Exec(`UPDATE user_sessions SET last_seen_at = ? WHERE token_hash = ?`, time.Now().UTC(), tokenHash); err != nil {
		return nil, err
	}

	return d.GetUserByID(userID)
}

func (d *DB) RevokeSessionByTokenHash(tokenHash string) error {
	_, err := d.db.Exec(`UPDATE user_sessions SET revoked_at = COALESCE(revoked_at, ?) WHERE token_hash = ?`, time.Now().UTC(), tokenHash)
	return err
}

func (d *DB) RevokeAllUserSessions(userID int64) error {
	_, err := d.db.Exec(`UPDATE user_sessions SET revoked_at = COALESCE(revoked_at, ?) WHERE user_id = ?`, time.Now().UTC(), userID)
	return err
}

func (d *DB) CreatePasswordResetToken(userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := d.db.Exec(
		`INSERT INTO password_reset_tokens (user_id, token_hash, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		userID, tokenHash, time.Now().UTC(), expiresAt.UTC(),
	)
	return err
}

func (d *DB) ConsumePasswordResetToken(tokenHash, passwordHash string) (int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var userID int64
	err = tx.QueryRow(
		`SELECT user_id
		 FROM password_reset_tokens
		 WHERE token_hash = ?
		   AND used_at IS NULL
		   AND expires_at > CURRENT_TIMESTAMP
		 LIMIT 1`,
		tokenHash,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, passwordHash, now, userID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE password_reset_tokens SET used_at = ? WHERE token_hash = ?`, now, tokenHash); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE user_sessions SET revoked_at = COALESCE(revoked_at, ?) WHERE user_id = ?`, now, userID); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return userID, nil
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
