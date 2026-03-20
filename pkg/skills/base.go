package skills

import (
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// BaseSkillInfo holds identity info for a skill.
// Used by native skills to self-describe without config dependency.
type BaseSkillInfo struct {
	Name        string
	Description string
	Version     string
	Enabled     bool
	AccessLevel string
	Tools       []tools.ToolDef
	Groups      map[tools.ToolGroup]string
}

// BaseSkill provides common functionality for all skills
type BaseSkill struct {
	name         string
	description  string
	enabled      bool
	accessLevel  string
	version      string
	skillType    SkillType
	toolDefs     []tools.ToolDef
	groups       map[tools.ToolGroup]string
	toolsByGroup map[tools.ToolGroup][]tools.ToolDef
}

// NewBaseSkill creates a base skill from a BaseSkillInfo.
// Used by native skills directly.
func NewBaseSkill(info BaseSkillInfo, t SkillType) *BaseSkill {
	toolsByGroup := make(map[tools.ToolGroup][]tools.ToolDef)
	for _, tool := range info.Tools {
		if tool.Group != "" {
			toolsByGroup[tool.Group] = append(toolsByGroup[tool.Group], tool)
		}
	}
	return &BaseSkill{
		name:         info.Name,
		description:  info.Description,
		enabled:      info.Enabled,
		accessLevel:  info.AccessLevel,
		version:      info.Version,
		skillType:    t,
		toolDefs:     info.Tools,
		groups:       info.Groups,
		toolsByGroup: toolsByGroup,
	}
}

// newBaseSkillFromConfig is used internally by factory adapters only.
func NewBaseSkillFromConfig(
	cfg Config,
	t SkillType,
	toolDefs []tools.ToolDef,
	groups map[tools.ToolGroup]string,
) *BaseSkill {
	return NewBaseSkill(BaseSkillInfo{
		Name:        cfg.Name,
		Description: cfg.Description,
		Enabled:     cfg.Enabled,
		AccessLevel: cfg.AccessLevel,
		Version:     cfg.Version,
		Tools:       toolDefs,
		Groups:      groups,
	}, t)
}

func (b *BaseSkill) Name() string                       { return b.name }
func (b *BaseSkill) Description() string                { return b.description }
func (b *BaseSkill) Version() string                    { return b.version }
func (b *BaseSkill) Type() SkillType                    { return b.skillType }
func (b *BaseSkill) Tools() []tools.ToolDef             { return b.toolDefs }
func (b *BaseSkill) Groups() map[tools.ToolGroup]string { return b.groups }

func (b *BaseSkill) ToolsByGroup(filter []tools.ToolGroup) []tools.ToolDef {
	if len(filter) == 0 {
		return b.toolDefs
	}
	var out []tools.ToolDef
	for _, g := range filter {
		out = append(out, b.toolsByGroup[g]...)
	}
	return out
}
