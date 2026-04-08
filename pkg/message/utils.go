package message

import (
	"encoding/json"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// toMessage converts a Message to an llm.Message.
//
// Tool result content is truncated to ToolResultHistoryMaxBytes — the agent
// already synthesised the full result into its response; the raw blob is not
// needed for conversational continuity. Tool call arguments are never
// truncated since they are small and tell the agent what it requested.
//
// Messages authored by a different agent get Name = agentID so the LLM
// treats them as a distinct participant.
func toMessage(m *Message, selfAgentID string) (llm.Message, error) {
	switch m.Role {
	case "user":
		return llm.UserMessage(m.Content), nil

	case "assistant":
		var tcs []tools.ToolCall
		if m.ToolCalls != "" && m.ToolCalls != "[]" {
			if err := json.Unmarshal([]byte(m.ToolCalls), &tcs); err != nil {
				return llm.Message{}, fmt.Errorf("unmarshal tool calls: %w", err)
			}
		}
		var msg llm.Message
		if len(tcs) > 0 {
			// Tool call arguments are kept intact — they are small and
			// tell the agent what it requested in prior turns.
			msg = llm.AssistantMessageWithTools(m.Content, m.ReasoningContent, tcs)
		} else {
			msg = llm.Message{
				Role:             llm.RoleAssistant,
				Content:          m.Content,
				ReasoningContent: m.ReasoningContent,
			}
		}
		if m.AgentID != selfAgentID {
			msg.Name = m.AgentID
		}
		return msg, nil

	case "tool":
		// Recover the tool name stored in ToolCalls during SaveToolResult.
		var nameHolder []struct {
			Name string `json:"name"`
		}
		toolName := ""
		if err := json.Unmarshal([]byte(m.ToolCalls), &nameHolder); err == nil && len(nameHolder) > 0 {
			toolName = nameHolder[0].Name
		}
		// Truncate the result content — the agent synthesised this into its
		// response already. The full content is preserved in the DB for the
		// summarizer and debugging.
		content := truncateToolContent(toolName, m.Content)
		return llm.ToolResultMessage(m.ToolCallID, toolName, content), nil
	case "notification":
		return llm.UserMessage("[System notification sent to user: " + m.Content + "]"), nil
	default:
		return llm.Message{}, fmt.Errorf("unknown role %q", m.Role)
	}
}

// truncateToolContent caps tool result content at ToolResultHistoryMaxBytes.
// If truncated, a prefix is added so the LLM understands why the content ends
// abruptly rather than treating it as a malformed response.
func truncateToolContent(toolName, content string) string {
	if len(content) <= ToolResultHistoryMaxBytes {
		return content
	}
	return fmt.Sprintf("[%s: full result was %d chars, truncated for context]\n%s...",
		toolName, len(content), content[:ToolResultHistoryMaxBytes])
}

// sanitizeToolCallSequence removes malformed tool call sequences from history
// before sending to the LLM. Specifically:
//   - tool messages without a preceding assistant+tool_calls message
//   - assistant messages with tool_calls that have no following tool messages
//
// This is a safety net — e agents' turn lock should prevent these from occurring,
// but history loaded from DB can be corrupt from earlier bugs or crashes.
func sanitizeToolCallSequence(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))

	i := 0
	for i < len(msgs) {
		msg := msgs[i]

		if msg.Role != llm.RoleAssistant || len(msg.ToolCalls) == 0 {
			// Non-tool-call messages pass through
			// But skip orphaned tool messages
			if msg.Role == llm.RoleTool {
				if len(out) == 0 || out[len(out)-1].Role != llm.RoleAssistant || len(out[len(out)-1].ToolCalls) == 0 {
					i++
					continue // orphaned tool result — drop
				}
			}
			out = append(out, msg)
			i++
			continue
		}

		// Assistant message with tool calls — collect all following tool results
		expectedIDs := make(map[string]struct{}, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			expectedIDs[tc.ID] = struct{}{}
		}

		// Look ahead for tool results
		j := i + 1
		var toolResults []llm.Message
		foundIDs := make(map[string]struct{})
		for j < len(msgs) && msgs[j].Role == llm.RoleTool {
			toolResults = append(toolResults, msgs[j])
			if msgs[j].ToolCallID != "" {
				foundIDs[msgs[j].ToolCallID] = struct{}{}
			}
			j++
		}

		// Check all expected IDs are covered
		allFound := true
		for id := range expectedIDs {
			if _, ok := foundIDs[id]; !ok {
				allFound = false
				break
			}
		}

		if !allFound {
			// Drop entire assistant+tool_results block — it's incomplete
			logger.Log.Warn("dropping incomplete tool call sequence from history",
				"assistant_tool_calls", len(msg.ToolCalls),
				"tool_results_found", len(toolResults))
			i = j // skip past the incomplete block
			continue
		}

		// Valid sequence — append assistant + all tool results
		out = append(out, msg)
		out = append(out, toolResults...)
		i = j
	}

	return out
}
