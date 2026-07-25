package parsecore

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"finnestdb/internal/parserffi"
	"finnestdb/internal/store"
)

const MaxTextChars = 1_500_000

// ParserVersion identifies the parser-behavior iteration that this binary was
// built from. Bumped on parser-affecting PRs and stamped into every eval JSON
// report under `parser_version` so a saved report is self-describing without
// needing the matching git commit on hand.
//
// Convention: YYYY.MM.DD followed by a lowercase iteration letter that maps
// 1:1 to the dated entries in docs/PARSER_EVOLUTION.md (e.g. 2026.05.07j ↔
// §2026-05-07j). For the SemVer-style `parser-vN` scheme used in
// docs/SYSTEM_VERSIONING.md, see that doc - they are the same idea expressed
// at different granularities, and SYSTEM_VERSIONING.md tracks the mapping.
const ParserVersion = "2026.05.15b"

type TokenResult struct {
	Form         string   `json:"form"`
	StubLemma    string   `json:"stub_lemma,omitempty"`
	StubPOS      string   `json:"stub_pos,omitempty"`
	Lemma        string   `json:"lemma"`
	POS          string   `json:"pos"`
	GrammarLabel string   `json:"grammar_label,omitempty"`
	Feats        string   `json:"feats,omitempty"`
	Source       string   `json:"source"`
	Resolved     bool     `json:"resolved"`
	Trace        []string `json:"trace,omitempty"`
	RuleTrace    []string `json:"rule_trace,omitempty"`
}

type SentenceResult struct {
	Text   string        `json:"text"`
	Tokens []TokenResult `json:"tokens"`
	// ChapterIdx tags which chapter this sentence belongs to when the parse
	// was started from a chapters payload (EPUB import flow). Nil otherwise so
	// the field is omitted from the wire format for plain-text parses.
	ChapterIdx *int `json:"chapter_idx,omitempty"`
}

// ChapterInput is one chapter's text submitted to /api/parse via the EPUB
// import flow. The server parses each chapter through the analyzer separately
// so per-chapter views can be derived from a single response without firing
// N additional /api/parse requests.
type ChapterInput struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// ChapterResult is the per-chapter rollup the server returns alongside the
// whole-book aggregation. The client uses TokenCount/LemmaCount for sidebar
// labels and switches the displayed words list to Words when the user clicks
// a chapter row - no extra HTTP needed.
type ChapterResult struct {
	Title            string      `json:"title"`
	CharCount        int         `json:"char_count"`
	SentenceCount    int         `json:"sentence_count"`
	TokenCount       int         `json:"token_count"` // non-punct tokens
	ResolvedTokens   int         `json:"resolved_tokens"`
	UnresolvedTokens int         `json:"unresolved_tokens"`
	LemmaCount       int         `json:"lemma_count"` // unique (lemma,pos) pairs
	Words            []WordEntry `json:"words"`       // per-chapter aggregation
}

type WordEntry struct {
	Lemma           string   `json:"lemma"`
	POS             string   `json:"pos"`
	Forms           []string `json:"forms"`
	Count           int      `json:"count"`
	Gloss           string   `json:"gloss,omitempty"`
	GrammarLabel    string   `json:"grammar_label,omitempty"`
	Feats           string   `json:"feats,omitempty"`
	ExampleSentence string   `json:"example_sentence,omitempty"`
	LearningState   string   `json:"learning_state,omitempty"`
}

type ParseTimings struct {
	AnalyzeNs          int64 `json:"analyze_ns"`
	LookupFormsNs      int64 `json:"lookup_forms_ns"`
	LookupGlossesNs    int64 `json:"lookup_glosses_ns"`
	ResolveSentencesNs int64 `json:"resolve_sentences_ns"`
	EnrichWordsNs      int64 `json:"enrich_words_ns"`
	TotalNs            int64 `json:"total_ns"`
}

