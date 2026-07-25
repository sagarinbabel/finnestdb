package main

import (
	"strings"
	"testing"

	"finnestdb/internal/starterdeck"
	"finnestdb/internal/store"
)

// fiExamplesArtifact is the checked-in starter-examples TSV cmd/seedcolddeck
// is documented to take via -examples (see docs/DEPLOYMENT.md,
// docs/FOR_MICHAEL.md). Path is relative to this package directory.
const fiExamplesArtifact = "../../testdata/starter-examples/fi-examples-v1.tsv"

func TestBuildDeckSentencesAttachesExample(t *testing.T) {
	entries := []starterdeck.LemmaEntry{
		{Lemma: "olla", POS: "VERB", ReprForm: "on"},
		{Lemma: "kissa", POS: "NOUN", ReprForm: "kissa"},
	}
	examples := map[starterdeck.LemmaKey]starterdeck.Example{
		{Lemma: "olla", POS: "VERB"}: {Form: "oli", Sentence: "Kissa oli pöydän alla."},
	}

	sentences, withExample := buildDeckSentences(entries, examples)
	if withExample != 1 {
		t.Fatalf("withExample=%d want 1", withExample)
	}
	if len(sentences) != 2 {
		t.Fatalf("want 2 sentences, got %d", len(sentences))
	}

	// olla: real corpus sentence, occurrence uses the inflected form "oli" at
	// its token index so the review card highlights the real inflection.
	olla := sentences[0]
	if olla.Text != "Kissa oli pöydän alla." {
		t.Fatalf("olla sentence text=%q, want corpus sentence", olla.Text)
	}
	if len(olla.Tokens) != 1 || olla.Tokens[0].Form != "oli" || olla.Tokens[0].TokenIx != 1 {
		t.Fatalf("olla token=%+v want form=oli ix=1", olla.Tokens)
	}
	if olla.Tokens[0].Lemma != "olla" || olla.Tokens[0].POS != "VERB" {
		t.Fatalf("olla token lemma/pos=%+v", olla.Tokens[0])
	}

	// kissa: no example -> falls back to the representative-form one-token
	// sentence, exactly as the pre-examples behaviour did.
	kissa := sentences[1]
	if kissa.Text != "kissa" || len(kissa.Tokens) != 1 || kissa.Tokens[0].Form != "kissa" {
		t.Fatalf("kissa fallback=%+v want repr-form sentence", kissa)
	}
}

func TestBuildDeckSentencesFallsBackWhenFormAbsent(t *testing.T) {
	// A corrupt example whose form is not in its sentence must not seed a card
	// with an absent highlight; it falls back to the representative form.
	entries := []starterdeck.LemmaEntry{{Lemma: "olla", POS: "VERB", ReprForm: "on"}}
	examples := map[starterdeck.LemmaKey]starterdeck.Example{
		{Lemma: "olla", POS: "VERB"}: {Form: "oli", Sentence: "Kissa istui pöydän alla."},
	}
	sentences, withExample := buildDeckSentences(entries, examples)
	if withExample != 0 {
		t.Fatalf("withExample=%d want 0 (form absent -> fallback)", withExample)
	}
	if len(sentences) != 1 || sentences[0].Text != "on" {
		t.Fatalf("want fallback repr-form sentence, got %+v", sentences)
	}
}

// newSeedTestDB opens a temp-file SQLite DB (never the shared finnestdb.db)
// with an admin owner already created, mirroring how an operator runs
// seedcolddeck against a real deployment DB.
func newSeedTestDB(t *testing.T) (*store.DB, *store.User) {
	t.Helper()
	t.Setenv("FINNESTDB_ADMIN_EMAILS", "admin@example.com")
	dbPath := t.TempDir() + "/seed-test.db"
	db, err := store.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	owner, err := db.CreateUser("admin@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if !owner.IsAdmin {
		t.Fatalf("owner.IsAdmin = false, want true (FINNESTDB_ADMIN_EMAILS not honored)")
	}
	return db, owner
}

