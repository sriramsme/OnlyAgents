package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/api/httpx"
)

// HealthHandler handles system health and version endpoints.
// Stays small forever — these endpoints never get complex.
type HealthHandler struct {
	deps   Deps
	logger *slog.Logger
}

func NewHealthHandler(deps Deps, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{deps: deps, logger: logger}
}

// Health handles GET /health
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": time.Now(),
	})
}

// Version handles GET /version
func (h *HealthHandler) Version(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"version": h.deps.Version,
		"name":    "OnlyAgents",
	})
}