type ParseStats struct {
	UniqueForms      int            `json:"unique_forms"`
	TotalSentences   int            `json:"total_sentences"`
	ResolvedTokens   int            `json:"resolved_tokens"`
	UnresolvedTokens int            `json:"unresolved_tokens"`
	PunctTokens      int            `json:"punct_tokens"`
	SourceCounts     map[string]int `json:"source_counts,omitempty"`
	Timings          ParseTimings   `json:"timings"`
}

type ParseResult struct {
	Lang            string           `json:"lang"`
	Parser          string           `json:"parser"`
	TotalTokens     int              `json:"total_tokens"`
	ParseDurationNs int64            `json:"parse_duration_ns"`
	Stats           ParseStats       `json:"stats"`
	Words           []WordEntry      `json:"words"`
	Sentences       []SentenceResult `json:"sentences"`
	// Chapters is set only when the parse was started from a chapters payload
	// (EPUB import flow). Each entry holds per-chapter rollup stats + the
	// aggregated WordEntry list scoped to that chapter, so the client can
	// switch chapter views without an extra /api/parse round-trip.
	Chapters []ChapterResult `json:"chapters,omitempty"`
}

type parser interface {
	Name() string
	Parse(db *store.DB, lang, text string) (*ParseResult, error)
}

type parsedToken struct {
	Form         string
	StubLemma    string
	StubPOS      string
	GrammarLabel string
	Feats        string
}

type parsedSentence struct {
	Tokens []parsedToken
	Text   string
	// chapterIdx propagates the chapter origin from AnalyzeChapters down to
	// the SentenceResult. nil for the plain-text path.
	chapterIdx *int
}

// AnalyzerFunc is the parser-FFI-shaped adapter contract used by production
// dictionary parsers and eval-only external analyzers.
type AnalyzerFunc func(lang, text string) (*parserffi.AnalysisResult, error)

func ValidateInput(lang, text string) error {
	if lang != "FI" && lang != "ET" {
		return fmt.Errorf("language must be FI or ET")
	}
	if text == "" {
		return fmt.Errorf("text is required")
	}
	if utf8.RuneCountInString(text) > MaxTextChars {
		return fmt.Errorf("text exceeds %d character limit", MaxTextChars)
	}
	return nil
}

// ValidateChaptersInput mirrors ValidateInput for the chapters payload. The
// cap is applied to the SUM of chapter rune counts so a chapters request and
// an equivalent text request enforce the same total.
func ValidateChaptersInput(lang string, chapters []ChapterInput) error {
	if lang != "FI" && lang != "ET" {
		return fmt.Errorf("language must be FI or ET")
	}
	if len(chapters) == 0 {
		return fmt.Errorf("chapters is required")
	}
	total := 0
	hasContent := false
	for _, ch := range chapters {
		if strings.TrimSpace(ch.Text) != "" {
			hasContent = true
		}
		total += utf8.RuneCountInString(ch.Text)
	}
	if !hasContent {
		return fmt.Errorf("chapters is required")
	}
	if total > MaxTextChars {
		return fmt.Errorf("text exceeds %d character limit", MaxTextChars)
	}
	return nil
}

func Analyze(db *store.DB, lang, text, parserName string) (*ParseResult, error) {
	if err := ValidateInput(lang, text); err != nil {
		return nil, err
	}
	if parserName == "" {
		parserName = "basic"
	}

	p, ok := registry()[parserName]
	if !ok {
		return nil, fmt.Errorf("unsupported parser %q", parserName)
	}
	return p.Parse(db, lang, text)
}

// AnalyzeChapters runs the standard parse pipeline on a chapters payload
// (typically from the EPUB import flow). Each chapter is sent through the
// analyzer separately so its sentences can be tagged with chapter_idx, but
// the dictionary lookup, gloss lookup, and word enrichment passes happen
// once across the whole book - collapsing N+1 HTTP round-trips down to one.
// Per-chapter Words are computed by re-enriching each chapter's sentence
// subset so the client can swap views without firing extra requests.
func AnalyzeChapters(db *store.DB, lang string, chapters []ChapterInput, parserName string) (*ParseResult, error) {
	if err := ValidateChaptersInput(lang, chapters); err != nil {
		return nil, err
	}
	if parserName == "" {
		parserName = "basic"
	}
	p, ok := registry()[parserName]
	if !ok {
		return nil, fmt.Errorf("unsupported parser %q", parserName)
	}
	dp, ok := p.(dictionaryParser)
	if !ok {
		return nil, fmt.Errorf("parser %q does not support chapters payload", parserName)
	}
	return dp.ParseChapters(db, lang, chapters)
}

