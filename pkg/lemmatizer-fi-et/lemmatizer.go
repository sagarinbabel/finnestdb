// Package lemmatizer is the parser-facing entry point for finnestdb's
// finite-state morphology analysers. It wraps language-specific FST
// runtimes (vfst, hfstol) and exposes one Lemmatize call.
//
// PR 1 ships the Finnish VFST path only (libvoikko's mor.vfst, embedded
// in the binary). PR 2 will add the HFST optimised-lookup path and
// merge results. PR 3 adds Estonian.
package lemmatizer

import (
	_ "embed"
	"fmt"

	"finnestdb/pkg/lemmatizer-fi-et/vfst"
	"finnestdb/pkg/lemmatizer-fi-et/voikkomap"
)

// Embed the libvoikko Finnish morphology FST. ~4 MB; trades a small
// binary-size bump for zero filesystem coupling at runtime — important
// for tests, CLI tools, and any deployment where vendoring data
// alongside binaries is awkward.

//go:embed data/fi/mor.vfst
var fiMorVfst []byte

// Analysis is one structured reading of a surface form. Re-exported
// from voikkomap so callers don't need to reach into a sub-package.
type Analysis = voikkomap.Analysis

// Lemmatizer holds the loaded FSTs and dispatches Lemmatize calls per
// language. Safe for concurrent use; the underlying vfst.Transducer is
// read-only after construction.
type Lemmatizer struct {
	fi *vfst.Transducer
	// future: fiHfstol *hfstol.Transducer
	// future: etHfstol *hfstol.Transducer
}

// New constructs a Lemmatizer with all currently-supported language
// backends loaded. Returns an error if any embedded data fails to
// parse, which would only happen on a build-time misconfiguration.
func New() (*Lemmatizer, error) {
	fi, err := vfst.OpenBytes(fiMorVfst)
	if err != nil {
		return nil, fmt.Errorf("lemmatizer: load embedded FI mor.vfst: %w", err)
	}
	return &Lemmatizer{fi: fi}, nil
}

// Close releases backing resources. After Close, Lemmatize is unsafe.
func (l *Lemmatizer) Close() error {
	if l.fi != nil {
		_ = l.fi.Close()
		l.fi = nil
	}
	return nil
}

// Lemmatize returns every morphological analysis the configured FSTs
// produce for the given word in the given language. Returns an empty
// slice if the language isn't supported or the word isn't recognised.
//
// The input word should already be lowercased; libvoikko's mor.vfst
// is case-sensitive and its alphabet is the lowercase Finnish set.
func (l *Lemmatizer) Lemmatize(lang, word string) []Analysis {
	if word == "" {
		return nil
	}
	switch lang {
	case "FI":
		if l.fi == nil {
			return nil
		}
		raw := l.fi.Analyze(word)
		if len(raw) == 0 {
			return nil
		}
		out := make([]Analysis, 0, len(raw))
		for _, fst := range raw {
			a := voikkomap.Parse(fst)
			if a.Lemma == "" {
				continue
			}
			out = append(out, a)
		}
		return out
	default:
		return nil
	}
}
