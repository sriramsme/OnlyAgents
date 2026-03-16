package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
)

// Accept parent context to pass to agents
func NewRegistry(
	ctx context.Context,
	v vault.Vault,
	outbox chan<- core.Event,
	uiBus core.UIBus,
	cm *memory.ConversationManager,
	mm *memory.MemoryManager,
) (*Registry, error) {
	configs, err := config.LoadAllAgentsConfig(v)
	if err != nil {
		return nil, fmt.Errorf("load agent configs: %w", err)
	}

	r := &Registry{
		agents: make(map[string]RuntimeAgent),
	}

	// Create all agents (but don't start them yet - let caller control that)
	for _, cfg := range configs {
		llmClient, err := llm.NewFactory(&cfg.LLM, v).Create()
		if err != nil {
			return nil, fmt.Errorf("agent %s: llm init: %w", cfg.ID, err)
		}

		// Pass parent context to agent
		agent, err := NewAgent(ctx, *cfg, llmClient, outbox, uiBus, cm, mm)
		if err != nil {
			return nil, fmt.Errorf("agent %s: init: %w", cfg.ID, err)
		}

		r.agents[cfg.ID] = agent
		if agent.IsExecutive() {
			if r.executive != nil {
				return nil, fmt.Errorf("multiple executive agents configured")
			}
			r.executive = agent
		}
		if agent.IsGeneral() {
			if r.general != nil {
				return nil, fmt.Errorf("multiple general agents configured")
			}
			r.general = agent
		}
	}

	// Validate
	if r.executive == nil || r.general == nil {
		var missing []string

		if r.executive == nil {
			missing = append(missing, "Executive agent not configured")
		}

		if r.general == nil {
			missing = append(missing, "General agent not configured")
		}

		return nil, errors.New(strings.Join(missing, " or "))
	}

	return r, nil
}

func (r *Registry) Register(a RuntimeAgent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[a.ID()] = a
}

func (r *Registry) Get(id string) (RuntimeAgent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, ok := r.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	return agent, nil
}

func (r *Registry) GetExecutive() RuntimeAgent { return r.executive }

func (r *Registry) GetGeneral() RuntimeAgent { return r.general }

func (r *Registry) All() []RuntimeAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]RuntimeAgent, 0, len(r.agents))
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
		agents = append(agents, agent.ID())
	}
	return agents
}

// Release lock before I/O operations
func (r *Registry) StartAll() error {
	// Get snapshot without holding lock during I/O
	r.mu.RLock()
	agents := make([]RuntimeAgent, 0, len(r.agents))
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
		go func(a RuntimeAgent) {
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
	agents := make([]RuntimeAgent, 0, len(r.agents))
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
