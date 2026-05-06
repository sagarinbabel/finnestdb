// cmd/importkotus imports the Kotus Nykysuomen sanalista XML into the
// finnestdb dictionary. Phase 3 of docs/FINNISH_LEXICAL_PLAN.md: populate
// the paradigm_class column on FI lemmas with the Kotus inflection class
// (1–78) so the future Voikko adapter (Phase 4) has the join key it needs.
//
// Source: official Kotus distribution at https://kaino.kotus.fi/sanat/nykysuomi/
// License: CC BY 4.0
//
// Usage:
//
//	go run ./cmd/importkotus -db finnestdb.db -file kotus-sanalista_v1.xml
//
// Conflict policy (different from cmd/importdict's):
//
//	For lemmas already in the database (typically from kaikki.org), this
//	importer ONLY updates paradigm_class. It does NOT touch source,
//	source_priority, gloss, or any other column. Kotus enriches existing
//	rows with morphology metadata; it does not claim authority over the
//	headword itself.
//
//	For lemmas not yet in the database (Kotus-only), it inserts a fresh
//	row at source='kotus', priority=10, gloss=NULL, paradigm_class set.
//
// POS inference (assumed schema; refine in Phase 3.2 against real data):
//
//	Type numbers 1–49: NOUN (default for nominals; some are adjectives,
//	  e.g. types 38 sininen, 41 vieras — heuristic not exact)
//	Type numbers 50–51: NUM (cardinal numerals like kahdeksan)
//	Type numbers 52–78: VERB
//	Other: X
//
// When a Kotus headword already exists in lemmas under a specific POS
// (e.g. kaikki imported "sininen" as ADJ), this importer queries existing
// rows first and upgrades paradigm_class for each existing (lemma, pos).
// Only Kotus-only headwords use the default POS inference, so the ADJ
// vs NOUN ambiguity for nominals only matters for entries kaikki missed.
//
// XML schema (kotus-sanalista_v1):
//
//	<kotus-sanalista version="1">
//	  <st>
//	    <s>aakkonen</s>
//	    <t><tn>32</tn></t>
//	  </st>
//	  <st>
//	    <s>aalto</s>
//	    <t><tn>1</tn><av>F</av></t>
//	  </st>
//	</kotus-sanalista>
//
// `tn` is the taivutustyyppi (inflection class number).
// `av` is the asteVaihtelu (consonant gradation pattern, letter A–N).
// paradigm_class is stored as either "<tn>" or "<tn>-<av>" (e.g. "32" or "1-F").
package main

import (
	"database/sql"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"finnestdb/internal/store"

	_ "github.com/mattn/go-sqlite3"
)

const (
	kotusSource         = "kotus"
	kotusSourcePriority = 10
	kotusSourceName     = "Kotus / Nykysuomen sanalista"
	kotusSourceLicense  = "CC BY 4.0"
	kotusAttribution    = "Kotus, Kotimaisten kielten keskus / Nykysuomen sanalista"
	kotusSourceURL      = "https://kaino.kotus.fi/sanat/nykysuomi/"
)

// kotusEntry is the parsed shape of a single <st> element.
type kotusEntry struct {
	Headword       string
	InflectionType string // "tn" — empty if missing
	Gradation      string // "av" — empty if absent
}

// ParadigmClass formats the entry's class as a stable TEXT key for the
// lemmas.paradigm_class column. Keeping `tn-av` joined in one column
// matches the schema and makes the Voikko adapter's join trivial.
func (e kotusEntry) ParadigmClass() string {
	if e.InflectionType == "" {
		return ""
	}
	if e.Gradation == "" {
		return e.InflectionType
	}
	return e.InflectionType + "-" + e.Gradation
}

// inferPOSFromInflectionType maps a Kotus inflection class number to a
// default UPOS. Used only when a Kotus-only headword needs a fresh row;
// existing kaikki rows always preserve their own POS.
//
// The 1–49 range is nominals (nouns + adjectives) — we default to NOUN
// because most type-classified words in the language are nouns, and
// Phase 4's Voikko generator only cares about the class number anyway.
// 52–78 is verbs. Type 51 is for words like kahdeksan (cardinal numerals)
// and is grouped with NUM here.
func inferPOSFromInflectionType(tn string) string {
	n, err := strconv.Atoi(tn)
	if err != nil {
		return "X"
	}
	switch {
	case n >= 1 && n <= 49:
		return "NOUN"
	case n == 50, n == 51:
		return "NUM"
	case n >= 52 && n <= 78:
		return "VERB"
	default:
		return "X"
	}
}

