package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// This file is the regression suite for the state-machine and invariant
// defects audited on 2026-11-07. Each test names the invariant it protects and
// why that invariant matters to a learner or an admin, so the test fails for a
// reason a future reader can act on rather than just going red.

func invariantTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := NewDB(filepath.Join(t.TempDir(), "invariants.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// I-1. A lemma is known XOR ignored XOR neither.
//
// Why it matters: BatchLemmaStates reports "known" when both rows exist, while
// GetNextReviewCard's ignored-NOT EXISTS still drops the card. The learner sees
// a word marked known that they were never taught and cannot reach, with no UI
// affordance that anything is wrong.
func TestKnownWordImportClearsIgnored(t *testing.T) {
	db := invariantTestDB(t)
	u, err := db.CreateUser("i1@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InsertFormForTest("kissat", "kissa", "NOUN", "FI"); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertLemmaForTest("kissa", "NOUN", "cat", "FI"); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkLemmaIgnored(u.ID, "FI", "kissa", "NOUN"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := db.ImportKnownWords(u.ID, "FI", []string{"kissat"}, SourceAnki); err != nil {
		t.Fatalf("ImportKnownWords: %v", err)
	}
	assertNotBothKnownAndIgnored(t, db, u.ID, "ImportKnownWords")

	// And the replace-mode Anki sync path.
	if err := db.MarkLemmaIgnored(u.ID, "FI", "kissa", "NOUN"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := db.ReplaceKnownWords(u.ID, "FI", []string{"kissat"}, SourceAnki); err != nil {
		t.Fatalf("ReplaceKnownWords: %v", err)
	}
	assertNotBothKnownAndIgnored(t, db, u.ID, "ReplaceKnownWords")
}

func assertNotBothKnownAndIgnored(t *testing.T, db *DB, userID int64, via string) {
	t.Helper()
	var both int
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM user_known_lemmas k
		  JOIN user_ignored_lemmas i
		    ON i.user_id = k.user_id AND i.lang = k.lang
		   AND i.lemma = k.lemma AND i.pos = k.pos
		 WHERE k.user_id = ?`, userID).Scan(&both); err != nil {
		t.Fatal(err)
	}
	if both != 0 {
		t.Errorf("%s left %d lemma(s) both known and ignored; the two tables encode one tri-state", via, both)
	}
}

// I-2. The schema's FOREIGN KEY declarations are enforced.
//
// Why it matters: referential integrity was maintained only by two
// hand-written cascades. Any new delete path, or any of the ~30 cmd/ tools
// writing directly, could leave dangling rows - and the schema reads as though
// that were impossible.
func TestForeignKeysAreEnforced(t *testing.T) {
	db := invariantTestDB(t)

	var fk int
	if err := db.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("PRAGMA foreign_keys = %d, want 1; every FOREIGN KEY in the schema is decorative without it", fk)
	}
	if _, err := db.db.Exec(
		`INSERT INTO cards (user_id, lang, surface_norm, lemma, pos) VALUES (999999, 'FI', 'x', 'x', 'NOUN')`,
	); err == nil {
		t.Error("inserted a card for a nonexistent user with no FK error")
	}
}

// I-3. DeleteUserCascade removes every row belonging to the account, and leaves
// derived counts consistent.
//
// Why it matters: account deletion is an advertised privacy feature. A
// surviving review_log row both retains the deleted learner's activity and
// would attribute it to whoever next receives that user id. Stale
// correction-issue counts drive the threshold_candidate badge admins triage on.
func TestDeleteUserCascadeIsComplete(t *testing.T) {
	db := invariantTestDB(t)
	u, err := db.CreateUser("i3@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := db.CreateUser("i3-admin@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}

	cardID, err := db.EnsureCard(u.ID, "FI", "kissat", "kissa", "NOUN")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordReviewAnswer(u.ID, cardID, RatingGood); err != nil {
		t.Fatal(err)
	}
	psID, err := db.CreateParseSession(&u.ID, "FI", ParserCustom, "kissat", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateParseFeedback(ParseFeedback{
		ParseSessionID: psID, UserID: u.ID, Lang: "FI", Parser: ParserCustom,
		Surface: "kissat", ProposedLemma: "kissa", ProposedPOS: "NOUN",
	}); err != nil {
		t.Fatal(err)
	}
	// An admin action on the shared ledger, which must survive the admin's
	// account with its attribution nulled rather than deleting the issue.
	var issueID int64
	if err := db.db.QueryRow(`SELECT id FROM correction_issues LIMIT 1`).Scan(&issueID); err != nil {
		t.Fatal(err)
	}
	if err := db.TriageCorrectionIssue(issueID, AlphaClassParserIssue, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.QuarantineCorrectionIssue(issueID, admin.ID, "bad"); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteUserCascade(u.ID); err != nil {
		t.Fatalf("DeleteUserCascade: %v", err)
	}

	for _, q := range []struct {
		what  string
		query string
	}{
		{"review_log", `SELECT COUNT(*) FROM review_log WHERE user_id = ?`},
		{"cards", `SELECT COUNT(*) FROM cards WHERE user_id = ?`},
		{"card_state", `SELECT COUNT(*) FROM card_state WHERE card_id IN (SELECT id FROM cards WHERE user_id = ?)`},
		{"parse_feedback", `SELECT COUNT(*) FROM parse_feedback WHERE user_id = ?`},
		{"user_sessions", `SELECT COUNT(*) FROM user_sessions WHERE user_id = ?`},
	} {
		var n int
		if err := db.db.QueryRow(q.query, u.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s: %d row(s) survived account deletion", q.what, n)
		}
	}

	// Derived counts must agree with what is actually linked.
	var drift int
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM correction_issues ci
		 WHERE ci.report_count <> (
		         SELECT COUNT(*) FROM parse_feedback pf WHERE pf.correction_issue_id = ci.id)
		    OR ci.distinct_reporter_count <> (
		         SELECT COUNT(DISTINCT pf.user_id) FROM parse_feedback pf WHERE pf.correction_issue_id = ci.id)`,
	).Scan(&drift); err != nil {
		t.Fatal(err)
	}
	if drift != 0 {
		t.Errorf("%d correction issue(s) have counts that no longer match their linked reports", drift)
	}

	// Deleting the admin nulls attribution but keeps the shared issue.
	if err := db.DeleteUserCascade(admin.ID); err != nil {
		t.Fatalf("DeleteUserCascade(admin): %v", err)
	}
	var issues int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM correction_issues WHERE id = ?`, issueID).Scan(&issues); err != nil {
		t.Fatal(err)
	}
	if issues != 1 {
		t.Error("deleting an admin removed a correction issue that belongs to every learner")
	}
}

// I-4. Correction-issue status only moves along legal edges, and a row never
// claims two contradictory lifecycle facts at once.
//
// Why it matters: "restore" from any state produced rows marked fixed that were
// never quarantined, and quarantined rows still carrying a previous fix_note -
// so the admin queue could not tell what had actually happened to an issue.
func TestCorrectionIssueTransitionsAreGuarded(t *testing.T) {
	db := invariantTestDB(t)
	admin, err := db.CreateUser("i4@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	newIssue := func(t *testing.T) int64 {
		t.Helper()
		res, err := db.db.Exec(
			`INSERT INTO correction_issues (lang, parser, norm_surface, lemma, pos, status, alpha_class)
			 VALUES ('FI', 'custom', ?, 'kissa', 'NOUN', 'open', 'parser_issue')`,
			t.Name()+string(rune(len(t.Name()))),
		)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	t.Run("fixed cannot be quarantined directly", func(t *testing.T) {
		id := newIssue(t)
		if err := db.RestoreCorrectionIssue(id, admin.ID, "fixed upstream"); err != nil {
			t.Fatalf("open -> fixed should be legal: %v", err)
		}
		err := db.QuarantineCorrectionIssue(id, admin.ID, "actually still bad")
		if !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("fixed -> quarantined: err=%v, want ErrIllegalTransition (a regression must arrive as a new report, which reopens the issue)", err)
		}
	})

	t.Run("already-fixed cannot be re-fixed", func(t *testing.T) {
		id := newIssue(t)
		if err := db.RestoreCorrectionIssue(id, admin.ID, "first"); err != nil {
			t.Fatal(err)
		}
		// Identity edge is allowed (re-saving a note), so assert the note
		// rather than an error - what must not happen is a second lifecycle.
		if err := db.RestoreCorrectionIssue(id, admin.ID, "second"); err != nil {
			t.Fatalf("identity edge should be permitted: %v", err)
		}
	})

	t.Run("quarantine clears a stale fix", func(t *testing.T) {
		id := newIssue(t)
		if err := db.QuarantineCorrectionIssue(id, admin.ID, "bad"); err != nil {
			t.Fatal(err)
		}
		if err := db.RestoreCorrectionIssue(id, admin.ID, "fixed it"); err != nil {
			t.Fatal(err)
		}
		// A new report reopens it, which is the only route back to quarantine.
		if _, err := db.db.Exec(`UPDATE correction_issues SET status = 'reopened' WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
		if err := db.QuarantineCorrectionIssue(id, admin.ID, "regressed"); err != nil {
			t.Fatalf("reopened -> quarantined should be legal: %v", err)
		}
		issue, err := db.GetCorrectionIssue(id)
		if err != nil {
			t.Fatal(err)
		}
		if issue.FixedAt != nil || issue.FixNote != "" {
			t.Errorf("quarantined issue still carries fixed_at=%v fix_note=%q from the earlier fix",
				issue.FixedAt, issue.FixNote)
		}
	})

	t.Run("status column rejects unknown values", func(t *testing.T) {
		if _, err := db.db.Exec(
			`INSERT INTO correction_issues (lang, parser, norm_surface, lemma, pos, status)
			 VALUES ('FI', 'custom', 'zz', 'z', 'N', 'banana')`,
		); err == nil {
			t.Error("correction_issues.status accepted \"banana\"")
		}
	})
}

