// Package lemmatizer is the parser-facing entry point for finnestdb's
// finite-state morphology analysers. It wraps language-specific FST
// runtimes (vfst, hfstol) and exposes one Lemmatize call.
//
// PR 1 shipped the Finnish VFST path (libvoikko's mor.vfst). PR 2 adds
// the HFST optimised-lookup path (Giellalt lang-fin's
// analyser-gt-desc.hfstol) and merges both results per FI lookup with
// VFST-priority on overlap. PR 3 will add Estonian via Giellalt.
package lemmatizer

import (
	_ "embed"
	"fmt"

	"finnestdb/pkg/lemmatizer-fi-et/giellaltmap"
	"finnestdb/pkg/lemmatizer-fi-et/hfstol"
	"finnestdb/pkg/lemmatizer-fi-et/vfst"
	"finnestdb/pkg/lemmatizer-fi-et/voikkomap"
)

//go:embed data/fi/mor.vfst
var fiMorVfst []byte

//go:embed data/fi/analyser-gt-desc.hfstol
var fiGiellaltHfstol []byte

//go:embed data/et/analyser-gt-desc.hfstol
var etGiellaltHfstol []byte

// Analysis is one structured reading of a surface form. Re-exported so
// callers don't reach into voikkomap or giellaltmap.
type Analysis = voikkomap.Analysis

// Lemmatizer holds the loaded FSTs and dispatches Lemmatize calls per
// language. Safe for concurrent use; the underlying transducers are
// read-only after construction.
type Lemmatizer struct {
	fiVfst   *vfst.Transducer
	fiHfstol *hfstol.Transducer
	etHfstol *hfstol.Transducer
}

// New constructs a Lemmatizer with all currently-supported language
// backends loaded. Returns an error if any embedded data fails to
// parse — that would only happen on a build-time misconfiguration.
func New() (*Lemmatizer, error) {
	fiV, err := vfst.OpenBytes(fiMorVfst)
	if err != nil {
		return nil, fmt.Errorf("lemmatizer: load embedded FI mor.vfst: %w", err)
	}
	fiH, err := hfstol.OpenBytes(fiGiellaltHfstol)
	if err != nil {
		_ = fiV.Close()
		return nil, fmt.Errorf("lemmatizer: load embedded FI analyser-gt-desc.hfstol: %w", err)
	}
	etH, err := hfstol.OpenBytes(etGiellaltHfstol)
	if err != nil {
		_ = fiV.Close()
		_ = fiH.Close()
		return nil, fmt.Errorf("lemmatizer: load embedded ET analyser-gt-desc.hfstol: %w", err)
	}
	return &Lemmatizer{fiVfst: fiV, fiHfstol: fiH, etHfstol: etH}, nil
}

// Close releases backing resources. After Close, Lemmatize is unsafe.
func (l *Lemmatizer) Close() error {
	if l.fiVfst != nil {
		_ = l.fiVfst.Close()
		l.fiVfst = nil
	}
	if l.fiHfstol != nil {
		_ = l.fiHfstol.Close()
		l.fiHfstol = nil
	}
	if l.etHfstol != nil {
		_ = l.etHfstol.Close()
		l.etHfstol = nil
	}
	return nil
}

// Lemmatize returns every distinct morphological analysis the
// configured FSTs produce for word. Results are merged across
// backends:
//
//   - For Finnish, both libvoikko (VFST, curated) and Giellalt (HFSTOL,
//     broader coverage) are queried. Exact morphology duplicates keep the
//     VFST reading because it is the more curated source. Giellalt-only
//     readings are kept, including grammar-distinct analyses for the same
//     lemma and UPOS.
//   - Compound readings from Giellalt (`+Cmp#`) are delayed until
//     non-compound readings have been considered, so equivalent
//     non-compound readings win without dropping grammar-distinct ones.
//
// The input word should already be lowercased; the FSTs are
// case-sensitive and built on lowercase Finnish.
func (l *Lemmatizer) Lemmatize(lang, word string) []Analysis {
	if word == "" {
		return nil
	}
	switch lang {
	case "FI":
		return l.lemmatizeFI(word)
	case "ET":
		return l.lemmatizeET(word)
	default:
		return nil
	}
}

