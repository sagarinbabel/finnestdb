#!/usr/bin/env bash
# Release DB invariants gate.
#
# Prints the full human-readable report from scripts/db-invariants.sql, then
# re-evaluates the same checks and exits nonzero if any invariant is violated,
# so release automation cannot read a dirty database as a green run
# (2026-07-04 walkthrough audit: `make db-invariants` exited 0 with 234 orphan
# deck rows).
#
# The gate query below must stay in sync with the checks in
# scripts/db-invariants.sql (everything except the informational
# source_breakdown section).
#
# Usage: scripts/db-invariants.sh [path/to/finnestdb.db]
set -euo pipefail

DB="${1:-finnestdb.db}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

sqlite3 "$DB" < "$SCRIPT_DIR/db-invariants.sql"

violations="$(sqlite3 -noheader -list "$DB" <<'SQL'
SELECT 'integrity_check: ' || integrity_check
FROM pragma_integrity_check
WHERE integrity_check <> 'ok';

SELECT check_name || ': ' || count
FROM (
  SELECT 'forms_orphan_lemmas' AS check_name, COUNT(*) AS count
  FROM forms f
  LEFT JOIN lemmas l ON l.lang = f.lang AND l.lemma = f.lemma AND l.pos = f.pos
  WHERE l.lemma IS NULL
  UNION ALL
  SELECT 'definitions_orphan_lemmas', COUNT(*)
  FROM definitions d
  LEFT JOIN lemmas l ON l.lang = d.lang AND l.lemma = d.lemma AND l.pos = d.pos
  WHERE l.lemma IS NULL
  UNION ALL
  SELECT 'translations_orphan_lemmas', COUNT(*)
  FROM translations t
  LEFT JOIN lemmas l ON l.lang = t.lang AND l.lemma = t.lemma AND l.pos = t.pos
  WHERE l.lemma IS NULL
  UNION ALL
  SELECT 'card_state_orphan_cards', COUNT(*)
  FROM card_state cs
  LEFT JOIN cards c ON c.id = cs.card_id
  WHERE c.id IS NULL
  UNION ALL
  SELECT 'occurrence_orphan_decks', COUNT(*)
  FROM occurrence o
  LEFT JOIN decks d ON d.id = o.deck_id
  WHERE d.id IS NULL
  UNION ALL
  SELECT 'sentences_orphan_decks', COUNT(*)
  FROM sentences s
  LEFT JOIN decks d ON d.id = s.deck_id
  WHERE d.id IS NULL
  UNION ALL
  SELECT 'known_ignored_overlap', COUNT(*)
  FROM user_known_lemmas k
  JOIN user_ignored_lemmas i
    ON i.user_id = k.user_id
   AND i.lang = k.lang
   AND i.lemma = k.lemma
   AND i.pos = k.pos
  UNION ALL
  SELECT 'parse_feedback_orphan_sessions', COUNT(*)
  FROM parse_feedback pf
  LEFT JOIN parse_sessions ps ON ps.id = pf.parse_session_id
  WHERE ps.id IS NULL
  UNION ALL
  SELECT 'deck_orphan_parse_sessions', COUNT(*)
  FROM decks d
  LEFT JOIN parse_sessions ps ON ps.id = d.parse_session_id
  WHERE d.parse_session_id IS NOT NULL AND ps.id IS NULL
)
WHERE count > 0;
SQL
)"

echo ""
if [ -n "$violations" ]; then
  echo "FAIL: database invariant violations:" >&2
  echo "$violations" >&2
  exit 1
fi
echo "OK: all release invariants hold"
