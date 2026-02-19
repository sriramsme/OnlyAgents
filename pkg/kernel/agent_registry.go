// pkg/kernel/registry.go
package kernel

import (
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/config"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
)

func NewAgentRegistry(configs []*config.Config, v vault.Vault) (*AgentRegistry, error) {
	r := &AgentRegistry{
		agents: make(map[string]*Agent),
	}

	for _, cfg := range configs {
		llmClient, err := llm.NewFactory(cfg, v).Create()
		if err != nil {
			return nil, fmt.Errorf("agent %s: llm init: %w", cfg.Agent.ID, err)
		}

		agent, err := NewAgent(config.AgentConfig{
			ID:             cfg.Agent.ID,
			MaxConcurrency: cfg.Agent.MaxConcurrency,
			BufferSize:     cfg.Agent.BufferSize,
		},
			llmClient,
		)
		if err != nil {
			return nil, fmt.Errorf("agent %s: init: %w", cfg.Agent.ID, err)
		}

		if err := agent.Start(); err != nil {
			return nil, fmt.Errorf("agent %s: start: %w", cfg.Agent.ID, err)
		}

		r.agents[cfg.Agent.ID] = agent
	}

	return r, nil
}

func (r *AgentRegistry) Get(id string) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, ok := r.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	return agent, nil
}

func (r *AgentRegistry) Executive() (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, agent := range r.agents {
		if agent.isExecutive {
			return agent, nil
		}
	}
	return nil, fmt.Errorf("no executive agent configured")
}

func (r *AgentRegistry) ListAll() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var agents []string
	for _, agent := range r.agents {
		agents = append(agents, agent.id)
	}
	return agents
}

func (r *AgentRegistry) StopAll() error {
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

// RegisterConnectors wires connectors to all agents based on their config
func (r *AgentRegistry) RegisterConnectors(cfgs []*config.Config, connectorRegistry *ConnectorRegistry) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for id, agent := range r.agents {
		for _, cfg := range cfgs {
			if cfg.Agent.ID == id {
				if err := agent.RegisterConnectors(cfg.Connectors, connectorRegistry); err != nil {
					errs = append(errs, fmt.Errorf("agent %s: %w", id, err))
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("connector registration errors: %v", errs)
	}

	return nil
}
