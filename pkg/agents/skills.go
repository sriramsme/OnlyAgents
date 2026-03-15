package agents

import (
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func (a *Agent) GetSkillBindings() []config.SkillBinding {
	return a.skillsBindings
}

func (a *Agent) GetSkillNames() []tools.SkillName {
	names := make([]tools.SkillName, 0, len(a.skills))
	for _, s := range a.skills {
		names = append(names, s.Name())
	}
	return names
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
