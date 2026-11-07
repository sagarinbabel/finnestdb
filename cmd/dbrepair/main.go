// dbrepair reports - and with -fix, repairs - the referential and derived-data
// integrity violations that accumulated while PRAGMA foreign_keys was off and
// while DeleteUserCascade was incomplete.
//
// Why this is a separate command rather than part of NewDB: the repair deletes
// rows. A 5 GB database that every server, CLI tool and cron job opens is not
// somewhere to run silent destructive migrations on startup. The maintainer
// sees the report, then decides.
//
// What it finds:
//
//   - Orphan rows: rows whose foreign key points at a row that no longer
//     exists. These were insertable (and un-cascaded) because the schema's
//     FOREIGN KEY declarations were never enforced - SQLite defaults
//     PRAGMA foreign_keys OFF and the DSN did not turn it on. Enabling the
//     pragma stops NEW orphans; it does not retroactively reject existing
//     ones, which is what this command is for.
//
//   - Known/ignored contradictions: a (user, lang, lemma, pos) present in both
//     user_known_lemmas and user_ignored_lemmas. The two tables encode one
//     tri-state; the known-words importers used to insert without clearing the
//     ignored row.
//
//   - Correction-issue count drift: report_count / distinct_reporter_count
//     that disagree with the linked parse_feedback rows. recountIssue defines
//     these as COUNT over those rows, but deleting a user removed the reports
//     without recounting.
//
//   - Stateless cards: a cards row with no card_state row. Such a card is
//     invisible to the review queue (every queue query INNER JOINs card_state)
//     yet returns an error if answered directly.
//
// What it deliberately does NOT touch:
//
//   - correction_issues rows whose linked parse_feedback rows are all gone.
//     Such a row is not a foreign-key orphan (nothing in it dangles), it just
//     has no evidence left behind it, so it reads report_count = 0 and can
//     never earn the threshold_candidate badge. Deleting or re-triaging those
//     is a product call, not a repair. Tracked as ledger entry DATA-1 in
//     TODO.md "Alpha launch issue ledger"; as of 2026-11-07 all 8 rows in the
//     maintainer DB are in this state, 5 of them QA residue from the
//     2026-07-04 walkthrough (zzqa* / zzlocal* surfaces).
//
// Usage:
//
//	go run ./cmd/dbrepair -db finnestdb.db           # report only
//	go run ./cmd/dbrepair -db finnestdb.db -fix      # report, then repair
//
// Exit code: 0 when clean (or after a successful -fix), 1 when violations
// remain, 2 on error. Safe to run repeatedly.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// orphanCheck is one referential-integrity rule: rows in Table whose Column
// references Parent(ParentKey) but matches nothing there.
type orphanCheck struct {
	Table     string
	Column    string
	Parent    string
	ParentKey string
	// Nullable columns only count as orphans when non-NULL (an admin-attribution
	// column legitimately holds NULL once the admin's account is gone).
	Nullable bool
	// Repair says what -fix does. Deleting a learner-data row is right; for an
	// attribution column, nulling the reference preserves the record that
	// belongs to everyone.
	NullOut bool
}

var orphanChecks = []orphanCheck{
	{Table: "cards", Column: "user_id", Parent: "users", ParentKey: "id"},
	{Table: "card_state", Column: "card_id", Parent: "cards", ParentKey: "id"},
	{Table: "review_log", Column: "user_id", Parent: "users", ParentKey: "id"},
	{Table: "review_log", Column: "card_id", Parent: "cards", ParentKey: "id"},
	{Table: "decks", Column: "user_id", Parent: "users", ParentKey: "id"},
	{Table: "sentences", Column: "deck_id", Parent: "decks", ParentKey: "id"},
	{Table: "occurrence", Column: "deck_id", Parent: "decks", ParentKey: "id"},
	{Table: "occurrence", Column: "sentence_id", Parent: "sentences", ParentKey: "id"},
	{Table: "user_deck_subscriptions", Column: "user_id", Parent: "users", ParentKey: "id"},
	{Table: "user_deck_subscriptions", Column: "deck_id", Parent: "decks", ParentKey: "id"},
	{Table: "user_known_lemmas", Column: "user_id", Parent: "users", ParentKey: "id"},
	{Table: "user_ignored_lemmas", Column: "user_id", Parent: "users", ParentKey: "id"},
	{Table: "user_sessions", Column: "user_id", Parent: "users", ParentKey: "id"},
	{Table: "parse_sessions", Column: "user_id", Parent: "users", ParentKey: "id", Nullable: true},
	{Table: "parse_feedback", Column: "user_id", Parent: "users", ParentKey: "id"},
	{Table: "parse_feedback", Column: "parse_session_id", Parent: "parse_sessions", ParentKey: "id"},
	{Table: "parse_feedback", Column: "reviewed_by_user_id", Parent: "users", ParentKey: "id", Nullable: true, NullOut: true},
	{Table: "parse_feedback", Column: "correction_issue_id", Parent: "correction_issues", ParentKey: "id", Nullable: true, NullOut: true},
	{Table: "correction_issues", Column: "quarantined_by", Parent: "users", ParentKey: "id", Nullable: true, NullOut: true},
	{Table: "correction_issues", Column: "fixed_by", Parent: "users", ParentKey: "id", Nullable: true, NullOut: true},
}

