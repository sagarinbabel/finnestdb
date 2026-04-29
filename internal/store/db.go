package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
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

	return nil
}

func (d *DB) GetOrCreateUser(email string) (*User, error) {
	var user User
	var settingsJSON string
	email = normalizeEmail(email)
	isAdmin := isAdminEmail(email)

	err := d.db.QueryRow(
		`SELECT id, email, email_verified, is_admin, settings_json
		 FROM users
		 WHERE lower(email) = ?`,
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
	if _, err := d.db.Exec(
		`INSERT OR IGNORE INTO cards (user_id, lang, lemma, pos, mwe_id) VALUES (?, ?, ?, ?, NULL)`,
		userID, lang, lemma, pos,
	); err != nil {
		return 0, err
	}

	var cardID int64
	if err := d.db.QueryRow(
		`SELECT id FROM cards WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ? AND mwe_id IS NULL ORDER BY id LIMIT 1`,
		userID, lang, lemma, pos,
	).Scan(&cardID); err != nil {
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

func (d *DB) ParseSessionExists(parseSessionID int64) (bool, error) {
	var count int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM parse_sessions WHERE id = ?`, parseSessionID).Scan(&count)
	return count > 0, err
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
	_, err := d.db.Exec(
		`UPDATE parse_feedback
		 SET status = ?, review_note = ?, reviewed_by_user_id = ?, reviewed_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		status, reviewNote, reviewerUserID, feedbackID,
	)
	return err
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
			FOREIGN KEY(card_id) REFERENCES cards_new(id)
		)`); err != nil {
		return err
	}

	copyCardState := fmt.Sprintf(
		`INSERT OR IGNORE INTO card_state_new (card_id, fsrs_json, next_due, last_answer_at)
		 SELECT cnew.id, cs.fsrs_json, cs.next_due, cs.last_answer_at
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
