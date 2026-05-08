package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhase4WriteSentencesUserFriendly(t *testing.T) {
	derived := t.TempDir()
	if err := os.Mkdir(filepath.Join(derived, "mining"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := testSentenceExportState()
	if err := s.Phase4Write(derived, time.Unix(0, 0).UTC()); err != nil {
		t.Fatal(err)
	}

	canonical := readTestTSV(t, filepath.Join(derived, "sentences.tsv"))
	if got, want := len(canonical), 5; got != want {
		t.Fatalf("sentences.tsv rows = %d, want %d", got, want)
	}
	friendly := readTestTSV(t, filepath.Join(derived, "sentences_user_friendly.tsv"))
	if got, want := len(friendly), 3; got != want {
		t.Fatalf("sentences_user_friendly.tsv rows = %d, want %d: %#v", got, want, friendly)
	}
	assertTexts(t, friendly, []string{
		"See on hea eestikeelne näitelause.",
		"He won the copyright case.",
	})
}

func TestPhase4WriteScratchSentencesUserFriendly(t *testing.T) {
	derived := t.TempDir()
	if err := os.Mkdir(filepath.Join(derived, "mining"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := testSentenceExportState()
	if err := s.openScratch(derived); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.flushSentencesToScratch(); err != nil {
		t.Fatal(err)
	}
	if err := s.Phase4Write(derived, time.Unix(0, 0).UTC()); err != nil {
		t.Fatal(err)
	}

	canonical := readTestTSV(t, filepath.Join(derived, "sentences.tsv"))
	if got, want := len(canonical), 5; got != want {
		t.Fatalf("sentences.tsv rows = %d, want %d", got, want)
	}
	friendly := readTestTSV(t, filepath.Join(derived, "sentences_user_friendly.tsv"))
	if got, want := len(friendly), 3; got != want {
		t.Fatalf("sentences_user_friendly.tsv rows = %d, want %d: %#v", got, want, friendly)
	}
	assertTexts(t, friendly, []string{
		"See on hea eestikeelne näitelause.",
		"He won the copyright case.",
	})
}

func testSentenceExportState() *state {
	sentences := []string{
		"See on hea eestikeelne näitelause.",
		"Sisukord",
		"Peatükk 1",
		"He won the copyright case.",
	}
	s := &state{
		langLower: "et",
		surfaces:  map[string]*surfaceStats{},
		sentences: map[string]*sentenceRec{},
		documents: map[string]*documentRec{},
	}
	for i, text := range sentences {
		hash := sentenceHash(text)
		s.sentences[hash] = &sentenceRec{hash: hash, text: text}
		s.sentenceOrder = append(s.sentenceOrder, hash)
		s.occurrences = append(s.occurrences, sentenceOccurrence{
			hash:       hash,
			source:     "fixture",
			documentID: "fixture:sentences",
			sentenceIx: i,
		})
	}
	return s
}

func readTestTSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func assertTexts(t *testing.T, rows [][]string, want []string) {
	t.Helper()
	if len(rows) != len(want)+1 {
		t.Fatalf("rows = %d, want header + %d", len(rows), len(want))
	}
	for i, text := range want {
		if got := rows[i+1][2]; got != text {
			t.Fatalf("row %d text = %q, want %q", i+1, got, text)
		}
	}
}
