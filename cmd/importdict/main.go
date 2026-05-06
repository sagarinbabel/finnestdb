// cmd/importdict imports Finnish and Estonian dictionary data from kaikki.org
// (Wiktionary-derived JSONL dumps) into the finnestdb SQLite database.
//
// Usage:
//
//	go run ./cmd/importdict -lang fi -db finnestdb.db
//	go run ./cmd/importdict -lang fi -db finnestdb.db -file ./kaikki.org-fi.jsonl
//	go run ./cmd/importdict -lang fi -db finnestdb.db -file ./kaikki.org-fi.jsonl.gz
//	go run ./cmd/importdict -lang fi -db finnestdb.db -file ./kaikki.org-fi.jsonl.bz2
//	go run ./cmd/importdict -lang fi -db finnestdb.db -custom-glosses ./overrides.csv
//
// The tool is idempotent:
//   - lemmas: INSERT OR REPLACE (re-running updates glosses to latest kaikki.org data)
//   - forms:  INSERT OR IGNORE  (first-imported form wins; use -reimport to start fresh)
//
// Finnish possessive forms (tagged "possessive" in kaikki.org) are skipped.
// They are resolved at enrichment time via suffix stripping (see internal/store/dict.go).
package main

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"finnestdb/internal/store"

	_ "github.com/mattn/go-sqlite3"
)

// kaikkiURL is the current kaikki.org dictionary dump URL for each language code.
var kaikkiURL = map[string]string{
	"fi": "https://kaikki.org/dictionary/Finnish/kaikki.org-dictionary-Finnish.jsonl",
	"et": "https://kaikki.org/dictionary/Estonian/kaikki.org-dictionary-Estonian.jsonl",
}

// Source-attribution defaults applied only when importing the built-in
// kaikki.org dump. Non-kaikki sources must supply these explicitly so EKI/
// Ekilex (or any other licensed corpus) is never silently tagged as Wiktionary.
const (
	kaikkiSourceName    = "kaikki.org"
	kaikkiSourceLicense = "Wiktionary-derived; verify per source terms"
	kaikkiAttribution   = "kaikki.org dictionary data derived from Wiktionary"
)

// posMap normalizes kaikki.org POS strings (lowercase) to UPOS (uppercase).
var posMap = map[string]string{
	"noun":     "NOUN",
	"verb":     "VERB",
	"adj":      "ADJ",
	"adv":      "ADV",
	"pron":     "PRON",
	"det":      "DET",
	"adp":      "ADP",
	"num":      "NUM",
	"intj":     "INTJ",
	"conj":     "CCONJ",
	"particle": "PART",
	"part":     "PART",
	"name":     "PROPN",
	"phrase":   "PHRASE",
	"abbrev":   "X",
	"suffix":   "X",
	"prefix":   "X",
	"affix":    "X",
}

type importMetadata struct {
	Source        string
	SourceName    string
	SourceURL     string
	SourceVersion string
	License       string
	Attribution   string
	ChangesNote   string
}

func normalizePos(raw string) string {
	if mapped, ok := posMap[strings.ToLower(strings.TrimSpace(raw))]; ok {
		return mapped
	}
	return strings.ToUpper(strings.TrimSpace(raw))
}

// openJSONLReader wraps the source in the decompressor implied by its name.
// Plain .jsonl sources are returned unchanged.
func openJSONLReader(r io.Reader, source string) (io.Reader, error) {
	lower := strings.ToLower(source)
	switch {
	case strings.HasSuffix(lower, ".bz2"):
		return bzip2.NewReader(r), nil
	case strings.HasSuffix(lower, ".gz"):
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		return gr, nil
	default:
		return r, nil
	}
}

// kaikkiEntry is the subset of kaikki.org JSONL fields we use.
// The full schema has many more fields; unrecognized fields are discarded.
type kaikkiEntry struct {
	Word     string `json:"word"`
	POS      string `json:"pos"`
	LangCode string `json:"lang_code"`
	Senses   []struct {
		Glosses []string `json:"glosses"`
	} `json:"senses"`
	Forms []struct {
		Form string   `json:"form"`
		Tags []string `json:"tags"`
	} `json:"forms"`
}

