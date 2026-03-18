package agents

import (
	"fmt"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func (a *Agent) GetSkillBindings() []config.SkillBinding {
	return a.skillsBindings
}

func (a *Agent) GetSkillNames() []string {
	names := make([]string, 0, len(a.skills))
	for _, s := range a.skills {
		names = append(names, s.Name())
	}
	return names
}

// AttachSkill adds a skill to a running agent and rebuilds the system prompt.
// Safe to call after boot for runtime skill injection.
func (a *Agent) AttachSkill(s skills.Skill) error {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()

	if err := s.Initialize(); err != nil {
		return fmt.Errorf("skill %q init failed: %w", s.Name(), err)
	}
	a.skills[s.Name()] = s
	a.AddTools(s.Tools())
	a.RebuildSystemPrompt()
	a.logger.Info("skill attached at runtime", "agent", a.id, "skill", s.Name())
	return nil
}

func (a *Agent) AddSkill(s skills.Skill) {
	a.skills[s.Name()] = s
	a.AddTools(s.Tools())
}

func (a *Agent) SetTools(tools []tools.ToolDef) {
	a.tools = tools
	for _, tool := range tools {
		a.toolSkillMap[tool.Name] = tool.Skill
	}
}

func (a *Agent) ListToolNames() []string {
	names := make([]string, len(a.tools))
	for i, t := range a.tools {
		names[i] = t.Name
	}
	return names
}

func (a *Agent) AddTools(tools []tools.ToolDef) {
	for _, tool := range tools {
		a.toolSkillMap[tool.Name] = tool.Skill
	}
	a.tools = append(a.tools, tools...)
}

// activeGroups holds the groups selected by the agent via meta_activate_groups.
// Scoped per session: map[sessionID]map[skillName][]ToolGroup
// Declared on Agent struct — added here for clarity:
//   activeGroups map[string]map[string][]tools.ToolGroup

// ToolsForRequest returns the tool set to inject into the current LLM API call.
// This is the only place tools should be read from — never use a.tools directly
// when building an API request.
//
//   - Group management inactive → all tools, no meta tools
//   - Groups not yet selected   → meta tools only (agent must plan first)
//   - Groups selected           → meta tools + active group tools
func (a *Agent) ToolsForRequest(sessionID string) []tools.ToolDef {
	if !a.needsGroupManagement() {
		return a.tools
	}

	selected, ok := a.activeGroups[sessionID]
	if !ok || len(selected) == 0 {
		// Agent hasn't activated groups yet — only expose meta tools
		return tools.GetSubAgentMetaTools()
	}

	result := tools.GetSubAgentMetaTools()
	for skillName, groups := range selected {
		skill, ok := a.skills[skillName]
		if !ok {
			continue
		}
		result = append(result, skill.ToolsByGroup(groups)...)
	}
	return result
}

// ResetGroupSelection clears the active group selection for a session.
// Call this at the start of each new task delegated from the executive,
// so the agent re-plans rather than reusing a previous task's groups.
func (a *Agent) ResetGroupSelection(sessionID string) {
	if a.activeGroups == nil {
		return
	}
	delete(a.activeGroups, sessionID)
}

// needsGroupManagement returns true when the agent has enough tools to warrant
// the planning step AND at least one skill exposes groups.
func (a *Agent) needsGroupManagement() bool {
	if len(a.tools) <= toolThreshold {
		return false
	}
	for _, skill := range a.skills {
		if len(skill.Groups()) > 0 {
			return true
		}
	}
	return false
}