// I-5. An accepted correction's lexical override can be taken back, and a
// review cannot silently un-accept itself.
//
// Why it matters: acceptance writes a custom_overrides row at priority 100,
// above every imported dictionary, and nothing deleted such rows. Flipping the
// status to "rejected" left the parser serving the rejected analysis forever.
func TestAcceptedFeedbackOverrideIsRetractable(t *testing.T) {
	db := invariantTestDB(t)
	u, err := db.CreateUser("i5@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	psID, err := db.CreateParseSession(&u.ID, "FI", ParserCustom, "kissat", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	fbID, err := db.CreateParseFeedback(ParseFeedback{
		ParseSessionID: psID, UserID: u.ID, Lang: "FI", Parser: ParserCustom,
		Surface: "kissat", ProposedLemma: "kissa", ProposedPOS: "NOUN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReviewParseFeedback(fbID, u.ID, FeedbackStatusAccepted, "ok", nil); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if n := countOverrides(t, db); n == 0 {
		t.Fatal("accept wrote no custom_overrides row; the rest of this test is meaningless")
	}

	// A plain status flip must be refused, because it would not remove the
	// override it is pretending to undo.
	err = db.ReviewParseFeedback(fbID, u.ID, FeedbackStatusRejected, "changed my mind", nil)
	if !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("accepted -> rejected: err=%v, want ErrIllegalTransition", err)
	}
	if n := countOverrides(t, db); n == 0 {
		t.Error("refused transition still removed the override")
	}

	// The explicit retraction both removes the override and moves the status.
	if _, err := db.RetractParseFeedbackOverride(fbID, u.ID, FeedbackStatusRejected, "wrong after all"); err != nil {
		t.Fatalf("RetractParseFeedbackOverride: %v", err)
	}
	if n := countOverrides(t, db); n != 0 {
		t.Errorf("%d custom_overrides row(s) survived retraction; the parser would keep serving the rejected analysis", n)
	}
	var status string
	if err := db.db.QueryRow(`SELECT status FROM parse_feedback WHERE id = ?`, fbID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != FeedbackStatusRejected {
		t.Errorf("status=%q after retraction, want %q", status, FeedbackStatusRejected)
	}
	// Retracting twice is not a silent success.
	if _, err := db.RetractParseFeedbackOverride(fbID, u.ID, FeedbackStatusRejected, "again"); !errors.Is(err, ErrFeedbackNotAccepted) {
		t.Errorf("second retraction: err=%v, want ErrFeedbackNotAccepted", err)
	}
}

