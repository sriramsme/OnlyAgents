package message

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
)

type Store interface {
	SaveMessage(ctx context.Context, msg *Message) error
	GetMessages(ctx context.Context, conversationID string) ([]*Message, error)
	GetMessagesByAgent(ctx context.Context, conversationID, agentID string) ([]*Message, error)
	GetMessagesBetween(ctx context.Context, roles []string, from, to time.Time) ([]*Message, error)
	DeleteOldMessages(ctx context.Context, olderThan time.Time) error
	// After send, update the existing message record with the platform ID
	UpdateMessagePlatformID(ctx context.Context, messageID, platformMessageID string) error

	// Lookup by platform ID — caller reads .AgentID from the result
	GetMessageByPlatformID(ctx context.Context, platformMessageID string) (*Message, error)
	LastMessageBefore(ctx context.Context, before time.Time, roles []string) (*Message, error)
}

// Message is one turn within a Conversation.
type Message struct {
	ID                string         `db:"id" json:"id"`
	ConversationID    string         `db:"conversation_id" json:"conversation_id"`
	PlatformMessageID string         `db:"platform_message_id" json:"platform_message_id,omitempty"`
	AgentID           string         `db:"agent_id" json:"agent_id"`
	Role              string         `db:"role" json:"role"`
	Content           string         `db:"content" json:"content,omitempty"`
	ReasoningContent  string         `db:"reasoning_content" json:"reasoning_content,omitempty"`
	ToolCalls         string         `db:"tool_calls" json:"tool_calls,omitempty"`
	ToolCallID        string         `db:"tool_call_id" json:"tool_call_id,omitempty"`
	Timestamp         dbtypes.DBTime `db:"timestamp" json:"timestamp"`
}
