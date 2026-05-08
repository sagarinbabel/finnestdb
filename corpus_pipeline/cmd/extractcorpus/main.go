// cmd/extractcorpus walks discovered sources and dispatches each to a
// format-specific extractor that produces text.txt (prose) or poems.jsonl
// (poetry) plus documents.jsonl. Idempotent — skip when text.txt is newer
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

	"finnestdb/corpus_pipeline/internal/cli"
	"finnestdb/corpus_pipeline/internal/sources"
)

func main() {
	var (
		dataRoot   = flag.String("data-root", "../localdata", "absolute or relative path to localdata/")
		lang       = flag.String("lang", "fi", "language code: fi or et")
		onlySource = flag.String("source", "", "if set, extract only this source slug")
		force      = flag.Bool("force", false, "re-extract even if text.txt is up to date")
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
		if err := extract(dir, m, *force); err != nil {
			log.Fatalf("extract %s: %v", m.Slug, err)
		}
		n++
	}
	fmt.Fprintf(os.Stderr, "extracted %d source(s) for lang=%s\n", n, langLower)
}
