package store

import (
	"database/sql"
	"encoding/json"
	"errors"
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
	// Tests that don't touch FI form lookups don't pay the disk-read +
	// JSON-parse cost.
	lemOnce sync.Once
	lem     *lemmatizer.Lemmatizer
}

const DefaultParseSourceRetentionDays = 30
const (
	SourceCustomOverrides         = "custom_overrides"
	CustomOverridesSourcePriority = 1000

	// GoldPromotionThreshold is how many distinct users must independently
	// submit (and have accepted) the same correction before it becomes a
	// gold-case candidate. One user's typo must not become a permanent
	// eval case; three independent accepted reports is strong signal.
	GoldPromotionThreshold = 3

	// goldConflictMinCases is how many gold token occurrences must back a
	// surface before a disagreeing override is refused. A single gold
	// occurrence can itself be context-specific; two or more independent
	// occurrences that all disagree with the proposal mean the override
	// would regress the frozen eval.
	goldConflictMinCases = 2
)

// ErrOverrideConflictsWithGold is returned when accepting a parse-feedback
// correction would contradict the frozen gold evaluation sets (correction-loop
// Phase 4). The acceptance is rolled back entirely; the admin sees the
// conflict and can fix the gold set first if the gold itself is wrong.
var ErrOverrideConflictsWithGold = errors.New("accepted correction contradicts the frozen gold sets; refusing to apply the override")

