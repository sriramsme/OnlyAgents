// internal/api/handlers/sessions.go
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/sriramsme/OnlyAgents/internal/api/httpx"
	"github.com/sriramsme/OnlyAgents/pkg/conversation"
	"github.com/sriramsme/OnlyAgents/pkg/message"
)

type SessionsHandler struct {
	store  conversation.Store
	msgs   message.Store
	logger *slog.Logger
}

func NewSessionsHandler(deps Deps, logger *slog.Logger) *SessionsHandler {
	return &SessionsHandler{store: deps.Store, msgs: deps.Store, logger: logger}
}

// GET /v1/sessions
func (h *SessionsHandler) List(w http.ResponseWriter, r *http.Request) {
	convs, err := h.store.ListConversationsByChannel(r.Context(), "onlyagents", 50)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sessions": convs, "count": len(convs)})
}

// GET /v1/sessions/{id}/history
func (h *SessionsHandler) History(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	msgs, err := h.msgs.GetMessages(r.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"history": msgs, "count": len(msgs)})
}

// DELETE /v1/sessions/{id}
func (h *SessionsHandler) End(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.EndConversation(r.Context(), id, ""); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"status": "ended"})
}
