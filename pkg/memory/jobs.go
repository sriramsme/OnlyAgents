package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/memory/summarizer"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// ── Daily summary ─────────────────────────────────────────────────────────────

type dailySummaryJob struct {
	summarizer *summarizer.Summarizer
	store      storage.Storage
}

func (j *dailySummaryJob) Name() string     { return "daily_summary" }
func (j *dailySummaryJob) Schedule() string { return "59 23 * * *" }

func (j *dailySummaryJob) Run(ctx context.Context) error {
	return j.summarizer.SummarizeDay(ctx, time.Now())
}

// CatchUp runs if the last daily summary is not from today.
func (j *dailySummaryJob) CatchUp(ctx context.Context) error {
	yesterday := time.Now().AddDate(0, 0, -1)
	_, err := j.store.GetDailySummary(ctx, yesterday)
	if err == nil {
		return nil // already summarized yesterday
	}
	logger.Log.Info("memory: catch-up daily", "date", yesterday.Format("2006-01-02"))
	return j.summarizer.SummarizeDay(ctx, yesterday)
}

// ── Weekly summary ────────────────────────────────────────────────────────────

type weeklySummaryJob struct {
	summarizer *summarizer.Summarizer
	store      storage.Storage
}

func (j *weeklySummaryJob) Name() string     { return "weekly_summary" }
func (j *weeklySummaryJob) Schedule() string { return "0 0 * * 0" }

func (j *weeklySummaryJob) Run(ctx context.Context) error {
	weekEnd := truncateToDayInLocation(time.Now(), j.summarizer.Loc()).Add(-time.Second)
	return j.summarizer.SummarizeWeek(ctx, weekEnd)
}

func (j *weeklySummaryJob) CatchUp(ctx context.Context) error {
	lastSunday := lastWeekday(time.Now(), time.Sunday, j.summarizer.Loc())
	from := lastSunday.AddDate(0, 0, -6)
	weeklies, err := j.store.GetWeeklySummaries(ctx, from, lastSunday)
	if err != nil {
		return fmt.Errorf("weekly catch-up check: %w", err)
	}
	if len(weeklies) > 0 {
		return nil // already have this week
	}
	logger.Log.Info("memory: catch-up weekly", "week_end", lastSunday.Format("2006-01-02"))
	return j.summarizer.SummarizeWeek(ctx, lastSunday.Add(-time.Second))
}

// ── Monthly summary ───────────────────────────────────────────────────────────

type monthlySummaryJob struct {
	summarizer *summarizer.Summarizer
	store      storage.Storage
}

func (j *monthlySummaryJob) Name() string     { return "monthly_summary" }
func (j *monthlySummaryJob) Schedule() string { return "0 0 1 * *" }

func (j *monthlySummaryJob) Run(ctx context.Context) error {
	last := time.Now().AddDate(0, -1, 0)
	return j.summarizer.SummarizeMonth(ctx, last.Year(), int(last.Month()))
}

func (j *monthlySummaryJob) CatchUp(ctx context.Context) error {
	last := time.Now().AddDate(0, -1, 0)
	monthlies, err := j.store.GetMonthlySummaries(ctx, last.Year())
	if err != nil {
		return fmt.Errorf("monthly catch-up check: %w", err)
	}
	for _, m := range monthlies {
		if m.Month == int(last.Month()) {
			return nil // already have last month
		}
	}
	logger.Log.Info("memory: catch-up monthly", "year", last.Year(), "month", last.Month())
	return j.summarizer.SummarizeMonth(ctx, last.Year(), int(last.Month()))
}

// ── Yearly archive ────────────────────────────────────────────────────────────

type yearlySummaryJob struct {
	summarizer *summarizer.Summarizer
	store      storage.Storage
}

func (j *yearlySummaryJob) Name() string     { return "yearly_summary" }
func (j *yearlySummaryJob) Schedule() string { return "30 23 31 12 *" }

func (j *yearlySummaryJob) Run(ctx context.Context) error {
	return j.summarizer.SummarizeYear(ctx, time.Now().Year())
}

func (j *yearlySummaryJob) CatchUp(ctx context.Context) error {
	lastYear := time.Now().Year() - 1
	_, err := j.store.GetYearlyArchive(ctx, lastYear)
	if err == nil {
		return nil // already have last year
	}
	logger.Log.Info("memory: catch-up yearly", "year", lastYear)
	return j.summarizer.SummarizeYear(ctx, lastYear)
}

// ── Daily digest ──────────────────────────────────────────────────────────────

type dailyDigestJob struct {
	store     storage.Storage
	deliverer channels.Channel
	loc       *time.Location
}

func (j *dailyDigestJob) Name() string     { return "daily_digest" }
func (j *dailyDigestJob) Schedule() string { return "0 20 * * *" }

// Digest has no catch-up — a missed evening digest is just skipped.
func (j *dailyDigestJob) Run(ctx context.Context) error {
	if j.deliverer == nil {
		logger.Log.Info("memory: digest deliverer not set, skipping")
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

	var tomorrowReminders []*storage.Reminder
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

	message := channels.OutgoingMessage{
		Content: msg,
	}
	return j.deliverer.Send(ctx, message)
}

// lastWeekday returns the most recent occurrence of the given weekday at 00:00 UTC.
// If today is that weekday, returns today's 00:00.
func lastWeekday(t time.Time, wd time.Weekday, loc *time.Location) time.Time {
	day := truncateToDayInLocation(t, loc)
	offset := int(day.Weekday()) - int(wd)
	if offset < 0 {
		offset += 7
	}
	return day.AddDate(0, 0, -offset)
}

// formatDigest builds the digest message string.
// Returns "" if there is nothing to report.
func formatDigest(date time.Time, tasks []*storage.Task, reminders []*storage.Reminder) string {
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
