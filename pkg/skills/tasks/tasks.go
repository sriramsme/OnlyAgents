package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

func init() {
	skills.Register("tasks", NewTasksSkill)
}

type TasksSkill struct {
	ctx    context.Context
	cancel context.CancelFunc
	*skills.BaseSkill
	conn connectors.TasksConnector
}

func NewTasksSkill(ctx context.Context, cfg config.SkillConfig, conn connectors.Connector) (skills.Skill, error) {
	tasksConn, ok := conn.(connectors.TasksConnector)
	if !ok {
		return nil, fmt.Errorf("tasks skill: connector is not a TasksConnector")
	}
	base := skills.NewBaseSkill(cfg, skills.SkillTypeNative)
	skillCtx, cancel := context.WithCancel(ctx)
	return &TasksSkill{
		BaseSkill: base,
		conn:      tasksConn,
		ctx:       skillCtx,
		cancel:    cancel,
	}, nil
}

func (s *TasksSkill) Initialize() error {
	return nil
}

func (s *TasksSkill) Shutdown() error {
	s.cancel()
	return nil
}

func (s *TasksSkill) Tools() []tools.ToolDef {
	return tools.GetTasksTools()
}

// nolint:gocyclo
func (s *TasksSkill) Execute(ctx context.Context, toolName string, args []byte) (any, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("tasks skill: connector not initialized")
	}
	switch toolName {
	// Projects
	case "project_create":
		return s.createProject(ctx, args)
	case "project_update":
		return s.updateProject(ctx, args)
	case "project_delete":
		return s.deleteProject(ctx, args)
	case "project_get":
		return s.getProject(ctx, args)
	case "project_list":
		return s.conn.ListProjects(ctx)
	// Tasks
	case "tasks_create":
		return s.createTasks(ctx, args)
	case "task_update":
		return s.updateTask(ctx, args)
	case "task_get":
		return s.getTask(ctx, args)
	case "task_delete":
		return s.deleteTask(ctx, args)
	case "task_complete":
		return s.completeTask(ctx, args)
	case "task_list":
		return s.listTasks(ctx, args)
	case "task_search":
		return s.searchTasks(ctx, args)
	case "task_today":
		return s.conn.GetTodaysTasks(ctx)
	case "task_move":
		return s.moveTask(ctx, args)
	case "task_set_priority":
		return s.setPriority(ctx, args)
	default:
		return nil, fmt.Errorf("tasks skill: unknown tool %q", toolName)
	}
}

// ── Projects ──────────────────────────────────────────────────────────────────

func (s *TasksSkill) createProject(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.ProjectCreateInput](args)
	if err != nil {
		return nil, err
	}
	return s.conn.CreateProject(ctx, &storage.Project{
		Name:        input.Name,
		Description: input.Description,
		Color:       input.Color,
	})
}

func (s *TasksSkill) updateProject(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.ProjectUpdateInput](args)
	if err != nil {
		return nil, err
	}
	project, err := s.conn.GetProject(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("tasks: project %q not found: %w", input.ID, err)
	}
	if input.Name != "" {
		project.Name = input.Name
	}
	if input.Description != "" {
		project.Description = input.Description
	}
	if input.Color != "" {
		project.Color = input.Color
	}
	return s.conn.UpdateProject(ctx, project)
}

func (s *TasksSkill) deleteProject(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.ProjectDeleteInput](args)
	if err != nil {
		return nil, err
	}
	if err := s.conn.DeleteProject(ctx, input.ID); err != nil {
		return nil, err
	}
	return map[string]any{"status": "deleted", "id": input.ID}, nil
}

func (s *TasksSkill) getProject(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.ProjectGetInput](args)
	if err != nil {
		return nil, err
	}
	return s.conn.GetProject(ctx, input.ID)
}

// ── Tasks ─────────────────────────────────────────────────────────────────────

