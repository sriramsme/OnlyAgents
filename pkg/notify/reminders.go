package notify

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/reminder"
)

type reminderNotifierJob struct {
	store reminder.Store
	bus   chan<- core.Event
}

func (j *reminderNotifierJob) Name() string     { return "reminder_notifier" }
func (j *reminderNotifierJob) Schedule() string { return "*/5 * * * *" } // every 5 min

func (j *reminderNotifierJob) Run(ctx context.Context) error {
	now := time.Now()
	// grab anything due within the next 5 min window + already past due but unsent
	reminders, err := j.store.GetDueReminders(ctx, now.Add(5*time.Minute))
	if err != nil {
		return err
	}
	for _, r := range reminders {
		if r.SentAt.Valid {
			continue
		}
		j.bus <- core.Event{
			Type:          core.OutboundMessage,
			CorrelationID: uuid.NewString(),
			Payload: core.OutboundMessagePayload{
				Content:        formatReminderAlert(r),
				IsNotification: true,
			},
		}
		err := j.store.MarkReminderSent(ctx, r.ID, time.Now())
		if err != nil {
			return err
		}
	}
	return nil
}

func formatReminderAlert(r *reminder.Reminder) string {
	var sb strings.Builder

	sb.WriteString("⏰ **")
	sb.WriteString(r.Title)
	sb.WriteString("**")

	if r.Body != "" {
		sb.WriteString("\n")
		sb.WriteString(r.Body)
	}

	return sb.String()
}
