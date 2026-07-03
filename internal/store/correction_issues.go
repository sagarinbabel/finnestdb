package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// QuarantineThreshold is the number of distinct reporters against one issue at
// which the admin queue shows a threshold_candidate badge. It is a display-only
// signal for alpha: reaching it never auto-quarantines (Decision 25, "trusted
// thresholds are traceability-only"). Admin confirmation is always required.
const QuarantineThreshold = 3

// Alpha triage classes. Exactly one must be set before an issue can be
// quarantined or marked fixed (docs/PARSER_FEEDBACK_LOOP.md "Alpha admin
// classification").
const (
	AlphaClassParserIssue    = "parser_issue"
	AlphaClassBadCardContent = "bad_card_content"
	AlphaClassSourceIssue    = "source_extraction_issue"
	AlphaClassNotSure        = "not_sure"
)

// Correction-issue lifecycle statuses.
const (
	IssueStatusOpen        = "open"
	IssueStatusQuarantined = "quarantined"
	IssueStatusFixed       = "fixed"
	IssueStatusReopened    = "reopened"
)

// ErrIssueNeedsClass is returned when an admin tries to quarantine or fix an
// issue that has not been triaged into a simple alpha class yet.
var ErrIssueNeedsClass = errors.New("issue must be triaged with an alpha class before quarantine or fix")

// ErrQuarantineReasonRequired is returned when a "Quarantine now" action omits
// the required reason.
var ErrQuarantineReasonRequired = errors.New("quarantine requires a reason")

// IsValidAlphaClass reports whether c is one of the four alpha triage classes.
func IsValidAlphaClass(c string) bool {
	switch c {
	case AlphaClassParserIssue, AlphaClassBadCardContent, AlphaClassSourceIssue, AlphaClassNotSure:
		return true
	default:
		return false
	}
}

// CorrectionIssue is one row of the global correction-issue ledger.
type CorrectionIssue struct {
	ID                    int64      `json:"id"`
	Lang                  string     `json:"lang"`
	Parser                string     `json:"parser"`
	NormSurface           string     `json:"norm_surface"`
	Lemma                 string     `json:"lemma"`
	POS                   string     `json:"pos"`
	Status                string     `json:"status"`
	AlphaClass            string     `json:"alpha_class,omitempty"`
	ReportCount           int        `json:"report_count"`
	DistinctReporterCount int        `json:"distinct_reporter_count"`
	FirstReportedAt       *time.Time `json:"first_reported_at,omitempty"`
	LastReportedAt        *time.Time `json:"last_reported_at,omitempty"`
	QuarantinedAt         *time.Time `json:"quarantined_at,omitempty"`
	QuarantinedBy         *int64     `json:"quarantined_by,omitempty"`
	QuarantineReason      string     `json:"quarantine_reason,omitempty"`
	FixedAt               *time.Time `json:"fixed_at,omitempty"`
	FixedBy               *int64     `json:"fixed_by,omitempty"`
	FixNote               string     `json:"fix_note,omitempty"`
	ReopenedCount         int        `json:"reopened_count"`
	AdminNote             string     `json:"admin_note,omitempty"`
	// ThresholdCandidate is a derived display flag: true when
	// DistinctReporterCount >= QuarantineThreshold and the issue is not already
	// quarantined/fixed. Never used to auto-quarantine.
	ThresholdCandidate bool `json:"threshold_candidate"`
}

// normalizeIssueSurface produces the surface component of the scope
// fingerprint. Matching is case-insensitive on a trimmed surface, mirroring how
// custom_overrides forms are stored (lower-cased). Keeping this deterministic
// and in one place is what lets the suppression queries below reproduce the
// same key.
func normalizeIssueSurface(surface string) string {
	return strings.ToLower(strings.TrimSpace(surface))
}

