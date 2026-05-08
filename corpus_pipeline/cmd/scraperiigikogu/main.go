// cmd/scraperiigikogu polite-scrapes Riigikogu (Estonian parliament)
// stenograms from https://stenogrammid.riigikogu.ee/.
//
// Strategy:
//  1. Fetch the index page, extract sitting IDs (YYYYMMDDHHMM format
//     in /et/<id> hrefs).
//  2. For each sitting ID, fetch the HTML page, save to
//     localdata/et-corpus/riigikogu/raw/<id>.html.
//  3. Sleep 1.5s between requests.
//  4. Idempotent: skip sittings already on disk.
//
// After scraping, `make extract-corpus -lang et -source riigikogu` runs
// the existing HTML extractor over the saved pages.
//
// Usage:
//
//	go run ./cmd/scraperiigikogu -limit 100      # most-recent 100 sittings
//	go run ./cmd/scraperiigikogu                  # whatever the index lists
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"finnestdb/corpus_pipeline/internal/cli"
	"finnestdb/corpus_pipeline/internal/sources"
)

const (
	indexURL  = "https://stenogrammid.riigikogu.ee/"
	pageURL   = "https://stenogrammid.riigikogu.ee/et/%s"
	delay     = 1500 * time.Millisecond
	userAgent = "finnestdb/scraperiigikogu (+local research; polite scraper)"
)

var idRE = regexp.MustCompile(`href="/et/(\d{12,14})(?:#[^"]*)?"`)

func main() {
	var (
		dataRoot = flag.String("data-root", "../localdata", "")
		limit    = flag.Int("limit", 0, "if >0, scrape at most this many sittings")
	)
	flag.Parse()
	roots, err := cli.Resolve(*dataRoot, "")
	if err != nil {
		log.Fatalf("resolve: %v", err)
	}
	srcDir := sources.SourceDir(roots.DataRoot, "et", "riigikogu")
	rawDir := filepath.Join(srcDir, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", rawDir, err)
	}
	// Write/refresh manifest
	mf := sources.Manifest{
		Slug:    "riigikogu",
		Kind:    "prose",
		Format:  "riigikogu",
		Lang:    "et",
		License: "Riigikogu open data (CC BY-SA 3.0 estimated)",
		URL:     indexURL,
		Notes:   "Estonian parliament (Riigikogu) stenograms. Polite-scraped from stenogrammid.riigikogu.ee.",
	}
	if err := writeManifest(filepath.Join(srcDir, "manifest.json"), mf); err != nil {
		log.Fatalf("manifest: %v", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	indexHTML, err := fetchOne(client, indexURL)
	if err != nil {
		log.Fatalf("index: %v", err)
	}
	matches := idRE.FindAllStringSubmatch(indexHTML, -1)
	seen := map[string]bool{}
	var ids []string
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			ids = append(ids, m[1])
		}
	}
	if *limit > 0 && len(ids) > *limit {
		ids = ids[:*limit]
	}
	fmt.Fprintf(os.Stderr, "[riigikogu] scraping %d sittings (delay=%v)\n", len(ids), delay)
	processed, skipped := 0, 0
	for _, id := range ids {
		outPath := filepath.Join(rawDir, id+".html")
		if fi, err := os.Stat(outPath); err == nil && fi.Size() > 1024 {
			skipped++
			continue
		}
		time.Sleep(delay)
		body, err := fetchOne(client, fmt.Sprintf(pageURL, id))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[riigikogu] %s: %v\n", id, err)
			continue
		}
		if err := os.WriteFile(outPath, []byte(body), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "[riigikogu] write %s: %v\n", outPath, err)
			continue
		}
		processed++
		if processed%10 == 0 {
			fmt.Fprintf(os.Stderr, "[riigikogu] %d sittings fetched (skipped %d)\n", processed, skipped)
		}
	}
	fmt.Fprintf(os.Stderr, "[riigikogu] done: %d new, %d skipped (already on disk)\n", processed, skipped)
}

func fetchOne(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("status %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(body), "<html") {
		return "", fmt.Errorf("body doesn't look like HTML")
	}
	return string(body), nil
}

func writeManifest(path string, m sources.Manifest) error {
	// Tiny manifest writer (no shared package needed)
	data, err := jsonMarshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
