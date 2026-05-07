package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	lemmatizer "finnestdb/pkg/lemmatizer-fi-et"
)

type DB struct {
	db *sql.DB

	// FST lemmatizer is loaded lazily on first FI BatchLookupForms call.
	// Tests that don't touch FI form lookups don't pay the embed-decode cost.
	lemOnce sync.Once
	lem     *lemmatizer.Lemmatizer
}

// finnishLemmatizer returns the (lazy-loaded) FST lemmatizer, or nil
// if loading failed. Callers must tolerate a nil result and fall back
// to the SQLite-only resolution chain.
func (d *DB) finnishLemmatizer() *lemmatizer.Lemmatizer {
	d.lemOnce.Do(func() {
		l, err := lemmatizer.New()
		if err != nil {
			log.Printf("store: lemmatizer init failed (FI FST disabled): %v", err)
			return
		}
		d.lem = l
	})
	return d.lem
}

type User struct {
	ID            int64
	Email         string
	EmailVerified bool
	IsAdmin       bool
	PasswordHash  string
	Settings      map[string]interface{}
}

// Session represents a server-side session record. The cookie holds the raw
// token; the database stores HashToken(token) only.
type Session struct {
	UserID    int64
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Deck struct {
	ID        int64
	UserID    int64
	Title     string
	Lang      string
	CreatedAt time.Time
}

type DeckStats struct {
	Deck
	Known  int
	Unique int
	Due    int
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

type ReviewCard struct {
	CardID       int64
	Lang         string
	Lemma        string
	POS          string
	Gloss        string
	SentenceText string
	SourceDeck   string
	DeckCounts   [][2]string
}

type ReviewSchedule struct {
	Streak int `json:"streak"`
	Step   int `json:"step"`
}

type DeckSentenceInput struct {
	Text   string
	Tokens []DeckTokenInput
}

type DeckTokenInput struct {
	TokenIx int
	// Form is the surface form of the token as it appeared in the source
	// text (e.g. "Eestit" for lemma "Eesti"). Stored on the occurrence row's
	// `surface` column so the deck detail view can highlight real inflections.
	Form  string
	Lemma string
	POS   string
}

type KnownLemma struct {
	Lemma string `json:"lemma"`
	POS   string `json:"pos"`
	Lang  string `json:"lang"`
}

type LemmaState struct {
	Lemma  string
	POS    string
	Status string
}

type ParseSession struct {
	ID          int64     `json:"id"`
	UserID      *int64    `json:"user_id,omitempty"`
	Lang        string    `json:"lang"`
	Parser      string    `json:"parser"`
	SourceText  string    `json:"source_text"`
	TotalTokens int       `json:"total_tokens"`
	UniqueWords int       `json:"unique_words"`
	CreatedAt   time.Time `json:"created_at"`
}

type ParseFeedback struct {
	ID                   int64      `json:"id"`
	ParseSessionID       int64      `json:"parse_session_id"`
	UserID               int64      `json:"user_id"`
	Lang                 string     `json:"lang"`
	Parser               string     `json:"parser"`
	Surface              string     `json:"surface"`
	Occurrence           int        `json:"occurrence"`
	OriginalLemma        string     `json:"original_lemma"`
	OriginalPOS          string     `json:"original_pos"`
	OriginalGrammarLabel string     `json:"original_grammar_label,omitempty"`
	ProposedLemma        string     `json:"proposed_lemma"`
	ProposedPOS          string     `json:"proposed_pos"`
	ProposedGrammarLabel string     `json:"proposed_grammar_label,omitempty"`
	Note                 string     `json:"note,omitempty"`
	Status               string     `json:"status"`
	ReviewNote           string     `json:"review_note,omitempty"`
	ReviewedByUserID     *int64     `json:"reviewed_by_user_id,omitempty"`
	ReviewedAt           *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
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
		-- Optional: if present, deck-detail "Suggest fix" can attribute feedback to
		-- the parse session that produced the deck.
		parse_session_id INTEGER,
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
		source TEXT NOT NULL DEFAULT '',
		source_priority INTEGER NOT NULL DEFAULT 0,
		paradigm_class TEXT,
		PRIMARY KEY(lemma, pos, lang)
	);

	-- One row per (token, lemma, pos). A single ambiguous surface form (e.g. ET
	-- "joon" = noun "line" or 1Sg of "jooma" / drink) writes multiple rows at
	-- the same (deck_id, sentence_id, token_ix). See EnsureMultiLemmaSchema.
	CREATE TABLE IF NOT EXISTS occurrence (
		deck_id INTEGER NOT NULL,
		sentence_id INTEGER NOT NULL,
		token_ix INTEGER NOT NULL,
		surface TEXT NOT NULL DEFAULT '',
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		FOREIGN KEY(deck_id) REFERENCES decks(id),
		FOREIGN KEY(sentence_id) REFERENCES sentences(id),
		UNIQUE(deck_id, sentence_id, token_ix, lemma, pos)
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
		introduced_at DATETIME,
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
	-- PRIMARY KEY (form, lang, lemma, pos) = a surface form may map to multiple
	-- (lemma, pos) pairs to model homonyms (e.g. ET "joon" = noun "line" or
	-- 1Sg of verb "jooma" / drink). See EnsureMultiLemmaSchema for the
	-- legacy (form, lang) PK migration.
	-- Finnish possessive forms (e.g. "kirjassani") are NOT imported here; they are handled
	-- at enrichment time via suffix stripping (see internal/store/dict.go).
	CREATE TABLE IF NOT EXISTS forms (
		form  TEXT NOT NULL,
		lemma TEXT NOT NULL,
		pos   TEXT NOT NULL,
		lang  TEXT NOT NULL,
		source TEXT NOT NULL DEFAULT '',
		source_priority INTEGER NOT NULL DEFAULT 0,
		feats TEXT,
		PRIMARY KEY (form, lang, lemma, pos)
	);

	-- translations and definitions schemas are owned by EnsureLexicalEntryTables
	-- below; both the server and the importer call it so the table set stays in
	-- one place.

	-- dict_metadata schema is owned by EnsureDictMetadataSchema below; both the
	-- server and the importer call it so the column set stays in one place.

	CREATE TABLE IF NOT EXISTS parse_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		lang TEXT NOT NULL,
		parser TEXT NOT NULL,
		source_text TEXT NOT NULL,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		unique_words INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS parse_feedback (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		parse_session_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		lang TEXT NOT NULL,
		parser TEXT NOT NULL,
		surface TEXT NOT NULL,
		occurrence INTEGER NOT NULL DEFAULT 0,
		original_lemma TEXT,
		original_pos TEXT,
		original_grammar_label TEXT,
		proposed_lemma TEXT NOT NULL,
		proposed_pos TEXT NOT NULL,
		proposed_grammar_label TEXT,
		note TEXT,
		status TEXT NOT NULL DEFAULT 'submitted',
		review_note TEXT,
		reviewed_by_user_id INTEGER,
		reviewed_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(parse_session_id) REFERENCES parse_sessions(id),
		FOREIGN KEY(user_id) REFERENCES users(id),
		FOREIGN KEY(reviewed_by_user_id) REFERENCES users(id)
	);
	`

	_, err := d.db.Exec(schema)
	if err != nil {
		return err
	}

	if _, err := d.db.Exec(`ALTER TABLE users ADD COLUMN is_admin INTEGER DEFAULT 0`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	if _, err := d.db.Exec(`ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	if _, err := d.db.Exec(`ALTER TABLE card_state ADD COLUMN introduced_at DATETIME`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}

	if _, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS user_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			revoked_at DATETIME,
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`); err != nil {
		return err
	}
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id)`); err != nil {
		return err
	}

	if err := d.ensureLangScopedKnownTable("user_known_lemmas"); err != nil {
		return err
	}
	if err := d.ensureLangScopedKnownTable("user_ignored_lemmas"); err != nil {
		return err
	}
	if err := d.ensureLangScopedCardsTable(); err != nil {
		return err
	}
	if _, err := d.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_cards_user_lang_lemma_pos_null_mwe ON cards(user_id, lang, lemma, pos) WHERE mwe_id IS NULL`); err != nil {
		return err
	}
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_lower_email ON users(lower(email))`); err != nil {
		return err
	}
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_card_state_introduced_at ON card_state(introduced_at) WHERE introduced_at IS NOT NULL`); err != nil {
		return err
	}
	if err := EnsureDictMetadataSchema(d.db); err != nil {
		return err
	}
	if err := EnsureDictionarySourceColumns(d.db); err != nil {
		return err
	}
	if err := EnsureLexicalEnrichmentColumns(d.db); err != nil {
		return err
	}
	if err := EnsureLexicalEntryTables(d.db); err != nil {
		return err
	}

	if err := EnsureMultiLemmaSchema(d.db); err != nil {
		return err
	}
	if err := EnsureOccurrenceSurfaceColumn(d.db); err != nil {
		return err
	}
	if err := EnsureDeckParseSessionColumn(d.db); err != nil {
		return err
	}

	return nil
}

func EnsureOccurrenceSurfaceColumn(db *sql.DB) error {
	// Backfill for older DB files (fresh DBs already include the column in CREATE TABLE).
	if _, err := db.Exec(`ALTER TABLE occurrence ADD COLUMN surface TEXT NOT NULL DEFAULT ''`); err != nil {
		// SQLite does not support IF NOT EXISTS for ADD COLUMN; treat "duplicate column"
		// as idempotent success.
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}

func EnsureDeckParseSessionColumn(db *sql.DB) error {
	// Backfill for older DB files (fresh DBs already include the column in CREATE TABLE).
	if _, err := db.Exec(`ALTER TABLE decks ADD COLUMN parse_session_id INTEGER`); err != nil {
		// SQLite does not support IF NOT EXISTS for ADD COLUMN; treat "duplicate column"
		// as idempotent success.
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}

// EnsureDictMetadataSchema is the single source of truth for the dict_metadata
// table. It creates the table on a fresh DB and backfills missing columns on
// older DB files. Both the server (internal/store) and the standalone importer
// (cmd/importdict) call it so the schema cannot drift between the two paths.
func EnsureDictMetadataSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS dict_metadata (
			lang        TEXT NOT NULL,
			source      TEXT NOT NULL,
			source_name TEXT,
			source_url  TEXT,
			source_version TEXT,
			license     TEXT,
			attribution TEXT,
			changes_note TEXT,
			imported_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			row_count   INTEGER,
			PRIMARY KEY (lang, source)
		);
	`); err != nil {
		return err
	}
	columns := []string{
		"source_name TEXT",
		"source_url TEXT",
		"source_version TEXT",
		"license TEXT",
		"attribution TEXT",
		"changes_note TEXT",
	}
	for _, column := range columns {
		if _, err := db.Exec(`ALTER TABLE dict_metadata ADD COLUMN ` + column); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}

