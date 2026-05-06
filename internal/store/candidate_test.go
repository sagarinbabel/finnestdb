package store

import (
	"testing"
)

// TestFormCandidate_AsResolution confirms the candidate→resolution
// downcast preserves all the back-compat fields.
func TestFormCandidate_AsResolution(t *testing.T) {
	c := FormCandidate{
		Lemma:          "talo",
		POS:            "NOUN",
		Feats:          "Case=Ine|Number=Sing",
		GrammarLabel:   "inessive",
		Source:         "fst",
		SourcePriority: 30,
		Confidence:     0.85,
	}
	r := c.AsResolution()
	if r.Lemma != "talo" || r.POS != "NOUN" {
		t.Errorf("lemma/pos: got {%q %q}", r.Lemma, r.POS)
	}
	if r.Feats != "Case=Ine|Number=Sing" {
		t.Errorf("feats: got %q", r.Feats)
	}
	if r.GrammarLabel != "inessive" {
		t.Errorf("grammar_label: got %q", r.GrammarLabel)
	}
	if r.Source != "fst" {
		t.Errorf("source: got %q", r.Source)
	}
}

// TestCandidateFromResolution confirms the resolution→candidate upcast
// fills the standard fields and leaves Phase 2/3 fields unset.
func TestCandidateFromResolution(t *testing.T) {
	r := FormResolution{
		Lemma:        "kohv",
		POS:          "NOUN",
		Feats:        "Case=Par|Number=Sing",
		GrammarLabel: "partitive",
		Source:       "dict",
	}
	c := candidateFromResolution(r)
	if c.Lemma != "kohv" || c.POS != "NOUN" {
		t.Errorf("lemma/pos: got {%q %q}", c.Lemma, c.POS)
	}
	if c.Feats != "Case=Par|Number=Sing" {
		t.Errorf("feats: got %q", c.Feats)
	}
	if c.SourcePriority != 0 {
		t.Errorf("SourcePriority should be 0 (Phase 2 fills it); got %d", c.SourcePriority)
	}
	if c.Confidence != 0 {
		t.Errorf("Confidence should be 0 (Phase 3 fills it); got %v", c.Confidence)
	}
}

// TestBatchLookupAllCandidates_WrapsBatchLookupForms verifies the
// Phase 1 implementation is a faithful wrapper — every form resolved
// by BatchLookupForms appears with one candidate in the multi-candidate
// API, and forms BatchLookupForms drops are also dropped here.
func TestBatchLookupAllCandidates_WrapsBatchLookupForms(t *testing.T) {
	db := newTestDB(t)
	seedForms(t, db, [][4]string{
		{"talossa", "talo", "NOUN", "FI"},
		{"kohvi", "kohv", "NOUN", "ET"},
	})

	got := db.BatchLookupAllCandidates([]string{"talossa", "kohvi", "missing"}, "FI", "basic")
	if len(got) != 1 {
		t.Fatalf("expected 1 form (talossa) resolved for FI, got %d (keys=%v)", len(got), got)
	}
	if cands, ok := got["talossa"]; !ok || len(cands) != 1 {
		t.Fatalf("talossa: expected 1 candidate, got %v", cands)
	}
	if got["talossa"][0].Lemma != "talo" {
		t.Errorf("talossa lemma: got %q", got["talossa"][0].Lemma)
	}
	if _, ok := got["missing"]; ok {
		t.Errorf("missing form should be absent from result map")
	}
}
