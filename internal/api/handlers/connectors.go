// internal/api/handlers/connectors.go
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/sriramsme/OnlyAgents/internal/api/httpx"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
)

type ConnectorsHandler struct {
	logger *slog.Logger
}

func NewConnectorsHandler(logger *slog.Logger) *ConnectorsHandler {
	return &ConnectorsHandler{logger: logger}
}

func (h *ConnectorsHandler) List(w http.ResponseWriter, r *http.Request) {
	cfgs, err := connectors.LoadAllConfigs("")
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
	httpx.JSON(w, http.StatusOK, map[string]any{"connectors": out})
}

func (h *ConnectorsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	cfgs, err := connectors.LoadAllConfigs("")
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

	httpx.Error(w, http.StatusNotFound, "connector not found")
}
