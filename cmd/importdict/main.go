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
	"time"

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

type importSourceConfig struct {
	Name     string
	Priority int
}

// Source priority convention (ascending). Higher priority overwrites lower on
// overlapping (lemma, pos, lang) or (form, lang) primary keys; lower-priority
// imports do not overwrite higher-priority rows.
//
//	  0  legacy / pre-priority rows (migration backfill default)
//	 10  kaikki / Wiktionary-derived dump (default for `-source-key kaikki`)
//	 20  sanctioned licensed corpora (EKI/Ekilex etc.)
//	100  user-supplied custom glosses (`-custom-glosses`)
//
// Operators can pick any integer between sources; these are the recommended
// canonical values so re-imports compose deterministically.
const customGlossPriority = 100

// Upsert SQL for the lemmas and forms tables. Defined once so all six
// call sites (initial Prepare, batch-restart Prepare, applyCustomGlosses)
// share the same conflict policy: incoming row wins iff its priority is
// at least as high as the existing row's.

const upsertLemmaSQL = `INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(lemma, pos, lang) DO UPDATE SET
		gloss = excluded.gloss,
		source = excluded.source,
		source_priority = excluded.source_priority
	WHERE lemmas.source_priority <= excluded.source_priority`

// upsertFormSQL writes a form row including UD FEATS. The conflict update
// refreshes feats too - re-importing the same source after the FEATS mapper
// learns a new tag (or kaikki upstream flips a tag) propagates the change
// instead of silently keeping the old NULL.
//
// Conflict target matches the post-EnsureMultiLemmaSchema PK
// (form, lang, lemma, pos): two rows that differ only in lemma or pos
// (homonyms like ET "joon" = noun "line" / 1Sg of jooma) are intentionally
// kept as separate rows, not collapsed.
const upsertFormSQL = `INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(form, lang, lemma, pos) DO UPDATE SET
		source = excluded.source,
		source_priority = excluded.source_priority,
		feats = excluded.feats
	WHERE forms.source_priority <= excluded.source_priority`

