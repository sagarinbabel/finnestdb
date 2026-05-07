// cmd/fetchfrequency downloads public Finnish/Estonian frequency baselines
// into localdata/frequency/{fi,et}/. These lists are NOT committed (per
// docs/ARTIFACT_POLICY.md "Single-folder bootstrap rule"); they live in
// localdata/ and are re-fetched on demand.
//
// Methodology, coverage curves, and license attribution: see
// docs/FREQUENCY_BASELINES.md. Ledger entries: docs/data_enhancement.md §5.
//
// Sources:
//
//   - Hermit Dave OpenSubtitles 2018 top-50k word lists (CC BY-SA 4.0):
//     https://github.com/hermitdave/FrequencyWords
//   - UD-Finnish-TDT train CoNLL-U (CC BY-SA 4.0):
//     https://github.com/UniversalDependencies/UD_Finnish-TDT
//   - UD-Estonian-EDT train CoNLL-U (CC BY-NC-SA 4.0):
//     https://github.com/UniversalDependencies/UD_Estonian-EDT
//
// For OpenSubtitles, the raw file is saved verbatim. For UD treebanks, the
// train CoNLL-U is downloaded then reduced into three TSVs:
//
//   - <prefix>-forms.tsv:        form<TAB>count, all tokens
//   - <prefix>-forms-words.tsv:  form<TAB>count, UPOS != PUNCT/SYM
//   - <prefix>-lemmas.tsv:       lemma<TAB>count, all tokens
//
// Re-running with no upstream change produces byte-identical files.
//
// Usage:
//
//	go run ./cmd/fetchfrequency           # download + derive everything
//	go run ./cmd/fetchfrequency -force    # ignore existing files
//	go run ./cmd/fetchfrequency -out DIR  # override output root
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultOutDir   = "localdata/frequency"
	defaultUserAgent = "finnestdb/fetchfrequency (+https://github.com/sagarinbabel/finnestdb)"
	httpTimeout     = 60 * time.Second
)

type kind int

const (
	kindRaw kind = iota // save HTTP body verbatim
	kindUD              // download CoNLL-U then derive forms/words/lemmas TSVs
)

type source struct {
	Lang     string
	Kind     kind
	URL      string
	OutName  string // for kindRaw: filename to save as
	UDPrefix string // for kindUD: filename prefix for derived TSVs
}

var sources = []source{
	{
		Lang:    "fi",
		Kind:    kindRaw,
		URL:     "https://raw.githubusercontent.com/hermitdave/FrequencyWords/master/content/2018/fi/fi_50k.txt",
		OutName: "opensubtitles-2018-fi-50k.txt",
	},
	{
		Lang:    "et",
		Kind:    kindRaw,
		URL:     "https://raw.githubusercontent.com/hermitdave/FrequencyWords/master/content/2018/et/et_50k.txt",
		OutName: "opensubtitles-2018-et-50k.txt",
	},
	{
		Lang:     "fi",
		Kind:     kindUD,
		URL:      "https://raw.githubusercontent.com/UniversalDependencies/UD_Finnish-TDT/master/fi_tdt-ud-train.conllu",
		UDPrefix: "UD_Finnish-TDT",
	},
	{
		Lang:     "et",
		Kind:     kindUD,
		URL:      "https://raw.githubusercontent.com/UniversalDependencies/UD_Estonian-EDT/master/et_edt-ud-train.conllu",
		UDPrefix: "UD_Estonian-EDT",
	},
}

