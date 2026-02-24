package kernel

import (
	"context"
	"fmt"
	"strings"

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
			execTools := getExecutiveTools()
			agent.SetTools(execTools)
			k.logger.Info("executive agent configured",
				"agent_id", agent.ID(),
				"tools", len(execTools),
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

// GetExecutiveTools returns orchestration tools for the executive agent
// These are NOT regular skills - they trigger kernel routing events
func getExecutiveTools() []tools.ToolDef {
	return []tools.ToolDef{
		// Delegate single task to specialized agent
		tools.NewToolDef(
			"delegate_to_agent",
			"Delegate a task to a specialized agent. Use when a request requires specific capabilities (calendar, email, web search, etc.) that you don't handle directly.",
			tools.BuildParams(
				map[string]tools.Property{
					"agent_id": tools.StringProp("ID of the agent to delegate to (from Available Capabilities & Agents list)"),
					"task":     tools.StringProp("Clear description of the task to delegate"),
					"capabilities": tools.ArrayProp(
						"Required capabilities for this task (for validation)",
						tools.EnumProp("", []string{
							"email", "calendar", "web_search", "web_fetch",
							"tasks", "storage", "notes", "git", "docker", "kubernetes",
						}),
					),
					"context": tools.Property{
						Type:        "object",
						Description: "Additional context for the delegated task (optional)",
					},
				},
				[]string{"agent_id", "task"}, // agent_id and task required, capabilities optional
			),
		),

		// Create multi-task workflow
		tools.NewToolDef(
			"create_workflow",
			"Create a workflow with multiple interdependent tasks. Use when a request requires multiple steps with dependencies (e.g., 'Check Bob's availability, then email him').",
			tools.BuildParams(
				map[string]tools.Property{
					"name": tools.StringProp("Name for this workflow"),
					"tasks": tools.ArrayProp(
						"List of tasks in the workflow",
						tools.Property{
							Type:        "object",
							Description: "Task definition",
							// Schema would include: id, name, description, capabilities, depends_on
						},
					),
				},
				[]string{"name", "tasks"},
			),
		),

		// Query available capabilities (for executive to know what's available)
		// tools.NewToolDef(
		// 	"query_capabilities",
		// 	"Query what capabilities and agents are currently available. Use when you need to check if a certain capability exists before delegating.",
		// 	tools.BuildParams(
		// 		map[string]tools.Property{
		// 			"capability": tools.StringProp("Capability to query (optional - leave empty for all)"),
		// 		},
		// 		[]string{},
		// 	),
		// ),
	}
}

// IsExecutiveTool checks if a tool is an executive meta-tool
func IsExecutiveTool(toolName string) bool {
	metaTools := map[string]bool{
		"delegate_to_agent":  true,
		"create_workflow":    true,
		"query_capabilities": true,
	}
	return metaTools[toolName]
}

func applyConfigDefaults(cfg Config) Config {
	if cfg.BusBufferSize == 0 {
		cfg.BusBufferSize = 256
	}
	if cfg.AgentConfigsDir == "" {
		cfg.AgentConfigsDir = "configs/agents/"
	}
	if cfg.ConnectorConfigsDir == "" {
		cfg.ConnectorConfigsDir = "configs/connectors/"
	}
	if cfg.ChannelConfigsDir == "" {
		cfg.ChannelConfigsDir = "configs/channels/"
	}
	if cfg.SkillConfigsDir == "" {
		cfg.SkillConfigsDir = "configs/skills/"
	}
	if cfg.VaultPath == "" {
		cfg.VaultPath = "configs/vault.yaml"
	}
	return cfg
}

type kernelComponents struct {
	agents        *agents.Registry
	connectors    *connectors.Registry
	channels      *channels.Registry
	skills        *skills.Registry
	user          *config.UserConfig
	capabilityMap map[core.Capability][]string
}

func loadComponents(ctx context.Context, cfg Config, bus chan core.Event) (kernelComponents, error) {
	var c kernelComponents

	v, err := loadVault(cfg.VaultPath)
	if err != nil {
		return c, fmt.Errorf("load vault: %w", err)
	}
	c.agents, err = loadAgents(ctx, v, cfg.AgentConfigsDir, bus)
	if err != nil {
		return c, fmt.Errorf("load agents: %w", err)
	}
	c.connectors, err = loadConnectors(ctx, v, cfg.ConnectorConfigsDir, bus)
	if err != nil {
		return c, fmt.Errorf("load connectors: %w", err)
	}
	c.channels, err = loadChannels(ctx, v, cfg.ChannelConfigsDir, bus)
	if err != nil {
		return c, fmt.Errorf("load channels: %w", err)
	}
	c.skills, err = loadSkills(ctx, cfg.SkillConfigsDir, bus)
	if err != nil {
		return c, fmt.Errorf("load skills: %w", err)
	}
	c.user, err = config.LoadUserConfig("configs/user.yaml")
	if err != nil {
		return c, fmt.Errorf("load user config: %w", err)
	}
	c.capabilityMap, err = validateAndBuildCapabilityMap(c.agents, c.skills)
	if err != nil {
		return c, fmt.Errorf("validate agent skills: %w", err)
	}

	return c, nil
}

// validateAndBuildCapabilityMap validates that all assigned skills exist in skill registry
// and builds a map of capabilities to agents
// Called by kernel after skill registry is initialized
func validateAndBuildCapabilityMap(agentsReg *agents.Registry, skillsReg *skills.Registry) (map[core.Capability][]string, error) {
	capabilityMap := make(map[core.Capability][]string)

	for _, agent := range agentsReg.All() {
		if agent.IsExecutive() {
			continue
		}

		for _, skillName := range agent.GetSkillNames() {
			skill, exists := skillsReg.Get(skillName)
			if !exists {
				return nil, fmt.Errorf("agent %s: skill '%s' not found in skill registry", agent.ID(), skillName)
			}
			for _, cap := range skill.RequiredCapabilities() {
				capabilityMap[cap] = append(capabilityMap[cap], agent.ID())
			}
		}
	}
	return capabilityMap, nil
}

func buildSystemPrompts(user *config.UserConfig, agentsReg *agents.Registry, capabilityMap map[core.Capability][]string) {
	userSection := formatUserProfile(user)
	for _, agent := range agentsReg.All() {
		extra := ""
		if agent.IsExecutive() {
			extra = buildCapabilitySection(capabilityMap)
		}
		agent.SetSystemPrompt(userSection, extra)
	}
}

func buildCapabilitySection(capabilityMap map[core.Capability][]string) string {
	if len(capabilityMap) == 0 {
		return "Available Capabilities & Agents:\n(No specialized agents available - handle all tasks directly)"
	}
	var sb strings.Builder
	sb.WriteString("Available Capabilities & Agents:\n")
	for cap, agents := range capabilityMap {
		sb.WriteString(fmt.Sprintf("- %s: %v\n", cap, agents))
	}
	return sb.String()
}

func formatUserProfile(user *config.UserConfig) string {
	return fmt.Sprintf(`
=== Who the user is ===
Name: %s (preferred: "%s")
Job: %s
Background: %s
Daily Routine: %s
Values: %s
Technical: %v | Collaboration: %s`,
		user.Identity.Name,
		user.Identity.PreferredName,
		user.Identity.Role,
		user.Background.Professional,
		user.DailyRoutine,
		strings.Join(user.Preferences.WhatIValue, ", "),
		user.Preferences.Technical,
		user.Preferences.Collaboration,
	)
}