// EnsureDictionarySourceColumns backfills row-level source-tracking columns on
// existing DBs. Fresh DBs already get these columns via the lemmas/forms
// CREATE TABLE statements above; this exists for upgrade compatibility. Both
// the server (internal/store) and the standalone importer (cmd/importdict)
// call it so the column set stays in one place.
func EnsureDictionarySourceColumns(db *sql.DB) error {
	for table, columns := range map[string][]string{
		"lemmas": {
			"source TEXT NOT NULL DEFAULT ''",
			"source_priority INTEGER NOT NULL DEFAULT 0",
		},
		"forms": {
			"source TEXT NOT NULL DEFAULT ''",
			"source_priority INTEGER NOT NULL DEFAULT 0",
		},
	} {
		for _, column := range columns {
			if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
				return err
			}
		}
	}
	return nil
}

// EnsureLexicalEnrichmentColumns backfills paradigm_class on lemmas (the join
// key from a paradigm-class adapter like Kotus to a paradigm generator like
// Voikko) and feats on forms (UD-style morph features as JSON). Fresh DBs
// already get these columns via the CREATE TABLE statements above; this
// exists for upgrade compatibility, mirroring EnsureDictionarySourceColumns.
func EnsureLexicalEnrichmentColumns(db *sql.DB) error {
	for table, columns := range map[string][]string{
		"lemmas": {"paradigm_class TEXT"},
		"forms":  {"feats TEXT"},
	} {
		for _, column := range columns {
			if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
				return err
			}
		}
	}
	return nil
}

