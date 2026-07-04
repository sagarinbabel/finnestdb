package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	gofsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

// setNewPerDay pins a user's daily new-card limit for deterministic queue drills.
func setNewPerDay(t *testing.T, db *DB, userID int64, n int) {
	t.Helper()
	if _, err := db.db.Exec(
		`UPDATE users SET settings_json = ? WHERE id = ?`,
		fmt.Sprintf(`{"new_per_day":%d,"retention":0.9,"theme":"system"}`, n),
		userID,
	); err != nil {
		t.Fatalf("setNewPerDay: %v", err)
	}
}

// This file is the staging-style FSRS rollout validation harness described in
// docs/DEPLOYMENT.md "FSRS scheduler rollout" and recorded in
// docs/launch-readiness/2026-07-04-fsrs-validation.md.
//
// It exercises the FSRS scheduler the way production will: through
// recordReviewAnswerAt with the flag and clock injected (the same seam the
// runtime uses via RecordReviewAnswer -> FSRSEnabled()). Every drill runs on a
// throwaway in-memory DB (newTestDB); none of them touch the shared
// finnestdb.db. They stay in the suite so the rollout gate is re-checkable.

// seedCardState overwrites a card's card_state row so a drill can start from a
// realistic prior state (legacy step payload, NULL state, or a mature interval)
// rather than only fresh cards.
func seedCardState(t *testing.T, db *DB, cardID int64, fsrsJSON *string, nextDue, lastAnswer, introduced *time.Time) {
	t.Helper()
	toStr := func(p *time.Time) any {
		if p == nil {
			return nil
		}
		return sqliteTime(*p)
	}
	var jsonVal any
	if fsrsJSON != nil {
		jsonVal = *fsrsJSON
	}
	if _, err := db.db.Exec(
		`UPDATE card_state
		    SET fsrs_json = ?, next_due = ?, last_answer_at = ?, introduced_at = ?
		  WHERE card_id = ?`,
		jsonVal, toStr(nextDue), toStr(lastAnswer), toStr(introduced), cardID,
	); err != nil {
		t.Fatalf("seedCardState(%d): %v", cardID, err)
	}
}

// readCardState reads back the persisted scheduler columns for assertions.
type persistedState struct {
	rawFSRS    sql.NullString
	nextDue    sql.NullString
	lastAnswer sql.NullString
	introduced sql.NullString
}

func readCardState(t *testing.T, db *DB, cardID int64) persistedState {
	t.Helper()
	var s persistedState
	if err := db.db.QueryRow(
		`SELECT fsrs_json, next_due, last_answer_at, introduced_at FROM card_state WHERE card_id = ?`,
		cardID,
	).Scan(&s.rawFSRS, &s.nextDue, &s.lastAnswer, &s.introduced); err != nil {
		t.Fatalf("readCardState(%d): %v", cardID, err)
	}
	return s
}

// legacyStepJSON is a legacy step payload as the step scheduler would have
// written it (no "v" field).
func legacyStepJSON(t *testing.T, step, streak int) string {
	t.Helper()
	b, err := json.Marshal(ReviewSchedule{Step: step, Streak: streak})
	if err != nil {
		t.Fatalf("legacyStepJSON: %v", err)
	}
	return string(b)
}

// dueDaysFromFSRS reads the scheduled interval (days) out of a persisted FSRS
// payload relative to a reference "now".
func dueDaysFromPayload(t *testing.T, raw string, now time.Time) float64 {
	t.Helper()
	var env fsrsPayload
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("dueDaysFromPayload: %v (%s)", err, raw)
	}
	return env.FSRS.Due.Sub(now).Hours() / 24.0
}

// -----------------------------------------------------------------------------
// Drill 1: Seeded-history validation
// -----------------------------------------------------------------------------

