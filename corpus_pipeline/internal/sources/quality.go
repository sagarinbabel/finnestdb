package sources

import "sort"

// QualityTier returns the per-source quality tier for the given (lang, slug).
// Lower tier = higher learner value. The second return is false for slugs
// not in the registry, in which case callers should sort them after known
// sources but still deterministically by slug.
//
// Tiers - current as of v1 corpus build:
//
//	0  hand-authored / human-curated (fixture, manual, lingq-parallel)
//	1  cleaned books, broadcaster news (epub, Yle, ERR, Riigikogu)
//	2  encyclopedic + Leipzig (Wikipedia + Leipzig sentence-of-the-day pulls)
//	3  conversational / talks / frequency lists (Tatoeba, TED, OpenSubtitles)
//	4  legal / EU institutional / domain (Europarl, Finlex, ECB, JRC, EMEA, Bookshop, Bible)
//	5  noisy aligned web (WikiMatrix, ParaCrawl, MultiParaCrawl)
//	6  mined web parallel (HPLT, MultiHPLT, DocHPLT, CCMatrix, NLLB)
//
// The intent: when we have a byte budget for the learner-facing user-friendly
// TSVs, fill it from tier 0 → 6 in that order. Higher tiers (especially 5–6)
// are useful for parser-improvement signal and frequency counts but are
// register-noisy for someone reading example sentences.
func QualityTier(lang, slug string) (int, bool) {
	var tiers map[string]int
	switch lang {
	case "fi":
		tiers = qualityTiersFI
	case "et":
		tiers = qualityTiersET
	default:
		return unknownTier, false
	}
	t, ok := tiers[slug]
	return t, ok
}

const unknownTier = 999

var qualityTiersFI = map[string]int{
	"fixture": 0,
	"manual":  0,

	"epub":               1,
	"yle-news-2022-2024": 1,
	"yle-news-2011-2018": 1,

	"wikipedia-fi":              2,
	"leipzig-fi-news-2020":      2,
	"leipzig-fi-newscrawl-2017": 2,
	"leipzig-fi-wikipedia-2021": 2,

	"opus-tatoeba":       3,
	"opus-ted2020":       3,
	"opus-opensubtitles": 3,
	"frequency-words-fi": 3,

	"opus-europarl":   4,
	"opus-finlex":     4,
	"opus-ecb":        4,
	"opus-jrc-acquis": 4,
	"opus-emea":       4,
	"opus-eubookshop": 4,
	"opus-bible":      4,

	"opus-wikimatrix":     5,
	"opus-paracrawl":      5,
	"opus-multiparacrawl": 5,

	"opus-hplt":      6,
	"opus-multihplt": 6,
	"opus-ccmatrix":  6,
	"opus-nllb":      6,
}

var qualityTiersET = map[string]int{
	"fixture":        0,
	"lingq-parallel": 0,

	"hf-err-newsroom": 1,
	"riigikogu":       1,

	"wikipedia-et":              2,
	"leipzig-et-news-2020":      2,
	"leipzig-et-newscrawl-2017": 2,
	"leipzig-et-wikipedia-2021": 2,

	"opus-tatoeba":       3,
	"opus-ted2020":       3,
	"opus-opensubtitles": 3,
	"frequency-words-et": 3,

	"opus-europarl":   4,
	"opus-jrc-acquis": 4,
	"opus-emea":       4,
	"opus-eubookshop": 4,
	"opus-bible":      4,

	"opus-wikimatrix":     5,
	"opus-paracrawl":      5,
	"opus-multiparacrawl": 5,

	"opus-hplt":      6,
	"opus-multihplt": 6,
	"opus-dochplt":   6,
	"opus-ccmatrix":  6,
	"opus-nllb":      6,
}

// SortForAggregation returns a *copy* of manifests in the order Phase 1
// should ingest them. Aggregation-only - Discover() still sorts globally
// by slug for tools that care about source identity (extractcorpus,
// fetchcorpus, etc).
//
// mode == "quality" (default): tier asc, then slug asc. Unknown slugs
// (no entry in qualityTiersFI/ET) sort after known sources, still by slug.
// mode == "slug": pure slug-asc order, mirroring Discover()'s behavior
// for callers that want the historical determinism.
//
// Quality ordering matters when the run carries a byte budget for the
// learner-facing user-friendly TSVs: higher-quality sources fill that
// budget first, and noisy mined-web OPUS dumps only contribute if room
// remains.
func SortForAggregation(manifests []Manifest, lang, mode string) []Manifest {
	out := make([]Manifest, len(manifests))
	copy(out, manifests)
	if mode == "slug" {
		sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
		return out
	}
	// Default: quality
	sort.SliceStable(out, func(i, j int) bool {
		ti, oi := QualityTier(lang, out[i].Slug)
		tj, oj := QualityTier(lang, out[j].Slug)
		if !oi {
			ti = unknownTier
		}
		if !oj {
			tj = unknownTier
		}
		if ti != tj {
			return ti < tj
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}