func SupportedParsers() []string {
	names := make([]string, 0, len(registry()))
	for name := range registry() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func registry() map[string]parser {
	return map[string]parser{
		"basic":  dictionaryParser{name: "basic", lookupMode: "basic", analyzer: parserffi.AnalyzeText},
		"custom": dictionaryParser{name: "custom", lookupMode: "custom", analyzer: parserffi.AnalyzeText},
	}
}

type dictionaryParser struct {
	name       string
	lookupMode string
	analyzer   AnalyzerFunc
}

func (p dictionaryParser) Name() string { return p.name }

func (p dictionaryParser) Parse(db *store.DB, lang, text string) (*ParseResult, error) {
	parseStartedAt := time.Now()
	result, err := p.analyzer(lang, text)
	analyzeNs := time.Since(parseStartedAt).Nanoseconds()
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("parser returned no result")
	}

	sentences := toParsedSentences(result)

	lookupStartedAt := time.Now()
	uniqueForms := collectUniqueForms(sentences)
	formResolutions := db.BatchLookupForms(uniqueForms, lang, p.lookupMode)
	lookupFormsNs := time.Since(lookupStartedAt).Nanoseconds()

	glossLookupStartedAt := time.Now()
	lemmaKeys := resolvedLemmaKeys(formResolutions)
	glosses := db.BatchLookupGlosses(lemmaKeys, lang)
	lookupGlossesNs := time.Since(glossLookupStartedAt).Nanoseconds()

	resolveStartedAt := time.Now()
	detailedSentences := resolveDictionarySentences(sentences, formResolutions)
	resolveSentencesNs := time.Since(resolveStartedAt).Nanoseconds()

	enrichStartedAt := time.Now()
	words := enrichWords(detailedSentences, glosses)
	enrichWordsNs := time.Since(enrichStartedAt).Nanoseconds()
	parseDurationNs := time.Since(parseStartedAt).Nanoseconds()
	stats := computeParseStats(detailedSentences, len(uniqueForms), ParseTimings{
		AnalyzeNs:          analyzeNs,
		LookupFormsNs:      lookupFormsNs,
		LookupGlossesNs:    lookupGlossesNs,
		ResolveSentencesNs: resolveSentencesNs,
		EnrichWordsNs:      enrichWordsNs,
		TotalNs:            parseDurationNs,
	})

	return &ParseResult{
		Lang:            lang,
		Parser:          p.name,
		TotalTokens:     countTokens(words),
		ParseDurationNs: parseDurationNs,
		Stats:           stats,
		Words:           words,
		Sentences:       detailedSentences,
	}, nil
}

