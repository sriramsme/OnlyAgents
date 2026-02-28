// pkg/skills/registry.go
package skills

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// Registry holds all skills available in the system.
type Registry struct {
	skills             map[string]Skill
	mu                 sync.RWMutex
	capabilityRegistry *core.CapabilityRegistry
}

// NewRegistry creates a new skill registry
func NewRegistry(
	ctx context.Context,
	configDir string,
	kernelBus chan<- core.Event,
	capabilityRegistry *core.CapabilityRegistry,
	cliExecutor interface{},
) (*Registry, error) {
	reg := &Registry{
		skills:             make(map[string]Skill),
		capabilityRegistry: capabilityRegistry,
	}

	// 1. Load native/system skills
	logger.Log.Info("loading native skills")
	for name, skillFactory := range factories {
		skill, err := skillFactory(ctx, kernelBus)
		if err != nil {
			return nil, fmt.Errorf("skill %s: %w", name, err)
		}

		err = reg.Register(skill)
		if err != nil {
			return nil, fmt.Errorf("register skill %s: %w", name, err)
		}

		logger.Log.Info("loaded native skill",
			"name", name,
			"type", skill.Type(),
			"tools", len(skill.Tools()))
	}

	// 2. Load file-based skills via loaders (CLI, etc.)
	logger.Log.Info("loading file-based skills")
	for loaderName, loader := range GetLoaders() {
		skills, err := loader(ctx, configDir, cliExecutor)
		if err != nil {
			logger.Log.Warn("skill loader failed",
				"loader", loaderName,
				"error", err)
			continue
		}

		for _, skill := range skills {
			if err := reg.Register(skill); err != nil {
				logger.Log.Warn("skill registration failed",
					"loader", loaderName,
					"skill", skill.Name(),
					"error", err)
				continue
			}
		}

		logger.Log.Info("loaded skills from loader",
			"loader", loaderName,
			"count", len(skills))
	}

	logger.Log.Info("skill registry initialized",
		"total_skills", len(reg.skills))

	return reg, nil
}

// Register registers a skill
func (r *Registry) Register(s Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.skills[s.Name()] = s

	// Register capabilities
	for _, cap := range s.RequiredCapabilities() {
		err := r.capabilityRegistry.Register(cap, &core.CapabilityInfo{
			Name:         cap,
			Source:       string(s.Type()),
			RegisteredBy: s.Name(),
		})
		if err != nil {
			return fmt.Errorf("register capability %s: %w", cap, err)
		}
	}

	return nil
}

// Get retrieves a skill by name
func (r *Registry) Get(name string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

// GetAll returns all registered skills
func (r *Registry) GetAll() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s)
	}
	return out
}

func (r *Registry) ListAll() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.skills))
	for name := range r.skills {
		out = append(out, name)
	}
	return out
}

// AllToolDefs returns all tool definitions
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
			logger.Log.Error("skill shutdown error",
				"skill", skill.Name(),
				"error", err)
		}
	}

	return nil
}

// FindByCapability searches for skills by capability
// Tries local first, then searches marketplaces and auto-installs
func (r *Registry) FindByCapability(ctx context.Context, cap core.Capability) (Skill, error) {
	// 1. Check local skills first
	r.mu.RLock()
	for _, skill := range r.skills {
		if slices.Contains(skill.RequiredCapabilities(), cap) {
			r.mu.RUnlock()
			logger.Log.Info("found skill locally",
				"capability", cap,
				"skill", skill.Name())
			return skill, nil
		}
	}
	r.mu.RUnlock()

	return nil, fmt.Errorf("no skill found for capability %s", cap)
}
