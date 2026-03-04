package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// Job name constants — used as primary keys in the job_runs table.
const (
	jobDaily       = "daily_summary"
	jobWeekly      = "weekly_summary"
	jobMonthly     = "monthly_summary"
	jobYearly      = "yearly_archive"
	jobDailyDigest = "daily_digest"
)

// DigestDeliverer delivers the daily digest message to the user.
// The kernel implements this by routing through the active channel (Telegram etc.).
// Keeping it as an interface means MemoryManager has no import dependency on
// pkg/channels or pkg/kernel.
type DigestDeliverer interface {
	Deliver(ctx context.Context, message string) error
}

// MemoryManager owns the background summarisation scheduler.
// One instance lives in the kernel alongside ConversationManager.
type MemoryManager struct {
	store      storage.Storage
	summarizer *Summarizer
	scheduler  *cron.Cron
	deliverer  DigestDeliverer // may be nil — digest skipped if not set
}

// NewMemoryManager creates a MemoryManager. llmClient should be the kernel's
// dedicated summarizer client (can be a cheaper/faster model than the agent's).
func NewMemoryManager(store storage.Storage, llmClient llm.Client) *MemoryManager {
	return &MemoryManager{
		store:      store,
		summarizer: newSummarizer(store, llmClient),
		scheduler:  cron.New(),
	}
}

// Start runs catch-up for any missed jobs then launches the cron scheduler.
// Call this once during kernel startup after the storage layer is ready.
func (mm *MemoryManager) Start(ctx context.Context) {
	mm.runCatchUp(ctx)
	mm.registerJobs(ctx)
	mm.scheduler.Start()
	logger.Log.Info("memory: scheduler started")
}

// Stop shuts down the cron scheduler gracefully.
func (mm *MemoryManager) Stop() {
	mm.scheduler.Stop()
	logger.Log.Info("memory: scheduler stopped")
}

func (mm *MemoryManager) registerJobs(ctx context.Context) {
	// Daily at 23:59 — summarise today's messages.
	mm.mustAddFunc("59 23 * * *", jobDaily, func() {
		mm.runJob(ctx, jobDaily, func() error {
			return mm.summarizer.SummarizeDay(ctx, time.Now())
		})
	})

	// Weekly on Sunday at 00:00 — summarise the past week's daily summaries.
	mm.mustAddFunc("0 0 * * 0", jobWeekly, func() {
		mm.runJob(ctx, jobWeekly, func() error {
			// Sunday 00:00: week just ended is the 7 days prior.
			weekEnd := truncateToDay(time.Now()).Add(-time.Second)
			return mm.summarizer.SummarizeWeek(ctx, weekEnd)
		})
	})

	// Monthly on 1st at 00:00 — summarise last month's weekly summaries.
	mm.mustAddFunc("0 0 1 * *", jobMonthly, func() {
		mm.runJob(ctx, jobMonthly, func() error {
			last := time.Now().AddDate(0, -1, 0)
			return mm.summarizer.SummarizeMonth(ctx, last.Year(), int(last.Month()))
		})
	})

	// Yearly on Dec 31 at 23:30 — summarise this year's monthly summaries.
	mm.mustAddFunc("30 23 31 12 *", jobYearly, func() {
		mm.runJob(ctx, jobYearly, func() error {
			return mm.summarizer.SummarizeYear(ctx, time.Now().Year())
		})
	})

	// Daily digest at 8pm: fetch tomorrow's tasks + reminders and deliver.
	mm.mustAddFunc("0 20 * * *", jobDailyDigest, func() {
		mm.runJob(ctx, jobDailyDigest, func() error {
			return mm.deliverDailyDigest(ctx)
		})
	})
}

// runJob executes a job function and records the outcome in job_runs.
func (mm *MemoryManager) runJob(ctx context.Context, name string, fn func() error) {
	logger.Log.Info("memory: running job", "job", name)
	err := fn()
	run := &storage.JobRun{
		JobName: name,
		LastRun: storage.DBTime{Time: time.Now()},
	}
	if err != nil {
		run.LastStatus = "error"
		run.LastError = err.Error()
		logger.Log.Error("memory: job failed", "job", name, "err", err)
	} else {
		run.LastStatus = "ok"
		logger.Log.Info("memory: job completed", "job", name)
	}
	if saveErr := mm.store.SaveJobRun(ctx, run); saveErr != nil {
		logger.Log.Warn("memory: failed to record job run", "job", name, "err", saveErr)
	}
}

// Catch-up