// upsertTranslationSQL writes one row per (sense, gloss) pair into the
// translations table. PK (lemma, pos, lang, target_lang, sense_idx, source)
// lets multiple sources coexist (kaikki + Ekilex translations for the same
// headword don't overwrite each other). On conflict within the SAME source
// - i.e. re-running an import after upstream gloss text changed - the
// `text` column is refreshed; without this, kaikki dumps with edited
// glosses would silently keep the old text. target_lang is hard-coded to
// 'EN' because both FI and ET dumps from kaikki.org are en.wiktionary
// extractions whose senses[].glosses[] arrays are English. fi.wiktionary's
// Finnish-language definitions will land under target_lang='FI' via a
// future import path; not in scope for this PR.
const upsertTranslationSQL = `INSERT INTO translations
	(lemma, pos, lang, target_lang, text, sense_idx, source)
	VALUES (?, ?, ?, 'EN', ?, ?, ?)
	ON CONFLICT (lemma, pos, lang, target_lang, sense_idx, source) DO UPDATE SET
	  text = excluded.text`

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
	sourceKey := flag.String("source-key", "kaikki", "Stable source key stored on each imported row (kaikki|ekilex|...; non-kaikki keys require -file or -source-url)")
	sourcePriority := flag.Int("source-priority", 10, "Source priority for conflict resolution; higher overwrites lower (kaikki=10, EKI~20, custom=100)")
	filePath := flag.String("file", "", "Path to local .jsonl, .jsonl.gz, or .jsonl.bz2 file (skips download)")
	customGlosses := flag.String("custom-glosses", "", "Path to CSV file of custom gloss overrides (word,pos,lang,gloss)")
	reimport := flag.Bool("reimport", false, "Drop existing entries for this lang before importing")
	sourceName := flag.String("source-name", "", "Human-readable lexical source name for dict_metadata (required for non-kaikki sources)")
	sourceURL := flag.String("source-url", "", "Canonical lexical source URL for dict_metadata")
	sourceVersion := flag.String("source-version", "", "Source version, dump date, or release identifier for dict_metadata")
	sourceLicense := flag.String("source-license", "", "License text or SPDX-like label for dict_metadata (required for non-kaikki sources)")
	sourceAttribution := flag.String("source-attribution", "", "Attribution text to preserve with imported rows (required for non-kaikki sources)")
	changesNote := flag.String("changes-note", "Normalized to FinnEst lemma/form/POS schema", "Change notice for dict_metadata")
	ekilexBaseURL := flag.String("ekilex-base-url", "https://ekilex.ee", "Ekilex API base URL")
	ekilexDatasets := flag.String("ekilex-datasets", "eki", "Comma-delimited Ekilex dataset codes to import")
	ekilexWords := flag.String("ekilex-words", "", "Comma-delimited word list for a small Ekilex smoke import")
	ekilexLimit := flag.Int("ekilex-limit", 0, "Maximum number of Ekilex words to import; 0 means no limit")
	ekilexTimeout := flag.Duration("ekilex-timeout", 45*time.Second, "Ekilex HTTP client timeout (e.g. 45s, 2m)")
	ekilexRetries := flag.Int("ekilex-retries", 0, "Retries per Ekilex HTTP request on network/HTTP/JSON error")
	backfillFeats := flag.Bool("backfill-feats", false, "Stream the kaikki JSONL and only UPDATE forms.feats on rows where feats IS NULL; skips lemma/translation writes")
	flag.Parse()

	langCode := strings.ToLower(*lang)
	if _, ok := kaikkiURL[langCode]; !ok {
		log.Fatalf("Unsupported language %q. Supported: fi, et", *lang)
	}
	dbLang := strings.ToUpper(langCode) // "fi" → "FI", "et" → "ET"
	sourceConfig := importSourceConfig{
		Name:     strings.ToLower(strings.TrimSpace(*sourceKey)),
		Priority: *sourcePriority,
	}
	if sourceConfig.Name == "" {
		log.Fatal("source-key is required")
	}

	if err := resolveProvenance(*filePath, sourceConfig.Name, sourceName, sourceURL, sourceLicense, sourceAttribution); err != nil {
		log.Fatalf("%v", err)
	}

	db, err := sql.Open("sqlite3", *dbPath+"?_busy_timeout=5000")
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
		log.Printf("--reimport: dropping existing %s entries from forms, lemmas, and translations tables...", dbLang)
		if _, err := db.Exec(`DELETE FROM forms WHERE lang = ?`, dbLang); err != nil {
			log.Fatalf("drop forms: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM lemmas WHERE lang = ?`, dbLang); err != nil {
			log.Fatalf("drop lemmas: %v", err)
		}
		// Translations table joined the cleanup in Phase 2 (this PR). Without
		// this, removed or renumbered senses from a refreshed kaikki dump
		// leave stale rows behind that the future read path would surface.
		if _, err := db.Exec(`DELETE FROM translations WHERE lang = ?`, dbLang); err != nil {
			log.Fatalf("drop translations: %v", err)
		}
	}

	count := 0
	if *backfillFeats {
		if sourceConfig.Name == "ekilex" {
			log.Fatalf("-backfill-feats is for kaikki JSONL only; Ekilex morph_codes are written by cmd/importekilexdetails")
		}
		if *filePath == "" {
			log.Fatalf("-backfill-feats requires -file pointing to the local kaikki JSONL")
		}
		f, err := os.Open(*filePath)
		if err != nil {
			log.Fatalf("open file: %v", err)
		}
		defer f.Close()
		reader, err := openJSONLReader(f, *filePath)
		if err != nil {
			log.Fatalf("open reader: %v", err)
		}
		log.Printf("Backfilling FEATS into %s rows from %s ...", dbLang, *filePath)
		updated, scanned, err := backfillFeatsJSONL(db, reader, dbLang)
		if err != nil {
			log.Fatalf("backfill: %v", err)
		}
		fmt.Printf("\nBackfill complete: %d forms updated (of %d scanned) for %s.\n", updated, scanned, dbLang)
		return
	}
	if sourceConfig.Name == "ekilex" {
		if dbLang != "ET" {
			log.Fatalf("Ekilex import currently supports ET only, got %s", dbLang)
		}
		if *sourceName == "kaikki.org" {
			*sourceName = "EKI/Ekilex/Sõnaveeb"
		}
		if *sourceLicense == "Wiktionary-derived; verify per source terms" {
			*sourceLicense = "CC BY 4.0"
		}
		if *sourceAttribution == "kaikki.org dictionary data derived from Wiktionary" {
			*sourceAttribution = "Eesti Keele Instituut; EKI sõnastiku- ja terminibaasisüsteem Ekilex; Sõnaveeb"
		}
		client, err := newEkilexClient(*ekilexBaseURL, os.Getenv("EKILEX_API_KEY"), *ekilexTimeout, *ekilexRetries)
		if err != nil {
			log.Fatalf("ekilex client: %v", err)
		}
		count, err = importEkilex(db, client, dbLang, splitCSV(*ekilexDatasets), splitCSV(*ekilexWords), *ekilexLimit, sourceConfig)
		if err != nil {
			log.Fatalf("ekilex import: %v", err)
		}
		log.Printf("Ekilex import complete: %d words processed for %s", count, dbLang)
	} else {
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

		skipped := 0
		count, skipped, err = importJSONL(db, reader, dbLang, sourceConfig)
		if err != nil {
			log.Fatalf("import: %v", err)
		}
		log.Printf("Import complete: %d entries processed (%d possessive forms skipped) for %s", count, skipped, dbLang)
	}

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
	if metadata.SourceURL == "" && sourceConfig.Name == "ekilex" {
		metadata.SourceURL = *ekilexBaseURL
	} else if metadata.SourceURL == "" {
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

	fmt.Printf("\nDone. Imported %d entries for %s from %s.\n", count, dbLang, sourceConfig.Name)
	fmt.Printf("Run './finnestdb' to start the server.\n")
}