// ParseChapters analyzes each chapter through the analyzer separately so
// sentences can be tagged with chapter_idx, then runs the standard
// lookup/gloss/enrich pipeline once across the whole book. Per-chapter Words
// are produced by re-enriching each chapter's sentence subset against the
// shared gloss map. Skipped chapters (empty or whitespace-only text) still
// emit a zero-stat ChapterResult so the response index matches the input.
func (p dictionaryParser) ParseChapters(db *store.DB, lang string, chapters []ChapterInput) (*ParseResult, error) {
	parseStartedAt := time.Now()

	analyzeStartedAt := time.Now()
	allSentences := make([]parsedSentence, 0)
	for chIdx, ch := range chapters {
		if strings.TrimSpace(ch.Text) == "" {
			continue
		}
		result, err := p.analyzer(lang, ch.Text)
		if err != nil {
			return nil, fmt.Errorf("parse error: %w", err)
		}
		if result == nil {
			continue
		}
		idxCopy := chIdx
		chSentences := toParsedSentences(result)
		for i := range chSentences {
			chSentences[i].chapterIdx = &idxCopy
		}
		allSentences = append(allSentences, chSentences...)
	}
	analyzeNs := time.Since(analyzeStartedAt).Nanoseconds()

	lookupStartedAt := time.Now()
	uniqueForms := collectUniqueForms(allSentences)
	formResolutions := db.BatchLookupForms(uniqueForms, lang, p.lookupMode)
	lookupFormsNs := time.Since(lookupStartedAt).Nanoseconds()

	glossLookupStartedAt := time.Now()
	lemmaKeys := resolvedLemmaKeys(formResolutions)
	glosses := db.BatchLookupGlosses(lemmaKeys, lang)
	lookupGlossesNs := time.Since(glossLookupStartedAt).Nanoseconds()

	resolveStartedAt := time.Now()
	detailedSentences := resolveDictionarySentences(allSentences, formResolutions)
	resolveSentencesNs := time.Since(resolveStartedAt).Nanoseconds()

	enrichStartedAt := time.Now()
	words := enrichWords(detailedSentences, glosses)
	chapterResults := computeChapterResults(chapters, detailedSentences, glosses)
	enrichWordsNs := time.Since(enrichStartedAt).Nanoseconds()
	parseDurationNs := time.Since(parseStartedAt).Nanoseconds()
	stats := computeParseStats(detailedSentences, len(uniqueForms), ParseTimings{
		AnalyzeNs:          analyzeNs,
		LookupFormsNs:      lookupFormsNs,
		LookupGlossesNs:    lookupGlossesNs,
		ResolveSentencesNs: resolveSentencesNs,
		EnrichWordsNs:      enrichWordsNs,
		TotalNs:            parseDurationNs,
	})

	return &ParseResult{
		Lang:            lang,
		Parser:          p.name,
		TotalTokens:     countTokens(words),
		ParseDurationNs: parseDurationNs,
		Stats:           stats,
		Words:           words,
		Sentences:       detailedSentences,
		Chapters:        chapterResults,
	}, nil
}

// computeChapterResults buckets resolved sentences by chapter_idx and produces
// per-chapter stats + a re-aggregated Words list scoped to that chapter. The
// gloss map is shared with the whole-book pass so per-chapter entries surface
// the same definitions without a second BatchLookupGlosses round-trip.
func computeChapterResults(chapters []ChapterInput, sentences []SentenceResult, glosses map[store.LemmaKey]string) []ChapterResult {
	out := make([]ChapterResult, len(chapters))
	for i, ch := range chapters {
		out[i] = ChapterResult{
			Title:     ch.Title,
			CharCount: utf8.RuneCountInString(ch.Text),
		}
	}
	buckets := make([][]SentenceResult, len(chapters))
	for _, sent := range sentences {
		if sent.ChapterIdx == nil {
			continue
		}
		idx := *sent.ChapterIdx
		if idx < 0 || idx >= len(chapters) {
			continue
		}
		buckets[idx] = append(buckets[idx], sent)
		out[idx].SentenceCount++
		for _, tok := range sent.Tokens {
			if tok.POS == "PUNCT" {
				continue
			}
			out[idx].TokenCount++
			if tok.Resolved {
				out[idx].ResolvedTokens++
			} else {
				out[idx].UnresolvedTokens++
			}
		}
	}
	for i := range chapters {
		if len(buckets[i]) == 0 {
			continue
		}
		out[i].Words = enrichWords(buckets[i], glosses)
		out[i].LemmaCount = len(out[i].Words)
	}
	return out
}

// featsFromJSON converts the analyzer FFI's JSON-object FEATS payload into
// the UD pipe-separated string ("Case=Ine|Number=Sing") that the rest of the
// pipeline uses. Returns "" for null/empty/non-object payloads. Keys are
// emitted in alphabetical order to match UD convention so equality checks
// against gold strings are stable. Multi-value attributes (e.g. arrays) are
// joined with "," - also UD convention.
func featsFromJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || len(obj) == 0 {
		return ""
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		val := featsValueString(obj[k])
		if val == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('|')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(val)
	}
	return b.String()
}

func featsValueString(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	}
	return ""
}