func main() {
	var (
		dbPath = flag.String("db", "finnestdb.db", "path to the SQLite database")
		fix    = flag.Bool("fix", false, "repair the violations instead of only reporting them")
	)
	flag.Parse()

	if _, err := os.Stat(*dbPath); err != nil {
		log.Fatalf("dbrepair: %v", err)
	}

	// foreign_keys stays OFF for this connection on purpose: the repair deletes
	// orphan rows, and with enforcement on, deleting a parent-less child in the
	// wrong order can itself trip a constraint. We are the thing that restores
	// the invariant, so we work beneath it.
	dsn := *dbPath + "?_busy_timeout=10000"
	if *fix {
		dsn = *dbPath + "?_busy_timeout=10000"
	} else {
		dsn = "file:" + *dbPath + "?mode=ro&_busy_timeout=10000"
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		log.Fatalf("dbrepair: open: %v", err)
	}
	defer db.Close()

	problems, err := report(db)
	if err != nil {
		log.Fatalf("dbrepair: %v", err)
	}
	if problems == 0 {
		fmt.Println("\nNo integrity violations found.")
		return
	}
	if !*fix {
		fmt.Printf("\n%d violation group(s) found. Re-run with -fix to repair.\n", problems)
		os.Exit(1)
	}
	if err := repair(db); err != nil {
		log.Fatalf("dbrepair: repair: %v", err)
	}
	fmt.Println("\nRepair applied. Re-checking...")
	remaining, err := report(db)
	if err != nil {
		log.Fatalf("dbrepair: re-check: %v", err)
	}
	if remaining > 0 {
		fmt.Printf("\n%d violation group(s) REMAIN after repair.\n", remaining)
		os.Exit(1)
	}
	fmt.Println("\nClean.")
}

