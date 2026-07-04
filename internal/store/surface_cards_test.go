package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// cardRow is a small helper to read a card + its state by id.
func cardSurfaceNorm(t *testing.T, db *DB, cardID int64) string {
	t.Helper()
	var s string
	if err := db.db.QueryRow(`SELECT surface_norm FROM cards WHERE id = ?`, cardID).Scan(&s); err != nil {
		t.Fatalf("read surface_norm for card %d: %v", cardID, err)
	}
	return s
}

// TestSurfaceCardCreationSameWordTwoDecksOneCard proves that the same surface +
// sense encountered in two decks produces exactly one global card: the card
// identity is (user, lang, surface_norm, lemma, pos), so re-encountering the
// same review unit does not fork a second card.
func TestSurfaceCardCreationSameWordTwoDecksOneCard(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "learner@example.com")

	createSingleTokenDeck(t, db, user.ID, "FI", "Koira juoksee.", "koira", "koira", "NOUN")
	createSingleTokenDeck(t, db, user.ID, "FI", "Koira nukkuu.", "koira", "koira", "NOUN")

	var n int
	if err := db.db.QueryRow(
		`SELECT COUNT(*) FROM cards WHERE user_id = ? AND lang = 'FI' AND lemma = 'koira' AND pos = 'NOUN'`,
		user.ID,
	).Scan(&n); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if n != 1 {
		t.Fatalf("same surface+sense in two decks made %d cards, want 1", n)
	}
}

