// cmd/fetchcorpus downloads sources to localdata/{lang}-corpus/<source>/raw/
// based on the per-language source registry. Idempotent via sha256 sidecars.
//
// Usage:
//
//	go run ./cmd/fetchcorpus -lang fi [-source <slug>] [-data-root ../localdata] [-dry-run]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"finnestdb/corpus_pipeline/internal/cli"
	"finnestdb/corpus_pipeline/internal/fetcher"
	"finnestdb/corpus_pipeline/internal/sources"
)

// Source describes one downloadable entry in the registry. URL is required;
// if the entry is folder-driven (e.g. EPUBs the user dropped in), URL is
// "" and we skip the fetch.
type Source struct {
	Slug     string
	Lang     string
	Kind     string // "prose" or "poetry"
	Format   string
	URL      string // empty = folder-driven
	Filename string // local filename under <source>/raw/
	License  string
	Notes    string
}

func main() {
	var (
		dataRoot = flag.String("data-root", "../localdata", "")
		lang     = flag.String("lang", "fi", "")
		only     = flag.String("source", "", "if set, fetch only this slug")
		dryRun   = flag.Bool("dry-run", false, "HEAD probe each URL, don't download")
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

	registry := registryFor(langLower)
	if *only != "" {
		registry = filterRegistry(registry, *only)
	}
	if len(registry) == 0 {
		log.Fatalf("no sources in registry for lang=%s only=%s", langLower, *only)
	}

	// Track failures so the process can exit nonzero. Previously every
	// per-source error printed "FAIL" to stderr and `continue`d, and main
	// returned 0 - so a partially fetched corpus was indistinguishable from a
	// complete one to the next stage and to any `make` target or CI step
	// chaining them. Downstream stages infer "this source was fetched" purely
	// from file presence, so a missing source silently shrinks the corpus
	// rather than stopping the pipeline.
	var failed []string

	for _, src := range registry {
		if src.URL == "" {
			fmt.Fprintf(os.Stderr, "[fetch %s] %s: folder-driven (no URL) - skipped\n", langLower, src.Slug)
			continue
		}
		dir := sources.SourceDir(roots.DataRoot, langLower, src.Slug)
		if err := os.MkdirAll(filepath.Join(dir, "raw"), 0o755); err != nil {
			log.Fatalf("mkdir %s: %v", dir, err)
		}
		// Write/refresh manifest.json
		mf := sources.Manifest{
			Slug: src.Slug, Kind: src.Kind, Format: src.Format,
			License: src.License, URL: src.URL, Lang: src.Lang, Notes: src.Notes,
		}
		if err := writeManifest(filepath.Join(dir, "manifest.json"), mf); err != nil {
			log.Fatalf("write manifest %s: %v", src.Slug, err)
		}

		outPath := filepath.Join(dir, "raw", src.Filename)
		if *dryRun {
			size, err := fetcher.HEAD(src.URL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[fetch %s] %s: HEAD FAIL %v\n", langLower, src.Slug, err)
				failed = append(failed, src.Slug)
				continue
			}
			fmt.Fprintf(os.Stderr, "[fetch %s] %s: HEAD OK content-length=%d url=%s\n", langLower, src.Slug, size, src.URL)
			continue
		}
		fmt.Fprintf(os.Stderr, "[fetch %s] %s: downloading %s\n", langLower, src.Slug, src.URL)
		n, err := fetcher.Download(src.URL, outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[fetch %s] %s: FAIL %v\n", langLower, src.Slug, err)
			failed = append(failed, src.Slug)
			continue
		}
		fmt.Fprintf(os.Stderr, "[fetch %s] %s: %d bytes → %s\n", langLower, src.Slug, n, outPath)
	}

	// Keep fetching every source (one dead mirror should not block the other
	// twenty-nine) but report the failure in the exit status, so a human or a
	// make target cannot mistake a partial corpus for a complete one.
	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "\n[fetch %s] %d of %d source(s) FAILED: %s\n",
			langLower, len(failed), len(registry), strings.Join(failed, ", "))
		fmt.Fprintf(os.Stderr, "[fetch %s] the corpus under %s is INCOMPLETE - "+
			"re-run fetchcorpus before extracting, or extraction will silently "+
			"produce a smaller corpus than intended\n", langLower, roots.DataRoot)
		os.Exit(1)
	}
}

func filterRegistry(r []Source, slug string) []Source {
	for _, s := range r {
		if s.Slug == slug {
			return []Source{s}
		}
	}
	return nil
}

func writeManifest(path string, m sources.Manifest) error {
	return writeJSON(path, m)
}