// EnsureLexicalEntryTables creates the translations and definitions tables if
// they don't yet exist. Fresh DBs already get them via the CREATE TABLE
// statements above; this exists for upgrade compatibility on databases
// created before these tables were introduced.
func EnsureLexicalEntryTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS translations (
			lemma       TEXT NOT NULL,
			pos         TEXT NOT NULL,
			lang        TEXT NOT NULL,
			target_lang TEXT NOT NULL,
			text        TEXT NOT NULL,
			sense_idx   INTEGER NOT NULL DEFAULT 0,
			source      TEXT NOT NULL,
			PRIMARY KEY (lemma, pos, lang, target_lang, sense_idx, source)
		);
		CREATE TABLE IF NOT EXISTS definitions (
			lemma     TEXT NOT NULL,
			pos       TEXT NOT NULL,
			lang      TEXT NOT NULL,
			sense_idx INTEGER NOT NULL DEFAULT 0,
			text      TEXT NOT NULL,
			source    TEXT NOT NULL,
			PRIMARY KEY (lemma, pos, lang, sense_idx, source)
		);
	`)
	return err
}

// EnsureMultiLemmaSchema rebuilds the forms and occurrence tables to allow
// multiple (lemma, pos) rows per surface form / per token. This models
// homonyms — e.g. ET "joon" is both the noun "line" (SgN of joon) and the
// 1Sg of the verb "jooma" / drink — so a single occurrence of "joon" in a
// sentence creates one row per candidate, and "joon + 1, jooma + 1" both
// hold for deck stats.
//
// Applies idempotently: detects whether the migration has already run by
// checking the table's CREATE statement in sqlite_master. The rebuild
// preserves whatever columns the existing table has (so it survives the
// later addition of source / source_priority columns by
// EnsureDictionarySourceColumns on a rebased main).
//
// Pre-migration:
//
//	forms      PRIMARY KEY (form, lang)
//	occurrence UNIQUE      (deck_id, sentence_id, token_ix)
//
// Post-migration:
//
//	forms      PRIMARY KEY (form, lang, lemma, pos)
//	occurrence UNIQUE      (deck_id, sentence_id, token_ix, lemma, pos)
//
// Both new keys are strict supersets, so existing rows always satisfy the
// new constraint.
func EnsureMultiLemmaSchema(db *sql.DB) error {
	if err := rebuildIfLegacyKey(db, "forms",
		"PRIMARY KEY (form, lang, lemma, pos)"); err != nil {
		return fmt.Errorf("forms multi-lemma migration: %w", err)
	}
	if err := rebuildIfLegacyKey(db, "occurrence",
		"UNIQUE(deck_id, sentence_id, token_ix, lemma, pos)"); err != nil {
		return fmt.Errorf("occurrence multi-lemma migration: %w", err)
	}
	return nil
}

// rebuildIfLegacyKey rebuilds the named table with the supplied key clause if
// the current sqlite_master CREATE statement doesn't already contain it.
// The rebuild copies whatever columns the legacy table had so this survives
// the later addition of source / source_priority by
// EnsureDictionarySourceColumns on a rebased main.
func rebuildIfLegacyKey(db *sql.DB, table, keyClause string) error {
	var createSQL string
	err := db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`,
		table,
	).Scan(&createSQL)
	if err == sql.ErrNoRows {
		// Table doesn't exist yet — initSchema's CREATE handles fresh DBs.
		return nil
	}
	if err != nil {
		return err
	}
	if strings.Contains(createSQL, keyClause) {
		return nil // already migrated
	}

	cols, err := tableColumns(db, table)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	colDefs := make([]string, 0, len(cols))
	colNames := make([]string, 0, len(cols))
	for _, c := range cols {
		def := c.Name + " " + c.Type
		if c.NotNull {
			def += " NOT NULL"
		}
		if c.DfltValue.Valid {
			def += " DEFAULT " + c.DfltValue.String
		}
		colDefs = append(colDefs, def)
		colNames = append(colNames, c.Name)
	}

	createNew := fmt.Sprintf(
		`CREATE TABLE %s_new (%s, %s)`,
		table, strings.Join(colDefs, ", "), keyClause,
	)
	if _, err := tx.Exec(createNew); err != nil {
		return fmt.Errorf("create %s_new: %w", table, err)
	}

	colList := strings.Join(colNames, ", ")
	copySQL := fmt.Sprintf(
		`INSERT INTO %s_new (%s) SELECT %s FROM %s`,
		table, colList, colList, table,
	)
	if _, err := tx.Exec(copySQL); err != nil {
		return fmt.Errorf("copy %s rows: %w", table, err)
	}

	if _, err := tx.Exec(`DROP TABLE ` + table); err != nil {
		return fmt.Errorf("drop %s: %w", table, err)
	}
	if _, err := tx.Exec(fmt.Sprintf(`ALTER TABLE %s_new RENAME TO %s`, table, table)); err != nil {
		return fmt.Errorf("rename %s_new: %w", table, err)
	}

	return tx.Commit()
}

type sqliteColumn struct {
	Name      string
	Type      string
	NotNull   bool
	DfltValue sql.NullString
}

func tableColumns(db *sql.DB, table string) ([]sqliteColumn, error) {
	rows, err := db.Query(`SELECT name, type, "notnull", dflt_value FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sqliteColumn
	for rows.Next() {
		var c sqliteColumn
		var nn int
		if err := rows.Scan(&c.Name, &c.Type, &nn, &c.DfltValue); err != nil {
			return nil, err
		}
		c.NotNull = nn != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetOrCreateUser is the legacy seeding helper used by tests and store
// internals when password-based auth is irrelevant. The HTTP auth path uses
// CreateUser + GetUserByEmail instead. is_admin is bootstrapped from
// FINNESTDB_ADMIN_EMAILS only on first creation; existing users are not
// re-evaluated, so admin status set via the API is preserved.
func (d *DB) GetOrCreateUser(email string) (*User, error) {
	email = normalizeEmail(email)
	user, err := d.GetUserByEmail(email)
	if err == nil {
		return user, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	return d.createUser(email, "", isAdminEmail(email))
}

// CreateUser inserts a new user with the given email and password hash.
// Returns an error if the email is already taken. is_admin is bootstrapped
// from FINNESTDB_ADMIN_EMAILS at creation time only.
func (d *DB) CreateUser(email, passwordHash string) (*User, error) {
	email = normalizeEmail(email)
	return d.createUser(email, passwordHash, isAdminEmail(email))
}

func (d *DB) createUser(email, passwordHash string, isAdmin bool) (*User, error) {
	settingsJSON := `{"new_per_day":20,"retention":0.9,"theme":"system"}`
	result, err := d.db.Exec(
		`INSERT INTO users (email, email_verified, is_admin, password_hash, settings_json)
		 VALUES (?, 1, ?, ?, ?)`,
		email, boolToInt(isAdmin), passwordHash, settingsJSON,
	)
	if err != nil {
		return nil, err
	}
	user := &User{
		Email:         email,
		EmailVerified: true,
		IsAdmin:       isAdmin,
		PasswordHash:  passwordHash,
	}
	user.ID, _ = result.LastInsertId()
	json.Unmarshal([]byte(settingsJSON), &user.Settings)
	return user, nil
}

func (d *DB) GetUserByID(userID int64) (*User, error) {
	var user User
	var settingsJSON string

	err := d.db.QueryRow(
		`SELECT id, email, email_verified, is_admin, password_hash, settings_json
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&user.ID, &user.Email, &user.EmailVerified, &user.IsAdmin, &user.PasswordHash, &settingsJSON)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(settingsJSON), &user.Settings)
	return &user, nil
}

// GetUserByEmail returns the user with the given email (case-insensitive),
// or sql.ErrNoRows if no such user exists.
func (d *DB) GetUserByEmail(email string) (*User, error) {
	email = normalizeEmail(email)
	var user User
	var settingsJSON string

	err := d.db.QueryRow(
		`SELECT id, email, email_verified, is_admin, password_hash, settings_json
		 FROM users WHERE lower(email) = ?`,
		email,
	).Scan(&user.ID, &user.Email, &user.EmailVerified, &user.IsAdmin, &user.PasswordHash, &settingsJSON)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(settingsJSON), &user.Settings)
	return &user, nil
}

// SetUserPassword writes a new password_hash for the given user.
func (d *DB) SetUserPassword(userID int64, passwordHash string) error {
	_, err := d.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	return err
}

