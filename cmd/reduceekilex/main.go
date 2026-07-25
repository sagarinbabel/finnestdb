// cmd/reduceekilex reads the gzipped Ekilex /api/word/details payloads written
// by cmd/fetchekilex and emits two committable artifacts:
//
//   - details-compact.jsonl: one line per word_id with lemma, morphology
//     metadata, datasets, and per-meaning definitions/usages/translations
//     (Estonian + English). Reconstruction is lossy by design - see the
//     dropped fields list in the README for what was excluded.
//
//   - forms.tsv: one row per inflected form (lemma, form, morph_code).
//     Header row included. Suitable for direct ingestion into the parser's
//     forms table.
//
// The reducer is deterministic and stable: outputs are sorted by word_id (and
// by paradigm/form within a word) so that re-running on unchanged input
// produces byte-identical output.
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Subset of the Ekilex /api/word/details response that the reducer consumes.
// Unused fields are intentionally absent; json.Unmarshal silently drops them.

type rawWordDetails struct {
	Word    rawWord     `json:"word"`
	Lexemes []rawLexeme `json:"lexemes"`
}

type rawWord struct {
	WordID          int64         `json:"wordId"`
	WordValue       string        `json:"wordValue"`
	Lang            string        `json:"lang"`
	HomonymNr       int           `json:"homonymNr"`
	MorphophonoForm string        `json:"morphophonoForm"`
	MorphComment    string        `json:"morphComment"`
	DatasetCodes    []string      `json:"datasetCodes"`
	Paradigms       []rawParadigm `json:"paradigms"`
}

type rawParadigm struct {
	ParadigmID       int64     `json:"paradigmId"`
	InflectionType   string    `json:"inflectionType"`
	InflectionTypeNr string    `json:"inflectionTypeNr"`
	WordClass        string    `json:"wordClass"`
	Comment          string    `json:"comment"`
	Forms            []rawForm `json:"forms"`
}

// inflectionType / inflectionTypeNr arrive as either strings ("22e") or
// numbers (22) depending on the entry. Decode tolerantly into string.
func (p *rawParadigm) UnmarshalJSON(b []byte) error {
	type alias struct {
		ParadigmID       int64           `json:"paradigmId"`
		InflectionType   json.RawMessage `json:"inflectionType"`
		InflectionTypeNr json.RawMessage `json:"inflectionTypeNr"`
		WordClass        string          `json:"wordClass"`
		Comment          string          `json:"comment"`
		Forms            []rawForm       `json:"forms"`
	}
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	p.ParadigmID = a.ParadigmID
	p.WordClass = a.WordClass
	p.Comment = a.Comment
	p.Forms = a.Forms
	p.InflectionType = decodeStringOrNumber(a.InflectionType)
	p.InflectionTypeNr = decodeStringOrNumber(a.InflectionTypeNr)
	return nil
}

func decodeStringOrNumber(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
	}
	return strings.TrimSpace(string(raw))
}

type rawForm struct {
	Value     string `json:"value"`
	MorphCode string `json:"morphCode"`
}

type rawLexeme struct {
	LexemeID          int64                 `json:"lexemeId"`
	DatasetCode       string                `json:"datasetCode"`
	POS               []rawPOS              `json:"pos"`
	Meaning           *rawMeaning           `json:"meaning"`
	Usages            []rawLangValue        `json:"usages"`
	SynonymLangGroups []rawSynonymLangGroup `json:"synonymLangGroups"`
}

type rawPOS struct {
	Code string `json:"code"`
}

type rawMeaning struct {
	MeaningID   int64          `json:"meaningId"`
	Definitions []rawLangValue `json:"definitions"`
}

type rawLangValue struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type rawSynonymLangGroup struct {
	Lang     string       `json:"lang"`
	Synonyms []rawSynonym `json:"synonyms"`
}

type rawSynonym struct {
	Words []rawSynonymWord `json:"words"`
}

type rawSynonymWord struct {
	WordValue string `json:"wordValue"`
	Lang      string `json:"lang"`
}

// Reduced (committable) output structures.

type compactWord struct {
	WordID           int64            `json:"word_id"`
	Lemma            string           `json:"lemma"`
	HomonymNr        int              `json:"homonym_nr,omitempty"`
	MorphophonoForm  string           `json:"morphophono_form,omitempty"`
	InflectionType   string           `json:"inflection_type,omitempty"`
	InflectionTypeNr string           `json:"inflection_type_nr,omitempty"`
	WordClass        string           `json:"word_class,omitempty"`
	MorphComment     string           `json:"morph_comment,omitempty"`
	Datasets         []string         `json:"datasets,omitempty"`
	Meanings         []compactMeaning `json:"meanings,omitempty"`
}

