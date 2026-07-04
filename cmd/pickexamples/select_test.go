package main

import "testing"

func TestAcceptableRejectsArtifacts(t *testing.T) {
	cases := []struct {
		name   string
		cand   Candidate
		want   bool
		reason string
	}{
		{
			name: "clean sentence",
			cand: Candidate{Text: "Kissa istui pöydän alla hiljaa.", Form: "istui"},
			want: true,
		},
		{
			name:   "too short",
			cand:   Candidate{Text: "Hän tuli.", Form: "tuli"},
			want:   false,
			reason: "word-count",
		},
		{
			name:   "too long",
			cand:   Candidate{Text: "Hän tuli talon luo ja seisoi pihalla katsellen ikkunoita joissa paloi lämmin valo aivan hiljaa.", Form: "tuli"},
			want:   false,
			reason: "word-count",
		},
		{
			name:   "has digit",
			cand:   Candidate{Text: "Kello oli tuli viisi 30 aamulla.", Form: "tuli"},
			want:   false,
			reason: "has-digit",
		},
		{
			name:   "has url",
			cand:   Candidate{Text: "Katso lisää osoitteesta www.example.com heti.", Form: "osoitteesta"},
			want:   false,
			reason: "has-url",
		},
		{
			name:   "all caps token",
			cand:   Candidate{Text: "Tämä tuli oli TÄRKEÄ viesti kaikille.", Form: "tuli"},
			want:   false,
			reason: "all-caps",
		},
		{
			name:   "leading dash subtitle",
			cand:   Candidate{Text: "- Tuli syttyi nopeasti pimeässä metsässä.", Form: "tuli"},
			want:   false,
			reason: "leading-dash",
		},
		{
			name:   "speaker colon subtitle",
			cand:   Candidate{Text: "Anna: tuli syttyi nopeasti metsässä.", Form: "tuli"},
			want:   false,
			reason: "speaker-colon",
		},
		{
			name:   "no terminal punctuation",
			cand:   Candidate{Text: "Tuli syttyi nopeasti pimeässä metsässä", Form: "tuli"},
			want:   false,
			reason: "no-terminal-punct",
		},
		{
			name:   "not capitalized",
			cand:   Candidate{Text: "tuli syttyi nopeasti pimeässä metsässä.", Form: "tuli"},
			want:   false,
			reason: "not-capitalized",
		},
		{
			name:   "unbalanced quote",
			cand:   Candidate{Text: "Hän sanoi \"tuli on kuuma ja vaarallinen.", Form: "tuli"},
			want:   false,
			reason: "unbalanced-quote",
		},
		{
			name:   "form absent",
			cand:   Candidate{Text: "Kissa istui pöydän alla hiljaa.", Form: "tuli"},
			want:   false,
			reason: "form-absent",
		},
		{
			name:   "dialogue join",
			cand:   Candidate{Text: "Tuli syttyi metsässä.- Sinulla on tulta.", Form: "tuli"},
			want:   false,
			reason: "dialogue-join",
		},
		{
			name:   "mid-word cap garble",
			cand:   Candidate{Text: "Näyttää aIka pelottavalta, tuli syttyi metsässä.", Form: "tuli"},
			want:   false,
			reason: "mid-word-cap",
		},
		{
			name:   "garbled target form",
			cand:   Candidate{Text: "Heidän eiNsä tarkoittaa kyllä vain sitä.", Form: "eiNsä"},
			want:   false,
			reason: "mid-word-cap", // caught by the sentence-level gate first
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// nil freqRanks: exercise every gate except the foreign-language one.
			got, reason := acceptable(tc.cand, nil)
			if got != tc.want {
				t.Fatalf("acceptable=%v (%s), want %v", got, reason, tc.want)
			}
			if !got && tc.reason != "" && reason != tc.reason {
				t.Fatalf("reason=%q want %q", reason, tc.reason)
			}
		})
	}
}