// fstLemmatizer returns the (lazy-loaded) FST lemmatizer, or nil if
// loading failed (e.g. no tables under localdata/lemmatizer-fi-et/tables/
// on a fresh clone without scripts/setup-local.sh). Callers must
// tolerate a nil result and fall back to the SQLite-only resolution
// chain. Both FI and ET share one loaded instance — the per-language
// analysis maps are read-only after lemmatizer.New() returns.
func (d *DB) fstLemmatizer() *lemmatizer.Lemmatizer {
	d.lemOnce.Do(func() {
		l, err := lemmatizer.New()
		if err != nil {
			log.Printf("store: lemmatizer init failed (FST analyzers disabled): %v", err)
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
	IsPublic  bool
	CreatedAt time.Time
}

type DeckStats struct {
	Deck
	Known      int
	Unique     int
	Due        int
	Subscribed bool
	// Token-weighted coverage: distinct content-token positions in the deck,
	// and how many of those are covered by the user's known/ignored lemmas.
	TotalTokens   int
	CoveredTokens int
}

// DeckComprehensionStats is the token-mass coverage summary for one deck as
// seen by one user.
type DeckComprehensionStats struct {
	Lang          string
	TotalTokens   int
	CoveredTokens int
	TopUnlocks    []LemmaTokenCount
}

// LemmaTokenCount ranks a candidate lemma by how many token positions it
// accounts for.
type LemmaTokenCount struct {
	Lemma      string
	POS        string
	TokenCount int
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

type ParseSessionHistoryItem struct {
	ID            int64     `json:"id"`
	Lang          string    `json:"lang"`
	Parser        string    `json:"parser"`
	SourcePreview string    `json:"source_preview"`
	TotalTokens   int       `json:"total_tokens"`
	UniqueWords   int       `json:"unique_words"`
	DeckCount     int       `json:"deck_count"`
	FeedbackCount int       `json:"feedback_count"`
	CreatedAt     time.Time `json:"created_at"`
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
		settings_json TEXT DEFAULT '{"new_per_day":20,"retention":0.9,"theme":"system","learning_languages":["FI","ET"],"active_language":"FI"}'
	);

	CREATE TABLE IF NOT EXISTS decks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		lang TEXT NOT NULL,
		-- Optional: if present, deck-detail "Suggest fix" can attribute feedback to
		-- the parse session that produced the deck.
		parse_session_id INTEGER,
		-- 1 = deck is part of the official deck library, visible to all users
		-- under the "Official decks" tab. Admin-only flag at create time.
		is_public INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	-- A user has opted in to study an official deck. Owners of decks are NOT
	-- listed here for their own decks — ownership implies study access.
	CREATE TABLE IF NOT EXISTS user_deck_subscriptions (
		user_id INTEGER NOT NULL,
		deck_id INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, deck_id),
		FOREIGN KEY(user_id) REFERENCES users(id),
		FOREIGN KEY(deck_id) REFERENCES decks(id)
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
		parse_feedback_id INTEGER,
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

	-- Correction-loop Phase 4 guard data: every (surface, lemma, pos) analysis
	-- that appears in the frozen gold evaluation sets, with how many gold
	-- token occurrences back it. Populated by cmd/importgoldsurfaces from
	-- testdata/parser-eval/*/gold; empty table = guard is a no-op.
	CREATE TABLE IF NOT EXISTS gold_surfaces (
		lang TEXT NOT NULL,
		surface TEXT NOT NULL,
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		case_count INTEGER NOT NULL DEFAULT 1,
		PRIMARY KEY(lang, surface, lemma, pos)
	);

	-- Correction-loop Phase 3 queue: corrections independently submitted and
	-- accepted for enough distinct users get promoted here as gold-case
	-- candidates. cmd/exportgoldcandidates renders pending rows for manual
	-- review before they enter the committed gold sets.
	CREATE TABLE IF NOT EXISTS gold_candidates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		lang TEXT NOT NULL,
		surface TEXT NOT NULL,
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		feats TEXT NOT NULL DEFAULT '',
		supporter_count INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(lang, surface, lemma, pos)
	);

	-- One row per answered review, appended by RecordReviewAnswer. card_state
	-- keeps only the latest answer; this log is what daily-activity stats on
	-- the progress dashboard aggregate over. Rows accumulate from the moment
	-- this table ships — there is no way to backfill history that was never
	-- recorded.
	CREATE TABLE IF NOT EXISTS review_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		card_id INTEGER NOT NULL,
		lang TEXT NOT NULL DEFAULT '',
		rating TEXT NOT NULL,
		reviewed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id),
		FOREIGN KEY(card_id) REFERENCES cards(id)
	);

	CREATE TABLE IF NOT EXISTS user_known_lemmas (
		user_id INTEGER NOT NULL,
		lang TEXT NOT NULL DEFAULT '',
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		-- Where the entry came from: 'manual' (textbox / file / inspect /
		-- review marking) or 'anki' (Anki sync). Lets the Anki sync flow
		-- scope its diff to only anki-source rows so words a user marked
		-- through any other path stay protected.
		source TEXT NOT NULL DEFAULT 'manual',
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
		parse_feedback_id INTEGER,
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
	if err := d.ensureUserKnownLemmasSource(); err != nil {
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
	// Covers GetDeckDetails' per-lemma example-sentence subquery. Without
	// this, novel-sized decks (e.g. an EPUB import with ~12k unique lemmas)
	// re-scan the occurrence table once per group and the deck-detail page
	// takes ~50s to load. Sentence_id + token_ix are included so the LIMIT 1
	// after ORDER BY uses the index for both the lookup and the ordering.
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_occurrence_deck_lemma_pos ON occurrence(deck_id, lemma, pos, sentence_id, token_ix)`); err != nil {
		return err
	}
	if _, err := d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_review_log_user_time ON review_log(user_id, reviewed_at)`); err != nil {
		return err
	}
	if err := EnsureDictMetadataSchema(d.db); err != nil {
		return err
	}
	if err := EnsureDictionarySourceColumns(d.db); err != nil {
		return err
	}
	if err := EnsureCorrectionBackpointerColumns(d.db); err != nil {
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
	if err := EnsureDeckIsPublicColumn(d.db); err != nil {
		return err
	}

	return nil
}

// EnsureCorrectionBackpointerColumns backfills the feedback provenance columns
// used by accepted learner corrections. Fresh DBs already include these
// columns in CREATE TABLE; older DB files need idempotent ALTER TABLEs.
func EnsureCorrectionBackpointerColumns(db *sql.DB) error {
	for table, column := range map[string]string{
		"lemmas": "parse_feedback_id INTEGER",
		"forms":  "parse_feedback_id INTEGER",
	} {
		if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}

// EnsureDeckIsPublicColumn backfills the is_public column on older DB files.
// Fresh DBs already include the column in CREATE TABLE.
func EnsureDeckIsPublicColumn(db *sql.DB) error {
	if _, err := db.Exec(`ALTER TABLE decks ADD COLUMN is_public INTEGER NOT NULL DEFAULT 0`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
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
//
// After ensuring the columns exist, BackfillLegacyKaikkiProvenance fills any
// rows whose source was left empty by an older importer that didn't thread
// source/priority through. The combination is idempotent: once a row carries
// real provenance, neither the ALTER TABLE nor the UPDATE touches it.
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
	return BackfillLegacyKaikkiProvenance(db)
}

// BackfillLegacyKaikkiProvenance labels FI/ET rows that were imported before
// cmd/importdict threaded -source-key / -source-priority through. Those rows
// carry the SQLite column defaults (source=” and source_priority=0), which
// makes verify-dict and `make doctor` flag the entire dictionary as untracked
// even though it's the kaikki dump every fresh install gets.
//
// We can attribute these rows to kaikki with high confidence: the only writers
// that ever produced empty-source rows were the FI/ET kaikki paths in
// cmd/importdict before the provenance flags landed; every later writer
// (Ekilex, Kotus, custom glosses) has always set both fields explicitly.
//
// Idempotent — the WHERE clause matches no rows after the first run, so
// re-applying is a no-op.
func BackfillLegacyKaikkiProvenance(db *sql.DB) error {
	const legacyKaikkiPriority = 10
	for _, table := range []string{"lemmas", "forms"} {
		if _, err := db.Exec(
			`UPDATE `+table+` SET source = 'kaikki', source_priority = ? `+
				`WHERE lang IN ('FI','ET') AND (source IS NULL OR source = '') AND source_priority = 0`,
			legacyKaikkiPriority,
		); err != nil {
			return fmt.Errorf("backfill %s provenance: %w", table, err)
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
	settingsJSON := `{"new_per_day":20,"retention":0.9,"theme":"system","learning_languages":["FI","ET"],"active_language":"FI"}`
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

// Healthcheck verifies the database answers a trivial query. Used by the
// /api/health deployment probe.
func (d *DB) Healthcheck() error {
	var one int
	return d.db.QueryRow(`SELECT 1`).Scan(&one)
}

// RevokeAllSessionsForUser marks every active session for the user as
// revoked. Used by the operator password-reset path so a reset also logs the
// account out everywhere. Returns the number of sessions revoked. Idempotent.
func (d *DB) RevokeAllSessionsForUser(userID int64) (int64, error) {
	res, err := d.db.Exec(
		`UPDATE user_sessions SET revoked_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND revoked_at IS NULL`,
		userID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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

// DeleteUserCascade removes the user and every user-owned row that can carry
// private learning data or pasted source text. SQLite foreign keys are not
// enabled for this app, so the cascade is explicit and ordered.
func (d *DB) DeleteUserCascade(userID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		UPDATE parse_feedback
		   SET reviewed_by_user_id = NULL
		 WHERE reviewed_by_user_id = ?
		   AND user_id <> ?
		   AND parse_session_id NOT IN (SELECT id FROM parse_sessions WHERE user_id = ?)`,
		userID, userID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM parse_feedback
		 WHERE user_id = ?
		    OR parse_session_id IN (SELECT id FROM parse_sessions WHERE user_id = ?)`,
		userID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM user_deck_subscriptions
		 WHERE user_id = ?
		    OR deck_id IN (SELECT id FROM decks WHERE user_id = ?)`,
		userID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM occurrence
		 WHERE deck_id IN (SELECT id FROM decks WHERE user_id = ?)`,
		userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM sentences
		 WHERE deck_id IN (SELECT id FROM decks WHERE user_id = ?)`,
		userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM decks WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM card_state
		 WHERE card_id IN (SELECT id FROM cards WHERE user_id = ?)`,
		userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM cards WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, table := range []string{
		"user_known_lemmas",
		"user_ignored_lemmas",
		"user_sessions",
		"parse_sessions",
	} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE user_id = ?`, userID); err != nil {
			return err
		}
	}
	result, err := tx.Exec(`DELETE FROM users WHERE id = ?`, userID)
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

	return tx.Commit()
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
		"INSERT INTO decks (user_id, title, lang, is_public) VALUES (?, ?, ?, 0)",
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
		"SELECT id, user_id, title, lang, is_public, created_at FROM decks WHERE user_id = ? ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decks []Deck
	for rows.Next() {
		var deck Deck
		if err := rows.Scan(&deck.ID, &deck.UserID, &deck.Title, &deck.Lang, &deck.IsPublic, &deck.CreatedAt); err != nil {
			return nil, err
		}
		decks = append(decks, deck)
	}
	return decks, rows.Err()
}

func (d *DB) GetUserDeckStats(userID int64) ([]DeckStats, error) {
	rows, err := d.db.Query(
		`SELECT d.id, d.user_id, d.title, d.lang, d.is_public, d.created_at,
		        EXISTS (
		            SELECT 1 FROM user_deck_subscriptions s
		             WHERE s.user_id = ? AND s.deck_id = d.id
		        ) AS subscribed,
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
		        END) AS due_count,
		        COUNT(DISTINCT o.sentence_id || char(31) || o.token_ix) AS total_token_count,
		        COUNT(DISTINCT CASE
		            WHEN uk.lemma IS NOT NULL OR ui.lemma IS NOT NULL
		            THEN o.sentence_id || char(31) || o.token_ix
		            ELSE NULL
		        END) AS covered_token_count
		   FROM decks d
		   LEFT JOIN occurrence o
		          ON o.deck_id = d.id
		   LEFT JOIN user_known_lemmas uk
		          ON uk.user_id = ?
		         AND uk.lang = d.lang
		         AND uk.lemma = o.lemma
		         AND uk.pos = o.pos
		   LEFT JOIN user_ignored_lemmas ui
		          ON ui.user_id = ?
		         AND ui.lang = d.lang
		         AND ui.lemma = o.lemma
		         AND ui.pos = o.pos
		   LEFT JOIN cards c
		          ON c.user_id = ?
		         AND c.lang = d.lang
		         AND c.lemma = o.lemma
		         AND c.pos = o.pos
		         AND c.mwe_id IS NULL
		   LEFT JOIN card_state cs
		          ON cs.card_id = c.id
		  WHERE d.user_id = ?
		     OR EXISTS (
		            SELECT 1 FROM user_deck_subscriptions s
		             WHERE s.user_id = ? AND s.deck_id = d.id
		        )
		  GROUP BY d.id, d.user_id, d.title, d.lang, d.is_public, d.created_at
		  ORDER BY d.created_at DESC, d.id DESC`,
		userID, userID, userID, userID, userID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := []DeckStats{}
	for rows.Next() {
		var item DeckStats
		// Subscribed is read straight from the SELECT, not derived from
		// UserID-vs-caller. The latter only happens to be correct under
		// today's WHERE clause (owned OR subscribed) — widening the listing
		// later (shared decks, team decks, etc.) would silently break the
		// invariant if we kept inferring it.
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.Title,
			&item.Lang,
			&item.IsPublic,
			&item.CreatedAt,
			&item.Subscribed,
			&item.Unique,
			&item.Known,
			&item.Due,
			&item.TotalTokens,
			&item.CoveredTokens,
		); err != nil {
			return nil, err
		}
		stats = append(stats, item)
	}
	return stats, rows.Err()
}

// DeckComprehension computes token-weighted coverage for a deck the user can
// access (owner, public, or subscribed): the share of content-token positions
// covered by the user's known lemmas, plus the top-N uncovered (lemma, pos)
// pairs ranked by how many token positions learning each would unlock.
//
// Token identity is (sentence_id, token_ix): multi-lemma homonym expansion
// stores one occurrence row per candidate, and a position counts as covered
// when ANY of its candidates is known. Ignored lemmas count as covered —
// "ignore" means "don't make me study this" (typically proper names), and
// coverage is a reading-comprehension proxy, not a study queue. Coverage is
// lemma-level; form-level display is a possible later toggle.
//
// Returns sql.ErrNoRows when the deck does not exist or the user cannot see
// it, matching GetDeckDetails.
func (d *DB) DeckComprehension(userID, deckID int64, topN int) (*DeckComprehensionStats, error) {
	var stats DeckComprehensionStats
	err := d.db.QueryRow(
		`SELECT lang FROM decks
		  WHERE id = ?
		    AND (user_id = ?
		         OR is_public = 1
		         OR EXISTS (SELECT 1 FROM user_deck_subscriptions s
		                     WHERE s.user_id = ? AND s.deck_id = decks.id))`,
		deckID, userID, userID,
	).Scan(&stats.Lang)
	if err != nil {
		return nil, err
	}

	const coveredFlags = `
		SELECT o.sentence_id, o.token_ix,
		       MAX(CASE WHEN uk.lemma IS NOT NULL OR ui.lemma IS NOT NULL
		           THEN 1 ELSE 0 END) AS covered
		  FROM occurrence o
		  LEFT JOIN user_known_lemmas uk
		         ON uk.user_id = ?1 AND uk.lang = ?2
		        AND uk.lemma = o.lemma AND uk.pos = o.pos
		  LEFT JOIN user_ignored_lemmas ui
		         ON ui.user_id = ?1 AND ui.lang = ?2
		        AND ui.lemma = o.lemma AND ui.pos = o.pos
		 WHERE o.deck_id = ?3
		 GROUP BY o.sentence_id, o.token_ix`

	if err := d.db.QueryRow(
		`WITH flags AS (`+coveredFlags+`)
		 SELECT COUNT(*), COALESCE(SUM(covered), 0) FROM flags`,
		userID, stats.Lang, deckID,
	).Scan(&stats.TotalTokens, &stats.CoveredTokens); err != nil {
		return nil, err
	}

	if topN <= 0 || stats.TotalTokens == stats.CoveredTokens {
		return &stats, nil
	}

	rows, err := d.db.Query(
		`WITH flags AS (`+coveredFlags+`)
		 SELECT o.lemma, o.pos, COUNT(*) AS cnt
		   FROM occurrence o
		   JOIN flags f
		     ON f.sentence_id = o.sentence_id
		    AND f.token_ix = o.token_ix
		    AND f.covered = 0
		  WHERE o.deck_id = ?3
		  GROUP BY o.lemma, o.pos
		  ORDER BY cnt DESC, o.lemma ASC, o.pos ASC
		  LIMIT ?4`,
		userID, stats.Lang, deckID, topN,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item LemmaTokenCount
		if err := rows.Scan(&item.Lemma, &item.POS, &item.TokenCount); err != nil {
			return nil, err
		}
		stats.TopUnlocks = append(stats.TopUnlocks, item)
	}
	return &stats, rows.Err()
}

func createDeck(q sqlReadWriter, userID int64, title, lang string, isPublic bool) (int64, error) {
	result, err := q.Exec(
		"INSERT INTO decks (user_id, title, lang, is_public) VALUES (?, ?, ?, ?)",
		userID, title, lang, boolToInt(isPublic),
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
	return d.CreateDeckWithSentencesOptions(userID, title, lang, false, sentences)
}

// CreateDeckWithSentencesOptions creates a deck and optionally marks it as
// public (visible to all users under "Official decks"). Callers are
// responsible for authorising the isPublic flag — see handleCreateDeck.
func (d *DB) CreateDeckWithSentencesOptions(userID int64, title, lang string, isPublic bool, sentences []DeckSentenceInput) (int64, error) {
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

	deckID, err := createDeck(tx, userID, title, lang, isPublic)
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

// UserCanStudyDeck returns true if the user owns the deck or has subscribed
// to it via the official-deck list. Used by review-by-deck filtering.
func (d *DB) UserCanStudyDeck(userID, deckID int64) (bool, error) {
	var exists int
	err := d.db.QueryRow(
		`SELECT 1 FROM decks d
		  WHERE d.id = ?
		    AND (d.user_id = ?
		         OR EXISTS (SELECT 1 FROM user_deck_subscriptions s
		                     WHERE s.user_id = ? AND s.deck_id = d.id))`,
		deckID, userID, userID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// PublicDeckSummary is one row of the official-deck listing.
type PublicDeckSummary struct {
	DeckStats
	// Subscribed is true if the requesting user has added this deck to
	// their studying list (does not apply when IsOwner is true).
	Subscribed bool
	// IsOwner is true if the deck was created by the requesting user.
	// Owners see their own deck in the catalog so they can verify what
	// other users will see; the UI suppresses the subscribe button.
	IsOwner bool
}

// ListPublicDecksForUser returns every is_public deck (including ones the
// requesting user owns), alongside whether they've added it to their
// studying list. The owner case is signaled via PublicDeckSummary.IsOwner so
// the UI can render it without a meaningless "Add to studying" button — an
// admin verifying their own publication still wants to see it in the
// catalog the way other users will.
func (d *DB) ListPublicDecksForUser(userID int64) ([]PublicDeckSummary, error) {
	rows, err := d.db.Query(
		`SELECT d.id, d.user_id, d.title, d.lang, d.is_public, d.created_at,
		        COUNT(DISTINCT o.lemma || char(31) || o.pos) AS unique_count,
		        COUNT(DISTINCT CASE
		            WHEN uk.lemma IS NOT NULL THEN o.lemma || char(31) || o.pos
		            ELSE NULL
		        END) AS known_count,
		        EXISTS (
		            SELECT 1 FROM user_deck_subscriptions s
		             WHERE s.user_id = ? AND s.deck_id = d.id
		        ) AS subscribed
		   FROM decks d
		   LEFT JOIN occurrence o
		          ON o.deck_id = d.id
		   LEFT JOIN user_known_lemmas uk
		          ON uk.user_id = ?
		         AND uk.lang = d.lang
		         AND uk.lemma = o.lemma
		         AND uk.pos = o.pos
		  WHERE d.is_public = 1
		  GROUP BY d.id, d.user_id, d.title, d.lang, d.is_public, d.created_at
		  ORDER BY d.created_at DESC, d.id DESC`,
		userID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []PublicDeckSummary{}
	for rows.Next() {
		var item PublicDeckSummary
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.Title,
			&item.Lang,
			&item.IsPublic,
			&item.CreatedAt,
			&item.Unique,
			&item.Known,
			&item.Subscribed,
		); err != nil {
			return nil, err
		}
		// Due not meaningful in the catalog — reviews are user-scoped and the
		// catalog row may not have any seeded cards yet.
		item.Due = 0
		item.IsOwner = item.UserID == userID
		results = append(results, item)
	}
	return results, rows.Err()
}

// SubscribeUserToPublicDeck records that the user has added an official deck
// to their studying list and seeds cards for each unique (lemma, pos) in the
// deck, skipping lemmas the user has already marked known or ignored. Idempotent.
// Returns sql.ErrNoRows if the deck does not exist or is not public.
func (d *DB) SubscribeUserToPublicDeck(userID, deckID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var lang string
	var ownerID int64
	var isPublic int
	if err := tx.QueryRow(
		`SELECT lang, user_id, is_public FROM decks WHERE id = ?`,
		deckID,
	).Scan(&lang, &ownerID, &isPublic); err != nil {
		return err
	}
	if isPublic != 1 {
		return sql.ErrNoRows
	}
	if ownerID == userID {
		// Owners implicitly study their own decks. No subscription row needed.
		return tx.Commit()
	}

	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO user_deck_subscriptions (user_id, deck_id) VALUES (?, ?)`,
		userID, deckID,
	); err != nil {
		return err
	}

	// Seed cards in two bulk INSERTs instead of a per-lemma Go loop. For a
	// novel-sized deck (~12k unique lemmas) the loop variant did ~24k
	// queries inside the write transaction and held the SQLite write lock
	// for tens of seconds, blocking every other writer. The two statements
	// below do the same work and the lock is held for milliseconds.
	//
	// 1. INSERT a card for each unique (lemma, pos) in the deck the user
	//    hasn't already marked known or ignored. The unique index on cards
	//    keeps this idempotent for users who re-subscribe.
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO cards (user_id, lang, lemma, pos, mwe_id)
		 SELECT DISTINCT ?, ?, o.lemma, o.pos, NULL
		   FROM occurrence o
		  WHERE o.deck_id = ?
		    AND o.lemma != ''
		    AND o.pos   != ''
		    AND NOT EXISTS (
		        SELECT 1 FROM user_known_lemmas uk
		         WHERE uk.user_id = ? AND uk.lang = ?
		           AND uk.lemma   = o.lemma AND uk.pos = o.pos
		    )
		    AND NOT EXISTS (
		        SELECT 1 FROM user_ignored_lemmas ui
		         WHERE ui.user_id = ? AND ui.lang = ?
		           AND ui.lemma   = o.lemma AND ui.pos = o.pos
		    )`,
		userID, lang, deckID,
		userID, lang,
		userID, lang,
	); err != nil {
		return err
	}

	// 2. Seed card_state for any card that doesn't have one yet (a card
	//    inserted just now, or a pre-existing card from another deck whose
	//    state row was somehow missing). Filtering on this deck's
	//    occurrence rows scopes the work to seeds we just made.
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO card_state (card_id, fsrs_json, next_due, last_answer_at)
		 SELECT c.id, NULL, NULL, NULL
		   FROM cards c
		  WHERE c.user_id = ? AND c.lang = ? AND c.mwe_id IS NULL
		    AND EXISTS (
		        SELECT 1 FROM occurrence o
		         WHERE o.deck_id = ?
		           AND o.lemma   = c.lemma
		           AND o.pos     = c.pos
		    )`,
		userID, lang, deckID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// UserSubscribedToDeck returns true if the user has added this deck to their
// studying list via user_deck_subscriptions. Returns false when they own the
// deck (ownership is not represented in the subscriptions table).
func (d *DB) UserSubscribedToDeck(userID, deckID int64) (bool, error) {
	var exists int
	err := d.db.QueryRow(
		`SELECT 1 FROM user_deck_subscriptions WHERE user_id = ? AND deck_id = ?`,
		userID, deckID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// UnsubscribeUserFromPublicDeck removes the studying-list entry. Cards
// previously seeded for this deck are left in place, matching how deleting
// one's own deck preserves global learning state.
func (d *DB) UnsubscribeUserFromPublicDeck(userID, deckID int64) error {
	res, err := d.db.Exec(
		`DELETE FROM user_deck_subscriptions WHERE user_id = ? AND deck_id = ?`,
		userID, deckID,
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

// SetDeckIsPublic toggles a deck's official-deck status. Caller is
// responsible for authorising the operation (admin-only at the handler
// layer) — this method intentionally does not filter by user_id so admins
// can re-publish decks they don't personally own. Returns sql.ErrNoRows if
// the deck doesn't exist.
func (d *DB) SetDeckIsPublic(deckID int64, isPublic bool) error {
	result, err := d.db.Exec(
		`UPDATE decks SET is_public = ? WHERE id = ?`,
		boolToInt(isPublic), deckID,
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

// UpdateDeckTitleAndPublic applies a combined title/visibility PATCH
// atomically. Visibility is admin-authorized at the handler layer; title still
// requires ownership here so the transaction cannot publish a deck if the
// rename half fails.
func (d *DB) UpdateDeckTitleAndPublic(userID, deckID int64, title string, isPublic bool) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE decks SET is_public = ? WHERE id = ?`,
		boolToInt(isPublic), deckID,
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

	result, err = tx.Exec(
		`UPDATE decks SET title = ? WHERE id = ? AND user_id = ?`,
		title, deckID, userID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return tx.Commit()
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
	// PRAGMA foreign_keys is not enabled, so FK declarations don't cascade.
	// Same guard pattern as above: only clear subscriptions when the deck is
	// actually owned by the caller (= will actually be deleted below).
	if _, err := tx.Exec(`DELETE FROM user_deck_subscriptions WHERE deck_id = ? AND deck_id IN (SELECT id FROM decks WHERE id = ? AND user_id = ?)`, deckID, deckID, userID); err != nil {
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
// sentence. Returns sql.ErrNoRows when none of these apply:
//   - the user owns the deck;
//   - the deck is currently public; or
//   - the user has an active subscription (was studying the deck before it
//     was unpublished — they keep read-only access).
//
// The third clause is the "grandfather" rule: unpublishing a deck must not
// 404 learners who already added it to their studying list. GetUserDeckStats
// keeps showing the deck for them; this query has to agree.
func (d *DB) GetDeckDetails(userID, deckID int64) (*DeckDetails, error) {
	var details DeckDetails
	var parseSessionID sql.NullInt64
	err := d.db.QueryRow(
		`SELECT id, user_id, title, lang, is_public, parse_session_id, created_at
		   FROM decks
		  WHERE id = ?
		    AND (user_id = ?
		         OR is_public = 1
		         OR EXISTS (SELECT 1 FROM user_deck_subscriptions s
		                     WHERE s.user_id = ? AND s.deck_id = ?))`,
		deckID, userID, userID, deckID,
	).Scan(&details.ID, &details.UserID, &details.Title, &details.Lang, &details.IsPublic, &parseSessionID, &details.CreatedAt)
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
	keys := make([]LemmaKey, 0)
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
		keys = append(keys, LemmaKey{Lemma: item.Lemma, POS: item.POS})
		details.Lemmas = append(details.Lemmas, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if glosses := d.BatchLookupGlosses(keys, details.Lang); len(glosses) > 0 {
		for i := range details.Lemmas {
			key := LemmaKey{Lemma: details.Lemmas[i].Lemma, POS: details.Lemmas[i].POS}
			if gloss := glosses[key]; gloss != "" {
				details.Lemmas[i].Gloss = gloss
			}
		}
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

// SourceManual / SourceAnki tag where a known-lemma row came from. The
// strings live on the wire (POST/PUT body and response) so they're stable
// here too. Other strings are accepted by the store but rejected by the
// handlers — keep the supported set small.
const (
	SourceManual = "manual"
	SourceAnki   = "anki"
)

// ErrKnownWordsReplaceNoResolvedWords is returned by ReplaceKnownWords when the
// caller submitted a non-empty word list but none of the surfaces resolved to a
// dictionary lemma. Without this guard the replace diff would treat every
// current row as removed and silently wipe the user's known-word list. An
// explicit empty list (clear-vocabulary) is still allowed.
var ErrKnownWordsReplaceNoResolvedWords = errors.New("no submitted words could be resolved; refusing to replace known words")

// ImportKnownWords is additive: new rows are inserted with the given source. A
// manual import that collides with an existing Anki row upgrades that row to
// source='manual', so a later Anki-scoped sync cannot delete a word the user
// explicitly confirmed. An Anki import never claims an existing manual row.
// source defaults to "manual" if empty.
func (d *DB) ImportKnownWords(userID int64, lang string, words []string, source string) ([]KnownLemma, []string, error) {
	if source == "" {
		source = SourceManual
	}
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
			`INSERT INTO user_known_lemmas (user_id, lang, lemma, pos, source) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(user_id, lang, lemma, pos) DO UPDATE SET source = excluded.source
			 WHERE excluded.source = 'manual'`,
			userID, lang, resolution.Lemma, resolution.POS, source,
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

// ReplaceKnownWords makes the user's known-words for one language exactly
// match the set produced by resolving `words`. Used by the Anki "sync"
// flow when the user opts into replace mode: vocabulary that no longer
// appears in their Anki deck is removed, and new entries are added.
//
// `scope` controls which rows the diff is allowed to remove:
//   - "all": every row in the user/lang scope is in play. A row not in the
//     new target set is deleted regardless of its source.
//   - "anki" (default): only rows with source='anki' can be deleted. Rows
//     the user marked manually (textbox, file, inspect/review) survive
//     the sync.
//
// New rows are always inserted with source='anki' since they came from a
// sync. INSERT OR IGNORE means an existing row keeps its current source —
// a word that's both in Anki and was previously marked manually stays
// manual and is therefore preserved by the next "anki" scope sync.
//
// The diff happens inside a single transaction so the user never observes a
// half-applied state, and we never DELETE rows we're about to re-INSERT.
//
// Returns:
//   - added:      lemma+POS pairs newly inserted this call (rows that
//     actually changed; pre-existing rows aren't counted)
//   - removed:    lemma+POS pairs deleted this call
//   - unresolved: surface forms the parser couldn't resolve (untouched)
func (d *DB) ReplaceKnownWords(userID int64, lang string, words []string, scope string) ([]KnownLemma, []KnownLemma, []string, error) {
	if scope == "" {
		scope = SourceAnki
	}
	if scope != "all" && scope != SourceAnki {
		return nil, nil, nil, fmt.Errorf("invalid scope %q", scope)
	}
	// Normalise + dedupe input. Mirrors ImportKnownWords so callers can rely
	// on the same parsing rules.
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

	// Resolve outside the transaction — BatchLookupForms is read-only against
	// the dictionary side of the store and we want to hold the write lock for
	// as little time as possible.
	resolutions := d.BatchLookupForms(normalized, lang, "custom")

	type key struct{ lemma, pos string }
	target := make(map[key]struct{}, len(resolutions))
	unresolved := make([]string, 0)
	for _, word := range normalized {
		res, ok := resolutions[word]
		if !ok || res.Lemma == "" || res.POS == "" {
			unresolved = append(unresolved, word)
			continue
		}
		target[key{res.Lemma, res.POS}] = struct{}{}
	}
	// Guard against a destructive wipe: if the caller submitted words but none
	// resolved, the diff below would delete every current row. Refuse before
	// opening the write transaction. An explicit empty list still clears.
	if len(words) > 0 && len(target) == 0 {
		return nil, nil, unresolved, ErrKnownWordsReplaceNoResolvedWords
	}

	tx, err := d.db.Begin()
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Current state for the user+lang, narrowed by scope. With scope='anki'
	// we only see rows that came from a prior Anki import, so the diff can
	// only ever delete those. Rows the user added through other paths
	// (textbox, file, inspect/review) are invisible to the diff and survive.
	currentQuery := `SELECT lemma, pos FROM user_known_lemmas WHERE user_id = ? AND lang = ?`
	args := []any{userID, lang}
	if scope == SourceAnki {
		currentQuery += ` AND source = ?`
		args = append(args, SourceAnki)
	}
	rows, err := tx.Query(currentQuery, args...)
	if err != nil {
		return nil, nil, nil, err
	}
	current := make(map[key]struct{})
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.lemma, &k.pos); err != nil {
			_ = rows.Close()
			return nil, nil, nil, err
		}
		current[k] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, nil, nil, err
	}
	_ = rows.Close()

	// Diff.
	removed := make([]KnownLemma, 0)
	for k := range current {
		if _, keep := target[k]; keep {
			continue
		}
		deleteQuery := `DELETE FROM user_known_lemmas WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?`
		deleteArgs := []any{userID, lang, k.lemma, k.pos}
		if scope == SourceAnki {
			deleteQuery += ` AND source = ?`
			deleteArgs = append(deleteArgs, SourceAnki)
		}
		if _, err := tx.Exec(deleteQuery, deleteArgs...); err != nil {
			return nil, nil, nil, err
		}
		removed = append(removed, KnownLemma{Lemma: k.lemma, POS: k.pos, Lang: lang})
	}
	added := make([]KnownLemma, 0)
	for k := range target {
		if _, exists := current[k]; exists {
			continue
		}
		// INSERT OR IGNORE: if a row already exists with source='manual' (so
		// it wasn't in `current` because we filtered to anki-source), we
		// leave it alone — the user marked this word through another path
		// and we don't want to "claim" it for Anki. RowsAffected lets us
		// distinguish real inserts from ignored conflicts so `added`
		// reflects what actually changed.
		res, err := tx.Exec(
			`INSERT OR IGNORE INTO user_known_lemmas (user_id, lang, lemma, pos, source) VALUES (?, ?, ?, ?, ?)`,
			userID, lang, k.lemma, k.pos, SourceAnki,
		)
		if err != nil {
			return nil, nil, nil, err
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			added = append(added, KnownLemma{Lemma: k.lemma, POS: k.pos, Lang: lang})
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, nil, err
	}
	return added, removed, unresolved, nil
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

// DeleteAllKnownWords clears every known-word row for one (user, lang). Used
// by the vocab page's "Delete all vocabulary" action; the per-language scope
// keeps a user studying both FI and ET from accidentally wiping the other
// language. Returns the number of rows removed.
func (d *DB) DeleteAllKnownWords(userID int64, lang string) (int64, error) {
	res, err := d.db.Exec(
		`DELETE FROM user_known_lemmas WHERE user_id = ? AND lang = ?`,
		userID, lang,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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
		`INSERT INTO user_known_lemmas (user_id, lang, lemma, pos, source) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, lang, lemma, pos) DO UPDATE SET source = excluded.source`,
		userID, lang, lemma, pos, SourceManual,
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

// CountKnownLemmasByLang returns per-language counts of the user's known
// lemmas. Languages with zero known words are not included; callers should
// treat a missing key as zero. Used by the Languages page to surface a
// "vocab stat" next to each language alongside the deck count.
func (d *DB) CountKnownLemmasByLang(userID int64) (map[string]int, error) {
	rows, err := d.db.Query(
		`SELECT lang, COUNT(*) FROM user_known_lemmas WHERE user_id = ? GROUP BY lang`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var lang string
		var count int
		if err := rows.Scan(&lang, &count); err != nil {
			return nil, err
		}
		out[lang] = count
	}
	return out, rows.Err()
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

func (d *DB) GetNextReviewCard(userID int64, deckID *int64, lang string) (*ReviewCard, error) {
	deckFilter := ""
	deckOccurrenceFilter := ""
	newCardFilter := ""
	langFilter := ""
	if deckID != nil {
		deckFilter = ` AND EXISTS (
			SELECT 1 FROM occurrence o
			WHERE o.deck_id = ? AND o.lemma = c.lemma AND o.pos = c.pos
		)`
		deckOccurrenceFilter = ` AND o.deck_id = ?`
	}
	if lang != "" {
		langFilter = ` AND c.lang = ?`
	}
	remainingNewCards, err := d.remainingNewCardsToday(userID)
	if err != nil {
		return nil, err
	}
	if remainingNewCards == 0 {
		newCardFilter = ` AND cs.last_answer_at IS NOT NULL`
	}

	// "Studied" deck = owned by the user OR added to their studying list via
	// the official-deck catalog (user_deck_subscriptions). Examples and source
	// titles only come from studied decks; other users' private decks stay
	// invisible.
	studiedDeckClause := `(d.user_id = c.user_id
		OR EXISTS (SELECT 1 FROM user_deck_subscriptions s
		            WHERE s.user_id = c.user_id AND s.deck_id = d.id))`

	query := `SELECT c.id, c.lang, c.lemma, c.pos, COALESCE(l.gloss, ''),
	                 COALESCE((
	                     SELECT s.text
	                       FROM occurrence o
	                       JOIN sentences s ON s.id = o.sentence_id
	                       JOIN decks d ON d.id = o.deck_id
	                      WHERE ` + studiedDeckClause + `
	                        AND d.lang = c.lang
	                        AND o.lemma = c.lemma AND o.pos = c.pos` + deckOccurrenceFilter + `
	                      ORDER BY s.id ASC
	                      LIMIT 1
	                 ), ''),
	                 COALESCE((
	                     SELECT d.title
	                       FROM occurrence o
	                       JOIN decks d ON d.id = o.deck_id
	                      WHERE ` + studiedDeckClause + `
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
	             AND (cs.next_due IS NULL OR cs.next_due <= CURRENT_TIMESTAMP)` + newCardFilter + deckFilter + langFilter + `
	        ORDER BY CASE WHEN cs.last_answer_at IS NULL THEN 0 ELSE 1 END,
	                 COALESCE(cs.next_due, '1970-01-01 00:00:00') ASC,
	                 c.id ASC
	           LIMIT 1`

	queryArgs := []any{userID}
	if deckID != nil {
		queryArgs = []any{*deckID, *deckID, userID, *deckID}
	}
	if lang != "" {
		queryArgs = append(queryArgs, lang)
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
	if glosses := d.BatchLookupGlosses([]LemmaKey{{Lemma: card.Lemma, POS: card.POS}}, card.Lang); len(glosses) > 0 {
		if gloss := glosses[LemmaKey{Lemma: card.Lemma, POS: card.POS}]; gloss != "" {
			card.Gloss = gloss
		}
	}

	countRows, err := d.db.Query(
		`SELECT d.title, COUNT(*)
		   FROM occurrence o
		   JOIN decks d ON d.id = o.deck_id
		  WHERE (d.user_id = ?
		         OR EXISTS (SELECT 1 FROM user_deck_subscriptions s
		                     WHERE s.user_id = ? AND s.deck_id = d.id))
		    AND d.lang = ?
		    AND o.lemma = ?
		    AND o.pos = ?
		  GROUP BY d.id, d.title
		  ORDER BY d.created_at DESC, d.id DESC`,
		userID, userID, card.Lang, card.Lemma, card.POS,
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

	nextDue, updated := nextAlphaStepScheduleForRating(schedule, rating)
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
	if _, err := tx.Exec(
		`INSERT INTO review_log (user_id, card_id, lang, rating) VALUES (?, ?, ?, ?)`,
		userID, cardID, card.Lang, rating,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ReviewActivityDay is one day of answered reviews (UTC dates, YYYY-MM-DD).
type ReviewActivityDay struct {
	Day   string
	Count int
}

// ReviewActivity returns per-day answered-review counts for the trailing
// `days` window (today included), oldest first, with zero-count days filled
// in so the dashboard chart has a fixed-width axis. Dates are UTC — the log
// writes CURRENT_TIMESTAMP and per-user timezones are not tracked in alpha.
func (d *DB) ReviewActivity(userID int64, days int) ([]ReviewActivityDay, error) {
	if days <= 0 {
		return nil, nil
	}
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -(days - 1))
	rows, err := d.db.Query(
		`SELECT date(reviewed_at), COUNT(*)
		   FROM review_log
		  WHERE user_id = ? AND date(reviewed_at) >= date(?)
		  GROUP BY date(reviewed_at)`,
		userID, start.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int, days)
	for rows.Next() {
		var day string
		var count int
		if err := rows.Scan(&day, &count); err != nil {
			return nil, err
		}
		counts[day] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]ReviewActivityDay, 0, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, ReviewActivityDay{Day: day, Count: counts[day]})
	}
	return out, nil
}

// CardsInReview counts the user's cards that have entered the review
// rotation (introduced or answered at least once).
func (d *DB) CardsInReview(userID int64) (int, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*)
		   FROM cards c
		   JOIN card_state cs ON cs.card_id = c.id
		  WHERE c.user_id = ?
		    AND (cs.introduced_at IS NOT NULL OR cs.last_answer_at IS NOT NULL)`,
		userID,
	).Scan(&count)
	return count, err
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
		`INSERT INTO user_known_lemmas (user_id, lang, lemma, pos, source) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, lang, lemma, pos) DO UPDATE SET source = excluded.source`,
		userID, card.Lang, card.Lemma, card.POS, SourceManual,
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

func (d *DB) ListUserParseSessions(userID int64) ([]ParseSessionHistoryItem, error) {
	rows, err := d.db.Query(
		`SELECT ps.id, ps.lang, ps.parser,
		        CASE
		          WHEN ps.source_text = '' THEN '(source text purged)'
		          ELSE substr(replace(replace(ps.source_text, char(10), ' '), char(13), ' '), 1, 240)
		        END,
		        ps.total_tokens, ps.unique_words, ps.created_at,
		        (SELECT COUNT(*) FROM decks d WHERE d.user_id = ? AND d.parse_session_id = ps.id),
		        (SELECT COUNT(*) FROM parse_feedback pf WHERE pf.user_id = ? AND pf.parse_session_id = ps.id)
		   FROM parse_sessions ps
		  WHERE ps.user_id = ?
		  ORDER BY ps.created_at DESC, ps.id DESC
		  LIMIT 200`,
		userID, userID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := []ParseSessionHistoryItem{}
	for rows.Next() {
		var item ParseSessionHistoryItem
		if err := rows.Scan(
			&item.ID,
			&item.Lang,
			&item.Parser,
			&item.SourcePreview,
			&item.TotalTokens,
			&item.UniqueWords,
			&item.CreatedAt,
			&item.DeckCount,
			&item.FeedbackCount,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, item)
	}
	return sessions, rows.Err()
}

func (d *DB) CountPurgeableParseSessionSourceText(cutoff time.Time) (int64, error) {
	var n int64
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM parse_sessions
		  WHERE created_at < ? AND source_text <> ''`,
		sqliteTime(cutoff),
	).Scan(&n)
	return n, err
}

func (d *DB) PurgeParseSessionSourceText(cutoff time.Time) (int64, error) {
	result, err := d.db.Exec(
		`UPDATE parse_sessions
		    SET source_text = ''
		  WHERE created_at < ? AND source_text <> ''`,
		sqliteTime(cutoff),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (d *DB) DeleteUserParseSession(userID, parseSessionID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE decks SET parse_session_id = NULL
		  WHERE user_id = ? AND parse_session_id = ?`,
		userID, parseSessionID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM parse_feedback
		  WHERE parse_session_id IN (
			SELECT id FROM parse_sessions WHERE id = ? AND user_id = ?
		  )`,
		parseSessionID, userID,
	); err != nil {
		return err
	}
	result, err := tx.Exec(
		`DELETE FROM parse_sessions WHERE id = ? AND user_id = ?`,
		parseSessionID, userID,
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
	return tx.Commit()
}

func (d *DB) DeleteUserParseSessions(userID int64) (int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE decks SET parse_session_id = NULL
		  WHERE user_id = ? AND parse_session_id IN (
			SELECT id FROM parse_sessions WHERE user_id = ?
		  )`,
		userID, userID,
	); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(
		`DELETE FROM parse_feedback
		  WHERE parse_session_id IN (
			SELECT id FROM parse_sessions WHERE user_id = ?
		  )`,
		userID,
	); err != nil {
		return 0, err
	}
	result, err := tx.Exec(`DELETE FROM parse_sessions WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return rowsAffected, nil
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
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var feedback ParseFeedback
	var proposedGrammarLabel sql.NullString
	err = tx.QueryRow(
		`SELECT id, lang, surface, proposed_lemma, proposed_pos, proposed_grammar_label
		 FROM parse_feedback
		 WHERE id = ?`,
		feedbackID,
	).Scan(&feedback.ID, &feedback.Lang, &feedback.Surface, &feedback.ProposedLemma, &feedback.ProposedPOS, &proposedGrammarLabel)
	if proposedGrammarLabel.Valid {
		feedback.ProposedGrammarLabel = proposedGrammarLabel.String
	}
	if err == sql.ErrNoRows {
		return sql.ErrNoRows
	}
	if err != nil {
		return err
	}

	result, err := tx.Exec(
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
	if status == "accepted" {
		if err := writeAcceptedParseFeedbackOverride(tx, feedback); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func writeAcceptedParseFeedbackOverride(tx *sql.Tx, feedback ParseFeedback) error {
	lemma := strings.TrimSpace(feedback.ProposedLemma)
	pos := strings.TrimSpace(feedback.ProposedPOS)
	lang := strings.TrimSpace(feedback.Lang)
	form := strings.ToLower(strings.TrimSpace(feedback.Surface))
	if lemma == "" || pos == "" || lang == "" || form == "" {
		return fmt.Errorf("accepted parse feedback %d is missing override fields", feedback.ID)
	}

	// Phase 4: eval-gated safety check. Refuse the override when the frozen
	// gold sets know this surface and unanimously disagree with the proposal
	// across enough occurrences — applying it would regress the eval. The
	// whole acceptance rolls back, so the feedback stays reviewable.
	if err := checkOverrideAgainstGold(tx, lang, form, lemma, pos); err != nil {
		return err
	}

	// Phase 2: accepted grammar-label corrections ride the same override
	// row as FEATS. The corrected feats live ONLY on the custom_overrides
	// row — upstream imported rows stay pristine so a dictionary re-import
	// never silently reverts or duplicates a correction (deliberate
	// deviation from the TODO sketch of editing the upstream row in place).
	feats := featsFromCaseLabel(strings.ToLower(strings.TrimSpace(feedback.ProposedGrammarLabel)))

	if _, err := tx.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority, parse_feedback_id)
		 VALUES (?, ?, NULL, ?, ?, ?, ?)
		 ON CONFLICT(lemma, pos, lang) DO UPDATE SET
			source = excluded.source,
			source_priority = excluded.source_priority,
			parse_feedback_id = excluded.parse_feedback_id
		 WHERE lemmas.source_priority <= excluded.source_priority`,
		lemma, pos, lang, SourceCustomOverrides, CustomOverridesSourcePriority, feedback.ID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE FROM forms
		 WHERE form = ? AND lang = ? AND source = ? AND source_priority = ?`,
		form, lang, SourceCustomOverrides, CustomOverridesSourcePriority,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, parse_feedback_id, feats)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(form, lang, lemma, pos) DO UPDATE SET
			source = excluded.source,
			source_priority = excluded.source_priority,
			parse_feedback_id = excluded.parse_feedback_id,
			feats = excluded.feats
		 WHERE forms.source_priority <= excluded.source_priority`,
		form, lemma, pos, lang, SourceCustomOverrides, CustomOverridesSourcePriority, feedback.ID, feats,
	); err != nil {
		return err
	}

	// Phase 3: promote to a gold-case candidate once enough distinct users
	// have independently submitted (and had accepted) the same correction.
	return maybePromoteGoldCandidate(tx, lang, form, lemma, pos, feats)
}

// checkOverrideAgainstGold implements the correction-loop Phase 4 guard.
// Empty gold_surfaces (importer never run) means no check — the guard cannot
// invent gold knowledge it doesn't have.
func checkOverrideAgainstGold(tx *sql.Tx, lang, form, lemma, pos string) error {
	var total, matching int
	if err := tx.QueryRow(
		`SELECT COALESCE(SUM(case_count), 0),
		        COALESCE(SUM(CASE WHEN lemma = ? AND pos = ? THEN case_count ELSE 0 END), 0)
		   FROM gold_surfaces
		  WHERE lang = ? AND surface = ?`,
		lemma, pos, lang, form,
	).Scan(&total, &matching); err != nil {
		return err
	}
	if total >= goldConflictMinCases && matching == 0 {
		return fmt.Errorf("%w: %d gold occurrence(s) of %q analyze it differently than %s/%s",
			ErrOverrideConflictsWithGold, total, form, lemma, pos)
	}
	return nil
}

// maybePromoteGoldCandidate upserts a pending gold-case candidate when the
// same (surface → lemma/pos) correction has been accepted for at least
// GoldPromotionThreshold distinct submitting users. Counting runs inside the
// acceptance transaction, so the row being accepted is included.
func maybePromoteGoldCandidate(tx *sql.Tx, lang, form, lemma, pos, feats string) error {
	var supporters int
	if err := tx.QueryRow(
		`SELECT COUNT(DISTINCT user_id)
		   FROM parse_feedback
		  WHERE lang = ? AND lower(surface) = ? AND proposed_lemma = ? AND proposed_pos = ?
		    AND status = 'accepted'`,
		lang, form, lemma, pos,
	).Scan(&supporters); err != nil {
		return err
	}
	if supporters < GoldPromotionThreshold {
		return nil
	}
	_, err := tx.Exec(
		`INSERT INTO gold_candidates (lang, surface, lemma, pos, feats, supporter_count)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(lang, surface, lemma, pos) DO UPDATE SET
			supporter_count = excluded.supporter_count`,
		lang, form, lemma, pos, feats, supporters,
	)
	return err
}

// ReplaceGoldSurfaces atomically replaces the Phase-4 guard data for a
// language with the given aggregated gold analyses. Used by
// cmd/importgoldsurfaces.
func (d *DB) ReplaceGoldSurfaces(lang string, rows []GoldSurface) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM gold_surfaces WHERE lang = ?`, lang); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO gold_surfaces (lang, surface, lemma, pos, case_count) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, row := range rows {
		if _, err := stmt.Exec(lang, strings.ToLower(row.Surface), row.Lemma, row.POS, row.CaseCount); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GoldSurface is one aggregated gold analysis for the Phase-4 guard.
type GoldSurface struct {
	Surface   string
	Lemma     string
	POS       string
	CaseCount int
}

// GoldCandidate is one pending Phase-3 gold-case promotion.
type GoldCandidate struct {
	ID             int64
	Lang           string
	Surface        string
	Lemma          string
	POS            string
	Feats          string
	SupporterCount int
	Status         string
}

// ListGoldCandidates returns promotion candidates with the given status
// (all statuses when empty), newest first.
func (d *DB) ListGoldCandidates(status string) ([]GoldCandidate, error) {
	query := `SELECT id, lang, surface, lemma, pos, feats, supporter_count, status
	            FROM gold_candidates`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id DESC`
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GoldCandidate{}
	for rows.Next() {
		var c GoldCandidate
		if err := rows.Scan(&c.ID, &c.Lang, &c.Surface, &c.Lemma, &c.POS, &c.Feats, &c.SupporterCount, &c.Status); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MarkGoldCandidatesExported flips pending candidates to exported so the next
// export run only shows new arrivals.
func (d *DB) MarkGoldCandidatesExported(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.Exec(`UPDATE gold_candidates SET status = 'exported' WHERE id = ? AND status = 'pending'`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
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

// SupportedLanguages is the closed set of languages the app knows. Used by
// settings reads/writes to validate active_language and learning_languages.
var SupportedLanguages = []string{"FI", "ET"}

// IsSupportedLanguage reports whether lang is one of the app's supported
// language codes. Codes are case-sensitive uppercase ("FI", "ET").
func IsSupportedLanguage(lang string) bool {
	for _, supported := range SupportedLanguages {
		if lang == supported {
			return true
		}
	}
	return false
}

// UserLanguages extracts the learning_languages list and active_language from a
// user's settings, applying defaults so callers never see an empty list. The
// defaults match the schema: ["FI","ET"] with "FI" active.
func UserLanguages(settings map[string]interface{}) (learning []string, active string) {
	learning = nil
	if raw, ok := settings["learning_languages"].([]interface{}); ok {
		for _, item := range raw {
			if s, ok := item.(string); ok && IsSupportedLanguage(s) {
				learning = append(learning, s)
			}
		}
	}
	if len(learning) == 0 {
		learning = append([]string{}, SupportedLanguages...)
	}

	if s, ok := settings["active_language"].(string); ok && IsSupportedLanguage(s) {
		active = s
	}
	if active == "" || !containsString(learning, active) {
		active = learning[0]
	}
	return learning, active
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

// UpdateUserLanguages writes learning_languages and active_language into the
// user's settings_json, leaving other keys (new_per_day, retention, theme)
// untouched. Validation: each entry in learning must be a supported code,
// learning must be non-empty, and active must be one of learning.
func (d *DB) UpdateUserLanguages(userID int64, learning []string, active string) error {
	if len(learning) == 0 {
		return fmt.Errorf("at least one learning language is required")
	}
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(learning))
	for _, lang := range learning {
		if !IsSupportedLanguage(lang) {
			return fmt.Errorf("unsupported language: %s", lang)
		}
		if seen[lang] {
			continue
		}
		seen[lang] = true
		cleaned = append(cleaned, lang)
	}
	if !IsSupportedLanguage(active) {
		return fmt.Errorf("unsupported language: %s", active)
	}
	if !seen[active] {
		return fmt.Errorf("active language %s is not in learning_languages", active)
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var settingsJSON string
	if err := tx.QueryRow(`SELECT settings_json FROM users WHERE id = ?`, userID).Scan(&settingsJSON); err != nil {
		return err
	}
	settings := map[string]interface{}{}
	if strings.TrimSpace(settingsJSON) != "" {
		_ = json.Unmarshal([]byte(settingsJSON), &settings)
	}
	settings["learning_languages"] = cleaned
	settings["active_language"] = active
	updated, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE users SET settings_json = ? WHERE id = ?`, string(updated), userID); err != nil {
		return err
	}
	return tx.Commit()
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

// nextAlphaStepScheduleForRating is the public-alpha review scheduler. It uses
// fixed steps and intentionally does not implement FSRS; the existing
// card_state.fsrs_json column stores this small JSON state until the FSRS
// migration replaces it.
func nextAlphaStepScheduleForRating(schedule ReviewSchedule, rating string) (time.Time, ReviewSchedule) {
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

// ensureUserKnownLemmasSource adds the `source` column to user_known_lemmas
// on existing databases. New columns are added with DEFAULT 'manual' so any
// pre-migration row is treated as user-provided and survives an Anki sync
// with scope='anki' (the default replace behaviour).
func (d *DB) ensureUserKnownLemmasSource() error {
	has, err := d.tableHasColumn("user_known_lemmas", "source")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = d.db.Exec(`ALTER TABLE user_known_lemmas ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'`)
	return err
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
