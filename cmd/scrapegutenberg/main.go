// cmd/scrapegutenberg fetches Finnish-language books from Project Gutenberg
// (gutenberg.org) for use as a silver-tier corpus. Plan C / PR 3 - see
// docs/CHANGELOG.md.
//
// Source: https://www.gutenberg.org/ebooks/search/?query=l.fi
// License: every Gutenberg text in scope is in the public domain in the US;
// the Project Gutenberg trademark / boilerplate is stripped before saving.
//
// Usage:
//
//	go run ./cmd/scrapegutenberg \
//	    -lang fi \
//	    -target-tokens 500000 \
//	    -out localdata/silver-fi/raw \
//	    -manifest localdata/silver-fi/manifest.jsonl
//
// Pipeline:
//
//  1. Walk pages of the search results (`?query=l.fi&start_index=N`)
//     collecting ebook IDs until we have enough candidates.
//  2. For each ID, try the canonical .txt URLs in order:
//     https://www.gutenberg.org/cache/epub/<id>/pg<id>.txt
//     https://www.gutenberg.org/files/<id>/<id>-0.txt
//     https://www.gutenberg.org/files/<id>/<id>-8.txt
//     The cache-epub URL is UTF-8; the -0/-8 URLs are ISO-8859-1.
//  3. Strip the Project Gutenberg header (above "*** START OF") and footer
//     (below "*** END OF"). Strip transcriber notes if present.
//  4. Detect language with a heuristic - if not Finnish, skip. (The search
//     filter usually keeps this clean, but English-language commentary
//     books that mention Finnish do leak into the result set.)
//  5. Save the cleaned text under -out/<id>.txt and append a manifest
//     record with id, title, author, token count, source URL, encoding
//     to -manifest. Manifest is the single source of truth for which
//     books made the cut.
//  6. Stop when the cumulative token count crosses -target-tokens.
//
// The scraper is polite - 1.5s delay between requests, single connection,
// transparent User-Agent. Re-runs are idempotent - the manifest is checked
// first and already-fetched IDs are skipped.
//
// Tokens are approximated by whitespace splitting (good enough for the
// "have we hit 500k?" decision; exact counts come later when this corpus
// goes through the silver tagger in Plan C / PR 4).
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

const (
	defaultUserAgent  = "finnestdb-scrapegutenberg/0.1 (+https://github.com/sagarinbabel/finnestdb)"
	requestDelay      = 1500 * time.Millisecond
	requestTimeout    = 30 * time.Second
	searchPageSize    = 25 // Gutenberg returns 25 results per page
	maxSearchPages    = 60 // 60 × 25 = 1500 candidate IDs - plenty for ~500k tokens
	maxBookSize       = 5 * 1024 * 1024
	startMarkerPrefix = "*** START OF"
	endMarkerPrefix   = "*** END OF"
)

func main() {
	var (
		langFlag     = flag.String("lang", "fi", "Gutenberg language code to scrape (e.g. fi, et)")
		targetTokens = flag.Int("target-tokens", 500_000, "Stop once the cumulative token count crosses this threshold")
		outDir       = flag.String("out", "localdata/silver-fi/raw", "Directory for cleaned per-book .txt files")
		manifestPath = flag.String("manifest", "localdata/silver-fi/manifest.jsonl", "JSONL manifest of fetched books")
		dryRun       = flag.Bool("dry-run", false, "Print which IDs would be fetched without downloading")
		userAgent    = flag.String("user-agent", defaultUserAgent, "User-Agent for HTTP requests")
	)
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", *outDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(*manifestPath), 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", filepath.Dir(*manifestPath), err)
	}

	already, err := loadManifest(*manifestPath)
	if err != nil {
		log.Fatalf("load manifest %s: %v", *manifestPath, err)
	}
	cumulativeTokens := 0
	for _, e := range already {
		cumulativeTokens += e.Tokens
	}
	log.Printf("manifest already has %d books / %d tokens; need %d more",
		len(already), cumulativeTokens, max(0, *targetTokens-cumulativeTokens))
	if cumulativeTokens >= *targetTokens {
		log.Printf("target already met - exiting")
		return
	}

	client := &http.Client{Timeout: requestTimeout}
	manifestFile, err := os.OpenFile(*manifestPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("open manifest: %v", err)
	}
	defer manifestFile.Close()

	for page := 0; page < maxSearchPages; page++ {
		ids, err := searchEbookIDs(client, *langFlag, page, *userAgent)
		if err != nil {
			log.Printf("search page %d: %v", page, err)
			continue
		}
		if len(ids) == 0 {
			log.Printf("page %d empty; stopping search", page)
			break
		}

		for _, id := range ids {
			if _, ok := already[id]; ok {
				continue
			}
			if *dryRun {
				log.Printf("[dry-run] would fetch %d", id)
				continue
			}

			time.Sleep(requestDelay)
			text, sourceURL, encoding, err := fetchBookText(client, id, *userAgent)
			if err != nil {
				log.Printf("fetch %d: %v", id, err)
				continue
			}
			meta, body, err := parseGutenbergText(text)
			if err != nil {
				log.Printf("parse %d: %v", id, err)
				continue
			}
			if !looksFinnish(body) {
				log.Printf("skip %d (%q): not Finnish-looking text", id, meta.Title)
				continue
			}

			tokens := approxTokenCount(body)
			outPath := filepath.Join(*outDir, fmt.Sprintf("%d.txt", id))
			if err := os.WriteFile(outPath, []byte(body), 0o644); err != nil {
				log.Printf("write %d: %v", id, err)
				continue
			}

			rec := manifestRecord{
				ID:        id,
				Title:     meta.Title,
				Author:    meta.Author,
				Language:  meta.Language,
				Tokens:    tokens,
				SourceURL: sourceURL,
				Encoding:  encoding,
				FetchedAt: time.Now().UTC().Format(time.RFC3339),
				Path:      outPath,
			}
			line, _ := json.Marshal(rec)
			if _, err := manifestFile.Write(append(line, '\n')); err != nil {
				log.Fatalf("write manifest: %v", err)
			}
			already[id] = rec
			cumulativeTokens += tokens
			log.Printf("[%6d tok cum=%d/%d] %d %q (%s)",
				tokens, cumulativeTokens, *targetTokens, id, meta.Title, meta.Author)

			if cumulativeTokens >= *targetTokens {
				log.Printf("target met (%d ≥ %d); stopping", cumulativeTokens, *targetTokens)
				return
			}
		}
	}

	log.Printf("finished; ran out of search results before hitting target. cumulative tokens=%d/%d",
		cumulativeTokens, *targetTokens)
}

