// cmd/extractcorpus walks discovered sources and dispatches each to a
// format-specific extractor that produces text.txt (prose) or poems.jsonl
// (poetry) plus documents.jsonl. Idempotent - skip when text.txt is newer
// than raw inputs.
//
// Usage:
//
//	go run ./cmd/extractcorpus -lang fi -data-root ../localdata [-source <slug>] [-force]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"finnestdb/corpus_pipeline/internal/cli"
	"finnestdb/corpus_pipeline/internal/fetcher"
	"finnestdb/corpus_pipeline/internal/sources"
)

func main() {
	var (
		dataRoot   = flag.String("data-root", "../localdata", "absolute or relative path to localdata/")
		lang       = flag.String("lang", "fi", "language code: fi or et")
		onlySource = flag.String("source", "", "if set, extract only this source slug")
		force      = flag.Bool("force", false, "re-extract even if text.txt is up to date")
		skipVerify = flag.Bool("skip-verify", false, "skip the raw-input sha256 check (not recommended)")
	)
	flag.Parse()

	roots, err := cli.Resolve(*dataRoot, "")
	if err != nil {
		log.Fatalf("resolve: %v", err)
	}
	langLower, _, err := cli.LangCodes(*lang)
	if err != nil {
		log.Fatalf("lang: %v", err)
	}

	manifests, err := sources.Discover(roots.DataRoot, langLower)
	if err != nil {
		log.Fatalf("discover: %v", err)
	}
	if len(manifests) == 0 {
		log.Fatalf("no sources found under %s/%s-corpus/", roots.DataRoot, langLower)
	}

	var n int
	for _, m := range manifests {
		if *onlySource != "" && m.Slug != *onlySource {
			continue
		}
		dir := sources.SourceDir(roots.DataRoot, langLower, m.Slug)
		// Verify raw inputs against the sha256 sidecars fetchcorpus wrote
		// before trusting them. Nothing downstream checked these: a source was
		// treated as successfully fetched because the file existed, so a
		// truncated or hand-edited raw file flowed through extraction into the
		// learner artifacts with no symptom other than a quietly smaller
		// corpus. Sources without sidecars (folder-driven EPUB drops) are
		// skipped by VerifyRawDir rather than failed.
		if !*skipVerify {
			bad, err := fetcher.VerifyRawDir(dir)
			if err != nil {
				log.Fatalf("verify %s: %v", m.Slug, err)
			}
			if len(bad) > 0 {
				log.Fatalf("verify %s: %d raw input(s) do not match their sha256 sidecar:\n  %s\n"+
					"re-run fetchcorpus for this source; pass -skip-verify to override",
					m.Slug, len(bad), strings.Join(bad, "\n  "))
			}
		}
		if err := extract(dir, m, *force); err != nil {
			log.Fatalf("extract %s: %v", m.Slug, err)
		}
		n++
	}
	fmt.Fprintf(os.Stderr, "extracted %d source(s) for lang=%s\n", n, langLower)
}
