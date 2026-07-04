package catalog

import "testing"

// The checked-in catalog is a shipped product artifact, so these tests guard
// its integrity: every entry must resolve to a readable fixture, carry a
// pending difficulty review (no entry may claim human sign-off it did not
// get), and expose non-empty license provenance (license discipline).

func TestEmbeddedCatalogLoads(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Entries) == 0 {
		t.Fatal("catalog has no entries")
	}
}

func TestEveryEntryHasReadableFixtureAndProvenance(t *testing.T) {
	for _, e := range Entries() {
		if e.ID == "" || e.Language == "" || e.Title == "" {
			t.Errorf("entry missing id/language/title: %+v", e)
		}
		if e.Language != "fi" && e.Language != "et" {
			t.Errorf("%s: unexpected language %q", e.ID, e.Language)
		}
		if e.License == "" || e.CorpusSource == "" {
			t.Errorf("%s: missing license/corpus provenance (license discipline)", e.ID)
		}
		switch e.Language {
		case "fi":
			// FI difficulty labels were human-reviewed 2026-07-04 (reviews.json).
			if e.DifficultyReview != "approved" || e.DifficultyReviewBy == "" || e.DifficultyReviewDate == "" {
				t.Errorf("%s: FI entries must carry an approved human review, got %q by %q", e.ID, e.DifficultyReview, e.DifficultyReviewBy)
			}
			if e.DifficultyComputed == "" {
				t.Errorf("%s: computed difficulty must be preserved alongside the review", e.ID)
			}
		default:
			// ET is still awaiting its reviewer; no entry may claim a sign-off
			// it did not get.
			if e.DifficultyReview != "pending" {
				t.Errorf("%s: difficulty_review = %q, want pending (ET human sanity-check has not happened)", e.ID, e.DifficultyReview)
			}
		}
		if len(e.Lemmas) == 0 {
			t.Errorf("%s: precomputed lemma list is empty", e.ID)
		}
		text, err := Text(e.ID)
		if err != nil {
			t.Errorf("%s: fixture unreadable: %v", e.ID, err)
			continue
		}
		if text == "" {
			t.Errorf("%s: fixture text is empty", e.ID)
		}
	}
}

func TestTextUnknownID(t *testing.T) {
	if _, err := Text("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown id")
	}
}

func TestCoverageIntersection(t *testing.T) {
	e := Entry{
		Lemmas: []LemmaCount{
			{Lemma: "kissa", POS: "NOUN", Count: 3},
			{Lemma: "juoda", POS: "VERB", Count: 1},
			{Lemma: "aurinko", POS: "NOUN", Count: 1},
			{Lemma: "ja", POS: "CCONJ", Count: 5},
		},
	}
	known := []KnownLemma{
		{Lemma: "Kissa", POS: "NOUN"}, // case-insensitive lemma match
		{Lemma: "ja", POS: "CCONJ"},
		{Lemma: "koira", POS: "NOUN"}, // not in text
	}
	frac, matched, total := e.Coverage(known)
	if total != 4 || matched != 2 {
		t.Fatalf("matched=%d total=%d, want 2/4", matched, total)
	}
	if frac != 0.5 {
		t.Fatalf("fraction = %.3f, want 0.5", frac)
	}
}

func TestCoveragePOSSensitive(t *testing.T) {
	// A known lemma with a different POS must not count as coverage: the
	// catalog keys on (lemma, pos), so kuusi/NOUN known does not cover
	// kuusi/NUM in the text.
	e := Entry{Lemmas: []LemmaCount{{Lemma: "kuusi", POS: "NUM", Count: 1}}}
	known := []KnownLemma{{Lemma: "kuusi", POS: "NOUN"}}
	_, matched, total := e.Coverage(known)
	if matched != 0 || total != 1 {
		t.Fatalf("POS-mismatched known should not match: matched=%d total=%d", matched, total)
	}
}

func TestCoverageNoLemmas(t *testing.T) {
	e := Entry{}
	frac, matched, total := e.Coverage([]KnownLemma{{Lemma: "x", POS: "NOUN"}})
	if frac != 0 || matched != 0 || total != 0 {
		t.Fatalf("empty-lemma coverage should be zero, got %.3f %d %d", frac, matched, total)
	}
}
