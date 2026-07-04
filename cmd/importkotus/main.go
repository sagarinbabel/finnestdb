// cmd/importkotus imports the Kotus Nykysuomen sanalista into the
// finnestdb dictionary. Phase 3 of docs/FINNISH_LEXICAL_PLAN.md: populate
// paradigm_class on FI lemmas with the Kotus inflection class so the
// future Voikko adapter (Phase 4) has the join key it needs.
//
// Source: official Kotus distribution at
// https://kaino.kotus.fi/lataa/nykysuomensanalista2024.txt
// (linked from
// https://kotus.fi/sanakirjat/kielitoimiston-sanakirja/nykysuomen-sana-aineistot/nykysuomen-sanalista/)
// License: CC BY 4.0
//
// Usage:
//
//	go run ./cmd/importkotus -db finnestdb.db \
//	    -file data/kotus/nykysuomensanalista2024.txt
//
// File format (confirmed against the 2024 distribution; PR 3.1's earlier
// XML guess turned out wrong — Kotus ships the modern sanalista as a
// header-prefixed TSV):
//
//	Hakusana    Homonymia    Sanaluokka                       Taivutustiedot
//	aakkonen                 substantiivi                     38
//	aalto                    substantiivi                     1*I
//	aallottaa                verbi                            53*C
//	sininen                  adjektiivi, substantiivi         38
//	kuusi       1            substantiivi                     27
//	kuusi       2            numeraali                        27
//	aaltoallas               substantiivi                     ← empty Taivutustiedot
//
// Field layout:
//
//	Hakusana       — the headword (one entry, possibly with a leading "‑"
//	                 for derivational suffixes; the parser preserves these
//	                 in their original form).
//	Homonymia      — empty or "1".."4" for words that share a surface
//	                 form but differ in meaning. Our schema collapses
//	                 multiple homonyms of the same (lemma, pos) onto one
//	                 row; the writer logs whichever paradigm_class arrived
//	                 last for that key.
//	Sanaluokka     — Finnish word-class label. Comma-separated when one
//	                 headword is multi-class (e.g. "adjektiivi,
//	                 substantiivi" for sininen). Plus-separated forms
//	                 like "adverbi + kieltoverbi" are leaf-classed by the
//	                 first part. Maps to UPOS via wordClassMap below.
//	Taivutustiedot — Inflection class number, optionally followed by
//	                 *<gradation-letter> (e.g. "1", "38", "1*I", "41*A").
//	                 Empty for compound nouns and other entries Kotus
//	                 doesn't taivutus-classify; those still get a
//	                 dictionary row but with paradigm_class=NULL.
//
// Conflict policy (different from cmd/importdict's source-priority upsert):
//
//	For lemmas already in the database (typically from kaikki.org), this
//	importer ONLY updates paradigm_class. It does NOT touch source,
//	source_priority, or gloss. Kotus enriches existing rows with
//	morphology metadata; it does not claim authority over the headword.
//
//	For lemmas not yet in the database (Kotus-only), it inserts a fresh
//	row at source='kotus', priority=10, gloss=NULL, with paradigm_class
//	set when Taivutustiedot is non-empty (NULL otherwise).
//
//	Multi-class entries (e.g. sininen as ADJ+NOUN) write/upgrade once per
//	UPOS — both rows get paradigm_class.
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"finnestdb/internal/store"

	_ "github.com/mattn/go-sqlite3"
)

const (
	kotusSource         = "kotus"
	kotusSourcePriority = 10
	kotusSourceName     = "Kotus / Nykysuomen sanalista 2024"
	kotusSourceLicense  = "CC BY 4.0"
	kotusAttribution    = "Kotus, Kotimaisten kielten keskus / Nykysuomen sanalista"
	kotusSourceURL      = "https://kaino.kotus.fi/lataa/nykysuomensanalista2024.txt"
	kotusSourceVersion  = "2024"
)

// kotusEntry is the parsed shape of a single TSV row.
type kotusEntry struct {
	Headword       string   // Hakusana
	Homonym        string   // Homonymia ("" or "1".."4")
	WordClasses    []string // raw Finnish names, possibly multiple per entry
	InflectionType string   // numeric portion of Taivutustiedot
	Gradation      string   // gradation letter after "*" — "" if absent
}

