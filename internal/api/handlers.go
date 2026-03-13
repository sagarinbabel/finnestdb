package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

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
	KnownCount        int              `json:"known_count"`
	DueCount          int              `json:"due_count"`
	NewCapacityToday  int              `json:"new_capacity_today"`
	Decks             []DeckSummary    `json:"decks"`
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
	CardID     string   `json:"card_id"`
	Mode       string   `json:"mode"`
	DeckCounts [][]string `json:"deck_counts"`
	Front      CardFront `json:"front"`
	Back       CardBack  `json:"back"`
}

type CardFront struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Highlight string `json:"highlight,omitempty"`
}

type CardBack struct {
	Lemma    string       `json:"lemma"`
	Meaning  string       `json:"meaning"`
	Grammar  string       `json:"grammar"`
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

	// Store sentences
	for _, sentence := range result.Sentences {
		// Reconstruct sentence text from tokens
		sentenceText := ""
		for i, token := range sentence.Tokens {
			if i > 0 {
				sentenceText += " "
			}
			sentenceText += token.Form
		}
		if sentenceText != "" {
			_, err := a.store.CreateSentence(deckID, sentenceText, req.Lang)
			if err != nil {
				// Log error but continue
				fmt.Fprintf(os.Stderr, "Error creating sentence: %v\n", err)
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

const maxTextBytes = 10_000

type ParseRequest struct {
	Lang string `json:"lang"`
	Text string `json:"text"`
}

type WordEntry struct {
	Lemma string   `json:"lemma"`
	POS   string   `json:"pos"`
	Forms []string `json:"forms"`
	Count int      `json:"count"`
}

type ParseResponse struct {
	Lang        string      `json:"lang"`
	TotalTokens int         `json:"total_tokens"`
	Words       []WordEntry `json:"words"`
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

	if len(req.Text) > maxTextBytes {
		http.Error(w, fmt.Sprintf("Text exceeds %d character limit", maxTextBytes), http.StatusBadRequest)
		return
	}

	result, err := parserffi.AnalyzeText(req.Lang, req.Text)
	if err != nil {
		http.Error(w, fmt.Sprintf("Parse error: %v", err), http.StatusInternalServerError)
		return
	}
	if result == nil {
		http.Error(w, "Parser returned no result", http.StatusInternalServerError)
		return
	}

	// Aggregate tokens by (lemma, pos)
	type key struct{ lemma, pos string }
	counts := make(map[key]int)
	formSets := make(map[key]map[string]struct{})
	totalTokens := 0

	for _, sentence := range result.Sentences {
		for _, token := range sentence.Tokens {
			if strings.TrimSpace(token.Lemma) == "" {
				continue
			}
			k := key{lemma: strings.ToLower(token.Lemma), pos: token.POS}
			counts[k]++
			totalTokens++
			if formSets[k] == nil {
				formSets[k] = make(map[string]struct{})
			}
			formSets[k][token.Form] = struct{}{}
		}
	}

	words := make([]WordEntry, 0, len(counts))
	for k, count := range counts {
		forms := make([]string, 0, len(formSets[k]))
		for f := range formSets[k] {
			forms = append(forms, f)
		}
		sort.Strings(forms)
		words = append(words, WordEntry{
			Lemma: k.lemma,
			POS:   k.pos,
			Forms: forms,
			Count: count,
		})
	}

	// Sort by count descending, then lemma alphabetically for stable ordering
	sort.Slice(words, func(i, j int) bool {
		if words[i].Count != words[j].Count {
			return words[i].Count > words[j].Count
		}
		return words[i].Lemma < words[j].Lemma
	})

	json.NewEncoder(w).Encode(ParseResponse{
		Lang:        req.Lang,
		TotalTokens: totalTokens,
		Words:       words,
	})
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

