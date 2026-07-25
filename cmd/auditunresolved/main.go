package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"finnestdb/internal/store"
)

type ankiResp[T any] struct {
	Result T      `json:"result"`
	Error  string `json:"error"`
}

func ankiInvoke(action string, params map[string]any, out any) error {
	body, _ := json.Marshal(map[string]any{
		"action":  action,
		"version": 6,
		"params":  params,
	})
	resp, err := http.Post("http://127.0.0.1:8765", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return json.Unmarshal(b, out)
}

// Mirror the client-side parseKnownWordsInput: split on \n , ;, trim, dedupe
// case-insensitively. Preserves the surface form for the first occurrence.
func splitWords(raw string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, part := range regexp.MustCompile(`[\n,;]+`).Split(raw, -1) {
		w := strings.TrimSpace(part)
		if w == "" {
			continue
		}
		key := strings.ToLower(w)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, w)
	}
	return out
}

// Mirror the client-side stripHtml: drop tags, decode entities (best effort),
// return trimmed text.
var htmlTag = regexp.MustCompile(`<[^>]+>`)
var entities = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'")

func stripHTML(s string) string {
	s = htmlTag.ReplaceAllString(s, "")
	s = entities.Replace(s)
	return strings.TrimSpace(s)
}

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "--check-words" {
		// Probe mode: read newline-separated surface forms from stdin and
		// print each with its resolved (lemma, POS) or UNRESOLVED.
		dbPath := os.Args[2]
		db, err := store.NewDB(dbPath)
		if err != nil {
			log.Fatalf("open db: %v", err)
		}
		defer db.Close()
		buf, _ := io.ReadAll(os.Stdin)
		var words []string
		for _, line := range strings.Split(string(buf), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			words = append(words, strings.ToLower(line))
		}
		res := db.BatchLookupForms(words, "ET", "custom")
		for _, w := range words {
			r, ok := res[w]
			if !ok || r.Lemma == "" || r.POS == "" {
				fmt.Printf("%-30s -> UNRESOLVED\n", w)
				continue
			}
			fmt.Printf("%-30s -> %s/%s\n", w, r.Lemma, r.POS)
		}
		return
	}

	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "usage:\n  %s <db-path> <deck-name> <field-name>\n  %s --check-words <db-path> < words.txt\n", os.Args[0], os.Args[0])
		os.Exit(2)
	}
	dbPath := os.Args[1]
	deck := os.Args[2]
	field := os.Args[3]

	// 1. Pull note IDs.
	var idsResp ankiResp[[]int64]
	if err := ankiInvoke("findNotes", map[string]any{"query": fmt.Sprintf(`deck:"%s"`, deck)}, &idsResp); err != nil {
		log.Fatalf("findNotes: %v", err)
	}
	if idsResp.Error != "" {
		log.Fatalf("findNotes: %s", idsResp.Error)
	}
	fmt.Fprintf(os.Stderr, "notes: %d\n", len(idsResp.Result))

	// 2. Pull notesInfo in chunks.
	type noteInfo struct {
		ModelName string                       `json:"modelName"`
		Fields    map[string]struct{ Value string } `json:"fields"`
	}
	var allNotes []noteInfo
	chunk := 500
	for i := 0; i < len(idsResp.Result); i += chunk {
		end := i + chunk
		if end > len(idsResp.Result) {
			end = len(idsResp.Result)
		}
		var nr ankiResp[[]noteInfo]
		if err := ankiInvoke("notesInfo", map[string]any{"notes": idsResp.Result[i:end]}, &nr); err != nil {
			log.Fatalf("notesInfo: %v", err)
		}
		if nr.Error != "" {
			log.Fatalf("notesInfo: %s", nr.Error)
		}
		allNotes = append(allNotes, nr.Result...)
	}
	fmt.Fprintf(os.Stderr, "notes fetched: %d\n", len(allNotes))

	// 3. Extract field values, dedupe like the client does.
	seen := map[string]struct{}{}
	words := []string{}
	skipped := 0
	for _, n := range allNotes {
		raw, ok := n.Fields[field]
		if !ok {
			continue
		}
		text := stripHTML(raw.Value)
		if text == "" {
			skipped++
			continue
		}
		for _, w := range splitWords(text) {
			key := strings.ToLower(w)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			words = append(words, w)
		}
	}
	fmt.Fprintf(os.Stderr, "field empty/missing: %d notes\n", skipped)
	fmt.Fprintf(os.Stderr, "unique surface forms: %d\n", len(words))

	// 4. Run BatchLookupForms (custom mode) - same as /api/known-words.
	db, err := store.NewDB(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	normalized := make([]string, 0, len(words))
	normSeen := map[string]struct{}{}
	for _, w := range words {
		t := strings.TrimSpace(strings.ToLower(w))
		if t == "" {
			continue
		}
		if _, ok := normSeen[t]; ok {
			continue
		}
		normSeen[t] = struct{}{}
		normalized = append(normalized, t)
	}
	resolutions := db.BatchLookupForms(normalized, "ET", "custom")

	// 5. Categorise: imported (resolved + unique lemma+POS) vs unresolved.
	importedLemmas := map[string]struct{}{}
	unresolved := []string{}
	for _, w := range normalized {
		r, ok := resolutions[w]
		if !ok || r.Lemma == "" || r.POS == "" {
			unresolved = append(unresolved, w)
			continue
		}
		importedLemmas[r.Lemma+"\x00"+r.POS] = struct{}{}
	}
	fmt.Fprintf(os.Stderr, "imported lemma+POS: %d\n", len(importedLemmas))
	fmt.Fprintf(os.Stderr, "unresolved surface forms: %d\n", len(unresolved))

	sort.Strings(unresolved)
	fmt.Println("\n=== UNRESOLVED ===")
	for _, w := range unresolved {
		fmt.Println(w)
	}
}
