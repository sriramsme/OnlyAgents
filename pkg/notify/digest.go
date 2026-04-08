package notify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/reminder"
	"github.com/sriramsme/OnlyAgents/pkg/productivity/task"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

type dailyDigestJob struct {
	store storage.Storage
	loc   *time.Location
	bus   chan<- core.Event
}

func (j *dailyDigestJob) Name() string     { return "daily_digest" }
func (j *dailyDigestJob) Schedule() string { return "0 20 * * *" }

// Digest has no catch-up — a missed evening digest is just skipped.
func (j *dailyDigestJob) Run(ctx context.Context) error {
	if j.bus == nil {
		logger.Log.Info("memory: digest bus not set, skipping")
		return nil
	}

	tomorrow := time.Now().AddDate(0, 0, 1)

	tasks, err := j.store.GetTasksDueOn(ctx, tomorrow)
	if err != nil {
		return fmt.Errorf("digest: tasks: %w", err)
	}

	tomorrowStart := truncateToDayInLocation(tomorrow, j.loc)
	tomorrowEnd := tomorrowStart.Add(24*time.Hour - time.Second)
	reminders, err := j.store.GetDueReminders(ctx, tomorrowEnd)
	if err != nil {
		return fmt.Errorf("digest: reminders: %w", err)
	}

	var tomorrowReminders []*reminder.Reminder
	for _, r := range reminders {
		if !r.DueAt.Before(tomorrowStart) {
			tomorrowReminders = append(tomorrowReminders, r)
		}
	}

	msg := formatDigest(tomorrow, tasks, tomorrowReminders)
	if msg == "" {
		logger.Log.Info("memory: nothing due tomorrow, skipping digest")
		return nil
	}
	j.bus <- core.Event{
		Type:          core.OutboundMessage,
		CorrelationID: uuid.NewString(),
		Payload: core.OutboundMessagePayload{
			Content:        msg,
			IsNotification: true,
		},
	}
	return nil
}

// formatDigest builds the digest message string.
// Returns "" if there is nothing to report.
func formatDigest(date time.Time, tasks []*task.Task, reminders []*reminder.Reminder) string {
	if len(tasks) == 0 && len(reminders) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📅 *Tomorrow — %s*\n\n", date.Format("Monday, Jan 2"))

	if len(tasks) > 0 {
		b.WriteString("*Tasks due:*\n")
		for _, t := range tasks {
			icon := priorityIcon(t.Priority)
			fmt.Fprintf(&b, "%s %s", icon, t.Title)

			if t.DueAt.Valid {
				fmt.Fprintf(&b, " _%s_", t.DueAt.Time.Format("3:04 PM"))
			}

			b.WriteString("\n")
		}
	}

	if len(reminders) > 0 {
		if len(tasks) > 0 {
			b.WriteString("\n")
		}

		b.WriteString("*Reminders:*\n")

		for _, r := range reminders {
			fmt.Fprintf(&b, "🔔 %s _%s_\n", r.Title, r.DueAt.Format("3:04 PM"))
		}
	}

	return strings.TrimSpace(b.String())
}

func priorityIcon(priority string) string {
	switch priority {
	case "high":
		return "🔴"
	case "medium":
		return "🟡"
	default:
		return "⚪"
	}
}

func truncateToDayInLocation(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}
