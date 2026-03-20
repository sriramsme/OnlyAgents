package reminders

import (
	"context"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

type RemindersSkill struct {
	ctx    context.Context
	cancel context.CancelFunc
	*skills.BaseSkill
	conn connectors.RemindersConnector
}

// external path — defaults baked in
func New(ctx context.Context, conn connectors.RemindersConnector) (*RemindersSkill, error) {
	if conn == nil {
		return nil, fmt.Errorf("reminders: connector required")
	}
	skillCtx, cancel := context.WithCancel(ctx)
	return &RemindersSkill{
		BaseSkill: skills.NewBaseSkill(skills.BaseSkillInfo{
			Name:        "reminders",
			Description: "Create, list, and manage reminders",
			Version:     "1.0.0",
			Tools:       tools.GetRemindersTools(),
			Groups:      tools.GetRemindersGroups(),
		}, skills.SkillTypeNative),
		conn:   conn,
		ctx:    skillCtx,
		cancel: cancel,
	}, nil
}

// internal path — config drives everything, never touches New()
func init() {
	skills.Register("reminders", func(ctx context.Context, cfg skills.Config,
		conn connectors.Connector,
	) (skills.Skill, error) {
		remindersConn, ok := conn.(connectors.RemindersConnector)
		if !ok {
			return nil, fmt.Errorf("reminders: connector is not a RemindersConnector")
		}
		skillCtx, cancel := context.WithCancel(ctx)
		return &RemindersSkill{
			BaseSkill: skills.NewBaseSkillFromConfig(
				cfg,
				skills.SkillTypeNative,
				tools.GetRemindersTools(),
				tools.GetRemindersGroups(),
			),
			conn:   remindersConn,
			ctx:    skillCtx,
			cancel: cancel,
		}, nil
	})
}

func (s *RemindersSkill) Initialize() error {
	return nil
}

func (s *RemindersSkill) Shutdown() error {
	s.cancel()
	return nil
}

func (s *RemindersSkill) Execute(ctx context.Context, toolName string, args []byte) tools.ToolExecution {
	if s.conn == nil {
		return tools.ExecErr(fmt.Errorf("reminders skill: connector not initialized"))
	}

	var result any
	var err error

	switch toolName {
	case "reminders_create":
		result, err = s.createReminders(ctx, args)
	case "reminder_get":
		result, err = s.getReminder(ctx, args)
	case "reminder_update":
		result, err = s.updateReminder(ctx, args)
	case "reminder_delete":
		result, err = s.deleteReminder(ctx, args)
	case "reminder_list":
		result, err = s.conn.ListReminders(ctx)
	default:
		return tools.ExecErr(fmt.Errorf("reminders skill: unknown tool %q", toolName))
	}

	if err != nil {
		return tools.ExecErr(err)
	}
	return tools.ExecOK(result)
}

func (s *RemindersSkill) createReminders(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.ReminderCreateInput](args)
	if err != nil {
		return nil, err
	}

	if len(input.Reminders) == 0 {
		return nil, fmt.Errorf("reminders: at least one reminder is required")
	}

	reminders := make([]*storage.Reminder, 0, len(input.Reminders))

	for _, item := range input.Reminders {
		dueAt, err := time.Parse(time.RFC3339, item.DueAt)
		if err != nil {
			return nil, fmt.Errorf("reminders: invalid due_at for %q: %w", item.Title, err)
		}

		reminder := &storage.Reminder{
			Title:     item.Title,
			Body:      item.Body,
			DueAt:     storage.DBTime{Time: dueAt},
			Recurring: item.Recurring,
		}

		reminders = append(reminders, reminder)
	}

	created, errs := s.conn.CreateReminders(ctx, reminders)

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

func (s *RemindersSkill) getReminder(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.ReminderGetInput](args)
	if err != nil {
		return nil, err
	}
	return s.conn.GetReminder(ctx, input.ID)
}

func (s *RemindersSkill) updateReminder(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.ReminderUpdateInput](args)
	if err != nil {
		return nil, err
	}
	rem, err := s.conn.GetReminder(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("reminders: %q not found: %w", input.ID, err)
	}
	if input.Title != "" {
		rem.Title = input.Title
	}
	if input.Body != "" {
		rem.Body = input.Body
	}
	if input.DueAt != "" {
		t, err := time.Parse(time.RFC3339, input.DueAt)
		if err != nil {
			return nil, fmt.Errorf("reminders: invalid due_at: %w", err)
		}
		rem.DueAt = storage.DBTime{Time: t}
	}
	return s.conn.UpdateReminder(ctx, rem)
}

func (s *RemindersSkill) deleteReminder(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.ReminderDeleteInput](args)
	if err != nil {
		return nil, err
	}
	if err := s.conn.DeleteReminder(ctx, input.ID); err != nil {
		return nil, err
	}
	return map[string]any{"status": "deleted", "id": input.ID}, nil
}
