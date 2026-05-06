// cmd/importekilexdetails imports the reduced Ekilex data drop produced by
// cmd/reduceekilex into the finnestdb SQLite database.
//
// Inputs (relative to -data):
//
//	definitions/<letter>.jsonl  — per-word lemma + morphology + meanings
//	forms/<letter>.tsv          — one row per (lemma, form, morph_code)
//
// Outputs land in the lemmas and forms tables. Every (lemma, pos) pair from
// every Ekilex meaning becomes a lemmas row; every (lemma, form, pos) tuple
// becomes a forms row. The new multi-lemma forms PK (form, lang, lemma, pos)
// allows ambiguous surface forms to map to multiple homonyms — see
// internal/store EnsureMultiLemmaSchema.
//
// Glosses come from the union of EN translations across a lemma's meanings,
// deduplicated case-insensitively and joined with "; ". Lemmas with no EN
// translations don't overwrite an existing kaikki gloss (empty-gloss guard).
//
// Usage:
//
//	go run ./cmd/importekilexdetails -db finnestdb.db
//	go run ./cmd/importekilexdetails -db finnestdb.db -data ./data/ekilex
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// posMap converts an Ekilex meaning-level POS code to UPOS. The table is
// documented in ARCHITECTURE.md (Dictionary section).
var posMap = map[string]string{
	"s":        "NOUN",
	"v":        "VERB",
	"vrm":      "VERB",
	"adj":      "ADJ",
	"adjg":     "ADJ",
	"adjid":    "ADJ",
	"adv":      "ADV",
	"prop":     "PROPN",
	"propgen":  "PROPN",
	"pron":     "PRON",
	"num":      "NUM",
	"postp":    "ADP",
	"prep":     "ADP",
	"konj":     "CCONJ",
	"interj":   "INTJ",
}

// wordClassFallback applies when a meaning has no `pos` field. The Ekilex
// `word_class` field at the entry level is one of "noomen", "verb",
// "muutumatu", or empty — coarser than meaning-level codes.
var wordClassFallback = map[string]string{
	"noomen":    "NOUN",
	"verb":      "VERB",
	"muutumatu": "X",
}

// definitionEntry mirrors the JSONL shape produced by cmd/reduceekilex.
type definitionEntry struct {
	WordID    int64    `json:"word_id"`
	Lemma     string   `json:"lemma"`
	HomonymNr int      `json:"homonym_nr"`
	WordClass string   `json:"word_class"`
	Datasets  []string `json:"datasets"`
	Meanings  []struct {
		POS             []string `json:"pos"`
		DefinitionsET   []string `json:"definitions_et"`
		TranslationsEN  []string `json:"translations_en"`
		UsagesET        []string `json:"usages_et"`
	} `json:"meanings"`
}

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	dataDir := flag.String("data", "data/ekilex", "Path to reduced Ekilex data root (containing definitions/ and forms/)")
	source := flag.String("source", "ekilex-reduced", "dict_metadata source identifier")
	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("open DB: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		log.Fatalf("WAL: %v", err)
	}
	if _, err := db.Exec(`PRAGMA synchronous=NORMAL`); err != nil {
		log.Fatalf("synchronous: %v", err)
	}

	if err := ensureSchema(db); err != nil {
		log.Fatalf("schema: %v", err)
	}

	defDir := filepath.Join(*dataDir, "definitions")
	formsDir := filepath.Join(*dataDir, "forms")
	if _, err := os.Stat(defDir); err != nil {
		log.Fatalf("definitions dir %q: %v", defDir, err)
	}
	if _, err := os.Stat(formsDir); err != nil {
		log.Fatalf("forms dir %q: %v", formsDir, err)
	}

	// Pass 1: definitions → lemmas. Builds a (lemma, pos) → struct{} set the
	// forms pass needs to know which (lemma, pos) pairs are legitimate.
	log.Println("Pass 1: importing definitions → lemmas")
	lemmaPOS, lemmaCount, err := importDefinitions(db, defDir)
	if err != nil {
		log.Fatalf("definitions: %v", err)
	}
	log.Printf("  imported/updated %d lemma rows (%d unique lemma+pos)", lemmaCount, len(lemmaPOS))

	// Pass 2: forms → forms table. Each (lemma, form) needs a POS, derived
	// from the (lemma, *) entries in lemmaPOS. If a lemma had multiple
	// distinct POS across its meanings (rare — the same headword providing
	// noun and verb meanings under one homonym), we emit one form row per POS.
	log.Println("Pass 2: importing forms")
	formCount, err := importForms(db, formsDir, lemmaPOS)
	if err != nil {
		log.Fatalf("forms: %v", err)
	}
	log.Printf("  imported %d form rows", formCount)

	if err := upsertDictMetadata(db, *source, lemmaCount); err != nil {
		log.Printf("warn: dict_metadata: %v", err)
	}

	fmt.Printf("\nDone. %d lemma rows, %d form rows imported from %s.\n", lemmaCount, formCount, *dataDir)
}

