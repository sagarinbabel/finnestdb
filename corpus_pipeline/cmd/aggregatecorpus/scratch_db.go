package main

import (
	"database/sql"
	"fmt"
	"strings"
)

// scratch_db.go: SQLite scratch DB for full-scale aggregation. Activated
// via -scratch flag when in-memory won't fit (>16 GB RSS extrapolated).
//
// Schema lives in tmp_* tables in _scratch.db; final TSV writers read
// from these instead of in-memory maps.

const scratchSchema = `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA cache_size=-262144;     -- 256 MB page cache
PRAGMA temp_store=MEMORY;
PRAGMA mmap_size=8589934592;   -- 8 GB

CREATE TABLE IF NOT EXISTS tmp_surfaces (
  surface TEXT PRIMARY KEY,
  count_prose INTEGER NOT NULL DEFAULT 0,
  count_poetry INTEGER NOT NULL DEFAULT 0,
  example_hash TEXT,
  example_poem_id INTEGER,
  resolved INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS tmp_surface_sources (
  surface TEXT NOT NULL,
  source TEXT NOT NULL,
  count INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (surface, source)
);
CREATE TABLE IF NOT EXISTS tmp_surface_docs (
  surface TEXT NOT NULL,
  document_id TEXT NOT NULL,
  is_poetry INTEGER NOT NULL,
  PRIMARY KEY (surface, document_id, is_poetry)
);
CREATE TABLE IF NOT EXISTS tmp_sentences (
  hash TEXT PRIMARY KEY,
  text TEXT NOT NULL,
  final_id INTEGER
);
CREATE TABLE IF NOT EXISTS tmp_sentence_occurrences (
  sentence_hash TEXT NOT NULL,
  source TEXT NOT NULL,
  document_id TEXT NOT NULL,
  sentence_ix INTEGER NOT NULL,
  quality_flags TEXT
);
CREATE INDEX IF NOT EXISTS idx_so_hash ON tmp_sentence_occurrences(sentence_hash);
CREATE TABLE IF NOT EXISTS tmp_documents (
  document_id TEXT PRIMARY KEY,
  source TEXT NOT NULL,
  title TEXT,
  author TEXT,
  raw_path TEXT
);
CREATE TABLE IF NOT EXISTS tmp_wordlist (
  surface TEXT NOT NULL,
  lemma TEXT,
  pos TEXT,
  feats TEXT,
  analysis_sources TEXT,
  analysis_rank INTEGER NOT NULL DEFAULT 1,
  is_parser_choice INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_wl_surface ON tmp_wordlist(surface);

-- tmp_sentence_id maps each unique sentence hash to its deterministic
-- final integer ID. Phase 4 populates this from the in-memory sorted
-- first-occurrence slice, then drops the (hash → text) and (hash → id)
-- maps from RAM. Subsequent writers stream via SQL JOINs against this
-- table, so sentences.tsv / sentence_occurrences.tsv writes are
-- bounded-memory.
CREATE TABLE IF NOT EXISTS tmp_sentence_id (
  hash TEXT PRIMARY KEY,
  final_id INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sid_final ON tmp_sentence_id(final_id);

-- tmp_surface_order holds per-surface sort keys + the resolved example
-- ref pair for each surface. Phase 4 fills it once from the in-memory
-- s.surfaces map (one row per surface), then runs ONE streaming JOIN
-- per output instead of millions of indexed per-surface lookups.
--
-- example_hash is the captured example sentence hash (or empty if the
-- surface has no example). After tmp_sentence_id is populated, an
-- UPDATE-FROM JOIN sets example_final_id to the matching deterministic
-- ID - that's the column the wordlist writer's JOIN reads. Two-step
-- (insert + update) instead of correlated subqueries to keep this
-- O(N) instead of O(N²) on 18M-surface ET.
--
-- example_poem_id is the deterministic poem ID for poetry-only
-- surfaces (or 0). When > 0, takes precedence over example_final_id
-- for the example_ref_type / example_ref_id pair.
CREATE TABLE IF NOT EXISTS tmp_surface_order (
  surface TEXT PRIMARY KEY,
  count_prose INTEGER NOT NULL DEFAULT 0,
  count_poetry INTEGER NOT NULL DEFAULT 0,
  count_total INTEGER NOT NULL DEFAULT 0,
  doc_count_prose INTEGER NOT NULL DEFAULT 0,
  doc_count_poetry INTEGER NOT NULL DEFAULT 0,
  example_hash TEXT,
  example_final_id INTEGER,
  example_poem_id INTEGER NOT NULL DEFAULT 0
);
`

func openScratch(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "file:"+path+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, err
	}
	for _, stmt := range strings.Split(scratchSchema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("exec %q: %w", firstLineLocal(stmt), err)
		}
	}
	return db, nil
}

func firstLineLocal(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
