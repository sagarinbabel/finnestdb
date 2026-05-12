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
// Glosses come from the union of EN translations across the meanings that
// share a (lemma, pos), deduplicated case-insensitively and joined with "; ".
// Each meaning is attributed to its own POS only — an entry whose meanings
// span multiple parts of speech (e.g. ADJ + NOUN) does not cross-pollinate.
// Lemmas with no EN translations fall back to the first matching ET
// definition with a `[ET] ` prefix (cap glossFallbackMaxLen runes). Entries
// with zero meanings (or only unmappable ones) get an empty (lemma, upos)
// row keyed by entry-level word_class so the form-import path can still
// attribute forms to them.
//
// Higher-priority rows (custom_overrides at priority 100) are preserved on
// conflict. Pre-existing rows already owned by `source = 'ekilex'` get
// their gloss refreshed from the current Ekilex content on every reimport,
// so lemmas.gloss can't drift from the per-meaning translations and
// definitions tables that are wipe-and-rebuilt by Pass 2.5 / 2.6.
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
	"unicode"

	"finnestdb/internal/glossfallback"
	"finnestdb/internal/store"

	_ "github.com/mattn/go-sqlite3"
)

// maxBadLineSamples caps how many JSONL parse errors we capture verbatim
// (with file:line) for the post-run summary. Beyond this we still count
// them but stop logging detail.
const maxBadLineSamples = 10

// posMap converts an Ekilex meaning-level POS code to UPOS. The table is
// documented in ARCHITECTURE.md (Dictionary section).
var posMap = map[string]string{
	"s":       "NOUN",
	"v":       "VERB",
	"vrm":     "VERB",
	"adj":     "ADJ",
	"adjg":    "ADJ",
	"adjid":   "ADJ",
	"adv":     "ADV",
	"prop":    "PROPN",
	"propgen": "PROPN",
	"pron":    "PRON",
	"num":     "NUM",
	"postp":   "ADP",
	"prep":    "ADP",
	"konj":    "CCONJ",
	"interj":  "INTJ",
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
		POS            []string `json:"pos"`
		DefinitionsET  []string `json:"definitions_et"`
		TranslationsEN []string `json:"translations_en"`
		UsagesET       []string `json:"usages_et"`
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

	// Pass 1: walk definitions and aggregate per (lemma, pos) so the gloss
	// column gets the merged EN translations across all homonyms instead of
	// only the first inserted one. (A naive per-entry write loses real
	// glosses — e.g. aste#1 "step" vs aste#2 "degree" both collapse to
	// aste/NOUN, and one would shadow the other.)
	log.Println("Pass 1: aggregating definitions → lemmas")
	lemmaPOS, aggStats, err := aggregateDefinitions(defDir)
	if err != nil {
		log.Fatalf("definitions: %v", err)
	}
	log.Printf("  aggregated %d unique (lemma, pos) entries", countLemmaPOS(lemmaPOS))
	if aggStats.badLines > 0 || aggStats.emptyLemma > 0 || aggStats.noPOS > 0 {
		log.Printf("  soft-skipped: %d JSONL parse errors, %d entries with no lemma, %d entries with no resolvable POS",
			aggStats.badLines, aggStats.emptyLemma, aggStats.noPOS)
		for _, s := range aggStats.badSamples {
			log.Printf("    bad-line: %s", s)
		}
		if aggStats.badLines > len(aggStats.badSamples) {
			log.Printf("    ... and %d more JSONL parse errors not shown",
				aggStats.badLines-len(aggStats.badSamples))
		}
	}

	// Pass 2: write the aggregated lemmas. Reports rows actually inserted
	// (vs ignored because they collide with kaikki) and rows whose gloss
	// got filled by Ekilex's merged translations.
	log.Println("Pass 2: writing lemma rows")
	lemmaInserted, glossFilled, err := writeLemmas(db, lemmaPOS)
	if err != nil {
		log.Fatalf("write lemmas: %v", err)
	}
	log.Printf("  touched %d lemma rows (inserted or source-upgraded), filled gloss on %d existing rows", lemmaInserted, glossFilled)

	// Pass 2.5: write per-meaning translations. Phase 2 of the lexical
	// plan: the read path consults the translations table first, so each
	// EN translation gets its own row with a stable sense_idx. Mirrors
	// cmd/importdict's PR #85 pattern — wipe-and-rebuild inside the
	// transaction so a failure here doesn't leave Ekilex translations in
	// an empty intermediate state.
	log.Println("Pass 2.5: writing translation rows")
	translationsWritten, err := writeTranslations(db, lemmaPOS)
	if err != nil {
		log.Fatalf("translations: %v", err)
	}
	log.Printf("  wrote %d translation rows", translationsWritten)

	// Pass 2.6: write per-meaning Estonian-language definitions. The
	// definitions table is the structured source-language counterpart of
	// translations. Wipe-and-rebuild for the same reason translations
	// does — a re-import after upstream Ekilex pruned senses must not
	// leave stale rows behind.
	log.Println("Pass 2.6: writing Estonian definition rows")
	definitionsWritten, err := writeDefinitions(db, lemmaPOS)
	if err != nil {
		log.Fatalf("definitions: %v", err)
	}
	log.Printf("  wrote %d definition rows", definitionsWritten)

	// Pass 3: walk forms and write (form, lemma, pos, lang) rows. Tuples
	// the importer actually emits is the count we care about (an Ekilex
	// duplicate, e.g. PtsPtIps + PtsPtIpsNeg both yielding "joodud" for
	// jooma/VERB, only adds one row; we don't double-count).
	log.Println("Pass 3: writing form rows")
	formInserted, err := importForms(db, formsDir, lemmaPOS)
	if err != nil {
		log.Fatalf("forms: %v", err)
	}
	log.Printf("  touched %d form rows (inserted or source-upgraded)", formInserted)

	if err := upsertDictMetadata(db, *source, countLemmaPOS(lemmaPOS)); err != nil {
		log.Printf("warn: dict_metadata: %v", err)
	}

	fmt.Printf("\nDone. %d unique (lemma, pos) Ekilex entries; %d lemma rows touched; %d gloss fills; %d translation rows; %d definition rows; %d form rows touched (data: %s).\n",
		countLemmaPOS(lemmaPOS), lemmaInserted, glossFilled, translationsWritten, definitionsWritten, formInserted, *dataDir)
}