type compactMeaning struct {
	MeaningID      int64    `json:"meaning_id,omitempty"`
	Dataset        string   `json:"dataset,omitempty"`
	POS            []string `json:"pos,omitempty"`
	DefinitionsET  []string `json:"definitions_et,omitempty"`
	DefinitionsEN  []string `json:"definitions_en,omitempty"`
	UsagesET       []string `json:"usages_et,omitempty"`
	UsagesEN       []string `json:"usages_en,omitempty"`
	TranslationsEN []string `json:"translations_en,omitempty"`
}

// reduce converts a raw payload into the committable compact form plus the
// flat list of (form, morph_code) tuples for the TSV.
func reduce(raw rawWordDetails) (compactWord, []formRow) {
	w := raw.Word
	out := compactWord{
		WordID:          w.WordID,
		Lemma:           w.WordValue,
		HomonymNr:       w.HomonymNr,
		MorphophonoForm: w.MorphophonoForm,
		MorphComment:    w.MorphComment,
		Datasets:        sortedUnique(w.DatasetCodes),
	}

	// Inflection metadata: take the first paradigm's class. Multi-paradigm
	// entries are rare (~6.6%); the alternate paradigm forms still land in
	// the forms output below, but the compact metadata reports the primary.
	if len(w.Paradigms) > 0 {
		p := w.Paradigms[0]
		out.InflectionType = p.InflectionType
		out.InflectionTypeNr = p.InflectionTypeNr
		out.WordClass = p.WordClass
	}

	for _, lx := range raw.Lexemes {
		m := compactMeaning{
			Dataset: lx.DatasetCode,
			POS:     posCodes(lx.POS),
		}
		if lx.Meaning != nil {
			m.MeaningID = lx.Meaning.MeaningID
			for _, d := range lx.Meaning.Definitions {
				switch strings.ToLower(d.Lang) {
				case "est":
					m.DefinitionsET = appendNonEmpty(m.DefinitionsET, d.Value)
				case "eng":
					m.DefinitionsEN = appendNonEmpty(m.DefinitionsEN, d.Value)
				}
			}
		}
		for _, u := range lx.Usages {
			switch strings.ToLower(u.Lang) {
			case "est":
				m.UsagesET = appendNonEmpty(m.UsagesET, u.Value)
			case "eng":
				m.UsagesEN = appendNonEmpty(m.UsagesEN, u.Value)
			}
		}
		for _, g := range lx.SynonymLangGroups {
			if !strings.EqualFold(g.Lang, "eng") {
				continue
			}
			for _, s := range g.Synonyms {
				for _, sw := range s.Words {
					m.TranslationsEN = appendNonEmpty(m.TranslationsEN, sw.WordValue)
				}
			}
		}
		// drop entirely empty meanings (lexemes that contributed nothing
		// after filtering - common for ety / cross-dictionary placeholder
		// lexemes that only carry foreign-language synonyms)
		if isEmptyMeaning(m) {
			continue
		}
		dedup(&m)
		out.Meanings = append(out.Meanings, m)
	}

	var rows []formRow
	seen := map[[2]string]struct{}{}
	for _, p := range w.Paradigms {
		for _, f := range p.Forms {
			if f.Value == "" || f.MorphCode == "" {
				continue
			}
			key := [2]string{f.Value, f.MorphCode}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			rows = append(rows, formRow{Lemma: w.WordValue, Form: f.Value, MorphCode: f.MorphCode})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Form != rows[j].Form {
			return rows[i].Form < rows[j].Form
		}
		return rows[i].MorphCode < rows[j].MorphCode
	})
	return out, rows
}

type formRow struct {
	Lemma     string
	Form      string
	MorphCode string
}

func posCodes(in []rawPOS) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p.Code != "" {
			out = append(out, p.Code)
		}
	}
	return sortedUnique(out)
}

func appendNonEmpty(s []string, v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return s
	}
	return append(s, v)
}

