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
	events := handlers.NewEventsHandler(deps, logger)

	// agent   := handlers.NewAgentHandler(deps, logger)     // uncomment when ready
	// skills  := handlers.NewSkillsHandler(deps, logger)    // uncomment when ready
	// memory  := handlers.NewMemoryHandler(deps, logger)    // uncomment when ready

	open := mid.Open()     // logging + recovery + cors
	authed := mid.Authed() // + auth

	// ── System ──────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /health", authed(health.Health))
	mux.HandleFunc("GET /version", open(health.Version))

	// ── Real-time event stream (war room backbone) ───────────────────────────
	// SSE — clients connect once and receive a continuous stream of UIEvents.
	// Kernel fans UIBus events to all connected clients via EventsHandler.
	mux.HandleFunc("GET /v1/events", authed(events.Stream))

	// ── Chat ─────────────────────────────────────────────────────────────────
	mux.HandleFunc("POST   /v1/chat", authed(chat.Send))
	mux.HandleFunc("GET    /v1/chat/history", authed(chat.History))
	mux.HandleFunc("DELETE /v1/chat/history", authed(chat.ClearHistory))
	mux.HandleFunc("POST   /v1/agents/{agent_id}/chat", authed(chat.SendToAgent))

	// ── Agents ───────────────────────────────────────────────────────────────
	// mux.HandleFunc("GET /v1/agents",      authed(agent.List))
	// mux.HandleFunc("GET /v1/agents/{id}", authed(agent.Get))

	// ── Memory ───────────────────────────────────────────────────────────────
	// mux.HandleFunc("GET /v1/memory/facts",         authed(memory.Facts))
	// mux.HandleFunc("GET /v1/memory/daily",         authed(memory.Daily))
	// mux.HandleFunc("GET /v1/memory/weekly",        authed(memory.Weekly))
	// mux.HandleFunc("GET /v1/memory/conversations", authed(memory.Conversations))

	// ── Calendar ─────────────────────────────────────────────────────────────
	// mux.HandleFunc("GET    /v1/calendar",      authed(productivity.ListEvents))
	// mux.HandleFunc("POST   /v1/calendar",      authed(productivity.CreateEvent))
	// mux.HandleFunc("PUT    /v1/calendar/{id}", authed(productivity.UpdateEvent))
	// mux.HandleFunc("DELETE /v1/calendar/{id}", authed(productivity.DeleteEvent))

	// ── Tasks ─────────────────────────────────────────────────────────────────
	// mux.HandleFunc("GET    /v1/tasks",               authed(productivity.ListTasks))
	// mux.HandleFunc("POST   /v1/tasks",               authed(productivity.CreateTask))
	// mux.HandleFunc("PUT    /v1/tasks/{id}",          authed(productivity.UpdateTask))
	// mux.HandleFunc("DELETE /v1/tasks/{id}",          authed(productivity.DeleteTask))
	// mux.HandleFunc("GET    /v1/tasks/projects",      authed(productivity.ListProjects))
	// mux.HandleFunc("POST   /v1/tasks/projects",      authed(productivity.CreateProject))
	// mux.HandleFunc("DELETE /v1/tasks/projects/{id}", authed(productivity.DeleteProject))

	// ── Notes ─────────────────────────────────────────────────────────────────
	// mux.HandleFunc("GET    /v1/notes",      authed(productivity.ListNotes))
	// mux.HandleFunc("POST   /v1/notes",      authed(productivity.CreateNote))
	// mux.HandleFunc("PUT    /v1/notes/{id}", authed(productivity.UpdateNote))
	// mux.HandleFunc("DELETE /v1/notes/{id}", authed(productivity.DeleteNote))

	// ── Reminders ─────────────────────────────────────────────────────────────
	// mux.HandleFunc("GET    /v1/reminders",      authed(productivity.ListReminders))
	// mux.HandleFunc("POST   /v1/reminders",      authed(productivity.CreateReminder))
	// mux.HandleFunc("PUT    /v1/reminders/{id}", authed(productivity.UpdateReminder))
	// mux.HandleFunc("DELETE /v1/reminders/{id}", authed(productivity.DeleteReminder))

	// ── Config ────────────────────────────────────────────────────────────────
	// mux.HandleFunc("GET /v1/config/agents",          authed(config.ListAgents))
	// mux.HandleFunc("GET /v1/config/agents/{id}",     authed(config.GetAgent))
	// mux.HandleFunc("PUT /v1/config/agents/{id}",     authed(config.WriteAgent))
	// mux.HandleFunc("GET /v1/config/channels",        authed(config.ListChannels))
	// mux.HandleFunc("GET /v1/config/channels/{id}",   authed(config.GetChannel))
	// mux.HandleFunc("PUT /v1/config/channels/{id}",   authed(config.WriteChannel))
	// mux.HandleFunc("GET /v1/config/connectors",      authed(config.ListConnectors))
	// mux.HandleFunc("GET /v1/config/connectors/{id}", authed(config.GetConnector))
	// mux.HandleFunc("PUT /v1/config/connectors/{id}", authed(config.WriteConnector))

	// ── A2A (Phase 7) ─────────────────────────────────────────────────────────
	// mux.HandleFunc("GET  /v1/a2a/connections",          authed(a2a.ListConnections))
	// mux.HandleFunc("POST /v1/a2a/connections/request",  authed(a2a.RequestConnection))
	// mux.HandleFunc("PUT  /v1/a2a/connections/{id}",     authed(a2a.UpdateConnection))
	// mux.HandleFunc("GET  /v1/a2a/messages",             authed(a2a.ListMessages))
}