// groupFeedbackIntoIssue finds or creates the correction_issues row matching a
// report's conservative scope fingerprint, bumps its counts, and returns the
// issue id so the caller can link the parse_feedback row.
//
// Scope fingerprint = (lang, parser, norm_surface, lemma, pos). lemma/pos are
// the report's proposed identity when present (a parser-identity claim), else
// empty (a flag-only or surface-only report). A fixed issue that gets a new
// matching report is reopened and its reopened_count bumped, so admins can tell
// a regression from a fresh case.
//
// reportedAt is passed in (rather than read from the row) so the caller can use
// the same timestamp it stamped on the feedback row. The reporter's user id is
// not needed here — distinct_reporter_count is recomputed from the linked
// parse_feedback rows by recountIssue after the row is linked.
func groupFeedbackIntoIssue(tx *sql.Tx, lang, parser, surface, lemma, pos string, reportedAt time.Time) (int64, error) {
	lang = strings.TrimSpace(lang)
	parser = strings.TrimSpace(parser)
	norm := normalizeIssueSurface(surface)
	lemma = strings.TrimSpace(lemma)
	pos = strings.TrimSpace(pos)

	var (
		issueID int64
		status  string
	)
	err := tx.QueryRow(
		`SELECT id, status FROM correction_issues
		  WHERE lang = ? AND parser = ? AND norm_surface = ? AND lemma = ? AND pos = ?`,
		lang, parser, norm, lemma, pos,
	).Scan(&issueID, &status)
	switch {
	case err == sql.ErrNoRows:
		res, err := tx.Exec(
			`INSERT INTO correction_issues
				(lang, parser, norm_surface, lemma, pos, status,
				 report_count, distinct_reporter_count, first_reported_at, last_reported_at)
			 VALUES (?, ?, ?, ?, ?, 'open', 0, 0, ?, ?)`,
			lang, parser, norm, lemma, pos, reportedAt, reportedAt,
		)
		if err != nil {
			return 0, err
		}
		issueID, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	case err != nil:
		return 0, err
	default:
		// A new report against a fixed issue is a possible regression: reopen it
		// and bump reopened_count so the admin can tell it apart from a still-
		// open case.
		if status == IssueStatusFixed {
			if _, err := tx.Exec(
				`UPDATE correction_issues
				    SET status = 'reopened', reopened_count = reopened_count + 1
				  WHERE id = ?`,
				issueID,
			); err != nil {
				return 0, err
			}
		}
	}

	// Recompute counts from the linked feedback rows AFTER this report is
	// linked, so counting is authoritative rather than incremental (survives
	// re-links and avoids double-counting on retries). The caller links the row
	// before calling recountIssue; here we only bump last_reported_at.
	if _, err := tx.Exec(
		`UPDATE correction_issues
		    SET last_reported_at = ?,
		        first_reported_at = COALESCE(first_reported_at, ?)
		  WHERE id = ?`,
		reportedAt, reportedAt, issueID,
	); err != nil {
		return 0, err
	}
	return issueID, nil
}

// recountIssue recomputes report_count and distinct_reporter_count from the
// linked parse_feedback rows. Called after a report is linked so the counts are
// derived (not incrementally maintained), which keeps them correct even if a
// feedback row is re-grouped.
func recountIssue(tx *sql.Tx, issueID int64) error {
	_, err := tx.Exec(
		`UPDATE correction_issues
		    SET report_count = (
		            SELECT COUNT(*) FROM parse_feedback WHERE correction_issue_id = ?1
		        ),
		        distinct_reporter_count = (
		            SELECT COUNT(DISTINCT user_id) FROM parse_feedback WHERE correction_issue_id = ?1
		        )
		  WHERE id = ?1`,
		issueID,
	)
	return err
}

// CorrectionIssueFilter narrows the admin issue listing. Empty Status means
// "any status". Lang narrows by language when set.
type CorrectionIssueFilter struct {
	Status string
	Lang   string
}

