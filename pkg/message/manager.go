package message

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

const (
	DefaultHistoryTurns       = 10  // number of user→assistant exchanges to include
	ToolResultHistoryMaxBytes = 500 // tool result content cap when building history
)

type Manager struct {
	store Store
	mu    sync.Map // map[sessionID]*sync.RWMutex
}

func New(store Store) (*Manager, error) {
	return &Manager{store: store}, nil
}

// SaveUserMessage persists an incoming user message turn.
func (mm *Manager) SaveUserMessage(ctx context.Context, sessionID, agentID, platformMessageID, content string) error {
	lock := mm.lockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()
	return mm.SaveMessage(ctx, &Message{
		ID:                uuid.NewString(),
		ConversationID:    sessionID,
		AgentID:           agentID,
		Role:              "user",
		Content:           content,
		PlatformMessageID: platformMessageID,
		Timestamp:         dbtypes.DBTime{Time: time.Now()},
	})
}

func (mm *Manager) SaveNotificationMessage(ctx context.Context, sessionID, agentID, content string) (string, error) {
	lock := mm.lockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()
	id := uuid.NewString()
	return id, mm.SaveMessage(ctx, &Message{
		ID:             id,
		ConversationID: sessionID,
		AgentID:        agentID,
		Role:           "notification",
		Content:        content,
		Timestamp:      dbtypes.DBTime{Time: time.Now()},
	})
}

// SaveAssistantMessage persists an assistant turn. toolCalls may be nil for
// a plain text response.
func (mm *Manager) SaveAssistantMessage(
	ctx context.Context,
	sessionID, agentID, content, reasoningContent string,
	toolCalls []tools.ToolCall,
) (string, error) {
	return mm.saveAssistantMessage(ctx, sessionID, agentID, content,
		reasoningContent, toolCalls, time.Time{})
}

func (mm *Manager) SaveAssistantMessageAt(
	ctx context.Context,
	sessionID, agentID, content, reasoningContent string,
	toolCalls []tools.ToolCall,
	saveAt time.Time,
) (string, error) {
	return mm.saveAssistantMessage(ctx, sessionID, agentID, content,
		reasoningContent, toolCalls, saveAt)
}

// SaveAssistantMessage persists an assistant turn. toolCalls may be nil for
// a plain text response.
func (mm *Manager) saveAssistantMessage(
	ctx context.Context,
	sessionID, agentID, content, reasoningContent string,
	toolCalls []tools.ToolCall,
	saveAt time.Time,
) (string, error) {
	lock := mm.lockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()
	tcJSON := "[]"
	if len(toolCalls) > 0 {
		b, err := json.Marshal(toolCalls)
		if err != nil {
			return "", fmt.Errorf("memory: marshal tool calls: %w", err)
		}
		tcJSON = string(b)
	}
	if saveAt.IsZero() {
		saveAt = time.Now()
	}

	id := uuid.NewString()
	return id, mm.SaveMessage(ctx, &Message{
		ID:               id,
		ConversationID:   sessionID,
		AgentID:          agentID,
		Role:             "assistant",
		Content:          content,
		ReasoningContent: reasoningContent,
		ToolCalls:        tcJSON,
		Timestamp:        dbtypes.DBTime{Time: saveAt},
	})
}

// SaveToolResult persists a tool result turn.
// toolCallID must match the ToolCall.ID from the preceding assistant turn.
func (mm *Manager) SaveToolResult(
	ctx context.Context,
	sessionID, agentID, toolCallID, toolName, result string,
	isError bool,
) error {
	lock := mm.lockFor(sessionID)
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
	return mm.SaveMessage(ctx, &Message{
		ID:             uuid.NewString(),
		ConversationID: sessionID,
		AgentID:        agentID,
		Role:           "tool",
		Content:        content,
		ToolCallID:     toolCallID,
		ToolCalls:      string(nameJSON),
		Timestamp:      dbtypes.DBTime{Time: time.Now()},
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
// getHistory is the shared base for both GetHistory and GetFullHistory.
// messages is pre-filtered by the caller.
func (mm *Manager) getHistory(
	messages []*Message,
	agentID string,
	maxTurns int,
) ([]llm.Message, error) {
	var turns [][]*Message
	var current []*Message
	for _, m := range messages {
		if m.Role == "user" && len(current) > 0 {
			turns = append(turns, current)
			current = nil
		}
		current = append(current, m)
	}
	if len(current) > 0 {
		turns = append(turns, current)
	}
	if len(turns) > maxTurns {
		turns = turns[len(turns)-maxTurns:]
	}
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
	return sanitizeToolCallSequence(out), nil
}

// GetHistory returns history filtered to the agent's own messages.
func (mm *Manager) GetHistory(
	ctx context.Context,
	sessionID, agentID string,
) ([]llm.Message, error) {
	lock := mm.lockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()

	all, err := mm.GetMessagesByAgent(ctx, sessionID, agentID)
	if err != nil {
		return nil, fmt.Errorf("memory: get history: %w", err)
	}
	return mm.getHistory(all, agentID, DefaultHistoryTurns)
}

// GetFullHistory returns history across all agents, used by the executive
// to maintain full conversation context. Sub-agent tool internals are stripped
// — only their final assistant responses are included.
func (mm *Manager) GetFullHistory(
	ctx context.Context,
	sessionID, agentID string,
) ([]llm.Message, error) {
	lock := mm.lockFor(sessionID)
	lock.Lock()
	defer lock.Unlock()

	all, err := mm.GetMessages(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("memory: get full history: %w", err)
	}

	filtered := make([]*Message, 0, len(all))
	for _, m := range all {
		switch m.Role {
		case "user", "notification":
			filtered = append(filtered, m)
		case "assistant":
			filtered = append(filtered, m)
		case "tool":
			if m.AgentID == agentID {
				filtered = append(filtered, m)
			}
		}
	}
	return mm.getHistory(filtered, agentID, DefaultHistoryTurns)
}

func (mm *Manager) lockFor(sessionID string) *sync.RWMutex {
	mu, _ := mm.mu.LoadOrStore(sessionID, &sync.RWMutex{})
	return mu.(*sync.RWMutex)
}

// Wrap Store interface methods

func (mm *Manager) SaveMessage(ctx context.Context, msg *Message) error {
	return mm.store.SaveMessage(ctx, msg)
}

func (mm *Manager) GetMessages(ctx context.Context, conversationID string) ([]*Message, error) {
	return mm.store.GetMessages(ctx, conversationID)
}

func (mm *Manager) GetMessagesByAgent(ctx context.Context, conversationID, agentID string) ([]*Message, error) {
	return mm.store.GetMessagesByAgent(ctx, conversationID, agentID)
}

func (mm *Manager) GetMessagesBetween(ctx context.Context, roles []string, from, to time.Time) ([]*Message, error) {
	return mm.store.GetMessagesBetween(ctx, roles, from, to)
}

func (mm *Manager) DeleteOldMessages(ctx context.Context, olderThan time.Time) error {
	return mm.store.DeleteOldMessages(ctx, olderThan)
}

func (mm *Manager) UpdateMessagePlatformID(ctx context.Context, messageID, platformMessageID string) error {
	return mm.store.UpdateMessagePlatformID(ctx, messageID, platformMessageID)
}

func (mm *Manager) GetMessageByPlatformID(ctx context.Context, platformMessageID string) (*Message, error) {
	return mm.store.GetMessageByPlatformID(ctx, platformMessageID)
}
