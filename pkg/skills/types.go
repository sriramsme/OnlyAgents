package skills

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// SkillType represents the type of skill
type SkillType string

const (
	// SkillTypeNative - Implemented in Go and may use connectors
	SkillTypeNative SkillType = "native"

	// SkillTypeCLI - Defined by SKILL.md and executed via installed CLI tools
	SkillTypeCLI SkillType = "cli"

	// SkillTypeSystem - Internal framework skills (meta tools, workflows)
	SkillTypeSystem SkillType = "system"
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

	// Tools returns the function definitions this skill exposes to the LLM
	Tools() []tools.ToolDef

	// Groups returns name → description manifest.
	Groups() map[tools.ToolGroup]string

	// ToolsByGroup returns only tools belonging to the given groups.
	// If groups is empty, returns all tools.
	ToolsByGroup(groups []tools.ToolGroup) []tools.ToolDef

	// Execute is called by kernel when LLM requests a tool call
	Execute(ctx context.Context, toolName string, args []byte) tools.ToolExecution

	// Initialize is called by kernel at startup
	Initialize() error

	// Shutdown is called when kernel shuts down
	Shutdown() error
}
