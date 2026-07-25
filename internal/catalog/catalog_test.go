package catalog

import (
	"encoding/json"
	"os"
	"testing"
)

// The checked-in catalog is a shipped product artifact, so these tests guard
// its integrity: every entry must resolve to a readable fixture, expose
// non-empty license provenance (license discipline), and carry an approved
// human review if and only if reviews.json signed it off - no entry may claim
// a sign-off it did not get, and none may hide one it did.

// reviewedIDs returns the set of entry ids that reviews.json signs off on. The
// review pin is derived from the same source the generator uses, so it stays
// honest automatically as texts are added or replaced (a new pending text does
// not need a test edit).
func reviewedIDs(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("reviews.json")
	if err != nil {
		t.Fatalf("read reviews.json: %v", err)
	}
	var f struct {
		Reviews map[string]json.RawMessage `json:"reviews"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse reviews.json: %v", err)
	}
	ids := make(map[string]bool, len(f.Reviews))
	for id := range f.Reviews {
		ids[id] = true
	}
	return ids
}

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
	reviewed := reviewedIDs(t)
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
		// The review pin is honest by construction: an entry is approved (with
		// reviewer metadata) exactly when reviews.json signed it off, and
		// pending otherwise. A pending text must never leak reviewer fields.
		if reviewed[e.ID] {
			if e.DifficultyReview != "approved" || e.DifficultyReviewBy == "" || e.DifficultyReviewDate == "" {
				t.Errorf("%s: reviews.json signs this off, so it must be approved with reviewer metadata, got %q by %q", e.ID, e.DifficultyReview, e.DifficultyReviewBy)
			}
			if e.DifficultyComputed == "" {
				t.Errorf("%s: computed difficulty must be preserved alongside the review", e.ID)
			}
		} else {
			if e.DifficultyReview != "pending" {
				t.Errorf("%s: difficulty_review = %q, want pending (no reviews.json sign-off)", e.ID, e.DifficultyReview)
			}
			if e.DifficultyReviewBy != "" || e.DifficultyReviewDate != "" {
				t.Errorf("%s: pending entry must not carry reviewer metadata (by=%q date=%q)", e.ID, e.DifficultyReviewBy, e.DifficultyReviewDate)
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