// ListCorrectionIssues returns the correction-issue ledger for admin triage,
// newest activity first. threshold_candidate is derived per row.
func (d *DB) ListCorrectionIssues(filter CorrectionIssueFilter) ([]CorrectionIssue, error) {
	query := `SELECT id, lang, parser, norm_surface, lemma, pos, status,
		COALESCE(alpha_class, ''), report_count, distinct_reporter_count,
		first_reported_at, last_reported_at,
		quarantined_at, quarantined_by, COALESCE(quarantine_reason, ''),
		fixed_at, fixed_by, COALESCE(fix_note, ''),
		reopened_count, COALESCE(admin_note, '')
		FROM correction_issues`
	conds := []string{}
	args := []any{}
	if filter.Status != "" {
		conds = append(conds, `status = ?`)
		args = append(args, filter.Status)
	}
	if filter.Lang != "" {
		conds = append(conds, `lang = ?`)
		args = append(args, filter.Lang)
	}
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, ` AND `)
	}
	query += ` ORDER BY COALESCE(last_reported_at, first_reported_at) DESC, id DESC`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	issues := []CorrectionIssue{}
	for rows.Next() {
		item, err := scanCorrectionIssue(rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, item)
	}
	return issues, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCorrectionIssue(row rowScanner) (CorrectionIssue, error) {
	var item CorrectionIssue
	var (
		alphaClass                       string
		firstAt, lastAt, quarAt, fixedAt sql.NullTime
		quarBy, fixedBy                  sql.NullInt64
	)
	if err := row.Scan(
		&item.ID, &item.Lang, &item.Parser, &item.NormSurface, &item.Lemma, &item.POS, &item.Status,
		&alphaClass, &item.ReportCount, &item.DistinctReporterCount,
		&firstAt, &lastAt,
		&quarAt, &quarBy, &item.QuarantineReason,
		&fixedAt, &fixedBy, &item.FixNote,
		&item.ReopenedCount, &item.AdminNote,
	); err != nil {
		return item, err
	}
	item.AlphaClass = alphaClass
	if firstAt.Valid {
		item.FirstReportedAt = &firstAt.Time
	}
	if lastAt.Valid {
		item.LastReportedAt = &lastAt.Time
	}
	if quarAt.Valid {
		item.QuarantinedAt = &quarAt.Time
	}
	if quarBy.Valid {
		item.QuarantinedBy = &quarBy.Int64
	}
	if fixedAt.Valid {
		item.FixedAt = &fixedAt.Time
	}
	if fixedBy.Valid {
		item.FixedBy = &fixedBy.Int64
	}
	item.ThresholdCandidate = item.DistinctReporterCount >= QuarantineThreshold &&
		item.Status != IssueStatusQuarantined && item.Status != IssueStatusFixed
	return item, nil
}

// CorrectionIssueIDForFeedback returns the correction issue a feedback row was
// grouped into, or (0, false) if it has none. Used by admin tooling and tests
// to walk from a report to its global issue.
func (d *DB) CorrectionIssueIDForFeedback(feedbackID int64) (int64, bool, error) {
	var id sql.NullInt64
	err := d.db.QueryRow(
		`SELECT correction_issue_id FROM parse_feedback WHERE id = ?`, feedbackID,
	).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	return id.Int64, id.Valid, nil
}