// TestFSRSValidationSeededHistory constructs the four realistic card shapes the
// rollout protocol calls out — new, learning mid-step, mature long-interval,
// legacy {step,streak} state — plus a NULL-state card, flips the flag ON, and
// rates through several simulated days with a deterministic clock. It asserts
// the FSRS scheduling invariants the gate requires:
//   - Again shortens vs. Good; Easy lengthens vs. Good (per rating).
//   - Stability grows monotonically across a Good streak.
//   - next_due / last_answer_at / introduced_at are always populated after a
//     rating and the FSRS payload is never NULL/corrupt.
func TestFSRSValidationSeededHistory(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "seeded@example.com")
	day0 := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)

	// --- Per-rating interval ordering on each seeded shape ---------------------
	// For each shape, rate a fresh copy with each button from the same start
	// state and compare the resulting intervals. Again < Good < Easy must hold.
	type shape struct {
		name  string
		setup func(t *testing.T, cardID int64)
	}
	shapes := []shape{
		{"new_null_state", func(t *testing.T, cardID int64) {
			seedCardState(t, db, cardID, nil, nil, nil, nil)
		}},
		{"learning_mid_step", func(t *testing.T, cardID int64) {
			j := legacyStepJSON(t, 1, 1)
			la := day0.Add(-8 * time.Hour)
			nd := day0.Add(-1 * time.Hour) // just came due
			seedCardState(t, db, cardID, &j, &nd, &la, &la)
		}},
		{"mature_long_interval", func(t *testing.T, cardID int64) {
			j := legacyStepJSON(t, 5, 12)
			la := day0.Add(-60 * 24 * time.Hour)
			nd := day0.Add(-1 * 24 * time.Hour) // ~59d interval, due yesterday
			seedCardState(t, db, cardID, &j, &nd, &la, &la)
		}},
		{"legacy_short_interval", func(t *testing.T, cardID int64) {
			j := legacyStepJSON(t, 3, 4)
			la := day0.Add(-7 * 24 * time.Hour)
			nd := day0.Add(-1 * 24 * time.Hour) // ~6d interval
			seedCardState(t, db, cardID, &j, &nd, &la, &la)
		}},
	}

	for _, sh := range shapes {
		dues := map[string]float64{}
		for _, rating := range []string{"again", "good", "easy"} {
			// Fresh card per (shape,rating) so each starts from the same seed.
			cardID, err := db.EnsureCard(user.ID, "FI", sh.name+"-"+rating, "lemma_"+sh.name, "NOUN")
			if err != nil {
				t.Fatalf("%s/%s EnsureCard: %v", sh.name, rating, err)
			}
			sh.setup(t, cardID)
			if err := db.recordReviewAnswerAt(user.ID, cardID, rating, day0, true); err != nil {
				t.Fatalf("%s/%s answer: %v", sh.name, rating, err)
			}
			st := readCardState(t, db, cardID)
			if !st.rawFSRS.Valid || !isFSRSPayload(st.rawFSRS.String) {
				t.Fatalf("%s/%s: state is not a valid FSRS payload: %q", sh.name, rating, st.rawFSRS.String)
			}
			if !st.nextDue.Valid || st.nextDue.String == "" {
				t.Fatalf("%s/%s: next_due not populated", sh.name, rating)
			}
			if !st.lastAnswer.Valid || st.lastAnswer.String == "" {
				t.Fatalf("%s/%s: last_answer_at not populated", sh.name, rating)
			}
			if !st.introduced.Valid || st.introduced.String == "" {
				t.Fatalf("%s/%s: introduced_at not populated", sh.name, rating)
			}
			dues[rating] = dueDaysFromPayload(t, st.rawFSRS.String, day0)
		}
		if !(dues["again"] < dues["good"]) {
			t.Fatalf("%s: Again (%.3fd) should be sooner than Good (%.3fd)", sh.name, dues["again"], dues["good"])
		}
		if !(dues["good"] < dues["easy"]) {
			t.Fatalf("%s: Good (%.3fd) should be sooner than Easy (%.3fd)", sh.name, dues["good"], dues["easy"])
		}
	}

	// --- Monotonic stability growth on a Good streak ---------------------------
	cardID, err := db.EnsureCard(user.ID, "FI", "streak-card", "streak_lemma", "NOUN")
	if err != nil {
		t.Fatalf("streak EnsureCard: %v", err)
	}
	seedCardState(t, db, cardID, nil, nil, nil, nil)
	now := day0
	var prevStability float64
	for i := 0; i < 6; i++ {
		if err := db.recordReviewAnswerAt(user.ID, cardID, "good", now, true); err != nil {
			t.Fatalf("streak good #%d: %v", i, err)
		}
		st := readCardState(t, db, cardID)
		var env fsrsPayload
		if err := json.Unmarshal([]byte(st.rawFSRS.String), &env); err != nil {
			t.Fatalf("streak #%d unmarshal: %v", i, err)
		}
		if i > 0 && !(env.FSRS.Stability > prevStability) {
			t.Fatalf("streak #%d: stability did not grow on Good: %.4f -> %.4f",
				i, prevStability, env.FSRS.Stability)
		}
		prevStability = env.FSRS.Stability
		// Advance the clock to the card's due date so the next Good is an
		// on-time review (the realistic path that grows stability).
		now = env.FSRS.Due
	}
}

