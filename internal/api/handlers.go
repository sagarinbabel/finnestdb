package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"finnestdb/internal/auth"
	"finnestdb/internal/glossfallback"
	"finnestdb/internal/parsecore"
	"finnestdb/internal/store"
)

const (
	sessionCookieName = "session_token"
	sessionLifetime   = 7 * 24 * time.Hour
	minPasswordLength = 8
	maxJSONBodyBytes  = 4 << 20
)

type API struct {
	store           *store.DB
	analyze         func(*store.DB, string, string, string) (*parsecore.ParseResult, error)
	analyzeChapters func(*store.DB, string, []parsecore.ChapterInput, string) (*parsecore.ParseResult, error)
	rateLimits      *rateLimitSet
}

func NewAPI(store *store.DB) *API {
	return &API{
		store:           store,
		analyze:         parsecore.Analyze,
		analyzeChapters: parsecore.AnalyzeChapters,
		rateLimits:      newRateLimitSetFromEnv(),
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
	Languages     *UserLanguages `json:"languages,omitempty"`
}

// UserLanguages is the per-user view of language settings sent to the client.
// `Learning` is the list of language codes the user is studying (closed set:
// "FI", "ET"). `Active` is the currently-selected language; it drives all
// list/filter views and the Inspect/Known-Words defaults. `Stats` carries
// vocab counts the Languages page renders next to each row; the key is the
// language code (e.g. "FI"). Languages with no decks/known words may be
// absent from the map — treat as zero.
type UserLanguages struct {
	Learning []string                 `json:"learning"`
	Active   string                   `json:"active"`
	Stats    map[string]LanguageStats `json:"stats"`
}

type LanguageStats struct {
	Decks      int `json:"decks"`
	KnownWords int `json:"known_words"`
}

// UpdateLanguagesRequest is the body for PATCH /api/me/languages. Either or
// both fields may be set; omitted fields leave the existing value alone.
type UpdateLanguagesRequest struct {
	Learning *[]string `json:"learning,omitempty"`
	Active   *string   `json:"active,omitempty"`
}

type DeckSummary struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Lang       string `json:"lang"`
	Known      int    `json:"known"`
	Unique     int    `json:"unique"`
	Due        int    `json:"due"`
	IsPublic   bool   `json:"is_public"`
	IsOwner    bool   `json:"is_owner,omitempty"`
	Subscribed bool   `json:"subscribed,omitempty"`
	// Token-weighted coverage of the deck by the user's known/ignored
	// lemmas, 0–100 with one decimal. Null when the deck has no tokens.
	ComprehensionPct *float64 `json:"comprehension_pct,omitempty"`
}

// DeckComprehensionResponse is the payload of GET /api/decks/{id}/comprehension.
type DeckComprehensionResponse struct {
	CoveragePct float64            `json:"coverage_pct"`
	TotalTokens int                `json:"total_tokens"`
	KnownTokens int                `json:"known_tokens"`
	TopUnlocks  []DeckUnlockEntry  `json:"top_unlocks"`
}

// DeckUnlockEntry is one "learn this next" candidate: an uncovered lemma
// ranked by the share of the deck's token mass it would unlock.
type DeckUnlockEntry struct {
	Lemma      string  `json:"lemma"`
	POS        string  `json:"pos"`
	TokenCount int     `json:"token_count"`
	GainPct    float64 `json:"gain_pct"`
}

type CreateDeckRequest struct {
	Title    string `json:"title"`
	Lang     string `json:"lang"`
	Text     string `json:"text"`
	IsPublic bool   `json:"is_public,omitempty"`
}

type CreateDeckResponse struct {
	DeckID int64 `json:"deck_id"`
}

type DeckListResponse struct {
	Decks []DeckSummary `json:"decks"`
}

type PublicDeckListResponse struct {
	Decks []DeckSummary `json:"decks"`
}

type UpdateDeckRequest struct {
	// Title is the new deck name. Empty / omitted means leave unchanged.
	// Updating the title still requires ownership.
	Title string `json:"title,omitempty"`
	// IsPublic toggles the official-deck flag. Pointer so the handler can
	// distinguish "omitted" (leave alone) from explicit false (unpublish).
	// Setting this field requires admin privileges.
	IsPublic *bool `json:"is_public,omitempty"`
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
	// Source tag for new rows on POST. Omit / empty → "manual".
	// Accepted: "manual" | "anki".
	Source string `json:"source,omitempty"`
	// Diff scope on PUT. Omit / empty → "anki" (preserve manual rows).
	// Accepted: "anki" | "all".
	Scope string `json:"scope,omitempty"`
}

type KnownWordsResponse struct {
	Imported   []store.KnownLemma `json:"imported"`
	Unresolved []string           `json:"unresolved"`
}

