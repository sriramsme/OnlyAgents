package conversation

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
)

// ConversationManager is shared across all agents. One instance lives in the
// kernel. Agents call it to save messages and retrieve history. Because it is
// a shared pointer, StartNewSession immediately affects every agent's next
// GetHistory call — no broadcast required.
type Manager struct {
	store Store
}

// New creates a ConversationManager. It resumes the last active session if one
// exists in the database, otherwise starts a fresh one.
func New(store Store) (*Manager, error) {
	cm := &Manager{store: store}
	return cm, nil
}

// NewSession creates session if it doesn't exist (idempotent)
func (cm *Manager) NewSession(ctx context.Context, payload *core.SessionNewPayload) (string, error) {
	id := uuid.NewString()
	err := cm.CreateConversation(ctx, &Conversation{
		ID:        id,
		Channel:   payload.Channel,
		AgentID:   payload.AgentID,
		ChatID:    payload.ChatID,
		StartedAt: dbtypes.DBTime{Time: time.Now()},
	})
	return id, err
}

// Explicit session reset (triggered by /new command etc.)
func (cm *Manager) EndSession(ctx context.Context, sessionID string) error {
	return cm.store.EndConversation(ctx, sessionID, "")
}

// Wrap Store interface methods

func (cm *Manager) CreateConversation(ctx context.Context, conv *Conversation) error {
	return cm.store.CreateConversation(ctx, conv)
}

func (cm *Manager) GetConversation(ctx context.Context, id string) (*Conversation, error) {
	return cm.store.GetConversation(ctx, id)
}

func (cm *Manager) GetActiveConversationByChannel(ctx context.Context, channel, agentID string) (*Conversation, error) {
	return cm.store.GetActiveConversationByChannel(ctx, channel, agentID)
}

func (cm *Manager) GetConversationsBetween(ctx context.Context, from, to time.Time) ([]*Conversation, error) {
	return cm.store.GetConversationsBetween(ctx, from, to)
}

func (cm *Manager) UpdateConversation(ctx context.Context, conv *Conversation) error {
	return cm.store.UpdateConversation(ctx, conv)
}

func (cm *Manager) ListConversations(ctx context.Context, limit int) ([]*Conversation, error) {
	return cm.store.ListConversations(ctx, limit)
}

func (cm *Manager) ListConversationsByChannel(ctx context.Context, channel string, limit int) ([]*Conversation, error) {
	return cm.store.ListConversationsByChannel(ctx, channel, limit)
}

func (cm *Manager) EndConversation(ctx context.Context, id string, summary string) error {
	return cm.store.EndConversation(ctx, id, summary)
}
