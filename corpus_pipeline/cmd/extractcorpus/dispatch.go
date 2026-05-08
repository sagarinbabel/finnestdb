package main

import (
	"fmt"
	"os"
	"path/filepath"

	"finnestdb/corpus_pipeline/internal/sources"
)

// extract dispatches to the format-specific extractor based on
// manifest.format, writing <dir>/text.txt (prose) or <dir>/poems.jsonl
// (poetry) plus <dir>/documents.jsonl.
func extract(dir string, m sources.Manifest, force bool) error {
	textPath := filepath.Join(dir, "text.txt")
	poemsPath := filepath.Join(dir, "poems.jsonl")
	if !force {
		if upToDate(textPath, dir) || upToDate(poemsPath, dir) {
			return nil
		}
	}
	switch m.Format {
	case "fixture":
		return extractFixture(dir, m)
	case "text":
		return extractText(dir, m)
	case "md_lingq_parallel":
		return extractMDLingQ(dir, m)
	case "epub":
		return extractEPUB(dir, m, force)
	case "gz":
		return extractGZ(dir, m)
	case "csv", "tsv":
		return extractCSV(dir, m)
	case "leipzig":
		return extractLeipzig(dir, m)
	case "skvr":
		return extractSKVR(dir, m)
	case "html":
		return extractHTML(dir, m)
	case "huggingface", "jsonl", "jsonl_gz":
		return extractHuggingFace(dir, m)
	case "riigikogu":
		return extractRiigikogu(dir, m)
	case "erab":
		return extractERAB(dir, m)
	case "eeva":
		return extractEEVA(dir, m)
	case "vrt", "vrt_zip":
		return extractVRT(dir, m)
	case "wiki", "mediawiki_xml_bz2", "wikipedia_dump":
		return extractWiki(dir, m)
	default:
		fmt.Fprintf(os.Stderr, "[extractcorpus] WARN: format %q not yet implemented; skipping source %s\n", m.Format, m.Slug)
		return nil
	}
}

// upToDate returns true if outPath exists and is newer than every file under
// dir/raw/ (excluding outPath itself). Cheap idempotency check.
func upToDate(outPath, dir string) bool {
	out, err := os.Stat(outPath)
	if err != nil {
		return false
	}
	rawDir := filepath.Join(dir, "raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil || len(entries) == 0 {
		return false
	}
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			return false
		}
		if fi.ModTime().After(out.ModTime()) {
			return false
		}
	}
	return true
}
