package parsecore

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"finnestdb/internal/parserffi"
	"finnestdb/internal/store"
)

const MaxTextChars = 300_000

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

type ParseResult struct {
	Lang            string           `json:"lang"`
	Parser          string           `json:"parser"`
	TotalTokens     int              `json:"total_tokens"`
	ParseDurationMs int64            `json:"parse_duration_ms"`
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
	}
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
	analyzer   baseAnalyzer
}

func (p dictionaryParser) Name() string { return p.name }

func (p dictionaryParser) Parse(db *store.DB, lang, text string) (*ParseResult, error) {
	parseStartedAt := time.Now()
	result, err := p.analyzer(lang, text)
	parseDurationMs := time.Since(parseStartedAt).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("parser returned no result")
	}

	sentences := toParsedSentences(result)
	uniqueForms := collectUniqueForms(sentences)
	formResolutions := db.BatchLookupForms(uniqueForms, lang, p.lookupMode)
	lemmaKeys := resolvedLemmaKeys(formResolutions)
	glosses := db.BatchLookupGlosses(lemmaKeys, lang)
	detailedSentences := resolveDictionarySentences(sentences, formResolutions)
	words := enrichWords(detailedSentences, glosses)

	return &ParseResult{
		Lang:            lang,
		Parser:          p.name,
		TotalTokens:     countTokens(words),
		ParseDurationMs: parseDurationMs,
		Words:           words,
		Sentences:       detailedSentences,
	}, nil
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
