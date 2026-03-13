package kernel

import (
	"context"
	"encoding/json"
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
func (k *Kernel) assignAgentTools() error {
	// Track which skills are claimed by specialized agents
	claimedSkills := make(map[tools.SkillName]bool)

	var generalAgent *agents.Agent

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
			connector, ok := k.resolveConnector(binding.Connector)
			if !ok {
				k.logger.Warn("connector not found",
					"agent", agent.ID(),
					"skill", binding.Name,
					"connector", binding.Connector)
				continue
			}

			skill := k.instantiateSkill(agent.ID(), binding.Name, connector)
			agent.AddSkill(skill)
		}

		k.logger.Info("specialized agent configured",
			"agent_id", agent.ID(),
			"skills", len(agent.GetSkillBindings()))
	}

	// --- Configure the single general agent ---
	if generalAgent != nil {
		var agentTools []tools.ToolDef
		for _, tmpl := range k.skills.GetAll() {
			if claimedSkills[tmpl.Name] {
				continue
			}

			connector := connectors.Connector(nil)
			if tmpl.Connector != nil && tmpl.Connector.Default != "" {
				c, ok := k.connectors.Get(tmpl.Connector.Default)
				if !ok {
					k.logger.Warn("default connector not found for general agent skill",
						"skill", tmpl.Name,
						"connector", tmpl.Connector.Default)
					continue
				}
				connector = c
			}

			skill := k.instantiateSkill(generalAgent.ID(), tmpl.Name, connector)
			generalAgent.AddSkill(skill)
		}
		generalAgent.SetHandleFindSkill(k.handleFindSkill) // marketplace fallback
		k.logger.Info("general agent configured",
			"agent_id", generalAgent.ID(),
			"tools", len(agentTools))
	}

	return nil
}

func (k *Kernel) validateSkillBindings(agent *agents.Agent) error {
	for _, binding := range agent.GetSkillBindings() {
		tmpl, ok := k.skills.Get(binding.Name)
		if !ok {
			return fmt.Errorf("agent %s: skill %q not found", agent.ID(), binding.Name)
		}

		// CLI skills never take connectors
		if tmpl.Type == "cli" && binding.Connector != "" {
			return fmt.Errorf("agent %s: skill %q is a CLI skill and does not use connectors",
				agent.ID(), binding.Name)
		}

		if tmpl.Connector == nil {
			continue // self-contained skill, nothing to validate
		}

		// Resolve which connector will be used
		connectorName := binding.Connector
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

func buildAvailableAgentsSection(agentsReg *agents.Registry, skillsReg *skills.Registry) string {
	out := make(map[string]AgentInfo)
	for _, agent := range agentsReg.All() {
		if agent.IsExecutive() || agent.IsGeneral() {
			continue
		}
		// Union of all skill capabilities
		capSet := make(map[string]bool)
		for _, skillName := range agent.GetSkillNames() {
			tmpl, ok := skillsReg.Get(skillName)
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
		sort.Strings(caps)

		out[agent.ID()] = AgentInfo{
			Name:         agent.Name(),
			Capabilities: caps,
		}
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		logger.Log.Error("failed to marshal agent info", "error", err)
		return ""
	}
	return "=== AVAILABLE SUB-AGENTS ===\n" + string(b)
}

func buildSystemPrompts(user *config.UserConfig, agentsReg *agents.Registry, skillsReg *skills.Registry) {
	userSection := formatUserProfile(user)
	for _, agent := range agentsReg.All() {
		extra := ""
		if agent.IsExecutive() {
			extra = buildAvailableAgentsSection(agentsReg, skillsReg)

			generalAgentInfo := AgentInfo{
				Name: agentsReg.GetGeneral().Name(),
			}
			generalJSON, err := json.MarshalIndent(generalAgentInfo, "", "  ")
			if err != nil {
				return
			}
			extra += "\n\n=== GENERAL AGENT ===\n" + string(generalJSON)
		}
		agent.SetSystemPrompt(userSection, extra)
	}
}

func formatUserProfile(user *config.UserConfig) string {
	return fmt.Sprintf(`
=== Who the user is ===
Name: %s (preferred: "%s")
Job: %s
Background: %s
Timezone: %s
Daily Routine: %s
Values: %s`,
		user.Identity.Name,
		user.Identity.PreferredName,
		user.Identity.Role,
		user.Background.Professional,
		user.Identity.Timezone,
		user.DailyRoutine,
		strings.Join(user.Preferences.WhatIValue, ", "),
	)
}

// Dependencies

// FindByCapability searches for skills
// Tries local first, then searches marketplaces and auto-installs
func (k *Kernel) findSkill(ctx context.Context, skillName string) (skills.Skill, error) {
	// 1. Check local registry
	if cfg, ok := k.skills.Get(tools.SkillName(skillName)); ok {
		logger.Log.Info("found skill locally", "skill", cfg.Name)
		return k.skills.Instantiate(k.ctx, cfg.Name, nil, k.bus)
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
	return k.skills.Instantiate(k.ctx, skillCfg.Name, nil, k.bus)
}

// find_skill execution in kernel
func (k *Kernel) handleFindSkill(ctx context.Context, agent *agents.Agent, skillName string) (any, error) {
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

// resolveConnector returns the Connector by name or nil, with ok=false if not found
func (k *Kernel) resolveConnector(name string) (connectors.Connector, bool) {
	if name == "" {
		return nil, true
	}
	return k.connectors.Get(name)
}

// instantiateSkill instantiates a skill and returns its tools; logs warnings on failure
func (k *Kernel) instantiateSkill(agentID string, skillName tools.SkillName, connector connectors.Connector) skills.Skill {
	skill, err := k.skills.Instantiate(k.ctx, skillName, connector, k.bus)
	if err != nil {
		k.logger.Warn("failed to instantiate skill",
			"agent", agentID,
			"skill", skillName,
			"error", err)
		return nil
	}
	return skill
}
