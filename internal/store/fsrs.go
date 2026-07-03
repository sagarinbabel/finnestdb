package store

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	gofsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

// FSRS scheduling for the public alpha, behind FINNESTDB_FSRS_ENABLED.
//
// The step scheduler (nextAlphaStepScheduleForRating) stays the default and is
// untouched. When the flag is ON, RecordReviewAnswer routes through
// FSRSScheduleForRating instead. Both schedulers persist their state in the
// existing card_state.fsrs_json column; a version discriminator lets the legacy
// {step,streak} payload and the FSRS payload coexist so a flag flip (or
// rollback) never corrupts a card:
//
//   - Legacy step payload:  {"step":N,"streak":M}         (no "v" field)
//   - FSRS payload:         {"v":2,"fsrs":{...go-fsrs Card...}}
//
// next_due, last_answer_at and introduced_at keep their meaning regardless of
// which scheduler wrote the row, so queueing and reporting are scheduler-
// agnostic.

// fsrsPayloadVersion marks a card_state.fsrs_json row as holding an FSRS Card.
const fsrsPayloadVersion = 2

// fsrsPayload is the versioned envelope written when the FSRS scheduler owns a
// card's state. The Version field distinguishes it from the un-versioned legacy
// step payload (ReviewSchedule) that shares the same column.
type fsrsPayload struct {
	Version int         `json:"v"`
	FSRS    gofsrs.Card `json:"fsrs"`
}

// FSRSEnabled reports whether the FSRS scheduler is turned on via the
// FINNESTDB_FSRS_ENABLED environment flag. Default OFF: the step scheduler
// remains the shipped behavior until the flag is flipped as a deploy decision.
func FSRSEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FINNESTDB_FSRS_ENABLED"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// fsrsScheduler builds an FSRS engine with default parameters. Fuzz is off in
// DefaultParam, so scheduling is deterministic for a fixed (card, rating, now).
func fsrsScheduler() *gofsrs.FSRS {
	return gofsrs.NewFSRS(gofsrs.DefaultParam())
}

func ratingToFSRS(rating string) gofsrs.Rating {
	switch rating {
	case "again":
		return gofsrs.Again
	case "hard":
		return gofsrs.Hard
	case "easy":
		return gofsrs.Easy
	default: // good
		return gofsrs.Good
	}
}

