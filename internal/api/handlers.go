package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"finnestdb/internal/auth"
	"finnestdb/internal/parsecore"
	"finnestdb/internal/store"
)

const (
	sessionCookieName = "session_token"
	sessionLifetime   = 7 * 24 * time.Hour
	minPasswordLength = 8
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

type LemmaStateRequest struct {
	Lang   string `json:"lang"`
	Lemma  string `json:"lemma"`
	POS    string `json:"pos"`
	Status string `json:"status"`
}

type LemmaStateResponse struct {
	Lang   string `json:"lang"`
	Lemma  string `json:"lemma"`
	POS    string `json:"pos"`
	Status string `json:"status"`
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
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, nil
	}
	user, err := a.store.GetUserBySessionTokenHash(auth.HashToken(cookie.Value), sessionLifetime)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	return &AuthContext{
		UserID:  user.ID,
		Email:   user.Email,
		IsAdmin: user.IsAdmin,
	}, nil
}

// issueSession creates a new server-side session for the user and writes the
// session_token cookie on the response. The raw token is only ever in the
// cookie; the database stores its SHA256 hash.
func (a *API) issueSession(w http.ResponseWriter, userID int64) error {
	token, err := auth.NewSessionToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(sessionLifetime)
	if err := a.store.CreateSession(userID, auth.HashToken(token), expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, sessionCookie(token, int(sessionLifetime.Seconds())))
	return nil
}

func sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.EqualFold(os.Getenv("APP_ENV"), "production"),
		MaxAge:   maxAge,
	}
}

func normalizeAuthEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func validateEmail(email string) error {
	if email == "" {
		return errBadRequest("Email is required")
	}
	if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
		return errBadRequest("Email is invalid")
	}
	return nil
}

func validatePassword(pw string) error {
	if len(pw) < minPasswordLength {
		return errBadRequest("Password must be at least 8 characters")
	}
	return nil
}

type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string { return e.message }

func errBadRequest(msg string) error { return &httpError{status: http.StatusBadRequest, message: msg} }

func writeAuthError(w http.ResponseWriter, err error) {
	if he, ok := err.(*httpError); ok {
		http.Error(w, he.message, he.status)
		return
	}
	http.Error(w, "Internal error", http.StatusInternalServerError)
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

func (a *API) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	email := normalizeAuthEmail(req.Email)
	if err := validateEmail(email); err != nil {
		writeAuthError(w, err)
		return
	}
	if err := validatePassword(req.Password); err != nil {
		writeAuthError(w, err)
		return
	}

	if existing, err := a.store.GetUserByEmail(email); err == nil && existing != nil {
		http.Error(w, "Email already registered", http.StatusConflict)
		return
	} else if err != nil && err != sql.ErrNoRows {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "Unable to secure password", http.StatusInternalServerError)
		return
	}
	user, err := a.store.CreateUser(email, hash)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if err := a.issueSession(w, user.ID); err != nil {
		http.Error(w, "Unable to create session", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, LoginResponse{
		Authenticated: true,
		User: &SessionUser{
			ID:      user.ID,
			Email:   user.Email,
			IsAdmin: user.IsAdmin,
		},
	})
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

	email := normalizeAuthEmail(req.Email)
	if err := validateEmail(email); err != nil {
		writeAuthError(w, err)
		return
	}
	if err := validatePassword(req.Password); err != nil {
		writeAuthError(w, err)
		return
	}

	user, err := a.store.GetUserByEmail(email)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Bootstrap-on-first-login: existing alpha accounts have empty
	// password_hash. The first password they submit becomes their permanent
	// password. Subsequent logins verify normally.
	if user.PasswordHash == "" {
		hash, hashErr := auth.HashPassword(req.Password)
		if hashErr != nil {
			http.Error(w, "Unable to secure password", http.StatusInternalServerError)
			return
		}
		if err := a.store.SetUserPassword(user.ID, hash); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	} else if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	if err := a.issueSession(w, user.ID); err != nil {
		http.Error(w, "Unable to create session", http.StatusInternalServerError)
		return
	}

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

	// Revoke server-side session if a token is presented; expire the cookie
	// either way so the browser stops sending it.
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		_ = a.store.RevokeSessionByTokenHash(auth.HashToken(cookie.Value))
	}
	http.SetCookie(w, sessionCookie("", -1))

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

func (a *API) HandleLemmaState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.requireAuth(a.handleLemmaState).ServeHTTP(w, r)
}

