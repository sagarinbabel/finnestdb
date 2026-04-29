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

const omorfiCommandEnv = "FINNESTDB_OMORFI_CMD"

type TokenResult struct {
	Form         string   `json:"form"`
	StubLemma    string   `json:"stub_lemma,omitempty"`
	StubPOS      string   `json:"stub_pos,omitempty"`
	Lemma        string   `json:"lemma"`
	POS          string   `json:"pos"`
	GrammarLabel string   `json:"grammar_label,omitempty"`
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
	ExampleSentence string   `json:"example_sentence,omitempty"`
}

type ParseTimings struct {
	AnalyzeMs          int64 `json:"analyze_ms"`
	LookupFormsMs      int64 `json:"lookup_forms_ms"`
	LookupGlossesMs    int64 `json:"lookup_glosses_ms"`
	ResolveSentencesMs int64 `json:"resolve_sentences_ms"`
	EnrichWordsMs      int64 `json:"enrich_words_ms"`
	TotalMs            int64 `json:"total_ms"`
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
	ParseDurationMs int64            `json:"parse_duration_ms"`
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
	}
}

func registry() map[string]parser {
	return map[string]parser{
		"basic":  dictionaryParser{name: "basic", lookupMode: "basic", analyzer: parserffi.AnalyzeText},
		"custom": dictionaryParser{name: "custom", lookupMode: "custom", analyzer: parserffi.AnalyzeText},
		"omorfi": omorfiParser{analyzer: runExternalOmorfi},
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
	analyzeMs := time.Since(parseStartedAt).Milliseconds()
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
	lookupFormsMs := time.Since(lookupStartedAt).Milliseconds()

	glossLookupStartedAt := time.Now()
	lemmaKeys := resolvedLemmaKeys(formResolutions)
	glosses := db.BatchLookupGlosses(lemmaKeys, lang)
	lookupGlossesMs := time.Since(glossLookupStartedAt).Milliseconds()

	resolveStartedAt := time.Now()
	detailedSentences := resolveDictionarySentences(sentences, formResolutions)
	resolveSentencesMs := time.Since(resolveStartedAt).Milliseconds()

	enrichStartedAt := time.Now()
	words := enrichWords(detailedSentences, glosses)
	enrichWordsMs := time.Since(enrichStartedAt).Milliseconds()
	parseDurationMs := time.Since(parseStartedAt).Milliseconds()
	stats := computeParseStats(detailedSentences, len(uniqueForms), ParseTimings{
		AnalyzeMs:          analyzeMs,
		LookupFormsMs:      lookupFormsMs,
		LookupGlossesMs:    lookupGlossesMs,
		ResolveSentencesMs: resolveSentencesMs,
		EnrichWordsMs:      enrichWordsMs,
		TotalMs:            parseDurationMs,
	})

	return &ParseResult{
		Lang:            lang,
		Parser:          p.name,
		TotalTokens:     countTokens(words),
		ParseDurationMs: parseDurationMs,
		Stats:           stats,
		Words:           words,
		Sentences:       detailedSentences,
	}, nil
}

type omorfiParser struct {
	analyzer baseAnalyzer
}

func (p omorfiParser) Name() string { return "omorfi" }

func (p omorfiParser) Parse(db *store.DB, lang, text string) (*ParseResult, error) {
	if lang != "FI" {
		return nil, fmt.Errorf("omorfi parser only supports FI")
	}
	parseStartedAt := time.Now()
	result, err := p.analyzer(lang, text)
	analyzeMs := time.Since(parseStartedAt).Milliseconds()
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("omorfi parser returned no result")
	}

	sentences := toParsedSentences(result)

	lookupStartedAt := time.Now()
	uniqueForms := collectUniqueForms(sentences)
	directResolutions := db.BatchLookupForms(uniqueForms, lang, "basic")
	customResolutions := db.BatchLookupForms(uniqueForms, lang, "custom")
	lookupFormsMs := time.Since(lookupStartedAt).Milliseconds()
	rules := defaultOmorfiRules()

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
				Source:       "analyzer:omorfi",
				Resolved:     token.StubLemma != "" && token.StubPOS != "" && token.StubPOS != "X",
				Trace:        []string{fmt.Sprintf("analyzer:omorfi lemma=%s pos=%s", token.StubLemma, token.StubPOS)},
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
	resolveSentencesMs := time.Since(resolveStartedAt).Milliseconds()

	glossLookupStartedAt := time.Now()
	lemmaKeys := make([]store.LemmaKey, 0, len(lemmaSet))
	for key := range lemmaSet {
		lemmaKeys = append(lemmaKeys, key)
	}
	glosses := db.BatchLookupGlosses(lemmaKeys, lang)
	lookupGlossesMs := time.Since(glossLookupStartedAt).Milliseconds()

	enrichStartedAt := time.Now()
	words := enrichWords(detailedSentences, glosses)
	enrichWordsMs := time.Since(enrichStartedAt).Milliseconds()
	parseDurationMs := time.Since(parseStartedAt).Milliseconds()
	stats := computeParseStats(detailedSentences, len(uniqueForms), ParseTimings{
		AnalyzeMs:          analyzeMs,
		LookupFormsMs:      lookupFormsMs,
		LookupGlossesMs:    lookupGlossesMs,
		ResolveSentencesMs: resolveSentencesMs,
		EnrichWordsMs:      enrichWordsMs,
		TotalMs:            parseDurationMs,
	})

	return &ParseResult{
		Lang:            lang,
		Parser:          "omorfi",
		TotalTokens:     countTokens(words),
		ParseDurationMs: parseDurationMs,
		Stats:           stats,
		Words:           words,
		Sentences:       detailedSentences,
	}, nil
}

