package main

import (
	"testing"

	"finnestdb/internal/starterdeck"
)

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
