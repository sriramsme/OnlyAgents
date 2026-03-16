package skills

import (
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// NewBaseSkill creates a new base skill
func NewBaseSkill(cfg config.Skill, t SkillType) *BaseSkill {
	return &BaseSkill{
		name:        cfg.Name,
		description: cfg.Description,
		enabled:     cfg.Enabled,
		accessLevel: cfg.AccessLevel,
		version:     cfg.Version,
		skillType:   t,
	}
}

func (b *BaseSkill) Name() string        { return b.name }
func (b *BaseSkill) Description() string { return b.description }
func (b *BaseSkill) Version() string     { return b.version }
func (b *BaseSkill) Type() SkillType     { return b.skillType }

// SetOutbox stores the event bus channel
func (b *BaseSkill) SetOutbox(outbox chan<- core.Event) {
	b.outbox = outbox
}
