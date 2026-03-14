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
)

func (i connectorsInitializer) Init(ctx context.Context, k *Kernel) error {
	k.registerLocalConnectors()
	return nil
}

func (i agentsInitializer) Init(ctx context.Context, k *Kernel) error {
	err := k.assignAgentSkills()
	if err != nil {
		return err
	}
	err = k.assignAgentDependencies()
	if err != nil {
		return err
	}
	return nil
}

func (i promptsInitializer) Init(ctx context.Context, k *Kernel) error {
	return k.buildSystemPrompts()
}
