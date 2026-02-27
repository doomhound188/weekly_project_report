package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/weekly-report/internal/config"
)

// Session stores user session data.
type Session struct {
	UserName  string
	UserEmail string
	ExpiresAt time.Time
}

// Handler manages Azure AD OAuth2 authentication.
type Handler struct {
	clientID     string
	clientSecret string
	tenantID     string
	authority    string
	redirectPath string
	configured   bool

	// In-memory session store (keyed by cookie value)
	sessions map[string]*Session
	mu       sync.RWMutex
}

// UserInfo is the JSON response for the current user.
type UserInfo struct {
	Name          string `json:"name"`
	Email         string `json:"email"`
	Authenticated bool   `json:"authenticated"`
}

// NewHandler creates a new auth handler.
func NewHandler(cfg *config.Config) *Handler {
	h := &Handler{
		redirectPath: "/auth/callback",
		sessions:     make(map[string]*Session),
	}

	if !cfg.AuthConfigured() {
		h.configured = false
		return h
	}

	h.clientID = cfg.AzureClientID
	h.clientSecret = cfg.AzureClientSecret
	h.tenantID = cfg.AzureTenantID
	h.authority = fmt.Sprintf("https://login.microsoftonline.com/%s", cfg.AzureTenantID)
	h.configured = true

	return h
}

// IsConfigured returns whether Azure AD auth is enabled.
func (h *Handler) IsConfigured() bool {
	return h.configured
}

// RequireAuth is middleware that checks for authentication.
// If auth is not configured, it passes through (bypass mode).
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.configured {
			next.ServeHTTP(w, r)
			return
		}

		sess := h.getSession(r)
		if sess == nil {
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// LoginHandler initiates the OAuth2 login flow.
func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if !h.configured {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	redirectURI := getBaseURL(r) + h.redirectPath
	scope := "openid profile email User.Read"

	authURL := fmt.Sprintf("%s/oauth2/v2.0/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&response_mode=query",
		h.authority, h.clientID, redirectURI, scope)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// CallbackHandler handles the OAuth2 callback.
func (h *Handler) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	if !h.configured {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		http.Error(w, "Login error: "+r.URL.Query().Get("error_description"), http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
		return
	}

	// Exchange code for token
	redirectURI := getBaseURL(r) + h.redirectPath
	tokenURL := fmt.Sprintf("%s/oauth2/v2.0/token", h.authority)

	resp, err := http.PostForm(tokenURL, map[string][]string{
		"client_id":     {h.clientID},
		"client_secret": {h.clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
		"scope":         {"openid profile email User.Read"},
	})
	if err != nil {
		http.Error(w, "Token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var tokenResult struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResult); err != nil {
		http.Error(w, "Failed to decode token", http.StatusInternalServerError)
		return
	}
	if tokenResult.Error != "" {
		http.Error(w, "Token error: "+tokenResult.ErrorDesc, http.StatusBadRequest)
		return
	}

	// Get user info from Graph API
	userReq, _ := http.NewRequest("GET", "https://graph.microsoft.com/v1.0/me", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenResult.AccessToken)

	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}
	defer userResp.Body.Close()

	var me struct {
		DisplayName       string `json:"displayName"`
		UserPrincipalName string `json:"userPrincipalName"`
		Mail              string `json:"mail"`
	}
	json.NewDecoder(userResp.Body).Decode(&me)

	email := me.Mail
	if email == "" {
		email = me.UserPrincipalName
	}

	// Create session
	sessionID := generateSessionID()
	h.mu.Lock()
	h.sessions[sessionID] = &Session{
		UserName:  me.DisplayName,
		UserEmail: email,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	h.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

// LogoutHandler clears the session and redirects.
func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_id"); err == nil {
		h.mu.Lock()
		delete(h.sessions, cookie.Value)
		h.mu.Unlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "session_id",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	if !h.configured {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	logoutURL := fmt.Sprintf("%s/oauth2/v2.0/logout?post_logout_redirect_uri=%s", h.authority, getBaseURL(r))
	http.Redirect(w, r, logoutURL, http.StatusFound)
}

// UserInfoHandler returns current user info as JSON.
func (h *Handler) UserInfoHandler(w http.ResponseWriter, r *http.Request) {
	sess := h.getSession(r)
	w.Header().Set("Content-Type", "application/json")

	if sess != nil {
		json.NewEncoder(w).Encode(UserInfo{
			Name:          sess.UserName,
			Email:         sess.UserEmail,
			Authenticated: true,
		})
		return
	}

	json.NewEncoder(w).Encode(UserInfo{Authenticated: !h.configured})
}

// GetCurrentUser returns the current user info or nil.
func (h *Handler) GetCurrentUser(r *http.Request) *UserInfo {
	if !h.configured {
		return &UserInfo{Authenticated: true}
	}
	sess := h.getSession(r)
	if sess == nil {
		return nil
	}
	return &UserInfo{
		Name:          sess.UserName,
		Email:         sess.UserEmail,
		Authenticated: true,
	}
}

func (h *Handler) getSession(r *http.Request) *Session {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return nil
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	sess, ok := h.sessions[cookie.Value]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil
	}
	return sess
}

func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
