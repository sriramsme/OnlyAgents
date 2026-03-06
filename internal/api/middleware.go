package api

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/config"
)

// Middleware holds the server config needed to build middleware chains
type Middleware struct {
	cfg    config.ServerConfig
	logger *slog.Logger
}

func NewMiddleware(cfg config.ServerConfig, logger *slog.Logger) *Middleware {
	return &Middleware{cfg: cfg, logger: logger}
}

type handlerFunc = http.HandlerFunc
type middlewareFn func(handlerFunc) handlerFunc

// Open returns a chain with no auth: logging → recovery → cors
func (m *Middleware) Open() middlewareFn {
	return chain(m.logging, m.recovery)
}

// Authed returns a chain with auth: logging → recovery → cors → auth
func (m *Middleware) Authed() middlewareFn {
	return chain(m.logging, m.recovery, m.auth)
}

// chain composes middleware so the first in the list runs first
func chain(middlewares ...middlewareFn) middlewareFn {
	return func(next handlerFunc) handlerFunc {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// logging logs method, path, status, latency
func (m *Middleware) logging(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next(rw, r)
		m.logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"ms", time.Since(start).Milliseconds(),
		)
	}
}

// recovery catches panics and returns 500
func (m *Middleware) recovery(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				m.logger.Error("panic", "err", err, "stack", string(debug.Stack()))
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

// corsGlobal wraps the entire mux so OPTIONS preflights are handled
// before Go's method router rejects them with 405.
func (m *Middleware) corsGlobal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// auth validates the API key — skipped entirely if no key is configured
func (m *Middleware) auth(next handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.logger.Debug("auth", "path", r.URL.Path, "key", m.cfg.APIKeyVault)
		if m.cfg.APIKeyVault == "" {
			// No key configured → open access (local dev / no-auth mode)
			next(w, r)
			return
		}

		// Extract key from one of three places (in priority order):
		//   1. X-API-Key header           — standard API clients
		//   2. Authorization: Bearer ...  — standard bearer auth
		//   3. ?key= query param          — SSE clients (EventSource limitation)
		key := r.Header.Get("X-API-Key")

		if key == "" {
			if auth := r.Header.Get("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
				key = auth[7:]
			}
		}

		if key == "" {
			key = r.URL.Query().Get("key")
		}

		// TODO: replace m.cfg.APIKeyVault string comparison with
		//       vault.GetAPIKey(m.cfg.APIKeyVault) lookup once vault is wired.
		if key != m.cfg.APIKeyVault {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// statusWriter captures the status code written by a handler
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// forwards Flush to the underlying writer if it supports it
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
