package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type ekilexClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type ekilexWordRef struct {
	WordID int
	Value  string
}

type ekilexWordDetails struct {
	WordClass string `json:"wordClass"`
	Paradigms []struct {
		Forms []struct {
			Value string `json:"value"`
			Mode  string `json:"mode"`
		} `json:"forms"`
	} `json:"paradigms"`
	Lexemes []struct {
		Words       []string `json:"words"`
		WordLang    string   `json:"wordLang"`
		DatasetCode string   `json:"datasetCode"`
		POS         []struct {
			Code  string `json:"code"`
			Value string `json:"value"`
		} `json:"pos"`
		Definitions []struct {
			Value string `json:"value"`
			Lang  string `json:"lang"`
		} `json:"definitions"`
		MeaningWords []struct {
			Value    string `json:"value"`
			Language string `json:"language"`
		} `json:"meaningWords"`
	} `json:"lexemes"`
}

type ekilexEntry struct {
	Lemma string
	POS   string
	Gloss string
	Forms []string
}

func newEkilexClient(baseURL, apiKey string) (*ekilexClient, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("EKILEX_API_KEY is required; create an API key in your Ekilex user profile and export it before running the import")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("ekilex base URL is empty")
	}
	return &ekilexClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		http: &http.Client{
			Timeout: 45 * time.Second,
		},
	}, nil
}

func (c *ekilexClient) getJSON(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("ekilex-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *ekilexClient) searchWord(word string, datasets []string) ([]ekilexWordRef, error) {
	path := "/api/word/search/" + url.PathEscape(word)
	if len(datasets) > 0 {
		path += "/" + url.PathEscape(strings.Join(datasets, ","))
	}
	var raw any
	if err := c.getJSON(path, &raw); err != nil {
		return nil, err
	}
	return collectEkilexWordRefs(raw), nil
}

func (c *ekilexClient) publicWords(dataset string) ([]ekilexWordRef, error) {
	var raw any
	if err := c.getJSON("/api/public_word/"+url.PathEscape(dataset), &raw); err != nil {
		return nil, err
	}
	return collectEkilexWordRefs(raw), nil
}

func (c *ekilexClient) wordDetails(wordID int, datasets []string) (*ekilexWordDetails, error) {
	path := fmt.Sprintf("/api/word/details/%d", wordID)
	if len(datasets) > 0 {
		path += "/" + url.PathEscape(strings.Join(datasets, ","))
	}
	var details ekilexWordDetails
	if err := c.getJSON(path, &details); err != nil {
		return nil, err
	}
	return &details, nil
}

func (c *ekilexClient) paradigmForms(wordID int) ([]string, error) {
	var raw any
	if err := c.getJSON(fmt.Sprintf("/api/paradigm/details/%d", wordID), &raw); err != nil {
		return nil, err
	}
	return collectEkilexFormValues(raw), nil
}

func importEkilex(db *sql.DB, client *ekilexClient, dbLang string, datasets, words []string, limit int, source importSourceConfig) (int, error) {
	refs, err := ekilexImportRefs(client, datasets, words)
	if err != nil {
		return 0, err
	}
	if limit > 0 && len(refs) > limit {
		refs = refs[:limit]
	}
	if len(refs) == 0 {
		return 0, errors.New("Ekilex returned no words to import")
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmtLemma, err := tx.Prepare(
		`INSERT INTO lemmas (lemma, pos, gloss, lang, source, source_priority)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(lemma, pos, lang) DO UPDATE SET
		 gloss = excluded.gloss,
		 source = excluded.source,
		 source_priority = excluded.source_priority
		 WHERE lemmas.source_priority <= excluded.source_priority`,
	)
	if err != nil {
		return 0, err
	}
	defer stmtLemma.Close()

	stmtForm, err := tx.Prepare(
		`INSERT INTO forms (form, lemma, pos, lang, source, source_priority)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(form, lang) DO UPDATE SET
		 lemma = excluded.lemma,
		 pos = excluded.pos,
		 source = excluded.source,
		 source_priority = excluded.source_priority
		 WHERE forms.source_priority <= excluded.source_priority`,
	)
	if err != nil {
		return 0, err
	}
	defer stmtForm.Close()

	processed := 0
	for _, ref := range refs {
		details, err := client.wordDetails(ref.WordID, datasets)
		if err != nil {
			return processed, err
		}
		paradigmForms, err := client.paradigmForms(ref.WordID)
		if err != nil {
			return processed, err
		}
		for _, entry := range ekilexEntriesFromDetails(details, ref.Value) {
			entry.Forms = uniqueStrings(append(entry.Forms, paradigmForms...))
			if _, err := stmtLemma.Exec(entry.Lemma, entry.POS, entry.Gloss, dbLang, source.Name, source.Priority); err != nil {
				return processed, err
			}
			forms := append([]string{entry.Lemma}, entry.Forms...)
			for _, form := range uniqueStrings(forms) {
				if _, err := stmtForm.Exec(form, entry.Lemma, entry.POS, dbLang, source.Name, source.Priority); err != nil {
					return processed, err
				}
			}
		}
		processed++
		if processed%100 == 0 {
			fmt.Printf("\r  %d Ekilex words processed...", processed)
		}
	}
	if processed >= 100 {
		fmt.Println()
	}
	if err := tx.Commit(); err != nil {
		return processed, err
	}
	return processed, nil
}

func ekilexImportRefs(client *ekilexClient, datasets, words []string) ([]ekilexWordRef, error) {
	seen := map[int]ekilexWordRef{}
	if len(words) > 0 {
		for _, word := range words {
			refs, err := client.searchWord(word, datasets)
			if err != nil {
				return nil, err
			}
			for _, ref := range refs {
				seen[ref.WordID] = ref
			}
		}
	} else {
		for _, dataset := range datasets {
			refs, err := client.publicWords(dataset)
			if err != nil {
				return nil, err
			}
			for _, ref := range refs {
				seen[ref.WordID] = ref
			}
		}
	}
	out := make([]ekilexWordRef, 0, len(seen))
	for _, ref := range seen {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WordID < out[j].WordID })
	return out, nil
}

func ekilexEntriesFromDetails(details *ekilexWordDetails, fallbackLemma string) []ekilexEntry {
	forms := ekilexForms(details)
	entries := make([]ekilexEntry, 0, len(details.Lexemes))
	for _, lexeme := range details.Lexemes {
		if lexeme.WordLang != "" && lexeme.WordLang != "est" {
			continue
		}
		lemma := firstNonEmpty(lexeme.Words...)
		if lemma == "" {
			lemma = fallbackLemma
		}
		if lemma == "" {
			continue
		}
		pos := ekilexPOS(lexeme.POS, details.WordClass)
		if pos == "" {
			pos = "X"
		}
		entries = append(entries, ekilexEntry{
			Lemma: normalizeDictText(lemma),
			POS:   pos,
			Gloss: ekilexGloss(lexeme.Definitions, lexeme.MeaningWords),
			Forms: forms,
		})
	}
	if len(entries) == 0 && len(details.Lexemes) == 0 && fallbackLemma != "" {
		entries = append(entries, ekilexEntry{
			Lemma: normalizeDictText(fallbackLemma),
			POS:   normalizePos(details.WordClass),
			Forms: forms,
		})
	}
	return entries
}

func ekilexForms(details *ekilexWordDetails) []string {
	var forms []string
	for _, paradigm := range details.Paradigms {
		for _, form := range paradigm.Forms {
			value := normalizeDictText(form.Value)
			if value != "" && value != "-" {
				forms = append(forms, value)
			}
		}
	}
	return uniqueStrings(forms)
}

func ekilexPOS(pos []struct {
	Code  string `json:"code"`
	Value string `json:"value"`
}, wordClass string) string {
	for _, item := range pos {
		if mapped := mapEkilexPOS(item.Code); mapped != "" {
			return mapped
		}
	}
	return mapEkilexPOS(wordClass)
}

func mapEkilexPOS(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "s", "subst", "substantiiv", "noun":
		return "NOUN"
	case "v", "verb":
		return "VERB"
	case "adj", "adjektiiv":
		return "ADJ"
	case "adv", "adverb":
		return "ADV"
	case "pron", "pronoomen":
		return "PRON"
	case "num", "number", "arvsõna":
		return "NUM"
	case "konj", "conj":
		return "CCONJ"
	case "interj", "intj":
		return "INTJ"
	case "prep", "postp", "adp":
		return "ADP"
	case "noomen":
		return "NOUN"
	default:
		return normalizePos(raw)
	}
}

