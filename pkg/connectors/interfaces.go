package connectors

import (
	"context"
	"io"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/productivity/calendar"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/notes"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/reminder"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/task"
)

type ConnectorType string

const (
	ConnectorTypeLocal   ConnectorType = "local"   // uses local db
	ConnectorTypeService ConnectorType = "service" // uses external service
)

// Connector is the base interface all connectors must implement
type Connector interface {
	// Metadata
	Name() string
	ID() string
	Type() ConnectorType
	Kind() string

	// Lifecycle
	Connect() error
	Disconnect() error
	Start() error
	Stop() error

	// Health
	HealthCheck() error
}

// EmailConnector
type EmailConnector interface {
	Connector
	SendEmail(ctx context.Context, req *SendEmailRequest) error
	GetEmail(ctx context.Context, id string) (*Email, error)
	DraftEmail(ctx context.Context, req *SendEmailRequest) (*Email, error)
	SearchEmails(ctx context.Context, req *SearchEmailsRequest) ([]*Email, error)
	DeleteEmail(ctx context.Context, id string) error
	MarkAsRead(ctx context.Context, id string) error
	MarkAsUnread(ctx context.Context, id string) error
}

// WebSearchConnector
type WebSearchConnector interface {
	Connector
	Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
}

// WebFetchConnector
type WebFetchConnector interface {
	Connector
	Fetch(ctx context.Context, req *FetchRequest) (*FetchResponse, error)
}

// StorageConnector
type StorageConnector interface {
	Connector
	Upload(ctx context.Context, req *UploadRequest) (*FileInfo, error)
	Download(ctx context.Context, fileID string) (io.ReadCloser, error)
	Delete(ctx context.Context, fileID string) error
	List(ctx context.Context, req *ListFilesRequest) ([]*FileInfo, error)
	Search(ctx context.Context, query string, maxResults int) ([]*FileInfo, error)
	Share(ctx context.Context, fileID string, req *ShareRequest) (*ShareResponse, error)
}

// CalendarConnector is implemented by native.CalendarConnector and any future
// external calendar connectors (Google Calendar, etc.).
type CalendarConnector interface {
	Connector
	CreateEvents(ctx context.Context, events []*calendar.CalendarEvent) ([]*calendar.CalendarEvent, []error)
	GetEvent(ctx context.Context, id string) (*calendar.CalendarEvent, error)
	UpdateEvent(ctx context.Context, event *calendar.CalendarEvent) (*calendar.CalendarEvent, error)
	DeleteEvent(ctx context.Context, id string) error
	ListEvents(ctx context.Context, from, to time.Time) ([]*calendar.CalendarEvent, error)
	GetUpcoming(ctx context.Context, limit int) ([]*calendar.CalendarEvent, error)
	FindAvailableSlots(ctx context.Context, from, to time.Time, minDuration time.Duration) ([]TimeSlot, error)
}

// NotesConnector is implemented by native.NotesConnector.
type NotesConnector interface {
	Connector
	CreateNotes(ctx context.Context, notes []*notes.Note) ([]*notes.Note, []error)
	GetNote(ctx context.Context, id string) (*notes.Note, error)
	UpdateNote(ctx context.Context, note *notes.Note) (*notes.Note, error)
	DeleteNote(ctx context.Context, id string) error
	ListNotes(ctx context.Context) ([]*notes.Note, error)
	SearchNotes(ctx context.Context, query string) ([]*notes.Note, error)
	PinNote(ctx context.Context, id string, pinned bool) error
}

// RemindersConnector is implemented by native.RemindersConnector.
type RemindersConnector interface {
	Connector
	CreateReminders(ctx context.Context, rems []*reminder.Reminder) ([]*reminder.Reminder, []error)
	GetReminder(ctx context.Context, id string) (*reminder.Reminder, error)
	UpdateReminder(ctx context.Context, rem *reminder.Reminder) (*reminder.Reminder, error)
	DeleteReminder(ctx context.Context, id string) error
	ListReminders(ctx context.Context) ([]*reminder.Reminder, error)
}

// TasksConnector is implemented by native.TasksConnector.
type TasksConnector interface {
	Connector
	CreateProject(ctx context.Context, project *task.Project) (*task.Project, error)
	GetProject(ctx context.Context, id string) (*task.Project, error)
	UpdateProject(ctx context.Context, project *task.Project) (*task.Project, error)
	DeleteProject(ctx context.Context, id string) error
	ListProjects(ctx context.Context) ([]*task.Project, error)

	CreateTasks(ctx context.Context, task []*task.Task) ([]*task.Task, []error)
	GetTask(ctx context.Context, id string) (*task.Task, error)
	UpdateTask(ctx context.Context, task *task.Task) (*task.Task, error)
	DeleteTask(ctx context.Context, id string) error
	CompleteTask(ctx context.Context, id string) error
	ListTasks(ctx context.Context, filter task.TaskFilter) ([]*task.Task, error)
	SearchTasks(ctx context.Context, query string) ([]*task.Task, error)
	GetTodaysTasks(ctx context.Context) ([]*task.Task, error)
	GetTasksByProject(ctx context.Context, projectID string, filter task.TaskFilter) ([]*task.Task, error)
	MoveToProject(ctx context.Context, taskID, projectID string) error
	SetPriority(ctx context.Context, id, priority string) error
}
