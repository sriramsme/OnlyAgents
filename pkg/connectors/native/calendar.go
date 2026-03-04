package native

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

type CalendarConnector struct {
	store storage.CalendarStore
	name  string
}

func NewCalendarConnector(store storage.CalendarStore) connectors.Connector {
	return &CalendarConnector{
		store: store,
		name:  "native_calendar",
	}
}

// ====================
// Connector Interface
// ====================

func (g *CalendarConnector) Name() string { return g.name }
func (g *CalendarConnector) Type() string { return "calendar" }

func (g *CalendarConnector) Connect() error {
	return nil
}

func (g *CalendarConnector) Disconnect() error {
	return nil
}

func (g *CalendarConnector) Start() error {
	return nil
}

func (g *CalendarConnector) Stop() error {
	return nil
}

func (g *CalendarConnector) HealthCheck() error {
	return nil
}

// createOne is internal — used by CreateEvents.
func (c *CalendarConnector) createOne(ctx context.Context, event storage.CalendarEvent) (*storage.CalendarEvent, error) {
	if event.Title == "" {
		return nil, fmt.Errorf("calendar: title is required")
	}
	if event.EndTime.Before(event.StartTime.Time) {
		return nil, fmt.Errorf("calendar: end_time must be after start_time")
	}

	now := storage.DBTime{Time: time.Now()}
	event.ID = uuid.NewString()
	event.CreatedAt = now
	event.UpdatedAt = now

	if err := c.store.CreateEvent(ctx, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// CreateEvents is the public batch method.
// Returns all created events and a slice of errors for failures.
func (c *CalendarConnector) CreateEvents(ctx context.Context, events []*storage.CalendarEvent) ([]*storage.CalendarEvent, []error) {
	results := make([]*storage.CalendarEvent, 0, len(events))
	var errs []error

	for _, e := range events {
		created, err := c.createOne(ctx, *e)
		if err != nil {
			errs = append(errs, fmt.Errorf("event %q: %w", e.Title, err))
			continue
		}
		results = append(results, created)
	}

	return results, errs
}

func (c *CalendarConnector) GetEvent(ctx context.Context, id string) (*storage.CalendarEvent, error) {
	return c.store.GetEvent(ctx, id)
}

func (c *CalendarConnector) UpdateEvent(ctx context.Context, event *storage.CalendarEvent) (*storage.CalendarEvent, error) {
	if err := c.store.UpdateEvent(ctx, event); err != nil {
		return nil, err
	}
	return c.store.GetEvent(ctx, event.ID)
}

func (c *CalendarConnector) DeleteEvent(ctx context.Context, id string) error {
	return c.store.DeleteEvent(ctx, id)
}

func (c *CalendarConnector) ListEvents(ctx context.Context, from, to time.Time) ([]*storage.CalendarEvent, error) {
	return c.store.ListEvents(ctx, from, to)
}

func (c *CalendarConnector) GetUpcoming(ctx context.Context, limit int) ([]*storage.CalendarEvent, error) {
	return c.store.GetUpcomingEvents(ctx, limit)
}

func (c *CalendarConnector) FindAvailableSlots(
	ctx context.Context,
	from, to time.Time,
	minDuration time.Duration,
) ([]TimeSlot, error) {
	events, err := c.store.ListEvents(ctx, from, to)
	if err != nil {
		return nil, err
	}
	var slots []TimeSlot
	cursor := from
	for _, e := range events {
		if e.StartTime.After(cursor) {
			gap := e.StartTime.Sub(cursor)
			if gap >= minDuration {
				slots = append(slots, TimeSlot{Start: cursor, End: e.StartTime.Time, Duration: gap})
			}
		}
		if e.EndTime.After(cursor) {
			cursor = e.EndTime.Time
		}
	}
	if to.After(cursor) && to.Sub(cursor) >= minDuration {
		slots = append(slots, TimeSlot{Start: cursor, End: to, Duration: to.Sub(cursor)})
	}
	return slots, nil
}

type TimeSlot struct {
	Start    time.Time
	End      time.Time
	Duration time.Duration
}
