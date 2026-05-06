// cmd/scraperunosto fetches Finnish poems from runosto.net via its
// WordPress REST API and saves them as plain-text files for use as a
// silver-tier adversarial corpus. Plan C / PR 7 — see docs/CHANGELOG.md.
//
// Source: https://runosto.net/wp-json/wp/v2/posts (WP REST API).
// At the time of writing the site advertises ~1,343 posts, each one a
// poem. Public-domain Finnish poetry — Kanteletar excerpts, Eino
// Leino, Aleksis Kivi short verse, etc.
//
// Why a *separate* scraper from cmd/scrapegutenberg: the source shape
// is structurally different — REST API instead of HTML pages,
// per-post HTML content instead of plain-text downloads, no boilerplate
// markers. Sharing code would mean a generic scraper with two
// configurations; the abstraction overhead is worse than the duplication.
//
// Output:
//   data/silver-fi-runosto/raw/<post_id>.txt    — cleaned poem text
//   data/silver-fi-runosto/manifest.jsonl       — one JSON record per poem
//
// Both gitignored at the repo level (per data/silver-fi/ convention).
//
// Usage:
//   go run ./cmd/scraperunosto \
//       -target-tokens 200000 \
//       -out data/silver-fi-runosto/raw \
//       -manifest data/silver-fi-runosto/manifest.jsonl
//
// Polite — 800 ms between requests, single connection. Re-runs are
// idempotent — manifest is checked first, already-fetched IDs skipped.
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
)

const (
	defaultUserAgent = "finnestdb-scraperunosto/0.1 (+https://github.com/sagarinbabel/finnestdb)"
	requestDelay     = 800 * time.Millisecond
	requestTimeout   = 30 * time.Second
	postsAPIBase     = "https://runosto.net/wp-json/wp/v2/posts"
	pageSize         = 100
	maxPages         = 50 // 50 × 100 = 5000 candidate posts
)

