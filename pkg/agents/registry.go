// pkg/kernel/registry.go
package agents

import (
	"context"
	"errors"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/config"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func NewRegistry(configsDir string, v vault.Vault, outbox chan<- core.Event) (*Registry, error) {
	configs, err := config.LoadAllAgentsConfig(configsDir, v)
	if err != nil {
		return nil, fmt.Errorf("load agent configs: %w", err)
	}
	r := &Registry{
		agents: make(map[string]*Agent),
	}

	for _, cfg := range configs {
		llmClient, err := llm.NewFactory(cfg, v).Create()
		if err != nil {
			return nil, fmt.Errorf("agent %s: llm init: %w", cfg.ID, err)
		}

		agent, err := NewAgent(*cfg, llmClient, []tools.ToolDef{}, outbox)
		if err != nil {
			return nil, fmt.Errorf("agent %s: init: %w", cfg.ID, err)
		}

		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("agent %s: start: %w", cfg.ID, err)
		}

		r.agents[cfg.ID] = agent
	}

	return r, nil
}

func (r *Registry) Register(a *Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[a.id] = a
}

func (r *Registry) Get(id string) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, ok := r.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	return agent, nil
}

func (r *Registry) Executive() (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, agent := range r.agents {
		if agent.isExecutive {
			return agent, nil
		}
	}
	return nil, fmt.Errorf("no executive agent configured")
}

func (r *Registry) All() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Agent, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	return out
}

func (r *Registry) ListAll() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var agents []string
	for _, agent := range r.agents {
		agents = append(agents, agent.id)
	}
	return agents
}

func (r *Registry) StartAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for _, agent := range r.agents {
		if err := agent.Start(); err != nil {
			errs = append(errs, fmt.Errorf("agent %s: %w", agent.id, err))
		}
	}
	return errors.Join(errs...)
}

func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for id, agent := range r.agents {
		if err := agent.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("agent %s: %w", id, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}
	return nil
}
