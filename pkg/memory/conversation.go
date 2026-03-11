package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

const (
	DefaultHistoryTurns       = 10  // number of user→assistant exchanges to include
	ToolResultHistoryMaxBytes = 500 // tool result content cap when building history
)

// ConversationManager is shared across all agents. One instance lives in the
// kernel. Agents call it to save messages and retrieve history. Because it is
// a shared pointer, StartNewSession immediately affects every agent's next
// GetHistory call — no broadcast required.
type ConversationManager struct {
	store storage.Storage
	mu    sync.Map // map[sessionID]*sync.RWMutex
}

// New creates a ConversationManager. It resumes the last active session if one
// exists in the database, otherwise starts a fresh one.
func New(ctx context.Context, store storage.Storage) (*ConversationManager, error) {
	cm := &ConversationManager{store: store}
	return cm, nil
}

// NewSession creates session if it doesn't exist (idempotent)
func (cm *ConversationManager) NewSession(ctx context.Context, channel, agentID string) (string, error) {
	id := uuid.NewString()
	err := cm.store.CreateConversation(ctx, &storage.Conversation{
		ID:        id,
		Channel:   channel,
		AgentID:   agentID,
		StartedAt: storage.DBTime{Time: time.Now()},
	})
	return id, err
}

// Explicit session reset (triggered by /new command etc.)
func (cm *ConversationManager) EndSession(ctx context.Context, sessionID string) error {
	return cm.store.EndConversation(ctx, sessionID, "")
}

// ── Message persistence ───────────────────────────────────────────────────────

// SaveUserMessage persists an incoming user message turn.
func (cm *ConversationManager) SaveUserMessage(ctx context.Context, sessionID, agentID, content string) error {
	lock := cm.lockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()
	return cm.store.SaveMessage(ctx, &storage.Message{
		ID:             uuid.NewString(),
		ConversationID: sessionID,
		AgentID:        agentID,
		Role:           "user",
		Content:        content,
		Timestamp:      storage.DBTime{Time: time.Now()},
	})
}

// SaveAssistantMessage persists an assistant turn. toolCalls may be nil for
// a plain text response.
func (cm *ConversationManager) SaveAssistantMessage(
	ctx context.Context,
	sessionID, agentID, content, reasoningContent string,
	toolCalls []tools.ToolCall,
) error {

	return cm.saveAssistantMessage(ctx, sessionID, agentID, content,
		reasoningContent, toolCalls, time.Time{})
}

func (cm *ConversationManager) SaveAssistantMessageAt(
	ctx context.Context,
	sessionID, agentID, content, reasoningContent string,
	toolCalls []tools.ToolCall,
	saveAt time.Time,
) error {
	return cm.saveAssistantMessage(ctx, sessionID, agentID, content,
		reasoningContent, toolCalls, saveAt)
}

// SaveAssistantMessage persists an assistant turn. toolCalls may be nil for
// a plain text response.
func (cm *ConversationManager) saveAssistantMessage(
	ctx context.Context,
	sessionID, agentID, content, reasoningContent string,
	toolCalls []tools.ToolCall,
	saveAt time.Time,
) error {
	lock := cm.lockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()
	tcJSON := "[]"
	if len(toolCalls) > 0 {
		b, err := json.Marshal(toolCalls)
		if err != nil {
			return fmt.Errorf("memory: marshal tool calls: %w", err)
		}
		tcJSON = string(b)
	}
	if saveAt.IsZero() {
		saveAt = time.Now()
	}
	return cm.store.SaveMessage(ctx, &storage.Message{
		ID:               uuid.NewString(),
		ConversationID:   sessionID,
		AgentID:          agentID,
		Role:             "assistant",
		Content:          content,
		ReasoningContent: reasoningContent,
		ToolCalls:        tcJSON,
		Timestamp:        storage.DBTime{Time: saveAt},
	})
}