// report prints every violation group and returns how many were non-empty.
func report(db *sql.DB) (int, error) {
	var groups int

	fmt.Println("Orphan rows (foreign key pointing at a row that no longer exists)")
	fmt.Println(strings.Repeat("-", 72))
	anyOrphans := false
	for _, c := range orphanChecks {
		n, err := countOrphans(db, c)
		if err != nil {
			// A table a given database predates is not an error here.
			if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
				continue
			}
			return 0, err
		}
		if n == 0 {
			continue
		}
		anyOrphans = true
		groups++
		action := "delete"
		if c.NullOut {
			action = "set NULL"
		}
		fmt.Printf("  %-26s %-20s -> %-18s %7d rows   (-fix: %s)\n",
			c.Table, c.Column, c.Parent+"."+c.ParentKey, n, action)
	}
	if !anyOrphans {
		fmt.Println("  none")
	}

	fmt.Println("\nKnown/ignored contradictions (a lemma marked both known and ignored)")
	fmt.Println(strings.Repeat("-", 72))
	var contradictions int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM user_known_lemmas k
		  JOIN user_ignored_lemmas i
		    ON i.user_id = k.user_id AND i.lang = k.lang
		   AND i.lemma = k.lemma AND i.pos = k.pos`).Scan(&contradictions); err != nil {
		return 0, err
	}
	if contradictions > 0 {
		groups++
		fmt.Printf("  %d (lemma, pos) in both tables   (-fix: drop the ignored row; known wins,\n", contradictions)
		fmt.Println("      matching how BatchLemmaStates already reports them)")
	} else {
		fmt.Println("  none")
	}

	fmt.Println("\nCorrection-issue count drift (derived counts vs linked parse_feedback rows)")
	fmt.Println(strings.Repeat("-", 72))
	var drift int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM correction_issues ci
		 WHERE ci.report_count <> (
		         SELECT COUNT(*) FROM parse_feedback pf WHERE pf.correction_issue_id = ci.id)
		    OR ci.distinct_reporter_count <> (
		         SELECT COUNT(DISTINCT pf.user_id) FROM parse_feedback pf WHERE pf.correction_issue_id = ci.id)`,
	).Scan(&drift); err != nil {
		return 0, err
	}
	if drift > 0 {
		groups++
		fmt.Printf("  %d issue(s) with stale counts   (-fix: recompute from linked rows)\n", drift)
		fmt.Println("      these drive the threshold_candidate badge in the admin queue")
	} else {
		fmt.Println("  none")
	}

	fmt.Println("\nStateless cards (a card with no card_state row)")
	fmt.Println(strings.Repeat("-", 72))
	var stateless int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM cards c
		  LEFT JOIN card_state cs ON cs.card_id = c.id
		 WHERE cs.card_id IS NULL`).Scan(&stateless); err != nil {
		return 0, err
	}
	if stateless > 0 {
		groups++
		fmt.Printf("  %d card(s) invisible to the review queue   (-fix: seed card_state)\n", stateless)
	} else {
		fmt.Println("  none")
	}

	return groups, nil
}

func countOrphans(db *sql.DB, c orphanCheck) (int, error) {
	q := fmt.Sprintf(
		`SELECT COUNT(*) FROM %s t WHERE %s NOT EXISTS (SELECT 1 FROM %s p WHERE p.%s = t.%s)`,
		c.Table, nullGuard(c), c.Parent, c.ParentKey, c.Column,
	)
	var n int
	err := db.QueryRow(q).Scan(&n)
	return n, err
}

func nullGuard(c orphanCheck) string {
	if c.Nullable {
		return fmt.Sprintf("t.%s IS NOT NULL AND", c.Column)
	}
	return ""
}

func repair(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, c := range orphanChecks {
		var q string
		if c.NullOut {
			q = fmt.Sprintf(
				`UPDATE %s SET %s = NULL WHERE %s NOT EXISTS (SELECT 1 FROM %s p WHERE p.%s = %s.%s)`,
				c.Table, c.Column, nullGuardFor(c, c.Table), c.Parent, c.ParentKey, c.Table, c.Column,
			)
		} else {
			q = fmt.Sprintf(
				`DELETE FROM %s WHERE %s NOT EXISTS (SELECT 1 FROM %s p WHERE p.%s = %s.%s)`,
				c.Table, nullGuardFor(c, c.Table), c.Parent, c.ParentKey, c.Table, c.Column,
			)
		}
		if _, err := tx.Exec(q); err != nil {
			if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
				continue
			}
			return fmt.Errorf("%s.%s: %w", c.Table, c.Column, err)
		}
	}

	// Known wins over ignored, matching BatchLemmaStates' `case known:` order.
	if _, err := tx.Exec(`
		DELETE FROM user_ignored_lemmas
		 WHERE EXISTS (
		     SELECT 1 FROM user_known_lemmas k
		      WHERE k.user_id = user_ignored_lemmas.user_id
		        AND k.lang    = user_ignored_lemmas.lang
		        AND k.lemma   = user_ignored_lemmas.lemma
		        AND k.pos     = user_ignored_lemmas.pos)`); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		UPDATE correction_issues
		   SET report_count = (
		           SELECT COUNT(*) FROM parse_feedback pf
		            WHERE pf.correction_issue_id = correction_issues.id),
		       distinct_reporter_count = (
		           SELECT COUNT(DISTINCT pf.user_id) FROM parse_feedback pf
		            WHERE pf.correction_issue_id = correction_issues.id)`); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO card_state (card_id, fsrs_json, next_due, last_answer_at)
		SELECT c.id, NULL, NULL, NULL
		  FROM cards c
		  LEFT JOIN card_state cs ON cs.card_id = c.id
		 WHERE cs.card_id IS NULL`); err != nil {
		return err
	}

	return tx.Commit()
}

func nullGuardFor(c orphanCheck, table string) string {
	if c.Nullable {
		return fmt.Sprintf("%s.%s IS NOT NULL AND", table, c.Column)
	}
	return ""
}
