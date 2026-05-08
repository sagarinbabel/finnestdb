// cmd/aggregatecorpus walks per-source extracted text and emits the v1
// derived TSVs (wordlist, sentences, sentence_occurrences, poems,
// documents, manifest) plus mining TSVs plus build_metadata.json +
// qa-report.json.
//
// v1 smoke vertical slice: in-memory aggregation. Once the smoke profile
// is green, scale features (SQLite scratch, deterministic ID assignment
// across concurrent workers, surface-analyses cache) get layered in for
// pilot/full.
//
// Usage:
//
//	go run ./cmd/aggregatecorpus -lang fi [-data-root ../localdata] [-only <slug>]
package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/parserffi"
	"finnestdb/internal/store"
	lemmatizer "finnestdb/pkg/lemmatizer-fi-et"

	"finnestdb/corpus_pipeline/internal/cli"
	"finnestdb/corpus_pipeline/internal/sources"
	"finnestdb/corpus_pipeline/internal/textfilter"
)

func main() {
	var (
		dataRoot   = flag.String("data-root", "../localdata", "")
		lang       = flag.String("lang", "fi", "")
		dbPath     = flag.String("db", "", "path to finnestdb.db (default: <repoRoot>/finnestdb.db)")
		onlySource = flag.String("only", "", "if set, aggregate only this source slug")
		skipFlag   = flag.String("skip", "", "comma-separated source slugs to exclude (e.g. for memory-bounded runs)")
		useScratch = flag.Bool("scratch", false, "use SQLite scratch DB for sentences/occurrences/wordlist (lower RSS, slightly slower)")
	)
	flag.Parse()

	roots, err := cli.Resolve(*dataRoot, *dbPath)
	if err != nil {
		log.Fatalf("resolve: %v", err)
	}
	langLower, langUpper, err := cli.LangCodes(*lang)
	if err != nil {
		log.Fatalf("lang: %v", err)
	}
	if err := cli.PreflightTables(roots.TablesDir); err != nil {
		log.Fatalf("preflight tables: %v", err)
	}
	if err := cli.PreflightDB(roots.DBPath, langUpper); err != nil {
		log.Fatalf("preflight db: %v", err)
	}

	manifests, err := sources.Discover(roots.DataRoot, langLower)
	if err != nil {
		log.Fatalf("discover: %v", err)
	}
	if *onlySource != "" {
		manifests = filterSources(manifests, *onlySource)
	}
	if *skipFlag != "" {
		skipSet := map[string]bool{}
		for _, s := range strings.Split(*skipFlag, ",") {
			skipSet[strings.TrimSpace(s)] = true
		}
		var kept []sources.Manifest
		for _, m := range manifests {
			if !skipSet[m.Slug] {
				kept = append(kept, m)
			}
		}
		manifests = kept
	}
	if len(manifests) == 0 {
		log.Fatalf("no sources to aggregate (lang=%s)", langLower)
	}

	derived := sources.DerivedDir(roots.DataRoot, langLower)
	if err := os.MkdirAll(filepath.Join(derived, "mining"), 0o755); err != nil {
		log.Fatalf("mkdir derived: %v", err)
	}

	state := newState(roots, langLower, langUpper, manifests)
	defer state.Close()
	if *useScratch {
		if err := state.openScratch(derived); err != nil {
			log.Fatalf("open scratch db: %v", err)
		}
	}

	runStart := time.Now().UTC()
	if err := state.Phase1(); err != nil {
		log.Fatalf("phase 1: %v", err)
	}
	if err := state.Phase2(); err != nil {
		log.Fatalf("phase 2: %v", err)
	}
	state.Phase3Mining()
	if err := state.Phase4Write(derived, runStart); err != nil {
		log.Fatalf("phase 4: %v", err)
	}
	fmt.Fprintf(os.Stderr, "aggregate complete: lang=%s sources=%d sentences=%d surfaces=%d\n",
		langLower, len(manifests), len(state.sentences), len(state.surfaces))
}

func filterSources(in []sources.Manifest, slug string) []sources.Manifest {
	for _, m := range in {
		if m.Slug == slug {
			return []sources.Manifest{m}
		}
	}
	return nil
}

// ── State ─────────────────────────────────────────────────────────────────

type surfaceStats struct {
	prose  int
	poetry int
	// docCountProse / docCountPoetry: distinct doc count, computed via
	// last-seen-doc tracking instead of set membership (saves ~5 KB per
	// frequent surface at scale).
	docCountProse  int
	docCountPoetry int
	lastDocProse   string
	lastDocPoetry  string
	sourceCount    map[string]int
	exampleHash    string // sentence hash of first prose occurrence
	examplePoem    int    // poem id of first poetry-only occurrence
	resolved       bool   // set in phase 2
}

