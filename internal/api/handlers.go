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

type ParseFeedbackRequest struct {
	ParseID              int64  `json:"parse_id"`
	Lang                 string `json:"lang"`
	Parser               string `json:"parser"`
	Surface              string `json:"surface"`
	Occurrence           int    `json:"occurrence"`
	OriginalLemma        string `json:"original_lemma"`
	OriginalPOS          string `json:"original_pos"`
	OriginalGrammarLabel string `json:"original_grammar_label"`
	ProposedLemma        string `json:"proposed_lemma"`
	ProposedPOS          string `json:"proposed_pos"`
	ProposedGrammarLabel string `json:"proposed_grammar_label"`
	Note                 string `json:"note"`
}

type ParseFeedbackReviewRequest struct {
	Status     string `json:"status"`
	ReviewNote string `json:"review_note"`
}

type ParseFeedbackResponse struct {
	FeedbackID int64  `json:"feedback_id"`
	Status     string `json:"status"`
}

type ParseFeedbackListResponse struct {
	Feedback []store.ParseFeedback `json:"feedback"`
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

	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if auth == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	parsed, err := a.analyze(a.store, req.Lang, req.Text, "custom")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create deck
	deckID, err := a.store.CreateDeck(auth.UserID, req.Title, req.Lang)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Store sentences and occurrence records.
	cardKeys := make(map[string]struct{})
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
			if pos == "" {
				continue
			}
			key := req.Lang + "\x00" + lemma + "\x00" + pos
			cardKeys[key] = struct{}{}
		}
	}

	for key := range cardKeys {
		parts := strings.Split(key, "\x00")
		if len(parts) != 3 {
			continue
		}
		isKnown, err := a.store.IsKnownOrIgnored(auth.UserID, parts[0], parts[1], parts[2])
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if isKnown {
			continue
		}
		if _, err := a.store.EnsureCard(auth.UserID, parts[0], parts[1], parts[2]); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
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
	ParseID         int64                `json:"parse_id"`
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

	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	var userID *int64
	if auth != nil {
		userID = &auth.UserID
	}
	parseID, err := a.store.CreateParseSession(userID, parsed.Lang, parsed.Parser, req.Text, parsed.TotalTokens, len(parsed.Words))
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, ParseResponse{
		ParseID:         parseID,
		Lang:            parsed.Lang,
		TotalTokens:     parsed.TotalTokens,
		ParseDurationMs: parsed.ParseDurationMs,
		Stats:           parsed.Stats,
		Words:           parsed.Words,
	})
}

func (a *API) HandleParseFeedback(w http.ResponseWriter, r *http.Request) {
	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if auth == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ParseFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.ParseID == 0 {
		http.Error(w, "Parse ID is required", http.StatusBadRequest)
		return
	}
	if req.Lang != "FI" && req.Lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}
	if req.Parser == "" || req.Surface == "" || req.ProposedLemma == "" || req.ProposedPOS == "" {
		http.Error(w, "Parser, surface, proposed lemma, and proposed POS are required", http.StatusBadRequest)
		return
	}

	exists, err := a.store.ParseSessionExists(req.ParseID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Parse session not found", http.StatusBadRequest)
		return
	}

	feedbackID, err := a.store.CreateParseFeedback(store.ParseFeedback{
		ParseSessionID:       req.ParseID,
		UserID:               auth.UserID,
		Lang:                 req.Lang,
		Parser:               req.Parser,
		Surface:              strings.TrimSpace(req.Surface),
		Occurrence:           req.Occurrence,
		OriginalLemma:        strings.TrimSpace(req.OriginalLemma),
		OriginalPOS:          strings.TrimSpace(req.OriginalPOS),
		OriginalGrammarLabel: strings.TrimSpace(req.OriginalGrammarLabel),
		ProposedLemma:        strings.TrimSpace(req.ProposedLemma),
		ProposedPOS:          strings.TrimSpace(req.ProposedPOS),
		ProposedGrammarLabel: strings.TrimSpace(req.ProposedGrammarLabel),
		Note:                 strings.TrimSpace(req.Note),
	})
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, ParseFeedbackResponse{
		FeedbackID: feedbackID,
		Status:     "submitted",
	})
}

func (a *API) HandleAdminParseFeedback(w http.ResponseWriter, r *http.Request) {
	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if auth == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	if !auth.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodGet:
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		feedback, err := a.store.ListParseFeedback(status)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, ParseFeedbackListResponse{Feedback: feedback})
	case http.MethodPatch:
		feedbackIDStr := strings.TrimSpace(r.URL.Query().Get("id"))
		feedbackID, _ := strconv.ParseInt(feedbackIDStr, 10, 64)
		if feedbackID == 0 {
			http.Error(w, "Feedback ID is required", http.StatusBadRequest)
			return
		}
		var req ParseFeedbackReviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.Status != "accepted" && req.Status != "rejected" && req.Status != "needs_follow_up" {
			http.Error(w, "Status must be accepted, rejected, or needs_follow_up", http.StatusBadRequest)
			return
		}
		if err := a.store.ReviewParseFeedback(feedbackID, auth.UserID, req.Status, strings.TrimSpace(req.ReviewNote)); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": req.Status})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
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
	mux.HandleFunc("/api/parse/feedback", a.HandleParseFeedback)
	mux.HandleFunc("/api/admin/parse-feedback", a.HandleAdminParseFeedback)

	// Decks
	mux.HandleFunc("/api/decks", a.HandleCreateDeck)
	mux.HandleFunc("/api/known-words", a.HandleKnownWords)

	// Review
	mux.HandleFunc("/api/review/next", a.HandleReviewNext)
	mux.HandleFunc("/api/review/answer", a.HandleReviewAnswer)
	mux.HandleFunc("/api/card/ignore", a.HandleCardIgnore)
	mux.HandleFunc("/api/card/known", a.HandleCardKnown)
}
