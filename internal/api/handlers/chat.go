package handlers

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/internal/api/httpx"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

type ChatHandler struct {
	deps    Deps
	logger  *slog.Logger
	mu      sync.RWMutex
	history []Message
}

func NewChatHandler(deps Deps, logger *slog.Logger) *ChatHandler {
	return &ChatHandler{deps: deps, logger: logger, history: make([]Message, 0)}
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type sendRequest struct {
	Message   string `json:"message"`
	AgentID   string `json:"agent_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type sendResponse struct {
	Response  string    `json:"response"`
	AgentID   string    `json:"agent_id"`
	SessionID string    `json:"session_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	LatencyMs int64     `json:"latency_ms"`
}

func (h *ChatHandler) Send(w http.ResponseWriter, r *http.Request) {
	h.send(w, r, "default")
}

func (h *ChatHandler) SendToAgent(w http.ResponseWriter, r *http.Request) {
	h.send(w, r, r.PathValue("agent_id"))
}

func (h *ChatHandler) send(w http.ResponseWriter, r *http.Request, agentID string) {
	var req sendRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.Message == "" {
		httpx.Error(w, http.StatusBadRequest, "message is required")
		return
	}

	h.save("user", req.Message)
	start := time.Now()

	replyCh := make(chan core.Event, 1)
	h.deps.Bus <- core.Event{
		Type:          core.AgentExecute,
		CorrelationID: uuid.NewString(),
		AgentID:       agentID,
		ReplyTo:       replyCh,
		Payload: core.AgentExecutePayload{
			Message: req.Message,
		},
	}

	select {
	case result := <-replyCh:
		response := result.Payload.(string)
		h.save("assistant", response)
		httpx.JSON(w, http.StatusOK, sendResponse{
			Response:  response,
			AgentID:   agentID,
			SessionID: req.SessionID,
			Timestamp: time.Now(),
			LatencyMs: time.Since(start).Milliseconds(),
		})
	case <-r.Context().Done():
		httpx.Error(w, http.StatusGatewayTimeout, "request timed out")
	}
}

// History handles GET /v1/chat/history.
func (h *ChatHandler) History(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	snapshot := make([]Message, len(h.history))
	copy(snapshot, h.history)
	h.mu.RUnlock()

	httpx.JSON(w, http.StatusOK, map[string]any{
		"history": snapshot,
		"count":   len(snapshot),
	})
}

// ClearHistory handles DELETE /v1/chat/history.
func (h *ChatHandler) ClearHistory(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.history = make([]Message, 0)
	h.mu.Unlock()

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

func (h *ChatHandler) save(role, content string) {
	h.mu.Lock()
	h.history = append(h.history, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	h.mu.Unlock()
}