func sortedUnique(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func dedup(m *compactMeaning) {
	m.DefinitionsET = sortedUnique(m.DefinitionsET)
	m.DefinitionsEN = sortedUnique(m.DefinitionsEN)
	m.UsagesET = sortedUnique(m.UsagesET)
	m.UsagesEN = sortedUnique(m.UsagesEN)
	m.TranslationsEN = sortedUnique(m.TranslationsEN)
}

func isEmptyMeaning(m compactMeaning) bool {
	return len(m.DefinitionsET) == 0 &&
		len(m.DefinitionsEN) == 0 &&
		len(m.UsagesET) == 0 &&
		len(m.UsagesEN) == 0 &&
		len(m.TranslationsEN) == 0
}

// I/O ------------------------------------------------------------------------

func main() {
	rawDir := flag.String("raw-dir", "localdata/ekilex/details/raw", "directory of bucketed *.json.gz files")
	outCompact := flag.String("out-compact", "", "single-file JSONL output path (mutually exclusive with -out-compact-dir)")
	outForms := flag.String("out-forms", "", "single-file TSV output path (mutually exclusive with -out-forms-dir)")
	outCompactDir := flag.String("out-compact-dir", "localdata/ekilex/definitions", "shard-per-letter JSONL output directory")
	outFormsDir := flag.String("out-forms-dir", "localdata/ekilex/forms", "shard-per-letter TSV output directory")
	limit := flag.Int("limit", 0, "stop after N words (0 = unlimited)")
	progressEvery := flag.Int("progress-every", 10000, "print a progress line every N words processed")
	flag.Parse()

	// If -out-compact / -out-forms are set, write a single combined file for
	// that stream. Otherwise shard per first-letter into the corresponding -dir.
	useCompactDir := *outCompact == ""
	useFormsDir := *outForms == ""

	if err := run(*rawDir, *outCompact, *outCompactDir, useCompactDir, *outForms, *outFormsDir, useFormsDir, *limit, *progressEvery); err != nil {
		log.Fatal(err)
	}
}

func run(rawDir, outCompactFile, outCompactDir string, useCompactDir bool, outFormsFile, outFormsDir string, useFormsDir bool, limit, progressEvery int) error {
	paths, err := listRawFiles(rawDir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no *.json.gz files under %s", rawDir)
	}
	sortRawByWordID(paths)
	if limit > 0 && len(paths) > limit {
		paths = paths[:limit]
	}

	compactSink, err := newCompactSink(outCompactFile, outCompactDir, useCompactDir)
	if err != nil {
		return err
	}
	defer compactSink.Close()

	formsSink, err := newFormsSink(outFormsFile, outFormsDir, useFormsDir)
	if err != nil {
		return err
	}
	defer formsSink.Close()

	start := time.Now()
	var (
		nWords, nMeanings, nForms, nSkipped int
	)
	for i, p := range paths {
		raw, err := readRaw(p)
		if err != nil {
			nSkipped++
			log.Printf("skip %s: %v", p, err)
			continue
		}
		compact, rows := reduce(raw)
		if compact.WordID == 0 {
			nSkipped++
			continue
		}
		if err := compactSink.Write(compact); err != nil {
			return err
		}
		for _, r := range rows {
			if err := formsSink.Write(compact.Lemma, r); err != nil {
				return err
			}
		}
		nWords++
		nMeanings += len(compact.Meanings)
		nForms += len(rows)
		if progressEvery > 0 && (i+1)%progressEvery == 0 {
			fmt.Printf("[%s] %d words, %d meanings, %d forms (%d skipped)\n",
				time.Now().Format("15:04:05"), nWords, nMeanings, nForms, nSkipped)
		}
	}
	if err := compactSink.Close(); err != nil {
		return err
	}
	if err := formsSink.Close(); err != nil {
		return err
	}
	fmt.Printf("[%s] done in %s: %d words, %d meanings, %d forms (%d skipped)\n",
		time.Now().Format("15:04:05"), time.Since(start).Round(time.Second),
		nWords, nMeanings, nForms, nSkipped)
	return nil
}

// shardKey returns the per-letter shard name for a given lemma. Latin a-z and
// Estonian-specific letters (ä, ö, ü, õ, š, ž) each get their own shard;
// everything else (digits, symbols, foreign scripts) goes into "_other".
func shardKey(lemma string) string {
	for _, r := range lemma {
		r = unicode.ToLower(r)
		switch {
		case r >= 'a' && r <= 'z':
			return string(r)
		case r == 'ä' || r == 'ö' || r == 'ü' || r == 'õ' || r == 'š' || r == 'ž':
			return string(r)
		}
		return "_other"
	}
	return "_other"
}

// compactSink writes compactWord JSON either to a single file or to one
// JSONL file per shard letter inside a directory.
type compactSink struct {
	single  *bufio.Writer
	singleF *os.File
	dir     string
	files   map[string]*bufio.Writer
	rawFs   map[string]*os.File
}

func newCompactSink(file, dir string, useDir bool) (*compactSink, error) {
	s := &compactSink{}
	if useDir {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		s.dir = dir
		s.files = map[string]*bufio.Writer{}
		s.rawFs = map[string]*os.File{}
		return s, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(file)
	if err != nil {
		return nil, err
	}
	s.singleF = f
	s.single = bufio.NewWriterSize(f, 1<<20)
	return s, nil
}

func (s *compactSink) writerFor(key string) (*bufio.Writer, error) {
	if s.single != nil {
		return s.single, nil
	}
	if w, ok := s.files[key]; ok {
		return w, nil
	}
	path := filepath.Join(s.dir, key+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	s.rawFs[key] = f
	s.files[key] = w
	return w, nil
}

func (s *compactSink) Write(c compactWord) error {
	key := shardKey(c.Lemma)
	w, err := s.writerFor(key)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(c)
}

func (s *compactSink) Close() error {
	if s.single != nil {
		if err := s.single.Flush(); err != nil {
			return err
		}
		err := s.singleF.Close()
		s.single = nil
		s.singleF = nil
		return err
	}
	for k, w := range s.files {
		if err := w.Flush(); err != nil {
			return err
		}
		if err := s.rawFs[k].Close(); err != nil {
			return err
		}
	}
	s.files = nil
	s.rawFs = nil
	return nil
}

// formsSink mirrors compactSink for the TSV output. Each shard file gets its
// own header row.
type formsSink struct {
	single  *csv.Writer
	singleF *os.File
	singleW *bufio.Writer
	dir     string
	writers map[string]*csv.Writer
	bws     map[string]*bufio.Writer
	files   map[string]*os.File
}

func newFormsSink(file, dir string, useDir bool) (*formsSink, error) {
	s := &formsSink{}
	if useDir {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		s.dir = dir
		s.writers = map[string]*csv.Writer{}
		s.bws = map[string]*bufio.Writer{}
		s.files = map[string]*os.File{}
		return s, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(file)
	if err != nil {
		return nil, err
	}
	bw := bufio.NewWriterSize(f, 1<<20)
	tsv := csv.NewWriter(bw)
	tsv.Comma = '\t'
	if err := tsv.Write([]string{"lemma", "form", "morph_code"}); err != nil {
		f.Close()
		return nil, err
	}
	s.singleF = f
	s.singleW = bw
	s.single = tsv
	return s, nil
}

func (s *formsSink) writerFor(key string) (*csv.Writer, error) {
	if s.single != nil {
		return s.single, nil
	}
	if w, ok := s.writers[key]; ok {
		return w, nil
	}
	path := filepath.Join(s.dir, key+".tsv")
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	bw := bufio.NewWriterSize(f, 1<<20)
	tsv := csv.NewWriter(bw)
	tsv.Comma = '\t'
	if err := tsv.Write([]string{"lemma", "form", "morph_code"}); err != nil {
		f.Close()
		return nil, err
	}
	s.files[key] = f
	s.bws[key] = bw
	s.writers[key] = tsv
	return tsv, nil
}

func (s *formsSink) Write(lemma string, r formRow) error {
	key := shardKey(lemma)
	w, err := s.writerFor(key)
	if err != nil {
		return err
	}
	return w.Write([]string{r.Lemma, r.Form, r.MorphCode})
}

func (s *formsSink) Close() error {
	if s.single != nil {
		s.single.Flush()
		if err := s.single.Error(); err != nil {
			return err
		}
		if err := s.singleW.Flush(); err != nil {
			return err
		}
		err := s.singleF.Close()
		s.single = nil
		s.singleF = nil
		s.singleW = nil
		return err
	}
	for k, w := range s.writers {
		w.Flush()
		if err := w.Error(); err != nil {
			return err
		}
		if err := s.bws[k].Flush(); err != nil {
			return err
		}
		if err := s.files[k].Close(); err != nil {
			return err
		}
	}
	s.writers = nil
	s.bws = nil
	s.files = nil
	return nil
}
func listRawFiles(rawDir string) ([]string, error) {
	var out []string
	err := filepath.Walk(rawDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".json.gz") {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// sortRawByWordID orders files numerically by the word_id encoded in the
// filename so the compact JSONL has a stable, predictable layout.
func sortRawByWordID(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		return wordIDFromPath(paths[i]) < wordIDFromPath(paths[j])
	})
}

func wordIDFromPath(p string) int64 {
	base := filepath.Base(p)
	base = strings.TrimSuffix(base, ".json.gz")
	n, _ := strconv.ParseInt(base, 10, 64)
	return n
}

func readRaw(path string) (rawWordDetails, error) {
	f, err := os.Open(path)
	if err != nil {
		return rawWordDetails{}, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return rawWordDetails{}, err
	}
	defer gz.Close()
	body, err := io.ReadAll(gz)
	if err != nil {
		return rawWordDetails{}, err
	}
	if len(body) == 0 {
		return rawWordDetails{}, errors.New("empty payload")
	}
	var raw rawWordDetails
	if err := json.Unmarshal(body, &raw); err != nil {
		return rawWordDetails{}, err
	}
	return raw, nil
}
