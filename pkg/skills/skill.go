package skills

import (
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// NewBaseSkill creates a new base skill
func NewBaseSkill(name tools.SkillName, description, version string, skillType SkillType) *BaseSkill {
	return &BaseSkill{
		name:        name,
		description: description,
		version:     version,
		skillType:   skillType,
	}
}

func (b *BaseSkill) Name() tools.SkillName { return b.name }
func (b *BaseSkill) Description() string   { return b.description }
func (b *BaseSkill) Version() string       { return b.version }
func (b *BaseSkill) Type() SkillType       { return b.skillType }

// SetOutbox stores the event bus channel
func (b *BaseSkill) SetOutbox(outbox chan<- core.Event) {
	b.outbox = outbox
}
