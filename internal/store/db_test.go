package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if !columnExists(t, d.db, "lemmas", "parse_feedback_id") {
		t.Error("lemmas.parse_feedback_id missing on fresh DB")
	}
	if !columnExists(t, d.db, "forms", "parse_feedback_id") {
		t.Error("forms.parse_feedback_id missing on fresh DB")
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
	if columnExists(t, raw, "lemmas", "parse_feedback_id") {
		t.Fatal("parse_feedback_id unexpectedly present in old-shape seed")
	}
	if tableExists(t, raw, "translations") {
		t.Fatal("translations unexpectedly present in old-shape seed")
	}

	if err := EnsureLexicalEnrichmentColumns(raw); err != nil {
		t.Fatalf("EnsureLexicalEnrichmentColumns: %v", err)
	}
	if err := EnsureCorrectionBackpointerColumns(raw); err != nil {
		t.Fatalf("EnsureCorrectionBackpointerColumns: %v", err)
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
	if !columnExists(t, raw, "lemmas", "parse_feedback_id") {
		t.Error("lemmas.parse_feedback_id not added by backfill")
	}
	if !columnExists(t, raw, "forms", "parse_feedback_id") {
		t.Error("forms.parse_feedback_id not added by backfill")
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
	if err := EnsureCorrectionBackpointerColumns(raw); err != nil {
		t.Errorf("EnsureCorrectionBackpointerColumns rerun: %v", err)
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

func TestManualKnownActionsUpgradeAnkiSource(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "source-upgrade@example.com")
	for _, l := range []struct{ lemma, pos string }{
		{"kissa", "NOUN"},
		{"koira", "NOUN"},
		{"juosta", "VERB"},
	} {
		if err := db.UpsertLemma(l.lemma, l.pos, "x", "FI"); err != nil {
			t.Fatalf("UpsertLemma %s: %v", l.lemma, err)
		}
		if err := db.UpsertForm(l.lemma, l.lemma, l.pos, "FI"); err != nil {
			t.Fatalf("UpsertForm %s: %v", l.lemma, err)
		}
	}

	if _, _, err := db.ImportKnownWords(user.ID, "FI", []string{"kissa", "koira", "juosta"}, SourceAnki); err != nil {
		t.Fatalf("ImportKnownWords anki: %v", err)
	}
	if _, _, err := db.ImportKnownWords(user.ID, "FI", []string{"kissa"}, SourceManual); err != nil {
		t.Fatalf("ImportKnownWords manual: %v", err)
	}
	if err := db.MarkLemmaKnown(user.ID, "FI", "koira", "NOUN"); err != nil {
		t.Fatalf("MarkLemmaKnown: %v", err)
	}
	cardID, err := db.EnsureCard(user.ID, "FI", "juosta", "VERB")
	if err != nil {
		t.Fatalf("EnsureCard: %v", err)
	}
	if err := db.MarkCardKnown(user.ID, cardID); err != nil {
		t.Fatalf("MarkCardKnown: %v", err)
	}

	_, removed, _, err := db.ReplaceKnownWords(user.ID, "FI", nil, SourceAnki)
	if err != nil {
		t.Fatalf("ReplaceKnownWords: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("default Anki sync removed manually confirmed words: %+v", removed)
	}

	got, err := db.ListKnownWords(user.ID, "FI")
	if err != nil {
		t.Fatalf("ListKnownWords: %v", err)
	}
	seen := map[string]bool{}
	for _, l := range got {
		seen[l.Lemma+"/"+l.POS] = true
	}
	for _, want := range []string{"kissa/NOUN", "koira/NOUN", "juosta/VERB"} {
		if !seen[want] {
			t.Fatalf("known words after Anki sync=%v, missing %s", seen, want)
		}
	}
}

func TestUpdateDeckTitleAndPublicRollsBackOnTitleFailure(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "deck-atomic@example.com")
	deckID, err := db.CreateDeck(user.ID, "Original title", "FI")
	if err != nil {
		t.Fatalf("CreateDeck: %v", err)
	}
	if _, err := db.db.Exec(`
		CREATE TRIGGER fail_deck_title_update
		BEFORE UPDATE OF title ON decks
		WHEN NEW.title = 'explode'
		BEGIN
			SELECT RAISE(FAIL, 'forced title update failure');
		END;
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := db.UpdateDeckTitleAndPublic(user.ID, deckID, "explode", true); err == nil {
		t.Fatal("UpdateDeckTitleAndPublic succeeded despite forced title failure")
	}

	var title string
	var isPublic bool
	if err := db.db.QueryRow(
		`SELECT title, is_public FROM decks WHERE id = ?`,
		deckID,
	).Scan(&title, &isPublic); err != nil {
		t.Fatalf("read deck after failed patch: %v", err)
	}
	if title != "Original title" {
		t.Fatalf("title=%q want original after rollback", title)
	}
	if isPublic {
		t.Fatal("is_public changed despite title failure")
	}
}

func TestDeleteUserCascadeRemovesPrivateRows(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "delete-me@example.com")
	other := createTestUser(t, db, "keep-me@example.com")

	ownedDeckID := createSingleTokenDeck(t, db, user.ID, "FI", "Kissa.", "Kissa", "kissa", "NOUN")
	publicDeckID := createSingleTokenDeck(t, db, other.ID, "FI", "Koira.", "Koira", "koira", "NOUN")
	if err := db.SetDeckIsPublic(publicDeckID, true); err != nil {
		t.Fatalf("SetDeckIsPublic: %v", err)
	}
	if err := db.SubscribeUserToPublicDeck(user.ID, publicDeckID); err != nil {
		t.Fatalf("SubscribeUserToPublicDeck: %v", err)
	}
	if err := db.MarkLemmaKnown(user.ID, "FI", "hauki", "NOUN"); err != nil {
		t.Fatalf("MarkLemmaKnown: %v", err)
	}
	if cardID, err := db.EnsureCard(user.ID, "FI", "ahven", "NOUN"); err != nil {
		t.Fatalf("EnsureCard: %v", err)
	} else if err := db.MarkCardIgnored(user.ID, cardID); err != nil {
		t.Fatalf("MarkCardIgnored: %v", err)
	}
	if err := db.CreateSession(user.ID, "delete-session-hash", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	parseID, err := db.CreateParseSession(&user.ID, "FI", "custom", "Kissa.", 1, 1)
	if err != nil {
		t.Fatalf("CreateParseSession: %v", err)
	}
	if _, err := db.CreateParseFeedback(ParseFeedback{
		ParseSessionID: parseID,
		UserID:         user.ID,
		Lang:           "FI",
		Parser:         "custom",
		Surface:        "Kissa",
		OriginalLemma:  "kissa",
		OriginalPOS:    "NOUN",
		ProposedLemma:  "kissa",
		ProposedPOS:    "NOUN",
	}); err != nil {
		t.Fatalf("CreateParseFeedback: %v", err)
	}

	if err := db.DeleteUserCascade(user.ID); err != nil {
		t.Fatalf("DeleteUserCascade: %v", err)
	}

	if _, err := db.GetUserByID(user.ID); err != sql.ErrNoRows {
		t.Fatalf("GetUserByID deleted user err=%v want sql.ErrNoRows", err)
	}
	for _, check := range []struct {
		name  string
		query string
		args  []any
	}{
		{"user sessions", `SELECT COUNT(*) FROM user_sessions WHERE user_id = ?`, []any{user.ID}},
		{"parse sessions", `SELECT COUNT(*) FROM parse_sessions WHERE user_id = ?`, []any{user.ID}},
		{"parse feedback", `SELECT COUNT(*) FROM parse_feedback WHERE user_id = ? OR parse_session_id = ? OR reviewed_by_user_id = ?`, []any{user.ID, parseID, user.ID}},
		{"owned decks", `SELECT COUNT(*) FROM decks WHERE user_id = ?`, []any{user.ID}},
		{"owned sentences", `SELECT COUNT(*) FROM sentences WHERE deck_id = ?`, []any{ownedDeckID}},
		{"owned occurrence", `SELECT COUNT(*) FROM occurrence WHERE deck_id = ?`, []any{ownedDeckID}},
		{"subscriptions", `SELECT COUNT(*) FROM user_deck_subscriptions WHERE user_id = ? OR deck_id = ?`, []any{user.ID, ownedDeckID}},
		{"cards", `SELECT COUNT(*) FROM cards WHERE user_id = ?`, []any{user.ID}},
		{"card state", `SELECT COUNT(*) FROM card_state WHERE card_id NOT IN (SELECT id FROM cards)`, nil},
		{"known lemmas", `SELECT COUNT(*) FROM user_known_lemmas WHERE user_id = ?`, []any{user.ID}},
		{"ignored lemmas", `SELECT COUNT(*) FROM user_ignored_lemmas WHERE user_id = ?`, []any{user.ID}},
	} {
		if got := countRows(t, db, check.query, check.args...); got != 0 {
			t.Fatalf("%s rows=%d want 0", check.name, got)
		}
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM decks WHERE id = ? AND user_id = ?`, publicDeckID, other.ID); got != 1 {
		t.Fatalf("other user's public deck rows=%d want 1", got)
	}
}

func TestParseSessionHistoryListAndDelete(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "parse-history@example.com")
	other := createTestUser(t, db, "parse-history-other@example.com")

	deckID := createSingleTokenDeck(t, db, user.ID, "FI", "Kissa.", "Kissa", "kissa", "NOUN")
	parseID, err := db.CreateParseSession(&user.ID, "FI", "custom", "Kissa juoksee.\nSe on nopea.", 4, 3)
	if err != nil {
		t.Fatalf("CreateParseSession user: %v", err)
	}
	if err := db.SetDeckParseSession(user.ID, deckID, parseID); err != nil {
		t.Fatalf("SetDeckParseSession: %v", err)
	}
	if _, err := db.CreateParseFeedback(ParseFeedback{
		ParseSessionID: parseID,
		UserID:         user.ID,
		Lang:           "FI",
		Parser:         "custom",
		Surface:        "Kissa",
		OriginalLemma:  "kissa",
		OriginalPOS:    "NOUN",
		ProposedLemma:  "kissa",
		ProposedPOS:    "NOUN",
	}); err != nil {
		t.Fatalf("CreateParseFeedback: %v", err)
	}
	otherParseID, err := db.CreateParseSession(&other.ID, "ET", "custom", "Koer jookseb.", 2, 2)
	if err != nil {
		t.Fatalf("CreateParseSession other: %v", err)
	}

	sessions, err := db.ListUserParseSessions(user.ID)
	if err != nil {
		t.Fatalf("ListUserParseSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions=%d want 1: %+v", len(sessions), sessions)
	}
	got := sessions[0]
	if got.ID != parseID || got.DeckCount != 1 || got.FeedbackCount != 1 {
		t.Fatalf("unexpected session summary: %+v", got)
	}
	if strings.Contains(got.SourcePreview, "\n") || !strings.Contains(got.SourcePreview, "Kissa juoksee") {
		t.Fatalf("source preview=%q", got.SourcePreview)
	}

	if err := db.DeleteUserParseSession(user.ID, otherParseID); err != sql.ErrNoRows {
		t.Fatalf("DeleteUserParseSession other err=%v want sql.ErrNoRows", err)
	}
	if err := db.DeleteUserParseSession(user.ID, parseID); err != nil {
		t.Fatalf("DeleteUserParseSession user: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM parse_sessions WHERE id = ?`, parseID); got != 0 {
		t.Fatalf("deleted parse session rows=%d want 0", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM parse_feedback WHERE parse_session_id = ?`, parseID); got != 0 {
		t.Fatalf("deleted parse feedback rows=%d want 0", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM decks WHERE id = ? AND parse_session_id IS NULL`, deckID); got != 1 {
		t.Fatalf("deck parse_session_id NULL rows=%d want 1", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM parse_sessions WHERE id = ?`, otherParseID); got != 1 {
		t.Fatalf("other parse session rows=%d want 1", got)
	}
}

func TestAcceptedParseFeedbackWritesCustomOverrideAndChangesLookup(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "feedback-override-user@example.com")
	admin := createTestUser(t, db, "feedback-override-admin@example.com")

	if _, err := db.db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES ('oldlemma', 'NOUN', 'old gloss', 'FI', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed old lemma: %v", err)
	}
	if _, err := db.db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority)
		 VALUES ('loopword', 'oldlemma', 'NOUN', 'FI', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed old form: %v", err)
	}

	before := db.BatchLookupForms([]string{"loopword"}, "FI", "custom")["loopword"]
	if before.Lemma != "oldlemma" || before.POS != "NOUN" {
		t.Fatalf("before acceptance got %s/%s, want oldlemma/NOUN", before.Lemma, before.POS)
	}

	parseID, err := db.CreateParseSession(&user.ID, "FI", "custom", "loopword", 1, 1)
	if err != nil {
		t.Fatalf("CreateParseSession: %v", err)
	}
	feedbackID, err := db.CreateParseFeedback(ParseFeedback{
		ParseSessionID: parseID,
		UserID:         user.ID,
		Lang:           "FI",
		Parser:         "custom",
		Surface:        "Loopword",
		OriginalLemma:  "oldlemma",
		OriginalPOS:    "NOUN",
		ProposedLemma:  "newlemma",
		ProposedPOS:    "VERB",
	})
	if err != nil {
		t.Fatalf("CreateParseFeedback: %v", err)
	}
	if err := db.ReviewParseFeedback(feedbackID, admin.ID, "accepted", "promote"); err != nil {
		t.Fatalf("ReviewParseFeedback: %v", err)
	}

	after := db.BatchLookupForms([]string{"loopword"}, "FI", "custom")["loopword"]
	if after.Lemma != "newlemma" || after.POS != "VERB" {
		t.Fatalf("after acceptance got %s/%s, want newlemma/VERB", after.Lemma, after.POS)
	}

	var formSource string
	var formPriority int
	var formFeedbackID int64
	if err := db.db.QueryRow(
		`SELECT source, source_priority, parse_feedback_id
		 FROM forms
		 WHERE form = 'loopword' AND lang = 'FI' AND lemma = 'newlemma' AND pos = 'VERB'`,
	).Scan(&formSource, &formPriority, &formFeedbackID); err != nil {
		t.Fatalf("lookup override form row: %v", err)
	}
	if formSource != SourceCustomOverrides || formPriority != CustomOverridesSourcePriority || formFeedbackID != feedbackID {
		t.Fatalf("form provenance got %q/%d/%d, want %q/%d/%d",
			formSource, formPriority, formFeedbackID, SourceCustomOverrides, CustomOverridesSourcePriority, feedbackID)
	}

	var lemmaSource string
	var lemmaPriority int
	var lemmaFeedbackID int64
	if err := db.db.QueryRow(
		`SELECT source, source_priority, parse_feedback_id
		 FROM lemmas
		 WHERE lemma = 'newlemma' AND lang = 'FI' AND pos = 'VERB'`,
	).Scan(&lemmaSource, &lemmaPriority, &lemmaFeedbackID); err != nil {
		t.Fatalf("lookup override lemma row: %v", err)
	}
	if lemmaSource != SourceCustomOverrides || lemmaPriority != CustomOverridesSourcePriority || lemmaFeedbackID != feedbackID {
		t.Fatalf("lemma provenance got %q/%d/%d, want %q/%d/%d",
			lemmaSource, lemmaPriority, lemmaFeedbackID, SourceCustomOverrides, CustomOverridesSourcePriority, feedbackID)
	}
}

func TestAcceptedParseFeedbackReplacesPriorCustomOverrideForSurface(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "feedback-replace-user@example.com")
	admin := createTestUser(t, db, "feedback-replace-admin@example.com")

	if _, err := db.db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority)
		 VALUES ('loopword', 'dictlemma', 'NOUN', 'FI', 'kaikki', 10)`,
	); err != nil {
		t.Fatalf("seed dict form: %v", err)
	}

	parseID, err := db.CreateParseSession(&user.ID, "FI", "custom", "loopword", 1, 1)
	if err != nil {
		t.Fatalf("CreateParseSession: %v", err)
	}
	firstID, err := db.CreateParseFeedback(ParseFeedback{
		ParseSessionID: parseID,
		UserID:         user.ID,
		Lang:           "FI",
		Parser:         "custom",
		Surface:        "Loopword",
		OriginalLemma:  "dictlemma",
		OriginalPOS:    "NOUN",
		ProposedLemma:  "aaalemma",
		ProposedPOS:    "NOUN",
	})
	if err != nil {
		t.Fatalf("CreateParseFeedback first: %v", err)
	}
	if err := db.ReviewParseFeedback(firstID, admin.ID, "accepted", "first"); err != nil {
		t.Fatalf("ReviewParseFeedback first: %v", err)
	}
	secondID, err := db.CreateParseFeedback(ParseFeedback{
		ParseSessionID: parseID,
		UserID:         user.ID,
		Lang:           "FI",
		Parser:         "custom",
		Surface:        "Loopword",
		OriginalLemma:  "aaalemma",
		OriginalPOS:    "NOUN",
		ProposedLemma:  "zzlemma",
		ProposedPOS:    "VERB",
	})
	if err != nil {
		t.Fatalf("CreateParseFeedback second: %v", err)
	}
	if err := db.ReviewParseFeedback(secondID, admin.ID, "accepted", "replace"); err != nil {
		t.Fatalf("ReviewParseFeedback second: %v", err)
	}

	got := db.BatchLookupForms([]string{"loopword"}, "FI", "custom")["loopword"]
	if got.Lemma != "zzlemma" || got.POS != "VERB" {
		t.Fatalf("lookup got %s/%s, want latest accepted zzlemma/VERB", got.Lemma, got.POS)
	}
	all := db.BatchLookupAllForms([]string{"loopword"}, "FI", "custom")["loopword"]
	if len(all) != 1 || all[0].Lemma != "zzlemma" || all[0].POS != "VERB" {
		t.Fatalf("all-form lookup got %+v, want only latest accepted zzlemma/VERB", all)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM forms WHERE form = 'loopword' AND lang = 'FI' AND source = ?`, SourceCustomOverrides); got != 1 {
		t.Fatalf("custom override rows=%d want 1", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM forms WHERE form = 'loopword' AND lang = 'FI' AND source = ? AND parse_feedback_id = ?`, SourceCustomOverrides, secondID); got != 1 {
		t.Fatalf("latest custom override rows=%d want 1", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM forms WHERE form = 'loopword' AND lang = 'FI' AND lemma = 'dictlemma' AND source = 'kaikki'`); got != 1 {
		t.Fatalf("lower-priority dictionary rows=%d want preserved", got)
	}
}

func TestDeleteUserParseSessionsBulk(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "parse-history-bulk@example.com")
	other := createTestUser(t, db, "parse-history-bulk-other@example.com")

	parseID1, err := db.CreateParseSession(&user.ID, "FI", "custom", "Kissa.", 1, 1)
	if err != nil {
		t.Fatalf("CreateParseSession user 1: %v", err)
	}
	parseID2, err := db.CreateParseSession(&user.ID, "ET", "custom", "Koer.", 1, 1)
	if err != nil {
		t.Fatalf("CreateParseSession user 2: %v", err)
	}
	otherParseID, err := db.CreateParseSession(&other.ID, "FI", "custom", "Koira.", 1, 1)
	if err != nil {
		t.Fatalf("CreateParseSession other: %v", err)
	}
	for _, parseID := range []int64{parseID1, parseID2} {
		if _, err := db.CreateParseFeedback(ParseFeedback{
			ParseSessionID: parseID,
			UserID:         user.ID,
			Lang:           "FI",
			Parser:         "custom",
			Surface:        "Kissa",
			OriginalLemma:  "kissa",
			OriginalPOS:    "NOUN",
			ProposedLemma:  "kissa",
			ProposedPOS:    "NOUN",
		}); err != nil {
			t.Fatalf("CreateParseFeedback: %v", err)
		}
	}

	deleted, err := db.DeleteUserParseSessions(user.ID)
	if err != nil {
		t.Fatalf("DeleteUserParseSessions: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted=%d want 2", deleted)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM parse_sessions WHERE user_id = ?`, user.ID); got != 0 {
		t.Fatalf("user parse sessions=%d want 0", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM parse_feedback WHERE user_id = ?`, user.ID); got != 0 {
		t.Fatalf("user parse feedback=%d want 0", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM parse_sessions WHERE id = ?`, otherParseID); got != 1 {
		t.Fatalf("other parse session rows=%d want 1", got)
	}
}

func TestPurgeParseSessionSourceTextPreservesDerivedRecords(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "parse-retention@example.com")

	deckID := createSingleTokenDeck(t, db, user.ID, "FI", "Kissa.", "Kissa", "kissa", "NOUN")
	oldParseID, err := db.CreateParseSession(&user.ID, "FI", "custom", "Old retained source.", 3, 3)
	if err != nil {
		t.Fatalf("CreateParseSession old: %v", err)
	}
	if err := db.SetDeckParseSession(user.ID, deckID, oldParseID); err != nil {
		t.Fatalf("SetDeckParseSession: %v", err)
	}
	if _, err := db.CreateParseFeedback(ParseFeedback{
		ParseSessionID: oldParseID,
		UserID:         user.ID,
		Lang:           "FI",
		Parser:         "custom",
		Surface:        "Old",
		OriginalLemma:  "old",
		OriginalPOS:    "NOUN",
		ProposedLemma:  "old",
		ProposedPOS:    "NOUN",
	}); err != nil {
		t.Fatalf("CreateParseFeedback: %v", err)
	}
	recentParseID, err := db.CreateParseSession(&user.ID, "ET", "custom", "Recent retained source.", 3, 3)
	if err != nil {
		t.Fatalf("CreateParseSession recent: %v", err)
	}
	oldCreatedAt := sqliteTime(time.Now().UTC().AddDate(0, 0, -DefaultParseSourceRetentionDays-1))
	if _, err := db.db.Exec(`UPDATE parse_sessions SET created_at = ? WHERE id = ?`, oldCreatedAt, oldParseID); err != nil {
		t.Fatalf("backdate old parse session: %v", err)
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -DefaultParseSourceRetentionDays)
	count, err := db.CountPurgeableParseSessionSourceText(cutoff)
	if err != nil {
		t.Fatalf("CountPurgeableParseSessionSourceText: %v", err)
	}
	if count != 1 {
		t.Fatalf("purgeable count=%d want 1", count)
	}
	purged, err := db.PurgeParseSessionSourceText(cutoff)
	if err != nil {
		t.Fatalf("PurgeParseSessionSourceText: %v", err)
	}
	if purged != 1 {
		t.Fatalf("purged=%d want 1", purged)
	}

	oldSession, err := db.GetParseSession(oldParseID)
	if err != nil {
		t.Fatalf("GetParseSession old: %v", err)
	}
	if oldSession.SourceText != "" {
		t.Fatalf("old source_text=%q want purged empty string", oldSession.SourceText)
	}
	recentSession, err := db.GetParseSession(recentParseID)
	if err != nil {
		t.Fatalf("GetParseSession recent: %v", err)
	}
	if recentSession.SourceText != "Recent retained source." {
		t.Fatalf("recent source_text=%q", recentSession.SourceText)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM decks WHERE id = ? AND parse_session_id = ?`, deckID, oldParseID); got != 1 {
		t.Fatalf("deck link rows=%d want 1", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM parse_feedback WHERE parse_session_id = ?`, oldParseID); got != 1 {
		t.Fatalf("feedback rows=%d want 1", got)
	}
	sessions, err := db.ListUserParseSessions(user.ID)
	if err != nil {
		t.Fatalf("ListUserParseSessions: %v", err)
	}
	foundPurgedPreview := false
	for _, session := range sessions {
		if session.ID == oldParseID && session.SourcePreview == "(source text purged)" {
			foundPurgedPreview = true
		}
	}
	if !foundPurgedPreview {
		t.Fatalf("purged session preview not found in history: %+v", sessions)
	}
}

func countRows(t *testing.T, db *DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
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
