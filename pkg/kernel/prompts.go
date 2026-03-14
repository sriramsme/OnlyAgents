package kernel

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func (k *Kernel) buildSystemPrompts() error {
	userSection := k.formatUserProfile()
	for _, agent := range k.agents.All() {
		extra := ""
		if agent.IsExecutive() {
			extra = k.buildAvailableAgentsSection()
		}
		agent.SetSystemPrompt(userSection, extra)
	}

	return nil
}

func (k *Kernel) buildAvailableAgentsSection() string {
	out := make(map[string]AgentInfo)
	agents := k.agents.All()
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name() < agents[j].Name()
	})

	for _, agent := range agents {
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

		agentInfo := AgentInfo{
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
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		k.logger.Error("failed to marshal agent info", "error", err)
		return ""
	}
	return "=== AVAILABLE SUB-AGENTS ===\n" + string(b)
}

func (k *Kernel) formatUserProfile() string {
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
