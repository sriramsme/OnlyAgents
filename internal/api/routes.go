package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/api/handlers"
	"github.com/sriramsme/OnlyAgents/internal/auth"
	"github.com/sriramsme/OnlyAgents/ui"
)

// registerRoutes is the single source of truth for the entire API surface.
// To add a new endpoint:
//  1. Add a handler method in the appropriate handlers/ file
//  2. Add one line here
//  3. That's it
//
// Routes are grouped by domain. Open routes at top, authed routes below.
func registerRoutes(mux *http.ServeMux, mid *Middleware, deps handlers.Deps, a *auth.Auth, logger *slog.Logger) {
	// Instantiate handler groups
	healthH := handlers.NewHealthHandler(deps, logger)
	authH := handlers.NewAuthHandler(a, logger)
	sessionsH := handlers.NewSessionsHandler(deps, logger)
	agentsH := handlers.NewAgentsHandler(logger)
	skillsH := handlers.NewSkillsHandler(logger)
	connectorsH := handlers.NewConnectorsHandler(logger)
	channelsH := handlers.NewChannelsHandler(logger)

	open := mid.Open()     // logging + recovery + cors
	authed := mid.Authed() // + auth

	// ── System ──────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /health", open(healthH.Health))
	mux.HandleFunc("GET /version", open(healthH.Version))

	// ── Auth ─────────────────────────────────────────────────────────────────
	mux.HandleFunc("POST /auth/login", open(authH.Login))
	mux.HandleFunc("POST /auth/logout", open(authH.Logout))
	mux.HandleFunc("GET /auth/me", authed(authH.Me))
	mux.HandleFunc("POST /auth/password", authed(authH.ChangePassword))

	// ── WebSocket — single connection for chat + war room + notifications ─────
	// Query params: ?session_id=<uuid>&agent_id=<id>
	// session_id: omit to start a new session, pass existing to resume
	// agent_id:   defaults to "executive"
	if deps.WSHandler != nil {
		mux.HandleFunc("GET /v1/ws", authed(func(w http.ResponseWriter, r *http.Request) {
			rc := http.NewResponseController(w)
			if err := rc.SetWriteDeadline(time.Time{}); err != nil {
				logger.Error("failed to set write deadline", "error", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			deps.WSHandler(w, r)
		}))
		logger.Info("registered OAChannel handler", "path", "/v1/ws")
	}

	// ── Chat Sessions ───────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /v1/sessions", authed(sessionsH.List))
	mux.HandleFunc("GET /v1/sessions/{id}/history", authed(sessionsH.History))
	mux.HandleFunc("DELETE /v1/sessions/{id}", authed(sessionsH.End))

	// ── Agents ───────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /v1/agents", authed(agentsH.List))
	mux.HandleFunc("GET /v1/agents/{id}", authed(agentsH.Get))

	// ── Skills ────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /v1/skills", authed(skillsH.List))
	mux.HandleFunc("GET /v1/skills/{id}", authed(skillsH.Get))

	// ── Connectors ────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /v1/connectors", authed(connectorsH.List))
	mux.HandleFunc("GET /v1/connectors/{id}", authed(connectorsH.Get))

	// ── Channels ────────────────────────────────────────────────────────
	mux.HandleFunc("GET /v1/channels", authed(channelsH.List))
	mux.HandleFunc("GET /v1/channels/{id}", authed(channelsH.Get))

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

	// ── A2A (Phase 7) ─────────────────────────────────────────────────────────
	// mux.HandleFunc("GET  /v1/a2a/connections",          authed(a2a.ListConnections))
	// mux.HandleFunc("POST /v1/a2a/connections/request",  authed(a2a.RequestConnection))
	// mux.HandleFunc("PUT  /v1/a2a/connections/{id}",     authed(a2a.UpdateConnection))
	// mux.HandleFunc("GET  /v1/a2a/messages",             authed(a2a.ListMessages))

	// Static assets — Vite outputs hashed files here e.g. /assets/index-abc123.js

	// Get a sub-FS rooted at dist/ so paths match request URLs
	distFS, err := fs.Sub(ui.WebFS, "dist")
	if err != nil {
		logger.Error("failed to create sub filesystem", "error", err)
	}
	fileServer := http.FileServerFS(distFS)

	mux.HandleFunc("GET /assets/", open(func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
	}))

	// SPA catch-all — anything that isn't an API route gets index.html
	// React Router handles the path client-side
	mux.HandleFunc("GET /", open(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") ||
			strings.HasPrefix(r.URL.Path, "/auth/") ||
			r.URL.Path == "/health" {
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, distFS, "index.html")
	}))
}
