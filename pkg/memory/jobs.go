package memory

import (
	"context"
	"fmt"
	"time"

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

func truncateToDayInLocation(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
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
