package lexadverbs

import "testing"

func TestLookupFI_Hits(t *testing.T) {
	// Each case asserts the curated lemma/POS combination the overlay
	// must ship. Every surface here is a known FST-output bug from
	// the yle_subs deck builder.
	cases := []struct {
		surface, wantLemma, wantPOS, wantFeats string
	}{
		{"tuskin", "tuskin", "ADV", ""},
		{"Tuskin", "tuskin", "ADV", ""},   // sentence-initial capital
		{"varsin", "varsin", "ADV", ""},   // not varsi/NOUN/Case=Ade
		{"yleensä", "yleensä", "ADV", ""}, // not ylä-/NOUN/Case=Ill
		{"Yleensä", "yleensä", "ADV", ""},
		{"sisään", "sisään", "ADV", ""},                                // not sisä-/NOUN/Case=Ill
		{"enemmän", "paljon", "ADV", "Degree=Cmp"},                     // lemma is paljon, not paljo
		{"tarpeeksi", "tarpeeksi", "ADV", ""},
		{"perillä", "perillä", "ADV", ""},
		{"peräisin", "peräisin", "ADV", ""},
		{"vahingossa", "vahingossa", "ADV", ""},                        // not vahinko/NOUN/Case=Ine
		{"asiaan", "asia", "NOUN", "Case=Ill|Number=Sing"},             // not as/NOUN
		{"kotona", "koti", "NOUN", "Case=Ess|Number=Sing"},
	}
	for _, tc := range cases {
		analyses, ok := LookupFI(tc.surface)
		if !ok {
			t.Errorf("LookupFI(%q): expected hit, got miss", tc.surface)
			continue
		}
		if len(analyses) != 1 {
			t.Errorf("LookupFI(%q): want 1 analysis, got %d", tc.surface, len(analyses))
			continue
		}
		a := analyses[0]
		if a.Lemma != tc.wantLemma {
			t.Errorf("LookupFI(%q).Lemma = %q, want %q", tc.surface, a.Lemma, tc.wantLemma)
		}
		if a.UPOS != tc.wantPOS {
			t.Errorf("LookupFI(%q).UPOS = %q, want %q", tc.surface, a.UPOS, tc.wantPOS)
		}
		if a.Feats != tc.wantFeats {
			t.Errorf("LookupFI(%q).Feats = %q, want %q", tc.surface, a.Feats, tc.wantFeats)
		}
	}
}

func TestLookupFI_Misses(t *testing.T) {
	// Productive forms the FST already gets right must NOT be shadowed.
	// If you find yourself adding one of these to the overlay, you're
	// shadowing the parser instead of fixing it — push back into the
	// FST or document why the overlay is the right layer.
	misses := []string{
		"",         // empty
		"pankki",   // bare noun, no shadowing needed
		"menemään", // MA-infinitive — handled by udfeats.NormalizeMaInfinitive, not here
		"talossa",  // ordinary inessive
		"juoksen",  // ordinary verb
		"hyvä",     // bare adjective
	}
	for _, s := range misses {
		if _, ok := LookupFI(s); ok {
			t.Errorf("LookupFI(%q) hit overlay; should miss (productive form)", s)
		}
	}
}

func TestLookupFI_ClonesSlice(t *testing.T) {
	// A caller mutating the returned slice must not corrupt the shared
	// overlay. This is the contract for any subsequent call returning
	// the same surface.
	first, ok := LookupFI("tuskin")
	if !ok || len(first) != 1 {
		t.Fatalf("setup: LookupFI(\"tuskin\") miss or wrong shape")
	}
	first[0].Lemma = "MUTATED"
	second, ok := LookupFI("tuskin")
	if !ok || len(second) != 1 {
		t.Fatalf("second LookupFI(\"tuskin\") miss or wrong shape")
	}
	if second[0].Lemma != "tuskin" {
		t.Errorf("second LookupFI(\"tuskin\").Lemma = %q; overlay mutated by prior caller", second[0].Lemma)
	}
}

func TestHasFI(t *testing.T) {
	if !HasFI("tuskin") {
		t.Error("HasFI(\"tuskin\") = false; want true")
	}
	if !HasFI("Tuskin") {
		t.Error("HasFI(\"Tuskin\") = false; want true (case-fold)")
	}
	if HasFI("pankki") {
		t.Error("HasFI(\"pankki\") = true; want false (not in overlay)")
	}
	if HasFI("") {
		t.Error("HasFI(\"\") = true; want false")
	}
}