// ParadigmClass formats Taivutustiedot for the lemmas.paradigm_class
// column. "" if the entry has no inflection class. The compact text form
// matches the on-disk Kotus encoding ("38", "1-I"), with "*" rewritten
// to "-" so the column value is a stable identifier without shell-quoting
// pitfalls if it ever ends up in a script.
func (e kotusEntry) ParadigmClass() string {
	if e.InflectionType == "" {
		return ""
	}
	if e.Gradation == "" {
		return e.InflectionType
	}
	return e.InflectionType + "-" + e.Gradation
}

// wordClassMap maps Kotus's Finnish Sanaluokka labels to UPOS. Coverage
// is based on the unique values seen in the 2024 distribution. When the
// label is one we don't recognise the writer skips inserting a new row
// (existing rows are still upgraded with paradigm_class — that path
// doesn't depend on POS inference).
var wordClassMap = map[string]string{
	"substantiivi":        "NOUN",
	"adjektiivi":          "ADJ",
	"verbi":               "VERB",
	"adverbi":             "ADV",
	"interjektio":         "INTJ",
	"pronomini":           "PRON",
	"numeraali":           "NUM",
	"järjestysluku":       "NUM",
	"konjunktio":          "CCONJ",
	"rinnastuskonjunktio": "CCONJ",
	"alistuskonjunktio":   "SCONJ",
	"postpositio":         "ADP",
	"prepositio":          "ADP",
	"kieltoverbi":         "AUX",
	"partikkeli":          "PART",
}

// mapWordClass picks a UPOS for a single Kotus class label. Returns the
// UPOS and true on success; "X" and false if the label is unknown.
// Plus-separated compounds like "adverbi + kieltoverbi" are leaf-classed
// by the first segment — the second is typically a co-occurrence
// annotation, not the head class.
func mapWordClass(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, " + "); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if upos, ok := wordClassMap[s]; ok {
		return upos, true
	}
	return "X", false
}