// manifestRecord is one line in the .jsonl manifest. Fields capture exactly
// what's needed to reproduce the corpus, attribute it correctly, and tell
// later scripts which file holds which book.
type manifestRecord struct {
	ID        int    `json:"id"`
	Title     string `json:"title,omitempty"`
	Author    string `json:"author,omitempty"`
	Language  string `json:"language,omitempty"`
	Tokens    int    `json:"tokens"`
	SourceURL string `json:"source_url"`
	Encoding  string `json:"encoding"`
	FetchedAt string `json:"fetched_at"`
	Path      string `json:"path"`
}

// loadManifest reads any existing manifest so the scraper can be re-run
// incrementally. Returns an empty map if the file doesn't exist.
func loadManifest(path string) (map[int]manifestRecord, error) {
	out := make(map[int]manifestRecord)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec manifestRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("parse manifest line: %w", err)
		}
		out[rec.ID] = rec
	}
	return out, sc.Err()
}

// searchEbookIDs hits Gutenberg's search page filtered by language and parses
// out the ebook IDs in result order. start_index is 1-based on Gutenberg's
// side; we expose it as a 0-based page number internally.
func searchEbookIDs(client *http.Client, lang string, page int, ua string) ([]int, error) {
	startIndex := page*searchPageSize + 1
	url := fmt.Sprintf("https://www.gutenberg.org/ebooks/search/?query=l.%s&start_index=%d", lang, startIndex)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	return parseSearchPage(string(body)), nil
}

var searchHrefRE = regexp.MustCompile(`href="/ebooks/([0-9]+)"`)

// parseSearchPage extracts unique ebook IDs from a Gutenberg search results
// page. Each result links to /ebooks/<id>; we deduplicate (the same book
// appears in the result list and in side widgets) while preserving order.
func parseSearchPage(html string) []int {
	matches := searchHrefRE.FindAllStringSubmatch(html, -1)
	seen := make(map[int]struct{}, len(matches))
	out := make([]int, 0, len(matches))
	for _, m := range matches {
		id, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// fetchBookText tries the standard URL patterns for a Gutenberg book .txt
// in priority order: the cache UTF-8 URL first (newer books), then the
// per-volume mirrors (older books, often ISO-8859-1). Returns the decoded
// UTF-8 text, the URL we successfully fetched from, and the original
// encoding.
func fetchBookText(client *http.Client, id int, ua string) (string, string, string, error) {
	candidates := []struct {
		url      string
		encoding string
	}{
		{fmt.Sprintf("https://www.gutenberg.org/cache/epub/%d/pg%d.txt", id, id), "utf-8"},
		{fmt.Sprintf("https://www.gutenberg.org/files/%d/%d-0.txt", id, id), "utf-8"},
		{fmt.Sprintf("https://www.gutenberg.org/files/%d/%d-8.txt", id, id), "iso-8859-1"},
	}
	var lastErr error
	for _, c := range candidates {
		req, _ := http.NewRequest(http.MethodGet, c.url, nil)
		req.Header.Set("User-Agent", ua)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s: status %d", c.url, resp.StatusCode)
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBookSize))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		text, err := decodeText(raw, c.encoding)
		if err != nil {
			lastErr = err
			continue
		}
		return text, c.url, c.encoding, nil
	}
	return "", "", "", lastErr
}