// TestFSRSValidationDueQueueAndNewLimit proves the due-queue and daily-new-card
// gate are scheduler-agnostic with the flag ON. Two properties, kept separate:
//
//  1. Daily new-card gate: with new_per_day=2, introducing (FSRS-rating) two
//     fresh cards drives remainingNewCardsToday to 0, and a third un-introduced
//     new card is then withheld from GetNextReviewCard.
//  2. Due-queue ordering keys off next_due regardless of which scheduler wrote
//     the row: a future-due FSRS card is withheld; an overdue FSRS card is
//     surfaced.
//
// Note on FSRS learning steps: a brand-new card rated Good enters *learning*
// state with a ~10-minute next_due (FSRS's learning ladder), not a multi-day
// interval — so "introduced" does not mean "gone from the queue". The queue
// keys off next_due vs. wall-clock CURRENT_TIMESTAMP, so this drill sets
// next_due explicitly (far future / past) to make queue membership
// deterministic rather than racing the injected clock against wall time.
func TestFSRSValidationDueQueueAndNewLimit(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "queue@example.com")
	// Pin the daily new-card limit to 2 for a deterministic gate check.
	setNewPerDay(t, db, user.ID, 2)

	now := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)

	// Three brand-new cards; the daily limit is 2, so only two may be introduced.
	var newCards []int64
	for i := 0; i < 3; i++ {
		id, err := db.EnsureCard(user.ID, "FI", fmt.Sprintf("new%d", i), fmt.Sprintf("newlemma%d", i), "NOUN")
		if err != nil {
			t.Fatalf("new card %d: %v", i, err)
		}
		newCards = append(newCards, id)
	}

	// Property 1: daily new-card gate. Introduce two new cards via FSRS; both now
	// carry introduced_at=today, exhausting the cap of 2.
	for _, id := range newCards[:2] {
		if err := db.recordReviewAnswerAt(user.ID, id, "good", now, true); err != nil {
			t.Fatalf("introduce card %d: %v", id, err)
		}
	}
	remaining, err := db.remainingNewCardsToday(user.ID)
	if err != nil {
		t.Fatalf("remainingNewCardsToday: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected 0 remaining new cards after introducing 2 with cap 2, got %d", remaining)
	}

	// Push the two introduced (learning-state) cards' next_due far into the
	// future so they are unambiguously NOT due against wall-clock
	// CURRENT_TIMESTAMP. This isolates the new-card gate: the only candidate the
	// gate could surface is the third, un-introduced new card, which the gate
	// must withhold because remaining==0 (last_answer_at IS NULL and cap hit).
	future := time.Now().UTC().Add(30 * 24 * time.Hour)
	for _, id := range newCards[:2] {
		if _, err := db.db.Exec(
			`UPDATE card_state SET next_due = ? WHERE card_id = ?`, sqliteTime(future), id,
		); err != nil {
			t.Fatalf("push introduced card %d to future: %v", id, err)
		}
	}
	next, err := db.GetNextReviewCard(user.ID, nil, "FI")
	if err != nil {
		t.Fatalf("GetNextReviewCard (new-limit hit): %v", err)
	}
	if next != nil {
		t.Fatalf("new-card gate leaked: expected no card (cap hit, introduced cards far-future due), got card %d", next.CardID)
	}

	// Property 2: due-queue ordering keys off next_due. Make one introduced FSRS
	// card overdue against wall clock and confirm it is surfaced — proving the
	// queue reads next_due regardless of the scheduler that wrote the row.
	past := time.Now().UTC().Add(-1 * time.Hour)
	if _, err := db.db.Exec(
		`UPDATE card_state SET next_due = ? WHERE card_id = ?`, sqliteTime(past), newCards[0],
	); err != nil {
		t.Fatalf("force overdue: %v", err)
	}
	next, err = db.GetNextReviewCard(user.ID, nil, "FI")
	if err != nil {
		t.Fatalf("GetNextReviewCard (overdue): %v", err)
	}
	if next == nil || next.CardID != newCards[0] {
		t.Fatalf("expected overdue FSRS card %d to be next, got %+v", newCards[0], next)
	}
}

