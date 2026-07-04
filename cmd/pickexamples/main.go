// Command pickexamples selects "beautiful evocative" corpus example sentences
// for the cold-start "Top N words" starter deck and emits a small checked-in
// artifact (testdata/starter-examples/<lang>-examples-v1.tsv) that
// cmd/seedcolddeck attaches to each deck card.
//
// Owner decision (2026-07-04): starter-deck cards must carry example sentences,
// and individual sentences drawn from the local licensed corpora are an
// acceptable source — single sentences used as dictionary-style usage examples,
// never bulk text reproduction. See testdata/starter-examples/README.md.
//
// Ranking reuse: the target lemmas are exactly seedcolddeck's Top-N ranking
// (internal/starterdeck.TopLemmas over the same OpenSubtitles baseline), so
// every deck card has a matching example row.
//
// How selection works (index, not a 66M-line scan):
//
//	Pass 1  Stream localdata/<lang>-corpus/_derived/wordlist_user_friendly.tsv.
//	        Each row is a distinct surface analysis carrying the pipeline's
//	        chosen example sentence id (example_ref_id) for that form. For every
//	        row whose (lemma, pos) is a target and whose surface is an inflected
//	        form (not the bare lemma) chosen by the parser, remember the form,
//	        its corpus count, and the example sentence id. This yields a bounded
//	        set of "needed" sentence ids (~tens per lemma) without touching the
//	        multi-GB sentence text at all.
//	Pass 2  Stream sentences_user_friendly.tsv once, keeping only the text of
//	        the needed ids. (Sentence ids are not line-contiguous, so a single
//	        streamed pass with a membership set is the cheap correct join.)
//	Select  Apply the deterministic quality heuristics (select.go) per lemma and
//	        keep the best 1-2 sentences.
//
// Both passes are single streaming scans with bounded memory. Nothing is read
// whole-file. Progress is logged per phase and every progressEvery lines.
//
// Usage:
//
//	go run ./cmd/pickexamples -db finnestdb.db -lang FI -top 1000 \
//	    -out testdata/starter-examples/fi-examples-v1.tsv
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"finnestdb/internal/starterdeck"
	"finnestdb/internal/store"
)

const (
	// progressEvery bounds how often long streaming scans log progress, so a
	// multi-GB pass is diagnosable while it runs instead of going silent.
	progressEvery = 5_000_000

	// examplesPerLemma is how many curated sentences we keep per deck card.
	// One shows usage; a second gives the learner an alternative inflection to
	// generalize from. The artifact stays small: <= this * top rows.
	examplesPerLemma = 2

	// maxIDsPerLemma caps how many candidate sentence ids we hold per lemma
	// before selection. Each inflected form contributes one id; this bounds a
	// pathological high-frequency lemma (e.g. "olla" with hundreds of forms)
	// so memory stays predictable. Candidates are gathered highest-count-first.
	maxIDsPerLemma = 60

	// minFormCount drops candidate forms seen fewer than this many times in the
	// corpus. Garbled parser analyses of noise ("einsä", "mItä", "aIka") sit at
	// single-digit counts, while real inflections are far more common; this
	// floor removes the noise without touching any genuine everyday form.
	minFormCount = 50
)

// wordlist_user_friendly.tsv column indices (0-based), pinned to the header
// verified against localdata/<lang>-corpus/_derived/wordlist_user_friendly.tsv.
const (
	wlSurface          = 0
	wlLemma            = 3
	wlPOS              = 4
	wlSurfaceCountTot  = 15
	wlIsParserChoice   = 21
	wlExampleRefType   = 25
	wlExampleRefID     = 26
	wlExpectedColumns  = 27
)

