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

			generalAgentInfo := AgentInfo{
				Name: k.agents.GetGeneral().Name(),
			}
			generalJSON, err := json.MarshalIndent(generalAgentInfo, "", "  ")
			if err != nil {
				return err
			}
			extra += "\n\n=== GENERAL AGENT ===\n" + string(generalJSON)
		}
		agent.SetSystemPrompt(userSection, extra)
	}

	return nil
}

func (k *Kernel) buildAvailableAgentsSection() string {
	out := make(map[string]AgentInfo)
	for _, agent := range k.agents.All() {
		if agent.IsExecutive() || agent.IsGeneral() {
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
		sort.Strings(caps)

		out[agent.ID()] = AgentInfo{
			Name:         agent.Name(),
			Capabilities: caps,
		}
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