type sentenceRec struct {
	hash string
	text string
	id   int // assigned in phase 4
}

type sentenceOccurrence struct {
	hash         string
	source       string
	documentID   string
	sentenceIx   int
	qualityFlags string
}

type documentRec struct {
	id      string
	source  string
	title   string
	author  string
	rawPath string
}

type wordlistRow struct {
	surface         string
	lemma           string
	pos             string
	feats           string
	analysisSources []string
	analysisRank    int
	isParserChoice  bool
}

// disagreementRow captures a surface where basic and custom parsers
// chose different analyses. Both rows recorded for diagnostic value.
type disagreementRow struct {
	surface     string
	basicLemma  string
	basicPOS    string
	basicFeats  string
	customLemma string
	customPOS   string
	customFeats string
}

// consensusRow captures a surface where basic + custom + FST top hit
// all agree on (lemma, pos). Useful prioritization signal — NOT silver.
type consensusRow struct {
	surface       string
	agreedLemma   string
	agreedPOS     string
	agreedFeats   string
	agreementKind string // "basic+custom+fst"
}

type state struct {
	roots     cli.Roots
	langLower string
	langUpper string
	manifests []sources.Manifest

	dictDB *store.DB
	lemma  *lemmatizer.Lemmatizer

	parserVersion   string
	fstTablesSHA    string
	dictFingerprint string

	surfaces            map[string]*surfaceStats // key: surface
	sentences           map[string]*sentenceRec  // key: hash (in-memory only when scratch=nil)
	sentenceOrder       []string                 // hashes in first-encounter order
	occurrences         []sentenceOccurrence
	documents           map[string]*documentRec // key: document_id
	docOrder            []string
	wordlistRows        []wordlistRow
	miningUnresolved    []wordlistRow
	miningPoetryUnresol []wordlistRow
	miningAmbiguous     []wordlistRow
	miningDisagreements []disagreementRow
	miningConsensus     []consensusRow

	srcDocCounts map[string]map[string]int // source -> document_id -> count

	// Scratch SQLite (nil = in-memory mode). When non-nil, sentences /
	// occurrences / documents / wordlist all live on disk; surfaces stays
	// in memory (it's compact and we hit it hot in phase 2).
	scratch *sql.DB
}

func newState(roots cli.Roots, langLower, langUpper string, manifests []sources.Manifest) *state {
	dictDB, err := cli.OpenDB(roots.DBPath)
	if err != nil {
		log.Fatalf("open dict db: %v", err)
	}
	lem, err := lemmatizer.NewFromDir(roots.TablesDir)
	if err != nil {
		log.Fatalf("load lemmatizer: %v", err)
	}
	parserVersion := os.Getenv("FINNESTDB_PARSER_VERSION")
	if parserVersion == "" {
		parserVersion = "dev-" + time.Now().UTC().Format("20060102")
	}
	fstSha, err := hashFile(filepath.Join(roots.TablesDir, langLower+"_min.json"))
	if err != nil {
		log.Fatalf("hash fst: %v", err)
	}
	dictRO, err := sql.Open("sqlite3", "file:"+roots.DBPath+"?mode=ro&immutable=1")
	if err != nil {
		log.Fatalf("open dict ro: %v", err)
	}
	dictFp, err := dictFingerprint(dictRO, langUpper)
	dictRO.Close()
	if err != nil {
		log.Fatalf("dict fingerprint: %v", err)
	}

	return &state{
		roots:           roots,
		langLower:       langLower,
		langUpper:       langUpper,
		manifests:       manifests,
		dictDB:          dictDB,
		lemma:           lem,
		parserVersion:   parserVersion,
		fstTablesSHA:    fstSha,
		dictFingerprint: dictFp,
		surfaces:        map[string]*surfaceStats{},
		sentences:       map[string]*sentenceRec{},
		documents:       map[string]*documentRec{},
		srcDocCounts:    map[string]map[string]int{},
	}
}

func (s *state) Close() {
	if s.dictDB != nil {
		_ = s.dictDB.Close()
	}
	if s.lemma != nil {
		_ = s.lemma.Close()
	}
	if s.scratch != nil {
		_ = s.scratch.Close()
	}
}

// openScratch opens (and resets) the scratch DB at <derived>/_scratch.db.
// Schema is in scratch_db.go.
func (s *state) openScratch(derived string) error {
	scratchPath := filepath.Join(derived, "_scratch.db")
	_ = os.Remove(scratchPath)
	db, err := openScratch(scratchPath)
	if err != nil {
		return err
	}
	s.scratch = db
	return nil
}

