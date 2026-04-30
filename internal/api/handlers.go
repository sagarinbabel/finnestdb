package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
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

type DeckListResponse struct {
	Decks []DeckSummary `json:"decks"`
}

type UpdateDeckRequest struct {
	Title string `json:"title"`
}

type ReviewAnswerRequest struct {
	CardID int64  `json:"card_id"`
	Rating string `json:"rating"`
}

type ReviewCardMutationRequest struct {
	CardID int64 `json:"card_id"`
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
		if err == sql.ErrNoRows {
			return nil, nil
		}
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

func (a *API) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Expire the session cookie regardless of whether one is present, so that
	// after this call the next /api/me will see an unauthenticated session.
	http.SetCookie(w, &http.Cookie{
		Name:     "user_id",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
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

	user, err := a.store.GetUserByID(auth.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	decks, err := a.store.GetUserDeckStats(auth.UserID)
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
			Known:  deck.Known,
			Unique: deck.Unique,
			Due:    deck.Due,
		}
	}

	knownCount, err := a.store.CountKnownLemmas(auth.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	dueCount, err := a.store.CountDueCards(auth.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	newCount, err := a.store.CountNewCards(auth.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	newPerDay := 20
	if raw, ok := user.Settings["new_per_day"]; ok {
		switch v := raw.(type) {
		case float64:
			if int(v) > 0 {
				newPerDay = int(v)
			}
		case int:
			if v > 0 {
				newPerDay = v
			}
		}
	}
	if newCount > newPerDay {
		newCount = newPerDay
	}

	writeJSON(w, http.StatusOK, MeResponse{
		Authenticated: true,
		User:          sessionUserFromAuth(auth),
		Dashboard: &DashboardData{
			KnownCount:       knownCount,
			DueCount:         dueCount,
			NewCapacityToday: newCount,
			Decks:            deckSummaries,
		},
	})
}

func (a *API) HandleDecks(w http.ResponseWriter, r *http.Request) {
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
		a.handleDecksList(w, auth)
	case http.MethodPost:
		a.handleCreateDeck(w, r, auth)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleDecksList(w http.ResponseWriter, auth *AuthContext) {
	decks, err := a.store.GetUserDeckStats(auth.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	resp := DeckListResponse{Decks: make([]DeckSummary, 0, len(decks))}
	for _, deck := range decks {
		resp.Decks = append(resp.Decks, DeckSummary{
			ID:     deck.ID,
			Title:  deck.Title,
			Lang:   deck.Lang,
			Known:  deck.Known,
			Unique: deck.Unique,
			Due:    deck.Due,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleCreateDeck(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	var req CreateDeckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Title) == "" {
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

	sentences := make([]store.DeckSentenceInput, 0, len(parsed.Sentences))
	for _, sent := range parsed.Sentences {
		sentence := store.DeckSentenceInput{Text: sent.Text}
		for tokenIx, token := range sent.Tokens {
			if token.POS == "PUNCT" {
				continue
			}
			lemma := token.Lemma
			if lemma == "" {
				lemma = strings.ToLower(token.Form)
			}
			pos := token.POS
			if lemma == "" || pos == "" {
				continue
			}
			sentence.Tokens = append(sentence.Tokens, store.DeckTokenInput{
				TokenIx: tokenIx,
				Lemma:   lemma,
				POS:     pos,
			})
		}
		if len(sentence.Tokens) == 0 && sentence.Text == "" {
			continue
		}
		sentences = append(sentences, sentence)
	}

	deckID, err := a.store.CreateDeckWithSentences(auth.UserID, strings.TrimSpace(req.Title), req.Lang, sentences)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, CreateDeckResponse{DeckID: deckID})
}

func (a *API) HandleDeckByID(w http.ResponseWriter, r *http.Request) {
	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if auth == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/decks/")
	deckID, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
	if err != nil || deckID <= 0 {
		http.Error(w, "Deck ID must be a positive integer", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var req UpdateDeckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			http.Error(w, "Title is required", http.StatusBadRequest)
			return
		}
		if err := a.store.UpdateDeckTitle(auth.UserID, deckID, title); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Deck not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case http.MethodDelete:
		if err := a.store.DeleteDeck(auth.UserID, deckID); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Deck not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
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

	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if auth == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	var deckID *int64
	deckIDStr := strings.TrimSpace(r.URL.Query().Get("deck_id"))
	if deckIDStr != "" {
		parsed, err := strconv.ParseInt(deckIDStr, 10, 64)
		if err != nil || parsed <= 0 {
			http.Error(w, "Deck ID must be a positive integer", http.StatusBadRequest)
			return
		}
		ownsDeck, err := a.store.UserOwnsDeck(auth.UserID, parsed)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if !ownsDeck {
			http.Error(w, "Deck not found", http.StatusNotFound)
			return
		}
		deckID = &parsed
	}

	card, err := a.store.GetNextReviewCard(auth.UserID, deckID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if card == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	deckCounts := make([][]string, 0, len(card.DeckCounts))
	for _, pair := range card.DeckCounts {
		deckCounts = append(deckCounts, []string{pair[0], pair[1]})
	}

	examples := []CardExample{}
	if card.SentenceText != "" {
		examples = append(examples, CardExample{
			Text:       card.SentenceText,
			SourceDeck: card.SourceDeck,
		})
	}

	frontText := card.Lemma
	if card.SentenceText != "" {
		frontText = card.SentenceText
	}

	writeJSON(w, http.StatusOK, CardResponse{
		CardID:     strconv.FormatInt(card.CardID, 10),
		Mode:       "sentence",
		DeckCounts: deckCounts,
		Front: CardFront{
			Type: "sentence",
			Text: frontText,
		},
		Back: CardBack{
			Lemma:    card.Lemma,
			Meaning:  card.Gloss,
			Grammar:  "",
			Examples: examples,
		},
	})
}

func (a *API) HandleReviewAnswer(w http.ResponseWriter, r *http.Request) {
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

	var req ReviewAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.CardID <= 0 {
		http.Error(w, "Card ID must be a positive integer", http.StatusBadRequest)
		return
	}
	switch req.Rating {
	case "again", "hard", "good", "easy":
	default:
		http.Error(w, "Rating must be again, hard, good, or easy", http.StatusBadRequest)
		return
	}
	if err := a.store.RecordReviewAnswer(auth.UserID, req.CardID, req.Rating); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Card not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) HandleCardIgnore(w http.ResponseWriter, r *http.Request) {
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

	var req ReviewCardMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.CardID <= 0 {
		http.Error(w, "Card ID must be a positive integer", http.StatusBadRequest)
		return
	}
	if err := a.store.MarkCardIgnored(auth.UserID, req.CardID); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Card not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) HandleCardKnown(w http.ResponseWriter, r *http.Request) {
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

	var req ReviewCardMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.CardID <= 0 {
		http.Error(w, "Card ID must be a positive integer", http.StatusBadRequest)
		return
	}
	if err := a.store.MarkCardKnown(auth.UserID, req.CardID); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Card not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
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
	ParseID         *int64               `json:"parse_id,omitempty"`
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

	var parseID *int64
	if auth != nil {
		id, err := a.store.CreateParseSession(&auth.UserID, parsed.Lang, parsed.Parser, req.Text, parsed.TotalTokens, len(parsed.Words))
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		parseID = &id
	}

	writeJSON(w, http.StatusOK, ParseResponse{
		Lang:            parsed.Lang,
		ParseID:         parseID,
		TotalTokens:     parsed.TotalTokens,
		ParseDurationMs: parsed.ParseDurationMs,
		Stats:           parsed.Stats,
		Words:           parsed.Words,
	})
}

func (a *API) HandleParseFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.requireAuth(a.handleParseFeedback).ServeHTTP(w, r)
}

func (a *API) handleParseFeedback(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	var req ParseFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	lang := strings.TrimSpace(req.Lang)
	parser := strings.TrimSpace(req.Parser)
	surface := strings.TrimSpace(req.Surface)
	originalLemma := strings.TrimSpace(req.OriginalLemma)
	originalPOS := strings.TrimSpace(req.OriginalPOS)
	originalGrammarLabel := strings.TrimSpace(req.OriginalGrammarLabel)
	proposedLemma := strings.TrimSpace(req.ProposedLemma)
	proposedPOS := strings.TrimSpace(req.ProposedPOS)
	proposedGrammarLabel := strings.TrimSpace(req.ProposedGrammarLabel)
	note := strings.TrimSpace(req.Note)

	if lang != "FI" && lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}
	if req.ParseID <= 0 || parser == "" || surface == "" || proposedLemma == "" || proposedPOS == "" {
		http.Error(w, "Parse ID, parser, surface, proposed lemma, and proposed POS are required", http.StatusBadRequest)
		return
	}
	if req.Occurrence < 0 {
		http.Error(w, "Occurrence must be non-negative", http.StatusBadRequest)
		return
	}

	session, err := a.store.GetParseSession(req.ParseID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Parse session not found", http.StatusBadRequest)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if session.UserID == nil || *session.UserID != auth.UserID {
		http.Error(w, "Parse session does not belong to the current user", http.StatusForbidden)
		return
	}
	if session.Lang != lang || session.Parser != parser {
		http.Error(w, "Parse feedback language/parser do not match the parse session", http.StatusBadRequest)
		return
	}

	feedbackID, err := a.store.CreateParseFeedback(store.ParseFeedback{
		ParseSessionID:       session.ID,
		UserID:               auth.UserID,
		Lang:                 lang,
		Parser:               parser,
		Surface:              surface,
		Occurrence:           req.Occurrence,
		OriginalLemma:        originalLemma,
		OriginalPOS:          originalPOS,
		OriginalGrammarLabel: originalGrammarLabel,
		ProposedLemma:        proposedLemma,
		ProposedPOS:          proposedPOS,
		ProposedGrammarLabel: proposedGrammarLabel,
		Note:                 note,
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
	switch r.Method {
	case http.MethodGet, http.MethodPatch:
		a.requireAdmin(a.handleAdminParseFeedback).ServeHTTP(w, r)
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func (a *API) handleAdminParseFeedback(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	switch r.Method {
	case http.MethodGet:
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		if !isValidParseFeedbackStatus(status, true) {
			http.Error(w, "Status must be submitted, accepted, rejected, or needs_follow_up", http.StatusBadRequest)
			return
		}
		feedback, err := a.store.ListParseFeedback(status)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, ParseFeedbackListResponse{Feedback: feedback})
	case http.MethodPatch:
		feedbackIDStr := strings.TrimSpace(r.URL.Query().Get("id"))
		feedbackID, err := strconv.ParseInt(feedbackIDStr, 10, 64)
		if err != nil || feedbackID <= 0 {
			http.Error(w, "Feedback ID must be a positive integer", http.StatusBadRequest)
			return
		}
		var req ParseFeedbackReviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if !isValidParseFeedbackStatus(req.Status, false) {
			http.Error(w, "Status must be accepted, rejected, or needs_follow_up", http.StatusBadRequest)
			return
		}
		if err := a.store.ReviewParseFeedback(feedbackID, auth.UserID, req.Status, strings.TrimSpace(req.ReviewNote)); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Parse feedback not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": req.Status})
	}
}

func isValidParseFeedbackStatus(status string, allowEmpty bool) bool {
	switch status {
	case "":
		return allowEmpty
	case "submitted", "accepted", "rejected", "needs_follow_up":
		return true
	default:
		return false
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
	mux.HandleFunc("/api/auth/logout", a.HandleLogout)

	// Dashboard
	mux.HandleFunc("/api/me", a.HandleMe)

	// Parse (word list view)
	mux.HandleFunc("/api/parse", a.HandleParse)
	mux.HandleFunc("/api/parse/feedback", a.HandleParseFeedback)
	mux.HandleFunc("/api/admin/parse-feedback", a.HandleAdminParseFeedback)

	// Decks
	mux.HandleFunc("/api/decks", a.HandleDecks)
	mux.HandleFunc("/api/decks/", a.HandleDeckByID)
	mux.HandleFunc("/api/known-words", a.HandleKnownWords)

	// Review
	mux.HandleFunc("/api/review/next", a.HandleReviewNext)
	mux.HandleFunc("/api/review/answer", a.HandleReviewAnswer)
	mux.HandleFunc("/api/card/ignore", a.HandleCardIgnore)
	mux.HandleFunc("/api/card/known", a.HandleCardKnown)
}
