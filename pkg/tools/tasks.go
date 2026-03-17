package tools

type ProjectCreateInput struct {
	Name        string `json:"name"                  desc:"Project name"                                cli_short:"n" cli_req:"true"`
	Description string `json:"description,omitempty" desc:"Project description"                         cli_short:"d"`
	Color       string `json:"color,omitempty"       desc:"Hex color for the project e.g. #6366f1"     cli_short:"c" cli_help:"e.g. #6366f1"`
}

type ProjectUpdateInput struct {
	ID          string `json:"id"                    desc:"Project ID"                                  cli_short:"i" cli_pos:"1" cli_req:"true"`
	Name        string `json:"name,omitempty"        desc:"New name"                                    cli_short:"n"`
	Description string `json:"description,omitempty" desc:"New description"                             cli_short:"d"`
	Color       string `json:"color,omitempty"       desc:"New hex color"                               cli_short:"c"`
}

type ProjectDeleteInput struct {
	ID string `json:"id" desc:"Project ID to delete. Tasks in this project become unassigned." cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type ProjectGetInput struct {
	ID string `json:"id" desc:"Project ID" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type TaskCreateInput struct {
	Tasks []TaskCreateItem `json:"tasks" desc:"One or more tasks to create" cli_short:"t" cli_req:"true" cli_help:"JSON array or repeated input"`
}

type TaskCreateItem struct {
	Title     string   `json:"title"                desc:"Task title"                                   cli_short:"t" cli_req:"true"`
	Body      string   `json:"body,omitempty"       desc:"Task description or notes"                   cli_short:"b"`
	ProjectID string   `json:"project_id,omitempty" desc:"Project ID to assign this task to"           cli_short:"p"`
	Priority  string   `json:"priority,omitempty"   desc:"Task priority"                               cli_short:"r" cli_help:"low, medium, high" enum:"low,medium,high"`
	DueAt     string   `json:"due_at,omitempty"     desc:"Due date/time in RFC3339 format"             cli_short:"d" cli_help:"e.g. 2026-03-20T15:04:05Z"`
	Tags      []string `json:"tags,omitempty"       desc:"Tags to categorize the task"                 cli_short:"g" cli_help:"comma-separated"`
}

type TaskUpdateInput struct {
	ID        string   `json:"id"                   desc:"Task ID to update"                            cli_short:"i" cli_pos:"1" cli_req:"true"`
	Title     string   `json:"title,omitempty"      desc:"New title"                                    cli_short:"t"`
	Body      string   `json:"body,omitempty"       desc:"New description"                              cli_short:"b"`
	ProjectID string   `json:"project_id,omitempty" desc:"New project ID (empty string to remove from project)" cli_short:"p"`
	Priority  string   `json:"priority,omitempty"   desc:"New priority"                                 cli_short:"r" cli_help:"low, medium, high" enum:"low,medium,high"`
	Status    string   `json:"status,omitempty"     desc:"New status"                                   cli_short:"s" cli_help:"todo, in_progress, done, cancelled" enum:"todo,in_progress,done,cancelled"`
	DueAt     string   `json:"due_at,omitempty"     desc:"New due date in RFC3339 format"               cli_short:"d" cli_help:"e.g. 2026-03-20T15:04:05Z"`
	Tags      []string `json:"tags,omitempty"       desc:"New tags (replaces existing)"                cli_short:"g" cli_help:"comma-separated"`
}

type TaskGetInput struct {
	ID string `json:"id" desc:"Task ID" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type TaskDeleteInput struct {
	ID string `json:"id" desc:"Task ID to delete" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type TaskCompleteInput struct {
	ID string `json:"id" desc:"Task ID to mark as done" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type TaskListInput struct {
	ProjectID string `json:"project_id,omitempty" desc:"Filter by project ID"                          cli_short:"p"`
	Status    string `json:"status,omitempty"     desc:"Filter by status"                              cli_short:"s" cli_help:"todo, in_progress, done, cancelled" enum:"todo,in_progress,done,cancelled"`
	Priority  string `json:"priority,omitempty"   desc:"Filter by priority"                            cli_short:"r" cli_help:"low, medium, high" enum:"low,medium,high"`
	DueFrom   string `json:"due_from,omitempty"   desc:"Filter tasks due on or after this date RFC3339" cli_short:"f" cli_help:"e.g. 2026-03-20T00:00:00Z"`
	DueTo     string `json:"due_to,omitempty"     desc:"Filter tasks due on or before this date RFC3339" cli_short:"t" cli_help:"e.g. 2026-03-21T00:00:00Z"`
}

type TaskSearchInput struct {
	Query string `json:"query" desc:"Full-text search across task titles and descriptions" cli_short:"q" cli_pos:"1" cli_req:"true"`
}

type TaskMoveInput struct {
	TaskID    string `json:"task_id"    desc:"Task ID to move"                               cli_short:"i" cli_pos:"1" cli_req:"true"`
	ProjectID string `json:"project_id" desc:"Project ID to move to, or empty string to remove from project" cli_short:"p" cli_req:"true"`
}

type TaskSetPriorityInput struct {
	ID       string `json:"id"       desc:"Task ID"        cli_short:"i" cli_pos:"1" cli_req:"true"`
	Priority string `json:"priority" desc:"New priority"   cli_short:"r" cli_req:"true" cli_help:"low, medium, high" enum:"low,medium,high"`
}

func GetTasksEntries() []ToolEntry {
	return []ToolEntry{
		// Projects
		{
			NewToolDef(
				"tasks",
				"project_create",
				"Create a new project to group related tasks",
				SchemaFromStruct(ProjectCreateInput{}),
			),
			ProjectCreateInput{},
		},
		{
			NewToolDef(
				"tasks",
				"project_update",
				"Update a project's name, description, or color",
				SchemaFromStruct(ProjectUpdateInput{}),
			),
			ProjectUpdateInput{},
		},
		{
			NewToolDef(
				"tasks",
				"project_delete",
				"Delete a project. Tasks in the project become unassigned.",
				SchemaFromStruct(ProjectDeleteInput{}),
			),
			ProjectDeleteInput{},
		},
		{
			NewToolDef(
				"tasks",
				"project_get",
				"Get details of a specific project",
				SchemaFromStruct(ProjectGetInput{}),
			),
			ProjectGetInput{},
		},
		{
			NewToolDef(
				"tasks",
				"project_list",
				"List all projects",
				SchemaFromStruct(struct{}{}),
			),
			struct{}{},
		},

		// Tasks
		{
			NewToolDef(
				"tasks",
				"tasks_create",
				"Create one or more tasks with optional project, priority, and due date. Always use this tool even for single-task creation.",
				SchemaFromStruct(TaskCreateInput{}),
			),
			TaskCreateInput{},
		},
		{
			NewToolDef(
				"tasks",
				"task_update",
				"Update a task's title, body, project, priority, status, due date, or tags",
				SchemaFromStruct(TaskUpdateInput{}),
			),
			TaskUpdateInput{},
		},
		{
			NewToolDef(
				"tasks",
				"task_get",
				"Get full details of a specific task by ID",
				SchemaFromStruct(TaskGetInput{}),
			),
			TaskGetInput{},
		},
		{
			NewToolDef(
				"tasks",
				"task_delete",
				"Delete a task by ID",
				SchemaFromStruct(TaskDeleteInput{}),
			),
			TaskDeleteInput{},
		},
		{
			NewToolDef(
				"tasks",
				"task_complete",
				"Mark a task as done and record its completion time",
				SchemaFromStruct(TaskCompleteInput{}),
			),
			TaskCompleteInput{},
		},
		{
			NewToolDef(
				"tasks",
				"task_list",
				"List tasks with optional filters by project, status, priority, or due date range",
				SchemaFromStruct(TaskListInput{}),
			),
			TaskListInput{},
		},
		{
			NewToolDef(
				"tasks",
				"task_search",
				"Search tasks by title and description using full-text search",
				SchemaFromStruct(TaskSearchInput{}),
			),
			TaskSearchInput{},
		},
		{
			NewToolDef(
				"tasks",
				"task_today",
				"Get all non-completed tasks due today",
				SchemaFromStruct(struct{}{}),
			),
			struct{}{},
		},
		{
			NewToolDef(
				"tasks",
				"task_move",
				"Move a task to a different project or remove it from its current project",
				SchemaFromStruct(TaskMoveInput{}),
			),
			TaskMoveInput{},
		},
		{
			NewToolDef(
				"tasks",
				"task_set_priority",
				"Change the priority of a task",
				SchemaFromStruct(TaskSetPriorityInput{}),
			),
			TaskSetPriorityInput{},
		},
	}
}

func GetTasksTools() []ToolDef {
	entries := GetTasksEntries()
	defs := make([]ToolDef, len(entries))
	for i, e := range entries {
		defs[i] = e.Def
	}
	return defs
}
