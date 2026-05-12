// Package lemmatizer is the parser-facing entry point for finnestdb's
// morphology analysis tables.
//
// Policy: per docs/ARTIFACT_POLICY.md, neither upstream transducer
// blobs nor the factual tables generated from them are tracked in git.
// The tables are produced by cmd/genlemmatizertables (FI) and equivalent
// pipelines (ET) into localdata/lemmatizer-fi-et/tables/, and loaded
// from disk on New(). Test fixtures live under testdata/lemmatizer/.
package lemmatizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"finnestdb/pkg/lemmatizer-fi-et/lexadverbs"
	"finnestdb/pkg/lemmatizer-fi-et/udfeats"
	"finnestdb/pkg/lemmatizer-fi-et/voikkomap"
)

// Analysis is one structured reading of a surface form.
type Analysis = voikkomap.Analysis

// DefaultTablesDir is where New() looks for the FI/ET tables. Relative
// to CWD; the LEMMATIZER_TABLES_DIR env var overrides it. Tests use
// NewFromDir directly with a path under testdata/lemmatizer/.
const DefaultTablesDir = "localdata/lemmatizer-fi-et/tables"

// ErrNoTables is returned (wrapped) by New / NewFromDir when neither
// fi_min.json nor et_min.json is present in the resolved directory.
// Callers can use errors.Is(err, ErrNoTables) to distinguish "tables
// have not been generated / setup-local.sh not run" from "table file
// is malformed", since the former is a degraded-but-acceptable state
// and the latter is a misconfiguration.
var ErrNoTables = errors.New("lemmatizer: no tables found")

// Lemmatizer holds the loaded analysis tables and dispatches Lemmatize
// calls per language. Safe for concurrent use after construction.
type Lemmatizer struct {
	fi map[string][]Analysis
	et map[string][]Analysis
}

// New constructs a Lemmatizer reading FI/ET tables from the directory
// resolved by LEMMATIZER_TABLES_DIR (falling back to DefaultTablesDir).
// Returns an error wrapping ErrNoTables when neither table is present
// — callers in production paths (e.g. internal/store) treat that as
// "FST disabled" rather than fatal so a fresh clone without
// scripts/setup-local.sh still serves the dict-only path.
func New() (*Lemmatizer, error) {
	return NewFromDir(resolveTablesDir())
}

// NewFromDir is the explicit-path constructor. Used by tests with the
// hand-authored fixtures under testdata/lemmatizer/, and by callers
// that want to load from a non-default location without touching env.
//
// Per-language coverage is independent: a deployment that has only
// fi_min.json (e.g. because no ET generator exists yet) loads FI and
// returns empty analyses for ET. Returns an error only when both
// tables are missing or when a present table fails to parse.
func NewFromDir(dir string) (*Lemmatizer, error) {
	fiPath := filepath.Join(dir, "fi_min.json")
	etPath := filepath.Join(dir, "et_min.json")
	fi, err := loadTable(fiPath)
	if err != nil {
		return nil, fmt.Errorf("lemmatizer: load FI table: %w", err)
	}
	et, err := loadTable(etPath)
	if err != nil {
		return nil, fmt.Errorf("lemmatizer: load ET table: %w", err)
	}
	if fi == nil && et == nil {
		return nil, fmt.Errorf("%w under %s (looked for fi_min.json, et_min.json)", ErrNoTables, dir)
	}
	return &Lemmatizer{fi: fi, et: et}, nil
}

// loadTable reads and parses one table. Returns (nil, nil) when the
// file is absent so the caller can degrade gracefully.
func loadTable(path string) (map[string][]Analysis, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t map[string][]Analysis
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return t, nil
}

func resolveTablesDir() string {
	if v := os.Getenv("LEMMATIZER_TABLES_DIR"); v != "" {
		return v
	}
	return DefaultTablesDir
}

// Close releases backing resources. After Close, Lemmatize is unsafe.
func (l *Lemmatizer) Close() error {
	l.fi = nil
	l.et = nil
	return nil
}

// Lemmatize returns every morphological analysis in the loaded table
// for the given word in the given language. Returns an empty slice if
// the language isn't supported or the word isn't recognised.
//
// Two post-processing layers run on the FST output before it is
// returned. Both are no-ops on the common case so the hot path stays
// cheap:
//
//   - The lexadverbs overlay shadows known FST-output bugs with a
//     curated analysis (FI: `tuskin` returns ADV not the productive
//     instructive of `tuska`; ET: `peale` returns ADP not the
//     allative of `pea`). When an overlay hit fires, the FST
//     readings are dropped entirely — the overlay asserts the
//     correct answer.
//   - The MA-infinitive normaliser rewrites Finnish FEATS on verb
//     surfaces like `tarjoamaan` where the analyzer's noun-cousin
//     reading (`tarjoama` Case=Ill|Person=3|Number=Sing) is replaced
//     by the verb's MA-infinitive shape (Case=Ill|VerbForm=Inf|InfForm=Ma).
//     This rule is FI-only: Estonian -ma supine forms are already
//     tagged VerbForm=Sup by the Ekilex-backed analyzer (see
//     cmd/importdict/feats.go), so the noun-cousin trap doesn't
//     arise the same way.
func (l *Lemmatizer) Lemmatize(lang, word string) []Analysis {
	if word == "" {
		return nil
	}
	switch lang {
	case "FI":
		if overlay, ok := lexadverbs.LookupFI(word); ok {
			return overlay
		}
		if l.fi == nil || len(l.fi) == 0 {
			return nil
		}
		raw := l.fi[word]
		if len(raw) == 0 {
			return nil
		}
		return normaliseFI(word, raw)
	case "ET":
		if overlay, ok := lexadverbs.LookupET(word); ok {
			return overlay
		}
		if l.et == nil || len(l.et) == 0 {
			return nil
		}
		return l.et[word]
	default:
		return nil
	}
}

// normaliseFI returns a copy of raw with FEATS rewritten by per-surface
// rules. Cloning is mandatory: the caller's map is the cached table,
// and downstream consumers (parser-compare, dict.go) may mutate the
// returned slice elements while ranking. Skipping the clone would
// poison the cache.
func normaliseFI(surface string, raw []Analysis) []Analysis {
	out := make([]Analysis, len(raw))
	for i, a := range raw {
		a.Feats = udfeats.NormalizeMaInfinitive(surface, a.UPOS, a.Feats)
		out[i] = a
	}
	return out
}
