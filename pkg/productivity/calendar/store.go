package calendar

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
)

type Store interface {
	CreateEvent(ctx context.Context, event *CalendarEvent) error
	GetEvent(ctx context.Context, id string) (*CalendarEvent, error)
	UpdateEvent(ctx context.Context, event *CalendarEvent) error
	DeleteEvent(ctx context.Context, id string) error
	ListEvents(ctx context.Context, from, to time.Time) ([]*CalendarEvent, error)
	GetUpcomingEvents(ctx context.Context, limit int) ([]*CalendarEvent, error)
	SearchEvents(ctx context.Context, query string, limit int) ([]*CalendarEvent, error)
}

// CalendarEvent is a native calendar entry.
type CalendarEvent struct {
	ID          string                    `db:"id" json:"id"`
	Title       string                    `db:"title" json:"title"`
	Description string                    `db:"description" json:"description,omitempty"`
	StartTime   dbtypes.DBTime            `db:"start_time" json:"start_time"`
	EndTime     dbtypes.DBTime            `db:"end_time" json:"end_time"`
	AllDay      bool                      `db:"all_day" json:"all_day,omitempty"`
	Location    string                    `db:"location" json:"location,omitempty"`
	Recurrence  string                    `db:"recurrence" json:"recurrence,omitempty"`
	Tags        dbtypes.JSONSlice[string] `db:"tags" json:"tags,omitempty"`
	CreatedAt   dbtypes.DBTime            `db:"created_at" json:"created_at"`
	UpdatedAt   dbtypes.DBTime            `db:"updated_at" json:"updated_at"`
}
