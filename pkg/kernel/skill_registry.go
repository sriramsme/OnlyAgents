package kernel

import (
	"fmt"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/skills"
)

// SkillRegistry manages available skills
type SkillRegistry struct {
	skills map[string]skills.Skill
	mu     sync.RWMutex
}

func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills: make(map[string]skills.Skill),
	}
}

func (r *SkillRegistry) Register(skill skills.Skill) error {
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

func (r *SkillRegistry) Get(name string) (skills.Skill, error) {
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

func (r *SkillRegistry) GetAll() []skills.Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	skills := make([]skills.Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		skills = append(skills, skill)
	}
	return skills
}
