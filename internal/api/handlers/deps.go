package handlers

import (
	"context"
	"net/http"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// Deps holds every dependency that handlers might need.
// Add a new field here when you add a new package (memory, skills, etc).
// Handlers only take what they need from this struct.
//
// This is the idiomatic Go alternative to a global service locator.
type Deps struct {
	Bus       chan<- core.Event
	Version   string
	Kernel    KernelReader
	Store     storage.Storage
	WSHandler http.HandlerFunc // WebSocket handler from custom channels like OAChannel

	// add these as you build them:
	// Skills    SkillsReader    — for /v1/skills
	// Memory    MemoryReader    — for /v1/memory
	// Agents    AgentsReader    — for /v1/agents
}

// Here's an example of how we can wire the skills reader and  kernel:
// // handlers/deps.go
// type SkillsReader interface {
//     ListSkills() []core.SkillInfo
//     GetSkill(name string) (core.SkillInfo, bool)
// }
//
// // pkg/kernel/reader.go
// func (k *Kernel) ListSkills() []core.SkillInfo { ... }
// func (k *Kernel) GetSkill(name string) (core.SkillInfo, bool) { ... }
//
// // start.go
// deps := handlers.Deps{
//     Kernel: k,
//     Skills: k, // same k, different interface
// }

// ─── Kernel interface ─────────────────────────────────────────────────────────

// KernelReader is the interface the API layer needs from the kernel.
// Defined here (not in pkg/kernel) so the API layer never imports the kernel
// package — avoiding a potential import cycle and keeping the boundary clean.
//
// pkg/kernel.Kernel implements this interface; it is passed in from cmd/server.
type KernelReader interface {
	// Agents returns a runtime snapshot of every registered agent.
	AgentsStatus() []core.AgentStatus

	// IsHealthy returns false if the kernel context has been cancelled.
	IsHealthy() bool
}

// ─── Agent interface ──────────────────────────────────────────────────────────

// AgentExecutor is the interface handlers need from a single agent.
// Keeping it small and defined here avoids importing pkg/agents into the API layer.
type AgentExecutor interface {
	Execute(ctx context.Context, message string) (string, error)
	ID() string
}
