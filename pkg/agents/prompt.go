package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

const (
	// toolThreshold is the number of tools above which group management activates.
	// Below this, all tools are injected directly — no meta tools, no planning step.
	toolThreshold = 7

	skillManifestHeader = `## Available Skills
Each skill represents a domain you can operate in.`

	groupManifestHeader = `## Tool Groups

Your tools are loaded on-demand by group. Before executing any task, call
meta_activate_groups with the groups you need. On the next turn you will have
access to all tools in those groups.

Select the minimum groups needed. Include a skill's "passthrough" group only
when no named group covers the operation.

Available groups:`

	groupManifestFooter = `
Call meta_activate_groups like:
{"groups": {"git": ["inspect", "commit"], "github": ["pr_write", "ci"]}}`
)

// RebuildSystemPrompt is the single assembly point for the agent's system prompt.
// Kernel calls this once after all skills are assigned at boot.
// Agent calls this internally on any runtime change (AttachSkill, RegisterPeer).
func (a *Agent) RebuildSystemPrompt() {
	parts := []string{
		a.soul.SystemPrompt(a.formatAvailableAgents()),
	}

	if a.userContext != "" {
		parts = append(parts, a.userContext)
	}

	if manifest := a.buildSkillManifest(); manifest != "" {
		parts = append(parts, manifest)
	}

	if manifest := a.buildGroupManifest(); manifest != "" {
		parts = append(parts, manifest)
	}

	a.systemPrompt = strings.Join(parts, "\n\n")

	a.logger.Info("rebuilt system prompt", "prompt len", len(a.systemPrompt))
}

// formatAvailableAgents renders the peer agent map into a prompt section.
// Returns empty string for non-executive agents or when no peers are registered.
func (a *Agent) formatAvailableAgents() string {
	if !a.isExecutive || len(a.availableAgents) == 0 {
		return ""
	}
	b, err := json.MarshalIndent(a.availableAgents, "", "  ")
	if err != nil {
		a.logger.Error("failed to marshal available agents", "error", err)
		return ""
	}
	return "=== AVAILABLE SUB-AGENTS ===\n" + string(b)
}

func (a *Agent) buildSkillManifest() string {
	if len(a.skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(skillManifestHeader)
	b.WriteString("\n\n")

	for skillName, skill := range a.skills {
		fmt.Fprintf(&b, "%-14s — %s\n", skillName, skill.Description())
	}

	return b.String()
}

// buildGroupManifest generates the tool groups section of the system prompt.
// Returns empty string if group management is not active for this agent
// (i.e. total tool count is below threshold or no skill defines groups).
func (a *Agent) buildGroupManifest() string {
	if !a.needsGroupManagement() {
		return ""
	}

	var b strings.Builder
	b.WriteString(groupManifestHeader)
	b.WriteString("\n\n")

	for skillName, skill := range a.skills {
		groups := skill.Groups()
		if len(groups) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s:\n", skillName)
		for groupName, desc := range groups {
			fmt.Fprintf(&b, "  %-14s — %s\n", string(groupName), desc)
		}
		b.WriteString("\n")
	}

	b.WriteString(groupManifestFooter)
	return b.String()
}

// AskLLM is a helper for skills that need LLM assistance (e.g. drafting text).
func (a *Agent) AskLLM(ctx context.Context, system, prompt string) (string, error) {
	resp, err := a.llmClient.Chat(ctx, &llm.Request{
		Messages: []llm.Message{
			llm.SystemMessage(system),
			llm.UserMessage(prompt),
		},
		Metadata: map[string]string{"agent_id": a.id, "context": "skill_helper"},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