// TestHandleExistingOfficialDeckReplaceDeletesPriorDeck proves the -replace
// path: a prior official deck with the same title+lang is deleted (through
// DeleteDeck, so occurrence/sentences/subscriptions go with it) before the
// caller reseeds, so re-running seedcolddeck with -replace never leaves two
// "Top 1000 Finnish words" decks around - the operator pain this flag exists
// to fix.
func TestHandleExistingOfficialDeckReplaceDeletesPriorDeck(t *testing.T) {
	db, owner := newSeedTestDB(t)

	priorID, err := db.CreateDeckWithSentencesOptions(owner.ID, "Top 1000 Finnish words", "FI", true, []store.DeckSentenceInput{
		{Text: "kissa", Tokens: []store.DeckTokenInput{{TokenIx: 0, Form: "kissa", Lemma: "kissa", POS: "NOUN"}}},
	})
	if err != nil {
		t.Fatalf("seed prior deck: %v", err)
	}

	if err := handleExistingOfficialDeck(db, owner.ID, "Top 1000 Finnish words", "FI", owner.Email, true); err != nil {
		t.Fatalf("handleExistingOfficialDeck: %v", err)
	}

	if _, err := db.FindOfficialDeckByTitle(owner.ID, "Top 1000 Finnish words", "FI"); err == nil {
		t.Fatalf("prior deck %d still exists after -replace", priorID)
	}
	decks, err := db.GetUserDecks(owner.ID)
	if err != nil {
		t.Fatalf("GetUserDecks: %v", err)
	}
	if len(decks) != 0 {
		t.Fatalf("decks after replace = %+v, want none (prior deck deleted, no reseed happened yet)", decks)
	}
}

// TestHandleExistingOfficialDeckWithoutReplaceWarnsAndKeepsDuplicateRisk
// proves the default (non -replace) path leaves the prior deck untouched -
// only a loud warning is logged - matching the documented "print a loud
// warning" behavior so operators who forget the flag are told immediately
// instead of silently ending up with the FIN duplicate-deck bug this task
// fixes.
func TestHandleExistingOfficialDeckWithoutReplaceWarnsAndKeepsDuplicateRisk(t *testing.T) {
	db, owner := newSeedTestDB(t)

	priorID, err := db.CreateDeckWithSentencesOptions(owner.ID, "Top 1000 Finnish words", "FI", true, []store.DeckSentenceInput{
		{Text: "kissa", Tokens: []store.DeckTokenInput{{TokenIx: 0, Form: "kissa", Lemma: "kissa", POS: "NOUN"}}},
	})
	if err != nil {
		t.Fatalf("seed prior deck: %v", err)
	}

	if err := handleExistingOfficialDeck(db, owner.ID, "Top 1000 Finnish words", "FI", owner.Email, false); err != nil {
		t.Fatalf("handleExistingOfficialDeck: %v", err)
	}

	stillID, err := db.FindOfficialDeckByTitle(owner.ID, "Top 1000 Finnish words", "FI")
	if err != nil {
		t.Fatalf("FindOfficialDeckByTitle after non-replace run: %v", err)
	}
	if stillID != priorID {
		t.Fatalf("existing deck id changed: got %d, want untouched %d", stillID, priorID)
	}
}

// TestHandleExistingOfficialDeckNoPriorDeckIsANoop proves the common case -
// first-ever run, no prior deck of this title - takes neither branch and
// returns cleanly regardless of -replace.
func TestHandleExistingOfficialDeckNoPriorDeckIsANoop(t *testing.T) {
	db, owner := newSeedTestDB(t)

	for _, replace := range []bool{false, true} {
		if err := handleExistingOfficialDeck(db, owner.ID, "Top 1000 Finnish words", "FI", owner.Email, replace); err != nil {
			t.Fatalf("handleExistingOfficialDeck(replace=%v): %v", replace, err)
		}
	}
	decks, err := db.GetUserDecks(owner.ID)
	if err != nil {
		t.Fatalf("GetUserDecks: %v", err)
	}
	if len(decks) != 0 {
		t.Fatalf("decks = %+v, want none created by handleExistingOfficialDeck itself", decks)
	}
}