// lemmaPOSMap maps a lemma to the set of POS values it appears with across
// all its meanings. Used by the forms pass to assign a POS to each form row.
type lemmaPOSMap map[string]map[string]struct{}

// importDefinitions walks definitions/*.jsonl and inserts/updates lemmas.
// Empty-gloss guard: if Ekilex has no EN translations for a lemma, an
// existing non-empty gloss in the lemmas table is preserved.
func importDefinitions(db *sql.DB, dir string) (lemmaPOSMap, int, error) {
	lemmaPOS := make(lemmaPOSMap)

	tx, err := db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	stmtInsert, err := tx.Prepare(
		`INSERT OR IGNORE INTO lemmas (lemma, pos, gloss, lang) VALUES (?, ?, ?, 'ET')`,
	)
	if err != nil {
		return nil, 0, err
	}
	defer stmtInsert.Close()

	// Empty-gloss guard: only fill gloss when the existing row has no gloss.
	// This avoids clobbering a richer kaikki gloss with an empty Ekilex gloss
	// (some Ekilex entries have ET definitions but no EN translations).
	stmtFillGloss, err := tx.Prepare(
		`UPDATE lemmas SET gloss = ? WHERE lemma = ? AND pos = ? AND lang = 'ET'
		   AND (gloss IS NULL OR gloss = '')`,
	)
	if err != nil {
		return nil, 0, err
	}
	defer stmtFillGloss.Close()

	count := 0
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var entry definitionEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			lemma := strings.TrimSpace(entry.Lemma)
			if lemma == "" {
				continue
			}

			// Collect the union of POS for this entry. A lemma's meanings
			// can have different `pos` values; record each distinct one.
			posSet := make(map[string]struct{})
			gloss := joinTranslations(entry)
			for _, m := range entry.Meanings {
				for _, raw := range m.POS {
					if upos, ok := posMap[raw]; ok {
						posSet[upos] = struct{}{}
					}
				}
			}
			if len(posSet) == 0 {
				if upos, ok := wordClassFallback[entry.WordClass]; ok {
					posSet[upos] = struct{}{}
				}
			}
			if len(posSet) == 0 {
				continue
			}

			for upos := range posSet {
				if _, err := stmtInsert.Exec(lemma, upos, gloss); err != nil {
					return fmt.Errorf("insert %q %s: %w", lemma, upos, err)
				}
				if gloss != "" {
					if _, err := stmtFillGloss.Exec(gloss, lemma, upos); err != nil {
						return fmt.Errorf("fill gloss %q %s: %w", lemma, upos, err)
					}
				}
				count++
				if _, ok := lemmaPOS[lemma]; !ok {
					lemmaPOS[lemma] = make(map[string]struct{})
				}
				lemmaPOS[lemma][upos] = struct{}{}
			}
		}
		return scanner.Err()
	})
	if walkErr != nil {
		return nil, 0, walkErr
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return lemmaPOS, count, nil
}

