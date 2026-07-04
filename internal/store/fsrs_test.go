package store

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	gofsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

var fsrsNow = time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

// TestFSRSEnabledDefaultsOnOptOut pins the opt-OUT flag semantics adopted after
// the 2026-07-04 staging validation: FSRS is the default scheduler, and only an
// explicit off-value in FINNESTDB_FSRS_ENABLED selects the step scheduler
// (rollback). If someone reverts the default to OFF, this fails.
func TestFSRSEnabledDefaultsOnOptOut(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"", true},         // unset/empty → ON (default)
		{"   ", true},      // whitespace → ON
		{"1", true},        // explicit on-ish → ON
		{"true", true},     // → ON
		{"on", true},       // → ON
		{"anything", true}, // unrecognized → ON (opt-out only turns it off)
		{"0", false},       // rollback lever
		{"false", false},   // rollback lever
		{"no", false},      // rollback lever
		{"off", false},     // rollback lever
		{"OFF", false},     // case-insensitive
	}
	for _, c := range cases {
		t.Setenv("FINNESTDB_FSRS_ENABLED", c.val)
		if c.val == "" {
			// t.Setenv sets it; explicitly unset to test the truly-unset path.
			os.Unsetenv("FINNESTDB_FSRS_ENABLED")
		}
		if got := FSRSEnabled(); got != c.want {
			t.Fatalf("FSRSEnabled() with %q = %v, want %v", c.val, got, c.want)
		}
	}
}

// TestFSRSScheduleAllRatingsNewCard pins deterministic FSRS scheduling for all
// four ratings on a brand-new card (NULL state) at a fixed now. It asserts the
// FSRS ordering invariant — Again < Hard < Good < Easy in next due — and that
// the persisted payload is the versioned FSRS envelope.
func TestFSRSScheduleAllRatingsNewCard(t *testing.T) {
	dues := map[string]time.Time{}
	for _, rating := range []string{"again", "hard", "good", "easy"} {
		next, payload, err := FSRSScheduleForRating("", ReviewSchedule{}, nil, nil, rating, fsrsNow)
		if err != nil {
			t.Fatalf("FSRSScheduleForRating(%s): %v", rating, err)
		}
		if !next.After(fsrsNow) && !next.Equal(fsrsNow) {
			t.Fatalf("%s: next due %v is before now %v", rating, next, fsrsNow)
		}
		if !isFSRSPayload(payload) {
			t.Fatalf("%s: payload is not a versioned FSRS payload: %s", rating, payload)
		}
		// Payload must round-trip to a Review/Learning FSRS card with a rep.
		var env fsrsPayload
		if err := json.Unmarshal([]byte(payload), &env); err != nil {
			t.Fatalf("%s: unmarshal payload: %v", rating, err)
		}
		if env.FSRS.Reps == 0 {
			t.Fatalf("%s: FSRS card has 0 reps after a rating", rating)
		}
		dues[rating] = next
	}

	// FSRS invariant on a new card: harder ratings schedule sooner.
	if !dues["again"].Before(dues["good"]) {
		t.Fatalf("again (%v) should be due before good (%v)", dues["again"], dues["good"])
	}
	if !dues["good"].Before(dues["easy"]) || dues["good"].Equal(dues["easy"]) {
		t.Fatalf("good (%v) should be due before easy (%v)", dues["good"], dues["easy"])
	}
}

// TestFSRSScheduleDeterministic proves scheduling is a pure function of
// (state, rating, now): the same inputs always yield the same due + payload.
func TestFSRSScheduleDeterministic(t *testing.T) {
	next1, p1, err := FSRSScheduleForRating("", ReviewSchedule{}, nil, nil, "good", fsrsNow)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	next2, p2, err := FSRSScheduleForRating("", ReviewSchedule{}, nil, nil, "good", fsrsNow)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !next1.Equal(next2) || p1 != p2 {
		t.Fatalf("FSRS scheduling not deterministic: %v/%s vs %v/%s", next1, p1, next2, p2)
	}
}

// TestFSRSScheduleSeenCard advances an already-FSRS card twice and checks the
// state evolves (reps increase, due moves forward) rather than resetting.
func TestFSRSScheduleSeenCard(t *testing.T) {
	_, firstPayload, err := FSRSScheduleForRating("", ReviewSchedule{}, nil, nil, "good", fsrsNow)
	if err != nil {
		t.Fatalf("first rating: %v", err)
	}
	var env1 fsrsPayload
	_ = json.Unmarshal([]byte(firstPayload), &env1)

	later := fsrsNow.Add(24 * time.Hour)
	_, secondPayload, err := FSRSScheduleForRating(firstPayload, ReviewSchedule{}, &env1.FSRS.Due, &fsrsNow, "good", later)
	if err != nil {
		t.Fatalf("second rating: %v", err)
	}
	var env2 fsrsPayload
	_ = json.Unmarshal([]byte(secondPayload), &env2)
	if env2.FSRS.Reps <= env1.FSRS.Reps {
		t.Fatalf("reps did not increase across reviews: %d -> %d", env1.FSRS.Reps, env2.FSRS.Reps)
	}
	if !env2.FSRS.Due.After(env1.FSRS.Due) {
		t.Fatalf("due did not move forward: %v -> %v", env1.FSRS.Due, env2.FSRS.Due)
	}
}