// KnownWordsReplaceResponse is returned by PUT /api/known-words. It mirrors
// the POST response shape but splits the result into adds/removes so the
// client can show the delta of an Anki "sync" import.
type KnownWordsReplaceResponse struct {
	Added      []store.KnownLemma `json:"added"`
	Removed    []store.KnownLemma `json:"removed"`
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

type LemmaStateLookupItem struct {
	Lemma  string `json:"lemma"`
	POS    string `json:"pos"`
	Status string `json:"status,omitempty"`
}

type LemmaStateLookupRequest struct {
	Lang   string                 `json:"lang"`
	Lemmas []LemmaStateLookupItem `json:"lemmas"`
}

type LemmaStateLookupResponse struct {
	States []LemmaStateLookupItem `json:"states"`
}

type ParseFeedbackRequest struct {
	// ParseID is the existing parse_sessions row to attribute this
	// feedback against. Optional: when 0 (the default after the eager
	// /api/parse persistence was removed), the handler creates a new
	// parse_session lazily from SourceText below. ParseID stays useful
	// for deck-detail feedback where the deck owns a real session row.
	ParseID int64 `json:"parse_id,omitempty"`
	// SourceText is required when ParseID is 0; it's the text the user
	// was viewing when they clicked "Suggest fix". The handler stores it
	// in the lazily-created parse_session so admin triage still has
	// context.
	SourceText           string `json:"source_text,omitempty"`
	TotalTokens          int    `json:"total_tokens,omitempty"`
	UniqueLemmaCount     int    `json:"unique_lemma_count,omitempty"`
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

type ParseSessionsResponse struct {
	Sessions []store.ParseSessionHistoryItem `json:"sessions"`
}

type DeleteParseSessionsResponse struct {
	Deleted int64 `json:"deleted"`
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
		if !allowStateChangingRequest(w, r) {
			return
		}
		next(w, r, auth)
	}
}

func allowStateChangingRequest(w http.ResponseWriter, r *http.Request) bool {
	if !isStateChangingMethod(r.Method) {
		return true
	}
	if sameOrigin(r.Header.Get("Origin"), r) {
		return true
	}
	if sameOrigin(r.Header.Get("Referer"), r) {
		return true
	}
	if r.Header.Get("Origin") == "" && r.Header.Get("Referer") == "" {
		return true
	}
	http.Error(w, "Cross-origin request rejected", http.StatusForbidden)
	return false
}

func isStateChangingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func sameOrigin(raw string, r *http.Request) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	requestScheme := "http"
	if r.TLS != nil {
		requestScheme = "https"
	}
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		requestScheme = strings.ToLower(forwardedProto)
	}
	return strings.EqualFold(u.Scheme, requestScheme) && strings.EqualFold(u.Host, r.Host)
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
	// Login CSRF: a foreign-origin register/login must not be able to issue a
	// session cookie for an attacker-controlled account in the victim browser.
	if !allowStateChangingRequest(w, r) {
		return
	}

	var req LoginRequest
	if !decodeJSONRequest(w, r, &req) {
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
	if !a.rateLimits.allowRegister(w, r, email) {
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
	if !allowStateChangingRequest(w, r) {
		return
	}

	var req LoginRequest
	if !decodeJSONRequest(w, r, &req) {
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
	if !a.rateLimits.allowLogin(w, r, email) {
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

	if user.PasswordHash == "" {
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}
	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
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
	if !allowStateChangingRequest(w, r) {
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
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
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
	if r.Method == http.MethodDelete {
		if !allowStateChangingRequest(w, r) {
			return
		}
		if err := a.store.DeleteUserCascade(auth.UserID); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "User not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, sessionCookie("", -1))
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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
			ID:               deck.ID,
			Title:            deck.Title,
			Lang:             deck.Lang,
			Known:            deck.Known,
			Unique:           deck.Unique,
			Due:              deck.Due,
			IsPublic:         deck.IsPublic,
			Subscribed:       deck.Subscribed,
			ComprehensionPct: coveragePct(deck.CoveredTokens, deck.TotalTokens),
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

	learning, active := store.UserLanguages(user.Settings)
	stats, err := a.buildLanguageStats(auth.UserID, decks)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
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
		Languages: &UserLanguages{
			Learning: learning,
			Active:   active,
			Stats:    stats,
		},
	})
}

// buildLanguageStats produces the per-language stat map sent to the client.
// `decks` is the user's full deck list (already loaded by the caller) so we
// don't re-query it; the known-word counts come straight from the store.
// Languages with no decks AND no known words are still emitted with zeros
// so the UI can render a row for every supported language.
func (a *API) buildLanguageStats(userID int64, decks []store.DeckStats) (map[string]LanguageStats, error) {
	known, err := a.store.CountKnownLemmasByLang(userID)
	if err != nil {
		return nil, err
	}
	stats := map[string]LanguageStats{}
	for _, lang := range store.SupportedLanguages {
		stats[lang] = LanguageStats{KnownWords: known[lang]}
	}
	for _, deck := range decks {
		if entry, ok := stats[deck.Lang]; ok {
			entry.Decks++
			stats[deck.Lang] = entry
		}
	}
	return stats, nil
}

// HandleUserLanguages updates the user's learning_languages list and/or
// active_language. PATCH only; both fields are optional. Validation lives in
// store.UpdateUserLanguages (closed set, non-empty list, active must be in
// learning).
func (a *API) HandleUserLanguages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
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
	if !allowStateChangingRequest(w, r) {
		return
	}

	var req UpdateLanguagesRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	user, err := a.store.GetUserByID(auth.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	learning, active := store.UserLanguages(user.Settings)
	if req.Learning != nil {
		learning = *req.Learning
	}
	if req.Active != nil {
		active = *req.Active
	}

	if err := a.store.UpdateUserLanguages(auth.UserID, learning, active); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Re-read so the response reflects exactly what was persisted (dedup,
	// canonical order, fallback active).
	user, err = a.store.GetUserByID(auth.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	persistedLearning, persistedActive := store.UserLanguages(user.Settings)
	decks, err := a.store.GetUserDeckStats(auth.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	stats, err := a.buildLanguageStats(auth.UserID, decks)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, UserLanguages{
		Learning: persistedLearning,
		Active:   persistedActive,
		Stats:    stats,
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
	if !allowStateChangingRequest(w, r) {
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
			ID:               deck.ID,
			Title:            deck.Title,
			Lang:             deck.Lang,
			Known:            deck.Known,
			Unique:           deck.Unique,
			Due:              deck.Due,
			IsPublic:         deck.IsPublic,
			Subscribed:       deck.Subscribed,
			ComprehensionPct: coveragePct(deck.CoveredTokens, deck.TotalTokens),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// coveragePct converts a covered/total token count into a display percentage
// rounded to one decimal, or nil for empty decks so the UI can render a dash
// instead of a misleading 0%.
func coveragePct(covered, total int) *float64 {
	if total <= 0 {
		return nil
	}
	pct := math.Round(float64(covered)/float64(total)*1000) / 10
	return &pct
}

// HandlePublicDecks lists every official deck the user does not already own.
// Each entry carries a "subscribed" flag so the UI can show
// "Add to studying list" vs "Remove" without an extra round-trip.
func (a *API) HandlePublicDecks(w http.ResponseWriter, r *http.Request) {
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

	decks, err := a.store.ListPublicDecksForUser(auth.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	resp := PublicDeckListResponse{Decks: make([]DeckSummary, 0, len(decks))}
	for _, deck := range decks {
		resp.Decks = append(resp.Decks, DeckSummary{
			ID:         deck.ID,
			Title:      deck.Title,
			Lang:       deck.Lang,
			Known:      deck.Known,
			Unique:     deck.Unique,
			Due:        0,
			IsPublic:   true,
			IsOwner:    deck.IsOwner,
			Subscribed: deck.Subscribed,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// tokenLemma is one (lemma, pos) candidate for a surface token.
type tokenLemma struct {
	Lemma string
	POS   string
}

// collectSurfaceForms returns the deduplicated set of non-empty surface forms
// across all tokens, used to batch-look-up homonym candidates from the dict.
func collectSurfaceForms(sentences []parsecore.SentenceResult) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, sent := range sentences {
		for _, token := range sent.Tokens {
			if token.POS == "PUNCT" || token.Form == "" {
				continue
			}
			lower := strings.ToLower(token.Form)
			if _, ok := seen[lower]; ok {
				continue
			}
			seen[lower] = struct{}{}
			out = append(out, token.Form)
		}
	}
	return out
}

// expandTokenLemmas resolves a single parsed token to one or more (lemma, pos)
// pairs. Direct dict candidates are used for homonym expansion only when they
// include the parser's selected lemma/POS; otherwise the parser pick stays
// authoritative. This preserves custom parser corrections such as lexical
// overlays and FST wins while still expanding genuine dict-known ambiguity.
// PUNCT tokens and empty lemmas are dropped — callers should not write
// occurrence rows for them.
func expandTokenLemmas(token parsecore.TokenResult, dict map[string][]store.FormResolution) []tokenLemma {
	if token.POS == "PUNCT" {
		return nil
	}
	parserPick, hasParserPick := parserTokenLemma(token)
	if hasParserPick && token.Source == "lex-overlay" {
		return []tokenLemma{parserPick}
	}

	if cands, ok := dict[token.Form]; ok && len(cands) > 0 {
		out := make([]tokenLemma, 0, len(cands))
		seen := make(map[tokenLemma]struct{}, len(cands))
		hasParserCandidate := false
		for _, c := range cands {
			if c.Lemma == "" || c.POS == "" {
				continue
			}
			tl := tokenLemma{Lemma: c.Lemma, POS: c.POS}
			if hasParserPick && tl == parserPick {
				hasParserCandidate = true
			}
			if _, dup := seen[tl]; dup {
				continue
			}
			seen[tl] = struct{}{}
			out = append(out, tl)
		}
		if len(out) > 0 && (!hasParserPick || hasParserCandidate) {
			return out
		}
	}
	if hasParserPick {
		return []tokenLemma{parserPick}
	}
	return nil
}

func parserTokenLemma(token parsecore.TokenResult) (tokenLemma, bool) {
	lemma := token.Lemma
	if lemma == "" {
		lemma = strings.ToLower(token.Form)
	}
	if lemma == "" || token.POS == "" {
		return tokenLemma{}, false
	}
	return tokenLemma{Lemma: lemma, POS: token.POS}, true
}

func (a *API) filterLowValueDictAlternatives(dict map[string][]store.FormResolution, lang string) (map[string][]store.FormResolution, map[store.LemmaKey]string, map[store.LemmaKey]struct{}) {
	keys := dictCandidateLemmaKeys(dict)
	if len(keys) == 0 {
		return dict, nil, nil
	}
	checkedKeys := lemmaKeySet(keys)
	glosses := a.store.BatchLookupGlosses(keys, lang)
	if len(glosses) == 0 {
		return dict, glosses, checkedKeys
	}
	return filterLowValueAlternatives(dict, glosses), glosses, checkedKeys
}

func dictCandidateLemmaKeys(dict map[string][]store.FormResolution) []store.LemmaKey {
	seen := map[store.LemmaKey]struct{}{}
	keys := make([]store.LemmaKey, 0)
	for _, candidates := range dict {
		for _, c := range candidates {
			if c.Lemma == "" || c.POS == "" {
				continue
			}
			k := store.LemmaKey{Lemma: c.Lemma, POS: c.POS}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			keys = append(keys, k)
		}
	}
	return keys
}

func lemmaKeySet(keys []store.LemmaKey) map[store.LemmaKey]struct{} {
	out := make(map[store.LemmaKey]struct{}, len(keys))
	for _, k := range keys {
		out[k] = struct{}{}
	}
	return out
}

// filterLowValueAlternatives keeps real ambiguity, but suppresses learner-facing
// clutter: empty-gloss alternatives, and inflected-form glosses when a lexical
// gloss for the same surface is available.
func filterLowValueAlternatives(dict map[string][]store.FormResolution, glosses map[store.LemmaKey]string) map[string][]store.FormResolution {
	out := make(map[string][]store.FormResolution, len(dict))
	for form, candidates := range dict {
		hasGlossedCandidate := false
		hasLexicalCandidate := false
		hasEnglishGlossCandidate := false
		hasNonXEnglishCandidate := false
		hasLowercaseUsefulCandidate := false
		for _, c := range candidates {
			gloss := glosses[store.LemmaKey{Lemma: c.Lemma, POS: c.POS}]
			if gloss != "" {
				hasGlossedCandidate = true
				if !isDefinitionFallbackGloss(gloss) {
					hasEnglishGlossCandidate = true
					if c.POS != "X" {
						hasNonXEnglishCandidate = true
					}
					if startsLower(c.Lemma) {
						hasLowercaseUsefulCandidate = true
					}
				}
				if !isInflectionalFormCandidate(form, c, gloss) {
					hasLexicalCandidate = true
				}
			}
		}
		if !hasGlossedCandidate {
			out[form] = candidates
			continue
		}

		filtered := make([]store.FormResolution, 0, len(candidates))
		for _, c := range candidates {
			gloss := glosses[store.LemmaKey{Lemma: c.Lemma, POS: c.POS}]
			if gloss == "" {
				continue
			}
			if hasEnglishGlossCandidate && isDefinitionFallbackGloss(gloss) {
				continue
			}
			if hasNonXEnglishCandidate && c.POS == "X" {
				continue
			}
			if hasLowercaseUsefulCandidate && strings.EqualFold(strings.TrimSpace(form), strings.TrimSpace(c.Lemma)) && startsUpper(c.Lemma) {
				continue
			}
			if hasLexicalCandidate && isInflectionalFormCandidate(form, c, gloss) {
				continue
			}
			filtered = append(filtered, c)
		}
		if len(filtered) > 0 {
			out[form] = filtered
		}
	}
	return out
}

func isDefinitionFallbackGloss(gloss string) bool {
	return glossfallback.HasETPrefix(gloss)
}

func startsLower(s string) bool {
	for _, r := range s {
		return unicode.IsLower(r)
	}
	return false
}

func startsUpper(s string) bool {
	for _, r := range s {
		return unicode.IsUpper(r)
	}
	return false
}

func isInflectionalFormCandidate(form string, candidate store.FormResolution, gloss string) bool {
	if strings.TrimSpace(form) == "" || !strings.EqualFold(strings.TrimSpace(form), strings.TrimSpace(candidate.Lemma)) {
		return false
	}
	g := strings.ToLower(strings.TrimSpace(gloss))
	if strings.ContainsAny(g, ";,") {
		return false
	}
	before, target, ok := strings.Cut(g, " of ")
	if !ok {
		return false
	}
	target = strings.Trim(strings.TrimSpace(target), ".")
	if target == "" || strings.Contains(target, " ") {
		return false
	}
	before = strings.NewReplacer("-", " ", "/", " ").Replace(before)
	parts := strings.Fields(before)
	if len(parts) == 0 {
		return false
	}
	hasMorphTerm := false
	allowed := map[string]struct{}{
		"abessive":    {},
		"ablative":    {},
		"active":      {},
		"adessive":    {},
		"allative":    {},
		"comparative": {},
		"comitative":  {},
		"conditional": {},
		"connegative": {},
		"degree":      {},
		"elative":     {},
		"essive":      {},
		"first":       {},
		"form":        {},
		"genitive":    {},
		"gerund":      {},
		"illative":    {},
		"imperative":  {},
		"indicative":  {},
		"inessive":    {},
		"infinitive":  {},
		"inflected":   {},
		"inflection":  {},
		"nominative":  {},
		"participle":  {},
		"partitive":   {},
		"passive":     {},
		"past":        {},
		"person":      {},
		"plural":      {},
		"potential":   {},
		"present":     {},
		"second":      {},
		"singular":    {},
		"superlative": {},
		"terminative": {},
		"third":       {},
		"translative": {},
	}
	for _, part := range parts {
		if _, ok := allowed[part]; !ok {
			return false
		}
		if part != "form" {
			hasMorphTerm = true
		}
	}
	if hasMorphTerm {
		return true
	}
	return len(parts) == 1 && parts[0] == "form"
}

func mergeGlosses(dst, src map[store.LemmaKey]string) map[store.LemmaKey]string {
	if len(dst) == 0 {
		dst = make(map[store.LemmaKey]string, len(src))
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func hasCheckedLemmaKey(checked map[store.LemmaKey]struct{}, key store.LemmaKey) bool {
	if checked == nil {
		return false
	}
	_, ok := checked[key]
	return ok
}

// expandParsedWords reapplies the same parser-gated homonym expansion that
// handleCreateDeck runs, producing one WordEntry per safe (lemma, pos)
// candidate for each token. The result mirrors the (lemma, pos) shape of the
// deck the user would get if they saved this parse, so the import overview's
// unique-lemma count agrees with the deck's unique count.
//
// For (lemma, pos) entries the parser also picked, GrammarLabel,
// ExampleSentence, and Gloss are inherited from the parser's WordEntry. For
// homonym alternatives the parser didn't pick, GrammarLabel stays empty;
// ExampleSentence and Gloss are derived from the first matching token's
// sentence and a dictionary lookup respectively.
func (a *API) expandParsedWords(parsed *parsecore.ParseResult, dict map[string][]store.FormResolution, glosses map[store.LemmaKey]string, checkedGlossKeys map[store.LemmaKey]struct{}) []parsecore.WordEntry {
	return a.expandSentencesToWords(parsed.Sentences, parsed.Words, parsed.Lang, dict, glosses, checkedGlossKeys)
}

// expandSentencesToWords is the sentence-scoped core of expandParsedWords:
// the parser-gated homonym expansion is run over an arbitrary sentence slice
// (whole-book or one chapter) using the same dict and gloss maps so per-
// chapter Words match what the whole-book view would have shown for the same
// (lemma, pos) pair. parserWords is consulted for grammar labels, feats, and
// example sentences when the parser already produced an entry for that key.
func (a *API) expandSentencesToWords(sentences []parsecore.SentenceResult, parserWords []parsecore.WordEntry, lang string, dict map[string][]store.FormResolution, glosses map[store.LemmaKey]string, checkedGlossKeys map[store.LemmaKey]struct{}) []parsecore.WordEntry {
	type aggKey struct {
		lemma string
		pos   string
	}
	type aggEntry struct {
		forms       []string
		formSet     map[string]struct{}
		count       int
		exampleText string
	}
	agg := map[aggKey]*aggEntry{}

	for _, sent := range sentences {
		for _, token := range sent.Tokens {
			for _, exp := range expandTokenLemmas(token, dict) {
				key := aggKey{lemma: exp.Lemma, pos: exp.POS}
				e, ok := agg[key]
				if !ok {
					e = &aggEntry{formSet: map[string]struct{}{}, exampleText: sent.Text}
					agg[key] = e
				}
				e.count++
				if token.Form != "" {
					if _, dup := e.formSet[token.Form]; !dup {
						e.formSet[token.Form] = struct{}{}
						e.forms = append(e.forms, token.Form)
					}
				}
			}
		}
	}

	// Defensive: if we couldn't extract any tokens (e.g. a test fixture or a
	// degenerate parse with empty Sentences), fall back to whatever the parser
	// already produced rather than discarding it.
	if len(agg) == 0 {
		return parserWords
	}

	parserIndex := map[aggKey]*parsecore.WordEntry{}
	for i := range parserWords {
		k := aggKey{lemma: parserWords[i].Lemma, pos: parserWords[i].POS}
		parserIndex[k] = &parserWords[i]
	}

	missingGlossKeys := make([]store.LemmaKey, 0)
	for key := range agg {
		pe, ok := parserIndex[key]
		if !ok || pe.Gloss == "" {
			lookupKey := store.LemmaKey{Lemma: key.lemma, POS: key.pos}
			if !hasCheckedLemmaKey(checkedGlossKeys, lookupKey) {
				missingGlossKeys = append(missingGlossKeys, lookupKey)
			}
		}
	}
	if len(missingGlossKeys) > 0 {
		glosses = mergeGlosses(glosses, a.store.BatchLookupGlosses(missingGlossKeys, lang))
	}

	out := make([]parsecore.WordEntry, 0, len(agg))
	for key, e := range agg {
		// Forms: alphabetical, matching parsecore.enrichWords (sort.Strings)
		// and GetDeckDetails' GROUP_CONCAT-then-strings.Sort path. Without
		// this the API would hand example-highlighting and other consumers
		// a different ordering depending on which entry point produced it.
		sort.Strings(e.forms)
		entry := parsecore.WordEntry{
			Lemma:           key.lemma,
			POS:             key.pos,
			Forms:           e.forms,
			Count:           e.count,
			ExampleSentence: e.exampleText,
		}
		if pe, ok := parserIndex[key]; ok {
			entry.Gloss = pe.Gloss
			if hasSingleNormalizedForm(e.forms) {
				entry.GrammarLabel = pe.GrammarLabel
				entry.Feats = pe.Feats
			}
			if pe.ExampleSentence != "" {
				entry.ExampleSentence = pe.ExampleSentence
			}
		}
		if entry.Gloss == "" {
			if g, ok := glosses[store.LemmaKey{Lemma: key.lemma, POS: key.pos}]; ok {
				entry.Gloss = g
			}
		}
		out = append(out, entry)
	}

	// Match parsecore.enrichWords / GetDeckDetails ordering: count desc, then
	// lemma asc. Map iteration above is non-deterministic, so without this
	// step the API contract for parsed.Words would silently drift.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Lemma != out[j].Lemma {
			return out[i].Lemma < out[j].Lemma
		}
		return out[i].POS < out[j].POS
	})
	return out
}

func hasSingleNormalizedForm(forms []string) bool {
	seen := map[string]struct{}{}
	for _, form := range forms {
		normalized := strings.ToLower(strings.TrimSpace(form))
		if normalized == "" {
			continue
		}
		seen[normalized] = struct{}{}
		if len(seen) > 1 {
			return false
		}
	}
	return len(seen) == 1
}

func (a *API) handleCreateDeck(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	var req CreateDeckRequest
	if !decodeJSONRequest(w, r, &req) {
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

	if req.IsPublic && !auth.IsAdmin {
		http.Error(w, "Admin access required to publish official decks", http.StatusForbidden)
		return
	}

	parsed, err := a.analyze(a.store, req.Lang, req.Text, "custom")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create a parse session so deck-detail feedback can be attributed to the
	// parser run that produced this deck.
	parseID, err := a.store.CreateParseSession(&auth.UserID, parsed.Lang, parsed.Parser, req.Text, parsed.TotalTokens, len(parsed.Words))
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Multi-lemma expansion: when the dictionary has multiple (lemma, pos)
	// candidates for a surface form (e.g. ET "joon" = noun "line" or 1Sg of
	// "jooma"), emit one DeckTokenInput per safe candidate so each genuine
	// homonym becomes its own card and contributes to the deck's word count.
	// Raw dict candidates are allowed to expand the token only when they still
	// contain the parser's selected lemma/POS, so custom parser protections do
	// not get overwritten during deck ingest.
	uniqueForms := collectSurfaceForms(parsed.Sentences)
	dictCandidates := a.store.BatchLookupAllForms(uniqueForms, req.Lang, "custom")
	dictCandidates, _, _ = a.filterLowValueDictAlternatives(dictCandidates, req.Lang)

	sentences := make([]store.DeckSentenceInput, 0, len(parsed.Sentences))
	for _, sent := range parsed.Sentences {
		sentence := store.DeckSentenceInput{Text: sent.Text}
		for tokenIx, token := range sent.Tokens {
			for _, exp := range expandTokenLemmas(token, dictCandidates) {
				sentence.Tokens = append(sentence.Tokens, store.DeckTokenInput{
					TokenIx: tokenIx,
					Form:    token.Form,
					Lemma:   exp.Lemma,
					POS:     exp.POS,
				})
			}
		}
		if len(sentence.Tokens) == 0 && sentence.Text == "" {
			continue
		}
		sentences = append(sentences, sentence)
	}

	deckID, err := a.store.CreateDeckWithSentencesOptions(auth.UserID, strings.TrimSpace(req.Title), req.Lang, req.IsPublic, sentences)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if err := a.store.SetDeckParseSession(auth.UserID, deckID, parseID); err != nil {
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
	if !allowStateChangingRequest(w, r) {
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/decks/")
	idStr := rest
	suffix := ""
	if slash := strings.Index(rest, "/"); slash >= 0 {
		idStr = rest[:slash]
		suffix = rest[slash:]
	}
	deckID, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
	if err != nil || deckID <= 0 {
		http.Error(w, "Deck ID must be a positive integer", http.StatusBadRequest)
		return
	}

	if suffix == "/subscribe" {
		a.handleDeckSubscribe(w, r, auth, deckID)
		return
	}
	if suffix == "/comprehension" {
		a.handleDeckComprehension(w, r, auth, deckID)
		return
	}
	if suffix != "" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.handleGetDeck(w, auth, deckID)
	case http.MethodPatch:
		var req UpdateDeckRequest
		if !decodeJSONRequest(w, r, &req) {
			return
		}
		title := strings.TrimSpace(req.Title)
		if title == "" && req.IsPublic == nil {
			http.Error(w, "Title or is_public is required", http.StatusBadRequest)
			return
		}
		// Authorisation differs between fields: is_public is admin-only on
		// any deck (admins manage the catalog), title is owner-only. When
		// both are present we have to check BOTH up front; otherwise an
		// admin patching a deck they don't own could flip is_public, fail
		// the title update, and walk away thinking the request 404'd while
		// the visibility actually changed.
		if req.IsPublic != nil && !auth.IsAdmin {
			http.Error(w, "Admin access required to change deck visibility", http.StatusForbidden)
			return
		}
		if title != "" {
			owns, err := a.store.UserOwnsDeck(auth.UserID, deckID)
			if err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			if !owns {
				// Distinguish "deck doesn't exist" from "you're not the
				// owner" only when the visibility flag isn't also being
				// changed — otherwise we'd leak existence to a non-owner
				// admin.
				if req.IsPublic == nil {
					http.Error(w, "Deck not found", http.StatusNotFound)
				} else {
					http.Error(w, "Only the deck owner can rename it", http.StatusForbidden)
				}
				return
			}
		}
		if req.IsPublic != nil && title != "" {
			if err := a.store.UpdateDeckTitleAndPublic(auth.UserID, deckID, title, *req.IsPublic); err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "Deck not found", http.StatusNotFound)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		if req.IsPublic != nil {
			if err := a.store.SetDeckIsPublic(deckID, *req.IsPublic); err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "Deck not found", http.StatusNotFound)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
		}
		if title != "" {
			if err := a.store.UpdateDeckTitle(auth.UserID, deckID, title); err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "Deck not found", http.StatusNotFound)
					return
				}
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
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

type DeckDetailResponse struct {
	ID          int64       `json:"id"`
	Title       string      `json:"title"`
	Lang        string      `json:"lang"`
	Parser      string      `json:"parser"`
	ParseID     *int64      `json:"parse_id,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	TotalTokens int         `json:"total_tokens"`
	Words       []WordEntry `json:"words"`
	IsPublic    bool        `json:"is_public"`
	IsOwner     bool        `json:"is_owner"`
	Subscribed  bool        `json:"subscribed"`
}

func (a *API) handleGetDeck(w http.ResponseWriter, auth *AuthContext, deckID int64) {
	details, err := a.store.GetDeckDetails(auth.UserID, deckID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Deck not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	isOwner := details.UserID == auth.UserID
	var subscribed bool
	if !isOwner && details.IsPublic {
		subscribed, err = a.store.UserSubscribedToDeck(auth.UserID, deckID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	}

	keys := make([]store.LemmaKey, 0, len(details.Lemmas))
	for _, item := range details.Lemmas {
		keys = append(keys, store.LemmaKey{Lemma: item.Lemma, POS: item.POS})
	}
	states, err := a.store.BatchLemmaStates(auth.UserID, details.Lang, keys)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	words := make([]WordEntry, 0, len(details.Lemmas))
	for _, item := range details.Lemmas {
		entry := WordEntry{
			Lemma:           item.Lemma,
			POS:             item.POS,
			Forms:           item.Forms,
			Count:           item.Count,
			Gloss:           item.Gloss,
			ExampleSentence: item.ExampleSentence,
		}
		if len(entry.Forms) == 0 {
			entry.Forms = []string{item.Lemma}
		}
		if status := states[store.LemmaKey{Lemma: item.Lemma, POS: item.POS}]; status != "" {
			entry.LearningState = status
		}
		words = append(words, entry)
	}

	writeJSON(w, http.StatusOK, DeckDetailResponse{
		ID:          details.ID,
		Title:       details.Title,
		Lang:        details.Lang,
		Parser:      "custom",
		ParseID:     details.ParseSessionID,
		CreatedAt:   details.CreatedAt,
		TotalTokens: details.TotalTokens,
		Words:       words,
		IsPublic:    details.IsPublic,
		IsOwner:     isOwner,
		Subscribed:  subscribed,
	})
}

// handleDeckSubscribe handles POST/DELETE /api/decks/:id/subscribe. POST adds
// an official deck to the user's studying list and seeds cards for each
// (lemma, pos) the user has not already marked known/ignored. DELETE removes
// the subscription but leaves seeded cards in place — matching how deleting
// an owned deck preserves global learning state.
// handleDeckComprehension serves GET /api/decks/{id}/comprehension: the
// user's token-weighted coverage of the deck plus the top-10 uncovered
// lemmas ranked by marginal comprehension gain ("learn these next").
func (a *API) handleDeckComprehension(w http.ResponseWriter, r *http.Request, auth *AuthContext, deckID int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stats, err := a.store.DeckComprehension(auth.UserID, deckID, 10)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Deck not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	resp := DeckComprehensionResponse{
		TotalTokens: stats.TotalTokens,
		KnownTokens: stats.CoveredTokens,
		TopUnlocks:  make([]DeckUnlockEntry, 0, len(stats.TopUnlocks)),
	}
	if pct := coveragePct(stats.CoveredTokens, stats.TotalTokens); pct != nil {
		resp.CoveragePct = *pct
	}
	for _, unlock := range stats.TopUnlocks {
		entry := DeckUnlockEntry{
			Lemma:      unlock.Lemma,
			POS:        unlock.POS,
			TokenCount: unlock.TokenCount,
		}
		if pct := coveragePct(unlock.TokenCount, stats.TotalTokens); pct != nil {
			entry.GainPct = *pct
		}
		resp.TopUnlocks = append(resp.TopUnlocks, entry)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleDeckSubscribe(w http.ResponseWriter, r *http.Request, auth *AuthContext, deckID int64) {
	switch r.Method {
	case http.MethodPost:
		if err := a.store.SubscribeUserToPublicDeck(auth.UserID, deckID); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Official deck not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case http.MethodDelete:
		if err := a.store.UnsubscribeUserFromPublicDeck(auth.UserID, deckID); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Subscription not found", http.StatusNotFound)
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
	if !allowStateChangingRequest(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.handleKnownWordsList(w, r, auth)
	case http.MethodPost:
		a.handleKnownWordsImport(w, r, auth)
	case http.MethodPut:
		a.handleKnownWordsReplace(w, r, auth)
	case http.MethodDelete:
		a.handleKnownWordsDelete(w, r, auth)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleKnownWordsReplace(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	var req KnownWordsRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}
	if req.Lang != "FI" && req.Lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}
	// Unlike POST, an empty words list is meaningful here — it means "clear
	// this language's vocabulary". We still require the field to be present
	// (decoded JSON), but len==0 is allowed.
	if req.Words == nil {
		http.Error(w, "Words list is required (use [] to clear)", http.StatusBadRequest)
		return
	}

	scope := req.Scope
	if scope == "" {
		scope = store.SourceAnki
	}
	if scope != "all" && scope != store.SourceAnki {
		http.Error(w, "scope must be 'anki' or 'all'", http.StatusBadRequest)
		return
	}

	added, removed, unresolved, err := a.store.ReplaceKnownWords(auth.UserID, req.Lang, req.Words, scope)
	if err != nil {
		if errors.Is(err, store.ErrKnownWordsReplaceNoResolvedWords) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, KnownWordsReplaceResponse{
		Added:      added,
		Removed:    removed,
		Unresolved: unresolved,
	})
}

func (a *API) handleKnownWordsImport(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	var req KnownWordsRequest
	if !decodeJSONRequest(w, r, &req) {
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

	source := req.Source
	if source == "" {
		source = store.SourceManual
	}
	if source != store.SourceManual && source != store.SourceAnki {
		http.Error(w, "source must be 'manual' or 'anki'", http.StatusBadRequest)
		return
	}

	imported, unresolved, err := a.store.ImportKnownWords(auth.UserID, req.Lang, req.Words, source)
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
	q := r.URL.Query()
	lang := strings.ToUpper(strings.TrimSpace(q.Get("lang")))
	lemma := strings.TrimSpace(q.Get("lemma"))
	pos := strings.TrimSpace(q.Get("pos"))
	if lang != "FI" && lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}

	// Bulk path: `all=1` clears every known-word row for this (user, lang).
	// Requiring the explicit flag prevents the per-row delete from
	// accidentally wiping the language when the client forgets to set
	// lemma/pos.
	if q.Get("all") == "1" {
		if lemma != "" || pos != "" {
			http.Error(w, "all=1 cannot be combined with lemma/pos", http.StatusBadRequest)
			return
		}
		deleted, err := a.store.DeleteAllKnownWords(auth.UserID, lang)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "deleted": deleted})
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

func (a *API) HandleLemmaStates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.requireAuth(a.handleLemmaStates).ServeHTTP(w, r)
}

func (a *API) handleLemmaState(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	var req LemmaStateRequest
	if !decodeJSONRequest(w, r, &req) {
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
	case "", "neutral":
		err = a.store.ClearLemmaState(auth.UserID, lang, lemma, pos)
		status = ""
	default:
		http.Error(w, "Status must be known, ignored, or empty", http.StatusBadRequest)
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

func (a *API) handleLemmaStates(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	var req LemmaStateLookupRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	lang := strings.ToUpper(strings.TrimSpace(req.Lang))
	if lang != "FI" && lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}

	keys := make([]store.LemmaKey, 0, len(req.Lemmas))
	seen := make(map[store.LemmaKey]struct{}, len(req.Lemmas))
	for _, item := range req.Lemmas {
		lemma := strings.TrimSpace(item.Lemma)
		pos := strings.ToUpper(strings.TrimSpace(item.POS))
		if lemma == "" || pos == "" {
			continue
		}
		key := store.LemmaKey{Lemma: lemma, POS: pos}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}

	states, err := a.store.BatchLemmaStates(auth.UserID, lang, keys)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	resp := LemmaStateLookupResponse{States: make([]LemmaStateLookupItem, 0, len(states))}
	for _, key := range keys {
		status := states[key]
		if status == "" {
			continue
		}
		resp.States = append(resp.States, LemmaStateLookupItem{
			Lemma:  key.Lemma,
			POS:    key.POS,
			Status: status,
		})
	}
	writeJSON(w, http.StatusOK, resp)
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
		canStudy, err := a.store.UserCanStudyDeck(auth.UserID, parsed)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if !canStudy {
			http.Error(w, "Deck not found", http.StatusNotFound)
			return
		}
		deckID = &parsed
	}

	lang := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("lang")))
	if lang != "" && !store.IsSupportedLanguage(lang) {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}

	card, err := a.store.GetNextReviewCard(auth.UserID, deckID, lang)
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
	if !allowStateChangingRequest(w, r) {
		return
	}

	var req ReviewAnswerRequest
	if !decodeJSONRequest(w, r, &req) {
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
	if !allowStateChangingRequest(w, r) {
		return
	}

	var req ReviewCardMutationRequest
	if !decodeJSONRequest(w, r, &req) {
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
	if !allowStateChangingRequest(w, r) {
		return
	}

	var req ReviewCardMutationRequest
	if !decodeJSONRequest(w, r, &req) {
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
	Lang     string                   `json:"lang"`
	Text     string                   `json:"text,omitempty"`
	Parser   string                   `json:"parser"` // parser name; defaults to "basic"
	Chapters []parsecore.ChapterInput `json:"chapters,omitempty"`
}

type WordEntry = parsecore.WordEntry

// ChapterResponse mirrors parsecore.ChapterResult but is keyed off the API's
// WordEntry alias so the wire shape stays stable if WordEntry diverges later.
// LearningState is populated from the authenticated user's per-lemma state so
// the client doesn't have to do a second round of state lookups per chapter.
type ChapterResponse = parsecore.ChapterResult

type ParseResponse struct {
	Lang            string               `json:"lang"`
	ParseID         *int64               `json:"parse_id,omitempty"`
	TotalTokens     int                  `json:"total_tokens"`
	ParseDurationMs float64              `json:"parse_duration_ms"`
	Stats           parsecore.ParseStats `json:"stats"`
	Words           []WordEntry          `json:"words"`
	// Chapters is only populated when the request was a chapters payload
	// (EPUB import flow). Each entry's Words list is independently usable —
	// the client can swap the displayed words to a chapter's Words without
	// another /api/parse round-trip.
	Chapters []ChapterResponse `json:"chapters,omitempty"`
}

func (a *API) HandleParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth, err := a.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if !a.rateLimits.allowParse(w, r, auth) {
		return
	}

	var req ParseRequest
	if !decodeJSONRequest(w, r, &req) {
		return
	}

	if req.Lang != "FI" && req.Lang != "ET" {
		http.Error(w, "Language must be FI or ET", http.StatusBadRequest)
		return
	}

	hasText := len(req.Text) > 0
	hasChapters := len(req.Chapters) > 0
	if hasText == hasChapters {
		// Reject both empty and both populated — the latter is an ambiguous
		// payload that earlier code would have silently dropped one half of.
		if !hasText && !hasChapters {
			http.Error(w, "Text or chapters is required", http.StatusBadRequest)
		} else {
			http.Error(w, "Provide either text or chapters, not both", http.StatusBadRequest)
		}
		return
	}

	var parsed *parsecore.ParseResult
	var parseErr error
	if hasChapters {
		parsed, parseErr = a.analyzeChapters(a.store, req.Lang, req.Chapters, req.Parser)
	} else {
		parsed, parseErr = a.analyze(a.store, req.Lang, req.Text, req.Parser)
	}
	if parseErr != nil {
		status := http.StatusInternalServerError
		switch parseErr.Error() {
		case "language must be FI or ET", "text is required", "chapters is required":
			status = http.StatusBadRequest
		default:
			if len(parseErr.Error()) >= 13 && parseErr.Error()[:13] == "text exceeds " {
				status = http.StatusBadRequest
			}
			if len(parseErr.Error()) >= 19 && parseErr.Error()[:19] == "unsupported parser " {
				status = http.StatusBadRequest
			}
		}
		http.Error(w, parseErr.Error(), status)
		return
	}

	// Apply the same parser-gated homonym expansion that handleCreateDeck runs,
	// so the import overview's unique-lemma count matches the count of the
	// deck the user gets when saving. See expandSentencesToWords for details.
	uniqueForms := collectSurfaceForms(parsed.Sentences)
	dictCandidates := a.store.BatchLookupAllForms(uniqueForms, req.Lang, parsed.Parser)
	dictCandidates, dictGlosses, checkedGlossKeys := a.filterLowValueDictAlternatives(dictCandidates, req.Lang)
	parserWords := parsed.Words
	parserChapterWords := make([][]parsecore.WordEntry, len(parsed.Chapters))
	if hasChapters {
		for i := range parsed.Chapters {
			parserChapterWords[i] = parsed.Chapters[i].Words
		}
	}
	parsed.Words = a.expandSentencesToWords(parsed.Sentences, parserWords, parsed.Lang, dictCandidates, dictGlosses, checkedGlossKeys)

	// Per-chapter Words go through the same homonym-expansion pipeline so
	// switching to a chapter view doesn't surface a different (lemma, pos)
	// set than the whole-book view would for the same tokens. dictCandidates
	// and the already-merged gloss map are reused across all chapters.
	if hasChapters {
		for i := range parsed.Chapters {
			chSentences := chapterSentenceSubset(parsed.Sentences, i)
			parsed.Chapters[i].Words = a.expandSentencesToWords(chSentences, parserChapterWords[i], parsed.Lang, dictCandidates, dictGlosses, checkedGlossKeys)
			parsed.Chapters[i].LemmaCount = len(parsed.Chapters[i].Words)
		}
	}

	// /api/parse no longer creates a parse_sessions row. Persistence is
	// deferred until the user does something durable with the parse — saves
	// it as a deck (handleCreateDeck creates the row) or submits feedback
	// (handleParseFeedback creates one lazily from inline source_text).
	// This matches the "return data, persist on save" model so a user who
	// inspects and walks away leaves nothing in parse_sessions.
	if auth != nil {
		applyLemmaStatesInPlace(a.store, auth.UserID, parsed.Lang, parsed.Words)
		for i := range parsed.Chapters {
			applyLemmaStatesInPlace(a.store, auth.UserID, parsed.Lang, parsed.Chapters[i].Words)
		}
	}

	writeJSON(w, http.StatusOK, ParseResponse{
		Lang: parsed.Lang,
		// ParseID is intentionally nil — /api/parse no longer persists.
		TotalTokens:     parsed.TotalTokens,
		ParseDurationMs: float64(parsed.ParseDurationNs) / 1e6,
		Stats:           parsed.Stats,
		Words:           parsed.Words,
		Chapters:        parsed.Chapters,
	})
}

// chapterSentenceSubset returns the sentences whose ChapterIdx matches idx.
// Sentences without a ChapterIdx (plain-text parses) are never included; the
// chapters branch always sets ChapterIdx in toParsedSentences.
func chapterSentenceSubset(sentences []parsecore.SentenceResult, idx int) []parsecore.SentenceResult {
	out := make([]parsecore.SentenceResult, 0, len(sentences))
	for _, sent := range sentences {
		if sent.ChapterIdx == nil {
			continue
		}
		if *sent.ChapterIdx != idx {
			continue
		}
		out = append(out, sent)
	}
	return out
}

// applyLemmaStatesInPlace fills LearningState on each WordEntry from the
// user's per-(lemma, pos) state map. A single BatchLemmaStates call per slice
// keeps the per-chapter passes cheap relative to the original 41-request
// architecture they replace. Errors are logged and swallowed — the lemma
// state is decorative, not load-bearing, and an error path here would
// otherwise mask the parse result the user just spent compute on.
func applyLemmaStatesInPlace(db *store.DB, userID int64, lang string, words []parsecore.WordEntry) {
	if len(words) == 0 {
		return
	}
	keys := make([]store.LemmaKey, 0, len(words))
	for _, w := range words {
		if strings.TrimSpace(w.Lemma) == "" || strings.TrimSpace(w.POS) == "" {
			continue
		}
		keys = append(keys, store.LemmaKey{Lemma: w.Lemma, POS: w.POS})
	}
	if len(keys) == 0 {
		return
	}
	states, err := db.BatchLemmaStates(userID, lang, keys)
	if err != nil {
		log.Printf("api: BatchLemmaStates(%s): %v", lang, err)
		return
	}
	for i := range words {
		key := store.LemmaKey{Lemma: words[i].Lemma, POS: words[i].POS}
		if status := states[key]; status != "" {
			words[i].LearningState = status
		}
	}
}

func (a *API) HandleParseSessions(w http.ResponseWriter, r *http.Request) {
	a.requireAuth(a.handleParseSessions).ServeHTTP(w, r)
}

func (a *API) handleParseSessions(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	switch r.Method {
	case http.MethodGet:
		sessions, err := a.store.ListUserParseSessions(auth.UserID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, ParseSessionsResponse{Sessions: sessions})
	case http.MethodDelete:
		deleted, err := a.store.DeleteUserParseSessions(auth.UserID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, DeleteParseSessionsResponse{Deleted: deleted})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) HandleParseSessionByID(w http.ResponseWriter, r *http.Request) {
	a.requireAuth(a.handleParseSessionByID).ServeHTTP(w, r)
}

func (a *API) handleParseSessionByID(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idPart := strings.TrimPrefix(r.URL.Path, "/api/parse/sessions/")
	if idPart == "" || strings.Contains(idPart, "/") {
		http.Error(w, "Parse session not found", http.StatusNotFound)
		return
	}
	parseSessionID, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil || parseSessionID <= 0 {
		http.Error(w, "Parse session not found", http.StatusNotFound)
		return
	}
	if err := a.store.DeleteUserParseSession(auth.UserID, parseSessionID); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Parse session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) HandleParseFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.requireAuth(a.handleParseFeedback).ServeHTTP(w, r)
}

func (a *API) handleParseFeedback(w http.ResponseWriter, r *http.Request, auth *AuthContext) {
	if !a.rateLimits.allowFeedback(w, r, auth) {
		return
	}
	var req ParseFeedbackRequest
	if !decodeJSONRequest(w, r, &req) {
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
	if parser == "" || surface == "" || proposedLemma == "" || proposedPOS == "" {
		http.Error(w, "Parser, surface, proposed lemma, and proposed POS are required", http.StatusBadRequest)
		return
	}
	if req.Occurrence < 0 {
		http.Error(w, "Occurrence must be non-negative", http.StatusBadRequest)
		return
	}

	// Resolve a parse_session to attribute this feedback against. Two
	// entry points:
	//   1. ParseID > 0: feedback from a deck-detail view, where the deck
	//      owns a real persisted session. Validate ownership + lang/parser.
	//   2. ParseID == 0: feedback from the Inspect view, where /api/parse
	//      no longer persists. Create a session lazily from the inline
	//      SourceText so admin triage still has context to review.
	var sessionID int64
	if req.ParseID > 0 {
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
		sessionID = session.ID
	} else {
		sourceText := strings.TrimSpace(req.SourceText)
		if sourceText == "" {
			http.Error(w, "source_text is required when parse_id is not provided", http.StatusBadRequest)
			return
		}
		id, err := a.store.CreateParseSession(&auth.UserID, lang, parser, req.SourceText, req.TotalTokens, req.UniqueLemmaCount)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		sessionID = id
	}

	feedbackID, err := a.store.CreateParseFeedback(store.ParseFeedback{
		ParseSessionID:       sessionID,
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
		if !decodeJSONRequest(w, r, &req) {
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
		if !decodeJSONRequest(w, r, &req) {
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

func decodeJSONRequest(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

// HandleHealth is the deployment liveness/readiness probe: 200 with a JSON
// body when the process is up and the database answers a trivial query, 503
// otherwise. Unauthenticated by design — it must never expose data beyond
// up/down, because uptime monitors hit it anonymously.
func (a *API) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.store.Healthcheck(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) SetupRoutes(mux *http.ServeMux) {
	// Ops
	mux.HandleFunc("/api/health", a.HandleHealth)

	// Auth routes
	mux.HandleFunc("/api/auth/register", a.HandleRegister)
	mux.HandleFunc("/api/auth/login", a.HandleLogin)
	mux.HandleFunc("/api/auth/logout", a.HandleLogout)

	// Dashboard
	mux.HandleFunc("/api/me", a.HandleMe)
	mux.HandleFunc("/api/me/languages", a.HandleUserLanguages)

	// Parse (word list view)
	mux.HandleFunc("/api/parse", a.HandleParse)
	mux.HandleFunc("/api/parse/sessions", a.HandleParseSessions)
	mux.HandleFunc("/api/parse/sessions/", a.HandleParseSessionByID)
	mux.HandleFunc("/api/parse/feedback", a.HandleParseFeedback)
	mux.HandleFunc("/api/admin/parse-feedback", a.HandleAdminParseFeedback)
	mux.HandleFunc("/api/admin/users", a.HandleAdminUsers)

	// Decks. net/http's ServeMux uses longest-prefix matching, not
	// registration order, so "/api/decks/public" (exact match) wins over
	// "/api/decks/" (subtree) regardless of which line goes first. Group
	// is just for readability.
	mux.HandleFunc("/api/decks", a.HandleDecks)
	mux.HandleFunc("/api/decks/public", a.HandlePublicDecks)
	mux.HandleFunc("/api/decks/", a.HandleDeckByID)

	// Import (file upload → plain text)
	mux.HandleFunc("/api/import/extract", a.HandleImportExtract)
	mux.HandleFunc("/api/known-words", a.HandleKnownWords)
	mux.HandleFunc("/api/lemma-state", a.HandleLemmaState)
	mux.HandleFunc("/api/lemma-states", a.HandleLemmaStates)

	// Review
	mux.HandleFunc("/api/review/next", a.HandleReviewNext)
	mux.HandleFunc("/api/review/answer", a.HandleReviewAnswer)
	mux.HandleFunc("/api/card/ignore", a.HandleCardIgnore)
	mux.HandleFunc("/api/card/known", a.HandleCardKnown)
}
