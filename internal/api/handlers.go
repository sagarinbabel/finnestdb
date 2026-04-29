package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"finnestdb/internal/parsecore"
	"finnestdb/internal/store"
)

type API struct {
	store   *store.DB
	analyze func(*store.DB, string, string, string) (*parsecore.ParseResult, error)
}

func NewAPI(store *store.DB) *API {
	return &API{
		store:   store,
		analyze: parsecore.Analyze,
	}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Authenticated bool         `json:"authenticated"`
	User          *SessionUser `json:"user"`
}

type DashboardData struct {
	KnownCount       int           `json:"known_count"`
	DueCount         int           `json:"due_count"`
	NewCapacityToday int           `json:"new_capacity_today"`
	Decks            []DeckSummary `json:"decks"`
}

type SessionUser struct {
	ID      int64  `json:"id"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

type MeResponse struct {
	Authenticated bool           `json:"authenticated"`
	User          *SessionUser   `json:"user"`
	Dashboard     *DashboardData `json:"dashboard,omitempty"`
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

type KnownWordsRequest struct {
	Lang  string   `json:"lang"`
	Words []string `json:"words"`
}

type KnownWordsResponse struct {
	Imported   []store.KnownLemma `json:"imported"`
	Unresolved []string           `json:"unresolved"`
}

type KnownWordsListResponse struct {
	KnownWords []store.KnownLemma `json:"known_words"`
}

type cardKey struct {
	Lang  string
	Lemma string
	POS   string
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

type AuthContext struct {
	UserID  int64
	Email   string
	IsAdmin bool
}

func (a *API) getCurrentUser(r *http.Request) (*AuthContext, error) {
	cookie, err := r.Cookie("user_id")
	if err != nil {
		return nil, nil
	}
	userID, _ := strconv.ParseInt(cookie.Value, 10, 64)
	if userID == 0 {
		return nil, nil
	}

	user, err := a.store.GetUserByID(userID)
	if err != nil {
		return nil, err
	}

	return &AuthContext{
		UserID:  user.ID,
		Email:   user.Email,
		IsAdmin: user.IsAdmin,
	}, nil
}

func (a *API) requireAuth(next func(http.ResponseWriter, *http.Request, *AuthContext)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth, err := a.getCurrentUser(r)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if auth == nil {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		next(w, r, auth)
	}
}

func (a *API) requireAdmin(next func(http.ResponseWriter, *http.Request, *AuthContext)) http.HandlerFunc {
	return a.requireAuth(func(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
		if !auth.IsAdmin {
			http.Error(w, "Admin access required", http.StatusForbidden)
			return
		}
		next(w, r, auth)
	})
}

func sessionUserFromAuth(auth *AuthContext) *SessionUser {
	if auth == nil {
		return nil
	}
	return &SessionUser{
		ID:      auth.UserID,
		Email:   auth.Email,
		IsAdmin: auth.IsAdmin,
	}
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

	writeJSON(w, http.StatusOK, LoginResponse{
		Authenticated: true,
		User: &SessionUser{
			ID:      user.ID,
			Email:   user.Email,
			IsAdmin: user.IsAdmin,
		},
	})
}

func (a *API) HandleMe(w http.ResponseWriter, r *http.Request) {
	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if auth == nil {
		writeJSON(w, http.StatusOK, MeResponse{
			Authenticated: false,
			User:          nil,
		})
		return
	}

	// Get decks
	decks, err := a.store.GetUserDecks(auth.UserID)
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

	writeJSON(w, http.StatusOK, MeResponse{
		Authenticated: true,
		User:          sessionUserFromAuth(auth),
		Dashboard: &DashboardData{
			KnownCount:       1234, // Mock data
			DueCount:         87,   // Mock data
			NewCapacityToday: 12,   // Mock data
			Decks:            deckSummaries,
		},
	})
}

func (a *API) HandleCreateDeck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if auth == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

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

	parsed, err := a.analyze(a.store, req.Lang, req.Text, "custom")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cardKeys := make(map[cardKey]struct{})
	for _, sent := range parsed.Sentences {
		for _, token := range sent.Tokens {
			if token.POS == "PUNCT" {
				continue
			}
			lemma := token.Lemma
			if lemma == "" {
				lemma = strings.ToLower(token.Form)
			}
			pos := token.POS
			if lemma == "" {
				continue
			}
			if pos == "" {
				continue
			}
			cardKeys[cardKey{Lang: req.Lang, Lemma: lemma, POS: pos}] = struct{}{}
		}
	}

	for key := range cardKeys {
		isKnown, err := a.store.IsKnownOrIgnored(auth.UserID, key.Lang, key.Lemma, key.POS)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if isKnown {
			continue
		}
		if _, err := a.store.EnsureCard(auth.UserID, key.Lang, key.Lemma, key.POS); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	}

	// Create deck only after global card seeding succeeds so failed seeding
	// cannot leave behind a half-populated deck.
	deckID, err := a.store.CreateDeck(auth.UserID, req.Title, req.Lang)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Store sentences and occurrence records.
	for _, sent := range parsed.Sentences {
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
			if token.POS == "PUNCT" {
				continue
			}
			lemma := token.Lemma
			if lemma == "" {
				lemma = strings.ToLower(token.Form)
			}
			pos := token.POS
			if lemma == "" {
				continue
			}
			if err := a.store.CreateOccurrence(deckID, sentenceID, tokenIx, lemma, pos); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating occurrence: %v\n", err)
			}
		}
	}

	writeJSON(w, http.StatusOK, CreateDeckResponse{DeckID: deckID})
}

func (a *API) HandleKnownWords(w http.ResponseWriter, r *http.Request) {
	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if auth == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.handleKnownWordsList(w, r, auth)
	case http.MethodPost:
		a.handleKnownWordsImport(w, r, auth)
	case http.MethodDelete:
		a.handleKnownWordsDelete(w, r, auth)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleKnownWordsImport(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	var req KnownWordsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.Lang != "FI" && req.Lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}
	if len(req.Words) == 0 {
		http.Error(w, "Words are required", http.StatusBadRequest)
		return
	}

	imported, unresolved, err := a.store.ImportKnownWords(auth.UserID, req.Lang, req.Words)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, KnownWordsResponse{
		Imported:   imported,
		Unresolved: unresolved,
	})
}

func (a *API) handleKnownWordsList(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	lang := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("lang")))
	if lang != "FI" && lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}

	knownWords, err := a.store.ListKnownWords(auth.UserID, lang)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, KnownWordsListResponse{KnownWords: knownWords})
}

func (a *API) handleKnownWordsDelete(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	lang := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("lang")))
	lemma := strings.TrimSpace(r.URL.Query().Get("lemma"))
	pos := strings.TrimSpace(r.URL.Query().Get("pos"))
	if lang != "FI" && lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}
	if lemma == "" || pos == "" {
		http.Error(w, "Lemma and POS are required", http.StatusBadRequest)
		return
	}

	if err := a.store.DeleteKnownWord(auth.UserID, lang, lemma, pos); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

	writeJSON(w, http.StatusOK, card)
}

func (a *API) HandleReviewAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Stub: just return success
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) HandleCardIgnore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Stub: just return success
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) HandleCardKnown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Stub: just return success
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type ParseRequest struct {
	Lang   string `json:"lang"`
	Text   string `json:"text"`
	Parser string `json:"parser"` // parser name; defaults to "basic"
}

type WordEntry = parsecore.WordEntry

type ParseResponse struct {
	Lang            string               `json:"lang"`
	TotalTokens     int                  `json:"total_tokens"`
	ParseDurationMs int64                `json:"parse_duration_ms"`
	Stats           parsecore.ParseStats `json:"stats"`
	Words           []WordEntry          `json:"words"`
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
	parsed, err := a.analyze(a.store, req.Lang, req.Text, req.Parser)
	if err != nil {
		status := http.StatusInternalServerError
		switch err.Error() {
		case "language must be FI or ET", "text is required":
			status = http.StatusBadRequest
		default:
			if len(err.Error()) >= 13 && err.Error()[:13] == "text exceeds " {
				status = http.StatusBadRequest
			}
			if len(err.Error()) >= 19 && err.Error()[:19] == "unsupported parser " {
				status = http.StatusBadRequest
			}
		}
		http.Error(w, err.Error(), status)
		return
	}

	writeJSON(w, http.StatusOK, ParseResponse{
		Lang:            parsed.Lang,
		TotalTokens:     parsed.TotalTokens,
		ParseDurationMs: parsed.ParseDurationMs,
		Stats:           parsed.Stats,
		Words:           parsed.Words,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
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
	mux.HandleFunc("/api/known-words", a.HandleKnownWords)

	// Review
	mux.HandleFunc("/api/review/next", a.HandleReviewNext)
	mux.HandleFunc("/api/review/answer", a.HandleReviewAnswer)
	mux.HandleFunc("/api/card/ignore", a.HandleCardIgnore)
	mux.HandleFunc("/api/card/known", a.HandleCardKnown)
}
