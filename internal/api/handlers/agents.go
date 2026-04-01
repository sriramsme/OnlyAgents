// internal/httpx/handlers/agents.go
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/sriramsme/OnlyAgents/internal/api/httpx"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
)

type AgentsHandler struct {
	logger *slog.Logger
}

func NewAgentsHandler(logger *slog.Logger) *AgentsHandler {
	return &AgentsHandler{logger: logger}
}

func (h *AgentsHandler) List(w http.ResponseWriter, r *http.Request) {
	cfgs, err := agents.LoadAllConfigs("")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Return summaries only
	type summary struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
	}
	out := make([]summary, 0, len(cfgs))
	for _, c := range cfgs {
		out = append(out, summary{c.ID, c.Name, c.Description, c.Enabled})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"agents": out})
}

func (h *AgentsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfgs, err := agents.LoadAllConfigs("")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, c := range cfgs {
		if c.ID == id {
			httpx.JSON(w, http.StatusOK, c)
			return
		}
	}
	httpx.Error(w, http.StatusNotFound, "agent not found")
}