// -----------------------------------------------------------------------------
// Drill 2: Migration correctness at scale
// -----------------------------------------------------------------------------

// TestFSRSValidationMigrationAtScale seeds ~1k cards across all state shapes,
// turns the flag ON, rates a sample, and proves:
//   - Lazy migration only touches RATED cards; un-rated cards keep their exact
//     seeded state (no bulk rewrite).
//   - Derived seeds are conservative: a rated legacy/mature card's derived FSRS
//     stability equals the observed interval (next_due - last_answer_at, >= 1d),
//     matching the documented derivation.
func TestFSRSValidationMigrationAtScale(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "scale@example.com")
	// Large daily limit so new-card gating doesn't interfere with the migration
	// assertions (this drill is about state derivation, not queueing).
	setNewPerDay(t, db, user.ID, 100000)

	day0 := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)
	const total = 1000

	type seeded struct {
		id           int64
		kind         string // "null" | "legacy" | "mature"
		seededRaw    sql.NullString
		expectedDays float64 // for legacy/mature: interval in days
	}
	cards := make([]seeded, 0, total)

	for i := 0; i < total; i++ {
		id, err := db.EnsureCard(user.ID, "FI", fmt.Sprintf("scale%d", i), fmt.Sprintf("scalelemma%d", i), "NOUN")
		if err != nil {
			t.Fatalf("scale card %d: %v", i, err)
		}
		var s seeded
		s.id = id
		switch i % 3 {
		case 0: // NULL-state card
			s.kind = "null"
			seedCardState(t, db, id, nil, nil, nil, nil)
		case 1: // legacy step, short interval (~6d)
			s.kind = "legacy"
			j := legacyStepJSON(t, 3, 4)
			la := day0.Add(-7 * 24 * time.Hour)
			nd := day0.Add(-1 * 24 * time.Hour)
			seedCardState(t, db, id, &j, &nd, &la, &la)
			s.expectedDays = nd.Sub(la).Hours() / 24.0
		case 2: // mature, long interval (~59d)
			s.kind = "mature"
			j := legacyStepJSON(t, 5, 11)
			la := day0.Add(-60 * 24 * time.Hour)
			nd := day0.Add(-1 * 24 * time.Hour)
			seedCardState(t, db, id, &j, &nd, &la, &la)
			s.expectedDays = nd.Sub(la).Hours() / 24.0
		}
		// Record the exact seeded fsrs_json so we can assert un-rated cards are
		// left byte-identical.
		st := readCardState(t, db, id)
		s.seededRaw = st.rawFSRS
		cards = append(cards, s)
	}

	// Rate a deterministic sample: every 7th card gets one Good via FSRS.
	rated := map[int64]bool{}
	for i := 6; i < total; i += 7 {
		if err := db.recordReviewAnswerAt(user.ID, cards[i].id, "good", day0, true); err != nil {
			t.Fatalf("rate scale card %d: %v", i, err)
		}
		rated[cards[i].id] = true
	}

	migratedUnrated := 0
	for _, c := range cards {
		st := readCardState(t, db, c.id)
		if !rated[c.id] {
			// Untouched: fsrs_json must be byte-identical to the seed. This is the
			// "no bulk rewrite" guarantee.
			if st.rawFSRS.Valid != c.seededRaw.Valid || st.rawFSRS.String != c.seededRaw.String {
				migratedUnrated++
			}
			continue
		}
		// Rated: must now be an FSRS payload.
		if !st.rawFSRS.Valid || !isFSRSPayload(st.rawFSRS.String) {
			t.Fatalf("rated card %d (%s) is not an FSRS payload: %q", c.id, c.kind, st.rawFSRS.String)
		}
		var env fsrsPayload
		if err := json.Unmarshal([]byte(st.rawFSRS.String), &env); err != nil {
			t.Fatalf("rated card %d unmarshal: %v", c.id, err)
		}
		switch c.kind {
		case "null":
			// A fresh card rated Good: exactly one rep, learning/review state.
			if env.FSRS.Reps != 1 {
				t.Fatalf("null card %d: reps=%d want 1", c.id, env.FSRS.Reps)
			}
		case "legacy", "mature":
			// Conservative seed: the FSRS card was derived Review-state with reps
			// carried from the legacy streak (so reps >= 2 after this rating), and
			// the pre-rating derived stability equals the observed interval.
			if env.FSRS.State == gofsrs.New {
				t.Fatalf("%s card %d derived as New; expected Review-state seed", c.kind, c.id)
			}
			if env.FSRS.Reps < 2 {
				t.Fatalf("%s card %d: reps=%d want >=2 (streak carried + this rating)", c.kind, c.id, env.FSRS.Reps)
			}
			// The interval-from-next_due conservatism is asserted independently
			// below via deriveFSRSCard.
		}
	}
	if migratedUnrated != 0 {
		t.Fatalf("%d un-rated cards had their state rewritten; lazy migration must only touch rated cards", migratedUnrated)
	}

	// Independent check of the conservative derivation for one legacy + one
	// mature seed: deriveFSRSCard(interval) must set Stability to the observed
	// interval in days.
	checkDerivation := func(kind string, lastAnswer, nextDue time.Time, streak int) {
		la := lastAnswer
		nd := nextDue
		card := deriveFSRSCard(ReviewSchedule{Step: 3, Streak: streak}, &nd, &la, day0)
		wantDays := nd.Sub(la).Hours() / 24.0
		if diff := card.Stability - wantDays; diff > 0.001 || diff < -0.001 {
			t.Fatalf("%s derivation: stability %.4f != observed interval %.4f", kind, card.Stability, wantDays)
		}
		if card.State != gofsrs.Review {
			t.Fatalf("%s derivation: state %v != Review", kind, card.State)
		}
	}
	checkDerivation("legacy", day0.Add(-7*24*time.Hour), day0.Add(-1*24*time.Hour), 4)
	checkDerivation("mature", day0.Add(-60*24*time.Hour), day0.Add(-1*24*time.Hour), 11)
}

