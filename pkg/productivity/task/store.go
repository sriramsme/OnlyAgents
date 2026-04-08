package task

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
)

// TaskStore manages tasks with optional project grouping.
// TaskFilter fields are all optional — nil means no filter on that field.
type Store interface {
	CreateTask(ctx context.Context, task *Task) error
	GetTask(ctx context.Context, id string) (*Task, error)
	UpdateTask(ctx context.Context, task *Task) error
	DeleteTask(ctx context.Context, id string) error
	CompleteTask(ctx context.Context, id string) error
	ListTasks(ctx context.Context, filter TaskFilter) ([]*Task, error)
	SearchTasks(ctx context.Context, query string) ([]*Task, error)
	GetTasksDueOn(ctx context.Context, date time.Time) ([]*Task, error)

	CreateProject(ctx context.Context, project *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	UpdateProject(ctx context.Context, project *Project) error
	DeleteProject(ctx context.Context, id string) error
	ListProjects(ctx context.Context) ([]*Project, error)
}

// Project groups related tasks together.
type Project struct {
	ID          string         `db:"id" json:"id"`
	Name        string         `db:"name" json:"name"`
	Description string         `db:"description" json:"description,omitempty"`
	Color       string         `db:"color" json:"color,omitempty"`
	CreatedAt   dbtypes.DBTime `db:"created_at" json:"created_at"`
	UpdatedAt   dbtypes.DBTime `db:"updated_at" json:"updated_at"`
}

// Task is a work item with optional project grouping and due date.
// Status: todo | in_progress | done | cancelled
// Priority: low | medium | high
type Task struct {
	ID          string                    `db:"id" json:"id"`
	ProjectID   string                    `db:"project_id" json:"project_id,omitempty"`
	Title       string                    `db:"title" json:"title"`
	Body        string                    `db:"body" json:"body,omitempty"`
	Status      string                    `db:"status" json:"status"`
	Priority    string                    `db:"priority" json:"priority,omitempty"`
	DueAt       dbtypes.NullDBTime        `db:"due_at" json:"due_at"`
	CompletedAt dbtypes.NullDBTime        `db:"completed_at" json:"completed_at"`
	Tags        dbtypes.JSONSlice[string] `db:"tags" json:"tags,omitempty"`
	CreatedAt   dbtypes.DBTime            `db:"created_at" json:"created_at"`
	UpdatedAt   dbtypes.DBTime            `db:"updated_at" json:"updated_at"`
}

// TaskFilter is used by ListTasks. All fields are optional — nil = no filter.
type TaskFilter struct {
	ProjectID *string    // filter by project; use pointer to "" to filter unprojectd tasks
	Status    *string    // "todo" | "in_progress" | "done" | "cancelled"
	Priority  *string    // "low" | "medium" | "high"
	DueFrom   *time.Time // inclusive lower bound on due_at
	DueTo     *time.Time // inclusive upper bound on due_at
}
