package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

var executiveMetaTools = map[string]bool{
	"delegate_to_agent": true,
	"create_workflow":   true,
}

var generalMetaTools = map[string]bool{
	"find_skill": true,
}

var subAgentMetaTools = map[string]bool{
	"meta_activate_groups": true,
}

func isExecutiveMetaTool(name string) bool {
	return executiveMetaTools[name]
}

func isGeneralMetaTool(name string) bool {
	return generalMetaTools[name]
}

func isSubAgentMetaTool(name string) bool {
	return subAgentMetaTools[name]
}

// handleMetaTool executes a meta tool call internally without routing to a skill.
func (a *Agent) handleMetaTool(ctx context.Context, sessionID string, tc tools.ToolCall) tools.ToolExecution {
	switch tc.Function.Name {

	case "meta_activate_groups":
		return a.handleActivateGroups(sessionID, tc)

	default:
		return tools.ExecErr(fmt.Errorf("unknown meta tool %q", tc.Function.Name))
	}
}

func (a *Agent) handleActivateGroups(sessionID string, tc tools.ToolCall) tools.ToolExecution {
	var args struct {
		Groups map[string][]tools.ToolGroup `json:"groups"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return tools.ExecErr(fmt.Errorf("meta_activate_groups: invalid args: %w", err))
	}
	if len(args.Groups) == 0 {
		return tools.ExecErr(fmt.Errorf("meta_activate_groups: groups map is empty"))
	}

	// Validate all requested groups exist on the bound skills
	for skillName, requestedGroups := range args.Groups {
		skill, ok := a.skills[skillName]
		if !ok {
			return tools.ExecErr(fmt.Errorf("meta_activate_groups: unknown skill %q", skillName))
		}
		available := skill.Groups()
		for _, g := range requestedGroups {
			if _, ok := available[g]; !ok {
				return tools.ExecErr(fmt.Errorf("meta_activate_groups: skill %q has no group %q", skillName, g))
			}
		}
	}

	// Commit selection for this session
	if a.activeGroups == nil {
		a.activeGroups = make(map[string]map[string][]tools.ToolGroup)
	}
	a.activeGroups[sessionID] = args.Groups

	// Build the confirmation — tell the agent exactly what it gets next turn
	var toolNames []string
	for skillName, groups := range args.Groups {
		for _, td := range a.skills[skillName].ToolsByGroup(groups) {
			toolNames = append(toolNames, td.Name)
		}
	}

	a.logger.Info("groups activated",
		"agent", a.id,
		"session", sessionID,
		"groups", args.Groups,
		"tools_count", len(toolNames),
	)

	return tools.ExecOK(map[string]any{
		"status":         "ok",
		"activated":      args.Groups,
		"tools_incoming": toolNames,
		"note":           "These tools are now available starting from your next turn.",
	})
}