// TestSeedWithExamplesArtifactReachesReviewPayload is the end-to-end proof
// that the real, checked-in fi-examples-v1.tsv (the artifact
// docs/DEPLOYMENT.md and docs/FOR_MICHAEL.md tell operators to pass via
// -examples) survives the full seedcolddeck pipeline - TopLemmas ranking,
// LoadExamples, buildDeckSentences, CreateDeckWithSentencesOptions - and
// comes back out of GetNextReviewCard (what /api/review/next serves) as the
// card's sentence text with the corpus-inflected surface form, not just the
// bare lemma. Uses a temp-file DB seeded with only the forms/lemmas this test
// needs, never the shared finnestdb.db.
func TestSeedWithExamplesArtifactReachesReviewPayload(t *testing.T) {
	db, owner := newSeedTestDB(t)

	// Minimal dictionary so TopLemmas' BatchLookupForms resolves "on" (a
	// surface form of "olla") to lemma=olla/POS=VERB, matching the first
	// example row in fi-examples-v1.tsv.
	if err := db.InsertLemmaForTest("olla", "VERB", "to be", "FI"); err != nil {
		t.Fatalf("InsertLemmaForTest: %v", err)
	}
	if err := db.InsertFormForTest("on", "olla", "VERB", "FI"); err != nil {
		t.Fatalf("InsertFormForTest: %v", err)
	}

	freqList := strings.NewReader("on 500\n")
	entries, _, err := starterdeck.TopLemmas(freqList, db, "FI", 10)
	if err != nil {
		t.Fatalf("TopLemmas: %v", err)
	}
	if len(entries) != 1 || entries[0].Lemma != "olla" {
		t.Fatalf("entries=%+v, want a single olla/VERB entry", entries)
	}

	exampleByLemma, err := starterdeck.LoadExamples(fiExamplesArtifact, "FI")
	if err != nil {
		t.Fatalf("LoadExamples(%s): %v", fiExamplesArtifact, err)
	}
	wantExample, ok := exampleByLemma[starterdeck.LemmaKey{Lemma: "olla", POS: "VERB"}]
	if !ok {
		t.Fatalf("fixture regression: %s has no olla/VERB example", fiExamplesArtifact)
	}

	sentences, withExample := buildDeckSentences(entries, exampleByLemma)
	if withExample != 1 {
		t.Fatalf("withExample=%d, want 1 (the seeded olla entry should carry the curated example)", withExample)
	}

	deckID, err := db.CreateDeckWithSentencesOptions(owner.ID, "Top 10 Finnish words", "FI", true, sentences)
	if err != nil {
		t.Fatalf("CreateDeckWithSentencesOptions: %v", err)
	}

	// Fetch the review payload exactly as GET /api/review/next does
	// (HandleReviewNext wraps this same call).
	card, err := db.GetNextReviewCard(owner.ID, &deckID, "FI")
	if err != nil {
		t.Fatalf("GetNextReviewCard: %v", err)
	}
	if card == nil {
		t.Fatal("GetNextReviewCard returned nil - expected the seeded olla card")
	}
	if card.SentenceText != wantExample.Sentence {
		t.Fatalf("card.SentenceText=%q, want the curated corpus sentence %q", card.SentenceText, wantExample.Sentence)
	}
	if card.Surface != wantExample.Form {
		t.Fatalf("card.Surface=%q, want the inflected occurrence form %q", card.Surface, wantExample.Form)
	}
	if !strings.Contains(card.SentenceText, card.Surface) {
		t.Fatalf("sentence %q does not contain its own highlighted surface %q", card.SentenceText, card.Surface)
	}
	if card.Lemma != "olla" || card.POS != "VERB" {
		t.Fatalf("card sense=%s/%s, want olla/VERB", card.Lemma, card.POS)
	}
}
