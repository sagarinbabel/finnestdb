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

// extractWiki handles MediaWiki XML.bz2 dumps. v1 implementation:
// requires wikiextractor (Python) or our own parser. Defer to v2
// when the Wikipedia source is added; gz/leipzig OPUS Wikipedia
// already covers most usage.
func extractWiki(dir string, m sources.Manifest) error {
	// Stub: wiki XML extraction requires either wikiextractor (python)
	// or a substantial pure-Go MediaWiki template stripper. v1 Wikipedia
	// data comes from OPUS-Wikipedia (.txt.gz) instead, which uses
	// extract_gz. Keep this as no-op so wiki-formatted sources don't
	// error out the pipeline.
	return nil
}