// GetCorrectionIssue returns a single issue by id.
func (d *DB) GetCorrectionIssue(issueID int64) (*CorrectionIssue, error) {
	row := d.db.QueryRow(
		`SELECT id, lang, parser, norm_surface, lemma, pos, status,
			COALESCE(alpha_class, ''), report_count, distinct_reporter_count,
			first_reported_at, last_reported_at,
			quarantined_at, quarantined_by, COALESCE(quarantine_reason, ''),
			fixed_at, fixed_by, COALESCE(fix_note, ''),
			reopened_count, COALESCE(admin_note, '')
		 FROM correction_issues WHERE id = ?`,
		issueID,
	)
	item, err := scanCorrectionIssue(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// TriageCorrectionIssue records the admin's simple alpha class and optional
// note. Triage is a prerequisite for quarantine/fix but does not itself change
// circulation.
func (d *DB) TriageCorrectionIssue(issueID int64, alphaClass, adminNote string) error {
	alphaClass = strings.TrimSpace(alphaClass)
	if !IsValidAlphaClass(alphaClass) {
		return fmt.Errorf("invalid alpha class %q", alphaClass)
	}
	res, err := d.db.Exec(
		`UPDATE correction_issues SET alpha_class = ?, admin_note = ? WHERE id = ?`,
		alphaClass, strings.TrimSpace(adminNote), issueID,
	)
	if err != nil {
		return err
	}
	return errIfNoRows(res)
}

// QuarantineCorrectionIssue is the admin-only "Quarantine now" action: it marks
// the issue quarantined so its scoped content is suppressed globally. It
// requires a prior alpha class (ErrIssueNeedsClass) and a non-empty reason
// (ErrQuarantineReasonRequired). Suppression is applied by the review/new-card
// and stats queries filtering on quarantined issues — see
// suppressedCardPredicate / suppressedOccurrencePredicate.
func (d *DB) QuarantineCorrectionIssue(issueID, adminUserID int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrQuarantineReasonRequired
	}
	issue, err := d.GetCorrectionIssue(issueID)
	if err != nil {
		return err
	}
	if issue.AlphaClass == "" {
		return ErrIssueNeedsClass
	}
	res, err := d.db.Exec(
		`UPDATE correction_issues
		    SET status = 'quarantined',
		        quarantined_at = CURRENT_TIMESTAMP,
		        quarantined_by = ?,
		        quarantine_reason = ?
		  WHERE id = ?`,
		adminUserID, reason, issueID,
	)
	if err != nil {
		return err
	}
	return errIfNoRows(res)
}

// RestoreCorrectionIssue marks a quarantined issue fixed, returning its scoped
// content to circulation. Because suppression is a live query filter (no card
// or scheduler rows are deleted at quarantine time), restoring is a pure status
// flip: the original card_state scheduler state is untouched throughout, so a
// restored card resumes with its existing due date and history.
func (d *DB) RestoreCorrectionIssue(issueID, adminUserID int64, fixNote string) error {
	issue, err := d.GetCorrectionIssue(issueID)
	if err != nil {
		return err
	}
	if issue.AlphaClass == "" {
		return ErrIssueNeedsClass
	}
	res, err := d.db.Exec(
		`UPDATE correction_issues
		    SET status = 'fixed',
		        fixed_at = CURRENT_TIMESTAMP,
		        fixed_by = ?,
		        fix_note = ?
		  WHERE id = ?`,
		adminUserID, strings.TrimSpace(fixNote), issueID,
	)
	if err != nil {
		return err
	}
	return errIfNoRows(res)
}

func errIfNoRows(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// --- Suppression predicates -------------------------------------------------
//
// Quarantine suppresses matching learner-facing content globally. Two match
// modes, deterministic and documented here because several queries embed them:
//
//   - Card scope: an issue with a concrete (lemma, pos) suppresses the review
//     card and occurrences of that (lang, lemma, pos). This is the parser-
//     identity scope — the analysis said "kissa/NOUN" and that identity is bad.
//   - Surface scope: an issue with empty lemma/pos (flag-only / surface-only)
//     suppresses occurrences of that (lang, normalized surface). Cards have no
//     surface column, so a surface-only issue cannot target a card by itself;
//     it suppresses the deck occurrences that carry that surface.
//
// Only status = 'quarantined' issues suppress. open/fixed/reopened do not.

// suppressedCardPredicate returns a SQL boolean expression (for a WHERE/CASE
// clause) that is TRUE when the card aliased as cardAlias is suppressed by a
// quarantined issue. It matches on (lang, lemma, pos) against issues that carry
// a concrete lemma/pos.
func suppressedCardPredicate(cardAlias string) string {
	return `EXISTS (
		SELECT 1 FROM correction_issues ci
		 WHERE ci.status = 'quarantined'
		   AND ci.lemma <> '' AND ci.pos <> ''
		   AND ci.lang = ` + cardAlias + `.lang
		   AND ci.lemma = ` + cardAlias + `.lemma
		   AND ci.pos = ` + cardAlias + `.pos
	)`
}

// suppressedOccurrencePredicate returns a SQL boolean expression that is TRUE
// when the occurrence aliased as occAlias is suppressed by a quarantined issue.
// It matches either the (lang, lemma, pos) card scope or the (lang, normalized
// surface) surface scope. deckLang is a SQL expression yielding the occurrence's
// language (occurrence has no lang column; callers pass the deck's lang).
func suppressedOccurrencePredicate(occAlias, deckLang string) string {
	return `EXISTS (
		SELECT 1 FROM correction_issues ci
		 WHERE ci.status = 'quarantined'
		   AND ci.lang = ` + deckLang + `
		   AND (
		         (ci.lemma <> '' AND ci.pos <> ''
		          AND ci.lemma = ` + occAlias + `.lemma AND ci.pos = ` + occAlias + `.pos)
		      OR (ci.lemma = '' AND ci.pos = ''
		          AND ci.norm_surface = lower(` + occAlias + `.surface))
		       )
	)`
}