// CreateSession persists a session row keyed on tokenHash with the given
// expiry. The raw token must NOT be passed in.
func (d *DB) CreateSession(userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := d.db.Exec(
		`INSERT INTO user_sessions (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID, tokenHash, expiresAt.UTC(),
	)
	return err
}

// GetUserBySessionTokenHash looks up the active session by token hash and
// returns the owning user. Returns (nil, nil) when no active session exists
// (missing, expired, or revoked). When a session is matched, expires_at is
// extended to (now + slidingWindow) before returning — this implements the
// rolling-expiry behavior.
func (d *DB) GetUserBySessionTokenHash(tokenHash string, slidingWindow time.Duration) (*User, error) {
	var userID int64
	now := time.Now().UTC()
	err := d.db.QueryRow(
		`SELECT user_id FROM user_sessions
		 WHERE token_hash = ?
		   AND revoked_at IS NULL
		   AND expires_at > ?`,
		tokenHash, now,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if slidingWindow > 0 {
		newExpiry := now.Add(slidingWindow)
		if _, err := d.db.Exec(
			`UPDATE user_sessions SET expires_at = ? WHERE token_hash = ?`,
			newExpiry, tokenHash,
		); err != nil {
			return nil, err
		}
	}
	return d.GetUserByID(userID)
}

// RevokeSessionByTokenHash marks the matching session row as revoked. Idempotent.
func (d *DB) RevokeSessionByTokenHash(tokenHash string) error {
	_, err := d.db.Exec(
		`UPDATE user_sessions SET revoked_at = CURRENT_TIMESTAMP
		 WHERE token_hash = ? AND revoked_at IS NULL`,
		tokenHash,
	)
	return err
}

// ListUsers returns every user, ordered by id. Used by the admin user
// management page.
func (d *DB) ListUsers() ([]User, error) {
	rows, err := d.db.Query(
		`SELECT id, email, email_verified, is_admin, password_hash, settings_json
		 FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var settingsJSON string
		if err := rows.Scan(&u.ID, &u.Email, &u.EmailVerified, &u.IsAdmin, &u.PasswordHash, &settingsJSON); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(settingsJSON), &u.Settings)
		users = append(users, u)
	}
	return users, rows.Err()
}

// SetUserAdmin updates is_admin for the given user.
func (d *DB) SetUserAdmin(userID int64, isAdmin bool) error {
	_, err := d.db.Exec(`UPDATE users SET is_admin = ? WHERE id = ?`, boolToInt(isAdmin), userID)
	return err
}

type sqlReadWriter interface {
	sqlQueryRower
	Exec(query string, args ...any) (sql.Result, error)
}

