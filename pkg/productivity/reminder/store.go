package reminder

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
)

type Store interface {
	CreateReminder(ctx context.Context, r *Reminder) error
	GetReminder(ctx context.Context, id string) (*Reminder, error)
	UpdateReminder(ctx context.Context, r *Reminder) error
	DeleteReminder(ctx context.Context, id string) error
	ListReminders(ctx context.Context) ([]*Reminder, error)
	GetDueReminders(ctx context.Context, before time.Time) ([]*Reminder, error)
	MarkReminderSent(ctx context.Context, id string, sentAt time.Time) error
}

// Reminder is a one-shot or recurring reminder delivered via the agent's channel.
type Reminder struct {
	ID        string             `db:"id" json:"id"`
	Title     string             `db:"title" json:"title"`
	Body      string             `db:"body" json:"body,omitempty"`
	DueAt     dbtypes.DBTime     `db:"due_at" json:"due_at"`
	SentAt    dbtypes.NullDBTime `db:"sent_at" json:"sent_at"`
	Recurring string             `db:"recurring" json:"recurring,omitempty"`
	CreatedAt dbtypes.DBTime     `db:"created_at" json:"created_at"`
}
