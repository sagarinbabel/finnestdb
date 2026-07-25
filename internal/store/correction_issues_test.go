package store

import (
	"database/sql"
	"testing"
)

// submitFeedback creates a parse session + feedback report, exercising the
// real grouping path (CreateParseFeedback). surface/lemma/pos define the scope.
// A flag-only report passes lemma="" pos="".
func submitFeedback(t *testing.T, db *DB, userID int64, lang, surface, lemma, pos string) int64 {
	t.Helper()
	sessionID, err := db.CreateParseSession(&userID, lang, "custom", "context "+surface, 1, 1)
	if err != nil {
		t.Fatalf("CreateParseSession: %v", err)
	}
	fb := ParseFeedback{
		ParseSessionID: sessionID,
		UserID:         userID,
		Lang:           lang,
		Parser:         "custom",
		Surface:        surface,
		ProposedLemma:  lemma,
		ProposedPOS:    pos,
		FlagOnly:       lemma == "" && pos == "",
	}
	id, err := db.CreateParseFeedback(fb)
	if err != nil {
		t.Fatalf("CreateParseFeedback: %v", err)
	}
	return id
}

// issueForFeedback returns the correction_issues.id linked to a feedback row.
func issueForFeedback(t *testing.T, db *DB, feedbackID int64) int64 {
	t.Helper()
	var id sql.NullInt64
	if err := db.db.QueryRow(
		`SELECT correction_issue_id FROM parse_feedback WHERE id = ?`, feedbackID,
	).Scan(&id); err != nil {
		t.Fatalf("select correction_issue_id: %v", err)
	}
	if !id.Valid {
		t.Fatalf("feedback %d has no correction_issue_id", feedbackID)
	}
	return id.Int64
}

