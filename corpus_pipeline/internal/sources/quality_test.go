package sources

import (
	"reflect"
	"testing"
)

func TestQualityTier(t *testing.T) {
	if tier, ok := QualityTier("fi", "fixture"); !ok || tier != 0 {
		t.Errorf("fi fixture: got tier=%d ok=%v, want 0/true", tier, ok)
	}
	if tier, ok := QualityTier("fi", "yle-news-2022-2024"); !ok || tier != 1 {
		t.Errorf("fi yle: got tier=%d ok=%v, want 1/true", tier, ok)
	}
	if tier, ok := QualityTier("fi", "opus-ccmatrix"); !ok || tier != 6 {
		t.Errorf("fi ccmatrix: got tier=%d ok=%v, want 6/true", tier, ok)
	}
	if tier, ok := QualityTier("et", "lingq-parallel"); !ok || tier != 0 {
		t.Errorf("et lingq: got tier=%d ok=%v, want 0/true", tier, ok)
	}
	if tier, ok := QualityTier("et", "opus-dochplt"); !ok || tier != 6 {
		t.Errorf("et dochplt: got tier=%d ok=%v, want 6/true", tier, ok)
	}
	if _, ok := QualityTier("fi", "no-such-source"); ok {
		t.Errorf("unknown source should return ok=false")
	}
}

func TestSortForAggregation_Quality(t *testing.T) {
	manifests := []Manifest{
		{Slug: "opus-ccmatrix"},   // tier 6
		{Slug: "yle-news-2022-2024"}, // tier 1
		{Slug: "fixture"},         // tier 0
		{Slug: "opus-tatoeba"},    // tier 3
		{Slug: "wikipedia-fi"},    // tier 2
		{Slug: "no-such-source"},  // unknown → end
	}
	out := SortForAggregation(manifests, "fi", "quality")
	want := []string{
		"fixture",
		"yle-news-2022-2024",
		"wikipedia-fi",
		"opus-tatoeba",
		"opus-ccmatrix",
		"no-such-source",
	}
	got := make([]string, len(out))
	for i, m := range out {
		got[i] = m.Slug
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("quality sort:\ngot:  %v\nwant: %v", got, want)
	}
}

func TestSortForAggregation_Slug(t *testing.T) {
	manifests := []Manifest{
		{Slug: "opus-ccmatrix"},
		{Slug: "yle-news-2022-2024"},
		{Slug: "fixture"},
	}
	out := SortForAggregation(manifests, "fi", "slug")
	want := []string{"fixture", "opus-ccmatrix", "yle-news-2022-2024"}
	got := make([]string, len(out))
	for i, m := range out {
		got[i] = m.Slug
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("slug sort:\ngot:  %v\nwant: %v", got, want)
	}
}

func TestSortForAggregation_DoesNotMutateInput(t *testing.T) {
	in := []Manifest{
		{Slug: "opus-ccmatrix"},
		{Slug: "fixture"},
	}
	_ = SortForAggregation(in, "fi", "quality")
	if in[0].Slug != "opus-ccmatrix" || in[1].Slug != "fixture" {
		t.Errorf("input mutated: %v", in)
	}
}
