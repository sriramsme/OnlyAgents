package api

import (
	"log/slog"
	"net/http"

	"github.com/sriramsme/OnlyAgents/internal/api/handlers"
)

// registerRoutes is the single source of truth for the entire API surface.
// To add a new endpoint:
//  1. Add a handler method in the appropriate handlers/ file
//  2. Add one line here
//  3. That's it
//
// Routes are grouped by domain. Open routes at top, authed routes below.
func registerRoutes(mux *http.ServeMux, mid *Middleware, deps handlers.Deps, logger *slog.Logger) {
	// Instantiate handler groups
	health := handlers.NewHealthHandler(deps, logger)
	chat := handlers.NewChatHandler(deps, logger)
	// agent   := handlers.NewAgentHandler(deps, logger)     // uncomment when ready
	// skills  := handlers.NewSkillsHandler(deps, logger)    // uncomment when ready
	// memory  := handlers.NewMemoryHandler(deps, logger)    // uncomment when ready

	open := mid.Open()     // logging + recovery + cors
	authed := mid.Authed() // + auth

	// ── System ──────────────────────────────────────────────
	mux.HandleFunc("GET /health", open(health.Health))
	mux.HandleFunc("GET /version", open(health.Version))

	// ── Chat ────────────────────────────────────────────────
	mux.HandleFunc("POST   /v1/chat", authed(chat.Send))
	mux.HandleFunc("GET    /v1/chat/history", authed(chat.History))
	mux.HandleFunc("DELETE /v1/chat/history", authed(chat.ClearHistory))

	// Agent-specific route — targets a named agent directly
	mux.HandleFunc("POST   /v1/agents/{agent_id}/chat", authed(chat.SendToAgent))

	// ── Agent ───────────────────────────────────────────────
	// mux.HandleFunc("GET  /v1/agent",             authed(agent.Info))
	// mux.HandleFunc("GET  /v1/agents",            authed(agent.List))
	// mux.HandleFunc("POST /v1/agents",            authed(agent.Create))
	// mux.HandleFunc("POST /v1/agents/{id}/chat",  authed(agent.Chat))

	// ── Skills ──────────────────────────────────────────────
	// mux.HandleFunc("GET  /v1/skills",            authed(skills.List))
	// mux.HandleFunc("POST /v1/skills/{name}/run", authed(skills.Run))

	// ── Memory ──────────────────────────────────────────────
	// mux.HandleFunc("GET  /v1/memory/summary",    authed(memory.Summary))
	// mux.HandleFunc("GET  /v1/memory/facts",      authed(memory.Facts))
	// mux.HandleFunc("GET  /v1/memory/daily",      authed(memory.Daily))
}