// TestSurfaceCardCreationHomographTwoSenses proves a homograph (same surface,
// two distinct (lemma,pos) senses) yields two sense cards that share a surface.
// This is the settled resolution: homographs are NOT collapsed to a pure
// surface key.
func TestSurfaceCardCreationHomographTwoSenses(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "learner@example.com")

	// Estonian "joon" is a classic homograph: noun "line" and 1Sg of verb
	// "jooma" (drink). Both share the surface "joon".
	deckID, err := db.CreateDeckWithSentences(user.ID, "Homograph deck", "ET", []DeckSentenceInput{
		{
			Text: "Ma joon vett. Vaata seda joont.",
			Tokens: []DeckTokenInput{
				{TokenIx: 0, Form: "joon", Lemma: "jooma", POS: "VERB"},
				{TokenIx: 1, Form: "joon", Lemma: "joon", POS: "NOUN"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateDeckWithSentences: %v", err)
	}
	_ = deckID

	rows, err := db.db.Query(
		`SELECT surface_norm, lemma, pos FROM cards
		  WHERE user_id = ? AND lang = 'ET' AND surface_norm = 'joon'
		  ORDER BY lemma`,
		user.ID,
	)
	if err != nil {
		t.Fatalf("query joon cards: %v", err)
	}
	defer rows.Close()
	var senses [][2]string
	for rows.Next() {
		var sn, lemma, pos string
		if err := rows.Scan(&sn, &lemma, &pos); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if sn != "joon" {
			t.Fatalf("surface_norm=%q want joon", sn)
		}
		senses = append(senses, [2]string{lemma, pos})
	}
	if len(senses) != 2 {
		t.Fatalf("homograph produced %d sense cards, want 2: %v", len(senses), senses)
	}
	if senses[0] != [2]string{"jooma", "VERB"} || senses[1] != [2]string{"joon", "NOUN"} {
		t.Fatalf("unexpected senses %v", senses)
	}
}

// TestSurfaceCardCreationDistinctSurfacesSameLemma proves that the same lemma
// under two different inflections seeds two surface cards — surface-in-context
// is the review identity, so "koira" and "koiran" are separate cards.
func TestSurfaceCardCreationDistinctSurfacesSameLemma(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "learner@example.com")

	_, err := db.CreateDeckWithSentences(user.ID, "Inflections", "FI", []DeckSentenceInput{
		{
			Text: "Koira nukkuu. Koiran häntä.",
			Tokens: []DeckTokenInput{
				{TokenIx: 0, Form: "Koira", Lemma: "koira", POS: "NOUN"},
				{TokenIx: 1, Form: "Koiran", Lemma: "koira", POS: "NOUN"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateDeckWithSentences: %v", err)
	}

	rows, err := db.db.Query(
		`SELECT surface_norm FROM cards
		  WHERE user_id = ? AND lang = 'FI' AND lemma = 'koira' AND pos = 'NOUN'
		  ORDER BY surface_norm`,
		user.ID,
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var surfaces []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		surfaces = append(surfaces, s)
	}
	if len(surfaces) != 2 || surfaces[0] != "koira" || surfaces[1] != "koiran" {
		t.Fatalf("distinct surfaces %v want [koira koiran]", surfaces)
	}
}

// TestReviewCardShowsCorpusExampleSentence proves the starter-deck example
// wiring reaches the review payload: when a card's deck sentence is a real
// multi-word corpus sentence and the occurrence highlights an inflected form,
// GetNextReviewCard returns that whole sentence as the card's example (not just
// the lemma), and the card surface is the inflected form. This is the contract
// cmd/pickexamples + cmd/seedcolddeck rely on for starter cards to carry
// example sentences.
func TestReviewCardShowsCorpusExampleSentence(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "learner@example.com")

	const sentence = "Kissa oli pöydän alla."
	_, err := db.CreateDeckWithSentences(user.ID, "Top words", "FI", []DeckSentenceInput{
		{
			Text: sentence,
			Tokens: []DeckTokenInput{
				// Inflected form "oli" of lemma "olla" at its real token index.
				{TokenIx: 1, Form: "oli", Lemma: "olla", POS: "VERB"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateDeckWithSentences: %v", err)
	}

	card, err := db.GetNextReviewCard(user.ID, nil, "FI")
	if err != nil {
		t.Fatalf("GetNextReviewCard: %v", err)
	}
	if card == nil {
		t.Fatal("expected a card for the seeded olla example")
	}
	if card.SentenceText != sentence {
		t.Fatalf("SentenceText=%q, want the corpus sentence %q", card.SentenceText, sentence)
	}
	if card.Surface != "oli" {
		t.Fatalf("Surface=%q, want the inflected form \"oli\"", card.Surface)
	}
	if card.Lemma != "olla" || card.POS != "VERB" {
		t.Fatalf("card sense=%s/%s, want olla/VERB", card.Lemma, card.POS)
	}
}

// TestReviewCardHomographNote proves GetNextReviewCard surfaces a homograph
// note naming the other sense when two cards share a surface.
func TestReviewCardHomographNote(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "learner@example.com")

	_, err := db.CreateDeckWithSentences(user.ID, "Homograph deck", "ET", []DeckSentenceInput{
		{
			Text: "Ma joon vett. Vaata seda joont.",
			Tokens: []DeckTokenInput{
				{TokenIx: 0, Form: "joon", Lemma: "jooma", POS: "VERB"},
				{TokenIx: 1, Form: "joon", Lemma: "joon", POS: "NOUN"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateDeckWithSentences: %v", err)
	}

	// Pull cards until we get one whose surface is "joon"; assert it carries a
	// homograph note referencing the other sense.
	sawNote := false
	for i := 0; i < 4; i++ {
		card, err := db.GetNextReviewCard(user.ID, nil, "ET")
		if err != nil {
			t.Fatalf("GetNextReviewCard: %v", err)
		}
		if card == nil {
			break
		}
		if card.Surface == "joon" {
			if card.HomographNote == "" {
				t.Fatalf("joon card (%s/%s) missing homograph note", card.Lemma, card.POS)
			}
			sawNote = true
		}
		// Answer it so the next call returns the other card.
		if err := db.RecordReviewAnswer(user.ID, card.CardID, "good"); err != nil {
			t.Fatalf("RecordReviewAnswer: %v", err)
		}
	}
	if !sawNote {
		t.Fatal("never observed a joon card with a homograph note")
	}
}

// TestSurfaceScopeQuarantineSuppressesCard proves a surface-only quarantine
// issue (empty lemma/pos) now suppresses review CARDS whose surface_norm
// matches — not just deck occurrences. Cards gained a surface_norm column, so
// surface-scope suppression reaches them.
func TestSurfaceScopeQuarantineSuppressesCard(t *testing.T) {
	db := newTestDB(t)
	admin := createTestUser(t, db, "admin@example.com")
	user := createTestUser(t, db, "learner@example.com")

	createSingleTokenDeck(t, db, user.ID, "FI", "Koira.", "Koira", "koira", "NOUN")

	// Card is present before quarantine.
	card, err := db.GetNextReviewCard(user.ID, nil, "FI")
	if err != nil {
		t.Fatalf("GetNextReviewCard before: %v", err)
	}
	if card == nil {
		t.Fatal("expected a koira card before quarantine")
	}

	// Surface-only quarantine (no lemma/pos) on the normalized surface "koira".
	quarantineIssueForSurface(t, db, admin.ID, user.ID, "FI", "Koira", "", "", AlphaClassSourceIssue)

	card, err = db.GetNextReviewCard(user.ID, nil, "FI")
	if err != nil {
		t.Fatalf("GetNextReviewCard after: %v", err)
	}
	if card != nil {
		t.Fatalf("surface-scope quarantine did not suppress card: %+v", card)
	}
	if n, err := db.CountDueCards(user.ID); err != nil || n != 0 {
		t.Fatalf("CountDueCards after surface quarantine = %d (err %v), want 0", n, err)
	}
}

// TestSurfaceCardMigrationPreservesStateAndBackfillsSurface builds a legacy
// (lang-scoped, no surface_norm) cards+card_state+review_log database, runs the
// full migration via NewDB, and asserts:
//   - the same number of cards survive (no scheduler state dropped),
//   - each card's scheduler state (fsrs_json, next_due, last_answer_at) is intact,
//   - surface_norm is backfilled from the most-frequent occurrence surface,
//     falling back to the lemma when there is no occurrence,
//   - review_log rows still point at surviving cards.
func TestSurfaceCardMigrationPreservesStateAndBackfillsSurface(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}

	// Legacy schema: cards already lang-scoped (post ensureLangScopedCardsTable)
	// but WITHOUT surface_norm — the pre-surface-migration state.
	legacySchema := `
	CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE,
		email_verified INTEGER DEFAULT 0,
		is_admin INTEGER DEFAULT 0,
		settings_json TEXT DEFAULT '{}'
	);
	CREATE TABLE decks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		lang TEXT NOT NULL,
		parse_session_id INTEGER,
		is_public INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE sentences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		deck_id INTEGER NOT NULL,
		text TEXT NOT NULL,
		lang TEXT NOT NULL
	);
	CREATE TABLE occurrence (
		deck_id INTEGER NOT NULL,
		sentence_id INTEGER NOT NULL,
		token_ix INTEGER NOT NULL,
		surface TEXT NOT NULL DEFAULT '',
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		UNIQUE(deck_id, sentence_id, token_ix, lemma, pos)
	);
	CREATE TABLE cards (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		lang TEXT NOT NULL DEFAULT '',
		lemma TEXT NOT NULL,
		pos TEXT NOT NULL,
		mwe_id INTEGER,
		UNIQUE(user_id, lang, lemma, pos, mwe_id)
	);
	CREATE TABLE card_state (
		card_id INTEGER PRIMARY KEY,
		fsrs_json TEXT,
		next_due DATETIME,
		last_answer_at DATETIME,
		introduced_at DATETIME
	);
	CREATE TABLE review_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		card_id INTEGER NOT NULL,
		lang TEXT NOT NULL DEFAULT '',
		rating TEXT NOT NULL,
		reviewed_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := legacy.Exec(legacySchema); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	stmts := []string{
		`INSERT INTO users (id, email) VALUES (1, 'legacy@example.com')`,
		`INSERT INTO decks (id, user_id, title, lang) VALUES (10, 1, 'Deck', 'FI')`,
		`INSERT INTO sentences (id, deck_id, text, lang) VALUES (100, 10, 'Koiran koira koiran.', 'FI')`,
		// "koiran" appears twice, "koira" once -> most frequent surface is "koiran".
		`INSERT INTO occurrence (deck_id, sentence_id, token_ix, surface, lemma, pos) VALUES (10, 100, 0, 'Koiran', 'koira', 'NOUN')`,
		`INSERT INTO occurrence (deck_id, sentence_id, token_ix, surface, lemma, pos) VALUES (10, 100, 1, 'koira', 'koira', 'NOUN')`,
		`INSERT INTO occurrence (deck_id, sentence_id, token_ix, surface, lemma, pos) VALUES (10, 100, 2, 'koiran', 'koira', 'NOUN')`,
		// Card 7: koira/NOUN with review history (should backfill surface "koiran").
		`INSERT INTO cards (id, user_id, lang, lemma, pos, mwe_id) VALUES (7, 1, 'FI', 'koira', 'NOUN', NULL)`,
		`INSERT INTO card_state (card_id, fsrs_json, next_due, last_answer_at, introduced_at) VALUES (7, '{"step":2,"streak":3}', '2026-05-01 10:00:00', '2026-04-29 09:00:00', '2026-04-20 08:00:00')`,
		`INSERT INTO review_log (user_id, card_id, lang, rating, reviewed_at) VALUES (1, 7, 'FI', 'good', '2026-04-29 09:00:00')`,
		// Card 8: kissa/NOUN with NO occurrence -> falls back to lemma "kissa".
		`INSERT INTO cards (id, user_id, lang, lemma, pos, mwe_id) VALUES (8, 1, 'FI', 'kissa', 'NOUN', NULL)`,
		`INSERT INTO card_state (card_id, fsrs_json, next_due, last_answer_at, introduced_at) VALUES (8, NULL, NULL, NULL, NULL)`,
	}
	for _, s := range stmts {
		if _, err := legacy.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	migrated, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB migration: %v", err)
	}
	t.Cleanup(func() { migrated.Close() })

	// Same number of cards: no scheduler state dropped.
	var cardCount int
	if err := migrated.db.QueryRow(`SELECT COUNT(*) FROM cards`).Scan(&cardCount); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if cardCount != 2 {
		t.Fatalf("card count after migration = %d, want 2", cardCount)
	}

	// Card 7: state intact + surface backfilled to most-frequent "koiran".
	if got := cardSurfaceNorm(t, migrated, 7); got != "koiran" {
		t.Fatalf("card 7 surface_norm=%q want koiran (most frequent occurrence)", got)
	}
	var fsrs, nextDue, lastAns, introduced string
	if err := migrated.db.QueryRow(
		`SELECT fsrs_json, next_due, last_answer_at, introduced_at FROM card_state WHERE card_id = 7`,
	).Scan(&fsrs, &nextDue, &lastAns, &introduced); err != nil {
		t.Fatalf("card 7 state: %v", err)
	}
	if fsrs != `{"step":2,"streak":3}` {
		t.Fatalf("card 7 fsrs_json=%q not preserved", fsrs)
	}
	if nextDue != "2026-05-01T10:00:00Z" || lastAns != "2026-04-29T09:00:00Z" || introduced != "2026-04-20T08:00:00Z" {
		t.Fatalf("card 7 timestamps changed: due=%q ans=%q intro=%q", nextDue, lastAns, introduced)
	}

	// Card 8: no occurrence -> fallback to lemma "kissa".
	if got := cardSurfaceNorm(t, migrated, 8); got != "kissa" {
		t.Fatalf("card 8 surface_norm=%q want kissa (lemma fallback)", got)
	}

	// review_log still points at a surviving card.
	var logCards int
	if err := migrated.db.QueryRow(
		`SELECT COUNT(*) FROM review_log WHERE card_id IN (SELECT id FROM cards)`,
	).Scan(&logCards); err != nil {
		t.Fatalf("review_log join: %v", err)
	}
	if logCards != 1 {
		t.Fatalf("review_log rows pointing at surviving cards = %d, want 1", logCards)
	}

	// Migration is idempotent: a second NewDB open is a no-op.
	if err := migrated.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	again, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("second NewDB: %v", err)
	}
	t.Cleanup(func() { again.Close() })
	if got := cardSurfaceNorm(t, again, 7); got != "koiran" {
		t.Fatalf("after re-open card 7 surface_norm=%q want koiran", got)
	}
}

// TestDeckFilteredReviewReturnsDeckSurfaceCard proves the deck filter in
// GetNextReviewCard is surface-scoped. Cards are surface-specific, so when
// deck A contains "koira" and deck B contains "koiran" (same lemma/POS),
// reviewing deck B must serve the koiran card with deck B's sentence — not
// deck A's koira card wearing a deck B example. A (lemma, pos)-only deck
// membership check passes both cards and, ordered by id, returns the wrong one.
func TestDeckFilteredReviewReturnsDeckSurfaceCard(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "learner@example.com")

	const sentenceA = "Koira nukkuu."
	const sentenceB = "Koiran häntä heiluu."
	deckA := createSingleTokenDeck(t, db, user.ID, "FI", sentenceA, "Koira", "koira", "NOUN")
	deckB := createSingleTokenDeck(t, db, user.ID, "FI", sentenceB, "Koiran", "koira", "NOUN")

	cardB, err := db.GetNextReviewCard(user.ID, &deckB, "FI")
	if err != nil {
		t.Fatalf("GetNextReviewCard(deck B): %v", err)
	}
	if cardB == nil {
		t.Fatal("expected a card when reviewing deck B")
	}
	if cardB.Surface != "koiran" {
		t.Fatalf("deck B review returned surface %q, want the deck's own form \"koiran\"", cardB.Surface)
	}
	if cardB.SentenceText != sentenceB {
		t.Fatalf("deck B review SentenceText=%q, want %q", cardB.SentenceText, sentenceB)
	}
	if len(cardB.DeckCounts) != 1 || cardB.DeckCounts[0][1] != "1" {
		t.Fatalf("deck B review DeckCounts=%v, want only the deck containing surface koiran", cardB.DeckCounts)
	}

	cardA, err := db.GetNextReviewCard(user.ID, &deckA, "FI")
	if err != nil {
		t.Fatalf("GetNextReviewCard(deck A): %v", err)
	}
	if cardA == nil {
		t.Fatal("expected a card when reviewing deck A")
	}
	if cardA.Surface != "koira" {
		t.Fatalf("deck A review returned surface %q, want \"koira\"", cardA.Surface)
	}
	if cardA.SentenceText != sentenceA {
		t.Fatalf("deck A review SentenceText=%q, want %q", cardA.SentenceText, sentenceA)
	}
	if len(cardA.DeckCounts) != 1 || cardA.DeckCounts[0][1] != "1" {
		t.Fatalf("deck A review DeckCounts=%v, want only the deck containing surface koira", cardA.DeckCounts)
	}
}

// TestDeckStatsDueCountIsSurfaceScoped proves GetUserDeckStats counts a lemma
// as due in a deck only when the due card belongs to one of THAT deck's
// surface forms. A due "koira" card (deck A) must not make deck B — which only
// contains "koiran" — report a due review the learner cannot actually get.
func TestDeckStatsDueCountIsSurfaceScoped(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "learner@example.com")

	deckA := createSingleTokenDeck(t, db, user.ID, "FI", "Koira nukkuu.", "Koira", "koira", "NOUN")
	deckB := createSingleTokenDeck(t, db, user.ID, "FI", "Koiran häntä heiluu.", "Koiran", "koira", "NOUN")

	// Make the koira card (deck A's surface) reviewed and overdue.
	res, err := db.db.Exec(
		`UPDATE card_state
		    SET last_answer_at = CURRENT_TIMESTAMP,
		        next_due = datetime('now', '-1 hour')
		  WHERE card_id = (SELECT id FROM cards
		                    WHERE user_id = ? AND lang = 'FI'
		                      AND surface_norm = 'koira' AND lemma = 'koira' AND pos = 'NOUN'
		                      AND mwe_id IS NULL)`,
		user.ID,
	)
	if err != nil {
		t.Fatalf("mark koira card due: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("marked %d card_state rows due, want 1", n)
	}

	stats, err := db.GetUserDeckStats(user.ID)
	if err != nil {
		t.Fatalf("GetUserDeckStats: %v", err)
	}
	due := map[int64]int{}
	for _, s := range stats {
		due[s.ID] = s.Due
	}
	if due[deckA] != 1 {
		t.Fatalf("deck A due=%d, want 1 (its koira card is overdue)", due[deckA])
	}
	if due[deckB] != 0 {
		t.Fatalf("deck B due=%d, want 0 (the due card is deck A's surface, not koiran)", due[deckB])
	}
}