// parseTaivutus splits Taivutustiedot into (inflection-type, gradation).
// Handles four patterns seen in the 2024 distribution:
//
//	"38"           → ("38", "")            — single class, no gradation
//	"1*I"          → ("1", "I")            — single class with gradation
//	"73, (77)"     → ("73", "")            — primary 73 + parenthesized alt
//	"72*D, 74*D"   → ("72", "D")           — primary 72*D + alt 74*D
//	"(9)"          → ("9", "")             — sole class wrapped in parens
//	""             → ("", "")              — no class (compound)
//
// 152 rows in the 2024 dump have comma-separated alternatives. We
// take the first entry as the primary (parens stripped) and drop the
// alts. Phase 4's Voikko adapter consumes paradigm_class as a single
// join key; alternatives can become a separate column or a follow-up
// when we have a use case for them.
func parseTaivutus(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if i := strings.Index(s, ","); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	// Strip surrounding parens (e.g. "(9)" → "9", "(77)" → "77").
	s = strings.Trim(s, "()")
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if i := strings.Index(s, "*"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// streamSanalista reads the TSV row-by-row and invokes fn for each
// non-header non-empty data row. fn returning an error stops the stream
// and surfaces the error to the caller — the surrounding transaction
// is what makes that an atomic abort. Memory stays O(1) regardless of
// input size, which matters at 100k+ rows.
//
// Returns the count of rows skipped (empty headword or unparseable
// shape). Header detection is permissive: a row whose first cell starts
// with "Hakusana" is treated as the header even if column count is off,
// since the file's first line is exactly that.
func streamSanalista(r io.Reader, fn func(kotusEntry) error) (int, error) {
	scanner := bufio.NewScanner(r)
	// 256KB max line — well above anything seen in the 2024 dump.
	scanner.Buffer(make([]byte, 64*1024), 256*1024)

	skipped := 0
	for lineNo := 0; scanner.Scan(); lineNo++ {
		line := scanner.Text()
		if lineNo == 0 && strings.HasPrefix(line, "Hakusana") {
			continue // header row
		}
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			skipped++
			continue
		}
		head := strings.TrimSpace(fields[0])
		if head == "" {
			skipped++
			continue
		}
		homonym := strings.TrimSpace(fields[1])
		// Sanaluokka splits on comma into multiple word-class labels.
		var classes []string
		for _, c := range strings.Split(fields[2], ",") {
			if c = strings.TrimSpace(c); c != "" {
				classes = append(classes, c)
			}
		}
		tn, gr := parseTaivutus(fields[3])

		if err := fn(kotusEntry{
			Headword:       head,
			Homonym:        homonym,
			WordClasses:    classes,
			InflectionType: tn,
			Gradation:      gr,
		}); err != nil {
			return skipped, err
		}
	}
	if err := scanner.Err(); err != nil {
		return skipped, fmt.Errorf("scan: %w", err)
	}
	return skipped, nil
}

// importStats summarises a run.
type importStats struct {
	EntriesProcessed       int // total data rows seen in the TSV
	ParadigmsUpgraded      int // existing rows whose paradigm_class was set/changed
	NewLemmasInserted      int // rows inserted at source='kotus'
	HeadwordsWithoutClass  int // entries where every Sanaluokka label was unknown
	EntriesWithoutParadigm int // entries with empty Taivutustiedot (compounds etc.)
}

// kotusWriter holds the prepared statements + accumulating stats used
// by Import's per-entry callback. Factored so streamSanalista can drive
// the write logic on each TSV row as it's parsed.
type kotusWriter struct {
	stats              *importStats
	stmtUpdateExisting *sql.Stmt
	stmtCheckPOS       *sql.Stmt
	stmtInsertNew      *sql.Stmt
}

func (w *kotusWriter) write(e kotusEntry) error {
	// EntriesProcessed counts TSV rows, not row effects. A single Kotus
	// row with multi-class Sanaluokka fans out into multiple writes;
	// deriving the count from row counters would over-count.
	w.stats.EntriesProcessed++

	paradigm := e.ParadigmClass()
	if paradigm == "" {
		w.stats.EntriesWithoutParadigm++
	}

	// Map the current row's Sanaluokka labels to a deduplicated UPOS
	// set. Updates and inserts are scoped to THIS list — any existing
	// row at a POS the current row doesn't claim is left untouched.
	//
	// Why scoped: the TSV often has the same headword on multiple
	// rows with different Sanaluokka (`aika` row 1 NOUN, row 2 ADJ+ADV;
	// `kuurata` row 1 NOUN row 2 VERB; `piilevä` row 1 ADJ row 2 NOUN).
	// An "update every existing POS" approach corrupts adjacent rows —
	// e.g. row 2 of `aika` would clobber the NOUN paradigm with the
	// ADJ row's class, and never insert the actual ADJ/ADV rows.
	uposSet := map[string]bool{}
	var poses []string
	for _, c := range e.WordClasses {
		upos, ok := mapWordClass(c)
		if !ok {
			continue
		}
		if uposSet[upos] {
			continue
		}
		uposSet[upos] = true
		poses = append(poses, upos)
	}
	if len(poses) == 0 {
		// Every Sanaluokka label was unknown; nothing safe to do.
		w.stats.HeadwordsWithoutClass++
		return nil
	}

	for _, pos := range poses {
		// Existence check (Scan into NullString just to avoid an
		// "unused result" lint; the value is unused — only the
		// presence/absence of the row matters here).
		var dummy sql.NullString
		err := w.stmtCheckPOS.QueryRow(e.Headword, pos).Scan(&dummy)
		if err == sql.ErrNoRows {
			res, err := w.stmtInsertNew.Exec(e.Headword, pos, nullableParadigm(paradigm))
			if err != nil {
				return fmt.Errorf("insert %q %s: %w", e.Headword, pos, err)
			}
			if n, _ := res.RowsAffected(); n == 1 {
				w.stats.NewLemmasInserted++
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("check pos %q %s: %w", e.Headword, pos, err)
		}
		// Row exists at this POS. Only UPDATE paradigm_class when the
		// current row HAS one — otherwise we'd nullify a previously
		// imported class (`piilevä` row 2 has empty Taivutustiedot
		// and would clobber row 1's class 10 with NULL).
		if paradigm == "" {
			continue
		}
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

// nullableParadigm returns nil when the paradigm string is empty so the
// SQL driver writes NULL (not the empty string). Compound headwords
// that Kotus doesn't classify get paradigm_class IS NULL.
func nullableParadigm(p string) interface{} {
	if p == "" {
		return nil
	}
	return p
}

// Import runs an end-to-end streaming import: reads the TSV row-by-row
// and writes each entry via the same transaction-scoped statements.
// Memory stays O(1) regardless of input size. The DB must already have
// the post-Phase-1 schema (paradigm_class column on lemmas).
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
	stats.EntriesProcessed += skipped

	if err := tx.Commit(); err != nil {
		return stats, err
	}
	return stats, nil
}

// newKotusWriter prepares the three statements the per-entry write path
// uses, sharing them across the streaming callback. Caller must Close().
func newKotusWriter(tx *sql.Tx, stats *importStats) (*kotusWriter, error) {
	stmtUpdateExisting, err := tx.Prepare(
		`UPDATE lemmas SET paradigm_class = ?
		 WHERE lemma = ? AND pos = ? AND lang = 'FI'`,
	)
	if err != nil {
		return nil, err
	}
	// Existence check scoped by (lemma, pos). Returning paradigm_class
	// is incidental — the callback only cares whether the row exists.
	stmtCheckPOS, err := tx.Prepare(
		`SELECT paradigm_class FROM lemmas WHERE lemma = ? AND pos = ? AND lang = 'FI'`,
	)
	if err != nil {
		stmtUpdateExisting.Close()
		return nil, err
	}
	stmtInsertNew, err := tx.Prepare(
		`INSERT OR IGNORE INTO lemmas (lemma, pos, gloss, lang, source, source_priority, paradigm_class)
		 VALUES (?, ?, NULL, 'FI', 'kotus', 10, ?)`,
	)
	if err != nil {
		stmtUpdateExisting.Close()
		stmtCheckPOS.Close()
		return nil, err
	}
	return &kotusWriter{
		stats:              stats,
		stmtUpdateExisting: stmtUpdateExisting,
		stmtCheckPOS:       stmtCheckPOS,
		stmtInsertNew:      stmtInsertNew,
	}, nil
}

func (w *kotusWriter) Close() error {
	w.stmtUpdateExisting.Close()
	w.stmtCheckPOS.Close()
	w.stmtInsertNew.Close()
	return nil
}

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	filePath := flag.String("file", "data/kotus/nykysuomensanalista2024.txt",
		"Path to Kotus sanalista TSV file (defaults to the tracked artifact)")
	flag.Parse()

	f, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("open %s: %v", *filePath, err)
	}
	defer f.Close()

	db, err := sql.Open("sqlite3", *dbPath+"?_busy_timeout=5000")
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

	log.Printf(
		"Done. processed=%d  paradigms_upgraded=%d  new_lemmas_inserted=%d  no_known_class=%d  no_paradigm=%d",
		stats.EntriesProcessed,
		stats.ParadigmsUpgraded,
		stats.NewLemmasInserted,
		stats.HeadwordsWithoutClass,
		stats.EntriesWithoutParadigm,
	)

	if err := upsertDictMetadata(db, stats.NewLemmasInserted+stats.ParadigmsUpgraded); err != nil {
		log.Printf("warn: dict_metadata: %v", err)
	}
}

// ensureSchema creates the lemmas + forms tables on a fresh DB and runs
// the shared idempotent migrations so this importer can stand alone.
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
// provenance table reflects the source.
func upsertDictMetadata(db *sql.DB, rowCount int) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO dict_metadata
		 (lang, source, source_name, source_url, source_version, license, attribution, changes_note, imported_at, row_count)
		 VALUES ('FI', ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)`,
		kotusSource, kotusSourceName, kotusSourceURL,
		kotusSourceVersion, kotusSourceLicense, kotusAttribution,
		"Mapped Sanaluokka → UPOS, Taivutustiedot → paradigm_class TEXT (with *<gradation> rewritten to -<gradation>); existing rows enriched in place",
		rowCount,
	)
	return err
}