// hasPossessiveTag reports whether any tag in the slice is "possessive".
// kaikki.org marks Finnish possessive suffix forms with this tag.
func hasPossessiveTag(tags []string) bool {
	for _, t := range tags {
		if t == "possessive" {
			return true
		}
	}
	return false
}

func main() {
	lang := flag.String("lang", "fi", "Language to import: fi or et")
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	filePath := flag.String("file", "", "Path to local .jsonl, .jsonl.gz, or .jsonl.bz2 file (skips download)")
	customGlosses := flag.String("custom-glosses", "", "Path to CSV file of custom gloss overrides (word,pos,lang,gloss)")
	reimport := flag.Bool("reimport", false, "Drop existing entries for this lang before importing")
	sourceName := flag.String("source-name", "", "Human-readable lexical source name for dict_metadata (required for non-kaikki sources)")
	sourceURL := flag.String("source-url", "", "Canonical lexical source URL for dict_metadata")
	sourceVersion := flag.String("source-version", "", "Source version, dump date, or release identifier for dict_metadata")
	sourceLicense := flag.String("source-license", "", "License text or SPDX-like label for dict_metadata (required for non-kaikki sources)")
	sourceAttribution := flag.String("source-attribution", "", "Attribution text to preserve with imported rows (required for non-kaikki sources)")
	changesNote := flag.String("changes-note", "Normalized to FinEstDB lemma/form/POS schema", "Change notice for dict_metadata")
	flag.Parse()

	langCode := strings.ToLower(*lang)
	if _, ok := kaikkiURL[langCode]; !ok {
		log.Fatalf("Unsupported language %q. Supported: fi, et", *lang)
	}
	dbLang := strings.ToUpper(langCode) // "fi" → "FI", "et" → "ET"

	if err := resolveProvenance(*filePath, sourceName, sourceURL, sourceLicense, sourceAttribution); err != nil {
		log.Fatalf("%v", err)
	}

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("open DB: %v", err)
	}
	defer db.Close()

	// Ensure tables exist (safe to run on an existing DB).
	if err := ensureSchema(db); err != nil {
		log.Fatalf("schema: %v", err)
	}

	// WAL mode for faster batch inserts.
	db.Exec(`PRAGMA journal_mode=WAL`)
	db.Exec(`PRAGMA synchronous=NORMAL`)

	if *reimport {
		log.Printf("--reimport: dropping existing %s entries from forms and lemmas tables...", dbLang)
		if _, err := db.Exec(`DELETE FROM forms WHERE lang = ?`, dbLang); err != nil {
			log.Fatalf("drop forms: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM lemmas WHERE lang = ?`, dbLang); err != nil {
			log.Fatalf("drop lemmas: %v", err)
		}
	}

	// Open the JSONL source.
	var reader io.Reader
	if *filePath != "" {
		f, err := os.Open(*filePath)
		if err != nil {
			log.Fatalf("open file: %v", err)
		}
		defer f.Close()
		reader, err = openJSONLReader(f, *filePath)
		if err != nil {
			log.Fatalf("open reader: %v", err)
		}
		log.Printf("Importing %s dictionary from local file: %s", dbLang, *filePath)
	} else {
		url := kaikkiURL[langCode]
		log.Printf("Downloading %s dictionary from %s ...", dbLang, url)
		resp, err := http.Get(url)
		if err != nil {
			log.Fatalf("download: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Fatalf("download HTTP %d from %s", resp.StatusCode, url)
		}
		reader, err = openJSONLReader(resp.Body, url)
		if err != nil {
			log.Fatalf("open reader: %v", err)
		}
	}

	count, skipped, err := importJSONL(db, reader, dbLang)
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	log.Printf("Import complete: %d entries processed (%d possessive forms skipped) for %s", count, skipped, dbLang)

	// Apply custom gloss overrides if provided.
	if *customGlosses != "" {
		n, err := applyCustomGlosses(db, *customGlosses, dbLang)
		if err != nil {
			log.Fatalf("custom glosses: %v", err)
		}
		log.Printf("Applied %d custom gloss overrides from %s", n, *customGlosses)
	}

	metadata := importMetadata{
		SourceName:    strings.TrimSpace(*sourceName),
		SourceURL:     strings.TrimSpace(*sourceURL),
		SourceVersion: strings.TrimSpace(*sourceVersion),
		License:       strings.TrimSpace(*sourceLicense),
		Attribution:   strings.TrimSpace(*sourceAttribution),
		ChangesNote:   strings.TrimSpace(*changesNote),
	}
	if metadata.SourceURL == "" {
		metadata.SourceURL = kaikkiURL[langCode]
	}
	metadata.Source = metadata.SourceURL
	if *filePath != "" {
		metadata.Source = *filePath
		if strings.TrimSpace(*sourceURL) == "" {
			metadata.SourceURL = *filePath
		}
	}
	if err := recordImportMetadata(db, dbLang, metadata, count); err != nil {
		log.Printf("warn: could not update dict_metadata: %v", err)
	}

	fmt.Printf("\nDone. Imported %d entries for %s.\n", count, dbLang)
	fmt.Printf("Run './finnestdb' to start the server.\n")
}

// resolveProvenance applies kaikki defaults when we are importing the built-in
// kaikki dump, and otherwise requires the operator to spell out source name,
// license, and attribution. It mutates the flag pointers in place.
func resolveProvenance(filePath string, sourceName, sourceURL, sourceLicense, sourceAttribution *string) error {
	usingKaikki := filePath == "" && strings.TrimSpace(*sourceURL) == ""
	if usingKaikki {
		if strings.TrimSpace(*sourceName) == "" {
			*sourceName = kaikkiSourceName
		}
		if strings.TrimSpace(*sourceLicense) == "" {
			*sourceLicense = kaikkiSourceLicense
		}
		if strings.TrimSpace(*sourceAttribution) == "" {
			*sourceAttribution = kaikkiAttribution
		}
		return nil
	}
	var missing []string
	if strings.TrimSpace(*sourceName) == "" {
		missing = append(missing, "-source-name")
	}
	if strings.TrimSpace(*sourceLicense) == "" {
		missing = append(missing, "-source-license")
	}
	if strings.TrimSpace(*sourceAttribution) == "" {
		missing = append(missing, "-source-attribution")
	}
	if len(missing) > 0 {
		return fmt.Errorf("non-kaikki imports require explicit provenance: %s", strings.Join(missing, ", "))
	}
	return nil
}

func recordImportMetadata(db *sql.DB, dbLang string, metadata importMetadata, rowCount int) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO dict_metadata (
			lang, source, source_name, source_url, source_version, license,
			attribution, changes_note, imported_at, row_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)`,
		dbLang,
		metadata.Source,
		metadata.SourceName,
		metadata.SourceURL,
		metadata.SourceVersion,
		metadata.License,
		metadata.Attribution,
		metadata.ChangesNote,
		rowCount,
	)
	return err
}

// importJSONL streams the JSONL reader and inserts lemmas + forms into the DB.
// Returns (entriesProcessed, possessiveFormsSkipped, error).
func importJSONL(db *sql.DB, r io.Reader, dbLang string) (int, int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4MB line buffer

	const batchSize = 10_000
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}

	stmtLemma, err := tx.Prepare(
		`INSERT OR REPLACE INTO lemmas (lemma, pos, gloss, lang) VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		tx.Rollback()
		return 0, 0, err
	}
	stmtForm, err := tx.Prepare(
		`INSERT OR IGNORE INTO forms (form, lemma, pos, lang) VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		tx.Rollback()
		return 0, 0, err
	}

	count := 0
	skipped := 0

	commit := func() error {
		stmtLemma.Close()
		stmtForm.Close()
		if err := tx.Commit(); err != nil {
			return err
		}
		tx, err = db.Begin()
		if err != nil {
			return err
		}
		stmtLemma, err = tx.Prepare(
			`INSERT OR REPLACE INTO lemmas (lemma, pos, gloss, lang) VALUES (?, ?, ?, ?)`,
		)
		if err != nil {
			return err
		}
		stmtForm, err = tx.Prepare(
			`INSERT OR IGNORE INTO forms (form, lemma, pos, lang) VALUES (?, ?, ?, ?)`,
		)
		return err
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry kaikkiEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Malformed line: skip and continue (logged below via skipped count).
			continue
		}

		if entry.Word == "" {
			continue
		}

		pos := normalizePos(entry.POS)

		// Extract first gloss from first sense.
		gloss := ""
		if len(entry.Senses) > 0 && len(entry.Senses[0].Glosses) > 0 {
			gloss = entry.Senses[0].Glosses[0]
		}

		// Insert lemma (the canonical headword form).
		if _, err := stmtLemma.Exec(entry.Word, pos, gloss, dbLang); err != nil {
			log.Printf("warn: lemma insert %q: %v", entry.Word, err)
		}

		// Insert the canonical form → lemma mapping (lemma maps to itself).
		stmtForm.Exec(entry.Word, entry.Word, pos, dbLang)

		// Insert inflected forms, skipping possessive-tagged forms.
		for _, f := range entry.Forms {
			if f.Form == "" || f.Form == "-" || f.Form == entry.Word {
				continue
			}
			if hasPossessiveTag(f.Tags) {
				skipped++
				continue
			}
			stmtForm.Exec(f.Form, entry.Word, pos, dbLang)
		}

		count++
		if count%batchSize == 0 {
			if err := commit(); err != nil {
				return count, skipped, err
			}
			fmt.Printf("\r  %d entries processed...", count)
		}
	}

	stmtLemma.Close()
	stmtForm.Close()

	// Roll back the pending transaction if the stream ended with an error
	// (IO error, truncated download, line exceeding 4MB buffer, etc.).
	// Without this check the last partial batch would be committed even
	// though the CLI exits non-zero, leaving the dictionary silently incomplete.
	if err := scanner.Err(); err != nil {
		tx.Rollback()
		return count, skipped, err
	}

	if err := tx.Commit(); err != nil {
		return count, skipped, err
	}
	fmt.Println() // newline after progress indicator
	return count, skipped, nil
}

// applyCustomGlosses reads a CSV file (columns: word,pos,lang,gloss) and
// applies INSERT OR REPLACE into the lemmas table, overriding any existing
// gloss from kaikki.org. The lang column in the CSV is ignored; dbLang is used.
func applyCustomGlosses(db *sql.DB, filePath, dbLang string) (int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comment = '#'
	r.TrimLeadingSpace = true

	// Skip header row if present.
	// An empty file (or one with only comments) is not an error — 0 overrides.
	header, err := r.Read()
	if err == io.EOF {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	// If the first field doesn't look like a word (it's "word" or "lemma"), skip it.
	if strings.EqualFold(header[0], "word") || strings.EqualFold(header[0], "lemma") {
		// header row consumed; proceed to data rows
	} else {
		// Not a header — process this row as data.
		if len(header) >= 4 {
			pos := normalizePos(header[1])
			db.Exec(`INSERT OR REPLACE INTO lemmas (lemma, pos, gloss, lang) VALUES (?, ?, ?, ?)`,
				header[0], pos, header[3], dbLang)
		}
	}

	count := 0
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}
		if len(row) < 4 {
			continue
		}
		word, pos, gloss := row[0], normalizePos(row[1]), row[3]
		if _, err := db.Exec(
			`INSERT OR REPLACE INTO lemmas (lemma, pos, gloss, lang) VALUES (?, ?, ?, ?)`,
			word, pos, gloss, dbLang,
		); err != nil {
			log.Printf("warn: custom gloss insert %q: %v", word, err)
			continue
		}
		count++
	}
	return count, nil
}

// ensureSchema creates the forms, lemmas, and dict_metadata tables if they
// don't exist. This allows the importer to run against a fresh database file
// without needing to start the main server first. The dict_metadata schema is
// delegated to internal/store so both paths stay in sync.
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
			PRIMARY KEY (form, lang)
		);
	`); err != nil {
		return err
	}
	return store.EnsureDictMetadataSchema(db)
}