// pkg/skills/tasks/tasks.go
func (s *TasksSkill) createTasks(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.TaskCreateInput](args)
	if err != nil {
		return nil, err
	}
	if len(input.Tasks) == 0 {
		return nil, fmt.Errorf("tasks: at least one task is required")
	}

	tasks := make([]*storage.Task, 0, len(input.Tasks))
	for _, item := range input.Tasks {
		task := &storage.Task{
			Title:     item.Title,
			Body:      item.Body,
			ProjectID: item.ProjectID,
			Priority:  item.Priority,
			Tags:      item.Tags,
		}
		if item.DueAt != "" {
			t, err := time.Parse(time.RFC3339, item.DueAt)
			if err != nil {
				return nil, fmt.Errorf("tasks: invalid due_at for %q: %w", item.Title, err)
			}
			task.DueAt = storage.NullDBTime{Time: t, Valid: true}
		}
		tasks = append(tasks, task)
	}

	created, errs := s.conn.CreateTasks(ctx, tasks)

	// Build response that reports both successes and failures.
	response := map[string]any{
		"created": created,
		"count":   len(created),
	}
	if len(errs) > 0 {
		errMsgs := make([]string, len(errs))
		for i, e := range errs {
			errMsgs[i] = e.Error()
		}
		response["errors"] = errMsgs
		response["failed_count"] = len(errs)
	}
	return response, nil
}

func (s *TasksSkill) updateTask(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.TaskUpdateInput](args)
	if err != nil {
		return nil, err
	}
	task, err := s.conn.GetTask(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("tasks: task %q not found: %w", input.ID, err)
	}
	if input.Title != "" {
		task.Title = input.Title
	}
	if input.Body != "" {
		task.Body = input.Body
	}
	if input.Status != "" {
		task.Status = input.Status
	}
	if input.Priority != "" {
		task.Priority = input.Priority
	}
	if input.ProjectID != "" {
		task.ProjectID = input.ProjectID
	}
	if input.Tags != nil {
		task.Tags = input.Tags
	}
	if input.DueAt != "" {
		t, err := time.Parse(time.RFC3339, input.DueAt)
		if err != nil {
			return nil, fmt.Errorf("tasks: invalid due_at: %w", err)
		}
		task.DueAt = storage.NullDBTime{Time: t, Valid: true}
	}
	return s.conn.UpdateTask(ctx, task)
}

func (s *TasksSkill) getTask(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.TaskGetInput](args)
	if err != nil {
		return nil, err
	}
	return s.conn.GetTask(ctx, input.ID)
}

func (s *TasksSkill) deleteTask(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.TaskDeleteInput](args)
	if err != nil {
		return nil, err
	}
	if err := s.conn.DeleteTask(ctx, input.ID); err != nil {
		return nil, err
	}
	return map[string]any{"status": "deleted", "id": input.ID}, nil
}

func (s *TasksSkill) completeTask(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.TaskCompleteInput](args)
	if err != nil {
		return nil, err
	}
	if err := s.conn.CompleteTask(ctx, input.ID); err != nil {
		return nil, err
	}
	return map[string]any{"status": "done", "id": input.ID}, nil
}

func (s *TasksSkill) listTasks(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.TaskListInput](args)
	if err != nil {
		return nil, err
	}
	filter := storage.TaskFilter{}
	if input.ProjectID != "" {
		filter.ProjectID = &input.ProjectID
	}
	if input.Status != "" {
		filter.Status = &input.Status
	}
	if input.Priority != "" {
		filter.Priority = &input.Priority
	}
	if input.DueFrom != "" {
		t, err := time.Parse(time.RFC3339, input.DueFrom)
		if err != nil {
			return nil, fmt.Errorf("tasks: invalid due_from: %w", err)
		}
		filter.DueFrom = &t
	}
	if input.DueTo != "" {
		t, err := time.Parse(time.RFC3339, input.DueTo)
		if err != nil {
			return nil, fmt.Errorf("tasks: invalid due_to: %w", err)
		}
		filter.DueTo = &t
	}
	return s.conn.ListTasks(ctx, filter)
}

func (s *TasksSkill) searchTasks(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.TaskSearchInput](args)
	if err != nil {
		return nil, err
	}
	return s.conn.SearchTasks(ctx, input.Query)
}

func (s *TasksSkill) moveTask(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.TaskMoveInput](args)
	if err != nil {
		return nil, err
	}
	if err := s.conn.MoveToProject(ctx, input.TaskID, input.ProjectID); err != nil {
		return nil, err
	}
	return map[string]any{"status": "moved", "task_id": input.TaskID, "project_id": input.ProjectID}, nil
}

func (s *TasksSkill) setPriority(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.TaskSetPriorityInput](args)
	if err != nil {
		return nil, err
	}
	if err := s.conn.SetPriority(ctx, input.ID, input.Priority); err != nil {
		return nil, err
	}
	return map[string]any{"status": "ok", "id": input.ID, "priority": input.Priority}, nil
}