func main() {
	outDir := flag.String("out", defaultOutDir, "Output root directory; per-language subdirs created underneath")
	force := flag.Bool("force", false, "Re-download and re-derive even if outputs exist")
	flag.Parse()

	logger := log.New(os.Stderr, "fetchfrequency: ", log.LstdFlags)
	client := &http.Client{Timeout: httpTimeout}

	var fetched, skipped, derived int

	for _, src := range sources {
		langDir := filepath.Join(*outDir, src.Lang)
		if err := os.MkdirAll(langDir, 0o755); err != nil {
			logger.Fatalf("mkdir %s: %v", langDir, err)
		}

		switch src.Kind {
		case kindRaw:
			path := filepath.Join(langDir, src.OutName)
			if !*force && exists(path) {
				logger.Printf("skip %s (exists; -force to refetch)", path)
				skipped++
				continue
			}
			if err := download(client, src.URL, path); err != nil {
				logger.Fatalf("fetch %s: %v", src.URL, err)
			}
			logger.Printf("wrote %s", path)
			fetched++

		case kindUD:
			// Outputs are three derived TSVs alongside the source CoNLL-U.
			conllu := filepath.Join(langDir, src.UDPrefix+"-train.conllu")
			formsAll := filepath.Join(langDir, src.UDPrefix+"-forms.tsv")
			formsWords := filepath.Join(langDir, src.UDPrefix+"-forms-words.tsv")
			lemmas := filepath.Join(langDir, src.UDPrefix+"-lemmas.tsv")

			if !*force && exists(formsAll) && exists(formsWords) && exists(lemmas) {
				logger.Printf("skip %s derivations (all exist; -force to redo)", src.UDPrefix)
				skipped++
				continue
			}
			if !exists(conllu) || *force {
				if err := download(client, src.URL, conllu); err != nil {
					logger.Fatalf("fetch %s: %v", src.URL, err)
				}
				logger.Printf("wrote %s", conllu)
				fetched++
			}
			if err := deriveUD(conllu, formsAll, formsWords, lemmas); err != nil {
				logger.Fatalf("derive %s: %v", src.UDPrefix, err)
			}
			logger.Printf("derived %s, %s, %s", filepath.Base(formsAll), filepath.Base(formsWords), filepath.Base(lemmas))
			derived += 3
		}
	}

	logger.Printf("done: %d fetched, %d skipped, %d derived TSVs", fetched, skipped, derived)
}

// download HTTP-GETs url and writes the body to path atomically (write to
// path.tmp then rename), returning any error.
func download(client *http.Client, url, path string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// deriveUD reads a CoNLL-U file and writes three sorted-by-count TSVs:
// forms (all), forms-words (UPOS != PUNCT/SYM), and lemmas.
func deriveUD(conlluPath, formsAllPath, formsWordsPath, lemmasPath string) error {
	formsAll := map[string]int{}
	formsWords := map[string]int{}
	lemmas := map[string]int{}

	f, err := os.Open(conlluPath)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 5 {
			continue
		}
		// Skip multi-word tokens (ID like "1-2") and empty-node IDs ("1.1").
		id := cols[0]
		if strings.ContainsAny(id, "-.") {
			continue
		}
		form := cols[1]
		lemma := cols[2]
		upos := cols[3]
		formsAll[form]++
		lemmas[lemma]++
		if upos != "PUNCT" && upos != "SYM" {
			formsWords[form]++
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	if err := writeRanked(formsAllPath, formsAll); err != nil {
		return err
	}
	if err := writeRanked(formsWordsPath, formsWords); err != nil {
		return err
	}
	if err := writeRanked(lemmasPath, lemmas); err != nil {
		return err
	}
	return nil
}

// writeRanked sorts counts descending (ties broken by token ascending) and
// writes one "<token>\t<count>" line per entry to path atomically.
func writeRanked(path string, counts map[string]int) error {
	type pair struct {
		Tok string
		N   int
	}
	pairs := make([]pair, 0, len(counts))
	for tok, n := range counts {
		pairs = append(pairs, pair{tok, n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].N != pairs[j].N {
			return pairs[i].N > pairs[j].N
		}
		return pairs[i].Tok < pairs[j].Tok
	})

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, p := range pairs {
		if _, err := fmt.Fprintf(w, "%s\t%d\n", p.Tok, p.N); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
