// Package main implements cmd/ambiguityeval: the ambiguity and
// meaning-check calibration eval runner specified in
// docs/PARSER_EVAL_METHODOLOGY.md §"Ambiguity and meaning-check calibration".
//
// It is intentionally separate from cmd/parsertest: the ambiguity metrics
// (candidate inclusion, per-class stratification, proxy-stratified accuracy)
// are not token-accuracy metrics and would distort the parsertest summary
// schema. Keeping them apart also keeps the ambiguity slice out of the
// headline sweep by construction.
package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Dataset is the ambiguity gold-file shape: a minimal extension of the
// committed gold shape (internal/eval.Dataset / DatasetCase /
// ExpectedTokenRef) with an ambiguity_class per case and an
// expected_candidates sense set on the scored target token. See
// testdata/parser-eval/fi-ambiguity/README.md for the full field docs and a
// worked example.
type Dataset struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Language string `json:"language"`
	Slice    string `json:"slice"`
	Cases    []Case `json:"cases"`
}

type Case struct {
	ID             string  `json:"id"`
	Text           string  `json:"text"`
	AmbiguityClass string  `json:"ambiguity_class"`
	Tokens         []Token `json:"tokens"`
}

type Token struct {
	Surface            string      `json:"surface"`
	Occurrence         int         `json:"occurrence,omitempty"`
	Target             bool        `json:"target,omitempty"`
	Lemma              string      `json:"lemma"`
	POS                string      `json:"pos"`
	GrammarLabel       string      `json:"grammar_label,omitempty"`
	Feats              string      `json:"feats,omitempty"`
	ExpectedCandidates []Candidate `json:"expected_candidates,omitempty"`
}

type Candidate struct {
	Lemma     string `json:"lemma"`
	POS       string `json:"pos"`
	GlossHint string `json:"gloss_hint,omitempty"`
}

// LoadDataset reads and validates one ambiguity gold file. Mirrors
// internal/eval.LoadDataset's validation strictness (required name/language,
// non-empty cases/tokens) plus the ambiguity-specific requirements: slice
// must say "ambiguity", every case needs an ambiguity_class, and exactly one
// token per case must be the scored target with a non-empty candidate set.
func LoadDataset(path string) (*Dataset, error) {
	b, err := readDatasetBytes(path)
	if err != nil {
		return nil, err
	}
	var dataset Dataset
	if err := json.Unmarshal(b, &dataset); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if dataset.Name == "" {
		return nil, fmt.Errorf("%s: dataset name is required", path)
	}
	if dataset.Language != "FI" && dataset.Language != "ET" {
		return nil, fmt.Errorf("%s: dataset language must be FI or ET", path)
	}
	if dataset.Slice != "ambiguity" {
		return nil, fmt.Errorf("%s: dataset slice must be %q, got %q", path, "ambiguity", dataset.Slice)
	}
	if len(dataset.Cases) == 0 {
		return nil, fmt.Errorf("%s: dataset must contain at least one case", path)
	}
	for i, c := range dataset.Cases {
		if c.ID == "" {
			return nil, fmt.Errorf("%s: case %d: id is required", path, i)
		}
		if c.Text == "" {
			return nil, fmt.Errorf("%s: case %s: text is required", path, c.ID)
		}
		if c.AmbiguityClass == "" {
			return nil, fmt.Errorf("%s: case %s: ambiguity_class is required", path, c.ID)
		}
		target, err := targetToken(c)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if len(target.ExpectedCandidates) == 0 {
			return nil, fmt.Errorf("%s: case %s: target token requires expected_candidates", path, c.ID)
		}
	}
	return &dataset, nil
}

// targetToken returns the single token marked target: true in a case.
// Exactly one is required — the slice is defined around one scored target
// per sentence (see the methodology doc and gold README).
func targetToken(c Case) (Token, error) {
	var found *Token
	for i := range c.Tokens {
		if !c.Tokens[i].Target {
			continue
		}
		if found != nil {
			return Token{}, fmt.Errorf("case %s: more than one token marked target", c.ID)
		}
		found = &c.Tokens[i]
	}
	if found == nil {
		return Token{}, fmt.Errorf("case %s: no token marked target", c.ID)
	}
	return *found, nil
}

func readDatasetBytes(path string) ([]byte, error) {
	if !strings.HasSuffix(strings.ToLower(path), ".gz") {
		return os.ReadFile(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}