func toParsedSentences(result *parserffi.AnalysisResult) []parsedSentence {
	sentences := make([]parsedSentence, 0, len(result.Sentences))
	for _, s := range result.Sentences {
		tokens := make([]parsedToken, 0, len(s.Tokens))
		var textBuilder strings.Builder
		prevWasOpen := false

		for i, t := range s.Tokens {
			if t.Form == "" {
				continue
			}
			tokens = append(tokens, parsedToken{
				Form:         t.Form,
				StubLemma:    t.Lemma,
				StubPOS:      t.POS,
				GrammarLabel: t.GrammarLabel,
				Feats:        featsFromJSON(t.Feats),
			})

			isPunct := t.POS == "PUNCT"
			isClose := isPunct && t.GrammarLabel == "PUNCT_CLOSE"
			isOpen := isPunct && t.GrammarLabel == "PUNCT_OPEN"
			if i > 0 && !isClose && !prevWasOpen {
				textBuilder.WriteByte(' ')
			}
			textBuilder.WriteString(t.Form)
			prevWasOpen = isOpen
		}

		text := textBuilder.String()
		if text != "" {
			sentences = append(sentences, parsedSentence{Tokens: tokens, Text: text})
		}
	}
	return sentences
}

func collectUniqueForms(sentences []parsedSentence) []string {
	seen := make(map[string]struct{})
	for _, s := range sentences {
		for _, t := range s.Tokens {
			if t.StubPOS == "PUNCT" {
				continue
			}
			seen[t.Form] = struct{}{}
		}
	}
	forms := make([]string, 0, len(seen))
	for f := range seen {
		forms = append(forms, f)
	}
	return forms
}