// ── Phase 1: tokenize via parserffi, count surfaces, dedup sentences ──

func (s *state) Phase1() error {
	for _, m := range s.manifests {
		dir := sources.SourceDir(s.roots.DataRoot, s.langLower, m.Slug)
		if err := s.ingestDocuments(dir, m); err != nil {
			return fmt.Errorf("ingest documents %s: %w", m.Slug, err)
		}
		if err := s.ingestText(dir, m); err != nil {
			return fmt.Errorf("ingest text %s: %w", m.Slug, err)
		}
		// Streaming mode: flush per-source so in-memory stays bounded.
		if s.scratch != nil {
			if err := s.flushSentencesToScratch(); err != nil {
				return fmt.Errorf("flush %s to scratch: %w", m.Slug, err)
			}
		}
	}
	return nil
}

func (s *state) ingestDocuments(dir string, m sources.Manifest) error {
	path := filepath.Join(dir, "documents.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var d struct {
			DocumentID string `json:"document_id"`
			Title      string `json:"title,omitempty"`
			Author     string `json:"author,omitempty"`
			RawPath    string `json:"raw_path,omitempty"`
		}
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			return fmt.Errorf("parse doc line %q: %w", line, err)
		}
		if _, ok := s.documents[d.DocumentID]; !ok {
			s.docOrder = append(s.docOrder, d.DocumentID)
		}
		s.documents[d.DocumentID] = &documentRec{
			id:      d.DocumentID,
			source:  m.Slug,
			title:   d.Title,
			author:  d.Author,
			rawPath: d.RawPath,
		}
	}
	return nil
}

