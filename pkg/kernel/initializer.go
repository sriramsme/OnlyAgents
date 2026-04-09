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
	memoryInitializer     struct{}
	cronInitializer       struct{}
	notifyInitializer     struct{}
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
	err = k.assignUserContext()
	if err != nil {
		return err
	}
	err = k.assignAvailableAgents()
	if err != nil {
		return err
	}
	return nil
}

func (i promptsInitializer) Init(ctx context.Context, k *Kernel) error {
	err := k.assignSystemPrompts()
	if err != nil {
		return err
	}
	return nil
}

// memoryInitializer is the last initializer — it must be the last one to run.
func (i memoryInitializer) Init(ctx context.Context, k *Kernel) error {
	// Register all memory jobs with the scheduler.
	for _, job := range k.mem.Jobs() {
		k.scheduler.Register(job)
	}
	return nil
}

func (i cronInitializer) Init(ctx context.Context, k *Kernel) error {
	k.loadCronJobs()
	return nil
}

func (i notifyInitializer) Init(ctx context.Context, k *Kernel) error {
	// Register all notification jobs with the scheduler.
	for _, job := range k.notifier.Jobs() {
		k.scheduler.Register(job)
	}
	return nil
}
