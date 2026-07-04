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
// all credit "olla". The ranking is shared with cmd/pickexamples through
// internal/starterdeck so the two tools agree on which lemmas the deck holds.
//
// Each deck card is backed by an example sentence. When an examples artifact
// (see cmd/pickexamples and testdata/starter-examples/) is supplied via
// -examples, the card shows a real corpus sentence containing an inflected
// form of the lemma, with the target form stored as the highlighted
// occurrence. Lemmas with no curated example — and every lemma when -examples
// is omitted — fall back to a one-token sentence whose text is the lemma's
// most frequent surface form, so deck detail and review cards still render.
//
// Usage:
//
//	go run ./cmd/seedcolddeck -db finnestdb.db -lang FI -owner-email admin@example.com
//	go run ./cmd/seedcolddeck -db finnestdb.db -lang ET -owner-email admin@example.com -top 1000
//	go run ./cmd/seedcolddeck -db finnestdb.db -lang FI -owner-email admin@example.com \
//	    -examples testdata/starter-examples/fi-examples-v1.tsv
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/internal/starterdeck"
	"finnestdb/internal/store"
)

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	lang := flag.String("lang", "", "Language (FI or ET)")
	source := flag.String("source", "", "Frequency list path (default: localdata/frequency/<lang>/opensubtitles-2018-<lang>-50k.txt)")
	examples := flag.String("examples", "", "Optional starter-examples TSV (see cmd/pickexamples); attaches a real corpus sentence to each matching card")
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

	entries, skipped, err := starterdeck.TopLemmas(f, db, *lang, *top)
	if err != nil {
		log.Fatalf("aggregate frequency list: %v", err)
	}
	if len(entries) < *top {
		log.Printf("warning: only %d of %d requested lemmas resolved from %s", len(entries), *top, *source)
	}
	if len(entries) == 0 {
		log.Fatal("no lemmas resolved — is the dictionary imported for this language?")
	}

	// Optional curated corpus examples keyed by (lemma, pos). Missing file or
	// missing lemma falls back to the representative-form sentence below.
	var exampleByLemma map[starterdeck.LemmaKey]starterdeck.Example
	if *examples != "" {
		exampleByLemma, err = starterdeck.LoadExamples(*examples, *lang)
		if err != nil {
			log.Fatalf("load examples %q: %v", *examples, err)
		}
	}

	sentences, withExample := buildDeckSentences(entries, exampleByLemma)
	deckID, err := db.CreateDeckWithSentencesOptions(owner.ID, *title, *lang, true, sentences)
	if err != nil {
		log.Fatalf("create official deck: %v", err)
	}

	exampleNote := ""
	if *examples != "" {
		exampleNote = fmt.Sprintf(", %d cards with a curated corpus example", withExample)
	}
	fmt.Printf("created official deck %q (id %d): %d lemmas, %d list forms skipped as unresolved%s\n",
		*title, deckID, len(entries), skipped, exampleNote)
}

// buildDeckSentences turns the ranked lemmas into deck sentences. When a
// curated example exists for a lemma and its stored form still tokenizes out of
// the sentence, the card is backed by the real corpus sentence with the
// inflected form as the highlighted occurrence; otherwise it falls back to a
// one-token sentence whose text is the lemma's representative surface form.
// Returns the sentences and how many carried a curated example.
func buildDeckSentences(entries []starterdeck.LemmaEntry, exampleByLemma map[starterdeck.LemmaKey]starterdeck.Example) ([]store.DeckSentenceInput, int) {
	sentences := make([]store.DeckSentenceInput, 0, len(entries))
	withExample := 0
	for _, e := range entries {
		if ex, ok := exampleByLemma[starterdeck.LemmaKey{Lemma: e.Lemma, POS: e.POS}]; ok {
			if ix := starterdeck.FormTokenIndex(ex.Sentence, ex.Form); ix >= 0 {
				sentences = append(sentences, store.DeckSentenceInput{
					Text: ex.Sentence,
					Tokens: []store.DeckTokenInput{
						{TokenIx: ix, Form: ex.Form, Lemma: e.Lemma, POS: e.POS},
					},
				})
				withExample++
				continue
			}
			// The stored form no longer tokenizes out of its sentence (should
			// not happen for artifacts this tool produced). Fall back rather
			// than seed a card whose highlighted form is absent.
			log.Printf("warning: form %q not found in example for %s/%s; using fallback", ex.Form, e.Lemma, e.POS)
		}
		sentences = append(sentences, store.DeckSentenceInput{
			Text: e.ReprForm,
			Tokens: []store.DeckTokenInput{
				{TokenIx: 0, Form: e.ReprForm, Lemma: e.Lemma, POS: e.POS},
			},
		})
	}
	return sentences, withExample
}