// lemmatizeET runs only the Giellalt HFSTOL (no Voikko equivalent for
// Estonian). Compound dedup matches the FI path.
func (l *Lemmatizer) lemmatizeET(word string) []Analysis {
	if l.etHfstol == nil {
		return nil
	}
	merged := make(map[analysisKey]Analysis)
	var compoundQueue []giellaltmap.Analysis
	for _, raw := range l.etHfstol.Analyze(word) {
		ga := giellaltmap.Parse(raw)
		if ga.Lemma == "" || ga.UPOS == "" {
			continue
		}
		if ga.IsCompound {
			compoundQueue = append(compoundQueue, ga)
			continue
		}
		k := analysisKey{lemma: ga.Lemma, upos: ga.UPOS}
		if _, exists := merged[k]; !exists {
			merged[k] = giellaltToAnalysis(ga)
		}
	}
	for _, ga := range compoundQueue {
		k := analysisKey{lemma: ga.Lemma, upos: ga.UPOS}
		if _, exists := merged[k]; !exists {
			merged[k] = giellaltToAnalysis(ga)
		}
	}
	if len(merged) == 0 {
		return nil
	}
	out := make([]Analysis, 0, len(merged))
	for _, a := range merged {
		out = append(out, a)
	}
	return out
}

func (l *Lemmatizer) lemmatizeFI(word string) []Analysis {
	var out []Analysis
	seen := make(map[analysisKey]struct{})

	// 1. VFST first — establishes curated source priority and output order.
	if l.fiVfst != nil {
		for _, raw := range l.fiVfst.Analyze(word) {
			a := voikkomap.Parse(raw)
			if a.Lemma == "" {
				continue
			}
			out = appendAnalysis(out, seen, a)
		}
	}

	// 2. HFSTOL second — fills coverage gaps. Exact duplicates are skipped
	//    because VFST has already populated `seen`, but grammar-distinct
	//    readings survive. Compound analyses are deferred so equivalent
	//    non-compound matches take priority.
	if l.fiHfstol != nil {
		var compoundQueue []giellaltmap.Analysis
		for _, raw := range l.fiHfstol.Analyze(word) {
			ga := giellaltmap.Parse(raw)
			if ga.Lemma == "" || ga.UPOS == "" {
				continue
			}
			if ga.IsCompound {
				compoundQueue = append(compoundQueue, ga)
				continue
			}
			out = appendAnalysis(out, seen, giellaltToAnalysis(ga))
		}
		for _, ga := range compoundQueue {
			out = appendAnalysis(out, seen, giellaltToAnalysis(ga))
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

type analysisKey struct {
	lemma        string
	upos         string
	grammarLabel string
	number       string
	tense        string
	mood         string
	person       string
}

func appendAnalysis(out []Analysis, seen map[analysisKey]struct{}, a Analysis) []Analysis {
	k := makeAnalysisKey(a)
	if _, exists := seen[k]; exists {
		return out
	}
	seen[k] = struct{}{}
	return append(out, a)
}

func makeAnalysisKey(a Analysis) analysisKey {
	return analysisKey{
		lemma:        a.Lemma,
		upos:         a.UPOS,
		grammarLabel: a.GrammarLabel,
		number:       a.Number,
		tense:        a.Tense,
		mood:         a.Mood,
		person:       a.Person,
	}
}

func giellaltToAnalysis(g giellaltmap.Analysis) Analysis {
	return Analysis{
		Lemma:        g.Lemma,
		UPOS:         g.UPOS,
		GrammarLabel: g.GrammarLabel,
		Number:       g.Number,
		Tense:        g.Tense,
		Mood:         g.Mood,
		Person:       g.Person,
		Raw:          "[giellalt] " + g.Raw,
	}
}
