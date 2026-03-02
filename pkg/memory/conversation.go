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

// sessionKey is a sentinel used to persist the current convID in agent_state.
// It is internal to this package — no real agent has this ID.
const sessionKey = "__session__"

// DefaultHistoryLimit is the sliding window size passed to GetHistory.
const DefaultHistoryLimit = 20

// ConversationManager is shared across all agents. One instance lives in the
// kernel. Agents call it to save messages and retrieve history. Because it is
// a shared pointer, StartNewSession immediately affects every agent's next
// GetHistory call — no broadcast required.
type ConversationManager struct {
	store  storage.Storage
	mu     sync.RWMutex
	convID string
}

// New creates a ConversationManager. It resumes the last active session if one
// exists in the database, otherwise starts a fresh one.
func New(ctx context.Context, store storage.Storage) (*ConversationManager, error) {
	cm := &ConversationManager{store: store}

	// Try to resume the last active session.
	state, err := store.GetAgentState(ctx, sessionKey)
	if err == nil && state.CurrentConversationID != "" {
		conv, convErr := store.GetConversation(ctx, state.CurrentConversationID)
		if convErr == nil && !conv.EndedAt.Valid {
			cm.convID = state.CurrentConversationID
			logger.Log.Info("memory: resumed session", "conv_id", cm.convID)
			return cm, nil
		}
	}

	// No active session found — start fresh.
	if err := cm.createSession(ctx); err != nil {
		return nil, fmt.Errorf("memory: init: %w", err)
	}
	return cm, nil
}

// CurrentConvID returns the active conversation ID.
func (cm *ConversationManager) CurrentConvID() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.convID
}

// StartNewSession ends the current conversation and begins a fresh one.
// Called by the kernel on a NewSession event. Returns the new conversation ID.
func (cm *ConversationManager) StartNewSession(ctx context.Context) (string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.convID != "" {
		if err := cm.store.EndConversation(ctx, cm.convID, ""); err != nil {
			// Non-fatal: log and continue — a new session must still start.
			logger.Log.Warn("memory: end conversation", "conv_id", cm.convID, "err", err)
		}
	}

	if err := cm.createSession(ctx); err != nil {
		return "", err
	}
	return cm.convID, nil
}

// createSession inserts a new Conversation row and persists the convID in
// agent_state so restarts can resume. Caller must hold mu write lock if
// called after initialisation.
func (cm *ConversationManager) createSession(ctx context.Context) error {
	convID := uuid.NewString()
	if err := cm.store.CreateConversation(ctx, &storage.Conversation{
		ID:        convID,
		AgentID:   sessionKey,
		StartedAt: storage.DBTime{Time: time.Now()},
	}); err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}

	if err := cm.store.SaveAgentState(ctx, &storage.AgentState{
		AgentID:               sessionKey,
		CurrentConversationID: convID,
		LastActive:            storage.DBTime{Time: time.Now()},
	}); err != nil {
		return fmt.Errorf("persist session state: %w", err)
	}

	cm.convID = convID
	logger.Log.Info("memory: new session", "conv_id", convID)
	return nil
}

// ── Message persistence ───────────────────────────────────────────────────────

// SaveUserMessage persists an incoming user message turn.
func (cm *ConversationManager) SaveUserMessage(ctx context.Context, agentID, content string) error {
	return cm.store.SaveMessage(ctx, &storage.Message{
		ID:             uuid.NewString(),
		ConversationID: cm.CurrentConvID(),
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
	agentID, content, reasoningContent string,
	toolCalls []tools.ToolCall,
) error {
	tcJSON := "[]"
	if len(toolCalls) > 0 {
		b, err := json.Marshal(toolCalls)
		if err != nil {
			return fmt.Errorf("memory: marshal tool calls: %w", err)
		}
		tcJSON = string(b)
	}
	return cm.store.SaveMessage(ctx, &storage.Message{
		ID:               uuid.NewString(),
		ConversationID:   cm.CurrentConvID(),
		AgentID:          agentID,
		Role:             "assistant",
		Content:          content,
		ReasoningContent: reasoningContent,
		ToolCalls:        tcJSON,
		Timestamp:        storage.DBTime{Time: time.Now()},
	})
}

// SaveToolResult persists a tool result turn.
// toolCallID must match the ToolCall.ID from the preceding assistant turn.
func (cm *ConversationManager) SaveToolResult(
	ctx context.Context,
	agentID, toolCallID, toolName, result string,
	isError bool,
) error {
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
		ConversationID: cm.CurrentConvID(),
		AgentID:        agentID,
		Role:           "tool",
		Content:        content,
		ToolCallID:     toolCallID,
		ToolCalls:      string(nameJSON),
		Timestamp:      storage.DBTime{Time: time.Now()},
	})
}

// ── History retrieval ─────────────────────────────────────────────────────────

// GetHistory returns the last `limit` messages in the current conversation,
// ready to be prepended with the agent's system prompt and passed to the LLM.
//
// Messages from other agents are given Name = agentID so the LLM understands
// they came from a different participant in the session.
func (cm *ConversationManager) GetHistory(
	ctx context.Context,
	agentID string,
	limit int,
) ([]llm.Message, error) {
	all, err := cm.store.GetMessages(ctx, cm.CurrentConvID())
	if err != nil {
		return nil, fmt.Errorf("memory: get history: %w", err)
	}

	// Sliding window: take the last `limit` messages.
	if len(all) > limit {
		all = all[len(all)-limit:]
	}

	out := make([]llm.Message, 0, len(all))
	for _, m := range all {
		msg, err := toMessage(m, agentID)
		if err != nil {
			logger.Log.Warn("memory: skipping malformed message", "id", m.ID, "err", err)
			continue
		}
		out = append(out, msg)
	}
	return out, nil
}

// toMessage converts a storage.Message to an llm.Message.
// Messages authored by a different agent are tagged with that agent's ID in
// the Name field so the LLM treats them as a distinct participant.
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
		// Recover the tool name we stored in ToolCalls.
		var nameHolder []struct {
			Name string `json:"name"`
		}
		toolName := ""
		if err := json.Unmarshal([]byte(m.ToolCalls), &nameHolder); err == nil && len(nameHolder) > 0 {
			toolName = nameHolder[0].Name
		}
		return llm.ToolResultMessage(m.ToolCallID, toolName, m.Content), nil

	default:
		return llm.Message{}, fmt.Errorf("unknown role %q", m.Role)
	}
}
