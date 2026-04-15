package memory

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func (e *Engine) Remember(ctx context.Context, input tools.RememberInput) error {
	e.nexus.ingest(ctx, "", fromRememberInput(input))
	return nil
}

// fromRememberInput maps a single RememberInput triple into NexusInput.
func fromRememberInput(input tools.RememberInput) NexusInput {
	return NexusInput{
		Entities: []extractedEntity{
			{Name: input.SubjectName, Type: input.SubjectType},
		},
		Relations: []extractedRelation{
			{
				Subject:   input.SubjectName,
				Predicate: input.Predicate,
				Object:    input.Object,
				StillTrue: true,
			},
		},
	}
}
