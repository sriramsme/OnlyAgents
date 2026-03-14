package reminders

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
	skills.Register("reminders", NewRemindersSkill)
}

type RemindersSkill struct {
	ctx    context.Context
	cancel context.CancelFunc
	*skills.BaseSkill
	conn connectors.RemindersConnector
}

func NewRemindersSkill(ctx context.Context, cfg config.SkillConfig, conn connectors.Connector,
	security config.SecurityConfig,
) (skills.Skill, error) {
	remindersConn, ok := conn.(connectors.RemindersConnector)
	if !ok {
		return nil, fmt.Errorf("reminders skill: connector is not a RemindersConnector")
	}
	base := skills.NewBaseSkill(cfg, skills.SkillTypeNative)
	skillCtx, cancel := context.WithCancel(ctx)
	return &RemindersSkill{
		BaseSkill: base,
		conn:      remindersConn,
		ctx:       skillCtx,
		cancel:    cancel,
	}, nil
}

func (s *RemindersSkill) Initialize() error {
	return nil
}

func (s *RemindersSkill) Shutdown() error {
	s.cancel()
	return nil
}

func (s *RemindersSkill) Tools() []tools.ToolDef {
	return tools.GetRemindersTools()
}

func (s *RemindersSkill) Execute(ctx context.Context, toolName string, args []byte) (any, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("reminders skill: connector not initialized")
	}
	switch toolName {
	case "reminders_create":
		return s.createReminders(ctx, args)
	case "reminder_get":
		return s.getReminder(ctx, args)
	case "reminder_update":
		return s.updateReminder(ctx, args)
	case "reminder_delete":
		return s.deleteReminder(ctx, args)
	case "reminder_list":
		return s.conn.ListReminders(ctx)
	default:
		return nil, fmt.Errorf("reminders skill: unknown tool %q", toolName)
	}
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
