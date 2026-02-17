package handlers

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/api/httpx"
	"github.com/sriramsme/OnlyAgents/pkg/kernel"
)

// ChatHandler handles all /v1/chat endpoints.
//
// History is stored in-memory for now.
// TODO: swap historyStore for memory.MemoryManager when that package is ready.
// The handler interface won't change — just the storage backend.
type ChatHandler struct {
	deps   Deps
	logger *slog.Logger

	mu      sync.RWMutex
	history []Message
}

func NewChatHandler(deps Deps, logger *slog.Logger) *ChatHandler {
	return &ChatHandler{
		deps:    deps,
		logger:  logger,
		history: make([]Message, 0),
	}
}

// Message is one turn in the conversation
type Message struct {
	Role      string    `json:"role"` // "user" | "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// sendRequest is the POST /v1/chat body
type sendRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

// sendResponse is the POST /v1/chat reply
type sendResponse struct {
	Response  string    `json:"response"`
	AgentID   string    `json:"agent_id"`
	SessionID string    `json:"session_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	LatencyMs int64     `json:"latency_ms"`
}

// Send handles POST /v1/chat — always routes to executive
func (h *ChatHandler) Send(w http.ResponseWriter, r *http.Request) {
	agent, err := h.deps.Agents.Executive()
	if err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "no executive agent configured")
		return
	}
	h.execute(w, r, agent)
}

// SendToAgent handles POST /v1/agents/{agent_id}/chat — routes to specific agent
func (h *ChatHandler) SendToAgent(w http.ResponseWriter, r *http.Request) {
	agent, err := h.deps.Agents.Get(r.PathValue("agent_id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "agent not found")
		return
	}
	h.execute(w, r, agent)
}

// execute is the shared core — decode, run, save, respond
func (h *ChatHandler) execute(w http.ResponseWriter, r *http.Request, agent *kernel.Agent) {
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

	response, err := agent.Execute(r.Context(), req.Message)
	if err != nil {
		h.logger.Error("agent execution failed", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "agent execution failed")
		return
	}

	h.save("assistant", response)
	httpx.JSON(w, http.StatusOK, sendResponse{
		Response:  response,
		AgentID:   agent.ID(),
		SessionID: req.SessionID,
		Timestamp: time.Now(),
		LatencyMs: time.Since(start).Milliseconds(),
	})
}

// History handles GET /v1/chat/history
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

// ClearHistory handles DELETE /v1/chat/history
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
