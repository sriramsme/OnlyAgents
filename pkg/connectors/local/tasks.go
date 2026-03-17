package local

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

type TasksConnector struct {
	store storage.TaskStore
	name  string
	id    string
}

func NewTasksConnector(store storage.TaskStore) *TasksConnector {
	return &TasksConnector{
		store: store,
		name:  "Local Tasks",
		id:    "local_tasks",
	}
}

// ====================
// Connector Interface
// ====================

func (g *TasksConnector) Name() string                   { return g.name }
func (g *TasksConnector) ID() string                     { return g.id }
func (g *TasksConnector) Type() connectors.ConnectorType { return connectors.ConnectorTypeLocal }
func (g *TasksConnector) Kind() string                   { return "tasks" }

func (g *TasksConnector) Connect() error {
	return nil
}

func (g *TasksConnector) Disconnect() error {
	return nil
}

func (g *TasksConnector) Start() error {
	return nil
}

func (g *TasksConnector) Stop() error {
	return nil
}

func (g *TasksConnector) HealthCheck() error {
	return nil
}

func (c *TasksConnector) CreateProject(ctx context.Context, project *storage.Project) (*storage.Project, error) {
	if project.Name == "" {
		return nil, fmt.Errorf("tasks: project name is required")
	}
	now := storage.DBTime{Time: time.Now()}
	project.ID = uuid.NewString()
	if project.Color == "" {
		project.Color = "#6366f1"
	}
	project.CreatedAt = now
	project.UpdatedAt = now
	if err := c.store.CreateProject(ctx, project); err != nil {
		return nil, err
	}
	return project, nil
}

func (c *TasksConnector) GetProject(ctx context.Context, id string) (*storage.Project, error) {
	return c.store.GetProject(ctx, id)
}

func (c *TasksConnector) UpdateProject(ctx context.Context, project *storage.Project) (*storage.Project, error) {
	if err := c.store.UpdateProject(ctx, project); err != nil {
		return nil, err
	}
	return c.store.GetProject(ctx, project.ID)
}

func (c *TasksConnector) DeleteProject(ctx context.Context, id string) error {
	return c.store.DeleteProject(ctx, id)
}

func (c *TasksConnector) ListProjects(ctx context.Context) ([]*storage.Project, error) {
	return c.store.ListProjects(ctx)
}

func (c *TasksConnector) createTask(ctx context.Context, task *storage.Task) (*storage.Task, error) {
	if task.Title == "" {
		return nil, fmt.Errorf("tasks: title is required")
	}
	if task.ProjectID != "" {
		if _, err := c.store.GetProject(ctx, task.ProjectID); err != nil {
			return nil, fmt.Errorf("tasks: project %q not found", task.ProjectID)
		}
	}
	now := storage.DBTime{Time: time.Now()}
	task.ID = uuid.NewString()
	if task.Status == "" {
		task.Status = "todo"
	}
	if task.Priority == "" {
		task.Priority = "medium"
	}
	task.CreatedAt = now
	task.UpdatedAt = now
	if err := c.store.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// CreateTasks is the public method called by the skill.
// Returns all created tasks and a combined error if any failed.
func (c *TasksConnector) CreateTasks(ctx context.Context, tasks []*storage.Task) ([]*storage.Task, []error) {
	results := make([]*storage.Task, 0, len(tasks))
	var errs []error
	for _, t := range tasks {
		created, err := c.createTask(ctx, t)
		if err != nil {
			errs = append(errs, fmt.Errorf("task %q: %w", t.Title, err))
			continue
		}
		results = append(results, created)
	}
	return results, errs
}

func (c *TasksConnector) GetTask(ctx context.Context, id string) (*storage.Task, error) {
	return c.store.GetTask(ctx, id)
}

func (c *TasksConnector) UpdateTask(ctx context.Context, task *storage.Task) (*storage.Task, error) {
	if err := c.store.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	return c.store.GetTask(ctx, task.ID)
}

func (c *TasksConnector) DeleteTask(ctx context.Context, id string) error {
	return c.store.DeleteTask(ctx, id)
}

func (c *TasksConnector) CompleteTask(ctx context.Context, id string) error {
	return c.store.CompleteTask(ctx, id)
}

func (c *TasksConnector) ListTasks(ctx context.Context, filter storage.TaskFilter) ([]*storage.Task, error) {
	return c.store.ListTasks(ctx, filter)
}

func (c *TasksConnector) SearchTasks(ctx context.Context, query string) ([]*storage.Task, error) {
	return c.store.SearchTasks(ctx, query)
}

func (c *TasksConnector) GetTodaysTasks(ctx context.Context) ([]*storage.Task, error) {
	return c.store.GetTasksDueOn(ctx, time.Now())
}

func (c *TasksConnector) GetTasksByProject(ctx context.Context, projectID string, filter storage.TaskFilter) ([]*storage.Task, error) {
	filter.ProjectID = &projectID
	return c.store.ListTasks(ctx, filter)
}

func (c *TasksConnector) SetPriority(ctx context.Context, id, priority string) error {
	task, err := c.store.GetTask(ctx, id)
	if err != nil {
		return err
	}
	task.Priority = priority
	return c.store.UpdateTask(ctx, task)
}

func (c *TasksConnector) MoveToProject(ctx context.Context, taskID, projectID string) error {
	task, err := c.store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if projectID != "" {
		if _, err := c.store.GetProject(ctx, projectID); err != nil {
			return fmt.Errorf("tasks: project %q not found", projectID)
		}
	}
	task.ProjectID = projectID
	return c.store.UpdateTask(ctx, task)
}
