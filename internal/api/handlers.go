package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"finnestdb/internal/parserffi"
	"finnestdb/internal/store"
)

type API struct {
	store *store.DB
}

func NewAPI(store *store.DB) *API {
	return &API{store: store}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
}

type DashboardResponse struct {
	KnownCount       int           `json:"known_count"`
	DueCount         int           `json:"due_count"`
	NewCapacityToday int           `json:"new_capacity_today"`
	Decks            []DeckSummary `json:"decks"`
}

type DeckSummary struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Lang   string `json:"lang"`
	Known  int    `json:"known"`
	Unique int    `json:"unique"`
	Due    int    `json:"due"`
}

type CreateDeckRequest struct {
	Title string `json:"title"`
	Lang  string `json:"lang"`
	Text  string `json:"text"`
}

type CreateDeckResponse struct {
	DeckID int64 `json:"deck_id"`
}

type CardResponse struct {
	CardID     string     `json:"card_id"`
	Mode       string     `json:"mode"`
	DeckCounts [][]string `json:"deck_counts"`
	Front      CardFront  `json:"front"`
	Back       CardBack   `json:"back"`
}

type CardFront struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Highlight string `json:"highlight,omitempty"`
}

type CardBack struct {
	Lemma    string        `json:"lemma"`
	Meaning  string        `json:"meaning"`
	Grammar  string        `json:"grammar"`
	Examples []CardExample `json:"examples"`
}

type CardExample struct {
	Text       string `json:"text"`
	SourceDeck string `json:"source_deck"`
}

func (a *API) getCurrentUser(r *http.Request) (int64, error) {
	// Mock auth: get user ID from cookie or default to 1
	cookie, err := r.Cookie("user_id")
	if err != nil {
		return 1, nil // Default user for stub
	}
	userID, _ := strconv.ParseInt(cookie.Value, 10, 64)
	if userID == 0 {
		return 1, nil
	}
	return userID, nil
}

func (a *API) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Mock auth: accept any credentials
	user, err := a.store.GetOrCreateUser(req.Email)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "user_id",
		Value:    fmt.Sprintf("%d", user.ID),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400 * 7, // 7 days
	})

	json.NewEncoder(w).Encode(LoginResponse{
		UserID: user.ID,
		Email:  user.Email,
	})
}

func (a *API) HandleMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := a.getCurrentUser(r)

	// Get decks
	decks, err := a.store.GetUserDecks(userID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	deckSummaries := make([]DeckSummary, len(decks))
	for i, deck := range decks {
		deckSummaries[i] = DeckSummary{
			ID:     deck.ID,
			Title:  deck.Title,
			Lang:   deck.Lang,
			Known:  0, // Stub: always 0
			Unique: 0, // Stub: always 0
			Due:    0, // Stub: always 0
		}
	}

	json.NewEncoder(w).Encode(DashboardResponse{
		KnownCount:       1234, // Mock data
		DueCount:         87,   // Mock data
		NewCapacityToday: 12,   // Mock data
		Decks:            deckSummaries,
	})
}

func (a *API) HandleCreateDeck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, _ := a.getCurrentUser(r)

	var req CreateDeckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	if req.Lang != "FI" && req.Lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}

	if len(req.Text) == 0 {
		http.Error(w, "Text is required", http.StatusBadRequest)
		return
	}

	if utf8.RuneCountInString(req.Text) > maxTextChars {
		http.Error(w, fmt.Sprintf("Text exceeds %d character limit", maxTextChars), http.StatusBadRequest)
		return
	}

	// Parse text using Rust parser
	result, err := parserffi.AnalyzeText(req.Lang, req.Text)
	if err != nil {
		http.Error(w, fmt.Sprintf("Parse error: %v", err), http.StatusInternalServerError)
		return
	}

	// Create deck
	deckID, err := a.store.CreateDeck(userID, req.Title, req.Lang)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Convert to CGO-free type; reuse the shared helper.
	sentences := toParsedSentences(result)

	// Batch-resolve forms to (lemma, pos) for occurrence tracking.
	uniqueForms := collectUniqueForms(sentences)
	formResolutions := a.store.BatchLookupForms(uniqueForms, req.Lang, "custom")

	// Store sentences and occurrence records.
	for _, sent := range sentences {
		if sent.Text == "" {
			continue
		}
		sentenceID, err := a.store.CreateSentence(deckID, sent.Text, req.Lang)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating sentence: %v\n", err)
			continue
		}
		for tokenIx, token := range sent.Tokens {
			// Skip punctuation — no occurrence records for ".", ")", etc.
			if token.StubPOS == "PUNCT" {
				continue
			}
			var lemma, pos string
			if resolved, ok := formResolutions[token.Form]; ok {
				lemma, pos = resolved.Lemma, resolved.POS
			} else {
				lemma = strings.ToLower(token.StubLemma)
				if lemma == "" {
					lemma = strings.ToLower(token.Form)
				}
				pos = token.StubPOS
			}
			if lemma == "" {
				continue
			}
			if err := a.store.CreateOccurrence(deckID, sentenceID, tokenIx, lemma, pos); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating occurrence: %v\n", err)
			}
		}
	}

	json.NewEncoder(w).Encode(CreateDeckResponse{DeckID: deckID})
}

