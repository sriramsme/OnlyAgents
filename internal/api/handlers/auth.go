package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/sriramsme/OnlyAgents/internal/api/httpx"
	"github.com/sriramsme/OnlyAgents/internal/auth"
)

// AuthHandler handles all /auth/* routes.
type AuthHandler struct {
	auth   *auth.Auth
	logger *slog.Logger
}

func NewAuthHandler(a *auth.Auth, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{auth: a, logger: logger}
}

// ─── POST /auth/login ─────────────────────────────────────────────────────────

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req auth.LoginRequest
	if !httpx.Decode(w, r, &req) {
		return
	}

	if req.Username == "" || req.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "username and password required")
		return
	}

	result, err := h.auth.Login(r, req)
	if err != nil {
		switch err {
		case auth.ErrRateLimited:
			httpx.Error(w, http.StatusTooManyRequests, err.Error())
		case auth.ErrBadCredentials:
			httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		default:
			httpx.Error(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	secure := isSecureRequest(r)
	http.SetCookie(w, auth.NewSessionCookie(result.Token, secure))

	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "logged in",
	})
}

// ─── POST /auth/logout ────────────────────────────────────────────────────────

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.auth.Logout(r)
	http.SetCookie(w, auth.ClearSessionCookie())

	w.Header().Set("Content-Type", "application/json")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "logged out",
	})
}

// ─── GET /auth/me ─────────────────────────────────────────────────────────────
// This route is protected by Authed() middleware — if we reach here, auth passed.

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"authenticated": true,
		"session_count": h.auth.SessionCount(),
	})
}

// ─── POST /auth/password ──────────────────────────────────────────────────────
// Protected by Authed() middleware.

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req auth.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.auth.ChangePassword(req); err != nil {
		switch err {
		case auth.ErrBadCredentials:
			httpx.Error(w, http.StatusUnauthorized, "current password is incorrect")
		case auth.ErrPasswordTooShort:
			httpx.Error(w, http.StatusBadRequest, err.Error())
		default:
			httpx.Error(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// Sessions invalidated — clear cookie on this device too
	http.SetCookie(w, auth.ClearSessionCookie())

	w.Header().Set("Content-Type", "application/json")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "password changed — please log in again",
	})
}

// ─── helpers ──────────────────────────────────────────────────────────────────
func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}