// isFSRSPayload reports whether a raw fsrs_json string holds a versioned FSRS
// payload (vs. a legacy step payload or empty/NULL state).
func isFSRSPayload(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	var probe struct {
		Version int             `json:"v"`
		FSRS    json.RawMessage `json:"fsrs"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return false
	}
	return probe.Version >= fsrsPayloadVersion && len(probe.FSRS) > 0
}

// deriveFSRSCard produces a conservative starter FSRS card for a card whose
// stored state predates FSRS (legacy step payload, or NULL/empty state). This
// is the lazy migration-on-first-rating: it runs only when the flag is ON and
// the card has no FSRS payload yet.
//
// Derivation, without overclaiming precision:
//   - No prior review (lastAnswerAt zero / legacy step 0 and no answer):
//     a fresh New card. FSRS will initialize stability/difficulty from the
//     first rating.
//   - Prior review present: seed a Review-state card whose Stability is the
//     observed interval (next_due - last_answer_at) in days, clamped to a small
//     positive minimum, with default initial Difficulty. The interval is the
//     only real evidence the old step scheduler left behind; treating it as the
//     current memory stability is a deliberately modest seed, not a claim of
//     FSRS-quality history. LastReview/Due are set from the known timestamps.
func deriveFSRSCard(legacy ReviewSchedule, nextDue, lastAnswerAt *time.Time, now time.Time) gofsrs.Card {
	card := gofsrs.NewCard()

	if lastAnswerAt == nil || lastAnswerAt.IsZero() {
		// Never answered: a genuine new card. Due now so it stays in queue.
		card.Due = now
		return card
	}

	// Seen before: seed a Review-state card from the observed interval.
	intervalDays := 1.0
	if nextDue != nil && !nextDue.IsZero() {
		d := nextDue.Sub(*lastAnswerAt).Hours() / 24.0
		if d >= 1.0 {
			intervalDays = d
		}
	}

	param := gofsrs.DefaultParam()
	card.State = gofsrs.Review
	card.Stability = intervalDays
	// Default initial difficulty for a "Good" first rating — a neutral middle
	// starting point, since the step scheduler tracked no difficulty signal.
	card.Difficulty = clampDifficulty(param.W[4])
	card.Reps = uint64(legacy.Streak)
	card.ScheduledDays = uint64(intervalDays)
	card.ElapsedDays = uint64(intervalDays)
	card.LastReview = *lastAnswerAt
	if nextDue != nil && !nextDue.IsZero() {
		card.Due = *nextDue
	} else {
		card.Due = lastAnswerAt.Add(time.Duration(intervalDays) * 24 * time.Hour)
	}
	return card
}

func clampDifficulty(d float64) float64 {
	if d < 1 {
		return 1
	}
	if d > 10 {
		return 10
	}
	return d
}

// FSRSScheduleForRating advances a card's FSRS state for a rating at time now,
// returning the next due time and the versioned payload to persist. The stored
// state (raw fsrs_json), plus next_due/last_answer_at for lazy derivation, is
// passed in so a card with legacy or NULL state is migrated on first rating.
func FSRSScheduleForRating(
	rawFSRSJSON string,
	legacy ReviewSchedule,
	nextDue, lastAnswerAt *time.Time,
	rating string,
	now time.Time,
) (time.Time, string, error) {
	var card gofsrs.Card
	if isFSRSPayload(rawFSRSJSON) {
		var env fsrsPayload
		if err := json.Unmarshal([]byte(rawFSRSJSON), &env); err != nil {
			return time.Time{}, "", err
		}
		card = env.FSRS
	} else {
		// Lazy migration: derive a conservative starter card from whatever the
		// step scheduler left behind (or a fresh New card).
		card = deriveFSRSCard(legacy, nextDue, lastAnswerAt, now)
	}

	info := fsrsScheduler().Next(card, now, ratingToFSRS(rating))
	updated := info.Card

	payload, err := json.Marshal(fsrsPayload{Version: fsrsPayloadVersion, FSRS: updated})
	if err != nil {
		return time.Time{}, "", err
	}
	return updated.Due.UTC(), string(payload), nil
}

// stepScheduleFromStoredState returns the step-scheduler next-due + payload for
// a rating, tolerating a card whose state was previously written by the FSRS
// scheduler. This is the rollback path: if the flag is turned OFF after FSRS
// touched a card, the step scheduler must keep working without corruption.
//
// When the stored payload is FSRS, we cannot recover a Step/Streak, so we
// derive a ReviewSchedule.Step from the current interval (next_due - now) —
// bucketed into the step ladder — and hand it to the normal step scheduler.
// A fresh/legacy payload is parsed as a ReviewSchedule directly.
func stepScheduleFromStoredState(
	rawFSRSJSON string,
	legacy ReviewSchedule,
	nextDue *time.Time,
	rating string,
	now time.Time,
) (time.Time, ReviewSchedule) {
	schedule := legacy
	if isFSRSPayload(rawFSRSJSON) {
		// Rollback from FSRS: approximate a step from the scheduled interval so
		// the card doesn't snap back to step 0 and lose all progress.
		schedule = ReviewSchedule{Step: stepFromInterval(nextDue, now), Streak: 0}
	}
	return nextAlphaStepScheduleForRatingAt(schedule, rating, now)
}

// stepFromInterval maps a remaining interval (nextDue - now) onto the step
// ladder used by reviewIntervalForStep, so an FSRS-scheduled card falls back to
// a comparable step when the flag is turned off.
func stepFromInterval(nextDue *time.Time, now time.Time) int {
	if nextDue == nil || nextDue.IsZero() {
		return 0
	}
	days := nextDue.Sub(now).Hours() / 24.0
	// Ladder boundaries from reviewIntervalForStep (good path): 1,3,7,14,30,60.
	switch {
	case days < 2:
		return 0
	case days < 5:
		return 1
	case days < 10:
		return 2
	case days < 22:
		return 3
	case days < 45:
		return 4
	default:
		return 5
	}
}
