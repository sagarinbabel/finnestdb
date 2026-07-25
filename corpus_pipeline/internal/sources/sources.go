// Package sources defines the per-source manifest shape and helpers to
// discover sources by walking <data-root>/{lang}-corpus/*/manifest.json.
package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Manifest is the per-source metadata stored at <source>/manifest.json.
// Determines how the fetcher, extractor, and aggregator handle this source.
type Manifest struct {
	Slug    string `json:"slug"`              // matches the directory name
	Kind    string `json:"kind"`              // "prose" or "poetry"
	Format  string `json:"format"`            // "text", "epub", "vrt", "gz", "leipzig", "wiki", "csv", "skvr", "html", "huggingface", "riigikogu", "erab", "eeva", "md_lingq_parallel", "fixture"
	License string `json:"license,omitempty"` // SPDX-ish identifier or upstream-name
	URL     string `json:"url_source,omitempty"`
	Lang    string `json:"lang,omitempty"`     // "fi" or "et" - usually inferred from parent dir
	Notes   string `json:"notes,omitempty"`
}

// Discover walks <dataRoot>/<lang>-corpus/*/manifest.json and returns the
// found manifests, sorted by slug for determinism.
func Discover(dataRoot, lang string) ([]Manifest, error) {
	dir := filepath.Join(dataRoot, lang+"-corpus")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "_derived" {
			continue
		}
		mfPath := filepath.Join(dir, e.Name(), "manifest.json")
		data, err := os.ReadFile(mfPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", mfPath, err)
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse %s: %w", mfPath, err)
		}
		if m.Slug == "" {
			m.Slug = e.Name()
		}
		if m.Lang == "" {
			m.Lang = lang
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// SourceDir returns the directory path for a given source.
func SourceDir(dataRoot, lang, slug string) string {
	return filepath.Join(dataRoot, lang+"-corpus", slug)
}

// DerivedDir returns the _derived/ root for a given language.
func DerivedDir(dataRoot, lang string) string {
	return filepath.Join(dataRoot, lang+"-corpus", "_derived")
}