func resolvedLemmaKeys(formResolutions map[string]store.FormResolution) []store.LemmaKey {
	seen := make(map[store.LemmaKey]struct{}, len(formResolutions))
	for _, v := range formResolutions {
		seen[store.LemmaKey{Lemma: v.Lemma, POS: v.POS}] = struct{}{}
	}
	keys := make([]store.LemmaKey, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return keys
}

func resolveDictionarySentences(sentences []parsedSentence, formResolutions map[string]store.FormResolution) []SentenceResult {
	out := make([]SentenceResult, 0, len(sentences))
	for _, sent := range sentences {
		tokens := make([]TokenResult, 0, len(sent.Tokens))
		for _, token := range sent.Tokens {
			resolved := TokenResult{
				Form:      token.Form,
				StubLemma: token.StubLemma,
				StubPOS:   token.StubPOS,
				Trace:     []string{fmt.Sprintf("analyzer:rust lemma=%s pos=%s", token.StubLemma, token.StubPOS)},
			}
			if token.StubPOS == "PUNCT" {
				resolved.Lemma = token.Form
				resolved.POS = token.StubPOS
				resolved.Source = "punct"
				tokens = append(tokens, resolved)
				continue
			}

			if dictRes, ok := formResolutions[token.Form]; ok {
				resolved.Lemma = dictRes.Lemma
				resolved.POS = dictRes.POS
				resolved.GrammarLabel = dictRes.GrammarLabel
				resolved.Feats = dictRes.Feats
				resolved.Source = dictRes.Source
				resolved.Resolved = true
				resolved.Trace = append(resolved.Trace, fmt.Sprintf("resolution:%s lemma=%s pos=%s", dictRes.Source, dictRes.Lemma, dictRes.POS))
			} else {
				resolved.Lemma = strings.ToLower(token.StubLemma)
				if resolved.Lemma == "" {
					resolved.Lemma = strings.ToLower(token.Form)
				}
				resolved.POS = token.StubPOS
				resolved.Source = "stub"
				resolved.Trace = append(resolved.Trace, fmt.Sprintf("fallback:stub lemma=%s pos=%s", resolved.Lemma, resolved.POS))
			}
			tokens = append(tokens, resolved)
		}
		out = append(out, SentenceResult{Text: sent.Text, Tokens: tokens, ChapterIdx: sent.chapterIdx})
	}
	return out
}

func enrichWords(sentences []SentenceResult, glosses map[store.LemmaKey]string) []WordEntry {
	type key struct{ lemma, pos string }
	counts := make(map[key]int)
	formSets := make(map[key]map[string]struct{})
	firstSentence := make(map[key]string)
	grammarLabelSets := make(map[key]map[string]struct{})
	featsSets := make(map[key]map[string]struct{})
	normalizedFormSets := make(map[key]map[string]struct{})

	for _, sent := range sentences {
		for _, token := range sent.Tokens {
			if strings.TrimSpace(token.Form) == "" || token.POS == "PUNCT" || token.Lemma == "" {
				continue
			}
			k := key{lemma: token.Lemma, pos: token.POS}
			counts[k]++
			if formSets[k] == nil {
				formSets[k] = make(map[string]struct{})
			}
			formSets[k][token.Form] = struct{}{}
			if normalizedFormSets[k] == nil {
				normalizedFormSets[k] = make(map[string]struct{})
			}
			normalizedFormSets[k][strings.ToLower(token.Form)] = struct{}{}
			if _, seen := firstSentence[k]; !seen && sent.Text != "" {
				firstSentence[k] = sent.Text
			}
			if token.GrammarLabel != "" {
				if grammarLabelSets[k] == nil {
					grammarLabelSets[k] = make(map[string]struct{})
				}
				grammarLabelSets[k][token.GrammarLabel] = struct{}{}
			}
			if token.Feats != "" {
				if featsSets[k] == nil {
					featsSets[k] = make(map[string]struct{})
				}
				featsSets[k][token.Feats] = struct{}{}
			}
		}
	}

	words := make([]WordEntry, 0, len(counts))
	for k, count := range counts {
		forms := make([]string, 0, len(formSets[k]))
		for f := range formSets[k] {
			forms = append(forms, f)
		}
		sort.Strings(forms)
		lk := store.LemmaKey{Lemma: k.lemma, POS: k.pos}
		grammarLabel, feats := aggregateMorphologyForWord(k.pos, normalizedFormSets[k], grammarLabelSets[k], featsSets[k])
		words = append(words, WordEntry{
			Lemma:           k.lemma,
			POS:             k.pos,
			Forms:           forms,
			Count:           count,
			Gloss:           glosses[lk],
			GrammarLabel:    grammarLabel,
			Feats:           feats,
			ExampleSentence: firstSentence[k],
		})
	}

	sort.Slice(words, func(i, j int) bool {
		if words[i].Count != words[j].Count {
			return words[i].Count > words[j].Count
		}
		return words[i].Lemma < words[j].Lemma
	})
	return words
}

func aggregateMorphologyForWord(pos string, normalizedForms, grammarLabels, feats map[string]struct{}) (string, string) {
	if len(normalizedForms) != 1 {
		return "", ""
	}
	grammarLabel := ""
	if learnerGrammarLabelPOS(pos) {
		grammarLabel = singleSetValue(grammarLabels)
	}
	return grammarLabel, singleSetValue(feats)
}

func learnerGrammarLabelPOS(pos string) bool {
	switch pos {
	case "ADJ", "NOUN", "NUM", "PRON", "PROPN":
		return true
	default:
		return false
	}
}

func singleSetValue(values map[string]struct{}) string {
	if len(values) != 1 {
		return ""
	}
	for value := range values {
		return value
	}
	return ""
}

func countTokens(words []WordEntry) int {
	total := 0
	for _, w := range words {
		total += w.Count
	}
	return total
}

func computeParseStats(sentences []SentenceResult, uniqueForms int, timings ParseTimings) ParseStats {
	stats := ParseStats{
		UniqueForms:    uniqueForms,
		TotalSentences: len(sentences),
		SourceCounts:   make(map[string]int),
		Timings:        timings,
	}
	for _, sentence := range sentences {
		for _, token := range sentence.Tokens {
			stats.SourceCounts[token.Source]++
			if token.POS == "PUNCT" {
				stats.PunctTokens++
				continue
			}
			if token.Resolved {
				stats.ResolvedTokens++
			} else {
				stats.UnresolvedTokens++
			}
		}
	}
	return stats
}
