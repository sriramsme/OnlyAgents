package local

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

type RemindersConnector struct {
	store storage.ReminderStore
	name  string
	id    string
}

func NewRemindersConnector(store storage.ReminderStore) connectors.RemindersConnector {
	return &RemindersConnector{
		store: store,
		name:  "Local Reminders",
		id:    "local_reminders",
	}
}

// ====================
// Connector Interface
// ====================

func (g *RemindersConnector) Name() string                   { return g.name }
func (g *RemindersConnector) ID() string                     { return g.id }
func (g *RemindersConnector) Type() connectors.ConnectorType { return connectors.ConnectorTypeLocal }
func (g *RemindersConnector) Kind() string                   { return "reminders" }

func (g *RemindersConnector) Connect() error {
	return nil
}

func (g *RemindersConnector) Disconnect() error {
	return nil
}

func (g *RemindersConnector) Start() error {
	return nil
}

func (g *RemindersConnector) Stop() error {
	return nil
}

func (g *RemindersConnector) HealthCheck() error {
	return nil
}

// createOne is internal — used by CreateReminders.
func (r *RemindersConnector) createOne(ctx context.Context, rem storage.Reminder) (*storage.Reminder, error) {
	if rem.Title == "" {
		return nil, fmt.Errorf("reminders: title is required")
	}
	if rem.DueAt.Before(time.Now()) {
		return nil, fmt.Errorf("reminders: due_at must be in the future")
	}

	now := storage.DBTime{Time: time.Now()}
	rem.ID = uuid.NewString()
	rem.CreatedAt = now

	if err := r.store.CreateReminder(ctx, &rem); err != nil {
		return nil, err
	}

	return &rem, nil
}

// CreateReminders is the public batch method.
// Returns all created reminders and a slice of errors for failures.
func (r *RemindersConnector) CreateReminders(ctx context.Context, reminders []*storage.Reminder) ([]*storage.Reminder, []error) {
	results := make([]*storage.Reminder, 0, len(reminders))
	var errs []error

	for _, rem := range reminders {
		created, err := r.createOne(ctx, *rem)
		if err != nil {
			errs = append(errs, fmt.Errorf("reminder %q: %w", rem.Title, err))
			continue
		}
		results = append(results, created)
	}

	return results, errs
}

func (r *RemindersConnector) GetReminder(ctx context.Context, id string) (*storage.Reminder, error) {
	return r.store.GetReminder(ctx, id)
}

func (r *RemindersConnector) UpdateReminder(ctx context.Context, rem *storage.Reminder) (*storage.Reminder, error) {
	if err := r.store.UpdateReminder(ctx, rem); err != nil {
		return nil, err
	}
	return r.store.GetReminder(ctx, rem.ID)
}

func (r *RemindersConnector) DeleteReminder(ctx context.Context, id string) error {
	return r.store.DeleteReminder(ctx, id)
}

func (r *RemindersConnector) ListReminders(ctx context.Context) ([]*storage.Reminder, error) {
	return r.store.ListReminders(ctx)
}
