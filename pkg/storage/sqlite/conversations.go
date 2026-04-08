package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/conversation"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// ── ConversationStore ─────────────────────────────────────────────────────────

func (d *DB) CreateConversation(ctx context.Context, conv *conversation.Conversation) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO conversations (id, channel, agent_id, chat_id, started_at, ended_at, context, summary, peer_agent_id)
		VALUES (:id, :channel , :agent_id, :chat_id, :started_at, :ended_at, :context, :summary, :peer_agent_id)
	`, conv)
	return wrap(err, "create conversation")
}

// GetActiveConversationByChannel returns the active conversation for a given channel and agent.
// agentID is optional. If empty, it returns the active conversation for the given channel.
func (d *DB) GetActiveConversationByChannel(ctx context.Context, channel, agentID string) (*conversation.Conversation, error) {
	var conv conversation.Conversation

	query := `
		SELECT * FROM conversations
		WHERE channel = ? AND ended_at IS NULL
	`
	args := []any{channel}

	if agentID != "" {
		query += " AND agent_id = ?"
		args = append(args, agentID)
	}

	query += " LIMIT 1"

	err := d.db.GetContext(ctx, &conv, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no active conversation found for channel %s", channel)
		}
		return nil, wrap(err, "get active conversation by channel")
	}

	return &conv, nil
}

func (d *DB) GetConversation(ctx context.Context, id string) (*conversation.Conversation, error) {
	var conv conversation.Conversation
	err := d.db.GetContext(ctx, &conv, `SELECT * FROM conversations WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get conversation")
	}
	return &conv, nil
}

func (d *DB) GetConversationsBetween(ctx context.Context, from, to time.Time) ([]*conversation.Conversation, error) {
	var convs []*conversation.Conversation

	err := d.db.SelectContext(ctx, &convs, `
		SELECT *
		FROM conversations
		WHERE started_at < ?
		  AND (ended_at IS NULL OR ended_at >= ?)
		ORDER BY started_at ASC
	`, storage.DBTime{Time: to}, storage.DBTime{Time: from})

	return convs, wrap(err, "get conversations by day")
}

func (d *DB) UpdateConversation(ctx context.Context, conv *conversation.Conversation) error {
	_, err := d.db.NamedExecContext(ctx, `
		UPDATE conversations
		SET ended_at = :ended_at, context = :context, summary = :summary
		WHERE id = :id
	`, conv)
	return wrap(err, "update conversation")
}

func (d *DB) ListConversations(ctx context.Context, limit int) ([]*conversation.Conversation, error) {
	var convs []*conversation.Conversation
	err := d.db.SelectContext(ctx, &convs, `
		SELECT * FROM conversations
		ORDER BY started_at DESC
		LIMIT ?
	`, limit)
	return convs, wrap(err, "list conversations")
}

func (d *DB) ListConversationsByChannel(ctx context.Context, channel string, limit int) ([]*conversation.Conversation, error) {
	var convs []*conversation.Conversation
	err := d.db.SelectContext(ctx, &convs, `
		SELECT * FROM conversations
		WHERE channel = ?
		ORDER BY started_at DESC
		LIMIT ?
	`, channel, limit)
	return convs, wrap(err, "list conversations")
}

func (d *DB) EndConversation(ctx context.Context, id string, summary string) error {
	now := storage.DBTime{Time: time.Now()}
	_, err := d.db.ExecContext(ctx, `
		UPDATE conversations SET ended_at = ?, summary = ? WHERE id = ?
	`, now, summary, id)
	return wrap(err, "end conversation")
}
