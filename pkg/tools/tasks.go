package tools

type ProjectCreateInput struct {
	Name        string `json:"name"                  desc:"Project name"`
	Description string `json:"description,omitempty" desc:"Project description"`
	Color       string `json:"color,omitempty"       desc:"Hex color for the project e.g. #6366f1"`
}

type ProjectUpdateInput struct {
	ID          string `json:"id"                    desc:"Project ID"`
	Name        string `json:"name,omitempty"        desc:"New name"`
	Description string `json:"description,omitempty" desc:"New description"`
	Color       string `json:"color,omitempty"       desc:"New hex color"`
}

type ProjectDeleteInput struct {
	ID string `json:"id" desc:"Project ID to delete. Tasks in this project become unassigned."`
}

type ProjectGetInput struct {
	ID string `json:"id" desc:"Project ID"`
}

type TaskCreateInput struct {
	Tasks []TaskCreateItem `json:"tasks" desc:"One or more tasks to create"`
}

type TaskCreateItem struct {
	Title     string   `json:"title"                desc:"Task title"`
	Body      string   `json:"body,omitempty"       desc:"Task description or notes"`
	ProjectID string   `json:"project_id,omitempty" desc:"Project ID to assign this task to"`
	Priority  string   `json:"priority,omitempty"   desc:"Task priority" enum:"low,medium,high"`
	DueAt     string   `json:"due_at,omitempty"     desc:"Due date/time in RFC3339 format"`
	Tags      []string `json:"tags,omitempty"       desc:"Tags to categorize the task"`
}

type TaskUpdateInput struct {
	ID        string   `json:"id"                   desc:"Task ID to update"`
	Title     string   `json:"title,omitempty"      desc:"New title"`
	Body      string   `json:"body,omitempty"       desc:"New description"`
	ProjectID string   `json:"project_id,omitempty" desc:"New project ID (empty string to remove from project)"`
	Priority  string   `json:"priority,omitempty"   desc:"New priority" enum:"low,medium,high"`
	Status    string   `json:"status,omitempty"     desc:"New status" enum:"todo,in_progress,done,cancelled"`
	DueAt     string   `json:"due_at,omitempty"     desc:"New due date in RFC3339 format"`
	Tags      []string `json:"tags,omitempty"       desc:"New tags (replaces existing)"`
}

type TaskGetInput struct {
	ID string `json:"id" desc:"Task ID"`
}

type TaskDeleteInput struct {
	ID string `json:"id" desc:"Task ID to delete"`
}

type TaskCompleteInput struct {
	ID string `json:"id" desc:"Task ID to mark as done"`
}

type TaskListInput struct {
	ProjectID string `json:"project_id,omitempty" desc:"Filter by project ID"`
	Status    string `json:"status,omitempty"     desc:"Filter by status" enum:"todo,in_progress,done,cancelled"`
	Priority  string `json:"priority,omitempty"   desc:"Filter by priority" enum:"low,medium,high"`
	DueFrom   string `json:"due_from,omitempty"   desc:"Filter tasks due on or after this date RFC3339"`
	DueTo     string `json:"due_to,omitempty"     desc:"Filter tasks due on or before this date RFC3339"`
}

type TaskSearchInput struct {
	Query string `json:"query" desc:"Full-text search across task titles and descriptions"`
}

type TaskMoveInput struct {
	TaskID    string `json:"task_id"    desc:"Task ID to move"`
	ProjectID string `json:"project_id" desc:"Project ID to move to, or empty string to remove from project"`
}

type TaskSetPriorityInput struct {
	ID       string `json:"id"       desc:"Task ID"`
	Priority string `json:"priority" desc:"New priority" enum:"low,medium,high"`
}

func GetTasksTools() []ToolDef {
	return []ToolDef{
		// Projects
		NewToolDef(
			SkillTasks,
			"project_create",
			"Create a new project to group related tasks",
			SchemaFromStruct(ProjectCreateInput{}),
		),
		NewToolDef(
			SkillTasks,
			"project_update",
			"Update a project's name, description, or color",
			SchemaFromStruct(ProjectUpdateInput{}),
		),
		NewToolDef(
			SkillTasks,
			"project_delete",
			"Delete a project. Tasks in the project become unassigned.",
			SchemaFromStruct(ProjectDeleteInput{}),
		),
		NewToolDef(
			SkillTasks,
			"project_get",
			"Get details of a specific project",
			SchemaFromStruct(ProjectGetInput{}),
		),
		NewToolDef(
			SkillTasks,
			"project_list",
			"List all projects",
			SchemaFromStruct(struct{}{}),
		),
		// Tasks
		NewToolDef(
			SkillTasks,
			"tasks_create",
			"Create one or more tasks with optional project, priority, and due date. Always use this tool even for single-task creation.",
			SchemaFromStruct(TaskCreateInput{}),
		),
		NewToolDef(
			SkillTasks,
			"task_update",
			"Update a task's title, body, project, priority, status, due date, or tags",
			SchemaFromStruct(TaskUpdateInput{}),
		),
		NewToolDef(
			SkillTasks,
			"task_get",
			"Get full details of a specific task by ID",
			SchemaFromStruct(TaskGetInput{}),
		),
		NewToolDef(
			SkillTasks,
			"task_delete",
			"Delete a task by ID",
			SchemaFromStruct(TaskDeleteInput{}),
		),
		NewToolDef(
			SkillTasks,
			"task_complete",
			"Mark a task as done and record its completion time",
			SchemaFromStruct(TaskCompleteInput{}),
		),
		NewToolDef(
			SkillTasks,
			"task_list",
			"List tasks with optional filters by project, status, priority, or due date range",
			SchemaFromStruct(TaskListInput{}),
		),
		NewToolDef(
			SkillTasks,
			"task_search",
			"Search tasks by title and description using full-text search",
			SchemaFromStruct(TaskSearchInput{}),
		),
		NewToolDef(
			SkillTasks,
			"task_today",
			"Get all non-completed tasks due today",
			SchemaFromStruct(struct{}{}),
		),
		NewToolDef(
			SkillTasks,
			"task_move",
			"Move a task to a different project or remove it from its current project",
			SchemaFromStruct(TaskMoveInput{}),
		),
		NewToolDef(
			SkillTasks,
			"task_set_priority",
			"Change the priority of a task",
			SchemaFromStruct(TaskSetPriorityInput{}),
		),
	}
}
