package parsecore

import (
	"fmt"
	"strings"
	"time"

	"finnestdb/internal/store"
)

// ExternalAnalyzerConfig describes an eval/lab analyzer whose raw JSON output
// already matches the parser FFI shape. The production registry does not add
// these analyzers by default; eval tooling opts into them explicitly.
type ExternalAnalyzerConfig struct {
	Name    string
	Lang    string
	Source  string
	Analyze AnalyzerFunc
	Rules   []ExternalAnalyzerRule
}

// ExternalAnalyzerRule can override or enrich tokens emitted by an external
// analyzer using dictionary evidence.
type ExternalAnalyzerRule interface {
	Name() string
	Apply(lang string, token *TokenResult, direct, custom store.FormResolution) bool
}

func AnalyzeWithExternalAnalyzer(db *store.DB, lang, text string, cfg ExternalAnalyzerConfig) (*ParseResult, error) {
	if err := ValidateInput(lang, text); err != nil {
		return nil, err
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("external analyzer name is required")
	}
	if cfg.Lang != "" && lang != cfg.Lang {
		return nil, fmt.Errorf("%s parser only supports %s", cfg.Name, cfg.Lang)
	}
	if cfg.Analyze == nil {
		return nil, fmt.Errorf("%s parser has no analyzer", cfg.Name)
	}
	source := cfg.Source
	if source == "" {
		source = "analyzer:" + cfg.Name
	}
	parseStartedAt := time.Now()
	result, err := cfg.Analyze(lang, text)
	analyzeNs := time.Since(parseStartedAt).Nanoseconds()
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%s parser returned no result", cfg.Name)
	}

	sentences := toParsedSentences(result)

	lookupStartedAt := time.Now()
	uniqueForms := collectUniqueForms(sentences)
	directResolutions := db.BatchLookupForms(uniqueForms, lang, "basic")
	customResolutions := db.BatchLookupForms(uniqueForms, lang, "custom")
	lookupFormsNs := time.Since(lookupStartedAt).Nanoseconds()

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
				Source:       source,
				Resolved:     token.StubLemma != "" && token.StubPOS != "" && token.StubPOS != "X",
				Trace:        []string{fmt.Sprintf("%s lemma=%s pos=%s", source, token.StubLemma, token.StubPOS)},
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

			for _, rule := range cfg.Rules {
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
		Parser:          cfg.Name,
		TotalTokens:     countTokens(words),
		ParseDurationNs: parseDurationNs,
		Stats:           stats,
		Words:           words,
		Sentences:       detailedSentences,
	}, nil
}
