package kernel

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// assignAgentTools assigns tools to agents based on their configured skills
// Called after both agent and skill registries are created
func (k *Kernel) assignAgentSkills() error {
	// Track which skills are claimed by specialized agents
	claimedSkills := make(map[string]bool)

	var generalAgent agents.RuntimeAgent

	// First pass: specialized agents + find the general agent
	for _, agent := range k.agents.All() {
		if agent.IsGeneral() {
			generalAgent = agent
			continue
		}

		if agent.IsExecutive() {
			execTools := tools.GetExecutiveTools()
			agent.SetTools(execTools)
			k.logger.Info("executive agent configured",
				"agent_id", agent.ID(),
				"tools", len(execTools))
			continue
		}

		// Validate bindings before instantiating
		if err := k.validateSkillBindings(agent); err != nil {
			return err
		}

		for _, binding := range agent.GetSkillBindings() {
			connector, ok := k.resolveConnector(binding.Name, binding.ConnectorID)
			if !ok {
				k.logger.Warn("connector not found",
					"agent", agent.ID(),
					"skill", binding.Name,
					"connector", binding.ConnectorID)
				continue
			}

			skill := k.instantiateSkill(agent.ID(), binding.Name, connector)
			agent.AddSkill(skill)
			claimedSkills[binding.Name] = true
		}

		k.logger.Info("specialized agent configured",
			"agent_id", agent.ID(),
			"skills", len(agent.GetSkillBindings()))
	}

	// --- Configure the single general agent ---
	if generalAgent != nil {
		for _, tmpl := range k.skills.GetAll() {
			if claimedSkills[tmpl.Name] {
				continue
			}
			connector, ok := k.resolveConnector(tmpl.Name, "") // empty = use default
			if !ok {
				k.logger.Warn("default connector not found for general agent skill",
					"skill", tmpl.Name,
					"connector", tmpl.Connector.Default)
				continue
			}
			skill := k.instantiateSkill(generalAgent.ID(), tmpl.Name, connector)
			if skill == nil {
				continue
			}
			generalAgent.AddSkill(skill)
		}
		k.logger.Info("general agent configured",
			"agent_id", generalAgent.ID(),
			"skills", generalAgent.GetSkillNames(),
			"tools", generalAgent.ListToolNames())
	}
	return nil
}

// assignAgentDependencies assigns dependencies to agents
func (k *Kernel) assignAgentDependencies() error {
	k.agents.GetExecutive().SetResolveAgentName(k.ResolveAgentName)
	k.agents.GetGeneral().SetHandleFindSkill(k.handleFindSkill)
	return nil
}

// assignUserContext assigns the user profile / preference section injected by kernel.
func (k *Kernel) assignUserContext() error {
	userSection := k.buildUserContext()
	for _, agent := range k.agents.All() {
		agent.SetUserContext(userSection)
	}
	return nil
}

// assignAvailableAgents assigns the peer agent manifest — executive agents only.
// Calling this on a non-executive agent is a no-op.
func (k *Kernel) assignAvailableAgents() error {
	availableAgents := k.buildAvailableAgentsMap()
	executive := k.agents.GetExecutive()
	executive.SetAvailableAgents(availableAgents)
	return nil
}

func (k *Kernel) buildAvailableAgentsMap() map[string]agents.AgentInfo {
	out := make(map[string]agents.AgentInfo)
	allAgents := k.agents.All()
	sort.Slice(allAgents, func(i, j int) bool {
		return allAgents[i].Name() < allAgents[j].Name()
	})

	for _, agent := range allAgents {
		if agent.IsExecutive() {
			continue
		}
		// Union of all skill capabilities
		capSet := make(map[string]bool)
		for _, skillName := range agent.GetSkillNames() {
			tmpl, ok := k.skills.Get(skillName)
			if !ok {
				continue
			}
			for _, c := range tmpl.Capabilities {
				capSet[c] = true
			}
		}
		caps := make([]string, 0, len(capSet))
		for c := range capSet {
			caps = append(caps, c)
		}
		if agent.IsGeneral() {
			caps = append(caps, "find_skill_online")
		}
		sort.Strings(caps)

		agentInfo := agents.AgentInfo{
			ID:           agent.ID(),
			Name:         agent.Name(),
			Description:  agent.Description(),
			Capabilities: caps,
		}
		if agent.IsGeneral() {
			agentInfo.IsGeneral = true
		}
		out[agent.ID()] = agentInfo
	}

	return out
}

func (k *Kernel) buildUserContext() string {
	return fmt.Sprintf(`
=== Who the user is ===
Name: %s (preferred: "%s")
Job: %s
Background: %s
Timezone: %s
Daily Routine: %s
Values: %s`,
		k.user.Identity.Name,
		k.user.Identity.PreferredName,
		k.user.Identity.Role,
		k.user.Background.Professional,
		k.user.Identity.Timezone,
		k.user.DailyRoutine,
		strings.Join(k.user.Preferences.WhatIValue, ", "),
	)
}

