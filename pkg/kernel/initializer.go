// kernel/initializers.go
package kernel

import (
	"context"
)

type Initializer interface {
	Init(ctx context.Context, k *Kernel) error
}
type (
	connectorsInitializer struct{}
	agentsInitializer     struct{}
	promptsInitializer    struct{}
	serverInitializer     struct{}
)

func (i connectorsInitializer) Init(ctx context.Context, k *Kernel) error {
	k.registerLocalConnectors()
	return nil
}

func (i agentsInitializer) Init(ctx context.Context, k *Kernel) error {
	return k.assignAgentSkills()
}

func (i promptsInitializer) Init(ctx context.Context, k *Kernel) error {
	return k.buildSystemPrompts()
}

func (i serverInitializer) Init(ctx context.Context, k *Kernel) error {
	k.wireOAChannel()
	return nil
}