// joinTranslations dedups EN translations across all meanings (case-insensitive
// match, original case preserved) and joins with "; ".
func joinTranslations(entry definitionEntry) string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, m := range entry.Meanings {
		for _, t := range m.TranslationsEN {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			key := strings.ToLower(t)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return strings.Join(out, "; ")
}

// morphFormClass categorises an Ekilex morph_code as verbal, nominal, or
// unknown so we can attribute each form to the right POS when a lemma has
// multiple homonyms (e.g. ET "jooma" is both VERB drink and NOUN
// drinking-party — the verb's IndPrSg1 form must not be emitted under NOUN).
type morphFormClass int

const (
	morphFormUnknown morphFormClass = iota
	morphFormVerbal
	morphFormNominal
)

// classifyMorphCode reads the Ekilex morph code's prefix. Verbal codes are
// Ind/Imp/Knd/Kvt/Sup/Pts/Inf/Ger/Neg. Nominal codes are Sg*/Pl*. "ID" is
// for invariable lemmas (e.g. "24/7") and counts as nominal-ish since
// invariable lemmas are not VERBs.
func classifyMorphCode(code string) morphFormClass {
	for _, prefix := range []string{"Ind", "Imp", "Knd", "Kvt", "Sup", "Pts", "Inf", "Ger", "Neg"} {
		if strings.HasPrefix(code, prefix) {
			return morphFormVerbal
		}
	}
	if strings.HasPrefix(code, "Sg") || strings.HasPrefix(code, "Pl") || code == "ID" {
		return morphFormNominal
	}
	return morphFormUnknown
}

// posMatchesMorphClass returns true if a (lemma's) POS is compatible with the
// given form-class. VERB matches verbal codes only; everything else matches
// nominal codes only. Unknown codes match everything (defensive fallback).
func posMatchesMorphClass(upos string, class morphFormClass) bool {
	switch class {
	case morphFormVerbal:
		return upos == "VERB"
	case morphFormNominal:
		return upos != "VERB"
	default:
		return true
	}
}

// importForms walks forms/*.tsv and inserts (form, lemma, pos) rows. Each
// row's POS is taken from lemmaPOS[lemma], filtered against the morph_code:
// verbal codes get attributed to VERB only, nominal codes to non-VERB
// POSes. This matters when a lemma has multiple homonyms with different
// POS — see classifyMorphCode for the rules.
func importForms(db *sql.DB, dir string, lemmaPOS lemmaPOSMap) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT OR IGNORE INTO forms (form, lemma, pos, lang) VALUES (?, ?, ?, 'ET')`,
	)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".tsv") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		// Skip header.
		if !scanner.Scan() {
			return scanner.Err()
		}

		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), "\t", 3)
			if len(parts) < 2 {
				continue
			}
			lemma := strings.TrimSpace(parts[0])
			form := strings.TrimSpace(parts[1])
			morphCode := ""
			if len(parts) > 2 {
				morphCode = strings.TrimSpace(parts[2])
			}
			if lemma == "" || form == "" || form == "-" {
				continue
			}
			form = strings.ToLower(form)
			poss, ok := lemmaPOS[lemma]
			if !ok {
				// Form belongs to a lemma we didn't import (no meanings or
				// definitions left after filtering). Drop it — without a POS
				// the row would be useless for parser lookup anyway.
				continue
			}
			class := classifyMorphCode(morphCode)
			for upos := range poss {
				if !posMatchesMorphClass(upos, class) {
					continue
				}
				if _, err := stmt.Exec(form, lemma, upos); err != nil {
					return fmt.Errorf("insert form %q %q %s: %w", form, lemma, upos, err)
				}
				count++
			}
		}
		return scanner.Err()
	})
	if walkErr != nil {
		return 0, walkErr
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func upsertDictMetadata(db *sql.DB, source string, rowCount int) error {
	if err := ensureDictMetadataTable(db); err != nil {
		return err
	}
	_, err := db.Exec(
		`INSERT OR REPLACE INTO dict_metadata (lang, source, imported_at, row_count) VALUES ('ET', ?, CURRENT_TIMESTAMP, ?)`,
		source, rowCount,
	)
	return err
}

func ensureDictMetadataTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS dict_metadata (
			lang        TEXT NOT NULL,
			source      TEXT NOT NULL,
			imported_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			row_count   INTEGER,
			PRIMARY KEY (lang, source)
		)
	`)
	return err
}

// ensureSchema creates the lemmas, forms, and dict_metadata tables if they
// don't exist — same shape used by the server's initSchema. Lets this
// importer run against a fresh database file without needing the server first.
func ensureSchema(db *sql.DB) error {
	_, err := db.Exec(`
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
	`)
	if err != nil {
		return err
	}
	return ensureDictMetadataTable(db)
}