// -----------------------------------------------------------------------------
// Drill 3: Rollback drill
// -----------------------------------------------------------------------------

// TestFSRSValidationRollbackDrill runs the full round trip the rollback lever
// must survive: rate via FSRS (flag ON) so an FSRS payload + multi-day interval
// builds up; flip OFF and rate via the step scheduler (must derive a step from
// the interval, not corrupt, not snap to step 0); flip ON again and confirm
// FSRS resumes cleanly from the step state.
func TestFSRSValidationRollbackDrill(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "rollbackdrill@example.com")
	cardID, err := db.EnsureCard(user.ID, "FI", "rollback", "rollbacklemma", "NOUN")
	if err != nil {
		t.Fatalf("EnsureCard: %v", err)
	}

	now := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)

	// Phase A: FSRS ON, build up state across two on-time Good reviews.
	if err := db.recordReviewAnswerAt(user.ID, cardID, "good", now, true); err != nil {
		t.Fatalf("A1: %v", err)
	}
	stA := readCardState(t, db, cardID)
	now = parseStoredTime(t, stA.nextDue.String)
	if err := db.recordReviewAnswerAt(user.ID, cardID, "good", now, true); err != nil {
		t.Fatalf("A2: %v", err)
	}
	stA2 := readCardState(t, db, cardID)
	if !isFSRSPayload(stA2.rawFSRS.String) {
		t.Fatalf("phase A: expected FSRS payload, got %q", stA2.rawFSRS.String)
	}
	intervalDaysBeforeRollback := parseStoredTime(t, stA2.nextDue.String).Sub(now).Hours() / 24.0
	if intervalDaysBeforeRollback < 1 {
		t.Fatalf("phase A: expected a multi-day FSRS interval, got %.2fd", intervalDaysBeforeRollback)
	}

	// Phase B: rollback — flag OFF. The step scheduler must answer without error,
	// write a legacy step payload (not FSRS), and NOT snap the step to 0.
	now = parseStoredTime(t, stA2.nextDue.String)
	if err := db.recordReviewAnswerAt(user.ID, cardID, "good", now, false); err != nil {
		t.Fatalf("phase B rollback answer: %v", err)
	}
	stB := readCardState(t, db, cardID)
	if isFSRSPayload(stB.rawFSRS.String) {
		t.Fatalf("phase B: rollback should write a legacy step payload, got FSRS: %q", stB.rawFSRS.String)
	}
	var sched ReviewSchedule
	if err := json.Unmarshal([]byte(stB.rawFSRS.String), &sched); err != nil {
		t.Fatalf("phase B: payload is not a step schedule: %q (%v)", stB.rawFSRS.String, err)
	}
	if sched.Step == 0 {
		t.Fatalf("phase B: rollback snapped step to 0, losing FSRS-earned progress: %+v", sched)
	}
	if !parseStoredTime(t, stB.nextDue.String).After(now) {
		t.Fatalf("phase B: rollback next_due not in the future")
	}

	// Phase C: flip ON again — FSRS must resume from the step state without
	// corruption, deriving a fresh conservative seed from the current interval.
	now = parseStoredTime(t, stB.nextDue.String)
	if err := db.recordReviewAnswerAt(user.ID, cardID, "good", now, true); err != nil {
		t.Fatalf("phase C resume answer: %v", err)
	}
	stC := readCardState(t, db, cardID)
	if !isFSRSPayload(stC.rawFSRS.String) {
		t.Fatalf("phase C: FSRS did not resume, got %q", stC.rawFSRS.String)
	}
	if !stC.nextDue.Valid || stC.nextDue.String == "" {
		t.Fatalf("phase C: next_due not populated after resume")
	}
}