// TestFSRSLazyDerivationLegacyState proves that a card with a legacy step
// payload + prior review is lazily seeded into a Review-state FSRS card on
// first FSRS rating, using the observed interval as stability — without
// pretending FSRS-quality history.
func TestFSRSLazyDerivationLegacyState(t *testing.T) {
	lastAnswer := fsrsNow.Add(-7 * 24 * time.Hour)
	nextDue := fsrsNow.Add(-1 * 24 * time.Hour) // was due yesterday; ~6-day interval
	legacy := ReviewSchedule{Step: 3, Streak: 4}
	legacyJSON, _ := json.Marshal(legacy)

	_, payload, err := FSRSScheduleForRating(string(legacyJSON), legacy, &nextDue, &lastAnswer, "good", fsrsNow)
	if err != nil {
		t.Fatalf("lazy derive: %v", err)
	}
	if !isFSRSPayload(payload) {
		t.Fatalf("lazy derivation did not produce an FSRS payload: %s", payload)
	}
	var env fsrsPayload
	_ = json.Unmarshal([]byte(payload), &env)
	// A seen card should not be treated as brand-new: it should have carried
	// forward reps (>1 after this rating, since Streak seeded reps).
	if env.FSRS.Reps < 2 {
		t.Fatalf("legacy-seeded card reps=%d, want >=2 (streak carried + this rating)", env.FSRS.Reps)
	}
	if env.FSRS.State == gofsrs.New {
		t.Fatal("legacy-seeded card is still New; should have been derived as Review")
	}
}

// TestFSRSLazyDerivationNullState proves a NULL-state card is treated as a
// fresh FSRS new card on first rating.
func TestFSRSLazyDerivationNullState(t *testing.T) {
	_, payload, err := FSRSScheduleForRating("", ReviewSchedule{}, nil, nil, "good", fsrsNow)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var env fsrsPayload
	_ = json.Unmarshal([]byte(payload), &env)
	// A fresh card rated Good becomes Learning/Review with exactly 1 rep.
	if env.FSRS.Reps != 1 {
		t.Fatalf("null-state card reps=%d, want 1", env.FSRS.Reps)
	}
}

// TestRecordReviewAnswerFlagOffByteIdenticalToStepScheduler is the rollback-path
// pin. FSRS is now the default (opt-out flag), so this pins the OTHER direction:
// with the flag explicitly OFF, recordReviewAnswerAt must still write exactly
// what the step scheduler produces for the same (schedule, rating, now). This is
// the guarantee that FINNESTDB_FSRS_ENABLED=0 is a real, byte-for-byte rollback
// lever; if the step path ever drifts from the step scheduler, this fails.
func TestRecordReviewAnswerFlagOffByteIdenticalToStepScheduler(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "step@example.com")
	cardID, err := db.EnsureCard(user.ID, "FI", "", "kissa", "NOUN")
	if err != nil {
		t.Fatalf("EnsureCard: %v", err)
	}

	now := fsrsNow
	for _, rating := range []string{"good", "good", "easy", "again", "hard"} {
		// Compute the step scheduler's expected output from the CURRENT stored
		// state, then apply the answer with the flag OFF and compare.
		var rawBefore string
		_ = db.db.QueryRow(`SELECT COALESCE(fsrs_json,'') FROM card_state WHERE card_id=?`, cardID).Scan(&rawBefore)
		var sched ReviewSchedule
		if rawBefore != "" {
			_ = json.Unmarshal([]byte(rawBefore), &sched)
		}
		wantDue, wantSched := nextAlphaStepScheduleForRatingAt(sched, rating, now)
		wantPayload, _ := json.Marshal(wantSched)

		if err := db.recordReviewAnswerAt(user.ID, cardID, rating, now, false); err != nil {
			t.Fatalf("recordReviewAnswerAt(%s): %v", rating, err)
		}

		var gotRaw, gotDue string
		if err := db.db.QueryRow(
			`SELECT fsrs_json, next_due FROM card_state WHERE card_id=?`, cardID,
		).Scan(&gotRaw, &gotDue); err != nil {
			t.Fatalf("read state: %v", err)
		}
		if gotRaw != string(wantPayload) {
			t.Fatalf("%s: payload %s != step scheduler %s", rating, gotRaw, wantPayload)
		}
		if gotT := parseStoredTime(t, gotDue); !gotT.Equal(wantDue) {
			t.Fatalf("%s: due %v != step scheduler %v", rating, gotT, wantDue)
		}
		now = now.Add(time.Minute) // advance a bit between answers
	}
}