func main() {
	dbPath := flag.String("db", "finnestdb.db", "Path to SQLite database")
	lang := flag.String("lang", "", "Language (FI or ET)")
	freqSource := flag.String("source", "", "Frequency list path (default: localdata/frequency/<lang>/opensubtitles-2018-<lang>-50k.txt)")
	corpusDir := flag.String("corpus-dir", "", "Corpus _derived dir (default: localdata/<lang>-corpus/_derived)")
	top := flag.Int("top", 1000, "Number of top lemmas to select examples for")
	out := flag.String("out", "", "Output TSV path (default: testdata/starter-examples/<lang>-examples-v1.tsv)")
	flag.Parse()

	if *lang != "FI" && *lang != "ET" {
		log.Fatal("-lang must be FI or ET")
	}
	if *top <= 0 {
		log.Fatal("-top must be positive")
	}
	langLower := strings.ToLower(*lang)
	if *freqSource == "" {
		*freqSource = filepath.Join("localdata", "frequency", langLower,
			fmt.Sprintf("opensubtitles-2018-%s-50k.txt", langLower))
	}
	if *corpusDir == "" {
		*corpusDir = filepath.Join("localdata", fmt.Sprintf("%s-corpus", langLower), "_derived")
	}
	if *out == "" {
		*out = filepath.Join("testdata", "starter-examples", fmt.Sprintf("%s-examples-v1.tsv", langLower))
	}
	wordlistPath := filepath.Join(*corpusDir, "wordlist_user_friendly.tsv")
	sentencesPath := filepath.Join(*corpusDir, "sentences_user_friendly.tsv")

	db, err := store.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	// --- Ranking: reuse seedcolddeck's Top-N lemma set ---------------------
	log.Printf("[%s] phase 1/4: ranking top %d lemmas from %s", *lang, *top, *freqSource)
	freqFile, err := os.Open(*freqSource)
	if err != nil {
		log.Fatalf("open frequency list: %v", err)
	}
	entries, skipped, err := starterdeck.TopLemmas(freqFile, db, *lang, *top)
	freqFile.Close()
	if err != nil {
		log.Fatalf("rank lemmas: %v", err)
	}
	if len(entries) == 0 {
		log.Fatal("no lemmas resolved — is the dictionary imported for this language?")
	}
	log.Printf("[%s] ranked %d lemmas (%d list forms unresolved)", *lang, len(entries), skipped)

	targets := make(map[starterdeck.LemmaKey]bool, len(entries))
	for _, e := range entries {
		targets[starterdeck.LemmaKey{Lemma: e.Lemma, POS: e.POS}] = true
	}

	// --- Frequency ranks for the readability heuristic ---------------------
	freqRanks, err := loadFreqRanks(*freqSource)
	if err != nil {
		log.Fatalf("load frequency ranks: %v", err)
	}
	log.Printf("[%s] loaded %d frequency ranks for readability scoring", *lang, len(freqRanks))

	// --- Pass 1: build the lemma -> candidate-ids index from the wordlist ---
	log.Printf("[%s] phase 2/4: indexing candidate example ids from %s", *lang, wordlistPath)
	perLemma, neededIDs, err := indexCandidates(wordlistPath, targets)
	if err != nil {
		log.Fatalf("index candidates: %v", err)
	}
	logMem()
	log.Printf("[%s] indexed candidates for %d/%d lemmas; %d distinct sentence ids needed",
		*lang, len(perLemma), len(entries), len(neededIDs))

	// --- Pass 2: fetch the text of just the needed sentence ids ------------
	log.Printf("[%s] phase 3/4: fetching %d sentence texts from %s", *lang, len(neededIDs), sentencesPath)
	texts, err := fetchSentences(sentencesPath, neededIDs)
	if err != nil {
		log.Fatalf("fetch sentences: %v", err)
	}
	logMem()
	log.Printf("[%s] fetched %d/%d sentence texts", *lang, len(texts), len(neededIDs))

	// --- Select + emit -----------------------------------------------------
	log.Printf("[%s] phase 4/4: selecting best %d examples/lemma and writing %s", *lang, examplesPerLemma, *out)
	rows := selectRows(entries, perLemma, texts, freqRanks, langLower)
	if err := writeArtifact(*out, rows); err != nil {
		log.Fatalf("write artifact: %v", err)
	}

	lemmasWithExample := countDistinctLemmas(rows)
	log.Printf("[%s] done: %d rows for %d/%d lemmas -> %s",
		*lang, len(rows), lemmasWithExample, len(entries), *out)
	fmt.Printf("%s: %d example rows covering %d of %d lemmas written to %s\n",
		*lang, len(rows), lemmasWithExample, len(entries), *out)
}

