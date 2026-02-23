package skills

import (
	"context"
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// Registry holds all skills available in the system.
// Lives in kernel — skills are registered once at startup.
type Registry struct {
	skills map[string]Skill
	mu     sync.RWMutex
}

func NewRegistry(
	ctx context.Context,
	configDir string, //cli skills via configs/skills/SKILL.md files
	kernelBus chan<- core.Event,
) (*Registry, error) {

	// configs, err := config.LoadAllConnectorConfigs(configDir)
	// if err != nil {
	// 	return nil, fmt.Errorf("load connector configs: %w", err)
	// }

	registry := &Registry{
		skills: make(map[string]Skill),
	}

	// first loop through auto-registered system skills and create each skill
	for name, skillFactory := range factories {

		skill, err := skillFactory(ctx, kernelBus)
		if err != nil {
			return nil, fmt.Errorf("skill %s: %w", name, err)
		}

		registry.skills[name] = skill
	}

	return registry, nil
}

func (r *Registry) Register(s Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[s.Name()] = s
	return nil
}

func (r *Registry) Get(name string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

func (r *Registry) GetAll() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s)
	}
	return out
}

// AllToolDefs returns all tool definitions across all registered skills.
// Kernel uses this to build the tools list for each agent.
func (r *Registry) AllToolDefs() []tools.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var toolDefs []tools.ToolDef
	for _, s := range r.skills {
		toolDefs = append(toolDefs, s.Tools()...)
	}
	return toolDefs
}

// Shutdown shuts down all skills
func (r *Registry) Shutdown() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, skill := range r.skills {
		if err := skill.Shutdown(); err != nil {
			// Log error but continue shutting down others
			continue
		}
	}
	return nil
}
