package kernel

import (
	"context"
	"fmt"
	"sync"
)

// Skill interface that all skills must implement
type Skill interface {
	// Metadata
	Name() string
	Description() string
	Version() string
	RequiredCapabilities() []string

	// Execution
	Execute(ctx context.Context, intent string, params map[string]interface{}) (interface{}, error)

	// LLM Integration
	GetSystemPrompt() string

	// Lifecycle
	Initialize() error
	Shutdown() error
}

// SkillRegistry manages available skills
type SkillRegistry struct {
	skills map[string]Skill
	mu     sync.RWMutex
}

func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills: make(map[string]Skill),
	}
}

func (r *SkillRegistry) Register(skill Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := skill.Name()
	if _, exists := r.skills[name]; exists {
		return fmt.Errorf("skill %s already registered", name)
	}

	if err := skill.Initialize(); err != nil {
		return fmt.Errorf("skill initialization failed: %w", err)
	}

	r.skills[name] = skill
	fmt.Printf("Registered skill: %s (v%s)\n", name, skill.Version())
	return nil
}

func (r *SkillRegistry) Get(name string) (Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, exists := r.skills[name]
	if !exists {
		return nil, fmt.Errorf("skill %s not found", name)
	}

	return skill, nil
}

func (r *SkillRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	return names
}
