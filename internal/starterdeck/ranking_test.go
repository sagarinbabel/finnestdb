package starterdeck

import (
	"strings"
	"testing"

	"finnestdb/internal/store"
)

// fakeResolver maps forms to fixed resolutions, standing in for the
// dictionary so the aggregation logic is testable without finnestdb.db.
type fakeResolver map[string]store.FormResolution

func (f fakeResolver) BatchLookupForms(forms []string, lang, parserMode string) map[string]store.FormResolution {
	out := make(map[string]store.FormResolution)
	for _, form := range forms {
		if res, ok := f[form]; ok {
			out[form] = res
		}
	}
	return out
}

func TestTopLemmasAggregatesInflectedForms(t *testing.T) {
	// olla appears as three forms whose counts must sum (300); kissa has one
	// form (150); "zzz" is unresolved; "123" and "?" are filtered before
	// lookup; "pekka" resolves to a PROPN and must be skipped; "sata"
	// resolves but should be cut by n=2.
	list := strings.Join([]string{
		"on 200",
		"kissa 150",
		"olen 60",
		"oli 40",
		"zzz 30",
		"123 25",
		"? 20",
		"pekka 15",
		"sata 10",
	}, "\n")
	resolver := fakeResolver{
		"on":    {Lemma: "olla", POS: "VERB"},
		"olen":  {Lemma: "olla", POS: "VERB"},
		"oli":   {Lemma: "olla", POS: "VERB"},
		"kissa": {Lemma: "kissa", POS: "NOUN"},
		"pekka": {Lemma: "Pekka", POS: "PROPN"},
		"sata":  {Lemma: "sata", POS: "NUM"},
	}

	entries, skipped, err := TopLemmas(strings.NewReader(list), resolver, "FI", 2)
	if err != nil {
		t.Fatalf("TopLemmas: %v", err)
	}
	if skipped != 2 {
		t.Fatalf("skipped=%d want 2 (unresolved zzz + PROPN pekka)", skipped)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%+v want 2", entries)
	}
	// olla outranks kissa because its forms sum to 300 > 150 even though no
	// single olla form beats kissa's 150... (200 does, but the sum is the
	// ranking contract this test pins down: 60+40 alone would not).
	if entries[0].Lemma != "olla" || entries[0].Count != 300 || entries[0].ReprForm != "on" {
		t.Fatalf("entries[0]=%+v want olla count=300 repr=on", entries[0])
	}
	if entries[1].Lemma != "kissa" || entries[1].Count != 150 {
		t.Fatalf("entries[1]=%+v want kissa count=150", entries[1])
	}
}

func TestTopLemmasRepresentativeFormIsMostFrequent(t *testing.T) {
	// The first-seen (= highest-count) form becomes the representative, not
	// a later rarer inflection.
	list := "sanoi 90\nsanoo 80\n"
	resolver := fakeResolver{
		"sanoi": {Lemma: "sanoa", POS: "VERB"},
		"sanoo": {Lemma: "sanoa", POS: "VERB"},
	}
	entries, _, err := TopLemmas(strings.NewReader(list), resolver, "FI", 10)
	if err != nil {
		t.Fatalf("TopLemmas: %v", err)
	}
	if len(entries) != 1 || entries[0].ReprForm != "sanoi" || entries[0].Count != 170 {
		t.Fatalf("entries=%+v want single sanoa repr=sanoi count=170", entries)
	}
}