// lemmaPOSMap maps a lemma to a set of POS values, each carrying the
// accumulated EN translations across every Ekilex entry that contributed
// to that (lemma, pos) pair. Multiple homonyms of the same lemma collapse
// here when they share a POS — e.g. aste#1 ("step") and aste#2 ("degree"):
// both are NOUN, so their translations merge into one
// `lemmaPOSMap["aste"]["NOUN"]` slice.
type lemmaPOSMap map[string]map[string]*lemmaPOSData

type lemmaPOSData struct {
	// translationsByKey maps the lowercase form of an EN translation to
	// the chosen original-cased version. Dedup is case-insensitive — when
	// the same lowercase key arrives with different casings (e.g. real
	// data has "Calvinism" and "calvinism", "Turbot" and "turbot") the
	// uppercase-leading variant wins, see chooseCasing. This biases
	// toward the proper-noun / acronym reading, which is the common
	// reason the same word appears with different cases in Ekilex.
	translationsByKey map[string]string

	// definitionsET holds Estonian-language definitions accumulated across
	// every meaning entry that contributed to this (lemma, pos) pair.
	// Order is insertion-order-stable — duplicates are dropped (case-
	// sensitive match, since Estonian definitions don't have the same
	// case-collision pattern that EN translations do). The gloss-fallback
	// path uses the first entry as a `[ET] `-prefixed gloss when no EN
	// translation is available; the full slice writes one row per sense
	// into the definitions table.
	definitionsET    []string
	definitionsETSet map[string]struct{}
}

func (m lemmaPOSMap) add(lemma, upos string, translations, definitionsET []string) {
	posMap, ok := m[lemma]
	if !ok {
		posMap = make(map[string]*lemmaPOSData)
		m[lemma] = posMap
	}
	d, ok := posMap[upos]
	if !ok {
		d = &lemmaPOSData{
			translationsByKey: make(map[string]string),
			definitionsETSet:  make(map[string]struct{}),
		}
		posMap[upos] = d
	}
	for _, t := range translations {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		existing, dup := d.translationsByKey[key]
		if !dup {
			d.translationsByKey[key] = t
			continue
		}
		d.translationsByKey[key] = chooseCasing(existing, t)
	}
	for _, def := range definitionsET {
		def = strings.TrimSpace(def)
		if def == "" {
			continue
		}
		if _, dup := d.definitionsETSet[def]; dup {
			continue
		}
		d.definitionsETSet[def] = struct{}{}
		d.definitionsET = append(d.definitionsET, def)
	}
}

