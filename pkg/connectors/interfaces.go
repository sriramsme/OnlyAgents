package connectors

import (
	"context"
	"io"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// Connector is the base interface all connectors must implement
type Connector interface {
	// Metadata
	Name() string
	Type() string

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
	CreateEvents(ctx context.Context, events []*storage.CalendarEvent) ([]*storage.CalendarEvent, []error)
	GetEvent(ctx context.Context, id string) (*storage.CalendarEvent, error)
	UpdateEvent(ctx context.Context, event *storage.CalendarEvent) (*storage.CalendarEvent, error)
	DeleteEvent(ctx context.Context, id string) error
	ListEvents(ctx context.Context, from, to time.Time) ([]*storage.CalendarEvent, error)
	GetUpcoming(ctx context.Context, limit int) ([]*storage.CalendarEvent, error)
	FindAvailableSlots(ctx context.Context, from, to time.Time, minDuration time.Duration) ([]TimeSlot, error)
}

// NotesConnector is implemented by native.NotesConnector.
type NotesConnector interface {
	Connector
	CreateNotes(ctx context.Context, notes []*storage.Note) ([]*storage.Note, []error)
	GetNote(ctx context.Context, id string) (*storage.Note, error)
	UpdateNote(ctx context.Context, note *storage.Note) (*storage.Note, error)
	DeleteNote(ctx context.Context, id string) error
	ListNotes(ctx context.Context) ([]*storage.Note, error)
	SearchNotes(ctx context.Context, query string) ([]*storage.Note, error)
	PinNote(ctx context.Context, id string, pinned bool) error
}

// RemindersConnector is implemented by native.RemindersConnector.
type RemindersConnector interface {
	Connector
	CreateReminders(ctx context.Context, rems []*storage.Reminder) ([]*storage.Reminder, []error)
	GetReminder(ctx context.Context, id string) (*storage.Reminder, error)
	UpdateReminder(ctx context.Context, rem *storage.Reminder) (*storage.Reminder, error)
	DeleteReminder(ctx context.Context, id string) error
	ListReminders(ctx context.Context) ([]*storage.Reminder, error)
}

// TasksConnector is implemented by native.TasksConnector.
type TasksConnector interface {
	Connector
	CreateProject(ctx context.Context, project *storage.Project) (*storage.Project, error)
	GetProject(ctx context.Context, id string) (*storage.Project, error)
	UpdateProject(ctx context.Context, project *storage.Project) (*storage.Project, error)
	DeleteProject(ctx context.Context, id string) error
	ListProjects(ctx context.Context) ([]*storage.Project, error)

	CreateTasks(ctx context.Context, task []*storage.Task) ([]*storage.Task, []error)
	GetTask(ctx context.Context, id string) (*storage.Task, error)
	UpdateTask(ctx context.Context, task *storage.Task) (*storage.Task, error)
	DeleteTask(ctx context.Context, id string) error
	CompleteTask(ctx context.Context, id string) error
	ListTasks(ctx context.Context, filter storage.TaskFilter) ([]*storage.Task, error)
	SearchTasks(ctx context.Context, query string) ([]*storage.Task, error)
	GetTodaysTasks(ctx context.Context) ([]*storage.Task, error)
	GetTasksByProject(ctx context.Context, projectID string, filter storage.TaskFilter) ([]*storage.Task, error)
	MoveToProject(ctx context.Context, taskID, projectID string) error
	SetPriority(ctx context.Context, id, priority string) error
}
