package calendar

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

type CalendarSkill struct {
	ctx    context.Context
	cancel context.CancelFunc
	*skills.BaseSkill
	conn connectors.CalendarConnector
}

// external path — defaults baked in
func New(ctx context.Context, conn connectors.CalendarConnector) (*CalendarSkill, error) {
	if conn == nil {
		return nil, fmt.Errorf("calendar: connector required")
	}

	skillCtx, cancel := context.WithCancel(ctx)

	return &CalendarSkill{
		BaseSkill: skills.NewBaseSkill(skills.BaseSkillInfo{
			Name:        "calendar",
			Description: "Create, view, and manage calendar events",
			Version:     "1.0.0",
			Enabled:     true,
		}, skills.SkillTypeNative),
		conn:   conn,
		ctx:    skillCtx,
		cancel: cancel,
	}, nil
}

// internal path — config drives everything, never touches New()
func init() {
	skills.Register("calendar", func(
		ctx context.Context,
		cfg config.Skill,
		conn connectors.Connector,
		security config.SecurityConfig,
	) (skills.Skill, error) {
		calendarConn, ok := conn.(connectors.CalendarConnector)
		if !ok {
			fmt.Printf("calendar: connector is not a CalendarConnector %T\n", conn)
			return nil, fmt.Errorf("calendar: connector is not a CalendarConnector")
		}

		skillCtx, cancel := context.WithCancel(ctx)

		return &CalendarSkill{
			BaseSkill: skills.NewBaseSkillFromConfig(cfg, skills.SkillTypeNative),
			conn:      calendarConn,
			ctx:       skillCtx,
			cancel:    cancel,
		}, nil
	})
}

func (s *CalendarSkill) Initialize() error {
	return nil
}

func (s *CalendarSkill) Shutdown() error {
	s.cancel()
	return nil
}

func (s *CalendarSkill) Tools() []tools.ToolDef {
	return tools.GetCalendarTools()
}

func (s *CalendarSkill) Execute(ctx context.Context, toolName string, args []byte) (any, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("calendar skill: connector not initialized")
	}
	switch toolName {
	case "calendar_create_events":
		return s.createEvents(ctx, args)
	case "calendar_update_event":
		return s.updateEvent(ctx, args)
	case "calendar_get_event":
		return s.getEvent(ctx, args)
	case "calendar_delete_event":
		return s.deleteEvent(ctx, args)
	case "calendar_list_events":
		return s.listEvents(ctx, args)
	case "calendar_get_upcoming":
		return s.getUpcoming(ctx, args)
	case "calendar_find_slots":
		return s.findSlots(ctx, args)
	default:
		return nil, fmt.Errorf("calendar skill: unknown tool %q", toolName)
	}
}

func (s *CalendarSkill) createEvents(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.CalendarCreateEventInput](args)
	if err != nil {
		return nil, err
	}

	if len(input.Events) == 0 {
		return nil, fmt.Errorf("calendar: at least one event is required")
	}

	events := make([]*storage.CalendarEvent, 0, len(input.Events))

	for _, item := range input.Events {
		start, err := tools.ParseEventTime(item.StartTime)
		if err != nil {
			return nil, fmt.Errorf("calendar: invalid start_time for %q: %w", item.Title, err)
		}

		end, err := tools.ParseEventTime(item.EndTime)
		if err != nil {
			return nil, fmt.Errorf("calendar: invalid end_time for %q: %w", item.Title, err)
		}

		event := &storage.CalendarEvent{
			Title:       item.Title,
			Description: item.Description,
			StartTime:   storage.DBTime{Time: start},
			EndTime:     storage.DBTime{Time: end},
			AllDay:      item.AllDay,
			Location:    item.Location,
			Recurrence:  item.Recurrence,
			Tags:        item.Tags,
		}

		events = append(events, event)
	}

	created, errs := s.conn.CreateEvents(ctx, events)

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

func (s *CalendarSkill) updateEvent(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.CalendarUpdateEventInput](args)
	if err != nil {
		return nil, err
	}
	event, err := s.conn.GetEvent(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("calendar: event %q not found: %w", input.ID, err)
	}
	if input.Title != "" {
		event.Title = input.Title
	}
	if input.Description != "" {
		event.Description = input.Description
	}
	if input.Location != "" {
		event.Location = input.Location
	}
	if input.StartTime != "" {
		t, err := tools.ParseEventTime(input.StartTime)
		if err != nil {
			return nil, fmt.Errorf("calendar: invalid start_time: %w", err)
		}
		event.StartTime = storage.DBTime{Time: t}
	}
	if input.EndTime != "" {
		t, err := tools.ParseEventTime(input.EndTime)
		if err != nil {
			return nil, fmt.Errorf("calendar: invalid end_time: %w", err)
		}
		event.EndTime = storage.DBTime{Time: t}
	}
	return s.conn.UpdateEvent(ctx, event)
}

func (s *CalendarSkill) getEvent(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.CalendarGetEventInput](args)
	if err != nil {
		return nil, err
	}
	return s.conn.GetEvent(ctx, input.ID)
}

func (s *CalendarSkill) deleteEvent(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.CalendarDeleteEventInput](args)
	if err != nil {
		return nil, err
	}
	if err := s.conn.DeleteEvent(ctx, input.ID); err != nil {
		return nil, err
	}
	return map[string]any{"status": "deleted", "id": input.ID}, nil
}

func (s *CalendarSkill) listEvents(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.CalendarListEventsInput](args)
	if err != nil {
		return nil, err
	}
	from, err := tools.ParseEventTime(input.From)
	if err != nil {
		return nil, fmt.Errorf("calendar: invalid from: %w", err)
	}
	to, err := tools.ParseEventTime(input.To)
	if err != nil {
		return nil, fmt.Errorf("calendar: invalid to: %w", err)
	}
	return s.conn.ListEvents(ctx, from, to)
}

func (s *CalendarSkill) getUpcoming(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.CalendarGetUpcomingInput](args)
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	return s.conn.GetUpcoming(ctx, limit)
}

func (s *CalendarSkill) findSlots(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.CalendarFindSlotsInput](args)
	if err != nil {
		return nil, err
	}
	from, err := tools.ParseEventTime(input.From)
	if err != nil {
		return nil, fmt.Errorf("calendar: invalid from: %w", err)
	}
	to, err := tools.ParseEventTime(input.To)
	if err != nil {
		return nil, fmt.Errorf("calendar: invalid to: %w", err)
	}
	mins := input.MinDurationMins
	if mins <= 0 {
		mins = 30
	}
	return s.conn.FindAvailableSlots(ctx, from, to, time.Duration(mins)*time.Minute)
}
