package parsecore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"finnestdb/internal/parserffi"
	"finnestdb/internal/store"
)

const MaxTextChars = 300_000

// ParserVersion identifies the parser-behavior iteration that this binary was
// built from. Bumped on parser-affecting PRs and stamped into every eval JSON
// report under `parser_version` so a saved report is self-describing without
// needing the matching git commit on hand.
//
// Convention: YYYY.MM.DD followed by a lowercase iteration letter that maps
// 1:1 to the dated entries in docs/PARSER_EVOLUTION.md (e.g. 2026.05.07j ↔
// §2026-05-07j). For the SemVer-style `parser-vN` scheme used in
// docs/SYSTEM_VERSIONING.md, see that doc — they are the same idea expressed
// at different granularities, and SYSTEM_VERSIONING.md tracks the mapping.
const ParserVersion = "2026.05.07k"

const omorfiCommandEnv = "FINNESTDB_OMORFI_CMD"
const estnltkCommandEnv = "FINNESTDB_ESTNLTK_CMD"

// External-analyzer subprocess timeouts. Both can be overridden with a Go
// duration string ("30s", "1m"). EstNLTK defaults higher because each call
// pays ~1s of Vabamorf model load before any analysis runs.
const omorfiTimeoutEnv = "FINNESTDB_OMORFI_TIMEOUT"
const estnltkTimeoutEnv = "FINNESTDB_ESTNLTK_TIMEOUT"
const omorfiDefaultTimeout = 5 * time.Second
const estnltkDefaultTimeout = 30 * time.Second

// analyzerTimeout reads a Go duration string from envVar and returns it,
// falling back to defaultDur on empty, malformed, or non-positive input.
func analyzerTimeout(envVar string, defaultDur time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(envVar))
	if raw == "" {
		return defaultDur
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return defaultDur
	}
	return parsed
}

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
}

type ParserDefinition struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Languages   []string `json:"languages,omitempty"`
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
}

type baseAnalyzer func(lang, text string) (*parserffi.AnalysisResult, error)