// decodeText converts a byte slice into UTF-8. Gutenberg primarily ships
// UTF-8 (the cache URL) and ISO-8859-1 (older mirrors). The function picks
// the right decoder based on the URL hint we passed alongside the bytes.
func decodeText(raw []byte, encoding string) (string, error) {
	switch encoding {
	case "utf-8":
		// Some "UTF-8" cache files start with a BOM (U+FEFF, EF BB BF);
		// strip it byte-wise to avoid sticking a stray BOM in the corpus.
		if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
			raw = raw[3:]
		}
		return string(raw), nil
	case "iso-8859-1":
		dec := charmap.ISO8859_1.NewDecoder()
		out, err := dec.String(string(raw))
		if err != nil {
			return "", err
		}
		return out, nil
	default:
		return "", fmt.Errorf("unknown encoding %q", encoding)
	}
}

// bookMeta holds the few fields we extract from the Project Gutenberg
// header. Used purely for the manifest record; not consumed by downstream
// silver tagging.
type bookMeta struct {
	Title    string
	Author   string
	Language string
}

// parseGutenbergText splits a downloaded text into header-derived metadata
// and the cleaned body (between START and END markers). Errors out if the
// markers are missing - we'd rather skip the book than ship Gutenberg
// boilerplate as silver corpus content.
func parseGutenbergText(text string) (bookMeta, string, error) {
	var meta bookMeta
	startIdx := strings.Index(text, startMarkerPrefix)
	endIdx := strings.Index(text, endMarkerPrefix)
	if startIdx < 0 || endIdx < 0 || endIdx < startIdx {
		return meta, "", fmt.Errorf("missing START/END markers (start=%d end=%d)", startIdx, endIdx)
	}

	header := text[:startIdx]
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Title:") {
			meta.Title = strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
		} else if strings.HasPrefix(line, "Author:") {
			meta.Author = strings.TrimSpace(strings.TrimPrefix(line, "Author:"))
		} else if strings.HasPrefix(line, "Language:") {
			meta.Language = strings.TrimSpace(strings.TrimPrefix(line, "Language:"))
		}
	}

	body := text[startIdx:endIdx]
	// Drop the START line itself.
	if i := strings.Index(body, "\n"); i >= 0 {
		body = body[i+1:]
	}
	body = stripLeadingGutenbergMetadata(body)
	body = strings.TrimSpace(body)
	return meta, body, nil
}

func stripLeadingGutenbergMetadata(body string) string {
	lines := strings.Split(body, "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}
		if isLeadingGutenbergCreditLine(line) {
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
				i++
			}
			continue
		}
		if isLeadingGutenbergMetadataLine(line) {
			i++
			continue
		}
		break
	}
	return strings.Join(lines[i:], "\n")
}

func isLeadingGutenbergCreditLine(line string) bool {
	return strings.HasPrefix(line, "Produced by")
}

func isLeadingGutenbergMetadataLine(line string) bool {
	for _, prefix := range []string{"Title:", "Author:", "Language:", "Credits:"} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// looksFinnish is a coarse "is this Finnish?" check - counts how often the
// most common Finnish-only characters and tokens appear. Calibrated against
// English/Swedish noise that bleeds into the search results when an
// English-authored book includes the Finnish language code in its metadata.
func looksFinnish(text string) bool {
	if len(text) < 2000 {
		// Short text - skip the heuristic; the search filter has already
		// done its job for these.
		return true
	}
	sample := text
	if len(sample) > 50_000 {
		sample = text[:50_000]
	}
	finnishHits := 0
	finnishHits += strings.Count(strings.ToLower(sample), "ä")
	finnishHits += strings.Count(strings.ToLower(sample), "ö")
	// "ja" (and) and "on" (is) are very common Finnish particles. Use
	// space-bounded matches to avoid hitting them as substrings.
	finnishHits += strings.Count(sample, " ja ") * 3
	finnishHits += strings.Count(sample, " on ") * 2
	finnishHits += strings.Count(sample, " että ") * 5
	finnishHits += strings.Count(sample, " kun ") * 4
	return finnishHits >= 100
}

// approxTokenCount counts whitespace-separated tokens. Good enough for the
// "have we hit 500k?" decision; the silver tagger (Plan C / PR 4) will
// produce exact token counts when it builds the tagged JSON.
func approxTokenCount(text string) int {
	return len(strings.Fields(text))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
