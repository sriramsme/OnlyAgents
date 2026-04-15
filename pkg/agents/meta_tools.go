package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/media"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// allMetaTools is the source of truth for meta-tool registration.
// The agent's tool definitions control which tools the LLM can actually call,
// so this map is only used at the dispatch boundary to skip skill routing.
var allMetaTools = map[string]bool{
	"delegate_to_agent":    true,
	"create_workflow":      true,
	"find_skill":           true,
	"meta_activate_groups": true,
}

func isMetaTool(name string) bool { return allMetaTools[name] }

// MetaToolInput is the unified context passed to handleMetaTool.
// Executive-only fields (OriginalMessage, ChannelMetadata, Attachments)
// are zero-valued for non-executive agents and ignored by non-executive handlers.
type MetaToolInput struct {
	SessionID     string
	CorrelationID string
	Call          tools.ToolCall

	// Executive-only context
	OriginalMessage string
	ChannelMetadata *core.ChannelMetadata
	Attachments     []*media.Attachment
}

// handleMetaTool executes a meta tool call internally without routing to a skill.
func (a *Agent) handleMetaTool(ctx context.Context, in MetaToolInput) tools.ToolExecution {
	a.logger.Debug("handling meta-tool",
		"tool", in.Call.Function.Name,
		"correlation_id", in.CorrelationID)

	switch in.Call.Function.Name {
	// ── executive
	case "delegate_to_agent":
		return a.requestDelegation(ctx, in.CorrelationID, in.Call, in.ChannelMetadata, in.Attachments)
	case "create_workflow":
		return a.requestWorkflow(ctx, in.CorrelationID, in.Call, in.OriginalMessage, in.ChannelMetadata, in.Attachments)

	// ── sub-agent
	case "meta_activate_groups":
		return a.handleActivateGroups(in.SessionID, in.Call)

		// ── memory
	case "remember":
		return a.handleRemember(ctx, in.SessionID, in.Call)

	case "recall":
		return a.handleRecall(ctx, in.SessionID, in.Call)

		// ── general
	case "find_skill":
		var input tools.FindSkillInput
		if err := json.Unmarshal([]byte(in.Call.Function.Arguments), &input); err != nil {
			return tools.ExecErr(fmt.Errorf("invalid find_skill args: %w", err))
		}
		result, err := a.handleFindSkill(ctx, a, input.SkillName)
		if err != nil {
			return tools.ExecErr(err)
		}
		return tools.ExecOK(result)
	default:
		return tools.ExecErr(fmt.Errorf("unknown meta-tool %q", in.Call.Function.Name))
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