func Analyze(db *store.DB, lang, text, parserName string) (*ParseResult, error) {
	if lang != "FI" && lang != "ET" {
		return nil, fmt.Errorf("language must be FI or ET")
	}
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if utf8.RuneCountInString(text) > MaxTextChars {
		return nil, fmt.Errorf("text exceeds %d character limit", MaxTextChars)
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

func SupportedParsers() []string {
	names := make([]string, 0, len(registry()))
	for name := range registry() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ParserDefinitions() []ParserDefinition {
	return []ParserDefinition{
		{Name: "basic", Description: "Rust tokenizer plus direct dictionary lookup", Languages: []string{"FI", "ET"}},
		{Name: "custom", Description: "Rust tokenizer plus possessive, compound, and case-suffix fallbacks", Languages: []string{"FI", "ET"}},
		{Name: "omorfi", Description: "External Finnish baseline adapter with inspectable override rules", Languages: []string{"FI"}},
		{Name: "estnltk", Description: "External Estonian EstNLTK/Vabamorf baseline adapter with inspectable override rules", Languages: []string{"ET"}},
	}
}

func registry() map[string]parser {
	return map[string]parser{
		"basic":  dictionaryParser{name: "basic", lookupMode: "basic", analyzer: parserffi.AnalyzeText},
		"custom": dictionaryParser{name: "custom", lookupMode: "custom", analyzer: parserffi.AnalyzeText},
		"omorfi": externalAnalyzerParser{
			name:        "omorfi",
			lang:        "FI",
			source:      "analyzer:omorfi",
			analyzer:    runExternalOmorfi,
			overrideSet: defaultExternalAnalyzerRules,
		},
		"estnltk": externalAnalyzerParser{
			name:        "estnltk",
			lang:        "ET",
			source:      "analyzer:estnltk",
			analyzer:    runExternalEstNLTK,
			overrideSet: defaultExternalAnalyzerRules,
		},
	}
}

type dictionaryParser struct {
	name       string
	lookupMode string
	analyzer   baseAnalyzer
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

type externalAnalyzerParser struct {
	name        string
	lang        string
	source      string
	analyzer    baseAnalyzer
	overrideSet func() []externalAnalyzerRule
}

func (p externalAnalyzerParser) Name() string { return p.name }

func (p externalAnalyzerParser) Parse(db *store.DB, lang, text string) (*ParseResult, error) {
	if lang != p.lang {
		return nil, fmt.Errorf("%s parser only supports %s", p.name, p.lang)
	}
	parseStartedAt := time.Now()
	result, err := p.analyzer(lang, text)
	analyzeNs := time.Since(parseStartedAt).Nanoseconds()
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%s parser returned no result", p.name)
	}

	sentences := toParsedSentences(result)

	lookupStartedAt := time.Now()
	uniqueForms := collectUniqueForms(sentences)
	directResolutions := db.BatchLookupForms(uniqueForms, lang, "basic")
	customResolutions := db.BatchLookupForms(uniqueForms, lang, "custom")
	lookupFormsNs := time.Since(lookupStartedAt).Nanoseconds()
	rules := p.overrideSet()

	resolveStartedAt := time.Now()
	detailedSentences := make([]SentenceResult, 0, len(sentences))
	lemmaSet := make(map[store.LemmaKey]struct{})
	for _, sent := range sentences {
		outSent := SentenceResult{Text: sent.Text, Tokens: make([]TokenResult, 0, len(sent.Tokens))}
		for _, token := range sent.Tokens {
			resolved := TokenResult{
				Form:         token.Form,
				StubLemma:    token.StubLemma,
				StubPOS:      token.StubPOS,
				Lemma:        strings.ToLower(token.StubLemma),
				POS:          token.StubPOS,
				GrammarLabel: token.GrammarLabel,
				Feats:        token.Feats,
				Source:       p.source,
				Resolved:     token.StubLemma != "" && token.StubPOS != "" && token.StubPOS != "X",
				Trace:        []string{fmt.Sprintf("%s lemma=%s pos=%s", p.source, token.StubLemma, token.StubPOS)},
			}
			if token.StubPOS == "PUNCT" {
				resolved.Lemma = token.Form
				resolved.POS = token.StubPOS
				resolved.Source = "punct"
				outSent.Tokens = append(outSent.Tokens, resolved)
				continue
			}
			if resolved.Lemma == "" {
				resolved.Lemma = strings.ToLower(token.Form)
			}

			for _, rule := range rules {
				if applied := rule.Apply(lang, &resolved, directResolutions[token.Form], customResolutions[token.Form]); applied {
					resolved.RuleTrace = append(resolved.RuleTrace, rule.Name())
				}
			}

			if resolved.Lemma != "" && resolved.POS != "" && resolved.POS != "PUNCT" {
				lemmaSet[store.LemmaKey{Lemma: resolved.Lemma, POS: resolved.POS}] = struct{}{}
			}
			outSent.Tokens = append(outSent.Tokens, resolved)
		}
		detailedSentences = append(detailedSentences, outSent)
	}
	resolveSentencesNs := time.Since(resolveStartedAt).Nanoseconds()

	glossLookupStartedAt := time.Now()
	lemmaKeys := make([]store.LemmaKey, 0, len(lemmaSet))
	for key := range lemmaSet {
		lemmaKeys = append(lemmaKeys, key)
	}
	glosses := db.BatchLookupGlosses(lemmaKeys, lang)
	lookupGlossesNs := time.Since(glossLookupStartedAt).Nanoseconds()

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

type externalAnalyzerRule interface {
	Name() string
	Apply(lang string, token *TokenResult, direct, custom store.FormResolution) bool
}

type externalPreferDirectDictRule struct{}

func (externalPreferDirectDictRule) Name() string { return "prefer_direct_dict_when_unknown" }

func (externalPreferDirectDictRule) Apply(_ string, token *TokenResult, direct, _ store.FormResolution) bool {
	if direct.Lemma == "" {
		return false
	}
	if token.Resolved && token.POS != "X" {
		return false
	}
	token.Trace = append(token.Trace, fmt.Sprintf("rule:direct_dict lemma=%s pos=%s", direct.Lemma, direct.POS))
	token.Lemma = direct.Lemma
	token.POS = direct.POS
	if direct.GrammarLabel != "" {
		token.GrammarLabel = direct.GrammarLabel
	}
	if direct.Feats != "" {
		token.Feats = direct.Feats
	}
	token.Source = "override:direct_dict"
	token.Resolved = true
	return true
}

type externalPreferCustomFallbackRule struct{}

func (externalPreferCustomFallbackRule) Name() string { return "prefer_custom_fallback_when_unknown" }

func (externalPreferCustomFallbackRule) Apply(_ string, token *TokenResult, _, custom store.FormResolution) bool {
	if custom.Lemma == "" {
		return false
	}
	if token.Resolved && token.POS != "X" {
		return false
	}
	token.Trace = append(token.Trace, fmt.Sprintf("rule:custom_fallback lemma=%s pos=%s source=%s", custom.Lemma, custom.POS, custom.Source))
	token.Lemma = custom.Lemma
	token.POS = custom.POS
	token.GrammarLabel = custom.GrammarLabel
	token.Feats = custom.Feats
	token.Source = "override:" + custom.Source
	token.Resolved = true
	return true
}

type externalAttachMorphologyRule struct{}

func (externalAttachMorphologyRule) Name() string { return "attach_custom_morphology" }

// Apply attaches custom GrammarLabel and/or Feats to an already-resolved
// analyzer token when the analyzer has no morphology of its own and lemma/POS
// agree. Fires for label-only customs (legacy case-suffix path), feats-only
// customs (FST verb morphology like Number/Tense/Mood/Person — no case label),
// and the both-present case. The earlier label-only gate dropped FEATS-only
// FST analyses on the floor when omorfi/estnltk had the lemma but no FEATS.
func (externalAttachMorphologyRule) Apply(_ string, token *TokenResult, _, custom store.FormResolution) bool {
	tokenNeedsLabel := token.GrammarLabel == "" && custom.GrammarLabel != ""
	tokenNeedsFeats := token.Feats == "" && custom.Feats != ""
	if !tokenNeedsLabel && !tokenNeedsFeats {
		return false
	}
	if custom.Lemma != "" && token.Lemma != custom.Lemma {
		return false
	}
	if custom.POS != "" && token.POS != custom.POS {
		return false
	}
	traceParts := make([]string, 0, 2)
	if tokenNeedsLabel {
		token.GrammarLabel = custom.GrammarLabel
		traceParts = append(traceParts, "label="+custom.GrammarLabel)
	}
	if tokenNeedsFeats {
		token.Feats = custom.Feats
		traceParts = append(traceParts, "feats="+custom.Feats)
	}
	token.Trace = append(token.Trace, "rule:attach_morphology "+strings.Join(traceParts, " "))
	return true
}

func defaultExternalAnalyzerRules() []externalAnalyzerRule {
	return []externalAnalyzerRule{
		externalPreferDirectDictRule{},
		externalPreferCustomFallbackRule{},
		externalAttachMorphologyRule{},
	}
}

func runExternalOmorfi(lang, text string) (*parserffi.AnalysisResult, error) {
	cmdSpec := strings.TrimSpace(os.Getenv(omorfiCommandEnv))
	if cmdSpec == "" {
		// Auto-default: when the bundled adapter script and python3 are
		// available, run them directly. Avoids requiring a per-shell env var
		// for the common dev-environment case after `make setup-omorfi`.
		//
		// Search order for the adapter script (cwd-independent — covers
		// `go run` from the repo root, installed binaries, and systemd):
		//   1. ./scripts/omorfi_adapter_example.py (cwd is the repo root)
		//   2. <repo>/scripts/omorfi_adapter_example.py where <repo> is
		//      walked up from the test executable / cwd looking for go.mod
		//   3. <executable-dir>/scripts/omorfi_adapter_example.py
		if py, err := exec.LookPath("python3"); err == nil {
			if path, ok := findOmorfiAdapter(); ok {
				cmdSpec = py + " " + path
			}
		}
	}
	if cmdSpec == "" {
		return nil, fmt.Errorf("omorfi parser is not configured; set %s or run `make setup-omorfi`", omorfiCommandEnv)
	}
	fields := strings.Fields(cmdSpec)
	if len(fields) == 0 {
		return nil, fmt.Errorf("omorfi parser command is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), analyzerTimeout(omorfiTimeoutEnv, omorfiDefaultTimeout))
	defer cancel()

	args := append(fields[1:], "--lang", lang)
	cmd := exec.CommandContext(ctx, fields[0], args...)
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("omorfi parser timed out")
	}
	if err != nil {
		return nil, fmt.Errorf("omorfi parser failed: %w", err)
	}

	var result parserffi.AnalysisResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("omorfi parser returned invalid JSON: %w", err)
	}
	return &result, nil
}

func runExternalEstNLTK(lang, text string) (*parserffi.AnalysisResult, error) {
	cmdSpec := strings.TrimSpace(os.Getenv(estnltkCommandEnv))
	if cmdSpec == "" {
		if py, err := exec.LookPath("python3"); err == nil {
			if path, ok := findEstNLTKAdapter(); ok {
				if venvPy, ok := findSiblingVenvPython(path, ".venv-estnltk"); ok {
					py = venvPy
				}
				cmdSpec = py + " " + path
			}
		}
	}
	if cmdSpec == "" {
		return nil, fmt.Errorf("estnltk parser is not configured; set %s or run `make setup-estnltk`", estnltkCommandEnv)
	}
	fields := strings.Fields(cmdSpec)
	if len(fields) == 0 {
		return nil, fmt.Errorf("estnltk parser command is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), analyzerTimeout(estnltkTimeoutEnv, estnltkDefaultTimeout))
	defer cancel()

	args := append(fields[1:], "--lang", lang)
	cmd := exec.CommandContext(ctx, fields[0], args...)
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("estnltk parser timed out")
	}
	if err != nil {
		return nil, fmt.Errorf("estnltk parser failed: %w", err)
	}

	var result parserffi.AnalysisResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("estnltk parser returned invalid JSON: %w", err)
	}
	return &result, nil
}

func findEstNLTKAdapter() (string, bool) {
	return findRepoScript("scripts/estnltk_adapter_example.py")
}

// findOmorfiAdapter locates the bundled python adapter script in a way that
// works whether the caller's cwd is the repo root, a sub-package directory
// (`go test ./internal/parsecore`), or an installed-binary deployment.
//
// Returns the absolute path to the script and true on success.
func findOmorfiAdapter() (string, bool) {
	return findRepoScript("scripts/omorfi_adapter_example.py")
}

func findRepoScript(scriptRel string) (string, bool) {
	// 1. cwd-relative.
	if abs, err := filepath.Abs(scriptRel); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs, true
		}
	}

	// 2. Walk up from cwd looking for go.mod (repo root).
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 8; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				candidate := filepath.Join(dir, scriptRel)
				if _, err := os.Stat(candidate); err == nil {
					return candidate, true
				}
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 3. Same directory as the running executable.
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), scriptRel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}

	return "", false
}

