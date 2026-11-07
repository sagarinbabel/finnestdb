package store

import (
	"errors"
	"fmt"
)

// This file is the single source of truth for the app's state machines: which
// states exist, which transitions are legal, and which free-text inputs are
// accepted into state-bearing columns.
//
// Before this file, each machine was implicit: the states were bare string
// literals, the valid set was duplicated in an HTTP-layer helper, and the
// transitions were whichever UPDATE statements happened to exist. Nothing
// checked the *current* state before writing the next one, so e.g. an "open"
// correction issue could be marked "fixed" without ever having been
// quarantined, and an accepted parse-feedback row could be re-accepted
// repeatedly - each time re-running the lexical writeback.
//
// Two layers enforce this now:
//
//  1. These validators, called by every store writer. They cover every caller,
//     including the ~30 cmd/ tools that bypass the HTTP layer entirely.
//  2. CHECK constraints in the CREATE TABLE statements (see initSchema), which
//     cover fresh databases. Pre-existing database files do not gain the
//     constraint - adding one to a populated SQLite table needs a full table
//     rebuild, which is not worth the risk here precisely because layer 1
//     already covers every writer.

// ErrInvalidState is returned when a state-bearing value is not one of the
// states its column recognizes.
var ErrInvalidState = errors.New("invalid state")

// ErrIllegalTransition is returned when a state change is not a legal edge of
// its state machine.
var ErrIllegalTransition = errors.New("illegal state transition")

// --- Parse feedback review status -------------------------------------------

// Parse-feedback review statuses. A report is created "submitted" and an admin
// resolves it into exactly one of the other three. Resolved states are
// terminal: see CanTransitionFeedbackStatus for why.
const (
	FeedbackStatusSubmitted     = "submitted"
	FeedbackStatusAccepted      = "accepted"
	FeedbackStatusRejected      = "rejected"
	FeedbackStatusNeedsFollowUp = "needs_follow_up"
)

// IsValidFeedbackStatus reports whether s is a recognized parse-feedback
// review status.
func IsValidFeedbackStatus(s string) bool {
	switch s {
	case FeedbackStatusSubmitted, FeedbackStatusAccepted,
		FeedbackStatusRejected, FeedbackStatusNeedsFollowUp:
		return true
	default:
		return false
	}
}

// CanTransitionFeedbackStatus reports whether a review can move from->to.
//
// The only legal edges are out of "submitted", plus the identity edge (a
// no-op re-save of the same status, which the admin UI can issue when only the
// review note changed).
//
// Accepting is deliberately one-way. Acceptance writes a custom_overrides
// lexical row that outranks every imported dictionary source, and a status
// flip cannot retract parser behavior that learners have already been taught
// against or that has already been promoted to a gold candidate. An admin who
// changes their mind needs an explicit retraction (RetractParseFeedbackOverride),
// not a second status write that silently leaves the override in place.
func CanTransitionFeedbackStatus(from, to string) bool {
	if !IsValidFeedbackStatus(from) || !IsValidFeedbackStatus(to) {
		return false
	}
	if from == to {
		return true
	}
	return from == FeedbackStatusSubmitted
}

// ValidateFeedbackTransition returns a descriptive error when from->to is not
// a legal parse-feedback review transition.
func ValidateFeedbackTransition(from, to string) error {
	if !IsValidFeedbackStatus(to) {
		return fmt.Errorf("%w: feedback status %q", ErrInvalidState, to)
	}
	if !CanTransitionFeedbackStatus(from, to) {
		return fmt.Errorf("%w: feedback is already %q and cannot become %q; retract the override first",
			ErrIllegalTransition, from, to)
	}
	return nil
}

// --- Correction-issue lifecycle ---------------------------------------------

