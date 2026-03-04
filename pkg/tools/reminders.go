package tools

type ReminderCreateInput struct {
	Reminders []ReminderCreateItem `json:"reminders" desc:"One or more reminders to create"`
}

type ReminderCreateItem struct {
	Title     string `json:"title"               desc:"Reminder title"`
	Body      string `json:"body,omitempty"       desc:"Optional longer description"`
	DueAt     string `json:"due_at"               desc:"When to fire the reminder, RFC3339 format"`
	Recurring string `json:"recurring,omitempty"  desc:"RRULE for recurring reminders"`
}

type ReminderGetInput struct {
	ID string `json:"id" desc:"Reminder ID"`
}

type ReminderUpdateInput struct {
	ID    string `json:"id"              desc:"Reminder ID to update"`
	Title string `json:"title,omitempty"  desc:"New title"`
	Body  string `json:"body,omitempty"   desc:"New body"`
	DueAt string `json:"due_at,omitempty" desc:"New due time in RFC3339 format"`
}

type ReminderDeleteInput struct {
	ID string `json:"id" desc:"Reminder ID to delete"`
}

func GetRemindersTools() []ToolDef {
	return []ToolDef{
		NewToolDef(
			SkillReminders,
			"reminders_create",
			"Create one or more reminders that fires at a specific time via the user's active channel. Always use this tool even for single-reminder creation.",
			SchemaFromStruct(ReminderCreateInput{}),
		),
		NewToolDef(
			SkillReminders,
			"reminder_get",
			"Get details of a specific reminder by ID",
			SchemaFromStruct(ReminderGetInput{}),
		),
		NewToolDef(
			SkillReminders,
			"reminder_update",
			"Update the title, body, or due time of a reminder",
			SchemaFromStruct(ReminderUpdateInput{}),
		),
		NewToolDef(
			SkillReminders,
			"reminder_delete",
			"Delete a reminder by ID",
			SchemaFromStruct(ReminderDeleteInput{}),
		),
		NewToolDef(
			SkillReminders,
			"reminder_list",
			"List all pending (unsent) reminders",
			SchemaFromStruct(struct{}{}),
		),
	}
}
