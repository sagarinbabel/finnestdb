// Command exportgoldcandidates renders pending correction-loop Phase-3
// promotions (gold_candidates rows) as gold-case JSON fragments for manual
// review. A human decides which fragments enter the committed gold sets under
// testdata/parser-eval/*/gold - auto-committing eval cases would let the
// thing being evaluated write its own exam.
//
// Usage:
//
//	go run ./cmd/exportgoldcandidates -db finnestdb.db                 # print pending
//	go run ./cmd/exportgoldcandidates -db finnestdb.db -mark-exported  # print + flip to exported
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"finnestdb/internal/store"
)

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	markExported := flag.Bool("mark-exported", false, "Mark printed candidates as exported")
	flag.Parse()

	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	candidates, err := db.ListGoldCandidates("pending")
	if err != nil {
		log.Fatalf("list gold candidates: %v", err)
	}
	if len(candidates) == 0 {
		fmt.Println("no pending gold candidates")
		return
	}

	ids := make([]int64, 0, len(candidates))
	for _, c := range candidates {
		fragment, err := candidateGoldFragment(c)
		if err != nil {
			log.Fatalf("render candidate %d: %v", c.ID, err)
		}
		fmt.Println(fragment)
		ids = append(ids, c.ID)
	}
	fmt.Printf("\n%d pending candidate(s). Review each fragment, add the good ones to the matching\n", len(candidates))
	fmt.Println("gold set under testdata/parser-eval/<lang>/gold/ with real sentence context, then")
	fmt.Println("re-run cmd/importgoldsurfaces and the parser eval before committing.")

	if *markExported {
		if err := db.MarkGoldCandidatesExported(ids); err != nil {
			log.Fatalf("mark exported: %v", err)
		}
		fmt.Printf("marked %d candidate(s) as exported\n", len(ids))
	}
}

// candidateGoldFragment renders one candidate as a gold-case token JSON
// object matching the committed dataset token shape, annotated with its
// promotion provenance.
func candidateGoldFragment(c store.GoldCandidate) (string, error) {
	token := map[string]any{
		"surface": c.Surface,
		"lemma":   c.Lemma,
		"pos":     c.POS,
	}
	if c.Feats != "" {
		token["feats"] = c.Feats
	}
	payload := map[string]any{
		"lang":            c.Lang,
		"token":           token,
		"supporter_count": c.SupporterCount,
		"source":          "accepted parse feedback (Phase 3 auto-promotion)",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
