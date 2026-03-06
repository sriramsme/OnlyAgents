package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// EventsHandler handles GET /v1/events — the SSE stream that powers the war room.
type EventsHandler struct {
	deps   Deps
	logger *slog.Logger
}

func NewEventsHandler(deps Deps, logger *slog.Logger) *EventsHandler {
	return &EventsHandler{deps: deps, logger: logger}
}

// Stream handles GET /v1/events.
//
// Each connected browser gets its own goroutine here. The kernel fans out
// UIEvents to all subscriber channels; we forward them as SSE frames.
// A 15s heartbeat keeps the connection alive through proxies and load balancers.
func (h *EventsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	// SSE headers — must be set before any write
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	clientID := uuid.NewString()
	ch, unsubscribe := h.deps.Kernel.Subscribe(clientID)
	defer unsubscribe()

	h.logger.Info("sse client connected", "client_id", clientID)
	defer h.logger.Info("sse client disconnected", "client_id", clientID)

	// Send a snapshot of current agent states immediately on connect so the
	// war room renders agents without waiting for the next state change.
	h.sendSnapshot(w, flusher)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case evt := <-ch:
			if err := writeEvent(w, evt); err != nil {
				h.logger.Debug("sse write error", "client_id", clientID, "error", err)
				return
			}
			flusher.Flush()

		case <-ticker.C:
			heartbeat := core.UIEvent{
				Type:      core.UIEventHeartbeat,
				Timestamp: time.Now(),
			}
			if err := writeEvent(w, heartbeat); err != nil {
				return
			}
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

// sendSnapshot pushes the current state of every agent as snapshot.agent events.
// The frontend initialises the war room from these before the live stream begins.
func (h *EventsHandler) sendSnapshot(w http.ResponseWriter, flusher http.Flusher) {
	if h.deps.Kernel == nil {
		return
	}
	for _, status := range h.deps.Kernel.Agents() {
		evt := core.UIEvent{
			Type:      core.UIEventSnapshotAgent,
			Timestamp: time.Now(),
			AgentID:   status.ID,
			Payload:   status,
		}
		if err := writeEvent(w, evt); err != nil {
			return
		}
	}
	flusher.Flush()
}

// writeEvent serialises a UIEvent as an SSE data frame.
// Format: "data: <json>\n\n"
func writeEvent(w http.ResponseWriter, evt core.UIEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal ui event: %w", err)
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}
