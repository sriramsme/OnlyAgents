package agents

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// Accept parent context to pass to agents
func NewRegistry(
	ctx context.Context,
	configsDir string,
	v vault.Vault,
	outbox chan<- core.Event,
) (*Registry, error) {
	configs, err := config.LoadAllAgentsConfig(configsDir, v)
	if err != nil {
		return nil, fmt.Errorf("load agent configs: %w", err)
	}

	r := &Registry{
		agents: make(map[string]*Agent),
	}

	// Create all agents (but don't start them yet - let caller control that)
	for _, cfg := range configs {
		llmClient, err := llm.NewFactory(cfg, v).Create()
		if err != nil {
			return nil, fmt.Errorf("agent %s: llm init: %w", cfg.ID, err)
		}

		// Pass parent context to agent
		agent, err := NewAgent(ctx, *cfg, llmClient, []tools.ToolDef{}, outbox)
		if err != nil {
			return nil, fmt.Errorf("agent %s: init: %w", cfg.ID, err)
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

// FIXED: Release lock before I/O operations
func (r *Registry) StartAll() error {
	// Get snapshot without holding lock during I/O
	r.mu.RLock()
	agents := make([]*Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}
	r.mu.RUnlock()

	// Start agents in parallel (they're independent)
	type result struct {
		agentID string
		err     error
	}

	resultCh := make(chan result, len(agents))
	var wg sync.WaitGroup

	for _, agent := range agents {
		wg.Add(1)
		go func(a *Agent) {
			defer wg.Done()
			if err := a.Start(); err != nil {
				resultCh <- result{agentID: a.ID(), err: err}
			}
		}(agent)
	}

	wg.Wait()
	close(resultCh)

	// Collect errors
	var errs []error
	for res := range resultCh {
		errs = append(errs, fmt.Errorf("agent %s: %w", res.agentID, res.err))
	}

	return errors.Join(errs...)
}

// Release lock before I/O operations
func (r *Registry) StopAll(ctx context.Context) error {
	// Get snapshot without holding lock during I/O
	r.mu.RLock()
	agents := make([]*Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}
	r.mu.RUnlock()

	// Stop agents sequentially (shutdown order might matter)
	// If you want parallel stop, use the same pattern as StartAll
	var errs []error
	for _, agent := range agents {
		if err := agent.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("agent %s: %w", agent.ID(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}
	return nil
}