// resolveProvenance applies kaikki defaults when we are importing the built-in
// kaikki dump, and otherwise requires the operator to spell out source name,
// license, and attribution. It also rejects the dangerous combination of a
// non-kaikki -source-key with the kaikki URL fallback (no -file, no
// -source-url), which would silently mislabel kaikki rows under a different
// source identity. It mutates the flag pointers in place.
func resolveProvenance(filePath, sourceKey string, sourceName, sourceURL, sourceLicense, sourceAttribution *string) error {
	usingKaikkiData := filePath == "" && strings.TrimSpace(*sourceURL) == ""
	usingKaikkiKey := strings.ToLower(strings.TrimSpace(sourceKey)) == "kaikki"

	if usingKaikkiData && !usingKaikkiKey {
		return fmt.Errorf("-source-key=%q requires -file or -source-url; otherwise data would be downloaded from kaikki and silently mislabeled as %q", sourceKey, sourceKey)
	}

	if usingKaikkiData && usingKaikkiKey {
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
func importJSONL(db *sql.DB, r io.Reader, dbLang string, source importSourceConfig) (int, int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4MB line buffer

	const batchSize = 10_000
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}

	// Wipe this source's existing translations for this language before
	// importing. Without this, an entry whose senses[] shrunk upstream
	// would leave its old sense_idx rows stranded - the upsert below
	// refreshes rows that still exist but never deletes ones that
	// disappeared. Scoped by (lang, source) so other sources' rows
	// (e.g. ekilex translations once PR 4 ships) are preserved across
	// a kaikki rerun. Lemmas/forms don't need analogous cleanup because
	// their PKs are stable - kaikki always emits the same canonical
	// headword + form rows for an entry, only sense lists drift.
	//
	// Inside the first batch's transaction so a failure before the first
	// commit (e.g. truncated stream, reader error before any entry is
	// processed) rolls the delete back along with any partial inserts.
	// Without this, a stream failure would leave the dictionary with no
	// translations for this source - strictly worse than the pre-import
	// state the import was supposed to refresh.
	if _, err := tx.Exec(`DELETE FROM translations WHERE lang = ? AND source = ?`, dbLang, source.Name); err != nil {
		tx.Rollback()
		return 0, 0, err
	}

	stmtLemma, err := tx.Prepare(upsertLemmaSQL)
	if err != nil {
		tx.Rollback()
		return 0, 0, err
	}
	stmtForm, err := tx.Prepare(upsertFormSQL)
	if err != nil {
		tx.Rollback()
		return 0, 0, err
	}
	stmtTranslation, err := tx.Prepare(upsertTranslationSQL)
	if err != nil {
		tx.Rollback()
		return 0, 0, err
	}

	count := 0
	skipped := 0

	commit := func() error {
		stmtLemma.Close()
		stmtForm.Close()
		stmtTranslation.Close()
		if err := tx.Commit(); err != nil {
			return err
		}
		tx, err = db.Begin()
		if err != nil {
			return err
		}
		stmtLemma, err = tx.Prepare(upsertLemmaSQL)
		if err != nil {
			return err
		}
		stmtForm, err = tx.Prepare(upsertFormSQL)
		if err != nil {
			return err
		}
		stmtTranslation, err = tx.Prepare(upsertTranslationSQL)
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

		// Extract the primary gloss for the lemmas.gloss cache, skipping
		// Wiktionary "form-of" restatements (e.g. "partitive singular of
		// vuosi") in favour of the first real meaning gloss. The full
		// senses[].glosses[] data is also written to the translations
		// table below - lemmas.gloss is the denormalised "primary
		// translation" cache for the existing UI's fast paths.
		flatGlosses := make([][]string, len(entry.Senses))
		for i, s := range entry.Senses {
			flatGlosses[i] = s.Glosses
		}
		gloss := pickPrimaryGloss(flatGlosses)

		// Insert lemma (the canonical headword form).
		if _, err := stmtLemma.Exec(entry.Word, pos, gloss, dbLang, source.Name, source.Priority); err != nil {
			log.Printf("warn: lemma insert %q: %v", entry.Word, err)
		}

		// Insert each gloss as a translation row. sense_idx is a flat
		// counter across (sense, gloss) pairs - preserves order, gives
		// each row a unique sense_idx so the PK doesn't conflict within
		// the same source. We deliberately don't preserve "which glosses
		// belong to the same sense" structure; if that becomes useful
		// later we can add a sense_group_idx column without breaking the
		// existing schema.
		//
		// Wiktionary "form-of" rows ("partitive singular of vuosi",
		// "past active participle of olla", etc.) are skipped: they
		// restate morphology the learner already sees on the card and
		// would otherwise displace real meaning rows under the
		// ORDER BY sense_idx ASC lookup in BatchLookupGlosses. The
		// lemmas.gloss cache above already preferred the meaning gloss;
		// the translations table needs the same discipline.
		senseIdx := 0
		for _, s := range entry.Senses {
			for _, g := range s.Glosses {
				g = strings.TrimSpace(g)
				if g == "" {
					continue
				}
				if isStructuralGloss(g) {
					continue
				}
				if _, err := stmtTranslation.Exec(entry.Word, pos, dbLang, g, senseIdx, source.Name); err != nil {
					log.Printf("warn: translation insert %q sense=%d: %v", entry.Word, senseIdx, err)
				}
				senseIdx++
			}
		}

		// Insert the canonical form → lemma mapping (lemma maps to itself).
		// Lemma form gets Reflex=Yes for known reflexive pronoun headwords;
		// no other FEATS - kaikki doesn't tag the headword itself, and the
		// dictionary form has no inflectional context.
		lemmaFeats := withReflex("", entry.Word, pos)
		stmtForm.Exec(entry.Word, entry.Word, pos, dbLang, source.Name, source.Priority, lemmaFeats)

		// Insert inflected forms, skipping possessive-tagged forms.
		for _, f := range entry.Forms {
			if f.Form == "" || f.Form == "-" || f.Form == entry.Word {
				continue
			}
			if hasPossessiveTag(f.Tags) {
				skipped++
				continue
			}
			feats := withReflex(kaikkiTagsToFeats(f.Tags, pos), entry.Word, pos)
			stmtForm.Exec(f.Form, entry.Word, pos, dbLang, source.Name, source.Priority, feats)
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
	stmtTranslation.Close()

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

// backfillFeatsJSONL streams a kaikki JSONL file and issues an UPDATE for
// each form row whose feats column is NULL, computing FEATS from the
// kaikki Tags via kaikkiTagsToFeats. Rows already populated (e.g. by an
// earlier importdict run that already had the FEATS mapper, or by the
// Ekilex pipeline for ET) are left untouched - the WHERE feats IS NULL
// guard is in the UPDATE statement.
//
// Returns (rowsUpdated, formsScanned, error).
func backfillFeatsJSONL(db *sql.DB, r io.Reader, dbLang string) (int, int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	const batchSize = 10_000
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}

	const updateSQL = `UPDATE forms SET feats = ?
		WHERE form = ? AND lang = ? AND (feats IS NULL OR feats = '')`
	stmt, err := tx.Prepare(updateSQL)
	if err != nil {
		tx.Rollback()
		return 0, 0, err
	}

	updated := 0
	scanned := 0
	commit := func() error {
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return err
		}
		tx, err = db.Begin()
		if err != nil {
			return err
		}
		stmt, err = tx.Prepare(updateSQL)
		return err
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry kaikkiEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Word == "" {
			continue
		}
		pos := normalizePos(entry.POS)

		// Lemma form: only the reflexive marker contributes (no inflection).
		if lf := withReflex("", entry.Word, pos); lf != "" {
			scanned++
			res, err := stmt.Exec(lf, entry.Word, dbLang)
			if err == nil {
				if n, _ := res.RowsAffected(); n > 0 {
					updated++
				}
			}
		}

		for _, f := range entry.Forms {
			if f.Form == "" || f.Form == "-" || f.Form == entry.Word {
				continue
			}
			if hasPossessiveTag(f.Tags) {
				continue
			}
			feats := withReflex(kaikkiTagsToFeats(f.Tags, pos), entry.Word, pos)
			if feats == "" {
				continue
			}
			scanned++
			res, err := stmt.Exec(feats, f.Form, dbLang)
			if err != nil {
				continue
			}
			if n, _ := res.RowsAffected(); n > 0 {
				updated++
			}
		}

		if scanned > 0 && scanned%batchSize == 0 {
			if err := commit(); err != nil {
				return updated, scanned, err
			}
			fmt.Printf("\r  %d forms scanned, %d updated...", scanned, updated)
		}
	}

	stmt.Close()
	if err := scanner.Err(); err != nil {
		tx.Rollback()
		return updated, scanned, err
	}
	if err := tx.Commit(); err != nil {
		return updated, scanned, err
	}
	return updated, scanned, nil
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
	// An empty file (or one with only comments) is not an error - 0 overrides.
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
		// Not a header - process this row as data.
		if len(header) >= 4 {
			pos := normalizePos(header[1])
			db.Exec(upsertLemmaSQL,
				header[0], pos, header[3], dbLang, "custom", customGlossPriority)
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
		if _, err := db.Exec(upsertLemmaSQL,
			word, pos, gloss, dbLang, "custom", customGlossPriority,
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
//
// The forms PRIMARY KEY matches the post-EnsureMultiLemmaSchema shape
// directly so fresh importer-only DBs accept upserts that conflict on
// (form, lang, lemma, pos). EnsureMultiLemmaSchema is also invoked to
// migrate any legacy DB the importer is pointed at after the server has
// not yet started against it.
func ensureSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS lemmas (
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			gloss TEXT,
			lang  TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_priority INTEGER NOT NULL DEFAULT 0,
			paradigm_class TEXT,
			PRIMARY KEY(lemma, pos, lang)
		);
		CREATE TABLE IF NOT EXISTS forms (
			form  TEXT NOT NULL,
			lemma TEXT NOT NULL,
			pos   TEXT NOT NULL,
			lang  TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_priority INTEGER NOT NULL DEFAULT 0,
			feats TEXT,
			PRIMARY KEY (form, lang, lemma, pos)
		);
	`); err != nil {
		return err
	}
	if err := store.EnsureDictMetadataSchema(db); err != nil {
		return err
	}
	if err := store.EnsureDictionarySourceColumns(db); err != nil {
		return err
	}
	if err := store.EnsureLexicalEnrichmentColumns(db); err != nil {
		return err
	}
	if err := store.EnsureLexicalEntryTables(db); err != nil {
		return err
	}
	return store.EnsureMultiLemmaSchema(db)
}