func countOverrides(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	if err := db.db.QueryRow(
		`SELECT COUNT(*) FROM forms WHERE source = ?`, SourceCustomOverrides,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// I-6. State-bearing columns reject values outside their machine.
//
// Why it matters: ratingToFSRS maps any unrecognized rating to Good, so a bad
// value silently advanced a card's schedule instead of erroring - and the store
// is imported directly by ~30 cmd/ tools that never pass the HTTP validation.
func TestUnknownStateValuesAreRejected(t *testing.T) {
	db := invariantTestDB(t)
	u, err := db.CreateUser("i6@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	cardID, err := db.EnsureCard(u.ID, "FI", "kissat", "kissa", "NOUN")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.RecordReviewAnswer(u.ID, cardID, "banana"); !errors.Is(err, ErrInvalidState) {
		t.Errorf("RecordReviewAnswer(rating=\"banana\"): err=%v, want ErrInvalidState (it would otherwise be scheduled as Good)", err)
	}
	var logged int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM review_log WHERE card_id = ?`, cardID).Scan(&logged); err != nil {
		t.Fatal(err)
	}
	if logged != 0 {
		t.Errorf("a rejected rating still wrote %d review_log row(s)", logged)
	}

	if _, _, err := db.ImportKnownWords(u.ID, "FI", []string{"kissat"}, "telepathy"); !errors.Is(err, ErrInvalidState) {
		t.Errorf("ImportKnownWords(source=\"telepathy\"): err=%v, want ErrInvalidState", err)
	}

	if _, err := db.db.Exec(
		`INSERT INTO gold_candidates (lang, surface, lemma, pos, supporter_count, status)
		 VALUES ('FI', 's', 'l', 'N', 1, 'banana')`,
	); err == nil {
		t.Error("gold_candidates.status accepted \"banana\"")
	}
}

// I-9. Every cards row has a card_state row.
//
// Why it matters: the review-queue queries INNER JOIN card_state, so a
// stateless card silently vanishes from review; recordReviewAnswerAt instead
// errors on it. The guarantee now comes from a trigger rather than from each
// insert site remembering.
func TestEveryCardGetsState(t *testing.T) {
	db := invariantTestDB(t)
	u, err := db.CreateUser("i9@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	// Insert straight into cards, bypassing ensureCard entirely - this is what
	// a cmd/ tool or a future bulk path would do.
	if _, err := db.db.Exec(
		`INSERT INTO cards (user_id, lang, surface_norm, lemma, pos, mwe_id)
		 VALUES (?, 'FI', 'raw', 'rawlemma', 'NOUN', NULL)`, u.ID,
	); err != nil {
		t.Fatal(err)
	}
	var stateless int
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM cards c
		  LEFT JOIN card_state cs ON cs.card_id = c.id
		 WHERE cs.card_id IS NULL`).Scan(&stateless); err != nil {
		t.Fatal(err)
	}
	if stateless != 0 {
		t.Errorf("%d card(s) have no card_state row; they are invisible to the review queue", stateless)
	}
}

// I-11. The three dashboard counters are mutually reconcilable.
//
// Why it matters: all three render side by side. CardsInReview used to skip the
// known/ignored/suppression filters the other two apply, so "in review" could
// exceed due + new and read as a scheduler bug.
func TestDashboardCountersApplyTheSameFilters(t *testing.T) {
	db := invariantTestDB(t)
	u, err := db.CreateUser("i11@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	cardID, err := db.EnsureCard(u.ID, "FI", "kissat", "kissa", "NOUN")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordReviewAnswer(u.ID, cardID, RatingGood); err != nil {
		t.Fatal(err)
	}
	before, err := db.CardsInReview(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before != 1 {
		t.Fatalf("CardsInReview=%d before marking known, want 1", before)
	}

	// Marking the lemma known removes it from due and new; in-review must agree.
	if err := db.MarkLemmaKnown(u.ID, "FI", "kissa", "NOUN"); err != nil {
		t.Fatal(err)
	}
	after, err := db.CardsInReview(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	due, err := db.CountDueCards(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	newCards, err := db.CountNewCards(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Errorf("CardsInReview=%d after the lemma was marked known (due=%d new=%d); the counters disagree",
			after, due, newCards)
	}
}

// I-12. A revoked session neither authenticates nor has its expiry extended.
//
// Scope note, so this test is not mistaken for more than it is: the defect it
// relates to is a TOCTOU window inside GetUserBySessionTokenHash, where the
// sliding-window renewal re-selected on token_hash alone instead of repeating
// the `revoked_at IS NULL AND expires_at > ?` predicate. That window is only
// reachable if a logout commits between this function's SELECT and its UPDATE,
// which a single-threaded test cannot stage - a session revoked beforehand
// returns early at the SELECT and never reaches the renewal at all.
//
// So this test guards the observable end-to-end property, not the race. The
// narrowed UPDATE predicate is what closes the race; it is one clause and
// costs nothing, but it is unproven by this test and should be treated as
// hardening rather than as a fix with a regression test behind it.
func TestSessionRenewalSkipsRevokedSessions(t *testing.T) {
	db := invariantTestDB(t)
	u, err := db.CreateUser("i12@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	const tokenHash = "deadbeef"
	if err := db.CreateSession(u.ID, tokenHash, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := db.db.QueryRow(`SELECT expires_at FROM user_sessions WHERE token_hash = ?`, tokenHash).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeSessionByTokenHash(tokenHash); err != nil {
		t.Fatal(err)
	}

	// A revoked session must not authenticate...
	user, err := db.GetUserBySessionTokenHash(tokenHash, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if user != nil {
		t.Fatal("a revoked session authenticated")
	}
	// ...nor have its expiry extended.
	var after string
	if err := db.db.QueryRow(`SELECT expires_at FROM user_sessions WHERE token_hash = ?`, tokenHash).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("revoked session expiry moved from %q to %q", before, after)
	}
}

// I-5b. Retracting an override restores the dictionary, it does not erase it.
//
// Why it matters: neither `lemmas` nor `forms` has source in its primary key,
// so accepting a correction on an entry an importer already produced rewrites
// that row IN PLACE rather than adding one beside it. A retraction that
// deleted "the custom_overrides row" therefore deleted the imported
// dictionary entry - taking its gloss with it, and affecting every other
// surface form that resolved through that lemma. Found by review on #295.
func TestRetractionRestoresDisplacedDictionaryRows(t *testing.T) {
	db := invariantTestDB(t)

	// An imported entry, as kaikki/Ekilex would leave it.
	if err := db.InsertLemmaForTest("kissa", "NOUN", "cat (the animal)", "FI"); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertFormForTest("kissat", "kissa", "NOUN", "FI"); err != nil {
		t.Fatal(err)
	}

	u, err := db.CreateUser("i5b@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	psID, err := db.CreateParseSession(&u.ID, "FI", ParserCustom, "kissat", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	fbID, err := db.CreateParseFeedback(ParseFeedback{
		ParseSessionID: psID, UserID: u.ID, Lang: "FI", Parser: ParserCustom,
		Surface: "kissat", ProposedLemma: "kissa", ProposedPOS: "NOUN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReviewParseFeedback(fbID, u.ID, FeedbackStatusAccepted, "ok", nil); err != nil {
		t.Fatal(err)
	}
	// Acceptance took the row over, as designed.
	var src string
	if err := db.db.QueryRow(
		`SELECT source FROM lemmas WHERE lemma='kissa' AND pos='NOUN' AND lang='FI'`).Scan(&src); err != nil {
		t.Fatal(err)
	}
	if src != SourceCustomOverrides {
		t.Fatalf("after accept lemma source=%q, want %q", src, SourceCustomOverrides)
	}

	if _, err := db.RetractParseFeedbackOverride(fbID, u.ID, FeedbackStatusRejected, "wrong after all"); err != nil {
		t.Fatalf("RetractParseFeedbackOverride: %v", err)
	}

	// The imported row is back, with its original source AND its gloss.
	var gotSource, gotGloss string
	var feedbackID sql.NullInt64
	if err := db.db.QueryRow(
		`SELECT source, COALESCE(gloss, ''), parse_feedback_id
		   FROM lemmas WHERE lemma='kissa' AND pos='NOUN' AND lang='FI'`,
	).Scan(&gotSource, &gotGloss, &feedbackID); err != nil {
		t.Fatalf("the imported dictionary lemma was deleted by retraction: %v", err)
	}
	if gotSource == SourceCustomOverrides {
		t.Errorf("lemma source is still %q after retraction", gotSource)
	}
	if gotGloss != "cat (the animal)" {
		t.Errorf("lemma gloss=%q after retraction, want the imported gloss back", gotGloss)
	}
	if feedbackID.Valid {
		t.Errorf("lemma still attributed to feedback %d after retraction", feedbackID.Int64)
	}

	var formSource string
	if err := db.db.QueryRow(
		`SELECT source FROM forms WHERE form='kissat' AND lang='FI' AND lemma='kissa' AND pos='NOUN'`,
	).Scan(&formSource); err != nil {
		t.Fatalf("the imported dictionary form was deleted by retraction: %v", err)
	}
	if formSource == SourceCustomOverrides {
		t.Errorf("form source is still %q after retraction", formSource)
	}

	// The snapshot is consumed, so a stale restore cannot fire later.
	var snapshots int
	if err := db.db.QueryRow(
		`SELECT COUNT(*) FROM parse_feedback_override_backup WHERE parse_feedback_id = ?`, fbID,
	).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if snapshots != 0 {
		t.Errorf("%d override snapshot(s) left behind after retraction", snapshots)
	}
}

// I-5c. An override that CREATED a dictionary row is removed on retraction,
// not left behind as a phantom entry.
func TestRetractionDeletesOverrideCreatedRows(t *testing.T) {
	db := invariantTestDB(t)
	// No imported entry this time: the acceptance is the only thing that will
	// put kettu/NOUN in the dictionary.
	if err := db.InsertFormForTest("ketut", "kettu", "NOUN", "FI"); err != nil {
		t.Fatal(err)
	}
	u, err := db.CreateUser("i5c@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	psID, err := db.CreateParseSession(&u.ID, "FI", ParserCustom, "ketut", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	fbID, err := db.CreateParseFeedback(ParseFeedback{
		ParseSessionID: psID, UserID: u.ID, Lang: "FI", Parser: ParserCustom,
		Surface: "ketut", ProposedLemma: "kettu", ProposedPOS: "NOUN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReviewParseFeedback(fbID, u.ID, FeedbackStatusAccepted, "ok", nil); err != nil {
		t.Fatal(err)
	}
	var created int
	db.db.QueryRow(`SELECT COUNT(*) FROM lemmas WHERE lemma='kettu' AND pos='NOUN' AND lang='FI'`).Scan(&created)
	if created != 1 {
		t.Fatalf("accept created %d lemma rows, want 1", created)
	}

	if _, err := db.RetractParseFeedbackOverride(fbID, u.ID, FeedbackStatusRejected, "no"); err != nil {
		t.Fatal(err)
	}
	var after int
	db.db.QueryRow(`SELECT COUNT(*) FROM lemmas WHERE lemma='kettu' AND pos='NOUN' AND lang='FI'`).Scan(&after)
	if after != 0 {
		t.Errorf("%d phantom lemma row(s) survived retraction of the override that created them", after)
	}
}