func findSiblingVenvPython(scriptPath, venvName string) (string, bool) {
	dir := filepath.Dir(scriptPath)
	for i := 0; i < 4; i++ {
		if filepath.Base(dir) == "scripts" {
			repoRoot := filepath.Dir(dir)
			candidate := filepath.Join(repoRoot, venvName, "bin", "python")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, true
			}
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// featsFromJSON converts the analyzer FFI's JSON-object FEATS payload into
// the UD pipe-separated string ("Case=Ine|Number=Sing") that the rest of the
// pipeline uses. Returns "" for null/empty/non-object payloads. Keys are
// emitted in alphabetical order to match UD convention so equality checks
// against gold strings are stable. Multi-value attributes (e.g. arrays) are
// joined with "," — also UD convention.
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
		out = append(out, SentenceResult{Text: sent.Text, Tokens: tokens})
	}
	return out
}

func enrichWords(sentences []SentenceResult, glosses map[store.LemmaKey]string) []WordEntry {
	type key struct{ lemma, pos string }
	counts := make(map[key]int)
	formSets := make(map[key]map[string]struct{})
	firstSentence := make(map[key]string)
	grammarLabels := make(map[key]string)
	feats := make(map[key]string)

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
			if _, seen := firstSentence[k]; !seen && sent.Text != "" {
				firstSentence[k] = sent.Text
			}
			if token.GrammarLabel != "" {
				if _, seen := grammarLabels[k]; !seen {
					grammarLabels[k] = token.GrammarLabel
				}
			}
			if token.Feats != "" {
				if _, seen := feats[k]; !seen {
					feats[k] = token.Feats
				}
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
		words = append(words, WordEntry{
			Lemma:           k.lemma,
			POS:             k.pos,
			Forms:           forms,
			Count:           count,
			Gloss:           glosses[lk],
			GrammarLabel:    grammarLabels[k],
			Feats:           feats[k],
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