func (a *API) HandleReviewNext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return mock card data
	card := CardResponse{
		CardID:     "c_123",
		Mode:       "sentence",
		DeckCounts: [][]string{{"Deck A", "1"}, {"Deck B", "2"}},
		Front: CardFront{
			Type:      "sentence",
			Text:      "Toissapäivänä menin pankkiin.",
			Highlight: "Toissapäivänä",
		},
		Back: CardBack{
			Lemma:   "toissapäivä",
			Meaning: "the day before yesterday",
			Grammar: "Essive singular (-nä)",
			Examples: []CardExample{
				{
					Text:       "Toissapäivänä menin pankkiin.",
					SourceDeck: "Everyday FI",
				},
			},
		},
	}

	json.NewEncoder(w).Encode(card)
}

func (a *API) HandleReviewAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Stub: just return success
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (a *API) HandleCardIgnore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Stub: just return success
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (a *API) HandleCardKnown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Stub: just return success
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

const maxTextChars = 300_000

type ParseRequest struct {
	Lang   string `json:"lang"`
	Text   string `json:"text"`
	Parser string `json:"parser"` // "basic" or "custom"; defaults to "basic"
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

type ParseResponse struct {
	Lang            string      `json:"lang"`
	TotalTokens     int         `json:"total_tokens"`
	ParseDurationMs int64       `json:"parse_duration_ms"`
	Words           []WordEntry `json:"words"`
}

func (a *API) HandleParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ParseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Lang != "FI" && req.Lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}

	if len(req.Text) == 0 {
		http.Error(w, "Text is required", http.StatusBadRequest)
		return
	}

	if utf8.RuneCountInString(req.Text) > maxTextChars {
		http.Error(w, fmt.Sprintf("Text exceeds %d character limit", maxTextChars), http.StatusBadRequest)
		return
	}

	parseStartedAt := time.Now()
	result, err := parserffi.AnalyzeText(req.Lang, req.Text)
	parseDurationMs := time.Since(parseStartedAt).Milliseconds()
	if err != nil {
		http.Error(w, fmt.Sprintf("Parse error: %v", err), http.StatusInternalServerError)
		return
	}
	if result == nil {
		http.Error(w, "Parser returned no result", http.StatusInternalServerError)
		return
	}

	// Step 1: reconstruct sentence texts and convert to CGO-free type.
	sentences := toParsedSentences(result)

	// Step 2: collect unique surface forms for batch DB lookup.
	uniqueForms := collectUniqueForms(sentences)

	// Step 3: batch-resolve forms → (lemma, pos) via dictionary.
	parserMode := req.Parser
	if parserMode == "" {
		parserMode = "basic"
	}
	formResolutions := a.store.BatchLookupForms(uniqueForms, req.Lang, parserMode)

	// Step 4: batch-fetch glosses for resolved lemmas.
	lemmaKeys := resolvedLemmaKeys(formResolutions)
	glosses := a.store.BatchLookupGlosses(lemmaKeys, req.Lang)

	// Step 5: enrich — aggregate counts, forms, glosses, example sentences.
	words := enrichWords(sentences, formResolutions, glosses)

	// Total token count (sum over all words).
	totalTokens := 0
	for _, w := range words {
		totalTokens += w.Count
	}

	json.NewEncoder(w).Encode(ParseResponse{
		Lang:            req.Lang,
		TotalTokens:     totalTokens,
		ParseDurationMs: parseDurationMs,
		Words:           words,
	})
}

// toParsedSentences converts parserffi output to the CGO-free ParsedSentence
// type used by enrichWords.
//
// Sentence text is reconstructed with smart punctuation joining:
//   - No space before closing punctuation: . , ; : ! ? ) ] }
//   - No space after opening punctuation: ( [ {
//
// This produces "kauppaan)." instead of "kauppaan ) ."
func toParsedSentences(result *parserffi.AnalysisResult) []ParsedSentence {
	sentences := make([]ParsedSentence, 0, len(result.Sentences))
	for _, s := range result.Sentences {
		tokens := make([]ParsedToken, 0, len(s.Tokens))
		var textBuilder strings.Builder
		prevWasOpen := false // true if previous token was opening punctuation

		for i, t := range s.Tokens {
			if t.Form == "" {
				continue
			}
			tokens = append(tokens, ParsedToken{
				Form:      t.Form,
				StubLemma: t.Lemma,
				StubPOS:   t.POS,
			})

			// Smart spacing: suppress space around punctuation.
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
			sentences = append(sentences, ParsedSentence{Tokens: tokens, Text: text})
		}
	}
	return sentences
}

// collectUniqueForms returns the deduplicated set of surface forms across all
// sentences, excluding PUNCT tokens (no point looking up "." in the dictionary).
func collectUniqueForms(sentences []ParsedSentence) []string {
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

// resolvedLemmaKeys extracts unique LemmaKey values from a form resolution map.
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

func (a *API) SetupRoutes(mux *http.ServeMux) {
	// Auth routes
	mux.HandleFunc("/api/auth/login", a.HandleLogin)

	// Dashboard
	mux.HandleFunc("/api/me", a.HandleMe)

	// Parse (word list view)
	mux.HandleFunc("/api/parse", a.HandleParse)

	// Decks
	mux.HandleFunc("/api/decks", a.HandleCreateDeck)

	// Review
	mux.HandleFunc("/api/review/next", a.HandleReviewNext)
	mux.HandleFunc("/api/review/answer", a.HandleReviewAnswer)
	mux.HandleFunc("/api/card/ignore", a.HandleCardIgnore)
	mux.HandleFunc("/api/card/known", a.HandleCardKnown)
}