func (a *API) handleLemmaState(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	var req LemmaStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	lang := strings.ToUpper(strings.TrimSpace(req.Lang))
	lemma := strings.TrimSpace(req.Lemma)
	pos := strings.ToUpper(strings.TrimSpace(req.POS))
	status := strings.ToLower(strings.TrimSpace(req.Status))

	if lang != "FI" && lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}
	if lemma == "" || pos == "" {
		http.Error(w, "Lemma and POS are required", http.StatusBadRequest)
		return
	}

	var err error
	switch status {
	case "known":
		err = a.store.MarkLemmaKnown(auth.UserID, lang, lemma, pos)
	case "ignored":
		err = a.store.MarkLemmaIgnored(auth.UserID, lang, lemma, pos)
	default:
		http.Error(w, "Status must be known or ignored", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, LemmaStateResponse{
		Lang:   lang,
		Lemma:  lemma,
		POS:    pos,
		Status: status,
	})
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

		keys := make([]store.LemmaKey, 0, len(parsed.Words))
		for _, word := range parsed.Words {
			if strings.TrimSpace(word.Lemma) == "" || strings.TrimSpace(word.POS) == "" {
				continue
			}
			keys = append(keys, store.LemmaKey{Lemma: word.Lemma, POS: word.POS})
		}
		states, err := a.store.BatchLemmaStates(auth.UserID, parsed.Lang, keys)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		for i := range parsed.Words {
			key := store.LemmaKey{Lemma: parsed.Words[i].Lemma, POS: parsed.Words[i].POS}
			if status := states[key]; status != "" {
				parsed.Words[i].LearningState = status
			}
		}
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

type AdminUser struct {
	ID      int64  `json:"id"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

type AdminUserListResponse struct {
	Users []AdminUser `json:"users"`
}

type AdminUserUpdateRequest struct {
	IsAdmin bool `json:"is_admin"`
}

func (a *API) HandleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPatch:
		a.requireAdmin(a.handleAdminUsers).ServeHTTP(w, r)
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func (a *API) handleAdminUsers(w http.ResponseWriter, r *http.Request, authCtx *AuthContext) {
	switch r.Method {
	case http.MethodGet:
		users, err := a.store.ListUsers()
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		out := make([]AdminUser, len(users))
		for i, u := range users {
			out[i] = AdminUser{ID: u.ID, Email: u.Email, IsAdmin: u.IsAdmin}
		}
		writeJSON(w, http.StatusOK, AdminUserListResponse{Users: out})
	case http.MethodPatch:
		idStr := strings.TrimSpace(r.URL.Query().Get("id"))
		userID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || userID <= 0 {
			http.Error(w, "User ID must be a positive integer", http.StatusBadRequest)
			return
		}
		var req AdminUserUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		// Self-demotion guard: an admin cannot remove their own is_admin flag,
		// otherwise nothing prevents the last admin from locking everyone out.
		if userID == authCtx.UserID && !req.IsAdmin {
			http.Error(w, "You cannot remove your own admin access", http.StatusForbidden)
			return
		}
		if _, err := a.store.GetUserByID(userID); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "User not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if err := a.store.SetUserAdmin(userID, req.IsAdmin); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"is_admin": req.IsAdmin})
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
	mux.HandleFunc("/api/auth/register", a.HandleRegister)
	mux.HandleFunc("/api/auth/login", a.HandleLogin)
	mux.HandleFunc("/api/auth/logout", a.HandleLogout)

	// Dashboard
	mux.HandleFunc("/api/me", a.HandleMe)

	// Parse (word list view)
	mux.HandleFunc("/api/parse", a.HandleParse)
	mux.HandleFunc("/api/parse/feedback", a.HandleParseFeedback)
	mux.HandleFunc("/api/admin/parse-feedback", a.HandleAdminParseFeedback)
	mux.HandleFunc("/api/admin/users", a.HandleAdminUsers)

	// Decks
	mux.HandleFunc("/api/decks", a.HandleDecks)
	mux.HandleFunc("/api/decks/", a.HandleDeckByID)
	mux.HandleFunc("/api/known-words", a.HandleKnownWords)
	mux.HandleFunc("/api/lemma-state", a.HandleLemmaState)

	// Review
	mux.HandleFunc("/api/review/next", a.HandleReviewNext)
	mux.HandleFunc("/api/review/answer", a.HandleReviewAnswer)
	mux.HandleFunc("/api/card/ignore", a.HandleCardIgnore)
	mux.HandleFunc("/api/card/known", a.HandleCardKnown)
}
