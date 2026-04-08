package conversation

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
)

type Store interface {
	CreateConversation(ctx context.Context, conv *Conversation) error
	GetConversation(ctx context.Context, id string) (*Conversation, error)
	GetActiveConversationByChannel(ctx context.Context, channel, agentID string) (*Conversation, error)
	GetConversationsBetween(ctx context.Context, from, to time.Time) ([]*Conversation, error)
	UpdateConversation(ctx context.Context, conv *Conversation) error
	ListConversations(ctx context.Context, limit int) ([]*Conversation, error)
	ListConversationsByChannel(ctx context.Context, channel string, limit int) ([]*Conversation, error)
	EndConversation(ctx context.Context, id string, summary string) error
}

// Conversation is a single session between a user and an agent.
type Conversation struct {
	ID          string             `db:"id" json:"id"`
	Channel     string             `db:"channel" json:"channel"`
	AgentID     string             `db:"agent_id" json:"agent_id"`
	ChatID      string             `db:"chat_id" json:"chat_id"`
	StartedAt   dbtypes.DBTime     `db:"started_at" json:"started_at"`
	EndedAt     dbtypes.NullDBTime `db:"ended_at" json:"ended_at"`
	Context     dbtypes.JSONMap    `db:"context" json:"context,omitempty"`
	Summary     string             `db:"summary" json:"summary,omitempty"`
	PeerAgentID string             `db:"peer_agent_id" json:"peer_agent_id,omitempty"`
}