// formEntry is one candidate inflected form of a target lemma, carrying the
// pipeline's example sentence id for that form.
type formEntry struct {
	Form      string
	Count     int64
	ExampleID int64
}

// indexCandidates streams the wordlist once and returns, per target (lemma,
// pos), the candidate inflected forms (highest count first, capped) plus the
// union of their example sentence ids. Bounded memory: only target lemmas are
// retained, and per-lemma forms are capped at maxIDsPerLemma.
func indexCandidates(path string, targets map[starterdeck.LemmaKey]bool) (map[starterdeck.LemmaKey][]formEntry, map[int64]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	perLemma := make(map[starterdeck.LemmaKey][]formEntry)
	needed := make(map[int64]struct{})

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)
	line := 0
	start := time.Now()
	for sc.Scan() {
		line++
		if line == 1 {
			continue // header
		}
		if line%progressEvery == 0 {
			log.Printf("  ...wordlist line %d (%.0fs)", line, time.Since(start).Seconds())
		}
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < wlExpectedColumns {
			continue
		}
		lemma := fields[wlLemma]
		pos := fields[wlPOS]
		key := starterdeck.LemmaKey{Lemma: lemma, POS: pos}
		if !targets[key] {
			continue
		}
		surface := fields[wlSurface]
		// Inflected forms only: showing the bare lemma teaches no inflection,
		// and seedcolddeck already falls back to the lemma form on its own.
		if strings.EqualFold(surface, lemma) {
			continue
		}
		// Trust the parser's chosen analysis; alt analyses of an ambiguous
		// surface point at the same sentence but a different (wrong) lemma.
		if fields[wlIsParserChoice] != "1" {
			continue
		}
		if fields[wlExampleRefType] != "sentence" {
			continue
		}
		exID, err := strconv.ParseInt(fields[wlExampleRefID], 10, 64)
		if err != nil || exID <= 0 {
			continue
		}
		count, _ := strconv.ParseInt(fields[wlSurfaceCountTot], 10, 64)
		if count < minFormCount {
			continue
		}
		perLemma[key] = append(perLemma[key], formEntry{Form: surface, Count: count, ExampleID: exID})
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}

	// Cap and collect needed ids after the scan so we keep the highest-count
	// forms per lemma (best canonical inflections) within the cap.
	for key, forms := range perLemma {
		sort.Slice(forms, func(i, j int) bool { return forms[i].Count > forms[j].Count })
		if len(forms) > maxIDsPerLemma {
			forms = forms[:maxIDsPerLemma]
			perLemma[key] = forms
		}
		for _, fe := range forms {
			needed[fe.ExampleID] = struct{}{}
		}
	}
	return perLemma, needed, nil
}

