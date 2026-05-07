// Package lemmatizer is the parser-facing entry point for finnestdb's
// morphology analysis tables.
//
// Policy: we do not vendor or embed transducer blobs (e.g. .vfst/.hfstol)
// in the repository. Instead we ship generated, factual tables derived
// offline from upstream analysers.
package lemmatizer

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"finnestdb/pkg/lemmatizer-fi-et/voikkomap"
)

//go:embed tables/fi_min.json
var fiMinTableJSON []byte

//go:embed tables/et_min.json
var etMinTableJSON []byte
// Analysis is one structured reading of a surface form.
type Analysis = voikkomap.Analysis

// Lemmatizer holds the loaded analysis tables and dispatches Lemmatize
// calls per language. Safe for concurrent use after construction.
type Lemmatizer struct {
	fi map[string][]Analysis
	et map[string][]Analysis
}

// New constructs a Lemmatizer with all currently-supported language
// tables loaded.
func New() (*Lemmatizer, error) {
	var fi map[string][]Analysis
	if err := json.Unmarshal(fiMinTableJSON, &fi); err != nil {
		return nil, fmt.Errorf("lemmatizer: parse embedded FI table: %w", err)
	}
	var et map[string][]Analysis
	if err := json.Unmarshal(etMinTableJSON, &et); err != nil {
		return nil, fmt.Errorf("lemmatizer: parse embedded ET table: %w", err)
	}
	return &Lemmatizer{fi: fi, et: et}, nil
}

// Close releases backing resources. After Close, Lemmatize is unsafe.
func (l *Lemmatizer) Close() error {
	l.fi = nil
	l.et = nil
	return nil
}

// Lemmatize returns every morphological analysis in the generated table
// for the given word in the given language. Returns an empty slice if
// the language isn't supported or the word isn't recognised.
func (l *Lemmatizer) Lemmatize(lang, word string) []Analysis {
	if word == "" {
		return nil
	}
	switch lang {
	case "FI":
		if l.fi == nil || len(l.fi) == 0 {
			return nil
		}
		return l.fi[word]
	case "ET":
		if l.et == nil || len(l.et) == 0 {
			return nil
		}
		return l.et[word]
	default:
		return nil
	}
}
