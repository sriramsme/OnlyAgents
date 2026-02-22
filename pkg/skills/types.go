package skills

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// SkillType represents the type of skill
type SkillType string

const (
	// SkillTypeNative - Implemented in Go, uses Connectors
	SkillTypeNative SkillType = "native"

	// SkillTypeCLI - From SKILL.md files, uses bash
	SkillTypeCLI SkillType = "cli"

	// SkillTypeSystem - Built-in, no external dependencies
	SkillTypeSystem SkillType = "system"

	// SkillTypeExecution - Runs code in sandboxes
	SkillTypeExecution SkillType = "execution"
)

// Skill defines the interface for all skills
// Skills do NOT hold references to other components directly.
// They receive everything they need via SkillDeps at initialization.
type Skill interface {
	// Metadata
	Name() string
	Description() string
	Version() string
	Type() SkillType

	// RequiredCapabilities declares what connector capabilities this skill needs
	// e.g., []core.Capability{core.CapabilityEmail, core.CapabilityCalendar}
	RequiredCapabilities() []core.Capability

	// Tools returns the function definitions this skill exposes to the LLM
	Tools() []tools.ToolDef

	// Execute is called by kernel when LLM requests a tool call
	Execute(ctx context.Context, toolName string, params map[string]any) (any, error)

	// Initialize is called by kernel at startup, injecting dependencies
	Initialize(deps SkillDeps) error

	// Shutdown is called when kernel shuts down
	Shutdown(ctx context.Context) error
}

// SkillDeps is what kernel provides to each skill at initialization.
// Skills ask for what they need; kernel fulfills it.
// Connectors are typed — skill casts to the capability interface it needs.
type SkillDeps struct {
	// Outbox to fire events back to kernel (e.g. AgentRequest for sub-agent tasks)
	Outbox chan<- core.Event

	// Typed connectors injected by kernel based on skill config
	// Skills cast these to the capability interface they need:
	// e.g. emailConn := deps.Connectors["gmail"].(connectors.EmailConnector)
	Connectors map[string]any

	// Skill-specific config (API keys, settings, etc. — sourced from ASec/vault)
	Config map[string]any
}

// BaseSkill provides common functionality for all skills
type BaseSkill struct {
	name        string
	description string
	version     string
	skillType   SkillType
	outbox      chan<- core.Event
}
