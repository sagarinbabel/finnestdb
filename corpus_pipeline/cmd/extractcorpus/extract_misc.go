package main

import "finnestdb/corpus_pipeline/internal/sources"

// Riigikogu, ERAB, EEVA all reduce to source-specific HTML/XML scrapes.
// For v1, they delegate to the generic html or skvr extractors which
// produce reasonable output. Later passes can specialize per format
// (e.g. Riigikogu has structured speaker metadata worth capturing).

func extractRiigikogu(dir string, m sources.Manifest) error {
	// Stenograms are HTML pages; treat with the HTML stripper.
	return extractHTML(dir, m)
}

func extractERAB(dir string, m sources.Manifest) error {
	// ERAB exports as XML; treat as SKVR-style poetry XML.
	return extractSKVR(dir, m)
}

func extractEEVA(dir string, m sources.Manifest) error {
	// EEVA pages are HTML. Per-document poetry/prose routing should
	// happen in a smarter scraper that writes manifest.kind correctly
	// per book; the extractor itself just strips HTML.
	return extractHTML(dir, m)
}

// extractWiki has moved to extract_wiki.go (real implementation).