// SaveToolResult persists a tool result turn.
// toolCallID must match the ToolCall.ID from the preceding assistant turn.
func (cm *ConversationManager) SaveToolResult(
	ctx context.Context,
	sessionID, agentID, toolCallID, toolName, result string,
	isError bool,
) error {
	lock := cm.lockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()
	content := result
	if isError {
		content = fmt.Sprintf(`{"error":%q}`, result)
	}
	// Encode tool name in ToolCalls so we can reconstruct the llm.Message on load.
	nameJSON, err := json.Marshal([]map[string]string{{"name": toolName}})
	if err != nil {
		return fmt.Errorf("marshal tool name: %w", err)
	}
	return cm.store.SaveMessage(ctx, &storage.Message{
		ID:             uuid.NewString(),
		ConversationID: sessionID,
		AgentID:        agentID,
		Role:           "tool",
		Content:        content,
		ToolCallID:     toolCallID,
		ToolCalls:      string(nameJSON),
		Timestamp:      storage.DBTime{Time: time.Now()},
	})
}

// ── History retrieval ─────────────────────────────────────────────────────────

// GetHistory returns the last `maxTurns` complete conversational turns from
// the current conversation, ready to be prepended with the agent's system
// prompt and passed to the LLM.
//
// A "turn" is one user message plus all subsequent assistant and tool messages
// up to the next user message. This means a turn containing 5 tool calls still
// counts as 1 turn — the window is defined by human exchanges, not raw message
// count.
//
// Tool result content is truncated to ToolResultHistoryMaxBytes when building
// the slice. Full content is preserved in the DB for the summarizer.
//
// Messages from other agents are given Name = agentID so the LLM understands
// they came from a different participant in the session.
func (cm *ConversationManager) GetHistory(
	ctx context.Context,
	sessionID, agentID string,
	maxTurns int,
) ([]llm.Message, error) {
	lock := cm.lockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()

	all, err := cm.store.GetMessagesByAgent(ctx, sessionID, agentID)
	if err != nil {
		return nil, fmt.Errorf("memory: get history: %w", err)
	}

	// Group messages into turns. A new turn starts on each user message.
	// Each element of turns is a slice of messages: [user, assistant, tool, tool, ...]
	var turns [][]*storage.Message
	var current []*storage.Message
	for _, m := range all {
		if m.Role == "user" && len(current) > 0 {
			turns = append(turns, current)
			current = nil
		}
		current = append(current, m)
	}
	if len(current) > 0 {
		turns = append(turns, current)
	}

	// Sliding window on turns.
	if len(turns) > maxTurns {
		turns = turns[len(turns)-maxTurns:]
	}

	// Flatten turns back to []llm.Message, truncating tool result content.
	var out []llm.Message
	for _, turn := range turns {
		for _, m := range turn {
			msg, err := toMessage(m, agentID)
			if err != nil {
				logger.Log.Warn("memory: skipping malformed message",
					"id", m.ID, "role", m.Role, "err", err)
				continue
			}
			out = append(out, msg)
		}
	}
	// Sanitize before returning — defensive against corrupt history
	out = sanitizeToolCallSequence(out)
	return out, nil
}

func (cm *ConversationManager) lockFor(sessionID string) *sync.RWMutex {
	v, _ := cm.mu.LoadOrStore(sessionID, &sync.RWMutex{})
	return v.(*sync.RWMutex)
}

// toMessage converts a storage.Message to an llm.Message.
//
// Tool result content is truncated to ToolResultHistoryMaxBytes — the agent
// already synthesised the full result into its response; the raw blob is not
// needed for conversational continuity. Tool call arguments are never
// truncated since they are small and tell the agent what it requested.
//
// Messages authored by a different agent get Name = agentID so the LLM
// treats them as a distinct participant.
func toMessage(m *storage.Message, selfAgentID string) (llm.Message, error) {
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

	for i, msg := range msgs {
		switch msg.Role {
		case llm.RoleTool:
			// Only keep if previous message in out is assistant+tool_calls
			if len(out) == 0 {
				continue // orphaned tool result — drop
			}
			prev := out[len(out)-1]
			if prev.Role != llm.RoleAssistant || len(prev.ToolCalls) == 0 {
				continue // tool result without tool_calls — drop
			}
			out = append(out, msg)

		case llm.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				// Peek ahead — if no tool messages follow before next user/assistant, drop
				hasToolResults := false
				for j := i + 1; j < len(msgs); j++ {
					if msgs[j].Role == llm.RoleTool {
						hasToolResults = true
						break
					}
					if msgs[j].Role == llm.RoleUser || msgs[j].Role == llm.RoleAssistant {
						break // hit next turn before finding tool results
					}
				}
				if !hasToolResults {
					continue // dangling tool_calls with no results — drop
				}
			}
			out = append(out, msg)

		default:
			out = append(out, msg)
		}
	}

	return out
}
