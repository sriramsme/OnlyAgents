package api

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/auth"
	"github.com/sriramsme/OnlyAgents/internal/config"
)

type handlerFunc = func(http.ResponseWriter, *http.Request)
type middlewareFn func(handlerFunc) handlerFunc

// Middleware holds the server config needed to build middleware chains
type Middleware struct {
	cfg    config.ServerConfig
	auth   *auth.Auth
	logger *slog.Logger
}

func NewMiddleware(cfg config.ServerConfig, a *auth.Auth, logger *slog.Logger) *Middleware {
	return &Middleware{cfg: cfg, auth: a, logger: logger}
}

// corsGlobal wraps the entire mux. OPTIONS preflights are handled here
// before Go's method router can return 405.
func (m *Middleware) corsGlobal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Open applies logging + recovery. No auth check.
func (m *Middleware) Open() middlewareFn {
	return chain(m.logging, m.recovery)
}

// Authed applies logging + recovery + auth.
// Accepts session cookie (browser) OR API key header/query (headless clients).
// Returns 401 if neither is valid.
func (m *Middleware) Authed() middlewareFn {
	return chain(m.logging, m.recovery, m.authCheck)
}

func (m *Middleware) authCheck(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !m.auth.ValidateRequest(r, m.cfg.APIKeyVault) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// Flush forwards to the underlying writer — required for SSE.
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (m *Middleware) logging(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next(sw, r)
		m.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start),
		)
	}
}

func (m *Middleware) recovery(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				m.logger.Error("panic recovered", "error", rec)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

func chain(middlewares ...func(handlerFunc) handlerFunc) middlewareFn {
	return func(final handlerFunc) handlerFunc {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}

// statusWriter captures the status code written by a handler
type statusWriter struct {
	http.ResponseWriter
	status int
}

// Unwrap lets websocket.Accept (and other hijack-needing code) reach
// the underlying ResponseWriter through our wrapper.
func (sw *statusWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}

// Hijack forwards to the underlying writer — required for WebSocket upgrades.
func (sw *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := sw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
	}
	return h.Hijack()
}