func (k *Kernel) validateSkillBindings(agent agents.RuntimeAgent) error {
	for _, binding := range agent.GetSkillBindings() {
		tmpl, ok := k.skills.Get(binding.Name)
		if !ok {
			return fmt.Errorf("agent %s: skill %q not found", agent.ID(), binding.Name)
		}

		// CLI skills never take connectors
		if tmpl.Type == "cli" && binding.ConnectorID != "" {
			return fmt.Errorf("agent %s: skill %q is a CLI skill and does not use connectors",
				agent.ID(), binding.Name)
		}

		if tmpl.Connector == nil {
			continue // self-contained skill, nothing to validate
		}

		// Resolve which connector will be used
		connectorName := binding.ConnectorID
		if connectorName == "" {
			connectorName = tmpl.Connector.Default
		}

		// Check it's in the supported list
		if !slices.Contains(tmpl.Connector.Supported, connectorName) {
			return fmt.Errorf("agent %s: skill %q does not support connector %q (supported: %s)",
				agent.ID(), binding.Name, connectorName,
				strings.Join(tmpl.Connector.Supported, ", "))
		}

		// Check the connector is actually registered
		if _, ok := k.connectors.Get(connectorName); !ok {
			return fmt.Errorf("agent %s: skill %q requires connector %q which is not enabled",
				agent.ID(), binding.Name, connectorName)
		}
	}
	return nil
}

// Agent Dependencies

// FindByCapability searches for skills
// Tries local first, then searches marketplaces and auto-installs
func (k *Kernel) findSkill(ctx context.Context, skillName string) (skills.Skill, error) {
	// 1. Check local registry
	if cfg, ok := k.skills.Get(skillName); ok {
		logger.Log.Info("found skill locally", "skill", cfg.Name)
		return k.skills.Instantiate(k.ctx, cfg.Name, nil, k.cfg.Security)
	}

	// 2. Search marketplace
	results, err := k.skillMarketplaceManager.FindSkill(ctx, skillName)
	if err != nil || len(results) == 0 {
		return nil, fmt.Errorf("skill %q not found locally or in marketplace", skillName)
	}

	best := results[0]
	logger.Log.Info("found skill in marketplace",
		"skill", best.Slug,
		"marketplace", best.Marketplace)

	// 3. Download and install
	skillPath, err := k.skillMarketplaceManager.DownloadAndInstall(
		ctx, best.Slug, best.Version, best.Marketplace, k.helperClient)
	if err != nil {
		return nil, fmt.Errorf("download skill %q: %w", skillName, err)
	}

	// 4. Load config
	skillCfg, err := config.LoadSkillConfig(skillPath)
	if err != nil {
		return nil, fmt.Errorf("load skill config %q: %w", skillPath, err)
	}

	// 5. Register
	k.skills.Register(*skillCfg)
	logger.Log.Info("skill registered", "skill", skillCfg.Name)

	// 6. Instantiate
	return k.skills.Instantiate(k.ctx, skillCfg.Name, nil, k.cfg.Security)
}

// find_skill execution in kernel
// plugged into general Agent as a meta-tool
func (k *Kernel) handleFindSkill(ctx context.Context, agent agents.RuntimeAgent, skillName string) (any, error) {
	skill, err := k.findSkill(ctx, skillName)
	if err != nil {
		return nil, err
	}

	// Inject real tools into agent — visible on next LLM call
	agent.AddTools(skill.Tools())

	toolNames := make([]string, len(skill.Tools()))
	for i, t := range skill.Tools() {
		toolNames[i] = t.Name
	}

	return map[string]any{
		"skill":       skillName,
		"tools_added": toolNames,
		"message":     fmt.Sprintf("Skill loaded. You can now call: %s", strings.Join(toolNames, ", ")),
	}, nil
}

func (k *Kernel) ResolveAgentName(agentID string) string {
	agent, err := k.agents.Get(agentID)
	if err != nil {
		return agentID
	}
	return agent.Name()
}

// Helpers

// resolveConnector returns the Connector by name or nil, with ok=false if not found
func (k *Kernel) resolveConnector(skillName string, connectorName string) (connectors.Connector, bool) {
	// If explicit connector specified, use it
	if connectorName != "" {
		return k.connectors.Get(connectorName)
	}
	// Fall back to skill's default connector
	tmpl, ok := k.skills.Get(skillName)
	if !ok || tmpl.Connector == nil || tmpl.Connector.Default == "" {
		return nil, true // skill doesn't need a connector
	}
	return k.connectors.Get(tmpl.Connector.Default)
}

// instantiateSkill instantiates a skill and returns its tools; logs warnings on failure
func (k *Kernel) instantiateSkill(agentID string, skillName string, connector connectors.Connector) skills.Skill {
	skill, err := k.skills.Instantiate(k.ctx, skillName, connector, k.cfg.Security)
	if err != nil {
		k.logger.Warn("failed to instantiate skill",
			"agent", agentID,
			"skill", skillName,
			"error", err)
		return nil
	}
	return skill
}