type omorfiRule interface {
	Name() string
	Apply(lang string, token *TokenResult, direct, custom store.FormResolution) bool
}

type omorfiPreferDirectDictRule struct{}

func (omorfiPreferDirectDictRule) Name() string { return "prefer_direct_dict_when_unknown" }

func (omorfiPreferDirectDictRule) Apply(_ string, token *TokenResult, direct, _ store.FormResolution) bool {
	if direct.Lemma == "" {
		return false
	}
	if token.Resolved && token.POS != "X" {
		return false
	}
	token.Trace = append(token.Trace, fmt.Sprintf("rule:direct_dict lemma=%s pos=%s", direct.Lemma, direct.POS))
	token.Lemma = direct.Lemma
	token.POS = direct.POS
	token.Source = "override:direct_dict"
	token.Resolved = true
	return true
}

type omorfiPreferCustomFallbackRule struct{}

func (omorfiPreferCustomFallbackRule) Name() string { return "prefer_custom_fallback_when_unknown" }

func (omorfiPreferCustomFallbackRule) Apply(_ string, token *TokenResult, _, custom store.FormResolution) bool {
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
	token.Source = "override:" + custom.Source
	token.Resolved = true
	return true
}

type omorfiAttachGrammarRule struct{}

func (omorfiAttachGrammarRule) Name() string { return "attach_custom_grammar_label" }

func (omorfiAttachGrammarRule) Apply(_ string, token *TokenResult, _, custom store.FormResolution) bool {
	if token.GrammarLabel != "" || custom.GrammarLabel == "" {
		return false
	}
	if custom.Lemma != "" && token.Lemma != custom.Lemma {
		return false
	}
	if custom.POS != "" && token.POS != custom.POS {
		return false
	}
	token.GrammarLabel = custom.GrammarLabel
	token.Trace = append(token.Trace, fmt.Sprintf("rule:attach_grammar label=%s", custom.GrammarLabel))
	return true
}

func defaultOmorfiRules() []omorfiRule {
	return []omorfiRule{
		omorfiPreferDirectDictRule{},
		omorfiPreferCustomFallbackRule{},
		omorfiAttachGrammarRule{},
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

// findOmorfiAdapter locates the bundled python adapter script in a way that
// works whether the caller's cwd is the repo root, a sub-package directory
// (`go test ./internal/parsecore`), or an installed-binary deployment.
//
// Returns the absolute path to the script and true on success.
func findOmorfiAdapter() (string, bool) {
	const scriptRel = "scripts/omorfi_adapter_example.py"

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