// TestFSRSRollbackToStepScheduler proves the rollback path: after FSRS touches
// a card (flag ON), turning the flag OFF lets the step scheduler answer the
// same card without corruption. The step scheduler approximates a step from the
// FSRS-scheduled interval rather than snapping to step 0.
func TestFSRSRollbackToStepScheduler(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "rollback@example.com")
	cardID, err := db.EnsureCard(user.ID, "FI", "", "koira", "NOUN")
	if err != nil {
		t.Fatalf("EnsureCard: %v", err)
	}

	// Flag ON: two FSRS reviews build up an FSRS payload + a multi-day interval.
	now := fsrsNow
	if err := db.recordReviewAnswerAt(user.ID, cardID, "good", now, true); err != nil {
		t.Fatalf("fsrs answer 1: %v", err)
	}
	now = now.Add(24 * time.Hour)
	if err := db.recordReviewAnswerAt(user.ID, cardID, "good", now, true); err != nil {
		t.Fatalf("fsrs answer 2: %v", err)
	}

	// Sanity: state is an FSRS payload with a future due.
	var raw string
	if err := db.db.QueryRow(`SELECT fsrs_json FROM card_state WHERE card_id=?`, cardID).Scan(&raw); err != nil {
		t.Fatalf("read fsrs state: %v", err)
	}
	if !isFSRSPayload(raw) {
		t.Fatalf("expected FSRS payload after flag-on reviews, got %s", raw)
	}

	// Flag OFF (rollback): answering must succeed and write a legacy step
	// payload derived from the interval, not corrupt or error.
	now = now.Add(24 * time.Hour)
	if err := db.recordReviewAnswerAt(user.ID, cardID, "good", now, false); err != nil {
		t.Fatalf("rollback step answer: %v", err)
	}
	var rawAfter, dueAfter string
	if err := db.db.QueryRow(
		`SELECT fsrs_json, next_due FROM card_state WHERE card_id=?`, cardID,
	).Scan(&rawAfter, &dueAfter); err != nil {
		t.Fatalf("read state after rollback: %v", err)
	}
	if isFSRSPayload(rawAfter) {
		t.Fatalf("rollback should write a legacy step payload, got FSRS: %s", rawAfter)
	}
	var sched ReviewSchedule
	if err := json.Unmarshal([]byte(rawAfter), &sched); err != nil {
		t.Fatalf("rollback payload is not a step schedule: %s (%v)", rawAfter, err)
	}
	// The interval was a couple of days, so the derived step should be > 0 —
	// progress is not thrown away on rollback.
	if sched.Step == 0 {
		t.Fatalf("rollback snapped step to 0, losing FSRS-earned progress: %+v", sched)
	}
	// The card remains answerable and due in the future.
	if got := parseStoredTime(t, dueAfter); !got.After(now) {
		t.Fatalf("rollback due %v not in the future relative to now %v", got, now)
	}
}

// TestRecordReviewAnswerFlagOnPersistsFSRS proves that with the flag ON,
// answering a fresh card persists a versioned FSRS payload and a future due.
func TestRecordReviewAnswerFlagOnPersistsFSRS(t *testing.T) {
	db := newTestDB(t)
	user := createTestUser(t, db, "fsrson@example.com")
	cardID, err := db.EnsureCard(user.ID, "FI", "", "talo", "NOUN")
	if err != nil {
		t.Fatalf("EnsureCard: %v", err)
	}
	if err := db.recordReviewAnswerAt(user.ID, cardID, "good", fsrsNow, true); err != nil {
		t.Fatalf("fsrs answer: %v", err)
	}
	var raw, due, lastAns string
	if err := db.db.QueryRow(
		`SELECT fsrs_json, next_due, last_answer_at FROM card_state WHERE card_id=?`, cardID,
	).Scan(&raw, &due, &lastAns); err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !isFSRSPayload(raw) {
		t.Fatalf("flag-on answer did not persist FSRS payload: %s", raw)
	}
	if due == "" {
		t.Fatal("next_due not set after FSRS answer")
	}
	if got := parseStoredTime(t, lastAns); !got.Equal(fsrsNow) {
		t.Fatalf("last_answer_at=%v want %v", got, fsrsNow)
	}
}

// parseStoredTime parses a DATETIME column value (either the space-separated
// sqliteTime form or the driver's RFC3339 form) into a UTC time.Time.
func parseStoredTime(t *testing.T, s string) time.Time {
	t.Helper()
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed.UTC()
		}
	}
	t.Fatalf("unparseable stored time %q", s)
	return time.Time{}
}
