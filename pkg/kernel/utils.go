package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/skills/cli"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

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
			execTools := tools.GetExecutiveTools()
			agent.SetTools(execTools)
			agent.SetFindBestAgent(k.findBestAgentToolDep)
			k.logger.Info("executive agent configured",
				"agent_id", agent.ID(),
				"tools", len(execTools),
				"role", "orchestrator")
			continue
		}
		if agent.IsGeneral() {
			agent.SetFindSkill(k.findSkillByCapability)
			agent.SetUseSkillTool(k.useSkillTool)
			k.logger.Info("general agent configured",
				"agent_id", agent.ID(),
				"tools", len(agent.GetSkillNames()),
				"role", "general")
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
func (k *Kernel) getToolsForAgent(skillNames []tools.SkillName) []tools.ToolDef {
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

// IsExecutiveTool checks if a tool is an executive meta-tool
func IsExecutiveTool(toolName string) bool {
	metaTools := map[string]bool{
		"delegate_to_agent":  true,
		"create_workflow":    true,
		"query_capabilities": true,
	}
	return metaTools[toolName]
}

// validateAndBuildCapabilityMap validates that all assigned skills exist in skill registry
// and builds a map of capabilities to agents
// Called by kernel after skill registry is initialized
func validateAndBuildCapabilityMap(agentsReg *agents.Registry, skillsReg *skills.Registry) (map[core.Capability][]AgentInfo, error) {
	capabilityMap := make(map[core.Capability][]AgentInfo)
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
				capabilityMap[cap] = append(capabilityMap[cap], AgentInfo{
					ID:   agent.ID(),
					Name: agent.Name(),
				})
			}
		}
	}
	return capabilityMap, nil
}

func buildCapabilitySection(capabilityMap map[core.Capability][]AgentInfo) string {
	if len(capabilityMap) == 0 {
		return "=== Available Sub-Agents & Capabilities ===\n(No specialized agents available - handle all tasks directly)"
	}

	// build a clean map for JSON marshaling
	out := make(map[string][]AgentInfo, len(capabilityMap))
	for cap, agents := range capabilityMap {
		out[string(cap)] = agents
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "=== AVAILABLE SUB-AGENTS & CAPABILITIES ===\n(error building capability map)"
	}

	return "=== AVAILABLE SUB-AGENTS & CAPABILITIES ===\n" + string(b)
}

func buildSystemPrompts(user *config.UserConfig, agentsReg *agents.Registry, capabilityMap map[core.Capability][]AgentInfo) {
	userSection := formatUserProfile(user)
	for _, agent := range agentsReg.All() {
		extra := ""
		if agent.IsExecutive() {
			extra = buildCapabilitySection(capabilityMap)

			generalAgentInfo := AgentInfo{
				ID:   agentsReg.GetGeneral().ID(),
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

func (k *Kernel) findBestAgentToolDep(ctx context.Context, task string, capabilities []core.Capability) (tools.AgentInfo, error) {
	agent, capabilities, found := k.findSpecializedAgent(capabilities)
	if !found {
		agent = k.agents.GetGeneral()
	}
	if agent == nil {
		return tools.AgentInfo{}, fmt.Errorf("no agent found for capabilities %v", capabilities)
	}
	return tools.AgentInfo{
		ID:           agent.ID(),
		Name:         agent.Name(),
		Capabilities: capabilities,
	}, nil
}

// FindByCapability searches for skills by capability
// Tries local first, then searches marketplaces and auto-installs
func (k *Kernel) findSkillByCapability(ctx context.Context, cap core.Capability) (interface{}, error) {
	// 1. Check local skills first
	skill, err := k.skills.FindByCapability(ctx, cap)
	if err != nil {
		return map[string]interface{}{
			"skill_name":      skill.Name(),
			"status":          "loaded",
			"available_tools": skill.Tools(),
			"usage_hint":      fmt.Sprintf("Use use_skill_tool with skill_name='%s' and choose from the tools above", skill.Name()),
		}, nil

	}

	// 2. Not found locally - search marketplaces
	results, err := k.skillMarketplaceManager.FindByCapability(ctx, string(cap))
	if err != nil || len(results) == 0 {
		return nil, fmt.Errorf("no skill found for capability: %s", cap)
	}

	// Pick best result (first one, already sorted by score)
	best := results[0]

	logger.Log.Info("found skill in marketplace",
		"capability", cap,
		"skill", best.Slug,
		"marketplace", best.Marketplace)

	// 3. Download the skill (gets SKILL.md file)
	skillPath, err := k.skillMarketplaceManager.DownloadAndInstall(
		ctx, best.Slug, best.Version, best.Marketplace, k.helperClient)
	if err != nil {
		return nil, fmt.Errorf("download skill failed: %w", err)
	}

	// 4. Load it using CLI skill loader (SAME PIPELINE!)
	logger.Log.Info("loading downloaded skill",
		"path", skillPath)

	skill, loadErr := cli.LoadCLISkill(skillPath, k.cliExecutor)
	if err != nil {
		return nil, fmt.Errorf("load skill failed: %w", loadErr)
	}

	// 5. Register it
	err = k.skills.Register(skill)
	if err != nil {
		return nil, fmt.Errorf("register skill failed: %w", err)
	}

	logger.Log.Info("downloaded skill registered",
		"skill", skill.Name(),
		"capability", cap,
		"source", best.Marketplace)

	// Return skill info with full tool definitions
	return map[string]interface{}{
		"skill_name":      skill.Name(),
		"status":          "loaded",
		"available_tools": skill.Tools(), // Already []ToolDef with full schema!
		"usage_hint":      fmt.Sprintf("Use use_skill_tool with skill_name='%s' and choose from the tools above", skill.Name()),
	}, nil
}

// useSkillTool executes a tool from a dynamically discovered skill
func (k *Kernel) useSkillTool(ctx context.Context, skillName tools.SkillName, toolName string, params map[string]interface{}) (interface{}, error) {
	// Get the skill
	skill, ok := k.skills.Get(skillName)
	if !ok {
		return nil, fmt.Errorf("skill not found: %s (did you call find_skill first?)", skillName)
	}

	// Find the tool in the skill
	found := false
	for _, t := range skill.Tools() {
		if t.Name == toolName {
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("tool '%s' not found in skill '%s'", toolName, skillName)
	}

	// Marshal params to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Execute the tool
	result, err := skill.Execute(ctx, toolName, paramsJSON)
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	k.logger.Info("executed skill tool",
		"skill", skillName,
		"tool", toolName,
		"success", true)

	return result, nil
}
