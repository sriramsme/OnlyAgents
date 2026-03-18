package tools

import "time"

type CalendarCreateEventInput struct {
	Events []CalendarCreateEventItem `json:"events" desc:"One or more events to create" cli_short:"e" cli_req:"true" cli_help:"JSON array or repeated input"`
}

type CalendarCreateEventItem struct {
	Title       string   `json:"title"                 desc:"Event title"                                      cli_short:"t" cli_req:"true"`
	Description string   `json:"description,omitempty" desc:"Event description"                                cli_short:"d"`
	StartTime   string   `json:"start_time"            desc:"Start time in RFC3339 format"                     cli_short:"s" cli_req:"true" cli_help:"e.g. 2026-03-20T15:04:05Z"`
	EndTime     string   `json:"end_time"              desc:"End time in RFC3339 format"                       cli_short:"e" cli_req:"true" cli_help:"e.g. 2026-03-20T16:04:05Z"`
	AllDay      bool     `json:"all_day,omitempty"     desc:"Whether this is an all-day event"                 cli_short:"a"`
	Location    string   `json:"location,omitempty"    desc:"Physical or virtual location"                     cli_short:"l"`
	Recurrence  string   `json:"recurrence,omitempty"  desc:"RRULE string for recurring events"                cli_short:"r" cli_help:"e.g. FREQ=WEEKLY;BYDAY=MO"`
	Tags        []string `json:"tags,omitempty"        desc:"Tags to categorize the event"                     cli_short:"g" cli_help:"comma-separated"`
}

type CalendarUpdateEventInput struct {
	ID          string `json:"id"                    desc:"Event ID to update"                                 cli_short:"i" cli_pos:"1" cli_req:"true"`
	Title       string `json:"title,omitempty"       desc:"New event title"                                   cli_short:"t"`
	Description string `json:"description,omitempty" desc:"New description"                                   cli_short:"d"`
	StartTime   string `json:"start_time,omitempty"  desc:"New start time in RFC3339 format"                  cli_short:"s" cli_help:"e.g. 2026-03-20T15:04:05Z"`
	EndTime     string `json:"end_time,omitempty"    desc:"New end time in RFC3339 format"                    cli_short:"e" cli_help:"e.g. 2026-03-20T16:04:05Z"`
	Location    string `json:"location,omitempty"    desc:"New location"                                      cli_short:"l"`
}

type CalendarGetEventInput struct {
	ID string `json:"id" desc:"Event ID" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type CalendarDeleteEventInput struct {
	ID string `json:"id" desc:"Event ID to delete" cli_short:"i" cli_pos:"1" cli_req:"true"`
}

type CalendarListEventsInput struct {
	From string `json:"from" desc:"Start of range in RFC3339 format" cli_short:"f" cli_req:"true" cli_help:"e.g. 2026-03-20T00:00:00Z"`
	To   string `json:"to"   desc:"End of range in RFC3339 format"   cli_short:"t" cli_req:"true" cli_help:"e.g. 2026-03-21T00:00:00Z"`
}

type CalendarGetUpcomingInput struct {
	Limit int `json:"limit,omitempty" desc:"Max number of events to return (default: 10)" cli_short:"n" cli_help:"e.g. 5, 10, 20"`
}

type CalendarFindSlotsInput struct {
	From            string `json:"from"              desc:"Search window start in RFC3339 format"            cli_short:"f" cli_req:"true" cli_help:"e.g. 2026-03-20T00:00:00Z"`
	To              string `json:"to"                desc:"Search window end in RFC3339 format"              cli_short:"t" cli_req:"true" cli_help:"e.g. 2026-03-21T00:00:00Z"`
	MinDurationMins int    `json:"min_duration_mins" desc:"Minimum free slot duration in minutes"            cli_short:"d" cli_req:"true" cli_help:"e.g. 30, 60"`
}

// ParseTime is a convenience used by the skill to parse RFC3339 strings from LLM.
func ParseEventTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

const (
	CalendarRead   ToolGroup = "calendar_read"
	CalendarWrite  ToolGroup = "calendar_write"
	CalendarManage ToolGroup = "calendar_manage"
)

func GetCalendarGroups() map[ToolGroup]string {
	return map[ToolGroup]string{
		CalendarRead:   "View and query calendar events, including availability and schedules",
		CalendarWrite:  "Create new calendar events",
		CalendarManage: "Modify or delete existing calendar events",
	}
}

func GetCalendarEntries() []ToolEntry {
	return []ToolEntry{
		{
			NewToolDef(
				"calendar",
				"calendar_create_events",
				"Create one or more calendar events. Always use this tool even for single-event creation.",
				SchemaFromStruct(CalendarCreateEventInput{}),
				CalendarWrite,
			),
			CalendarCreateEventInput{},
		},
		{
			NewToolDef(
				"calendar",
				"calendar_update_event",
				"Update an existing calendar event by ID",
				SchemaFromStruct(CalendarUpdateEventInput{}),
				CalendarManage,
			),
			CalendarUpdateEventInput{},
		},
		{
			NewToolDef(
				"calendar",
				"calendar_get_event",
				"Get full details of a specific calendar event by ID",
				SchemaFromStruct(CalendarGetEventInput{}),
				CalendarRead,
			),
			CalendarGetEventInput{},
		},
		{
			NewToolDef(
				"calendar",
				"calendar_delete_event",
				"Delete a calendar event by ID",
				SchemaFromStruct(CalendarDeleteEventInput{}),
				CalendarManage,
			),
			CalendarDeleteEventInput{},
		},
		{
			NewToolDef(
				"calendar",
				"calendar_list_events",
				"List calendar events within a date/time range",
				SchemaFromStruct(CalendarListEventsInput{}),
				CalendarRead,
			),
			CalendarListEventsInput{},
		},
		{
			NewToolDef(
				"calendar",
				"calendar_get_upcoming",
				"Get the next N upcoming calendar events from now",
				SchemaFromStruct(CalendarGetUpcomingInput{}),
				CalendarRead,
			),
			CalendarGetUpcomingInput{},
		},
		{
			NewToolDef(
				"calendar",
				"calendar_find_slots",
				"Find available free time slots in the calendar within a given window",
				SchemaFromStruct(CalendarFindSlotsInput{}),
				CalendarRead,
			),
			CalendarFindSlotsInput{},
		},
	}
}

func GetCalendarTools() []ToolDef {
	entries := GetCalendarEntries()
	defs := make([]ToolDef, len(entries))
	for i, e := range entries {
		defs[i] = e.Def
	}
	return defs
}
