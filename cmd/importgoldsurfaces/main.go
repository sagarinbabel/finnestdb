// Command importgoldsurfaces loads the frozen gold evaluation sets into the
// gold_surfaces table, which backs the correction-loop Phase-4 safety guard:
// accepting a parse-feedback correction that contradicts the gold analyses is
// refused (see store.ErrOverrideConflictsWithGold).
//
// Run it after cloning (and again whenever the committed gold sets change):
//
//	go run ./cmd/importgoldsurfaces -db finnestdb.db
//
// With an empty gold_surfaces table the guard is a no-op, so skipping this
// step degrades safety silently rather than breaking acceptance.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/internal/store"
)

// goldFile mirrors the committed gold dataset shape
// (testdata/parser-eval/*/gold/*.json).
type goldFile struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Cases    []struct {
		Tokens []struct {
			Surface string `json:"surface"`
			Lemma   string `json:"lemma"`
			POS     string `json:"pos"`
		} `json:"tokens"`
	} `json:"cases"`
}

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	goldRoot := flag.String("gold-root", "testdata/parser-eval", "Root containing <lang>/gold/*.json")
	flag.Parse()

	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for _, langDir := range []string{"fi", "et"} {
		lang := strings.ToUpper(langDir)
		pattern := filepath.Join(*goldRoot, langDir, "gold", "*.json")
		files, err := filepath.Glob(pattern)
		if err != nil {
			log.Fatalf("glob %s: %v", pattern, err)
		}
		if len(files) == 0 {
			log.Printf("warning: no gold files under %s - %s guard data will be empty", pattern, lang)
		}

		rows, datasets, err := aggregateGoldSurfaces(files, lang)
		if err != nil {
			log.Fatal(err)
		}
		if err := db.ReplaceGoldSurfaces(lang, rows); err != nil {
			log.Fatalf("replace %s gold surfaces: %v", lang, err)
		}
		fmt.Printf("%s: %d gold surface analyses loaded from %d dataset(s)\n", lang, len(rows), datasets)
	}
}

// aggregateGoldSurfaces reads gold files and counts occurrences per
// lowercased (surface, lemma, pos). Files whose language field disagrees
// with the directory are skipped loudly rather than polluting the guard.
func aggregateGoldSurfaces(files []string, lang string) ([]store.GoldSurface, int, error) {
	type key struct{ surface, lemma, pos string }
	counts := make(map[key]int)
	datasets := 0
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, 0, fmt.Errorf("read %s: %w", path, err)
		}
		var gf goldFile
		if err := json.Unmarshal(raw, &gf); err != nil {
			return nil, 0, fmt.Errorf("parse %s: %w", path, err)
		}
		if !strings.EqualFold(gf.Language, lang) {
			log.Printf("warning: %s declares language %q, expected %s - skipped", path, gf.Language, lang)
			continue
		}
		datasets++
		for _, c := range gf.Cases {
			for _, tok := range c.Tokens {
				if tok.Surface == "" || tok.Lemma == "" || tok.POS == "" {
					continue
				}
				counts[key{strings.ToLower(tok.Surface), tok.Lemma, tok.POS}]++
			}
		}
	}
	rows := make([]store.GoldSurface, 0, len(counts))
	for k, n := range counts {
		rows = append(rows, store.GoldSurface{Surface: k.surface, Lemma: k.lemma, POS: k.pos, CaseCount: n})
	}
	return rows, datasets, nil
}