// chooseCasing breaks ties between two translations that share a lowercase
// form. A translation that begins with an uppercase letter wins over its
// all-lowercase variant — proper nouns and acronyms are the dominant cause
// of case collisions in real Ekilex data ("Calvinism" / "calvinism",
// "Turbot" / "turbot"). When neither or both have an uppercase first
// letter, we fall back to lexicographic order for determinism so re-runs
// produce byte-stable output.
func chooseCasing(a, b string) string {
	switch {
	case a == "" && b != "":
		return b
	case b == "" && a != "":
		return a
	}
	upperA := startsUpper(a)
	upperB := startsUpper(b)
	switch {
	case upperA && !upperB:
		return a
	case upperB && !upperA:
		return b
	case a < b:
		return a
	default:
		return b
	}
}

func startsUpper(s string) bool {
	if s == "" {
		return false
	}
	r := []rune(s)[0]
	return unicode.IsUpper(r)
}

func countLemmaPOS(m lemmaPOSMap) int {
	n := 0
	for _, byPOS := range m {
		n += len(byPOS)
	}
	return n
}

// aggregateStats records soft failures encountered while walking the
// definitions JSONL — silent skips would let upstream schema drift go
// unnoticed.
type aggregateStats struct {
	badLines   int
	badSamples []string // up to maxBadLineSamples, "<file>:<line>: <error>"
	emptyLemma int      // valid JSON but missing Lemma
	noPOS      int      // entry had no POS we could resolve
}

// aggregateDefinitions walks definitions/*.jsonl and builds the lemmaPOSMap.
// No DB writes happen here — the returned aggregator is consumed by
// writeLemmas and importForms. The stats struct surfaces soft failures
// (JSON parse errors, lemma-less entries, entries with no resolvable POS)
// so we can log a summary instead of skipping silently.
func aggregateDefinitions(dir string) (lemmaPOSMap, *aggregateStats, error) {
	lemmaPOS := make(lemmaPOSMap)
	stats := &aggregateStats{}

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
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var entry definitionEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				stats.badLines++
				if len(stats.badSamples) < maxBadLineSamples {
					stats.badSamples = append(stats.badSamples,
						fmt.Sprintf("%s:%d: %v", path, lineNo, err))
				}
				continue
			}
			lemma := strings.TrimSpace(entry.Lemma)
			if lemma == "" {
				stats.emptyLemma++
				continue
			}

			// Attribute meanings per-POS, not per whole entry. A single
			// Ekilex entry can carry meanings under different parts of speech
			// (e.g. aastatagune has an ADJ meaning followed by NOUN meanings);
			// flattening would write ADJ definitions into the NOUN row and
			// vice versa. Each meaning's translations and definitions go
			// only to the POSes that meaning resolves to. The entry-level
			// word_class is the per-meaning fallback when a meaning's own
			// pos field is empty or unmappable.
			hasAnyPOS := false
			for _, m := range entry.Meanings {
				meaningPOS := make(map[string]struct{})
				for _, raw := range m.POS {
					if upos, ok := posMap[raw]; ok {
						meaningPOS[upos] = struct{}{}
					}
				}
				if len(meaningPOS) == 0 {
					if upos, ok := wordClassFallback[entry.WordClass]; ok {
						meaningPOS[upos] = struct{}{}
					}
				}
				if len(meaningPOS) == 0 {
					continue
				}
				hasAnyPOS = true
				for upos := range meaningPOS {
					lemmaPOS.add(lemma, upos, m.TranslationsEN, m.DefinitionsET)
				}
			}

			// Entry-level fallback for entries with zero meanings (or where
			// every meaning failed to resolve a POS): write an empty
			// (lemma, upos) row keyed by word_class so the form-import path
			// can still attribute forms to this lemma. Real Ekilex carries
			// ~19k such fallback-importable lemmas (e.g. verbs like
			// "alajahtuma") whose definitions/forms files split — meanings
			// often live in a paired entry while forms still need a target
			// (lemma, pos) to attach to.
			if !hasAnyPOS {
				if upos, ok := wordClassFallback[entry.WordClass]; ok {
					lemmaPOS.add(lemma, upos, nil, nil)
					hasAnyPOS = true
				}
			}
			if !hasAnyPOS {
				stats.noPOS++
				continue
			}
		}
		return scanner.Err()
	})
	if walkErr != nil {
		return nil, stats, walkErr
	}
	return lemmaPOS, stats, nil
}

