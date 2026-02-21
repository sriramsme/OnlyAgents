package skills

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// NewBaseSkill creates a new base skill
func NewBaseSkill(name, description, version string, skillType SkillType) *BaseSkill {
	return &BaseSkill{
		name:        name,
		description: description,
		version:     version,
		skillType:   skillType,
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

// RequestSubAgent fires an AgentRequest event to kernel.
// Use when a skill needs another agent to perform a task (e.g. drafting).
func (b *BaseSkill) RequestSubAgent(ctx context.Context, correlationID string, task string, context map[string]any) {
	if b.outbox == nil {
		return
	}
	b.outbox <- core.Event{
		Type:          core.AgentRequest,
		CorrelationID: correlationID,
		Payload: core.AgentRequestPayload{
			RequestingSkill: b.name,
			Task:            task,
			Context:         context,
		},
	}
}