func (s *state) ingestText(dir string, m sources.Manifest) error {
	textPath := filepath.Join(dir, "text.txt")
	data, err := os.ReadFile(textPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	isPoetry := m.Kind == "poetry"
	// Split on blank-line document boundaries
	docs := strings.Split(string(data), "\n\n")
	docIx := 0
	for _, docText := range docs {
		docText = strings.TrimSpace(docText)
		if docText == "" {
			continue
		}
		docID := fmt.Sprintf("%s:doc-%d", m.Slug, docIx)
		// If we have a per-source document already at this position, prefer
		// that ID. Otherwise create a synthetic one.
		if existing := s.findFirstDocFor(m.Slug, docIx); existing != "" {
			docID = existing
		}
		docIx++
		if err := s.tokenizeDoc(docText, m.Slug, docID, isPoetry); err != nil {
			return err
		}
	}
	return nil
}

// findFirstDocFor returns a documents.jsonl-defined ID at the given index
// for a source, if one exists.
func (s *state) findFirstDocFor(source string, ix int) string {
	count := 0
	for _, id := range s.docOrder {
		if d, ok := s.documents[id]; ok && d.source == source {
			if count == ix {
				return id
			}
			count++
		}
	}
	return ""
}

func (s *state) tokenizeDoc(docText, source, docID string, isPoetry bool) error {
	result, err := parserffi.AnalyzeText(s.langUpper, docText)
	if err != nil {
		return fmt.Errorf("AnalyzeText: %w", err)
	}
	for ix, sent := range result.Sentences {
		if len(sent.Tokens) == 0 {
			continue
		}
		text := reconstructText(sent.Tokens)
		hash := sentenceHash(text)
		if _, ok := s.sentences[hash]; !ok {
			s.sentences[hash] = &sentenceRec{hash: hash, text: text}
			s.sentenceOrder = append(s.sentenceOrder, hash)
		}
		s.occurrences = append(s.occurrences, sentenceOccurrence{
			hash:         hash,
			source:       source,
			documentID:   docID,
			sentenceIx:   ix,
			qualityFlags: qualityFlags(text),
		})
		for _, t := range sent.Tokens {
			if isPunctOnly(t.Form) {
				continue
			}
			ss := s.getOrCreateSurface(t.Form)
			if isPoetry {
				ss.poetry++
				if ss.lastDocPoetry != docID {
					ss.docCountPoetry++
					ss.lastDocPoetry = docID
				}
			} else {
				ss.prose++
				if ss.lastDocProse != docID {
					ss.docCountProse++
					ss.lastDocProse = docID
				}
				if ss.exampleHash == "" {
					ss.exampleHash = hash
				}
			}
			if ss.sourceCount == nil {
				ss.sourceCount = map[string]int{}
			}
			ss.sourceCount[source]++
		}
	}
	return nil
}

func (s *state) getOrCreateSurface(surface string) *surfaceStats {
	if ss, ok := s.surfaces[surface]; ok {
		return ss
	}
	ss := &surfaceStats{}
	s.surfaces[surface] = ss
	return ss
}

// ── Phase 2: enrich unique surfaces ──

func (s *state) Phase2() error {
	for surface, ss := range s.surfaces {
		rows := s.enrichSurface(surface)
		ss.resolved = len(rows) > 0 && rows[0].lemma != ""
		s.wordlistRows = append(s.wordlistRows, rows...)
	}
	return nil
}

func (s *state) enrichSurface(surface string) []wordlistRow {
	type key struct{ lemma, pos, feats string }
	rows := map[key]*wordlistRow{}

	// Custom-mode parser choice
	var customLemma, customPOS, customFeats string
	if pr, err := parsecore.Analyze(s.dictDB, s.langUpper, surface, "custom"); err == nil && pr != nil {
		for _, sent := range pr.Sentences {
			for _, tok := range sent.Tokens {
				if tok.Form != surface {
					continue
				}
				customLemma, customPOS, customFeats = tok.Lemma, tok.POS, tok.Feats
				k := key{tok.Lemma, tok.POS, tok.Feats}
				if r, ok := rows[k]; ok {
					if !contains(r.analysisSources, "parser_choice") {
						r.analysisSources = append(r.analysisSources, "parser_choice")
					}
					r.isParserChoice = true
				} else {
					rows[k] = &wordlistRow{
						surface:         surface,
						lemma:           tok.Lemma,
						pos:             tok.POS,
						feats:           tok.Feats,
						analysisSources: []string{"parser_choice"},
						isParserChoice:  true,
					}
				}
				break // only need first token (single-surface input)
			}
			if customLemma != "" {
				break
			}
		}
	}
	// Basic-mode parser choice (for disagreement mining)
	var basicLemma, basicPOS, basicFeats string
	if pr, err := parsecore.Analyze(s.dictDB, s.langUpper, surface, "basic"); err == nil && pr != nil {
		for _, sent := range pr.Sentences {
			for _, tok := range sent.Tokens {
				if tok.Form != surface {
					continue
				}
				basicLemma, basicPOS, basicFeats = tok.Lemma, tok.POS, tok.Feats
				break
			}
			if basicLemma != "" {
				break
			}
		}
	}
	// FST analyses + top hit (for consensus mining)
	var fstTopLemma, fstTopPOS string
	fstHits := s.lemma.Lemmatize(s.langUpper, surface)
	if len(fstHits) > 0 {
		fstTopLemma = fstHits[0].Lemma
		fstTopPOS = fstHits[0].UPOS
	}
	for _, an := range fstHits {
		k := key{an.Lemma, an.UPOS, an.Feats}
		if r, ok := rows[k]; ok {
			if !contains(r.analysisSources, "fst") {
				r.analysisSources = append(r.analysisSources, "fst")
			}
		} else {
			rows[k] = &wordlistRow{
				surface:         surface,
				lemma:           an.Lemma,
				pos:             an.UPOS,
				feats:           an.Feats,
				analysisSources: []string{"fst"},
			}
		}
	}
	// Mining: parser-disagreements (basic vs custom diff on lemma OR pos)
	if customLemma != "" && basicLemma != "" &&
		(customLemma != basicLemma || customPOS != basicPOS) {
		s.miningDisagreements = append(s.miningDisagreements, disagreementRow{
			surface:    surface,
			basicLemma: basicLemma, basicPOS: basicPOS, basicFeats: basicFeats,
			customLemma: customLemma, customPOS: customPOS, customFeats: customFeats,
		})
	}
	// Mining: internal-consensus (basic + custom + FST top all agree on lemma+pos)
	if customLemma != "" && basicLemma == customLemma && basicPOS == customPOS &&
		fstTopLemma == customLemma && fstTopPOS == customPOS {
		s.miningConsensus = append(s.miningConsensus, consensusRow{
			surface:       surface,
			agreedLemma:   customLemma,
			agreedPOS:     customPOS,
			agreedFeats:   customFeats,
			agreementKind: "basic+custom+fst",
		})
	}

	if len(rows) == 0 {
		return []wordlistRow{{surface: surface, analysisRank: 1}}
	}
	out := make([]wordlistRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].isParserChoice != out[j].isParserChoice {
			return out[i].isParserChoice
		}
		if out[i].lemma != out[j].lemma {
			return out[i].lemma < out[j].lemma
		}
		if out[i].pos != out[j].pos {
			return out[i].pos < out[j].pos
		}
		return out[i].feats < out[j].feats
	})
	for i := range out {
		out[i].analysisRank = i + 1
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// ── Phase 3: mining outputs ──

func (s *state) Phase3Mining() {
	// Group by surface — wordlist already has one or more rows per surface.
	bySurface := map[string][]wordlistRow{}
	for _, r := range s.wordlistRows {
		bySurface[r.surface] = append(bySurface[r.surface], r)
	}
	for surface, rows := range bySurface {
		ss := s.surfaces[surface]
		// Unresolved: no real analyses (lemma empty).
		hasReal := false
		for _, r := range rows {
			if r.lemma != "" {
				hasReal = true
				break
			}
		}
		if !hasReal {
			s.miningUnresolved = append(s.miningUnresolved, wordlistRow{surface: surface})
			// Poetry-dominant threshold
			if ss != nil && ss.poetry >= 10 && ss.poetry >= 5*max(ss.prose, 1) {
				s.miningPoetryUnresol = append(s.miningPoetryUnresol, wordlistRow{surface: surface})
			}
			continue
		}
		// Ambiguous: ≥2 distinct (lemma,pos) analyses
		distinct := map[string]struct{}{}
		for _, r := range rows {
			if r.lemma != "" {
				distinct[r.lemma+"|"+r.pos] = struct{}{}
			}
		}
		if len(distinct) >= 2 {
			s.miningAmbiguous = append(s.miningAmbiguous, wordlistRow{surface: surface})
		}
	}
}

// ── Phase 4: write everything with deterministic IDs ──

func (s *state) Phase4Write(derived string, runStart time.Time) error {
	if s.scratch != nil {
		return s.phase4WriteScratch(derived, runStart)
	}
	// In-memory path (existing) follows.
	// Assign deterministic sentence IDs by sorting first-occurrence
	// (source_order, document_id, sentence_ix, hash).
	type firstOcc struct {
		hash       string
		source     string
		documentID string
		sentenceIx int
	}
	firstByHash := map[string]firstOcc{}
	for _, occ := range s.occurrences {
		if _, ok := firstByHash[occ.hash]; ok {
			continue
		}
		firstByHash[occ.hash] = firstOcc{occ.hash, occ.source, occ.documentID, occ.sentenceIx}
	}
	hashes := make([]firstOcc, 0, len(firstByHash))
	for _, fo := range firstByHash {
		hashes = append(hashes, fo)
	}
	sort.Slice(hashes, func(i, j int) bool {
		if hashes[i].source != hashes[j].source {
			return hashes[i].source < hashes[j].source
		}
		if hashes[i].documentID != hashes[j].documentID {
			return hashes[i].documentID < hashes[j].documentID
		}
		if hashes[i].sentenceIx != hashes[j].sentenceIx {
			return hashes[i].sentenceIx < hashes[j].sentenceIx
		}
		return hashes[i].hash < hashes[j].hash
	})
	for i, fo := range hashes {
		s.sentences[fo.hash].id = i + 1
	}

	// Write sentences.tsv
	if err := writeTSV(filepath.Join(derived, "sentences.tsv"),
		[]string{"id", "lang", "text"},
		func(yield func([]string)) {
			for _, fo := range hashes {
				rec := s.sentences[fo.hash]
				yield([]string{itoa(rec.id), s.langLower, rec.text})
			}
		}); err != nil {
		return err
	}
	if err := writeTSV(filepath.Join(derived, "sentences_user_friendly.tsv"),
		[]string{"id", "lang", "text"},
		func(yield func([]string)) {
			for _, fo := range hashes {
				rec := s.sentences[fo.hash]
				if textfilter.IsUserFriendlySentence(rec.text) {
					yield([]string{itoa(rec.id), s.langLower, rec.text})
				}
			}
		}); err != nil {
		return err
	}

	// Write sentence_occurrences.tsv
	sort.Slice(s.occurrences, func(i, j int) bool {
		if s.occurrences[i].hash != s.occurrences[j].hash {
			return s.sentences[s.occurrences[i].hash].id < s.sentences[s.occurrences[j].hash].id
		}
		if s.occurrences[i].source != s.occurrences[j].source {
			return s.occurrences[i].source < s.occurrences[j].source
		}
		return s.occurrences[i].sentenceIx < s.occurrences[j].sentenceIx
	})
	if err := writeTSV(filepath.Join(derived, "sentence_occurrences.tsv"),
		[]string{"sentence_id", "source", "document_id", "sentence_ix", "quality_flags"},
		func(yield func([]string)) {
			for _, occ := range s.occurrences {
				rec := s.sentences[occ.hash]
				yield([]string{itoa(rec.id), occ.source, occ.documentID, itoa(occ.sentenceIx), occ.qualityFlags})
			}
		}); err != nil {
		return err
	}

	// Write documents.tsv
	if err := writeTSV(filepath.Join(derived, "documents.tsv"),
		[]string{"document_id", "source", "title", "author", "raw_path"},
		func(yield func([]string)) {
			for _, id := range s.docOrder {
				d := s.documents[id]
				yield([]string{d.id, d.source, d.title, d.author, d.rawPath})
			}
		}); err != nil {
		return err
	}

	// Write poems.tsv (empty in v1 smoke — no poetry sources active)
	if err := writeTSV(filepath.Join(derived, "poems.tsv"),
		[]string{"id", "lang", "source", "document_id", "title", "author", "line_count", "text"},
		func(yield func([]string)) {}); err != nil {
		return err
	}

	// Write manifest.tsv
	if err := writeTSV(filepath.Join(derived, "manifest.tsv"),
		[]string{"source", "kind", "license", "url", "format", "fetched_utc"},
		func(yield func([]string)) {
			for _, m := range s.manifests {
				yield([]string{m.Slug, m.Kind, m.License, m.URL, m.Format, runStart.Format(time.RFC3339)})
			}
		}); err != nil {
		return err
	}

	// Write wordlist.tsv with example_ref resolution
	sort.Slice(s.wordlistRows, func(i, j int) bool {
		si, sj := s.surfaces[s.wordlistRows[i].surface], s.surfaces[s.wordlistRows[j].surface]
		if si.prose != sj.prose {
			return si.prose > sj.prose
		}
		if si.prose+si.poetry != sj.prose+sj.poetry {
			return si.prose+si.poetry > sj.prose+sj.poetry
		}
		if s.wordlistRows[i].surface != s.wordlistRows[j].surface {
			return s.wordlistRows[i].surface < s.wordlistRows[j].surface
		}
		return s.wordlistRows[i].analysisRank < s.wordlistRows[j].analysisRank
	})
	// Canonical wordlist.tsv carries one row per analysis. example_ref_type
	// and example_ref_id point at the canonical sentence/poem id; the
	// example body itself is recoverable via JOIN against sentences.tsv or
	// poems.tsv on that id. example_text is no longer denormalized into
	// every row — at full FI scale it accounted for ~80% of wordlist.tsv
	// bytes (4M rows × ~400 chars). The user-friendly export below carries
	// the example_ref pair too; downstream consumers join when needed.
	if err := writeTSV(filepath.Join(derived, "wordlist.tsv"),
		[]string{
			"surface", "surface_count_prose", "surface_count_poetry", "surface_count_total",
			"doc_count_prose", "doc_count_poetry", "source_counts_json",
			"lang", "lemma", "pos", "feats",
			"analysis_sources", "analysis_rank", "is_parser_choice",
			"parser_version", "fst_tables_sha", "dict_fingerprint",
			"example_ref_type", "example_ref_id",
		},
		func(yield func([]string)) {
			for _, r := range s.wordlistRows {
				ss := s.surfaces[r.surface]
				exType, exID := s.exampleRefFor(ss)
				srcJSON, _ := json.Marshal(ss.sourceCount)
				yield([]string{
					r.surface,
					itoa(ss.prose), itoa(ss.poetry), itoa(ss.prose + ss.poetry),
					itoa(ss.docCountProse), itoa(ss.docCountPoetry),
					string(srcJSON),
					s.langLower, r.lemma, r.pos, r.feats,
					strings.Join(r.analysisSources, ";"), itoa(r.analysisRank), boolStr(r.isParserChoice),
					s.parserVersion, s.fstTablesSHA, s.dictFingerprint,
					exType, exID,
				})
			}
		}); err != nil {
		return err
	}

	// User-friendly wordlist: one row per parser-choice analysis with the
	// fields a learner-facing UI needs. Joins lemmas.gloss for meaning;
	// splits FEATS into named morphology columns; carries the example_ref
	// pair for downstream sentence-text rendering.
	if err := s.writeUserFriendlyWordlist(filepath.Join(derived, "wordlist_user_friendly.tsv")); err != nil {
		return err
	}

	// Mining TSVs
	miningDir := filepath.Join(derived, "mining")
	miningWriter := func(name string, surfaces []wordlistRow) error {
		return writeTSV(filepath.Join(miningDir, name),
			[]string{"surface", "surface_count_prose", "surface_count_poetry", "surface_count_total"},
			func(yield func([]string)) {
				for _, r := range surfaces {
					ss := s.surfaces[r.surface]
					yield([]string{r.surface, itoa(ss.prose), itoa(ss.poetry), itoa(ss.prose + ss.poetry)})
				}
			})
	}
	sortBySurfaceProseDesc := func(rows []wordlistRow) {
		sort.Slice(rows, func(i, j int) bool {
			si, sj := s.surfaces[rows[i].surface], s.surfaces[rows[j].surface]
			if si.prose != sj.prose {
				return si.prose > sj.prose
			}
			return rows[i].surface < rows[j].surface
		})
	}
	sortBySurfacePoetryDesc := func(rows []wordlistRow) {
		sort.Slice(rows, func(i, j int) bool {
			si, sj := s.surfaces[rows[i].surface], s.surfaces[rows[j].surface]
			if si.poetry != sj.poetry {
				return si.poetry > sj.poetry
			}
			return rows[i].surface < rows[j].surface
		})
	}
	sortBySurfaceProseDesc(s.miningUnresolved)
	sortBySurfacePoetryDesc(s.miningPoetryUnresol)
	sortBySurfaceProseDesc(s.miningAmbiguous)

	if err := miningWriter("unresolved.tsv", s.miningUnresolved); err != nil {
		return err
	}
	if err := miningWriter("poetry-unresolved.tsv", s.miningPoetryUnresol); err != nil {
		return err
	}
	if err := miningWriter("high-frequency-ambiguous.tsv", s.miningAmbiguous); err != nil {
		return err
	}
	// Sort disagreements by surface_count_prose desc
	sort.Slice(s.miningDisagreements, func(i, j int) bool {
		si, sj := s.surfaces[s.miningDisagreements[i].surface], s.surfaces[s.miningDisagreements[j].surface]
		if si.prose != sj.prose {
			return si.prose > sj.prose
		}
		return s.miningDisagreements[i].surface < s.miningDisagreements[j].surface
	})
	if err := writeTSV(filepath.Join(miningDir, "parser-disagreements.tsv"),
		[]string{"surface", "surface_count_prose", "surface_count_poetry", "basic_lemma", "basic_pos", "basic_feats", "custom_lemma", "custom_pos", "custom_feats"},
		func(yield func([]string)) {
			for _, d := range s.miningDisagreements {
				ss := s.surfaces[d.surface]
				yield([]string{
					d.surface, itoa(ss.prose), itoa(ss.poetry),
					d.basicLemma, d.basicPOS, d.basicFeats,
					d.customLemma, d.customPOS, d.customFeats,
				})
			}
		}); err != nil {
		return err
	}
	// Sort consensus by surface_count_prose desc
	sort.Slice(s.miningConsensus, func(i, j int) bool {
		si, sj := s.surfaces[s.miningConsensus[i].surface], s.surfaces[s.miningConsensus[j].surface]
		if si.prose != sj.prose {
			return si.prose > sj.prose
		}
		return s.miningConsensus[i].surface < s.miningConsensus[j].surface
	})
	if err := writeTSV(filepath.Join(miningDir, "internal-consensus-candidates.tsv"),
		[]string{"surface", "surface_count_prose", "surface_count_poetry", "agreed_lemma", "agreed_pos", "agreed_feats", "agreement_kind"},
		func(yield func([]string)) {
			for _, c := range s.miningConsensus {
				ss := s.surfaces[c.surface]
				yield([]string{
					c.surface, itoa(ss.prose), itoa(ss.poetry),
					c.agreedLemma, c.agreedPOS, c.agreedFeats, c.agreementKind,
				})
			}
		}); err != nil {
		return err
	}
	// silver-candidates.tsv NOT created (only enrichcorpus does)

	// build_metadata.json
	if err := writeJSON(filepath.Join(derived, "build_metadata.json"), map[string]any{
		"lang":             s.langLower,
		"parser_version":   s.parserVersion,
		"fst_tables_sha":   s.fstTablesSHA,
		"dict_fingerprint": s.dictFingerprint,
		"db_path":          s.roots.DBPath,
		"data_root":        s.roots.DataRoot,
		"run_start_utc":    runStart.Format(time.RFC3339),
		"run_end_utc":      time.Now().UTC().Format(time.RFC3339),
		"sources":          s.manifests,
	}); err != nil {
		return err
	}

	// qa-report.json
	if err := s.writeQAReport(filepath.Join(derived, "qa-report.json"), runStart); err != nil {
		return err
	}
	return nil
}

// exampleRefFor returns the (example_ref_type, example_ref_id) pair for a
// surface. The canonical wordlist no longer denormalizes example_text into
// every row — readers reconstruct the example body by joining
// example_ref_id against sentences.tsv (or poems.tsv when the type is
// "poem"). Returns "" / "" when the surface has no captured example.
func (s *state) exampleRefFor(ss *surfaceStats) (string, string) {
	if ss.exampleHash != "" {
		if rec, ok := s.sentences[ss.exampleHash]; ok && rec.id > 0 {
			return "sentence", itoa(rec.id)
		}
	}
	if ss.examplePoem > 0 {
		return "poem", itoa(ss.examplePoem)
	}
	return "", ""
}

func (s *state) writeQAReport(path string, runStart time.Time) error {
	tokensProse, tokensPoetry := 0, 0
	uniqProse, uniqPoetry, poetryOnly, proseOnly := 0, 0, 0, 0
	unresolvedProse, unresolvedPoetry := 0, 0
	for _, ss := range s.surfaces {
		tokensProse += ss.prose
		tokensPoetry += ss.poetry
		if ss.prose > 0 && ss.poetry > 0 {
			uniqProse++
			uniqPoetry++
		} else if ss.prose > 0 {
			uniqProse++
			proseOnly++
		} else if ss.poetry > 0 {
			uniqPoetry++
			poetryOnly++
		}
		if !ss.resolved {
			if ss.prose > 0 {
				unresolvedProse++
			}
			if ss.poetry > 0 {
				unresolvedPoetry++
			}
		}
	}
	rateOf := func(num, denom int) float64 {
		if denom == 0 {
			return 0
		}
		return float64(num) / float64(denom)
	}
	report := map[string]any{
		"lang":             s.langLower,
		"run_start_utc":    runStart.Format(time.RFC3339),
		"run_end_utc":      time.Now().UTC().Format(time.RFC3339),
		"parser_version":   s.parserVersion,
		"fst_tables_sha":   s.fstTablesSHA,
		"dict_fingerprint": s.dictFingerprint,
		"totals": map[string]any{
			"sources":                     len(s.manifests),
			"documents":                   len(s.documents),
			"sentences_unique":            len(s.sentences),
			"sentences_total_occurrences": len(s.occurrences),
			"tokens_total":                tokensProse + tokensPoetry,
			"tokens_prose":                tokensProse,
			"tokens_poetry":               tokensPoetry,
			"unique_surfaces":             len(s.surfaces),
			"unique_surfaces_prose":       uniqProse,
			"unique_surfaces_poetry":      uniqPoetry,
			"poetry_only_surfaces":        poetryOnly,
			"prose_only_surfaces":         proseOnly,
			"unresolved_surfaces_total":   unresolvedProse + unresolvedPoetry,
			"unresolved_rate_total":       rateOf(unresolvedProse+unresolvedPoetry, len(s.surfaces)),
			"unresolved_surfaces_prose":   unresolvedProse,
			"unresolved_rate_prose":       rateOf(unresolvedProse, uniqProse),
			"unresolved_surfaces_poetry":  unresolvedPoetry,
			"unresolved_rate_poetry":      rateOf(unresolvedPoetry, uniqPoetry),
			"ambiguous_surfaces":          len(s.miningAmbiguous),
			"poems":                       0,
		},
	}
	return writeJSON(path, report)
}

// ── Helpers ──

func writeTSV(path string, header []string, yield func(yield func([]string))) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Comma = '\t'
	if err := w.Write(header); err != nil {
		return err
	}
	yield(func(row []string) {
		_ = w.Write(row)
	})
	w.Flush()
	return w.Error()
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func reconstructText(tokens []parserffi.Token) string {
	var b strings.Builder
	for i, t := range tokens {
		if i > 0 && !isPunctOnly(t.Form) {
			b.WriteByte(' ')
		}
		b.WriteString(t.Form)
	}
	return b.String()
}

func isPunctOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r > 127:
			return false
		}
	}
	return true
}

func sentenceHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:16])
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:8]), nil
}

func qualityFlags(s string) string {
	var flags []string
	if strings.Contains(s, "http://") || strings.Contains(s, "https://") || strings.Contains(s, "www.") {
		flags = append(flags, "has_url")
	}
	if strings.Contains(s, "@") && strings.Contains(s, ".") {
		flags = append(flags, "has_email")
	}
	if len(s) < 8 {
		flags = append(flags, "very_short")
	}
	if len(s) > 600 {
		flags = append(flags, "very_long")
	}
	return strings.Join(flags, ";")
}

func dictFingerprint(db *sql.DB, langUpper string) (string, error) {
	var formCount, lemmaCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM forms WHERE lang = ?`, langUpper).Scan(&formCount); err != nil {
		return "", err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM lemmas WHERE lang = ?`, langUpper).Scan(&lemmaCount); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\nforms=%d\nlemmas=%d", langUpper, formCount, lemmaCount)))
	return hex.EncodeToString(digest[:8]), nil
}
