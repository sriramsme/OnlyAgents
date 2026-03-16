package skills

import (
	"github.com/sriramsme/OnlyAgents/internal/config"
)

// BaseSkillInfo holds identity info for a skill.
// Used by native skills to self-describe without config dependency.
type BaseSkillInfo struct {
	Name        string
	Description string
	Version     string
	Enabled     bool
	AccessLevel string
}

// BaseSkill provides common functionality for all skills
type BaseSkill struct {
	name        string
	description string
	enabled     bool
	accessLevel string
	version     string
	skillType   SkillType
}

// NewBaseSkill creates a base skill from a BaseSkillInfo.
// Used by native skills directly.
func NewBaseSkill(info BaseSkillInfo, t SkillType) *BaseSkill {
	return &BaseSkill{
		name:        info.Name,
		description: info.Description,
		enabled:     info.Enabled,
		accessLevel: info.AccessLevel,
		version:     info.Version,
		skillType:   t,
	}
}

// newBaseSkillFromConfig is used internally by factory adapters only.
func NewBaseSkillFromConfig(cfg config.Skill, t SkillType) *BaseSkill {
	return NewBaseSkill(BaseSkillInfo{
		Name:        cfg.Name,
		Description: cfg.Description,
		Enabled:     cfg.Enabled,
		AccessLevel: cfg.AccessLevel,
		Version:     cfg.Version,
	}, t)
}

func (b *BaseSkill) Name() string        { return b.name }
func (b *BaseSkill) Description() string { return b.description }
func (b *BaseSkill) Version() string     { return b.version }
func (b *BaseSkill) Type() SkillType     { return b.skillType }
