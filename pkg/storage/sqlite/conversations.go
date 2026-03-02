package sqlite

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// ── ConversationStore ─────────────────────────────────────────────────────────

func (d *DB) CreateConversation(ctx context.Context, conv *storage.Conversation) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO conversations (id, agent_id, started_at, ended_at, context, summary)
		VALUES (:id, :agent_id, :started_at, :ended_at, :context, :summary)
	`, conv)
	return wrap(err, "create conversation")
}

func (d *DB) GetConversation(ctx context.Context, id string) (*storage.Conversation, error) {
	var conv storage.Conversation
	err := d.db.GetContext(ctx, &conv, `SELECT * FROM conversations WHERE id = ?`, id)
	if err != nil {
		return nil, wrap(err, "get conversation")
	}
	return &conv, nil
}

func (d *DB) UpdateConversation(ctx context.Context, conv *storage.Conversation) error {
	_, err := d.db.NamedExecContext(ctx, `
		UPDATE conversations
		SET ended_at = :ended_at, context = :context, summary = :summary
		WHERE id = :id
	`, conv)
	return wrap(err, "update conversation")
}

func (d *DB) ListConversations(ctx context.Context, agentID string, limit int) ([]*storage.Conversation, error) {
	var convs []*storage.Conversation
	err := d.db.SelectContext(ctx, &convs, `
		SELECT * FROM conversations
		WHERE agent_id = ?
		ORDER BY started_at DESC
		LIMIT ?
	`, agentID, limit)
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
			(id, conversation_id, agent_id, role, content, reasoning_content, tool_calls, timestamp)
		VALUES
			(:id, :conversation_id, :agent_id, :role, :content, :reasoning_content, :tool_calls, :timestamp)
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

func (d *DB) GetRecentMessages(ctx context.Context, agentID string, since time.Time) ([]*storage.Message, error) {
	var msgs []*storage.Message
	sinceVal := storage.DBTime{Time: since}
	err := d.db.SelectContext(ctx, &msgs, `
		SELECT * FROM messages WHERE agent_id = ? AND timestamp >= ? ORDER BY timestamp ASC
	`, agentID, sinceVal)
	return msgs, wrap(err, "get recent messages")
}

func (d *DB) DeleteOldMessages(ctx context.Context, olderThan time.Time) error {
	val := storage.DBTime{Time: olderThan}
	_, err := d.db.ExecContext(ctx, `DELETE FROM messages WHERE timestamp < ?`, val)
	return wrap(err, "delete old messages")
}
