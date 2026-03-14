package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	SessionCookieName = "onlyagents_session"
	sessionTokenBytes = 32 // 256 bits entropy
	maxSessions       = 1000
)

// LoginAttemptLimiter is implemented by an IP-based rate limiter.
type LoginAttemptLimiter interface {
	Allow(ip string) bool
}

// Auth coordinates login, sessions, and auth validation.
type Auth struct {
	dataDir  string
	sessions *SessionStore
	limiter  LoginAttemptLimiter
	done     chan struct{}
}

// New creates a new Auth instance.
func New(dataDir string, limiter LoginAttemptLimiter) *Auth {
	return &Auth{
		dataDir:  dataDir,
		sessions: newSessionStore(),
		limiter:  limiter,
		done:     make(chan struct{}),
	}
}

// Start begins background session cleanup.
func (a *Auth) Start() {
	a.sessions.start(a.done)
}

// Stop stops background workers.
func (a *Auth) Stop() {
	close(a.done)
}

// ─────────────────────────────────────────
// Login
// ─────────────────────────────────────────

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResult struct {
	Token string
}

// Login verifies credentials and creates a session.
func (a *Auth) Login(r *http.Request, req LoginRequest) (LoginResult, error) {
	ip := clientIP(r)

	if !a.limiter.Allow(ip) {
		return LoginResult{}, ErrRateLimited
	}

	if err := VerifyPassword(a.dataDir, req.Username, req.Password); err != nil {
		return LoginResult{}, ErrBadCredentials
	}

	// Prevent session store exhaustion
	if a.sessions.count() >= maxSessions {
		return LoginResult{}, fmt.Errorf("too many active sessions")
	}

	token, err := generateSessionToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate session: %w", err)
	}

	a.sessions.create(token)

	return LoginResult{Token: token}, nil
}

// ─────────────────────────────────────────
// Session validation
// ─────────────────────────────────────────

// ValidateRequest checks session cookie or API key.
func (a *Auth) ValidateRequest(r *http.Request, apiKey string) bool {
	// Session cookie (browser)
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if a.sessions.validate(cookie.Value) {
			return true
		}
	}

	// API key fallback
	if apiKey != "" {
		key := extractAPIKey(r)
		if key != "" &&
			subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) == 1 {
			return true
		}
	}

	return false
}

// ─────────────────────────────────────────
// Logout
// ─────────────────────────────────────────

func (a *Auth) Logout(r *http.Request) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return
	}

	a.sessions.delete(cookie.Value)
}

// ─────────────────────────────────────────
// Password change
// ─────────────────────────────────────────

type ChangePasswordRequest struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (a *Auth) ChangePassword(req ChangePasswordRequest) error {
	if len(req.NewPassword) < 8 {
		return ErrPasswordTooShort
	}

	if err := ChangePassword(
		a.dataDir,
		req.Username,
		req.CurrentPassword,
		req.NewPassword,
	); err != nil {
		return err
	}

	// Force re-login everywhere
	a.sessions.invalidateAll()

	return nil
}

// SessionCount returns the active session count.
func (a *Auth) SessionCount() int {
	return a.sessions.count()
}

// ─────────────────────────────────────────
// Cookie helpers
// ─────────────────────────────────────────

func NewSessionCookie(token string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	}
}

func ClearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	}
}

// ─────────────────────────────────────────
// Errors
// ─────────────────────────────────────────

type authError string

func (e authError) Error() string { return string(e) }

const (
	ErrBadCredentials   = authError("invalid username or password")
	ErrRateLimited      = authError("too many attempts, try again later")
	ErrPasswordTooShort = authError("password must be at least 8 characters")
)

// ─────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────

func generateSessionToken() (string, error) {
	b := make([]byte, sessionTokenBytes)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}

// Extract client IP safely when behind proxies.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip := strings.Split(xff, ",")[0]
		return strings.TrimSpace(ip)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

func extractAPIKey(r *http.Request) string {
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}

	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}

	return r.URL.Query().Get("key")
}