// streamSanalista decodes the XML token-by-token and invokes fn for each
// <st> entry as it's parsed — never materializing the full list. Used by
// Import to keep memory O(1) on a 95k-headword distribution. fn returning
// an error stops the stream and surfaces the error to the caller (the
// surrounding transaction is what makes this an atomic abort).
//
// Returns the count of <st> elements skipped because they had no <s>
// headword (defensive — the real distribution shouldn't have these).
func streamSanalista(r io.Reader, fn func(kotusEntry) error) (int, error) {
	dec := xml.NewDecoder(r)

	type tElem struct {
		TN string `xml:"tn"`
		AV string `xml:"av"`
	}
	type stElem struct {
		S string `xml:"s"`
		T tElem  `xml:"t"`
	}

	skipped := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return skipped, fmt.Errorf("xml decode: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "st" {
			continue
		}
		var st stElem
		if err := dec.DecodeElement(&st, &se); err != nil {
			return skipped, fmt.Errorf("decode <st>: %w", err)
		}
		head := strings.TrimSpace(st.S)
		if head == "" {
			skipped++
			continue
		}
		if err := fn(kotusEntry{
			Headword:       head,
			InflectionType: strings.TrimSpace(st.T.TN),
			Gradation:      strings.TrimSpace(st.T.AV),
		}); err != nil {
			return skipped, err
		}
	}
	return skipped, nil
}

// ParseSanalista is a slice-returning convenience wrapper around
// streamSanalista. Tests use this; the production Import path uses
// streamSanalista directly so it doesn't accumulate the full list.
func ParseSanalista(r io.Reader) ([]kotusEntry, int, error) {
	var out []kotusEntry
	skipped, err := streamSanalista(r, func(e kotusEntry) error {
		out = append(out, e)
		return nil
	})
	return out, skipped, err
}

// importStats summarises a run.
type importStats struct {
	EntriesProcessed  int
	ParadigmsUpgraded int // existing rows whose paradigm_class was set/changed
	NewLemmasInserted int // rows inserted at source='kotus'
	HeadwordsWithNoTN int // entries we skipped because tn was empty
}

// kotusWriter holds the prepared statements + accumulating stats used
// by Import's per-entry callback. Factored out so streamSanalista can
// invoke the write logic on each <st> as it's decoded — no intermediate
// slice, O(1) memory regardless of input size.
type kotusWriter struct {
	stats              *importStats
	stmtUpdateExisting *sql.Stmt
	stmtListPOS        *sql.Stmt
	stmtInsertNew      *sql.Stmt
}

