package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func (a *Agent) isGeneralMetaTool(toolName string) bool {
	metaTools := map[string]bool{
		"find_skill": true,
	}
	return metaTools[toolName]
}

func (a *Agent) handleGeneralMetaTool(ctx context.Context, correlationID string, tc tools.ToolCall) tools.ToolExecution {
	a.logger.Debug("handling meta-tool",
		"tool", tc.Function.Name,
		"correlation_id", correlationID)

	switch tc.Function.Name {
	case "find_skill":
		var input tools.FindSkillInput
		args := tc.Function.Arguments
		if err := json.Unmarshal([]byte(args), &input); err != nil {
			return tools.ExecErr(fmt.Errorf("invalid find_skill args: %w", err))
		}
		result, err := a.handleFindSkill(ctx, a, input.SkillName)
		if err != nil {
			return tools.ExecErr(err)
		}
		return tools.ExecOK(result)
	default:
		return tools.ExecErr(fmt.Errorf("unknown meta-tool: %s", tc.Function.Name))
	}
}

func (a *Agent) SetHandleFindSkill(fn handleFindSkillFunc) {
	a.handleFindSkill = fn
}