// -----------------------------------------------------------------------------
// Drill 4: Real-DB smoke (read-mostly)
// -----------------------------------------------------------------------------

// TestFSRSValidationRealDBSmoke copies a small sampled subset of real
// cards/card_state rows from the shared finnestdb.db into a temp DB and runs the
// FSRS + rollback drill against those real row shapes. It catches shape drift
// synthetic seeds miss (unexpected fsrs_json shapes, NULL patterns, timestamp
// formats). It NEVER writes to the shared DB — it only ATTACHes it read-only and
// copies out.
//
// Skips (does not fail) when the shared DB is absent, so CI without the 5GB
// artifact stays green; the gate report records whether it ran.
func TestFSRSValidationRealDBSmoke(t *testing.T) {
	const sharedDBPath = "/Users/sagar/Downloads/projects/finnestdb/finnestdb.db"
	if _, err := os.Stat(sharedDBPath); err != nil {
		t.Skipf("shared finnestdb.db not available (%v); skipping real-DB smoke", err)
	}

	db := newTestDB(t)

	// Attach the shared DB read-only and copy a bounded sample of real card_state
	// rows (<= 5000) plus their cards. Read-only mode guarantees we never mutate
	// the shared file.
	if _, err := db.db.Exec(
		fmt.Sprintf(`ATTACH DATABASE 'file:%s?mode=ro' AS shared`, sharedDBPath),
	); err != nil {
		t.Skipf("could not attach shared DB read-only (%v); skipping", err)
	}
	defer db.db.Exec(`DETACH DATABASE shared`)

	// Ensure a local user to own the sampled cards (real user_ids may not exist
	// locally; we remap all sampled cards onto one local user).
	user := createTestUser(t, db, "realsmoke@example.com")

	// Copy up to 5000 real cards that have a card_state row, remapping user_id.
	// We only need the columns the scheduler reads.
	rows, err := db.db.Query(`
		SELECT c.lang, c.surface_norm, c.lemma, c.pos,
		       cs.fsrs_json, cs.next_due, cs.last_answer_at, cs.introduced_at
		  FROM shared.cards c
		  JOIN shared.card_state cs ON cs.card_id = c.id
		 WHERE c.mwe_id IS NULL
		 LIMIT 5000`)
	if err != nil {
		t.Skipf("could not read shared cards (%v); skipping", err)
	}
	defer rows.Close()

	type realRow struct {
		lang, surface, lemma, pos string
		fsrsJSON                  sql.NullString
		nextDue, lastAnswer       sql.NullString
		introduced                sql.NullString
	}
	var sampled []realRow
	for rows.Next() {
		var r realRow
		if err := rows.Scan(&r.lang, &r.surface, &r.lemma, &r.pos,
			&r.fsrsJSON, &r.nextDue, &r.lastAnswer, &r.introduced); err != nil {
			t.Fatalf("scan real row: %v", err)
		}
		sampled = append(sampled, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate real rows: %v", err)
	}

	if len(sampled) == 0 {
		t.Skip("shared DB has no card_state rows to sample; skipping real-DB smoke")
	}

	// Recreate each sampled card locally with its real scheduler state, then run
	// the drill: FSRS-rate, assert non-corrupt; rollback-rate, assert non-corrupt.
	now := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)
	rateSample := 0
	corrupt := 0
	for i, r := range sampled {
		lang := r.lang
		if lang == "" {
			lang = "FI"
		}
		cardID, err := db.EnsureCard(user.ID, lang, r.surface, fmt.Sprintf("%s#%d", r.lemma, i), r.pos)
		if err != nil {
			t.Fatalf("recreate sampled card %d: %v", i, err)
		}
		// Preserve the real state verbatim.
		var jsonp *string
		if r.fsrsJSON.Valid {
			s := r.fsrsJSON.String
			jsonp = &s
		}
		seedCardState(t, db, cardID,
			jsonp,
			sqlTimePtr(r.nextDue), sqlTimePtr(r.lastAnswer), sqlTimePtr(r.introduced))

		// Rate a bounded sample (every 5th, capped) through FSRS then rollback.
		if i%5 != 0 || rateSample >= 500 {
			continue
		}
		rateSample++

		if err := db.recordReviewAnswerAt(user.ID, cardID, "good", now, true); err != nil {
			t.Fatalf("real-smoke FSRS rate card %d (state=%q): %v", i, safeStr(r.fsrsJSON), err)
		}
		st := readCardState(t, db, cardID)
		if !st.rawFSRS.Valid || !isFSRSPayload(st.rawFSRS.String) {
			corrupt++
			t.Errorf("real-smoke card %d: FSRS rating did not persist a valid payload (seed=%q -> %q)",
				i, safeStr(r.fsrsJSON), st.rawFSRS.String)
		}
		if !st.nextDue.Valid || st.nextDue.String == "" {
			t.Errorf("real-smoke card %d: next_due not populated after FSRS rating", i)
		}
		// Rollback: flag OFF must also not error/corrupt.
		if err := db.recordReviewAnswerAt(user.ID, cardID, "good", now.Add(24*time.Hour), false); err != nil {
			t.Fatalf("real-smoke rollback rate card %d: %v", i, err)
		}
		stR := readCardState(t, db, cardID)
		if isFSRSPayload(stR.rawFSRS.String) {
			t.Errorf("real-smoke card %d: rollback should write step payload, got FSRS: %q", i, stR.rawFSRS.String)
		}
		var sched ReviewSchedule
		if err := json.Unmarshal([]byte(stR.rawFSRS.String), &sched); err != nil {
			t.Errorf("real-smoke card %d: rollback payload not a step schedule: %q", i, stR.rawFSRS.String)
		}
	}

	if corrupt != 0 {
		t.Fatalf("real-DB smoke: %d/%d rated cards produced corrupt FSRS state", corrupt, rateSample)
	}
	t.Logf("real-DB smoke: recreated %d real cards, rated %d through FSRS+rollback, 0 corrupt",
		len(sampled), rateSample)
}

// sqlTimePtr converts a nullable stored-time string into a *time.Time seed for
// seedCardState, preserving NULL.
func sqlTimePtr(v sql.NullString) *time.Time {
	if !v.Valid {
		return nil
	}
	return parseSQLiteTimePtr(v)
}

func safeStr(v sql.NullString) string {
	if !v.Valid {
		return "<NULL>"
	}
	return v.String
}
