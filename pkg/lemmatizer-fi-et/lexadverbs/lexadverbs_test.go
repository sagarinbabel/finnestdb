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
		{"sisään", "sisään", "ADV", ""},            // not sisä-/NOUN/Case=Ill
		{"enemmän", "paljon", "ADV", "Degree=Cmp"}, // lemma is paljon, not paljo
		{"tarpeeksi", "tarpeeksi", "ADV", ""},
		{"perillä", "perillä", "ADV", ""},
		{"peräisin", "peräisin", "ADV", ""},
		{"vahingossa", "vahingossa", "ADV", ""},            // not vahinko/NOUN/Case=Ine
		{"asiaan", "asia", "NOUN", "Case=Ill|Number=Sing"}, // not as/NOUN
		{"kotona", "koti", "NOUN", "Case=Ess|Number=Sing"},
		{"sanoin", "sanoa", "VERB", "Mood=Ind|Number=Sing|Person=1|Tense=Past|VerbForm=Fin|Voice=Act"}, // not sana/NOUN
		{"Sanoin", "sanoa", "VERB", "Mood=Ind|Number=Sing|Person=1|Tense=Past|VerbForm=Fin|Voice=Act"},
		{"Maria", "Maria", "PROPN", "Number=Sing"},           // exact-case proper name, not mari/NOUN
		{"Norjan", "Norja", "PROPN", "Case=Gen|Number=Sing"}, // exact-case country name
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
	// shadowing the parser instead of fixing it - push back into the
	// FST or document why the overlay is the right layer.
	misses := []string{
		"",         // empty
		"pankki",   // bare noun, no shadowing needed
		"menemään", // MA-infinitive - handled by udfeats.NormalizeMaInfinitive, not here
		"talossa",  // ordinary inessive
		"juoksen",  // ordinary verb
		"hyvä",     // bare adjective
		"maria",    // lowercase homonym must not hit exact proper-name overlay
		"norjan",   // lowercase adjective form must not hit exact country-name overlay
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
	if !HasFI("Maria") {
		t.Error("HasFI(\"Maria\") = false; want true (exact proper-name overlay)")
	}
	if HasFI("maria") {
		t.Error("HasFI(\"maria\") = true; want false (exact overlay must not case-fold)")
	}
	if HasFI("pankki") {
		t.Error("HasFI(\"pankki\") = true; want false (not in overlay)")
	}
	if HasFI("") {
		t.Error("HasFI(\"\") = true; want false")
	}
}

func TestLookupET_Hits(t *testing.T) {
	// Each case asserts the curated lemma/POS combination the ET
	// overlay must ship. Every surface here is a known
	// productive-case-on-closed-class trap from the Vabamorf parser
	// (3,137 surfaces with ~38M occurrences identified in the
	// working ET corpus).
	cases := []struct {
		surface, wantLemma, wantPOS string
	}{
		{"välja", "välja", "ADV"},
		{"Välja", "välja", "ADV"},       // sentence-initial capital
		{"seal", "seal", "ADV"},         // not siga/NOUN/Case=Ade
		{"sisse", "sisse", "ADV"},       // not siss/NOUN/partitive plural
		{"veel", "veel", "ADV"},         // not vesi/NOUN/Case=Ade
		{"peale", "peale", "ADP"},       // not pea/NOUN/Case=All
		{"jaoks", "jaoks", "ADP"},       // not jagu/NOUN/Case=Tra
		{"Ta", "tema", "PRON"},          // not TA/NOUN or Ta/X
		{"ei", "ei", "ADV"},             // not nominal ei/NOUN or interjection fallback
		{"lihtsalt", "lihtsalt", "ADV"}, // not lihtne/ADJ/Case=Abl
		{"ma", "ma", "PRON"},            // not mA/MA unit or degree abbreviations
		{"Ma", "ma", "PRON"},
		{"tegelikult", "tegelikult", "ADV"},
		{"Tegelikult", "tegelikult", "ADV"},
		{"pärast", "pärast", "ADP"},
		{"eest", "eest", "ADP"},
		{"sina", "sina", "PRON"}, // not the source-language-only noun sense
		{"Sina", "sina", "PRON"},
		{"taga", "taga", "ADP"},
	}
	for _, tc := range cases {
		analyses, ok := LookupET(tc.surface)
		if !ok {
			t.Errorf("LookupET(%q): expected hit, got miss", tc.surface)
			continue
		}
		if len(analyses) != 1 {
			t.Errorf("LookupET(%q): want 1 analysis, got %d", tc.surface, len(analyses))
			continue
		}
		a := analyses[0]
		if a.Lemma != tc.wantLemma {
			t.Errorf("LookupET(%q).Lemma = %q, want %q", tc.surface, a.Lemma, tc.wantLemma)
		}
		if a.UPOS != tc.wantPOS {
			t.Errorf("LookupET(%q).UPOS = %q, want %q", tc.surface, a.UPOS, tc.wantPOS)
		}
	}
}

func TestLookupET_Misses(t *testing.T) {
	misses := []string{
		"",
		"pood",        // bare ET noun, no shadowing needed
		"linn",        // city
		"maja",        // house
		"sõpra",       // partitive noun, productive
		"raamatupoes", // long compound noun
		"tuskin",      // FI overlay key - must NOT hit the ET table
		"varsin",      // FI overlay key
	}
	for _, s := range misses {
		if _, ok := LookupET(s); ok {
			t.Errorf("LookupET(%q) hit overlay; should miss", s)
		}
	}
}

func TestFIAndETOverlaysAreIndependent(t *testing.T) {
	// The two overlays are different bug catalogues. A surface that
	// appears in one MUST NOT cross-contaminate lookups in the other.
	// Today there's no overlap, but a future addition could introduce
	// one accidentally - this test fails loudly when that happens so
	// the maintainer makes an explicit choice.
	for fiSurface := range fiOverlay {
		if _, ok := etOverlay[fiSurface]; ok {
			t.Errorf("surface %q appears in both fiOverlay and etOverlay; resolve which language owns it",
				fiSurface)
		}
	}
}

func TestHasET(t *testing.T) {
	if !HasET("välja") {
		t.Error("HasET(\"välja\") = false; want true")
	}
	if !HasET("Välja") {
		t.Error("HasET(\"Välja\") = false; want true (case-fold)")
	}
	if HasET("pood") {
		t.Error("HasET(\"pood\") = true; want false (not in overlay)")
	}
	if HasET("") {
		t.Error("HasET(\"\") = true; want false")
	}
}
