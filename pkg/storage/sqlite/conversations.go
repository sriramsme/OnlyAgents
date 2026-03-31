package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// ── ConversationStore ─────────────────────────────────────────────────────────

func (d *DB) CreateConversation(ctx context.Context, conv *storage.Conversation) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO conversations (id, channel, agent_id, chat_id, started_at, ended_at, context, summary, peer_agent_id)
		VALUES (:id, :channel , :agent_id, :chat_id, :started_at, :ended_at, :context, :summary, :peer_agent_id)
	`, conv)
	return wrap(err, "create conversation")
}

func (d *DB) GetConversationByChannel(ctx context.Context, channel, agentID string) (*storage.Conversation, error) {
	var conv storage.Conversation

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
			return nil, fmt.Errorf("no conversation found for channel %s", channel)
		}
		return nil, wrap(err, "get conversation by channel")
	}

	return &conv, nil
}

func (d *DB) GetConversation(ctx context.Context, id string) (*storage.Conversation, error) {
	var conv storage.Conversation
	err := d.db.GetContext(ctx, &conv, `SELECT * FROM conversations WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get conversation")
	}
	return &conv, nil
}

func (d *DB) GetConversationsByDay(ctx context.Context, from, to time.Time) ([]*storage.Conversation, error) {
	var convs []*storage.Conversation

	err := d.db.SelectContext(ctx, &convs, `
		SELECT *
		FROM conversations
		WHERE started_at < ?
		  AND (ended_at IS NULL OR ended_at >= ?)
		ORDER BY started_at ASC
	`, storage.DBTime{Time: to}, storage.DBTime{Time: from})

	return convs, wrap(err, "get conversations by day")
}

func (d *DB) UpdateConversation(ctx context.Context, conv *storage.Conversation) error {
	_, err := d.db.NamedExecContext(ctx, `
		UPDATE conversations
		SET ended_at = :ended_at, context = :context, summary = :summary
		WHERE id = :id
	`, conv)
	return wrap(err, "update conversation")
}

func (d *DB) ListConversations(ctx context.Context, limit int) ([]*storage.Conversation, error) {
	var convs []*storage.Conversation
	err := d.db.SelectContext(ctx, &convs, `
		SELECT * FROM conversations
		ORDER BY started_at DESC
		LIMIT ?
	`, limit)
	return convs, wrap(err, "list conversations")
}

func (d *DB) ListConversationsByChannel(ctx context.Context, channel string, limit int) ([]*storage.Conversation, error) {
	var convs []*storage.Conversation
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

// ── MessageStore ──────────────────────────────────────────────────────────────

func (d *DB) SaveMessage(ctx context.Context, msg *storage.Message) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO messages
			(id, conversation_id, agent_id, platform_message_id, role, content, reasoning_content, tool_calls, tool_call_id, timestamp)
		VALUES
		(:id, :conversation_id, :agent_id, :platform_message_id, :role, :content, :reasoning_content, :tool_calls, :tool_call_id, :timestamp)
	`, msg)
	return wrap(err, "save message")
}

func (d *DB) GetMessages(ctx context.Context, conversationID string) ([]*storage.Message, error) {
	var msgs []*storage.Message
	err := d.db.SelectContext(ctx, &msgs, `
		SELECT * FROM messages WHERE conversation_id = ? ORDER BY timestamp ASC
	`, conversationID)
	return msgs, wrap(err, "get messages")
}

func (d *DB) GetMessagesByAgent(ctx context.Context, conversationID, agentID string) ([]*storage.Message, error) {
	var msgs []*storage.Message
	err := d.db.SelectContext(ctx, &msgs, `
        SELECT * FROM messages
        WHERE conversation_id = ? AND agent_id = ?
        ORDER BY timestamp ASC
    `, conversationID, agentID)
	return msgs, wrap(err, "get messages by agent")
}

func (d *DB) UpdateMessagePlatformID(ctx context.Context, messageID, platformMessageID string) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE messages SET platform_message_id = ? WHERE id = ?`,
		platformMessageID, messageID)
	return wrap(err, "update message platform id")
}

func (d *DB) GetMessageByPlatformID(ctx context.Context, platformMessageID string) (*storage.Message, error) {
	var msg storage.Message
	err := d.db.GetContext(ctx, &msg,
		`SELECT * FROM messages WHERE platform_message_id = ? LIMIT 1`,
		platformMessageID)
	return &msg, wrap(err, "get message by platform id")
}

// GetMessagesBetween returns all messages between the given times.
// If roles is non-empty, only messages with those roles are returned.
func (d *DB) GetMessagesBetween(
	ctx context.Context,
	roles []string,
	from, to time.Time,
) ([]*storage.Message, error) {
	var msgs []*storage.Message

	query := `
		SELECT m.*
		FROM messages m
		JOIN conversations c ON m.conversation_id = c.id
		WHERE  m.timestamp >= ?
		  AND m.timestamp < ?
	`
	args := []any{
		storage.DBTime{Time: from},
		storage.DBTime{Time: to},
	}

	// Optional role filter
	if len(roles) > 0 {
		query += " AND m.role IN (?)"
		args = append(args, roles)
	}

	query += " ORDER BY m.timestamp ASC"

	// Expand IN clause safely
	var err error
	query, args, err = sqlx.In(query, args...)
	if err != nil {
		return nil, err
	}

	query = d.db.Rebind(query)

	err = d.db.SelectContext(ctx, &msgs, query, args...)
	return msgs, wrap(err, "get messages between")
}

func (d *DB) DeleteOldMessages(ctx context.Context, olderThan time.Time) error {
	val := storage.DBTime{Time: olderThan}
	_, err := d.db.ExecContext(ctx, `DELETE FROM messages WHERE timestamp < ?`, val)
	return wrap(err, "delete old messages")
}