func ekilexGloss(definitions []struct {
	Value string `json:"value"`
	Lang  string `json:"lang"`
}, meaningWords []struct {
	Value    string `json:"value"`
	Language string `json:"language"`
}) string {
	var parts []string
	for _, definition := range definitions {
		if value := strings.TrimSpace(definition.Value); value != "" {
			parts = append(parts, value)
		}
	}
	for _, word := range meaningWords {
		if word.Language == "" || word.Language == "est" {
			continue
		}
		if value := strings.TrimSpace(word.Value); value != "" {
			parts = append(parts, word.Language+": "+value)
		}
	}
	return strings.Join(uniqueStrings(parts), "; ")
}

func collectEkilexWordRefs(raw any) []ekilexWordRef {
	seen := map[int]ekilexWordRef{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				walk(item)
			}
		case map[string]any:
			id := intFromAny(x["wordId"])
			if id != 0 {
				seen[id] = ekilexWordRef{WordID: id, Value: stringFromAny(x["value"])}
			}
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(raw)
	out := make([]ekilexWordRef, 0, len(seen))
	for _, ref := range seen {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WordID < out[j].WordID })
	return out
}

func collectEkilexFormValues(raw any) []string {
	var forms []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				walk(item)
			}
		case map[string]any:
			if formItems, ok := x["forms"].([]any); ok {
				for _, item := range formItems {
					if form, ok := item.(map[string]any); ok {
						if value := normalizeDictText(stringFromAny(form["value"])); value != "" && value != "-" {
							forms = append(forms, value)
						}
					}
				}
			}
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(raw)
	return uniqueStrings(forms)
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return 0
}

func stringFromAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeDictText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