// fetchSentences streams the user-friendly sentence bank once and returns the
// text of just the needed ids. Sentence ids are not line-contiguous, so a
// single streamed membership join is the cheapest correct lookup. Memory is
// bounded by the needed set (tens of thousands of short strings), not the file.
func fetchSentences(path string, needed map[int64]struct{}) (map[int64]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	texts := make(map[int64]string, len(needed))
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)
	line := 0
	start := time.Now()
	for sc.Scan() {
		line++
		if line == 1 {
			continue // header: id, lang, text
		}
		if line%progressEvery == 0 {
			log.Printf("  ...sentences line %d (%.0fs, %d/%d found)",
				line, time.Since(start).Seconds(), len(texts), len(needed))
		}
		// Only split the two leading tab boundaries; the text may itself be
		// tab-free but we avoid over-splitting long sentences.
		row := sc.Text()
		firstTab := strings.IndexByte(row, '\t')
		if firstTab < 0 {
			continue
		}
		id, err := strconv.ParseInt(row[:firstTab], 10, 64)
		if err != nil {
			continue
		}
		if _, want := needed[id]; !want {
			continue
		}
		rest := row[firstTab+1:]
		secondTab := strings.IndexByte(rest, '\t')
		if secondTab < 0 {
			continue
		}
		texts[id] = strings.TrimSpace(rest[secondTab+1:])
		if len(texts) == len(needed) {
			// Every needed sentence found; no reason to scan the rest.
			log.Printf("  ...all %d needed sentences found at line %d; stopping scan", len(needed), line)
			break
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return texts, nil
}

// artifactRow is one emitted TSV row.
type artifactRow struct {
	Lemma        string
	POS          string
	Form         string
	Sentence     string
	SourceCorpus string
}

// selectRows applies the quality heuristics per lemma (in ranking order) and
// keeps up to examplesPerLemma sentences each. Ranking order makes the output
// stable and readable (most important words first).
func selectRows(entries []starterdeck.LemmaEntry, perLemma map[starterdeck.LemmaKey][]formEntry, texts map[int64]string, freqRanks map[string]int, corpusTag string) []artifactRow {
	var rows []artifactRow
	for _, e := range entries {
		key := starterdeck.LemmaKey{Lemma: e.Lemma, POS: e.POS}
		forms := perLemma[key]
		if len(forms) == 0 {
			continue
		}
		cands := make([]Candidate, 0, len(forms))
		for _, fe := range forms {
			text := texts[fe.ExampleID]
			if text == "" {
				continue
			}
			cands = append(cands, Candidate{
				SentenceID:   fe.ExampleID,
				Text:         text,
				Form:         fe.Form,
				FormCount:    fe.Count,
				SourceCorpus: corpusTag,
			})
		}
		best := pickBest(cands, freqRanks, examplesPerLemma)
		for _, c := range best {
			rows = append(rows, artifactRow{
				Lemma:        e.Lemma,
				POS:          e.POS,
				Form:         c.Form,
				Sentence:     c.Text,
				SourceCorpus: c.SourceCorpus,
			})
		}
	}
	return rows
}

func writeArtifact(path string, rows []artifactRow) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	if _, err := fmt.Fprintln(w, strings.Join([]string{"lemma", "pos", "form", "sentence", "source_corpus"}, "\t")); err != nil {
		return err
	}
	for _, r := range rows {
		// TSV rows must not contain literal tabs/newlines in fields; corpus
		// sentences are single-line, but guard anyway so the file stays valid.
		line := strings.Join([]string{
			sanitizeField(r.Lemma),
			sanitizeField(r.POS),
			sanitizeField(r.Form),
			sanitizeField(r.Sentence),
			sanitizeField(r.SourceCorpus),
		}, "\t")
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func sanitizeField(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

func countDistinctLemmas(rows []artifactRow) int {
	seen := make(map[starterdeck.LemmaKey]struct{})
	for _, r := range rows {
		seen[starterdeck.LemmaKey{Lemma: r.Lemma, POS: r.POS}] = struct{}{}
	}
	return len(seen)
}

// loadFreqRanks loads the OpenSubtitles "form count" list into a lowercased
// form -> 1-based rank map (rank 1 = most common). First occurrence wins.
func loadFreqRanks(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ranks := make(map[string]int)
	sc := bufio.NewScanner(f)
	rank := 0
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		form := strings.ToLower(fields[0])
		if form == "" {
			continue
		}
		rank++
		if _, seen := ranks[form]; !seen {
			ranks[form] = rank
		}
	}
	return ranks, sc.Err()
}

func logMem() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf("  mem: heap=%dMB sys=%dMB", m.HeapAlloc>>20, m.Sys>>20)
}