func (w *kotusWriter) write(e kotusEntry) error {
	// EntriesProcessed counts XML <st> elements, not rows touched. A
	// single entry can fan out into multiple paradigm-class updates
	// (e.g. sininen at both ADJ and NOUN), so summing the row counters
	// would over-count entries. Increment once per entry, before any
	// row-level work decides which counters fire.
	w.stats.EntriesProcessed++

	paradigm := e.ParadigmClass()
	if paradigm == "" {
		w.stats.HeadwordsWithNoTN++
		return nil
	}

	// Find existing rows for this headword.
	var existingPOS []string
	rows, err := w.stmtListPOS.Query(e.Headword)
	if err != nil {
		return fmt.Errorf("list pos %q: %w", e.Headword, err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		existingPOS = append(existingPOS, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("list pos %q: cursor: %w", e.Headword, err)
	}
	rows.Close()

	if len(existingPOS) > 0 {
		for _, pos := range existingPOS {
			res, err := w.stmtUpdateExisting.Exec(paradigm, e.Headword, pos)
			if err != nil {
				return fmt.Errorf("update %q %s: %w", e.Headword, pos, err)
			}
			if n, _ := res.RowsAffected(); n == 1 {
				w.stats.ParadigmsUpgraded++
			}
		}
		return nil
	}

	pos := inferPOSFromInflectionType(e.InflectionType)
	res, err := w.stmtInsertNew.Exec(e.Headword, pos, paradigm)
	if err != nil {
		return fmt.Errorf("insert %q %s: %w", e.Headword, pos, err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		w.stats.NewLemmasInserted++
	}
	return nil
}

// Import runs an end-to-end streaming import: decodes the XML
// token-by-token and writes each entry via the same transaction-scoped
// prepared statements. Memory stays O(1) regardless of input size.
// The DB must already have the post-Phase-1 schema (paradigm_class
// column on lemmas) — the caller is expected to have run
// store.EnsureLexicalEnrichmentColumns.
func Import(db *sql.DB, r io.Reader) (importStats, error) {
	stats := importStats{}

	tx, err := db.Begin()
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	w, err := newKotusWriter(tx, &stats)
	if err != nil {
		return stats, err
	}
	defer w.Close()

	skipped, err := streamSanalista(r, w.write)
	if err != nil {
		return stats, err
	}
	// w.write incremented EntriesProcessed for each <st> the stream
	// dispatched to the callback. Add the empty-headword <st>s that
	// streamSanalista skipped before they reached the callback so the
	// total matches "<st> elements seen in XML."
	stats.EntriesProcessed += skipped

	if err := tx.Commit(); err != nil {
		return stats, err
	}
	return stats, nil
}

// newKotusWriter prepares the three statements the per-entry write
// path uses, sharing them across the streaming callback. Caller must
// Close() the writer (defers the prepared-statement closes).
func newKotusWriter(tx *sql.Tx, stats *importStats) (*kotusWriter, error) {
	// Update-only statement: leaves source / source_priority / gloss
	// alone. Used when the lemma already exists (typically from kaikki).
	stmtUpdateExisting, err := tx.Prepare(
		`UPDATE lemmas SET paradigm_class = ?
		 WHERE lemma = ? AND pos = ? AND lang = 'FI'`,
	)
	if err != nil {
		return nil, err
	}
	stmtListPOS, err := tx.Prepare(
		`SELECT pos FROM lemmas WHERE lemma = ? AND lang = 'FI'`,
	)
	if err != nil {
		stmtUpdateExisting.Close()
		return nil, err
	}
	// Insert-only statement for Kotus-exclusive headwords. INSERT OR
	// IGNORE rather than upsert because the existing-rows branch already
	// handled the conflict case — if we reach this and a row already
	// exists, the listPOS lookup raced with another writer or the input
	// has duplicates; either way IGNORE preserves the prior row.
	stmtInsertNew, err := tx.Prepare(
		`INSERT OR IGNORE INTO lemmas (lemma, pos, gloss, lang, source, source_priority, paradigm_class)
		 VALUES (?, ?, NULL, 'FI', 'kotus', 10, ?)`,
	)
	if err != nil {
		stmtUpdateExisting.Close()
		stmtListPOS.Close()
		return nil, err
	}
	return &kotusWriter{
		stats:              stats,
		stmtUpdateExisting: stmtUpdateExisting,
		stmtListPOS:        stmtListPOS,
		stmtInsertNew:      stmtInsertNew,
	}, nil
}

func (w *kotusWriter) Close() error {
	w.stmtUpdateExisting.Close()
	w.stmtListPOS.Close()
	w.stmtInsertNew.Close()
	return nil
}

// writeKotusEntries is the slice-based write path kept for tests; it
// drives the same kotusWriter as Import but reads from a pre-built
// slice instead of a stream. Production callers should use Import.
func writeKotusEntries(db *sql.DB, entries []kotusEntry) (importStats, error) {
	stats := importStats{}

	tx, err := db.Begin()
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	w, err := newKotusWriter(tx, &stats)
	if err != nil {
		return stats, err
	}
	defer w.Close()

	for _, e := range entries {
		if err := w.write(e); err != nil {
			return stats, err
		}
	}

	return stats, tx.Commit()
}

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	filePath := flag.String("file", "", "Path to Kotus sanalista XML file (required)")
	flag.Parse()

	if *filePath == "" {
		log.Fatal("-file is required (path to Kotus sanalista XML)")
	}

	f, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("open %s: %v", *filePath, err)
	}
	defer f.Close()

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := ensureSchema(db); err != nil {
		log.Fatalf("schema: %v", err)
	}

	log.Printf("Importing Kotus sanalista from %s into %s ...", *filePath, *dbPath)
	stats, err := Import(db, f)
	if err != nil {
		log.Fatalf("import: %v", err)
	}

	log.Printf("Done. processed=%d  paradigms_upgraded=%d  new_lemmas_inserted=%d  skipped_no_tn=%d",
		stats.EntriesProcessed, stats.ParadigmsUpgraded, stats.NewLemmasInserted, stats.HeadwordsWithNoTN)

	if err := upsertDictMetadata(db, stats.NewLemmasInserted+stats.ParadigmsUpgraded); err != nil {
		log.Printf("warn: dict_metadata: %v", err)
	}
}

// ensureSchema creates the lemmas + forms tables on a fresh DB and runs
// the shared idempotent migrations so this importer can stand alone,
// the way cmd/importekilexdetails does. Forms is created (even though
// this importer never writes to it) because EnsureDictionarySourceColumns
// alters both lemmas and forms. Order matters: CREATE first so the
// ALTER TABLE migrations have something to alter.
func ensureSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS lemmas (
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			gloss TEXT,
			lang  TEXT NOT NULL,
			PRIMARY KEY(lemma, pos, lang)
		);
		CREATE TABLE IF NOT EXISTS forms (
			form  TEXT NOT NULL,
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			lang  TEXT NOT NULL,
			PRIMARY KEY (form, lang, lemma, pos)
		);
	`); err != nil {
		return fmt.Errorf("create base tables: %w", err)
	}
	if err := store.EnsureDictionarySourceColumns(db); err != nil {
		return fmt.Errorf("source-priority columns: %w", err)
	}
	if err := store.EnsureLexicalEnrichmentColumns(db); err != nil {
		return fmt.Errorf("paradigm_class column: %w", err)
	}
	if err := store.EnsureDictMetadataSchema(db); err != nil {
		return fmt.Errorf("dict_metadata schema: %w", err)
	}
	return nil
}

// upsertDictMetadata records the Kotus import in dict_metadata so the
// provenance table reflects this PR's data. License + attribution come
// from the constants above; row_count is touched-rows for this run.
func upsertDictMetadata(db *sql.DB, rowCount int) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO dict_metadata
		 (lang, source, source_name, source_url, source_version, license, attribution, changes_note, imported_at, row_count)
		 VALUES ('FI', ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)`,
		kotusSource, kotusSourceName, kotusSourceURL,
		"", kotusSourceLicense, kotusAttribution,
		"Mapped <tn> + <av> into paradigm_class TEXT; default POS by class range; existing rows enriched in place",
		rowCount,
	)
	return err
}