// CanTransitionIssueStatus reports whether a correction issue can move
// from->to. The machine, with the action that drives each edge:
//
//	open       --quarantine--> quarantined
//	open       --fix---------> fixed          (triaged as a non-parser class,
//	                                           or fixed upstream without ever
//	                                           needing suppression)
//	quarantined --fix--------> fixed
//	fixed      --new report--> reopened
//	reopened   --quarantine--> quarantined
//	reopened   --fix---------> fixed
//
// Notably absent: fixed -> quarantined. A fixed issue that turns out to still
// be broken arrives via a new report, which moves it to "reopened" first and
// bumps reopened_count, so the admin queue can tell a regression from a fresh
// case. Allowing the direct edge is what let a row carry both fixed_at and
// quarantined_at with no way to say which was current.
func CanTransitionIssueStatus(from, to string) bool {
	if !IsValidIssueStatus(from) || !IsValidIssueStatus(to) {
		return false
	}
	if from == to {
		return true
	}
	switch to {
	case IssueStatusQuarantined:
		return from == IssueStatusOpen || from == IssueStatusReopened
	case IssueStatusFixed:
		return from == IssueStatusOpen || from == IssueStatusQuarantined || from == IssueStatusReopened
	case IssueStatusReopened:
		return from == IssueStatusFixed
	case IssueStatusOpen:
		// "open" is only ever an initial state; nothing reverts to it.
		return false
	default:
		return false
	}
}

// IsValidIssueStatus reports whether s is a recognized correction-issue status.
func IsValidIssueStatus(s string) bool {
	switch s {
	case IssueStatusOpen, IssueStatusQuarantined, IssueStatusFixed, IssueStatusReopened:
		return true
	default:
		return false
	}
}

// ValidateIssueTransition returns a descriptive error when from->to is not a
// legal correction-issue transition.
func ValidateIssueTransition(from, to string) error {
	if !IsValidIssueStatus(to) {
		return fmt.Errorf("%w: correction-issue status %q", ErrInvalidState, to)
	}
	if !CanTransitionIssueStatus(from, to) {
		return fmt.Errorf("%w: correction issue is %q and cannot become %q",
			ErrIllegalTransition, from, to)
	}
	return nil
}

// --- Gold-candidate lifecycle -----------------------------------------------

// Gold-candidate statuses. A promoted candidate is "pending" until
// cmd/exportgoldcandidates renders it for manual review.
const (
	GoldCandidateStatusPending  = "pending"
	GoldCandidateStatusExported = "exported"
)

// IsValidGoldCandidateStatus reports whether s is a recognized gold-candidate
// status.
func IsValidGoldCandidateStatus(s string) bool {
	switch s {
	case GoldCandidateStatusPending, GoldCandidateStatusExported:
		return true
	default:
		return false
	}
}

// --- Review ratings ---------------------------------------------------------

// The four review ratings, matching FSRS's grade scale.
const (
	RatingAgain = "again"
	RatingHard  = "hard"
	RatingGood  = "good"
	RatingEasy  = "easy"
)

// IsValidRating reports whether s is one of the four review ratings.
//
// This matters more than it looks: ratingToFSRS maps any unrecognized string
// to Good, and nextAlphaStepScheduleForRatingAt treats it as the default
// branch, so an unvalidated rating silently advances a card instead of
// erroring. review_log.rating then records the bad value verbatim.
func IsValidRating(s string) bool {
	switch s {
	case RatingAgain, RatingHard, RatingGood, RatingEasy:
		return true
	default:
		return false
	}
}

// --- Parser mode ------------------------------------------------------------

// The two dictionary lookup modes. "custom" runs the FST + enrichment
// pipeline; "basic" is the SQLite dictionary lookup alone.
//
// These are named constants so store code stops comparing against bare
// "custom" literals, but they are NOT a validator: parsecore's parser registry
// is the single source of truth for which parser names exist, and
// parsecore.IsRegisteredParser is what callers should check against. Defining
// a second list here is how the two would drift.
const (
	ParserBasic  = "basic"
	ParserCustom = "custom"
)

// --- Known-lemma provenance -------------------------------------------------

// IsValidKnownWordSource reports whether s is a recognized
// user_known_lemmas.source value.
func IsValidKnownWordSource(s string) bool {
	switch s {
	case SourceManual, SourceAnki:
		return true
	default:
		return false
	}
}