func TestAcceptableRejectsForeignLanguage(t *testing.T) {
	// An Estonian-baseline vocabulary where the English sentence's words are
	// absent, but the target form "me" and its Estonian neighbours are present.
	freq := map[string]int{"me": 1, "aitame": 2, "üksteist": 3, "kodutööde": 4, "tegemisel": 5}
	foreign := Candidate{Text: "My teachers like me a lot.", Form: "me"}
	if ok, reason := acceptable(foreign, freq); ok || reason != "foreign-language" {
		t.Fatalf("expected foreign-language rejection, got ok=%v reason=%q", ok, reason)
	}
	native := Candidate{Text: "Me aitame üksteist kodutööde tegemisel.", Form: "me"}
	if ok, reason := acceptable(native, freq); !ok {
		t.Fatalf("native sentence should pass, got reason=%q", reason)
	}
}

func TestScorePrefersNonInitialForm(t *testing.T) {
	// Same surrounding words; the sentence where the form is not sentence-
	// initial must score higher (it demonstrates inflection in context).
	freq := map[string]int{"kissa": 1, "istui": 2, "pöydän": 3, "alla": 4, "hiljaa": 5, "tuli": 6}
	initial := Candidate{Text: "Tuli kissa istui pöydän alla.", Form: "tuli"}
	nonInitial := Candidate{Text: "Kissa tuli istui pöydän alla.", Form: "tuli"}
	if score(nonInitial, freq) <= score(initial, freq) {
		t.Fatalf("non-initial form should outscore initial: non=%.2f init=%.2f",
			score(nonInitial, freq), score(initial, freq))
	}
}

func TestScorePrefersReadableSurroundings(t *testing.T) {
	// Both non-initial; the one with more common surrounding words wins.
	freq := map[string]int{
		"kissa": 1, "istui": 2, "pöydän": 3, "alla": 4, "tuli": 5,
		"harvinaisuus": 40000, "epätavallinen": 45000, "monimutkainen": 48000,
	}
	common := Candidate{Text: "Kissa tuli istui pöydän alla.", Form: "tuli"}
	rare := Candidate{Text: "Harvinaisuus tuli epätavallinen monimutkainen alla.", Form: "tuli"}
	if score(common, freq) <= score(rare, freq) {
		t.Fatalf("common-word sentence should outscore rare-word one")
	}
}

func TestPickBestDedupsAndCaps(t *testing.T) {
	freq := map[string]int{"kissa": 1, "istui": 2, "pöydän": 3, "alla": 4, "hiljaa": 5, "nukkui": 6, "tuli": 7}
	cands := []Candidate{
		{SentenceID: 1, Text: "Kissa tuli istui pöydän alla.", Form: "tuli", FormCount: 100},
		{SentenceID: 2, Text: "kissa tuli istui pöydän alla.", Form: "tuli", FormCount: 90}, // dup after norm+lowercase? no: id1 is capitalized. Distinct text but normalized differs only by first letter case -> treated as dup
		{SentenceID: 3, Text: "Koira tuli nukkui pöydän alla hiljaa.", Form: "tuli", FormCount: 80},
	}
	got := pickBest(cands, freq, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 picks, got %d: %+v", len(got), got)
	}
	// The two picks must be distinct normalized sentences.
	if normText(got[0].Text) == normText(got[1].Text) {
		t.Fatalf("picks are duplicates: %q / %q", got[0].Text, got[1].Text)
	}
}

func TestPickBestSkipsUnacceptable(t *testing.T) {
	cands := []Candidate{
		{SentenceID: 1, Text: "Hän tuli.", Form: "tuli"},                 // too short
		{SentenceID: 2, Text: "- Tuli syttyi metsässä.", Form: "tuli"},   // leading dash + short
		{SentenceID: 3, Text: "Kissa tuli istui pöydän alla.", Form: "tuli"}, // good
	}
	got := pickBest(cands, nil, 2)
	if len(got) != 1 || got[0].SentenceID != 3 {
		t.Fatalf("want only sentence 3, got %+v", got)
	}
}
