package kernel

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// mustLoadVault loads vault config or exits
func loadVault(path string) (vault.Vault, error) {
	v, err := config.LoadVault(path)
	if err != nil {
		return nil, fmt.Errorf("load vault: %w", err)
	}
	return v, nil
}

// bootstrap.go
func loadAgents(ctx context.Context, v vault.Vault, configDir string, kernelBus chan<- core.Event) (*agents.Registry, error) {
	registry, err := agents.NewRegistry(ctx, configDir, v, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("create agents registry: %w", err)
	}
	return registry, nil
}

func loadConnectors(ctx context.Context, v vault.Vault, configDir string, kernelBus chan<- core.Event) (*connectors.Registry, error) {
	registry, err := connectors.NewRegistry(ctx, configDir, v, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("create connector registry: %w", err)
	}
	if err := registry.ConnectAll(); err != nil {
		return nil, fmt.Errorf("connect connectors: %w", err)
	}
	return registry, nil
}

func loadChannels(ctx context.Context, v vault.Vault, configDir string, kernelBus chan<- core.Event) (*channels.Registry, error) {
	// Create connector registry
	registry, err := channels.NewRegistry(ctx, configDir, v, kernelBus)

	if err != nil {
		return nil, fmt.Errorf("create channel registry: %w", err)
	}

	// Connect all
	if err := registry.ConnectAll(); err != nil {
		return nil, fmt.Errorf("connect channels: %w", err)
	}

	return registry, nil
}

func loadSkills(ctx context.Context, configDir string, kernelBus chan<- core.Event) (*skills.Registry, error) {
	// Create connector registr
	registry, err := skills.NewRegistry(ctx, configDir, kernelBus)
	if err != nil {
		return nil, fmt.Errorf("create skills registry: %w", err)
	}

	return registry, nil
}

func (k *Kernel) initializeSkills() error {
	for _, skill := range k.skills.GetAll() {
		if err := skill.Initialize(k.prepareSkillDeps(skill)); err != nil {
			return fmt.Errorf("initialize skill %s: %w", skill.Name(), err)
		}
	}
	return nil
}
func (k *Kernel) prepareSkillDeps(skill skills.Skill) skills.SkillDeps {
	// Get what the skill needs
	requiredCaps := skill.RequiredCapabilities()

	// Find matching connectors
	conns := make(map[string]any)
	for _, cap := range requiredCaps {
		// Get all connectors that support this capability
		for _, conn := range k.connectors.GetByCapability(cap) {
			conns[conn.Name()] = conn
		}
	}

	return skills.SkillDeps{
		Outbox:     k.bus,
		Connectors: conns, // Only relevant connectors
		Config:     nil,
	}
}

// assignAgentTools assigns tools to agents based on their configured skills
// Called after both agent and skill registries are created
func (k *Kernel) assignAgentTools() error {
	for _, agent := range k.agents.All() {

		// Executive agents get NO tools (they delegate)
		if agent.IsExecutive() {
			agent.SetTools([]tools.ToolDef{})
			k.logger.Info("executive agent configured",
				"agent_id", agent.ID(),
				"tools", 0,
				"role", "orchestrator")
			continue
		}

		// Specialized agents get tools from assigned skills
		agentTools := k.getToolsForAgent(agent.GetSkillNames())
		agent.SetTools(agentTools)

		k.logger.Info("specialized agent configured",
			"agent_id", agent.ID(),
			"skills", len(agent.GetSkillNames()),
			"tools", len(agentTools))
	}

	return nil
}

// getToolsForAgent returns all tools from the given skill names
func (k *Kernel) getToolsForAgent(skillNames []string) []tools.ToolDef {
	var agentTools []tools.ToolDef

	for _, skillName := range skillNames {
		skill, ok := k.skills.Get(skillName)
		if !ok {
			k.logger.Warn("skill not found for agent",
				"skill", skillName)
			continue
		}
		agentTools = append(agentTools, skill.Tools()...)
	}

	return agentTools
}

//
// // getDynamicToolsForTask analyzes a task and returns relevant tools
// // Used by executive when routing to general agent
// func (k *Kernel) getDynamicToolsForTask(taskDescription string, requiredCapabilities []core.Capability) []tools.ToolDef {
// 	var relevantTools []tools.ToolDef
//
// 	// Get all skills that support the required capabilities
// 	relevantSkills := make(map[string]skills.Skill)
// 	for _, cap := range requiredCapabilities {
// 		for _, skill := range k.skills.GetAll() {
// 			if slices.Contains(skill.RequiredCapabilities(), cap) {
// 				relevantSkills[skill.Name()] = skill
// 				break
// 			}
// 		}
// 	}
//
// 	// Collect tools from relevant skills
// 	for _, skill := range relevantSkills {
// 		relevantTools = append(relevantTools, skill.Tools()...)
// 	}
//
// 	k.logger.Debug("dynamic tool assignment",
// 		"capabilities", requiredCapabilities,
// 		"skills", len(relevantSkills),
// 		"tools", len(relevantTools))
//
// 	return relevantTools
// }

// findSpecializedAgent finds an agent that supports the required capabilities
func (k *Kernel) findSpecializedAgent(capabilities []core.Capability) (string, bool) {
	for _, agent := range k.agents.All() {
		if agent.IsExecutive() {
			continue
		}

		// Check if agent has skills covering all capabilities
		agentCapabilities := k.getAgentCapabilities(agent.GetSkillNames())
		if hasAllCapabilities(agentCapabilities, capabilities) {
			return agent.ID(), true
		}
	}
	return "", false
}

// getAgentCapabilities returns all capabilities covered by agent's assigned skills
func (k *Kernel) getAgentCapabilities(skillNames []string) []core.Capability {
	capSet := make(map[core.Capability]bool)

	for _, skillName := range skillNames {
		skill, ok := k.skills.Get(skillName)
		if !ok {
			continue
		}
		for _, cap := range skill.RequiredCapabilities() {
			capSet[cap] = true
		}
	}

	caps := make([]core.Capability, 0, len(capSet))
	for cap := range capSet {
		caps = append(caps, cap)
	}
	return caps
}

// hasAllCapabilities checks if agentCaps covers all required capabilities
func hasAllCapabilities(agentCaps, required []core.Capability) bool {
	capMap := make(map[core.Capability]bool)
	for _, cap := range agentCaps {
		capMap[cap] = true
	}

	for _, req := range required {
		if !capMap[req] {
			return false
		}
	}
	return true
}

// ValidateAgentSkills validates that all assigned skills exist in skill registry
// Called by kernel after skill registry is initialized
func validateAgentSkills(agentRegistry *agents.Registry, skillRegistry *skills.Registry) error {
	for _, agent := range agentRegistry.All() {
		for _, skillName := range agent.GetSkillNames() {
			if _, exists := skillRegistry.Get(skillName); !exists {
				return fmt.Errorf("agent %s: skill '%s' not found in skill registry", agent.ID(), skillName)
			}
		}
	}

	return nil
}
