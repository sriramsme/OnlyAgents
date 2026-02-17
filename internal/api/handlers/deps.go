package handlers

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/kernel"
)

// Deps holds every dependency that handlers might need.
// Add a new field here when you add a new package (memory, skills, etc).
// Handlers only take what they need from this struct.
//
// This is the idiomatic Go alternative to a global service locator.
type Deps struct {
	Agents  *kernel.AgentRegistry
	Version string

	// Uncomment as you build these packages:
	// Memory  MemoryReader
	// Skills  SkillRegistry
	// Vault   vault.Vault
}

// AgentExecutor is the interface handlers need from the kernel.
// Keeping it small and defined here avoids importing the kernel package
// into the api layer directly.
type AgentExecutor interface {
	Execute(ctx context.Context, message string) (string, error)
	ID() string
}

// MemoryReader will be used by the memory handler (add when ready)
// type MemoryReader interface {
// 	GetDailySummary(ctx context.Context, date time.Time) (string, error)
// 	GetFacts(ctx context.Context, entity string) ([]memory.Fact, error)
// 	GetRecentMessages(ctx context.Context, hours int) ([]memory.Message, error)
// }

// SkillRegistry will be used by the skills handler (add when ready)
// type SkillRegistry interface {
// 	List() []string
// 	Get(name string) skills.Skill
// }