// writeLemmas issues one INSERT OR IGNORE + one conditional UPDATE per
// aggregated (lemma, pos), with the merged Ekilex gloss. Returns
// (newRowsInserted, glossFillsApplied) — both measured from RowsAffected
// so they reflect actual database changes, not insert attempts.
func writeLemmas(db *sql.DB, lemmaPOS lemmaPOSMap) (int, int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	// Pre-INSERT refresh: rewrite the gloss on rows already owned by
	// `ekilex` BEFORE the upsert below has a chance to source-upgrade
	// anything. This is the bookkeeping that lets us tell "row was already
	// Ekilex-owned at the start of this run" apart from "row will be
	// Ekilex-owned after the upgrade fires" — once stmtInsert has run, the
	// two are indistinguishable from the source column alone.
	//
	// Why: lemmas.gloss is a denormalized cache of the (lemma, pos) entry
	// that wordlist consumers join on directly. Pass 2.5 / 2.6 wipe-and-
	// rebuild translations and definitions per source, so an Ekilex run
	// where the EN translation changed, was removed, fell back to an ET
	// definition, or disappeared entirely (the (lemma, pos) is now reached
	// only via the word_class fallback) would otherwise leave lemmas.gloss
	// pointing at stale text. The refresh fires unconditionally — empty
	// new gloss is allowed and clears the cache to match the now-empty
	// translations / definitions tables.
	//
	// `COALESCE(gloss, '') <> ?` keeps the RowsAffected counter honest:
	// rows where the value would not change (existing gloss already matches
	// the new gloss, including the empty-to-empty case) are filtered out
	// by the WHERE clause and don't inflate `glossFilled`.
	//
	// Higher-priority rows (custom_overrides at priority 100) and rows
	// owned by a different lower-priority source (e.g. kaikki) are
	// untouched — only `source = 'ekilex'` rows match. Kaikki bootstrap
	// glosses preserved through a previous Ekilex source-upgrade keep
	// their text on the *first* same-source reimport, but a second one
	// will replace them with the current Ekilex output: that's the price
	// of keeping translations/definitions and lemmas.gloss consistent.
	stmtRefreshEkilexGloss, err := tx.Prepare(
		`UPDATE lemmas SET gloss = ?
		 WHERE lemma = ? AND pos = ? AND lang = 'ET' AND source = 'ekilex'
		   AND COALESCE(gloss, '') <> ?`,
	)
	if err != nil {
		return 0, 0, err
	}
	defer stmtRefreshEkilexGloss.Close()

	// Upsert: insert a fresh row, or upgrade an existing row's source /
	// source_priority when this import is *strictly stronger* (higher
	// priority). `<` instead of `<=` so same-priority self-conflicts during
	// a single import don't fire no-op updates — keeps RowsAffected
	// honestly counting "rows whose source actually changed plus rows newly
	// inserted." Gloss is intentionally not part of the conflict update —
	// the pre-INSERT refresh above handles already-ekilex-owned rows; the
	// post-INSERT empty-gloss fill below handles freshly-upgraded rows
	// whose gloss column was empty.
	stmtInsert, err := tx.Prepare(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES (?, ?, ?, 'ET', 'ekilex', 20)
		 ON CONFLICT(lemma, pos, lang) DO UPDATE SET
		   source = excluded.source,
		   source_priority = excluded.source_priority
		 WHERE lemmas.source_priority < excluded.source_priority`,
	)
	if err != nil {
		return 0, 0, err
	}
	defer stmtInsert.Close()

	// Empty-gloss fill: protect non-ekilex rows that arrived empty (e.g.
	// a pre-existing kaikki row created before glosses were threaded
	// through the importer). Once the upsert above has source-upgraded
	// such a row to ekilex, this fills the gloss slot from the current
	// Ekilex content. Non-empty glosses on non-ekilex sources are left
	// alone; the pre-INSERT refresh has already handled the ekilex case.
	stmtFillGloss, err := tx.Prepare(
		`UPDATE lemmas SET gloss = ? WHERE lemma = ? AND pos = ? AND lang = 'ET'
		   AND (gloss IS NULL OR gloss = '')`,
	)
	if err != nil {
		return 0, 0, err
	}
	defer stmtFillGloss.Close()

	inserted := 0
	glossFilled := 0
	for lemma, byPOS := range lemmaPOS {
		for upos, data := range byPOS {
			gloss := joinTranslationData(data)
			res, err := stmtRefreshEkilexGloss.Exec(gloss, lemma, upos, gloss)
			if err != nil {
				return inserted, glossFilled, fmt.Errorf("refresh ekilex gloss %q %s: %w", lemma, upos, err)
			}
			if n, _ := res.RowsAffected(); n == 1 {
				glossFilled++
			}
			res, err = stmtInsert.Exec(lemma, upos, gloss)
			if err != nil {
				return inserted, glossFilled, fmt.Errorf("insert %q %s: %w", lemma, upos, err)
			}
			if n, _ := res.RowsAffected(); n == 1 {
				inserted++
			}
			if gloss != "" {
				res, err := stmtFillGloss.Exec(gloss, lemma, upos)
				if err != nil {
					return inserted, glossFilled, fmt.Errorf("fill gloss %q %s: %w", lemma, upos, err)
				}
				if n, _ := res.RowsAffected(); n == 1 {
					glossFilled++
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return inserted, glossFilled, err
	}
	return inserted, glossFilled, nil
}

// writeTranslations writes per-(lemma, pos) sense-level translation rows.
// Phase 2 read path (BatchLookupGlosses) prefers the translations table
// over lemmas.gloss, so each EN translation gets its own row with a
// deterministic sense_idx (matching the lemmas.gloss `; `-joined order).
//
// Mirrors cmd/importdict's PR #85 pattern:
//   - Wipe this source's existing ET translations BEFORE writing, so
//     entries whose translation list shrunk upstream don't leave stale
//     sense_idx rows behind.
//   - DELETE inside the transaction so a failure preserves the existing
//     translations rather than leaving the dictionary empty.
//   - Upsert (ON CONFLICT DO UPDATE SET text = excluded.text) refreshes
//     text when the same sense_idx is rewritten. Conflicts during a
//     single import don't happen (we wipe first), but the upsert is
//     defensive.
func writeTranslations(db *sql.DB, lemmaPOS lemmaPOSMap) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM translations WHERE lang = 'ET' AND source = 'ekilex'`); err != nil {
		return 0, err
	}

	stmt, err := tx.Prepare(
		`INSERT INTO translations (lemma, pos, lang, target_lang, text, sense_idx, source)
		 VALUES (?, ?, 'ET', 'EN', ?, ?, 'ekilex')
		 ON CONFLICT (lemma, pos, lang, target_lang, sense_idx, source) DO UPDATE SET
		   text = excluded.text`,
	)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	written := 0
	for lemma, byPOS := range lemmaPOS {
		for pos, data := range byPOS {
			translations := sortedTranslations(data)
			for senseIdx, text := range translations {
				if _, err := stmt.Exec(lemma, pos, text, senseIdx); err != nil {
					return written, fmt.Errorf("write translation %q %s sense=%d: %w", lemma, pos, senseIdx, err)
				}
				written++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return written, err
	}
	return written, nil
}

// writeDefinitions writes per-(lemma, pos) sense-level Estonian definitions
// into the definitions table. The translations table holds target-language
// (EN) glosses; this is the source-language (ET) counterpart, used by:
//
//   - the user-friendly wordlist exporter when an EN gloss is unavailable —
//     so the row carries a meaning, even if it's in Estonian.
//   - future glossary tools that want the original EKI definitions (longer,
//     more nuanced than EN translations) for advanced learners.
//
// sense_idx preserves Ekilex's per-meaning ordering as definitions arrive
// from the JSONL — entries earlier in `meanings[]` get lower sense_idx.
// Wipe-and-rebuild within a transaction so a re-import after upstream
// pruning doesn't leave stale rows behind.
func writeDefinitions(db *sql.DB, lemmaPOS lemmaPOSMap) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM definitions WHERE lang = 'ET' AND source = 'ekilex'`); err != nil {
		return 0, err
	}

	stmt, err := tx.Prepare(
		`INSERT INTO definitions (lemma, pos, lang, sense_idx, text, source)
		 VALUES (?, ?, 'ET', ?, ?, 'ekilex')
		 ON CONFLICT (lemma, pos, lang, sense_idx, source) DO UPDATE SET
		   text = excluded.text`,
	)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	written := 0
	for lemma, byPOS := range lemmaPOS {
		for pos, data := range byPOS {
			for senseIdx, def := range data.definitionsET {
				if _, err := stmt.Exec(lemma, pos, senseIdx, def); err != nil {
					return written, fmt.Errorf("write definition %q %s sense=%d: %w", lemma, pos, senseIdx, err)
				}
				written++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return written, err
	}
	return written, nil
}

// glossFallbackMaxLen caps the prefixed-fallback length so that ET
// definitions, which can run multiple hundred characters, don't bloat the
// gloss column past sensible UI lengths. Full definitions land in the
// definitions table without truncation.
const glossFallbackMaxLen = 240

// sortedTranslations returns the deduplicated translations sorted by their
// original-cased value. Used both for the lemmas.gloss `; `-joined cache
// and for assigning sense_idx in the translations table — the same sort
// order in both places keeps `lemmas.gloss[0]` and `translations[sense_idx=0]`
// referentially consistent for a given (lemma, pos).
func sortedTranslations(d *lemmaPOSData) []string {
	if d == nil || len(d.translationsByKey) == 0 {
		return nil
	}
	out := make([]string, 0, len(d.translationsByKey))
	for _, t := range d.translationsByKey {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// joinTranslationData sorts the deduplicated translations and joins with
// "; " for the gloss column. Sorting keeps output byte-stable across runs.
//
// When no EN translations are available for a (lemma, pos), the first
// Estonian-language definition is returned with the `[ET] ` prefix as a
// fallback — better to ship "[ET] kuke kujutis varasema aja aabitsa kaanel"
// than an empty gloss, since the empty case is what the user-friendly
// wordlist would otherwise render as a blank meaning. The prefix lets
// downstream renderers tell EN translations apart from ET fallbacks and
// style them differently (or hide the fallback when EN-only is needed).
func joinTranslationData(d *lemmaPOSData) string {
	out := sortedTranslations(d)
	if len(out) > 0 {
		return strings.Join(out, "; ")
	}
	if d == nil || len(d.definitionsET) == 0 {
		return ""
	}
	def := d.definitionsET[0]
	if utf8RuneCount(def) > glossFallbackMaxLen {
		def = utf8Truncate(def, glossFallbackMaxLen) + "…"
	}
	return glossfallback.ETPrefix + def
}

// utf8RuneCount returns the rune count without allocating a []rune slice for
// the common short-string case.
func utf8RuneCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// utf8Truncate returns the prefix of s containing the first n runes,
// preserving rune boundaries. Used to cap ET-definition fallback glosses.
func utf8Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
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

// importForms walks forms/*.tsv and inserts (form, lemma, pos) rows. POS is
// taken from lemmaPOS[lemma], filtered against the morph_code: verbal codes
// get attributed to VERB only, nominal codes to non-VERB POSes. This matters
// when a lemma has multiple homonyms with different POS — see
// classifyMorphCode for the rules.
//
// Returns the count of rows actually inserted (RowsAffected==1). Tuples
// already present from a previous import (e.g. the same canonical form
// being emitted under multiple morph codes — "joodud" PtsPtIps and
// PtsPtIpsNeg both yield (joodud, jooma, VERB)) only insert once and are
// not re-counted.
func importForms(db *sql.DB, dir string, lemmaPOS lemmaPOSMap) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Upsert: insert fresh, or upgrade source/source_priority on conflict
	// when strictly stronger. See writeLemmas comment for rationale.
	//
	// 2026-05-07: extended to also write `feats` (UD FEATS string) derived
	// from Ekilex's morph_code. Ekilex carries case+number+verbal-feature
	// data per form; the import previously discarded it. See
	// docs/LEARNINGS.md "Estonian still falls behind FI" entry.
	//
	// On conflict, feats is upgraded when the incoming row is higher priority,
	// when an equal-priority row has empty FEATS, or when an Ekilex re-import
	// repairs one of the stale non-finite FEATS values produced by older
	// versions of this importer. This protects higher-priority curated rows
	// and avoids replacing legitimate same-source ambiguity such as genitive
	// vs. partitive noun homographs.
	stmt, err := tx.Prepare(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority, feats)
		 VALUES (?, ?, ?, 'ET', 'ekilex', 20, ?)
		 ON CONFLICT(form, lang, lemma, pos) DO UPDATE SET
		   source = excluded.source,
		   source_priority = excluded.source_priority,
		   feats = CASE
		     WHEN excluded.feats != '' AND (
		       forms.source_priority < excluded.source_priority
		       OR (forms.source_priority = excluded.source_priority AND (forms.feats IS NULL OR forms.feats = ''))
		       OR (forms.source = excluded.source AND forms.source_priority = excluded.source_priority AND (
		         (forms.feats = 'VerbForm=Inf' AND excluded.feats = 'Case=Ill|VerbForm=Sup')
		         OR (forms.feats = 'VerbForm=Sup' AND excluded.feats = 'VerbForm=Inf')
		         OR (forms.feats = 'VerbForm=Sup|Voice=Pass' AND excluded.feats = 'Case=Ill|VerbForm=Sup|Voice=Pass')
		         OR (forms.feats = 'Polarity=Neg|VerbForm=Sup' AND excluded.feats = 'Polarity=Neg|VerbForm=Inf')
		       ))
		     )
		     THEN excluded.feats
		     ELSE forms.feats
		   END
		 WHERE forms.source_priority < excluded.source_priority
		    OR (excluded.feats != '' AND (
		      (forms.source_priority = excluded.source_priority AND (forms.feats IS NULL OR forms.feats = ''))
		      OR (forms.source = excluded.source AND forms.source_priority = excluded.source_priority AND (
		        (forms.feats = 'VerbForm=Inf' AND excluded.feats = 'Case=Ill|VerbForm=Sup')
		        OR (forms.feats = 'VerbForm=Sup' AND excluded.feats = 'VerbForm=Inf')
		        OR (forms.feats = 'VerbForm=Sup|Voice=Pass' AND excluded.feats = 'Case=Ill|VerbForm=Sup|Voice=Pass')
		        OR (forms.feats = 'Polarity=Neg|VerbForm=Sup' AND excluded.feats = 'Polarity=Neg|VerbForm=Inf')
		      ))
		    ))`,
	)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	inserted := 0
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
			feats := ekilexMorphToFeats(morphCode)
			for upos := range poss {
				if !posMatchesMorphClass(upos, class) {
					continue
				}
				res, err := stmt.Exec(form, lemma, upos, feats)
				if err != nil {
					return fmt.Errorf("insert form %q %q %s: %w", form, lemma, upos, err)
				}
				if n, _ := res.RowsAffected(); n == 1 {
					inserted++
				}
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
	return inserted, nil
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
// don't exist — same shape used by the server's initSchema. Then runs the
// shared idempotent migrations so the schema matches what cmd/server uses
// even when the importer runs against a fresh DB. Order matters:
//   - EnsureMultiLemmaSchema lifts the legacy (form, lang) PK to
//     (form, lang, lemma, pos) when an existing DB had the old shape.
//   - EnsureDictionarySourceColumns adds source / source_priority on
//     lemmas and forms; insert statements below reference these.
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
	if err := ensureDictMetadataTable(db); err != nil {
		return err
	}
	if err := store.EnsureMultiLemmaSchema(db); err != nil {
		return fmt.Errorf("multi-lemma migration: %w", err)
	}
	if err := store.EnsureDictionarySourceColumns(db); err != nil {
		return fmt.Errorf("source-priority columns: %w", err)
	}
	// Phase 2: writeTranslations writes per-meaning rows into the
	// translations table. EnsureLexicalEntryTables creates it on a
	// fresh DB; on an existing DB (post-#76) it's a no-op via the
	// idempotent CREATE TABLE IF NOT EXISTS pattern.
	if err := store.EnsureLexicalEntryTables(db); err != nil {
		return fmt.Errorf("lexical entry tables: %w", err)
	}
	// 2026-05-07: backfill the `feats` column on forms (and paradigm_class
	// on lemmas) so the FEATS-from-morph_code import path can write into
	// it. No-op on fresh DBs (CREATE TABLE in initSchema already includes
	// these columns); only matters on legacy DBs being upgraded.
	if err := store.EnsureLexicalEnrichmentColumns(db); err != nil {
		return fmt.Errorf("lexical enrichment columns: %w", err)
	}
	return nil
}