type sqlQueryRower interface {
	QueryRow(query string, args ...any) *sql.Row
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

func (d *DB) GetUserDeckStats(userID int64) ([]DeckStats, error) {
	rows, err := d.db.Query(
		`SELECT d.id, d.user_id, d.title, d.lang, d.created_at,
		        COUNT(DISTINCT o.lemma || char(31) || o.pos) AS unique_count,
		        COUNT(DISTINCT CASE
		            WHEN uk.lemma IS NOT NULL THEN o.lemma || char(31) || o.pos
		            ELSE NULL
		        END) AS known_count,
		        COUNT(DISTINCT CASE
		            WHEN uk.lemma IS NULL
		             AND ui.lemma IS NULL
		             AND c.id IS NOT NULL
		             AND (cs.next_due IS NULL OR cs.next_due <= CURRENT_TIMESTAMP)
		            THEN o.lemma || char(31) || o.pos
		            ELSE NULL
		        END) AS due_count
		   FROM decks d
		   LEFT JOIN occurrence o
		          ON o.deck_id = d.id
		   LEFT JOIN user_known_lemmas uk
		          ON uk.user_id = d.user_id
		         AND uk.lang = d.lang
		         AND uk.lemma = o.lemma
		         AND uk.pos = o.pos
		   LEFT JOIN user_ignored_lemmas ui
		          ON ui.user_id = d.user_id
		         AND ui.lang = d.lang
		         AND ui.lemma = o.lemma
		         AND ui.pos = o.pos
		   LEFT JOIN cards c
		          ON c.user_id = d.user_id
		         AND c.lang = d.lang
		         AND c.lemma = o.lemma
		         AND c.pos = o.pos
		         AND c.mwe_id IS NULL
		   LEFT JOIN card_state cs
		          ON cs.card_id = c.id
		  WHERE d.user_id = ?
		  GROUP BY d.id, d.user_id, d.title, d.lang, d.created_at
		  ORDER BY d.created_at DESC, d.id DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := []DeckStats{}
	for rows.Next() {
		var item DeckStats
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.Title,
			&item.Lang,
			&item.CreatedAt,
			&item.Unique,
			&item.Known,
			&item.Due,
		); err != nil {
			return nil, err
		}
		stats = append(stats, item)
	}
	return stats, rows.Err()
}

func createDeck(q sqlReadWriter, userID int64, title, lang string) (int64, error) {
	result, err := q.Exec(
		"INSERT INTO decks (user_id, title, lang) VALUES (?, ?, ?)",
		userID, title, lang,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func createSentence(q sqlReadWriter, deckID int64, text, lang string) (int64, error) {
	result, err := q.Exec(
		"INSERT INTO sentences (deck_id, text, lang) VALUES (?, ?, ?)",
		deckID, text, lang,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func createOccurrence(q sqlReadWriter, deckID, sentenceID int64, tokenIx int, surface, lemma, pos string) error {
	_, err := q.Exec(
		`INSERT OR IGNORE INTO occurrence (deck_id, sentence_id, token_ix, surface, lemma, pos)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		deckID, sentenceID, tokenIx, surface, lemma, pos,
	)
	return err
}

func ensureCard(q sqlReadWriter, userID int64, lang, lemma, pos string) (int64, error) {
	if _, err := q.Exec(
		`INSERT OR IGNORE INTO cards (user_id, lang, lemma, pos, mwe_id) VALUES (?, ?, ?, ?, NULL)`,
		userID, lang, lemma, pos,
	); err != nil {
		return 0, err
	}

	var cardID int64
	if err := q.QueryRow(
		`SELECT id FROM cards WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ? AND mwe_id IS NULL ORDER BY id LIMIT 1`,
		userID, lang, lemma, pos,
	).Scan(&cardID); err != nil {
		return 0, err
	}
	if _, err := q.Exec(
		`INSERT OR IGNORE INTO card_state (card_id, fsrs_json, next_due, last_answer_at) VALUES (?, NULL, NULL, NULL)`,
		cardID,
	); err != nil {
		return 0, err
	}

	return cardID, nil
}

func isKnownOrIgnored(q sqlReadWriter, userID int64, lang, lemma, pos string) (bool, error) {
	var count int
	err := q.QueryRow(
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

func (d *DB) CreateDeckWithSentences(userID int64, title, lang string, sentences []DeckSentenceInput) (int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	type cardKey struct {
		lemma string
		pos   string
	}

	cardKeys := make(map[cardKey]struct{})
	for _, sentence := range sentences {
		for _, token := range sentence.Tokens {
			if token.Lemma == "" || token.POS == "" {
				continue
			}
			cardKeys[cardKey{lemma: token.Lemma, pos: token.POS}] = struct{}{}
		}
	}

	for key := range cardKeys {
		knownOrIgnored, err := isKnownOrIgnored(tx, userID, lang, key.lemma, key.pos)
		if err != nil {
			return 0, err
		}
		if knownOrIgnored {
			continue
		}
		if _, err := ensureCard(tx, userID, lang, key.lemma, key.pos); err != nil {
			return 0, err
		}
	}

	deckID, err := createDeck(tx, userID, title, lang)
	if err != nil {
		return 0, err
	}

	for _, sentence := range sentences {
		if sentence.Text == "" {
			continue
		}
		sentenceID, err := createSentence(tx, deckID, sentence.Text, lang)
		if err != nil {
			return 0, err
		}
		for _, token := range sentence.Tokens {
			if token.Lemma == "" || token.POS == "" {
				continue
			}
			if err := createOccurrence(tx, deckID, sentenceID, token.TokenIx, token.Form, token.Lemma, token.POS); err != nil {
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deckID, nil
}

func (d *DB) UserOwnsDeck(userID, deckID int64) (bool, error) {
	var exists int
	err := d.db.QueryRow(
		`SELECT 1 FROM decks WHERE id = ? AND user_id = ?`,
		deckID, userID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (d *DB) UpdateDeckTitle(userID, deckID int64, title string) error {
	result, err := d.db.Exec(
		`UPDATE decks SET title = ? WHERE id = ? AND user_id = ?`,
		title, deckID, userID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) DeleteDeck(userID, deckID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM occurrence WHERE deck_id = ? AND deck_id IN (SELECT id FROM decks WHERE id = ? AND user_id = ?)`, deckID, deckID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sentences WHERE deck_id = ? AND deck_id IN (SELECT id FROM decks WHERE id = ? AND user_id = ?)`, deckID, deckID, userID); err != nil {
		return err
	}
	deckResult, err := tx.Exec(`DELETE FROM decks WHERE id = ? AND user_id = ?`, deckID, userID)
	if err != nil {
		return err
	}
	rowsAffected, err := deckResult.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

type DeckLemma struct {
	Lemma           string
	POS             string
	Forms           []string
	Count           int
	Gloss           string
	ExampleSentence string
}

type DeckDetails struct {
	Deck
	ParseSessionID *int64
	TotalTokens    int
	Lemmas         []DeckLemma
}

// GetDeckDetails returns the deck's metadata and an aggregated list of
// (lemma, pos) entries with counts, glosses, and a representative example
// sentence. Returns sql.ErrNoRows if the deck does not exist or is not
// owned by userID.
func (d *DB) GetDeckDetails(userID, deckID int64) (*DeckDetails, error) {
	var details DeckDetails
	var parseSessionID sql.NullInt64
	err := d.db.QueryRow(
		`SELECT id, user_id, title, lang, parse_session_id, created_at
		   FROM decks WHERE id = ? AND user_id = ?`,
		deckID, userID,
	).Scan(&details.ID, &details.UserID, &details.Title, &details.Lang, &parseSessionID, &details.CreatedAt)
	if err != nil {
		return nil, err
	}
	if parseSessionID.Valid {
		details.ParseSessionID = &parseSessionID.Int64
	}

	rows, err := d.db.Query(
		`SELECT o.lemma,
		        o.pos,
		        COALESCE(GROUP_CONCAT(DISTINCT NULLIF(o.surface, '')), '') AS forms,
		        COUNT(*) AS cnt,
		        COALESCE(l.gloss, '') AS gloss,
		        COALESCE((
		            SELECT s.text
		              FROM occurrence o2
		              JOIN sentences s ON s.id = o2.sentence_id
		             WHERE o2.deck_id = o.deck_id
		               AND o2.lemma   = o.lemma
		               AND o2.pos     = o.pos
		             ORDER BY o2.sentence_id, o2.token_ix
		             LIMIT 1
		        ), '') AS example
		   FROM occurrence o
		   LEFT JOIN lemmas l
		          ON l.lemma = o.lemma
		         AND l.pos   = o.pos
		         AND l.lang  = ?
		  WHERE o.deck_id = ?
		  GROUP BY o.lemma, o.pos, l.gloss
		  ORDER BY cnt DESC, o.lemma`,
		details.Lang, deckID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	details.Lemmas = []DeckLemma{}
	idx := map[LemmaKey]int{}
	for rows.Next() {
		var item DeckLemma
		var formsCSV string
		if err := rows.Scan(&item.Lemma, &item.POS, &formsCSV, &item.Count, &item.Gloss, &item.ExampleSentence); err != nil {
			return nil, err
		}
		if formsCSV != "" {
			item.Forms = strings.Split(formsCSV, ",")
			sort.Strings(item.Forms)
		}
		details.TotalTokens += item.Count
		idx[LemmaKey{Lemma: item.Lemma, POS: item.POS}] = len(details.Lemmas)
		details.Lemmas = append(details.Lemmas, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &details, nil
}

func (d *DB) SetDeckParseSession(userID, deckID, parseSessionID int64) error {
	res, err := d.db.Exec(
		`UPDATE decks SET parse_session_id = ? WHERE id = ? AND user_id = ?`,
		parseSessionID, deckID, userID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CreateOccurrence records that a lemma+pos token appeared at position tokenIx
// within the given sentence (which belongs to the given deck).
func (d *DB) CreateOccurrence(deckID, sentenceID int64, tokenIx int, lemma, pos string) error {
	return createOccurrence(d.db, deckID, sentenceID, tokenIx, "", lemma, pos)
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

func (d *DB) BatchLemmaStates(userID int64, lang string, lemmas []LemmaKey) (map[LemmaKey]string, error) {
	result := make(map[LemmaKey]string, len(lemmas))
	if len(lemmas) == 0 {
		return result, nil
	}

	seen := make(map[LemmaKey]struct{}, len(lemmas))
	unique := make([]LemmaKey, 0, len(lemmas))
	for _, lemma := range lemmas {
		if lemma.Lemma == "" || lemma.POS == "" {
			continue
		}
		if _, ok := seen[lemma]; ok {
			continue
		}
		seen[lemma] = struct{}{}
		unique = append(unique, lemma)
	}
	if len(unique) == 0 {
		return result, nil
	}

	stmt, err := d.db.Prepare(`
		SELECT
			EXISTS(SELECT 1 FROM user_known_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?),
			EXISTS(SELECT 1 FROM user_ignored_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?)
	`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	for _, lemma := range unique {
		var known, ignored bool
		if err := stmt.QueryRow(
			userID, lang, lemma.Lemma, lemma.POS,
			userID, lang, lemma.Lemma, lemma.POS,
		).Scan(&known, &ignored); err != nil {
			return nil, err
		}
		switch {
		case known:
			result[lemma] = "known"
		case ignored:
			result[lemma] = "ignored"
		}
	}

	return result, nil
}

func (d *DB) MarkLemmaKnown(userID int64, lang, lemma, pos string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO user_known_lemmas (user_id, lang, lemma, pos) VALUES (?, ?, ?, ?)`,
		userID, lang, lemma, pos,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM user_ignored_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?`,
		userID, lang, lemma, pos,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) MarkLemmaIgnored(userID int64, lang, lemma, pos string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO user_ignored_lemmas (user_id, lang, lemma, pos) VALUES (?, ?, ?, ?)`,
		userID, lang, lemma, pos,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM user_known_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?`,
		userID, lang, lemma, pos,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearLemmaState removes the given (lemma, pos) from both the known and
// ignored lists for a user, returning the lemma to the neutral / unknown
// state. If a deck containing this lemma was created while the lemma was
// known/ignored, CreateDeckWithSentences would have skipped seeding a card
// row — so we ensure one here so the lemma is reachable from the review
// queue once the user has marked it unknown again.
func (d *DB) ClearLemmaState(userID int64, lang, lemma, pos string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM user_known_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?`,
		userID, lang, lemma, pos,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM user_ignored_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?`,
		userID, lang, lemma, pos,
	); err != nil {
		return err
	}
	if _, err := ensureCard(tx, userID, lang, lemma, pos); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) IsKnownOrIgnored(userID int64, lang, lemma, pos string) (bool, error) {
	return isKnownOrIgnored(d.db, userID, lang, lemma, pos)
}

func (d *DB) EnsureCard(userID int64, lang, lemma, pos string) (int64, error) {
	return ensureCard(d.db, userID, lang, lemma, pos)
}

func (d *DB) CountCards(userID int64, lang string) (int, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM cards WHERE user_id = ? AND lang = ?`,
		userID, lang,
	).Scan(&count)
	return count, err
}

func (d *DB) CountKnownLemmas(userID int64) (int, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM user_known_lemmas WHERE user_id = ?`,
		userID,
	).Scan(&count)
	return count, err
}

func (d *DB) CountDueCards(userID int64) (int, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*)
		   FROM cards c
		   JOIN card_state cs ON cs.card_id = c.id
		  WHERE c.user_id = ?
		    AND c.mwe_id IS NULL
		    AND NOT EXISTS (
		        SELECT 1 FROM user_known_lemmas uk
		         WHERE uk.user_id = c.user_id AND uk.lang = c.lang AND uk.lemma = c.lemma AND uk.pos = c.pos
		    )
		    AND NOT EXISTS (
		        SELECT 1 FROM user_ignored_lemmas ui
		         WHERE ui.user_id = c.user_id AND ui.lang = c.lang AND ui.lemma = c.lemma AND ui.pos = c.pos
		    )
		    AND (cs.next_due IS NULL OR cs.next_due <= CURRENT_TIMESTAMP)`,
		userID,
	).Scan(&count)
	return count, err
}

func (d *DB) CountNewCards(userID int64) (int, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*)
		   FROM cards c
		   JOIN card_state cs ON cs.card_id = c.id
		  WHERE c.user_id = ?
		    AND c.mwe_id IS NULL
		    AND cs.last_answer_at IS NULL
		    AND NOT EXISTS (
		        SELECT 1 FROM user_known_lemmas uk
		         WHERE uk.user_id = c.user_id AND uk.lang = c.lang AND uk.lemma = c.lemma AND uk.pos = c.pos
		    )
		    AND NOT EXISTS (
		        SELECT 1 FROM user_ignored_lemmas ui
		         WHERE ui.user_id = c.user_id AND ui.lang = c.lang AND ui.lemma = c.lemma AND ui.pos = c.pos
		    )`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	remaining, err := d.remainingNewCardsToday(userID)
	if err != nil {
		return 0, err
	}
	if count > remaining {
		count = remaining
	}
	return count, nil
}

func (d *DB) GetNextReviewCard(userID int64, deckID *int64) (*ReviewCard, error) {
	deckFilter := ""
	deckOccurrenceFilter := ""
	newCardFilter := ""
	if deckID != nil {
		deckFilter = ` AND EXISTS (
			SELECT 1 FROM occurrence o
			WHERE o.deck_id = ? AND o.lemma = c.lemma AND o.pos = c.pos
		)`
		deckOccurrenceFilter = ` AND o.deck_id = ?`
	}
	remainingNewCards, err := d.remainingNewCardsToday(userID)
	if err != nil {
		return nil, err
	}
	if remainingNewCards == 0 {
		newCardFilter = ` AND cs.last_answer_at IS NOT NULL`
	}

	query := `SELECT c.id, c.lang, c.lemma, c.pos, COALESCE(l.gloss, ''),
	                 COALESCE((
	                     SELECT s.text
	                       FROM occurrence o
	                       JOIN sentences s ON s.id = o.sentence_id
	                       JOIN decks d ON d.id = o.deck_id
	                      WHERE d.user_id = c.user_id
	                        AND d.lang = c.lang
	                        AND o.lemma = c.lemma AND o.pos = c.pos` + deckOccurrenceFilter + `
	                      ORDER BY s.id ASC
	                      LIMIT 1
	                 ), ''),
	                 COALESCE((
	                     SELECT d.title
	                       FROM occurrence o
	                       JOIN decks d ON d.id = o.deck_id
	                      WHERE d.user_id = c.user_id
	                        AND d.lang = c.lang
	                        AND o.lemma = c.lemma
	                        AND o.pos = c.pos` + deckOccurrenceFilter + `
	                      ORDER BY d.created_at DESC, d.id DESC
	                      LIMIT 1
	                 ), '')
	            FROM cards c
	            JOIN card_state cs ON cs.card_id = c.id
	       LEFT JOIN lemmas l
	              ON l.lang = c.lang AND l.lemma = c.lemma AND l.pos = c.pos
	           WHERE c.user_id = ?
	             AND c.mwe_id IS NULL
	             AND NOT EXISTS (
	                 SELECT 1 FROM user_known_lemmas uk
	                  WHERE uk.user_id = c.user_id AND uk.lang = c.lang AND uk.lemma = c.lemma AND uk.pos = c.pos
	             )
	             AND NOT EXISTS (
	                 SELECT 1 FROM user_ignored_lemmas ui
	                  WHERE ui.user_id = c.user_id AND ui.lang = c.lang AND ui.lemma = c.lemma AND ui.pos = c.pos
	             )
	             AND (cs.next_due IS NULL OR cs.next_due <= CURRENT_TIMESTAMP)` + newCardFilter + deckFilter + `
	        ORDER BY CASE WHEN cs.last_answer_at IS NULL THEN 0 ELSE 1 END,
	                 COALESCE(cs.next_due, '1970-01-01 00:00:00') ASC,
	                 c.id ASC
	           LIMIT 1`

	queryArgs := []any{userID}
	if deckID != nil {
		queryArgs = []any{*deckID, *deckID, userID, *deckID}
	}

	var card ReviewCard
	err = d.db.QueryRow(query, queryArgs...).Scan(
		&card.CardID,
		&card.Lang,
		&card.Lemma,
		&card.POS,
		&card.Gloss,
		&card.SentenceText,
		&card.SourceDeck,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	countRows, err := d.db.Query(
		`SELECT d.title, COUNT(*)
		   FROM occurrence o
		   JOIN decks d ON d.id = o.deck_id
		  WHERE d.user_id = ?
		    AND d.lang = ?
		    AND o.lemma = ?
		    AND o.pos = ?
		  GROUP BY d.id, d.title
		  ORDER BY d.created_at DESC, d.id DESC`,
		userID, card.Lang, card.Lemma, card.POS,
	)
	if err != nil {
		return nil, err
	}
	defer countRows.Close()

	for countRows.Next() {
		var title string
		var count int
		if err := countRows.Scan(&title, &count); err != nil {
			return nil, err
		}
		card.DeckCounts = append(card.DeckCounts, [2]string{title, fmt.Sprintf("%d", count)})
	}
	if err := countRows.Err(); err != nil {
		return nil, err
	}

	return &card, nil
}

func (d *DB) RecordReviewAnswer(userID, cardID int64, rating string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	card, schedule, err := getOwnedCardSchedule(tx, userID, cardID)
	if err != nil {
		return err
	}
	if card == nil {
		return sql.ErrNoRows
	}

	nextDue, updated := nextScheduleForRating(schedule, rating)
	payload, err := json.Marshal(updated)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(
		`UPDATE card_state
		    SET fsrs_json = ?,
		        next_due = ?,
		        introduced_at = CASE
		            WHEN introduced_at IS NULL AND last_answer_at IS NULL THEN CURRENT_TIMESTAMP
		            ELSE introduced_at
		        END,
		        last_answer_at = CURRENT_TIMESTAMP
		  WHERE card_id = ?`,
		string(payload),
		sqliteTime(nextDue),
		cardID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) MarkCardKnown(userID, cardID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	card, _, err := getOwnedCardSchedule(tx, userID, cardID)
	if err != nil {
		return err
	}
	if card == nil {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO user_known_lemmas (user_id, lang, lemma, pos) VALUES (?, ?, ?, ?)`,
		userID, card.Lang, card.Lemma, card.POS,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM user_ignored_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?`,
		userID, card.Lang, card.Lemma, card.POS,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) MarkCardIgnored(userID, cardID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	card, _, err := getOwnedCardSchedule(tx, userID, cardID)
	if err != nil {
		return err
	}
	if card == nil {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO user_ignored_lemmas (user_id, lang, lemma, pos) VALUES (?, ?, ?, ?)`,
		userID, card.Lang, card.Lemma, card.POS,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM user_known_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?`,
		userID, card.Lang, card.Lemma, card.POS,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) CreateParseSession(userID *int64, lang, parser, sourceText string, totalTokens, uniqueWords int) (int64, error) {
	result, err := d.db.Exec(
		`INSERT INTO parse_sessions (user_id, lang, parser, source_text, total_tokens, unique_words)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, lang, parser, sourceText, totalTokens, uniqueWords,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) GetParseSession(parseSessionID int64) (*ParseSession, error) {
	var session ParseSession
	var userID sql.NullInt64
	err := d.db.QueryRow(
		`SELECT id, user_id, lang, parser, source_text, total_tokens, unique_words, created_at
		 FROM parse_sessions WHERE id = ?`,
		parseSessionID,
	).Scan(
		&session.ID,
		&userID,
		&session.Lang,
		&session.Parser,
		&session.SourceText,
		&session.TotalTokens,
		&session.UniqueWords,
		&session.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		session.UserID = &userID.Int64
	}
	return &session, nil
}

func (d *DB) CreateParseFeedback(feedback ParseFeedback) (int64, error) {
	result, err := d.db.Exec(
		`INSERT INTO parse_feedback (
			parse_session_id, user_id, lang, parser, surface, occurrence,
			original_lemma, original_pos, original_grammar_label,
			proposed_lemma, proposed_pos, proposed_grammar_label, note, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'submitted')`,
		feedback.ParseSessionID,
		feedback.UserID,
		feedback.Lang,
		feedback.Parser,
		feedback.Surface,
		feedback.Occurrence,
		feedback.OriginalLemma,
		feedback.OriginalPOS,
		feedback.OriginalGrammarLabel,
		feedback.ProposedLemma,
		feedback.ProposedPOS,
		feedback.ProposedGrammarLabel,
		feedback.Note,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) ListParseFeedback(status string) ([]ParseFeedback, error) {
	query := `SELECT id, parse_session_id, user_id, lang, parser, surface, occurrence,
		COALESCE(original_lemma, ''), COALESCE(original_pos, ''), COALESCE(original_grammar_label, ''),
		proposed_lemma, proposed_pos, COALESCE(proposed_grammar_label, ''), COALESCE(note, ''),
		status, COALESCE(review_note, ''), reviewed_by_user_id, reviewed_at, created_at
		FROM parse_feedback`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	feedback := []ParseFeedback{}
	for rows.Next() {
		var item ParseFeedback
		var reviewedBy sql.NullInt64
		var reviewedAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.ParseSessionID,
			&item.UserID,
			&item.Lang,
			&item.Parser,
			&item.Surface,
			&item.Occurrence,
			&item.OriginalLemma,
			&item.OriginalPOS,
			&item.OriginalGrammarLabel,
			&item.ProposedLemma,
			&item.ProposedPOS,
			&item.ProposedGrammarLabel,
			&item.Note,
			&item.Status,
			&item.ReviewNote,
			&reviewedBy,
			&reviewedAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if reviewedBy.Valid {
			item.ReviewedByUserID = &reviewedBy.Int64
		}
		if reviewedAt.Valid {
			item.ReviewedAt = &reviewedAt.Time
		}
		feedback = append(feedback, item)
	}

	return feedback, rows.Err()
}

func (d *DB) ReviewParseFeedback(feedbackID, reviewerUserID int64, status, reviewNote string) error {
	result, err := d.db.Exec(
		`UPDATE parse_feedback
		 SET status = ?, review_note = ?, reviewed_by_user_id = ?, reviewed_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		status, reviewNote, reviewerUserID, feedbackID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
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

func getOwnedCardSchedule(q sqlQueryRower, userID, cardID int64) (*Card, ReviewSchedule, error) {
	var card Card
	var fsrsJSON sql.NullString
	err := q.QueryRow(
		`SELECT c.id, c.user_id, c.lang, c.lemma, c.pos, c.mwe_id, cs.fsrs_json
		   FROM cards c
		   JOIN card_state cs ON cs.card_id = c.id
		  WHERE c.id = ? AND c.user_id = ?`,
		cardID, userID,
	).Scan(&card.ID, &card.UserID, &card.Lang, &card.Lemma, &card.POS, &card.MWEID, &fsrsJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ReviewSchedule{}, nil
		}
		return nil, ReviewSchedule{}, err
	}

	schedule := ReviewSchedule{}
	if fsrsJSON.Valid && strings.TrimSpace(fsrsJSON.String) != "" {
		_ = json.Unmarshal([]byte(fsrsJSON.String), &schedule)
	}
	return &card, schedule, nil
}

func (d *DB) getOwnedCardSchedule(userID, cardID int64) (*Card, ReviewSchedule, error) {
	return getOwnedCardSchedule(d.db, userID, cardID)
}

func (d *DB) remainingNewCardsToday(userID int64) (int, error) {
	newPerDay, err := d.userNewCardsPerDay(userID)
	if err != nil {
		return 0, err
	}

	var introducedToday int
	err = d.db.QueryRow(
		`SELECT COUNT(*)
		   FROM cards c
		   JOIN card_state cs ON cs.card_id = c.id
		  WHERE c.user_id = ?
		    AND c.mwe_id IS NULL
		    AND cs.introduced_at IS NOT NULL
		    AND date(cs.introduced_at) = date(CURRENT_TIMESTAMP)`,
		userID,
	).Scan(&introducedToday)
	if err != nil {
		return 0, err
	}

	remaining := newPerDay - introducedToday
	if remaining < 0 {
		return 0, nil
	}
	return remaining, nil
}

func (d *DB) userNewCardsPerDay(userID int64) (int, error) {
	var settingsJSON string
	if err := d.db.QueryRow(`SELECT settings_json FROM users WHERE id = ?`, userID).Scan(&settingsJSON); err != nil {
		return 0, err
	}

	settings := map[string]any{}
	if strings.TrimSpace(settingsJSON) != "" {
		_ = json.Unmarshal([]byte(settingsJSON), &settings)
	}

	newPerDay := 20
	if raw, ok := settings["new_per_day"]; ok {
		switch v := raw.(type) {
		case float64:
			if int(v) > 0 {
				newPerDay = int(v)
			}
		case int:
			if v > 0 {
				newPerDay = v
			}
		}
	}
	return newPerDay, nil
}

func nextScheduleForRating(schedule ReviewSchedule, rating string) (time.Time, ReviewSchedule) {
	now := time.Now().UTC()
	if schedule.Step < 0 {
		schedule.Step = 0
	}
	if schedule.Streak < 0 {
		schedule.Streak = 0
	}

	switch rating {
	case "again":
		schedule.Step = 0
		schedule.Streak = 0
		return now.Add(10 * time.Minute), schedule
	case "hard":
		if schedule.Step < 1 {
			schedule.Step = 1
		}
		schedule.Streak++
		return now.Add(8 * time.Hour), schedule
	case "easy":
		schedule.Step += 2
		if schedule.Step > 5 {
			schedule.Step = 5
		}
		schedule.Streak++
		return now.Add(reviewIntervalForStep(schedule.Step, true)), schedule
	default: // good
		schedule.Step++
		if schedule.Step > 5 {
			schedule.Step = 5
		}
		schedule.Streak++
		return now.Add(reviewIntervalForStep(schedule.Step, false)), schedule
	}
}

func reviewIntervalForStep(step int, easy bool) time.Duration {
	days := []int{1, 3, 7, 14, 30, 60}
	if easy {
		days = []int{3, 7, 14, 30, 60, 90}
	}
	if step < 0 {
		step = 0
	}
	if step >= len(days) {
		step = len(days) - 1
	}
	return time.Duration(days[step]) * 24 * time.Hour
}

func sqliteTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

func (d *DB) ensureLangScopedKnownTable(table string) error {
	hasLang, err := d.tableHasColumn(table, "lang")
	if err != nil {
		return err
	}
	inPrimaryKey, err := d.columnInPrimaryKey(table, "lang")
	if err != nil {
		return err
	}
	if hasLang && inPrimaryKey {
		return nil
	}

	tmpTable := table + "_new"
	createStmt := fmt.Sprintf(`
		CREATE TABLE %s (
			user_id INTEGER NOT NULL,
			lang TEXT NOT NULL DEFAULT 'FI',
			lemma TEXT NOT NULL,
			pos TEXT NOT NULL,
			PRIMARY KEY(user_id, lang, lemma, pos),
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`, tmpTable)
	if _, err := d.db.Exec(createStmt); err != nil {
		return err
	}

	insertStmt := fmt.Sprintf(
		`INSERT OR IGNORE INTO %s (user_id, lang, lemma, pos)
		 SELECT user_id, 'FI', lemma, pos FROM %s`,
		tmpTable, table,
	)
	if _, err := d.db.Exec(insertStmt); err != nil {
		return err
	}
	if _, err := d.db.Exec(`DROP TABLE ` + table); err != nil {
		return err
	}
	if _, err := d.db.Exec(`ALTER TABLE ` + tmpTable + ` RENAME TO ` + table); err != nil {
		return err
	}
	return nil
}

func (d *DB) ensureLangScopedCardsTable() error {
	hasLang, err := d.tableHasColumn("cards", "lang")
	if err != nil {
		return err
	}
	hasUnique, err := d.tableHasUniqueIndex("cards", []string{"user_id", "lang", "lemma", "pos", "mwe_id"})
	if err != nil {
		return err
	}
	if hasLang && hasUnique {
		return nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE cards_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			lang TEXT NOT NULL DEFAULT '',
			lemma TEXT NOT NULL,
			pos TEXT NOT NULL,
			mwe_id INTEGER,
			FOREIGN KEY(user_id) REFERENCES users(id),
			UNIQUE(user_id, lang, lemma, pos, mwe_id)
		)`); err != nil {
		return err
	}

	insertLangExpr := `'FI'`
	copyLangExpr := `'FI'`
	if hasLang {
		insertLangExpr = `COALESCE(NULLIF(lang, ''), 'FI')`
		copyLangExpr = `COALESCE(NULLIF(cold.lang, ''), 'FI')`
	}
	insertCards := fmt.Sprintf(
		`INSERT OR IGNORE INTO cards_new (id, user_id, lang, lemma, pos, mwe_id)
		 SELECT id, user_id, %s, lemma, pos, mwe_id FROM cards`,
		insertLangExpr,
	)
	if _, err := tx.Exec(insertCards); err != nil {
		return err
	}
	// Preserve existing scheduling state while backfilling FI onto legacy
	// pre-language cards. Finnestdb was Finnish-only before this migration.
	if _, err := tx.Exec(`
		CREATE TABLE card_state_new (
			card_id INTEGER PRIMARY KEY,
			fsrs_json TEXT,
			next_due DATETIME,
			last_answer_at DATETIME,
			introduced_at DATETIME,
			FOREIGN KEY(card_id) REFERENCES cards_new(id)
		)`); err != nil {
		return err
	}

	copyCardState := fmt.Sprintf(
		`INSERT OR IGNORE INTO card_state_new (card_id, fsrs_json, next_due, last_answer_at, introduced_at)
		 SELECT cnew.id, cs.fsrs_json, cs.next_due, cs.last_answer_at, NULL
		 FROM card_state cs
		 JOIN cards cold ON cold.id = cs.card_id
		 JOIN cards_new cnew
		   ON cnew.user_id = cold.user_id
		  AND cnew.lang = %s
		  AND cnew.lemma = cold.lemma
		  AND cnew.pos = cold.pos
		  AND ((cnew.mwe_id IS NULL AND cold.mwe_id IS NULL) OR cnew.mwe_id = cold.mwe_id)`,
		copyLangExpr,
	)
	if _, err := tx.Exec(copyCardState); err != nil {
		return err
	}

	if _, err := tx.Exec(`DROP TABLE card_state`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE cards`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE cards_new RENAME TO cards`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE card_state_new RENAME TO card_state`); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *DB) tableHasColumn(table, column string) (bool, error) {
	rows, err := d.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (d *DB) columnInPrimaryKey(table, column string) (bool, error) {
	rows, err := d.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column && primaryKey > 0 {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (d *DB) tableHasUniqueIndex(table string, columns []string) (bool, error) {
	rows, err := d.db.Query(`PRAGMA index_list(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin, partial string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return false, err
		}
		if unique == 0 {
			continue
		}
		matches, err := d.indexMatchesColumns(name, columns)
		if err != nil {
			return false, err
		}
		if matches {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (d *DB) indexMatchesColumns(indexName string, columns []string) (bool, error) {
	rows, err := d.db.Query(`PRAGMA index_info(` + indexName + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	found := []string{}
	for rows.Next() {
		var seqno, cid int
		var name string
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return false, err
		}
		found = append(found, name)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(found) != len(columns) {
		return false, nil
	}
	for i := range found {
		if found[i] != columns[i] {
			return false, nil
		}
	}
	return true, nil
}