func main() {
	var (
		targetTokens = flag.Int("target-tokens", 200_000, "Stop once cumulative token count crosses this threshold (0 = fetch all available)")
		outDir       = flag.String("out", "data/silver-fi-runosto/raw", "Directory for cleaned per-poem .txt files")
		manifestPath = flag.String("manifest", "data/silver-fi-runosto/manifest.jsonl", "JSONL manifest of fetched poems")
		userAgent    = flag.String("user-agent", defaultUserAgent, "User-Agent for HTTP requests")
		dryRun       = flag.Bool("dry-run", false, "List post IDs that would be fetched without downloading")
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
	log.Printf("manifest has %d poems / %d tokens; target=%d",
		len(already), cumulativeTokens, *targetTokens)
	if *targetTokens > 0 && cumulativeTokens >= *targetTokens {
		log.Printf("target already met — exiting")
		return
	}

	client := &http.Client{Timeout: requestTimeout}
	manifestFile, err := os.OpenFile(*manifestPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("open manifest: %v", err)
	}
	defer manifestFile.Close()

	for page := 1; page <= maxPages; page++ {
		time.Sleep(requestDelay)
		posts, err := fetchPostsPage(client, page, *userAgent)
		if err != nil {
			log.Printf("page %d: %v", page, err)
			continue
		}
		if len(posts) == 0 {
			log.Printf("page %d empty; stopping", page)
			break
		}

		for _, p := range posts {
			if _, ok := already[p.ID]; ok {
				continue
			}
			if *dryRun {
				log.Printf("[dry-run] would fetch %d %q", p.ID, p.Title.Rendered)
				continue
			}

			body := stripHTML(p.Content.Rendered)
			body = strings.TrimSpace(body)
			if body == "" {
				continue
			}
			tokens := approxTokenCount(body)
			outPath := filepath.Join(*outDir, fmt.Sprintf("%d.txt", p.ID))
			if err := os.WriteFile(outPath, []byte(body), 0o644); err != nil {
				log.Printf("write %d: %v", p.ID, err)
				continue
			}

			rec := manifestRecord{
				ID:        p.ID,
				Title:     stripHTML(p.Title.Rendered),
				Slug:      p.Slug,
				Tokens:    tokens,
				SourceURL: fmt.Sprintf("https://runosto.net/?p=%d", p.ID),
				FetchedAt: time.Now().UTC().Format(time.RFC3339),
				Path:      outPath,
			}
			line, _ := json.Marshal(rec)
			if _, err := manifestFile.Write(append(line, '\n')); err != nil {
				log.Fatalf("write manifest: %v", err)
			}
			already[p.ID] = rec
			cumulativeTokens += tokens
			log.Printf("[%5d tok cum=%d/%d] %d %q",
				tokens, cumulativeTokens, *targetTokens, p.ID, rec.Title)
			if *targetTokens > 0 && cumulativeTokens >= *targetTokens {
				log.Printf("target met (%d ≥ %d); stopping", cumulativeTokens, *targetTokens)
				return
			}
		}
	}
	log.Printf("finished; cumulative=%d/%d", cumulativeTokens, *targetTokens)
}

// manifestRecord captures everything needed to reproduce a fetch and
// attribute the source.
type manifestRecord struct {
	ID        int    `json:"id"`
	Title     string `json:"title,omitempty"`
	Slug      string `json:"slug,omitempty"`
	Tokens    int    `json:"tokens"`
	SourceURL string `json:"source_url"`
	FetchedAt string `json:"fetched_at"`
	Path      string `json:"path"`
}

// wpPost is the subset of the WP REST `/posts` shape we consume.
type wpPost struct {
	ID    int `json:"id"`
	Slug  string `json:"slug"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
}

// fetchPostsPage returns posts for one page of the WP REST endpoint.
// per_page=pageSize ensures consistent pagination.
func fetchPostsPage(client *http.Client, page int, ua string) ([]wpPost, error) {
	url := fmt.Sprintf("%s?per_page=%d&page=%d&_fields=id,slug,title,content", postsAPIBase, pageSize, page)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		// 400 with rest_post_invalid_page_number means we're past the end.
		if resp.StatusCode == 400 {
			return nil, nil
		}
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, err
	}
	var posts []wpPost
	if err := json.Unmarshal(body, &posts); err != nil {
		return nil, err
	}
	return posts, nil
}

// loadManifest reads any existing manifest so re-runs are incremental.
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
			return nil, fmt.Errorf("parse manifest: %w", err)
		}
		out[rec.ID] = rec
	}
	return out, sc.Err()
}

var (
	htmlTagRE     = regexp.MustCompile(`<[^>]+>`)
	htmlEntityRE  = regexp.MustCompile(`&#?\w+;`)
	multiWhiteRE  = regexp.MustCompile(`[ \t]+`)
	multiNewlineRE = regexp.MustCompile(`\n{3,}`)
)

// stripHTML turns runosto's WP-rendered HTML into plain text. WordPress
// emits paragraph and line-break tags that we map to newlines so the
// silver tagger's sentence splitter behaves; everything else is dropped.
//
// We deliberately avoid a full HTML parser — runosto's content is just
// poems wrapped in <p>...<br/>...</p>. Tag-and-entity stripping is
// sufficient and faster.
func stripHTML(s string) string {
	// Replace structural tags with newlines so the splitter sees boundaries.
	r := strings.NewReplacer(
		"<br />", "\n",
		"<br/>", "\n",
		"<br>", "\n",
		"</p>", "\n\n",
		"<p>", "",
		"<p ", "<p ",
	)
	s = r.Replace(s)
	s = htmlTagRE.ReplaceAllString(s, "")
	s = htmlEntityRE.ReplaceAllStringFunc(s, decodeEntity)
	s = multiWhiteRE.ReplaceAllString(s, " ")
	s = multiNewlineRE.ReplaceAllString(s, "\n\n")
	return s
}

// decodeEntity handles the small set of HTML entities WordPress emits.
// Unknown entities pass through unchanged — better than dropping them.
func decodeEntity(e string) string {
	switch e {
	case "&amp;":
		return "&"
	case "&lt;":
		return "<"
	case "&gt;":
		return ">"
	case "&quot;":
		return `"`
	case "&apos;":
		return "'"
	case "&nbsp;":
		return " "
	}
	if strings.HasPrefix(e, "&#") && strings.HasSuffix(e, ";") {
		num := strings.TrimSuffix(strings.TrimPrefix(e, "&#"), ";")
		if strings.HasPrefix(num, "x") || strings.HasPrefix(num, "X") {
			if n, err := strconv.ParseInt(num[1:], 16, 32); err == nil {
				return string(rune(n))
			}
		} else if n, err := strconv.Atoi(num); err == nil {
			return string(rune(n))
		}
	}
	return e
}

// approxTokenCount: whitespace splitting. Same convention as
// cmd/scrapegutenberg.
func approxTokenCount(text string) int {
	return len(strings.Fields(text))
}
