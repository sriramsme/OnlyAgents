package connectors

import (
	"context"
	"io"

	"github.com/sriramsme/OnlyAgents/pkg/core"
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

// ====================
// Capability Interfaces
// ====================

// EmailConnector provides email capabilities
type EmailConnector interface {
	Connector
	SendEmail(ctx context.Context, req *SendEmailRequest) error
	GetEmail(ctx context.Context, id string) (*Email, error)
	SearchEmails(ctx context.Context, req *SearchEmailsRequest) ([]*Email, error)
	DeleteEmail(ctx context.Context, id string) error
	MarkAsRead(ctx context.Context, id string) error
	MarkAsUnread(ctx context.Context, id string) error
}

// CalendarConnector provides calendar capabilities
type CalendarConnector interface {
	Connector
	CreateEvent(ctx context.Context, event *CalendarEvent) (*CalendarEvent, error)
	GetEvent(ctx context.Context, id string) (*CalendarEvent, error)
	ListEvents(ctx context.Context, req *ListEventsRequest) ([]*CalendarEvent, error)
	UpdateEvent(ctx context.Context, id string, event *CalendarEvent) (*CalendarEvent, error)
	DeleteEvent(ctx context.Context, id string) error
	FindAvailableSlots(ctx context.Context, req *FindSlotsRequest) ([]*TimeSlot, error)
}

// WebSearchConnector provides web search capabilities
type WebSearchConnector interface {
	Connector
	Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
}

// WebFetchConnector provides web fetching capabilities
type WebFetchConnector interface {
	Connector
	Fetch(ctx context.Context, req *FetchRequest) (*FetchResponse, error)
}

// TaskConnector provides task management capabilities
type TaskConnector interface {
	Connector
	CreateTask(ctx context.Context, task *Task) (*Task, error)
	GetTask(ctx context.Context, id string) (*Task, error)
	ListTasks(ctx context.Context, req *ListTasksRequest) ([]*Task, error)
	UpdateTask(ctx context.Context, id string, task *Task) (*Task, error)
	CompleteTask(ctx context.Context, id string) error
	DeleteTask(ctx context.Context, id string) error
}

// StorageConnector provides file storage capabilities
type StorageConnector interface {
	Connector
	Upload(ctx context.Context, req *UploadRequest) (*FileInfo, error)
	Download(ctx context.Context, fileID string) (io.ReadCloser, error)
	Delete(ctx context.Context, fileID string) error
	List(ctx context.Context, req *ListFilesRequest) ([]*FileInfo, error)
	Search(ctx context.Context, query string, maxResults int) ([]*FileInfo, error)
	Share(ctx context.Context, fileID string, req *ShareRequest) (*ShareResponse, error)
}

// NotesConnector provides note-taking capabilities
type NotesConnector interface {
	Connector
	CreateNote(ctx context.Context, note *Note) (*Note, error)
	GetNote(ctx context.Context, id string) (*Note, error)
	UpdateNote(ctx context.Context, id string, note *Note) (*Note, error)
	DeleteNote(ctx context.Context, id string) error
	SearchNotes(ctx context.Context, query string) ([]*Note, error)
}

// ====================
// Helper Functions
// ====================

// SupportsCapability checks if a connector implements a specific capability
func SupportsCapability(conn Connector, capability core.Capability) bool {
	switch capability {
	case core.CapabilityEmail:
		_, ok := conn.(EmailConnector)
		return ok
	case core.CapabilityCalendar:
		_, ok := conn.(CalendarConnector)
		return ok
	case core.CapabilityWebSearch:
		_, ok := conn.(WebSearchConnector)
		return ok
	case core.CapabilityWebFetch:
		_, ok := conn.(WebFetchConnector)
		return ok
	case core.CapabilityTasks:
		_, ok := conn.(TaskConnector)
		return ok
	case core.CapabilityStorage:
		_, ok := conn.(StorageConnector)
		return ok
	case core.CapabilityNotes:
		_, ok := conn.(NotesConnector)
		return ok
	default:
		return false
	}
}

// GetCapabilities returns all capabilities a connector supports
func GetCapabilities(conn Connector) []core.Capability {
	var caps []core.Capability

	if _, ok := conn.(EmailConnector); ok {
		caps = append(caps, core.CapabilityEmail)
	}
	if _, ok := conn.(CalendarConnector); ok {
		caps = append(caps, core.CapabilityCalendar)
	}
	if _, ok := conn.(WebSearchConnector); ok {
		caps = append(caps, core.CapabilityWebSearch)
	}
	if _, ok := conn.(WebFetchConnector); ok {
		caps = append(caps, core.CapabilityWebFetch)
	}
	if _, ok := conn.(TaskConnector); ok {
		caps = append(caps, core.CapabilityTasks)
	}
	if _, ok := conn.(StorageConnector); ok {
		caps = append(caps, core.CapabilityStorage)
	}
	if _, ok := conn.(NotesConnector); ok {
		caps = append(caps, core.CapabilityNotes)
	}

	return caps
}