// runCatchUp checks each job on startup and runs any that were missed
// (e.g. the machine was off at the scheduled time).
func (mm *MemoryManager) runCatchUp(ctx context.Context) {
	now := time.Now()
	logger.Log.Info("memory: running catch-up check")

	// Daily: missed if last_run date is before today.
	if mm.missedSince(ctx, jobDaily, truncateToDay(now)) {
		logger.Log.Info("memory: catch-up daily", "date", now.AddDate(0, 0, -1).Format("2006-01-02"))
		mm.runJob(ctx, jobDaily, func() error {
			return mm.summarizer.SummarizeDay(ctx, now.AddDate(0, 0, -1))
		})
	}

	// Weekly: missed if last_run is before the most recent Sunday 00:00.
	lastSunday := lastWeekday(now, time.Sunday)
	if mm.missedSince(ctx, jobWeekly, lastSunday) {
		logger.Log.Info("memory: catch-up weekly", "week_end", lastSunday.Format("2006-01-02"))
		mm.runJob(ctx, jobWeekly, func() error {
			return mm.summarizer.SummarizeWeek(ctx, lastSunday.Add(-time.Second))
		})
	}

	// Monthly: missed if last_run is before the 1st of this month.
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if mm.missedSince(ctx, jobMonthly, firstOfMonth) {
		last := now.AddDate(0, -1, 0)
		logger.Log.Info("memory: catch-up monthly", "year", last.Year(), "month", last.Month())
		mm.runJob(ctx, jobMonthly, func() error {
			return mm.summarizer.SummarizeMonth(ctx, last.Year(), int(last.Month()))
		})
	}

	// Yearly: missed if last_run is before Jan 1 of this year.
	firstOfYear := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	if mm.missedSince(ctx, jobYearly, firstOfYear) {
		logger.Log.Info("memory: catch-up yearly", "year", now.Year()-1)
		mm.runJob(ctx, jobYearly, func() error {
			return mm.summarizer.SummarizeYear(ctx, now.Year()-1)
		})
	}
}

// missedSince returns true if the job has never run or last ran before threshold.
func (mm *MemoryManager) missedSince(ctx context.Context, jobName string, threshold time.Time) bool {
	run, err := mm.store.GetJobRun(ctx, jobName)
	if err != nil {
		logger.Log.Warn("memory: catch-up check failed", "job", jobName, "err", err)
		return false
	}
	if run == nil {
		return true // never ran
	}
	return run.LastRun.Before(threshold)
}

// lastWeekday returns the most recent occurrence of the given weekday at 00:00 UTC.
// If today is that weekday, returns today's 00:00.
func lastWeekday(t time.Time, wd time.Weekday) time.Time {
	day := truncateToDay(t)
	offset := int(day.Weekday()) - int(wd)
	if offset < 0 {
		offset += 7
	}
	return day.AddDate(0, 0, -offset)
}

func (mm *MemoryManager) mustAddFunc(spec string, name string, fn func()) {
	if _, err := mm.scheduler.AddFunc(spec, fn); err != nil {
		logger.Log.Error("memory: failed to register job",
			"job", name,
			"spec", spec,
			"err", err,
		)
	}
}

// ── Daily digest ──────────────────────────────────────────────────────────────

func (mm *MemoryManager) deliverDailyDigest(ctx context.Context) error {
	if mm.deliverer == nil {
		logger.Log.Info("memory: digest deliverer not set, skipping")
		return nil
	}

	tomorrow := time.Now().AddDate(0, 0, 1)

	// Tasks due tomorrow.
	tasks, err := mm.store.GetTasksDueOn(ctx, tomorrow)
	if err != nil {
		return fmt.Errorf("digest: tasks: %w", err)
	}

	// Standalone reminders due tomorrow (before end of day).
	tomorrowStart := truncateToDay(tomorrow)
	tomorrowEnd := tomorrowStart.Add(24*time.Hour - time.Second)
	reminders, err := mm.store.GetDueReminders(ctx, tomorrowEnd)
	if err != nil {
		return fmt.Errorf("digest: reminders: %w", err)
	}
	// Filter to only tomorrow's reminders (GetDueReminders returns everything up to the time).
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

	return mm.deliverer.Deliver(ctx, msg)
}

// formatDigest builds the digest message string.
// Returns "" if there is nothing to report.
func formatDigest(date time.Time, tasks []*storage.Task, reminders []*storage.Reminder) string {
	if len(tasks) == 0 && len(reminders) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("📅 *Tomorrow — %s*\n\n", date.Format("Monday, Jan 2")))

	if len(tasks) > 0 {
		b.WriteString("*Tasks due:*\n")
		for _, t := range tasks {
			icon := priorityIcon(t.Priority)
			b.WriteString(fmt.Sprintf("%s %s", icon, t.Title))
			if t.DueAt.Valid {
				b.WriteString(fmt.Sprintf(" _%s_", t.DueAt.Time.Format("3:04 PM")))
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
			b.WriteString(fmt.Sprintf("🔔 %s _%s_\n", r.Title, r.DueAt.Format("3:04 PM")))
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
