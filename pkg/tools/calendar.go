package tools

import "time"

type CalendarCreateEventInput struct {
	Events []CalendarCreateEventItem `json:"events" desc:"One or more events to create"`
}

type CalendarCreateEventItem struct {
	Title       string   `json:"title"                  desc:"Event title"`
	Description string   `json:"description,omitempty"  desc:"Event description"`
	StartTime   string   `json:"start_time"             desc:"Start time in RFC3339 format"`
	EndTime     string   `json:"end_time"               desc:"End time in RFC3339 format"`
	AllDay      bool     `json:"all_day,omitempty"      desc:"Whether this is an all-day event"`
	Location    string   `json:"location,omitempty"     desc:"Physical or virtual location"`
	Recurrence  string   `json:"recurrence,omitempty"   desc:"RRULE string for recurring events"`
	Tags        []string `json:"tags,omitempty"         desc:"Tags to categorize the event"`
}

type CalendarUpdateEventInput struct {
	ID          string `json:"id"                     desc:"Event ID to update"`
	Title       string `json:"title,omitempty"        desc:"New event title"`
	Description string `json:"description,omitempty"  desc:"New description"`
	StartTime   string `json:"start_time,omitempty"   desc:"New start time in RFC3339 format"`
	EndTime     string `json:"end_time,omitempty"     desc:"New end time in RFC3339 format"`
	Location    string `json:"location,omitempty"     desc:"New location"`
}

type CalendarGetEventInput struct {
	ID string `json:"id" desc:"Event ID"`
}

type CalendarDeleteEventInput struct {
	ID string `json:"id" desc:"Event ID to delete"`
}

type CalendarListEventsInput struct {
	From string `json:"from" desc:"Start of range in RFC3339 format"`
	To   string `json:"to"   desc:"End of range in RFC3339 format"`
}

type CalendarGetUpcomingInput struct {
	Limit int `json:"limit,omitempty" desc:"Max number of events to return (default: 10)"`
}

type CalendarFindSlotsInput struct {
	From            string `json:"from"             desc:"Search window start in RFC3339 format"`
	To              string `json:"to"               desc:"Search window end in RFC3339 format"`
	MinDurationMins int    `json:"min_duration_mins" desc:"Minimum free slot duration in minutes"`
}

// ParseTime is a convenience used by the skill to parse RFC3339 strings from LLM.
func ParseEventTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func GetCalendarTools() []ToolDef {
	return []ToolDef{
		NewToolDef(
			SkillCalendar,
			"calendar_create_events",
			"Create one ore more calendar events. Always use this tool even for single-event creation.",
			SchemaFromStruct(CalendarCreateEventInput{}),
		),
		NewToolDef(
			SkillCalendar,
			"calendar_update_event",
			"Update an existing calendar event by ID",
			SchemaFromStruct(CalendarUpdateEventInput{}),
		),
		NewToolDef(
			SkillCalendar,
			"calendar_get_event",
			"Get full details of a specific calendar event by ID",
			SchemaFromStruct(CalendarGetEventInput{}),
		),
		NewToolDef(
			SkillCalendar,
			"calendar_delete_event",
			"Delete a calendar event by ID",
			SchemaFromStruct(CalendarDeleteEventInput{}),
		),
		NewToolDef(
			SkillCalendar,
			"calendar_list_events",
			"List calendar events within a date/time range",
			SchemaFromStruct(CalendarListEventsInput{}),
		),
		NewToolDef(
			SkillCalendar,
			"calendar_get_upcoming",
			"Get the next N upcoming calendar events from now",
			SchemaFromStruct(CalendarGetUpcomingInput{}),
		),
		NewToolDef(
			SkillCalendar,
			"calendar_find_slots",
			"Find available free time slots in the calendar within a given window",
			SchemaFromStruct(CalendarFindSlotsInput{}),
		),
	}
}
