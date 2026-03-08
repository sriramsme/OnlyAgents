package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

const (
	SessionCookieName = "onlyagents_session"
	sessionTokenBytes = 32 // 256 bits of entropy
)

// RateLimiter is satisfied by golang.org/x/time/rate.Limiter wrapped per-IP.
// Defined here as an interface so the handler layer can swap implementations.
type LoginAttemptLimiter interface {
	// Allow returns true if the attempt should be permitted.
	Allow(ip string) bool
}

// Auth is the central auth coordinator. Holds the session store and
// exposes the operations needed by HTTP handlers and middleware.
type Auth struct {
	dataDir  string
	sessions *SessionStore
	limiter  LoginAttemptLimiter
	done     chan struct{}
}

// New creates an Auth instance. Call Start() to begin the session cleanup goroutine.
func New(dataDir string, limiter LoginAttemptLimiter) *Auth {
	return &Auth{
		dataDir:  dataDir,
		sessions: newSessionStore(),
		limiter:  limiter,
		done:     make(chan struct{}),
	}
}

// Start begins background session cleanup. Call Stop() on shutdown.
func (a *Auth) Start() {
	a.sessions.start(a.done)
}

// Stop shuts down background goroutines.
func (a *Auth) Stop() {
	close(a.done)
}

// ─── Login ────────────────────────────────────────────────────────────────────

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResult struct {
	Token string // set as cookie by the handler
}

// Login verifies credentials and creates a session.
// Returns ErrRateLimited, ErrBadCredentials, or nil.
func (a *Auth) Login(r *http.Request, req LoginRequest) (LoginResult, error) {
	ip := clientIP(r)

	if !a.limiter.Allow(ip) {
		return LoginResult{}, ErrRateLimited
	}

	// Username check — constant time to prevent user enumeration
	expectedUser, err := GetUsername(a.dataDir)
	if err != nil {
		return LoginResult{}, fmt.Errorf("reading credentials: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(req.Username), []byte(expectedUser)) == 0 {
		return LoginResult{}, ErrBadCredentials
	}

	// Password check — bcrypt is timing-safe by design
	if err := VerifyPassword(a.dataDir, req.Password); err != nil {
		return LoginResult{}, ErrBadCredentials
	}

	token, err := generateSessionToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("generating session token: %w", err)
	}

	a.sessions.create(token)
	return LoginResult{Token: token}, nil
}

// ─── Session validation ───────────────────────────────────────────────────────

// ValidateRequest checks the session cookie on an incoming request.
// Returns true if the session is valid.
// Also accepts API key via header/query for non-browser clients (headless API).
func (a *Auth) ValidateRequest(r *http.Request, apiKeyVault string) bool {
	// Check session cookie first (browser flow)
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if a.sessions.validate(cookie.Value) {
			return true
		}
	}

	// Fall back to API key (headless / programmatic clients)
	if apiKeyVault != "" {
		key := extractAPIKey(r)
		if key != "" && subtle.ConstantTimeCompare([]byte(key), []byte(apiKeyVault)) == 1 {
			return true
		}
	}

	return false
}

// ─── Logout ───────────────────────────────────────────────────────────────────

// Logout removes the session token from the store.
func (a *Auth) Logout(r *http.Request) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return
	}
	a.sessions.delete(cookie.Value)
}

// ─── Password change ──────────────────────────────────────────────────────────

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword verifies the current password, updates the hash, and
// invalidates all active sessions (forces re-login on all devices).
func (a *Auth) ChangePassword(req ChangePasswordRequest) error {
	if err := VerifyPassword(a.dataDir, req.CurrentPassword); err != nil {
		return ErrBadCredentials
	}

	if len(req.NewPassword) < 8 {
		return ErrPasswordTooShort
	}

	if err := UpdatePassword(a.dataDir, req.NewPassword); err != nil {
		return fmt.Errorf("updating password: %w", err)
	}

	// Invalidate all sessions — user must re-login everywhere
	a.sessions.invalidateAll()
	return nil
}

// SessionCount returns the number of active sessions (useful for /auth/me).
func (a *Auth) SessionCount() int {
	return a.sessions.count()
}

// ─── Cookie helpers ───────────────────────────────────────────────────────────

// NewSessionCookie returns a configured session cookie.
// secure=true when the request came in over HTTPS.
func NewSessionCookie(token string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,                    // JS cannot read — XSS protection
		Secure:   secure,                  // HTTPS only when behind TLS
		SameSite: http.SameSiteStrictMode, // CSRF protection
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	}
}

// ClearSessionCookie returns an expired cookie that clears the session.
func ClearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	}
}

// ─── Errors ───────────────────────────────────────────────────────────────────

type authError string

func (e authError) Error() string { return string(e) }

const (
	ErrBadCredentials   = authError("invalid username or password")
	ErrRateLimited      = authError("too many attempts, try again later")
	ErrPasswordTooShort = authError("password must be at least 8 characters")
)

// ─── Internal helpers ─────────────────────────────────────────────────────────

func generateSessionToken() (string, error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func clientIP(r *http.Request) string {
	// Respect X-Forwarded-For when behind a reverse proxy
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}

func extractAPIKey(r *http.Request) string {
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}
	if auth := r.Header.Get("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return r.URL.Query().Get("key")
}
