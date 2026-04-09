package sqlite

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/message"
)

func (d *DB) SaveMessage(ctx context.Context, msg *message.Message) error {
	_, err := d.db.NamedExecContext(ctx, `
		INSERT INTO messages
			(id, conversation_id, agent_id, platform_message_id, role, content, reasoning_content, tool_calls, tool_call_id, timestamp)
		VALUES
		(:id, :conversation_id, :agent_id, :platform_message_id, :role, :content, :reasoning_content, :tool_calls, :tool_call_id, :timestamp)
	`, msg)
	return wrap(err, "save message")
}

func (d *DB) GetMessages(ctx context.Context, conversationID string) ([]*message.Message, error) {
	var msgs []*message.Message
	err := d.db.SelectContext(ctx, &msgs, `
		SELECT * FROM messages WHERE conversation_id = ? ORDER BY timestamp ASC
	`, conversationID)
	return msgs, wrap(err, "get messages")
}

func (d *DB) GetMessagesByAgent(ctx context.Context, conversationID, agentID string) ([]*message.Message, error) {
	var msgs []*message.Message
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

func (d *DB) GetMessageByPlatformID(ctx context.Context, platformMessageID string) (*message.Message, error) {
	var msg message.Message
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
) ([]*message.Message, error) {
	var msgs []*message.Message

	query := `
		SELECT m.*
		FROM messages m
		JOIN conversations c ON m.conversation_id = c.id
		WHERE  m.timestamp >= ?
		  AND m.timestamp < ?
	`
	args := []any{
		dbtypes.DBTime{Time: from},
		dbtypes.DBTime{Time: to},
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
	val := dbtypes.DBTime{Time: olderThan}
	_, err := d.db.ExecContext(ctx, `DELETE FROM messages WHERE timestamp < ?`, val)
	return wrap(err, "delete old messages")
}
