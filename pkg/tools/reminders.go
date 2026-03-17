package tools

type ReminderCreateInput struct {
	Reminders []ReminderCreateItem `json:"reminders" desc:"One or more reminders to create" cli_short:"r" cli_req:"true" cli_help:"JSON array or repeated input"`
}

type ReminderCreateItem struct {
	Title     string `json:"title"               desc:"Reminder title"                          cli_short:"t" cli_req:"true"`
	Body      string `json:"body,omitempty"      desc:"Optional longer description"             cli_short:"b"`
	DueAt     string `json:"due_at"              desc:"When to fire the reminder (RFC3339)"     cli_short:"d" cli_req:"true" cli_help:"e.g. 2026-03-20T15:04:05Z"`
	Recurring string `json:"recurring,omitempty" desc:"RRULE for recurring reminders"           cli_short:"r" cli_help:"e.g. FREQ=DAILY;INTERVAL=1"`
}

type ReminderGetInput struct {
	ID string `json:"id" desc:"Reminder ID" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type ReminderUpdateInput struct {
	ID    string `json:"id"                desc:"Reminder ID to update"                         cli_short:"i" cli_pos:"1" cli_req:"true"`
	Title string `json:"title,omitempty"   desc:"New title"                                     cli_short:"t"`
	Body  string `json:"body,omitempty"    desc:"New body"                                      cli_short:"b"`
	DueAt string `json:"due_at,omitempty"  desc:"New due time in RFC3339 format"                cli_short:"d" cli_help:"e.g. 2026-03-20T15:04:05Z"`
}

type ReminderDeleteInput struct {
	ID string `json:"id" desc:"Reminder ID to delete" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

func GetRemindersEntries() []ToolEntry {
	return []ToolEntry{
		{
			NewToolDef(
				"reminders",
				"reminders_create",
				"Create one or more reminders that fires at a specific time via the user's active channel. Always use this tool even for single-reminder creation.",
				SchemaFromStruct(ReminderCreateInput{}),
			),
			ReminderCreateInput{},
		},
		{
			NewToolDef(
				"reminders",
				"reminder_get",
				"Get details of a specific reminder by ID",
				SchemaFromStruct(ReminderGetInput{}),
			),
			ReminderGetInput{},
		},
		{
			NewToolDef(
				"reminders",
				"reminder_update",
				"Update the title, body, or due time of a reminder",
				SchemaFromStruct(ReminderUpdateInput{}),
			),
			ReminderUpdateInput{},
		},
		{
			NewToolDef(
				"reminders",
				"reminder_delete",
				"Delete a reminder by ID",
				SchemaFromStruct(ReminderDeleteInput{}),
			),
			ReminderDeleteInput{},
		},
		{
			NewToolDef(
				"reminders",
				"reminder_list",
				"List all pending (unsent) reminders",
				SchemaFromStruct(struct{}{}),
			),
			struct{}{},
		},
	}
}

func GetRemindersTools() []ToolDef {
	entries := GetRemindersEntries()
	defs := make([]ToolDef, len(entries))
	for i, e := range entries {
		defs[i] = e.Def
	}
	return defs
}
