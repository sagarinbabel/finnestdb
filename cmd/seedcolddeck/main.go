// Command seedcolddeck creates the cold-start "Top N words" official deck for
// a language from a public frequency baseline list (see
// docs/FREQUENCY_BASELINES.md and localdata/frequency/).
//
// It is an operator tool, run once per language per deployment, because the
// frequency lists are gitignored local artifacts: baking the deck into the
// database at seed time means the running app has no dependency on those
// files. Users pick the deck up through the existing official-decks surface
// ("Add to studying list"); the dashboard/decks empty states link there.
//
// Surface forms from the list are resolved to (lemma, pos) through the
// dictionary; unresolved forms are skipped and counted. Lemmas are ranked by
// summed token count across all their inflected forms, so "olen"/"on"/"oli"
// all credit "olla". Each deck entry is a one-token sentence whose text is
// the lemma's most frequent surface form — deck detail and review cards work
// unchanged, showing that form as the example.
//
// Usage:
//
//	go run ./cmd/seedcolddeck -db finnestdb.db -lang FI -owner-email admin@example.com
//	go run ./cmd/seedcolddeck -db finnestdb.db -lang ET -owner-email admin@example.com -top 1000
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"finnestdb/internal/store"
)

const (
	// maxListLines bounds how deep into the frequency list we read before
	// resolution. Resolution loses some forms (names, typos, foreign words),
	// so we read well past -top to have enough resolved lemmas.
	maxListLines = 20000
	lookupChunk  = 500
)

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	lang := flag.String("lang", "", "Language (FI or ET)")
	source := flag.String("source", "", "Frequency list path (default: localdata/frequency/<lang>/opensubtitles-2018-<lang>-50k.txt)")
	top := flag.Int("top", 1000, "Number of lemmas in the deck")
	ownerEmail := flag.String("owner-email", "", "Admin account that owns the official deck (required)")
	title := flag.String("title", "", "Deck title (default: \"Top <N> <language> words\")")
	flag.Parse()

	if *lang != "FI" && *lang != "ET" {
		log.Fatal("-lang must be FI or ET")
	}
	if *ownerEmail == "" {
		log.Fatal("-owner-email is required")
	}
	if *top <= 0 {
		log.Fatal("-top must be positive")
	}
	if *source == "" {
		langLower := strings.ToLower(*lang)
		*source = filepath.Join("localdata", "frequency", langLower,
			fmt.Sprintf("opensubtitles-2018-%s-50k.txt", langLower))
	}
	if *title == "" {
		langName := "Finnish"
		if *lang == "ET" {
			langName = "Estonian"
		}
		*title = fmt.Sprintf("Top %d %s words", *top, langName)
	}

	f, err := os.Open(*source)
	if err != nil {
		log.Fatalf("open frequency list: %v (run cmd/fetchfrequency or make fetch-frequency-baselines first)", err)
	}
	defer f.Close()

	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	owner, err := db.GetUserByEmail(*ownerEmail)
	if err != nil {
		log.Fatalf("look up owner %q: %v", *ownerEmail, err)
	}
	if !owner.IsAdmin {
		log.Fatalf("owner %q is not an admin; official decks are admin-owned", *ownerEmail)
	}

	entries, skipped, err := topLemmas(f, db, *lang, *top)
	if err != nil {
		log.Fatalf("aggregate frequency list: %v", err)
	}
	if len(entries) < *top {
		log.Printf("warning: only %d of %d requested lemmas resolved from %s", len(entries), *top, *source)
	}
	if len(entries) == 0 {
		log.Fatal("no lemmas resolved — is the dictionary imported for this language?")
	}

	sentences := make([]store.DeckSentenceInput, 0, len(entries))
	for _, e := range entries {
		sentences = append(sentences, store.DeckSentenceInput{
			Text: e.ReprForm,
			Tokens: []store.DeckTokenInput{
				{TokenIx: 0, Form: e.ReprForm, Lemma: e.Lemma, POS: e.POS},
			},
		})
	}
	deckID, err := db.CreateDeckWithSentencesOptions(owner.ID, *title, *lang, true, sentences)
	if err != nil {
		log.Fatalf("create official deck: %v", err)
	}

	fmt.Printf("created official deck %q (id %d): %d lemmas, %d list forms skipped as unresolved\n",
		*title, deckID, len(entries), skipped)
}

// lemmaEntry is one aggregated deck entry: a lemma ranked by the summed
// token count of all its surface forms, with its most frequent form kept as
// the representative example.
type lemmaEntry struct {
	Lemma    string
	POS      string
	ReprForm string
	Count    int64
}

// formResolver is the dictionary lookup seam, satisfied by *store.DB.
type formResolver interface {
	BatchLookupForms(forms []string, lang string, parserMode string) map[string]store.FormResolution
}

// topLemmas reads a "form count" frequency list (highest count first),
// resolves forms through the dictionary in chunks, aggregates counts per
// (lemma, pos), and returns the top N entries by summed count. Returns the
// number of list forms that did not resolve.
func topLemmas(r io.Reader, resolver formResolver, lang string, n int) ([]lemmaEntry, int, error) {
	type formCount struct {
		Form  string
		Count int64
	}
	var list []formCount
	scanner := bufio.NewScanner(r)
	for scanner.Scan() && len(list) < maxListLines {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		count, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || count <= 0 {
			continue
		}
		if !hasLetter(fields[0]) {
			continue
		}
		list = append(list, formCount{Form: fields[0], Count: count})
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	type lemmaKey struct{ Lemma, POS string }
	totals := make(map[lemmaKey]*lemmaEntry)
	skipped := 0
	for start := 0; start < len(list); start += lookupChunk {
		end := start + lookupChunk
		if end > len(list) {
			end = len(list)
		}
		chunk := list[start:end]
		forms := make([]string, len(chunk))
		for i, fc := range chunk {
			forms[i] = fc.Form
		}
		resolved := resolver.BatchLookupForms(forms, lang, "custom")
		// Iterate the chunk in list order (highest count first) so the first
		// form credited to a lemma is its most frequent one.
		for _, fc := range chunk {
			res, ok := resolved[fc.Form]
			if !ok || res.Lemma == "" || res.POS == "" || res.POS == "PUNCT" {
				skipped++
				continue
			}
			// Subtitle corpora are dense with character names; a starter
			// vocabulary deck should teach words, not the cast of 50k films.
			if res.POS == "PROPN" {
				skipped++
				continue
			}
			key := lemmaKey{Lemma: res.Lemma, POS: res.POS}
			if entry, exists := totals[key]; exists {
				entry.Count += fc.Count
			} else {
				totals[key] = &lemmaEntry{Lemma: res.Lemma, POS: res.POS, ReprForm: fc.Form, Count: fc.Count}
			}
		}
	}

	entries := make([]lemmaEntry, 0, len(totals))
	for _, e := range totals {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Lemma < entries[j].Lemma
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	return entries, skipped, nil
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