// TestGroupingDedupAndReopen verifies the scope fingerprint groups duplicate
// reports into one issue, counts distinct reporters, keeps different scopes
// apart, and reopens a fixed issue on a new report. This encodes the core
// promise of Phase 1c: "is this the same problem, or a new one?"
func TestGroupingDedupAndReopen(t *testing.T) {
	db := newTestDB(t)
	u1 := createTestUser(t, db, "reporter1@example.com")
	u2 := createTestUser(t, db, "reporter2@example.com")

	// Two reports, same scope, different reporters -> one issue, 2 reports, 2
	// distinct reporters. Case-insensitive surface match ("Koira" vs "koira").
	fb1 := submitFeedback(t, db, u1.ID, "FI", "Koira", "koira", "NOUN")
	fb2 := submitFeedback(t, db, u2.ID, "FI", "koira", "koira", "NOUN")
	if a, b := issueForFeedback(t, db, fb1), issueForFeedback(t, db, fb2); a != b {
		t.Fatalf("same-scope reports grouped into different issues %d != %d", a, b)
	}
	issueID := issueForFeedback(t, db, fb1)
	issue, err := db.GetCorrectionIssue(issueID)
	if err != nil {
		t.Fatalf("GetCorrectionIssue: %v", err)
	}
	if issue.ReportCount != 2 || issue.DistinctReporterCount != 2 {
		t.Fatalf("report=%d distinct=%d want 2/2", issue.ReportCount, issue.DistinctReporterCount)
	}

	// Same reporter reporting the same scope again bumps report_count but not
	// distinct_reporter_count.
	submitFeedback(t, db, u1.ID, "FI", "koira", "koira", "NOUN")
	issue, _ = db.GetCorrectionIssue(issueID)
	if issue.ReportCount != 3 || issue.DistinctReporterCount != 2 {
		t.Fatalf("after dup report=%d distinct=%d want 3/2", issue.ReportCount, issue.DistinctReporterCount)
	}

	// A different scope (different lemma) makes a distinct issue.
	fbOther := submitFeedback(t, db, u1.ID, "FI", "koira", "koiras", "NOUN")
	if issueForFeedback(t, db, fbOther) == issueID {
		t.Fatal("different-lemma report grouped into the same issue")
	}

	// Fix the issue, then a new report must reopen it and bump reopened_count.
	if err := db.TriageCorrectionIssue(issueID, AlphaClassParserIssue, ""); err != nil {
		t.Fatalf("triage: %v", err)
	}
	if err := db.RestoreCorrectionIssue(issueID, u1.ID, "fixed upstream"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if issue, _ = db.GetCorrectionIssue(issueID); issue.Status != IssueStatusFixed {
		t.Fatalf("status=%q want fixed", issue.Status)
	}
	submitFeedback(t, db, u2.ID, "FI", "koira", "koira", "NOUN")
	issue, _ = db.GetCorrectionIssue(issueID)
	if issue.Status != IssueStatusReopened || issue.ReopenedCount != 1 {
		t.Fatalf("status=%q reopened=%d want reopened/1", issue.Status, issue.ReopenedCount)
	}
}

// quarantineIssueForSurface reports a scope and quarantines the resulting issue.
// Returns the issue id.
func quarantineIssueForSurface(t *testing.T, db *DB, adminID, reporterID int64, lang, surface, lemma, pos, class string) int64 {
	t.Helper()
	fb := submitFeedback(t, db, reporterID, lang, surface, lemma, pos)
	issueID := issueForFeedback(t, db, fb)
	if err := db.TriageCorrectionIssue(issueID, class, ""); err != nil {
		t.Fatalf("triage: %v", err)
	}
	if err := db.QuarantineCorrectionIssue(issueID, adminID, "known bad"); err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	return issueID
}

// TestQuarantineSuppressesReviewAndNewCardQueues proves a quarantined
// (lang,lemma,pos) issue removes the matching card from the due queue, the
// new-card queue, and GetNextReviewCard, for every learner.
func TestQuarantineSuppressesReviewAndNewCardQueues(t *testing.T) {
	db := newTestDB(t)
	admin := createTestUser(t, db, "admin@example.com")
	user := createTestUser(t, db, "learner@example.com")

	createSingleTokenDeck(t, db, user.ID, "FI", "Koira juoksee.", "Koira", "koira", "NOUN")

	// Before quarantine the card is a fresh new card and appears.
	if n, err := db.CountNewCards(user.ID); err != nil || n != 1 {
		t.Fatalf("new cards before=%d err=%v want 1", n, err)
	}
	if card, err := db.GetNextReviewCard(user.ID, nil, ""); err != nil || card == nil {
		t.Fatalf("next card before quarantine=%v err=%v want a card", card, err)
	}

	quarantineIssueForSurface(t, db, admin.ID, user.ID, "FI", "Koira", "koira", "NOUN", AlphaClassParserIssue)

	if n, err := db.CountNewCards(user.ID); err != nil || n != 0 {
		t.Fatalf("new cards after quarantine=%d err=%v want 0", n, err)
	}
	if n, err := db.CountDueCards(user.ID); err != nil || n != 0 {
		t.Fatalf("due cards after quarantine=%d err=%v want 0", n, err)
	}
	if card, err := db.GetNextReviewCard(user.ID, nil, ""); err != nil || card != nil {
		t.Fatalf("next card after quarantine=%v err=%v want nil", card, err)
	}
}

// TestQuarantineExcludedFromDeckStatsAndComprehension pins the deliberate
// stats behavior change: a quarantined (lemma,pos) drops out of deck word/
// coverage counts and out of DeckComprehension coverage + unlocks.
func TestQuarantineExcludedFromDeckStatsAndComprehension(t *testing.T) {
	db := newTestDB(t)
	admin := createTestUser(t, db, "admin@example.com")
	user := createTestUser(t, db, "learner@example.com")

	// Deck: two positions, "koira" and "kissa". Quarantine "koira".
	deckID, err := db.CreateDeckWithSentences(user.ID, "Animals", "FI", []DeckSentenceInput{
		{Text: "Koira ja kissa.", Tokens: []DeckTokenInput{
			{TokenIx: 0, Form: "Koira", Lemma: "koira", POS: "NOUN"},
			{TokenIx: 2, Form: "kissa", Lemma: "kissa", POS: "NOUN"},
		}},
	})
	if err != nil {
		t.Fatalf("CreateDeckWithSentences: %v", err)
	}

	// Baseline: 2 unique, 2 tokens, comprehension total 2.
	if stats, err := db.DeckComprehension(user.ID, deckID, 10); err != nil || stats.TotalTokens != 2 {
		t.Fatalf("comprehension total before=%d err=%v want 2", stats.TotalTokens, err)
	}
	if ds, err := db.GetUserDeckStats(user.ID); err != nil || ds[0].Unique != 2 || ds[0].TotalTokens != 2 {
		t.Fatalf("deck stats before unique=%d tokens=%d want 2/2", ds[0].Unique, ds[0].TotalTokens)
	}

	quarantineIssueForSurface(t, db, admin.ID, user.ID, "FI", "koira", "koira", "NOUN", AlphaClassParserIssue)

	// After quarantine "koira" is gone from every current-facing count: 1
	// unique, 1 token, comprehension total 1, and "koira" is not an unlock.
	stats, err := db.DeckComprehension(user.ID, deckID, 10)
	if err != nil {
		t.Fatalf("DeckComprehension after: %v", err)
	}
	if stats.TotalTokens != 1 {
		t.Fatalf("comprehension total after=%d want 1 (koira suppressed)", stats.TotalTokens)
	}
	for _, u := range stats.TopUnlocks {
		if u.Lemma == "koira" {
			t.Fatalf("quarantined koira still listed as an unlock: %+v", stats.TopUnlocks)
		}
	}
	ds, err := db.GetUserDeckStats(user.ID)
	if err != nil {
		t.Fatalf("GetUserDeckStats after: %v", err)
	}
	if ds[0].Unique != 1 || ds[0].TotalTokens != 1 {
		t.Fatalf("deck stats after unique=%d tokens=%d want 1/1", ds[0].Unique, ds[0].TotalTokens)
	}
}

// TestSurfaceScopeQuarantineSuppressesOccurrence proves a flag-only report
// (no lemma/pos) quarantines by (lang, normalized surface): its deck
// occurrences drop out even though the card lemma/pos is not itself the scope.
func TestSurfaceScopeQuarantineSuppressesOccurrence(t *testing.T) {
	db := newTestDB(t)
	admin := createTestUser(t, db, "admin@example.com")
	user := createTestUser(t, db, "learner@example.com")

	deckID := createSingleTokenDeck(t, db, user.ID, "FI", "Koira.", "Koira", "koira", "NOUN")

	// Flag-only issue keyed on surface only.
	quarantineIssueForSurface(t, db, admin.ID, user.ID, "FI", "Koira", "", "", AlphaClassSourceIssue)

	stats, err := db.DeckComprehension(user.ID, deckID, 10)
	if err != nil {
		t.Fatalf("DeckComprehension: %v", err)
	}
	if stats.TotalTokens != 0 {
		t.Fatalf("comprehension total=%d want 0 (surface Koira suppressed)", stats.TotalTokens)
	}
}

// TestRestorePreservesSchedulerState is the load-bearing restore test:
// quarantine a card, answer other cards (time passes / due state changes for
// others), restore the issue, and confirm the original card resumes with its
// existing card_state (next_due unchanged) rather than being reset.
func TestRestorePreservesSchedulerState(t *testing.T) {
	db := newTestDB(t)
	admin := createTestUser(t, db, "admin@example.com")
	user := createTestUser(t, db, "learner@example.com")

	createSingleTokenDeck(t, db, user.ID, "FI", "Koira.", "Koira", "koira", "NOUN")

	// Find the koira card and give it a real schedule (answer it "good" so it
	// has a future next_due), then capture that state.
	var koiraCardID int64
	if err := db.db.QueryRow(
		`SELECT id FROM cards WHERE user_id = ? AND lemma = 'koira' AND pos = 'NOUN'`, user.ID,
	).Scan(&koiraCardID); err != nil {
		t.Fatalf("find koira card: %v", err)
	}
	if err := db.RecordReviewAnswer(user.ID, koiraCardID, "good"); err != nil {
		t.Fatalf("RecordReviewAnswer: %v", err)
	}
	var dueBefore, fsrsBefore sql.NullString
	if err := db.db.QueryRow(
		`SELECT next_due, fsrs_json FROM card_state WHERE card_id = ?`, koiraCardID,
	).Scan(&dueBefore, &fsrsBefore); err != nil {
		t.Fatalf("read card_state before: %v", err)
	}

	// Quarantine koira.
	issueID := quarantineIssueForSurface(t, db, admin.ID, user.ID, "FI", "Koira", "koira", "NOUN", AlphaClassParserIssue)

	// Study happens on other content while it's quarantined: add + answer a
	// second card so the scheduler is exercised for someone else.
	createSingleTokenDeck(t, db, user.ID, "FI", "Kissa.", "Kissa", "kissa", "NOUN")
	var kissaCardID int64
	if err := db.db.QueryRow(
		`SELECT id FROM cards WHERE user_id = ? AND lemma = 'kissa' AND pos = 'NOUN'`, user.ID,
	).Scan(&kissaCardID); err != nil {
		t.Fatalf("find kissa card: %v", err)
	}
	if err := db.RecordReviewAnswer(user.ID, kissaCardID, "good"); err != nil {
		t.Fatalf("RecordReviewAnswer kissa: %v", err)
	}

	// Restore koira: its scheduler state must be exactly what it was - quarantine
	// pauses circulation, it does not reset memory.
	if err := db.RestoreCorrectionIssue(issueID, admin.ID, "fixed"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	var dueAfter, fsrsAfter sql.NullString
	if err := db.db.QueryRow(
		`SELECT next_due, fsrs_json FROM card_state WHERE card_id = ?`, koiraCardID,
	).Scan(&dueAfter, &fsrsAfter); err != nil {
		t.Fatalf("read card_state after: %v", err)
	}
	if dueBefore != dueAfter || fsrsBefore != fsrsAfter {
		t.Fatalf("scheduler state changed across quarantine/restore: due %v->%v fsrs %v->%v",
			dueBefore, dueAfter, fsrsBefore, fsrsAfter)
	}

	// And the card is back in circulation: it is due again after restore (its
	// next_due from a "good" answer may be in the future, so assert on the
	// suppression predicate being lifted via the deck stats unique count).
	ds, err := db.GetUserDeckStats(user.ID)
	if err != nil {
		t.Fatalf("GetUserDeckStats: %v", err)
	}
	var koiraDeckUnique int
	for _, d := range ds {
		if d.Title == "Test deck" && d.Unique == 1 {
			koiraDeckUnique = d.Unique
		}
	}
	if koiraDeckUnique != 1 {
		t.Fatalf("koira deck unique after restore=%d want 1 (back in circulation)", koiraDeckUnique)
	}
}

// TestThresholdCandidateBadge verifies distinct-reporter counting drives the
// threshold_candidate display flag at >=3 distinct reporters, and that it
// never auto-quarantines.
func TestThresholdCandidateBadge(t *testing.T) {
	db := newTestDB(t)
	u1 := createTestUser(t, db, "r1@example.com")
	u2 := createTestUser(t, db, "r2@example.com")
	u3 := createTestUser(t, db, "r3@example.com")

	fb := submitFeedback(t, db, u1.ID, "FI", "koira", "koira", "NOUN")
	issueID := issueForFeedback(t, db, fb)

	issue, _ := db.GetCorrectionIssue(issueID)
	if issue.ThresholdCandidate {
		t.Fatal("threshold_candidate true at 1 reporter")
	}
	submitFeedback(t, db, u2.ID, "FI", "koira", "koira", "NOUN")
	submitFeedback(t, db, u3.ID, "FI", "koira", "koira", "NOUN")

	issue, _ = db.GetCorrectionIssue(issueID)
	if issue.DistinctReporterCount != 3 {
		t.Fatalf("distinct=%d want 3", issue.DistinctReporterCount)
	}
	if !issue.ThresholdCandidate {
		t.Fatal("threshold_candidate false at 3 distinct reporters")
	}
	// Never auto-quarantines.
	if issue.Status != IssueStatusOpen {
		t.Fatalf("status=%q want open (threshold must not auto-quarantine)", issue.Status)
	}
}

// TestQuarantineRequiresClassAndReason guards the two hard preconditions:
// no class -> ErrIssueNeedsClass; empty reason -> ErrQuarantineReasonRequired.
func TestQuarantineRequiresClassAndReason(t *testing.T) {
	db := newTestDB(t)
	admin := createTestUser(t, db, "admin@example.com")
	u := createTestUser(t, db, "r@example.com")

	fb := submitFeedback(t, db, u.ID, "FI", "koira", "koira", "NOUN")
	issueID := issueForFeedback(t, db, fb)

	// Untriaged: quarantine refused.
	if err := db.QuarantineCorrectionIssue(issueID, admin.ID, "reason"); err != ErrIssueNeedsClass {
		t.Fatalf("quarantine without class err=%v want ErrIssueNeedsClass", err)
	}
	if err := db.TriageCorrectionIssue(issueID, AlphaClassNotSure, ""); err != nil {
		t.Fatalf("triage: %v", err)
	}
	// Triaged but empty reason: refused.
	if err := db.QuarantineCorrectionIssue(issueID, admin.ID, "  "); err != ErrQuarantineReasonRequired {
		t.Fatalf("quarantine with empty reason err=%v want ErrQuarantineReasonRequired", err)
	}
	// Valid.
	if err := db.QuarantineCorrectionIssue(issueID, admin.ID, "leaving it live is worse"); err != nil {
		t.Fatalf("quarantine: %v", err)
	}
}
