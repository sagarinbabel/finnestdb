-- Release database invariants for the production candidate SQLite DB.
-- Usage:
--   sqlite3 finnestdb.db < scripts/db-invariants.sql

.headers on
.mode column

SELECT 'integrity_check' AS check_name, integrity_check AS result
FROM pragma_integrity_check;

SELECT 'forms_orphan_lemmas' AS check_name, COUNT(*) AS count
FROM forms f
LEFT JOIN lemmas l ON l.lang = f.lang AND l.lemma = f.lemma AND l.pos = f.pos
WHERE l.lemma IS NULL;

SELECT 'definitions_orphan_lemmas' AS check_name, COUNT(*) AS count
FROM definitions d
LEFT JOIN lemmas l ON l.lang = d.lang AND l.lemma = d.lemma AND l.pos = d.pos
WHERE l.lemma IS NULL;

SELECT 'translations_orphan_lemmas' AS check_name, COUNT(*) AS count
FROM translations t
LEFT JOIN lemmas l ON l.lang = t.lang AND l.lemma = t.lemma AND l.pos = t.pos
WHERE l.lemma IS NULL;

SELECT 'card_state_orphan_cards' AS check_name, COUNT(*) AS count
FROM card_state cs
LEFT JOIN cards c ON c.id = cs.card_id
WHERE c.id IS NULL;

SELECT 'occurrence_orphan_decks' AS check_name, COUNT(*) AS count
FROM occurrence o
LEFT JOIN decks d ON d.id = o.deck_id
WHERE d.id IS NULL;

SELECT 'sentences_orphan_decks' AS check_name, COUNT(*) AS count
FROM sentences s
LEFT JOIN decks d ON d.id = s.deck_id
WHERE d.id IS NULL;

SELECT 'known_ignored_overlap' AS check_name, COUNT(*) AS count
FROM user_known_lemmas k
JOIN user_ignored_lemmas i
  ON i.user_id = k.user_id
 AND i.lang = k.lang
 AND i.lemma = k.lemma
 AND i.pos = k.pos;

SELECT 'parse_feedback_orphan_sessions' AS check_name, COUNT(*) AS count
FROM parse_feedback pf
LEFT JOIN parse_sessions ps ON ps.id = pf.parse_session_id
WHERE ps.id IS NULL;

SELECT 'deck_orphan_parse_sessions' AS check_name, COUNT(*) AS count
FROM decks d
LEFT JOIN parse_sessions ps ON ps.id = d.parse_session_id
WHERE d.parse_session_id IS NOT NULL AND ps.id IS NULL;

SELECT 'source_breakdown' AS check_name, lang, COALESCE(source, '') AS source, source_priority, COUNT(*) AS rows
FROM forms
GROUP BY lang, source, source_priority
ORDER BY lang, source_priority DESC, source;
