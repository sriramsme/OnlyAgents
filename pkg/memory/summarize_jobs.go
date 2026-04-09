package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
)

// RegisterJobs registers all memory compression jobs with the provided
// scheduler. Call this during kernel boot, before scheduler.Start.
func (s *Summarizer) RegisterJobs(sched *scheduler.Scheduler) {
	sched.Register(&dailySummaryJob{s: s})
	sched.Register(&weeklySummaryJob{s: s})
	sched.Register(&monthlySummaryJob{s: s})
	sched.Register(&yearlySummaryJob{s: s})
}

// ── Daily summary ─────────────────────────────────────────────────────────────

type dailySummaryJob struct{ s *Summarizer }

func (j *dailySummaryJob) Name() string     { return "daily_summary" }
func (j *dailySummaryJob) Schedule() string { return "59 23 * * *" }

func (j *dailySummaryJob) Run(ctx context.Context) error {
	return j.s.SummarizeDay(ctx, time.Now())
}

// CatchUp runs SummarizeDay for yesterday if a daily episode for that date
// doesn't already exist in the store.
func (j *dailySummaryJob) CatchUp(ctx context.Context) error {
	yesterday := time.Now().AddDate(0, 0, -1)
	from, to := dayBounds(yesterday, j.s.loc)

	eps, err := j.s.store.GetEpisodesByScope(ctx, ScopeDaily, from, to)
	if err != nil {
		return fmt.Errorf("daily catch-up check: %w", err)
	}
	if len(eps) > 0 {
		return nil // already have yesterday
	}

	logger.Log.Info("summarizer: catch-up daily", "date", yesterday.Format("2006-01-02"))
	return j.s.SummarizeDay(ctx, yesterday)
}

// ── Weekly summary ────────────────────────────────────────────────────────────

type weeklySummaryJob struct{ s *Summarizer }

func (j *weeklySummaryJob) Name() string     { return "weekly_summary" }
func (j *weeklySummaryJob) Schedule() string { return "0 0 * * 0" }

func (j *weeklySummaryJob) Run(ctx context.Context) error {
	weekEnd := truncateToDayInLocation(time.Now(), j.s.Loc()).Add(-time.Second)
	return j.s.SummarizeWeek(ctx, weekEnd)
}

// CatchUp runs SummarizeWeek for the last Sunday if no weekly episode exists
// for that window.
func (j *weeklySummaryJob) CatchUp(ctx context.Context) error {
	lastSunday := lastWeekday(time.Now(), time.Sunday, j.s.Loc())
	from := lastSunday.AddDate(0, 0, -6)

	eps, err := j.s.store.GetEpisodesByScope(ctx, ScopeWeekly, from, lastSunday)
	if err != nil {
		return fmt.Errorf("weekly catch-up check: %w", err)
	}
	if len(eps) > 0 {
		return nil // already have this week
	}

	logger.Log.Info("summarizer: catch-up weekly", "week_end", lastSunday.Format("2006-01-02"))
	return j.s.SummarizeWeek(ctx, lastSunday.Add(-time.Second))
}

// ── Monthly summary ───────────────────────────────────────────────────────────

type monthlySummaryJob struct{ s *Summarizer }

func (j *monthlySummaryJob) Name() string     { return "monthly_summary" }
func (j *monthlySummaryJob) Schedule() string { return "0 0 1 * *" }

func (j *monthlySummaryJob) Run(ctx context.Context) error {
	last := time.Now().AddDate(0, -1, 0)
	return j.s.SummarizeMonth(ctx, last.Year(), int(last.Month()))
}

// CatchUp runs SummarizeMonth for last month if no monthly episode exists.
func (j *monthlySummaryJob) CatchUp(ctx context.Context) error {
	last := time.Now().AddDate(0, -1, 0)
	from := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, j.s.loc)
	to := from.AddDate(0, 1, -1)

	eps, err := j.s.store.GetEpisodesByScope(ctx, ScopeMonthly, from, to)
	if err != nil {
		return fmt.Errorf("monthly catch-up check: %w", err)
	}
	if len(eps) > 0 {
		return nil // already have last month
	}

	logger.Log.Info("summarizer: catch-up monthly", "year", last.Year(), "month", last.Month())
	return j.s.SummarizeMonth(ctx, last.Year(), int(last.Month()))
}

// ── Yearly archive ────────────────────────────────────────────────────────────

type yearlySummaryJob struct{ s *Summarizer }

func (j *yearlySummaryJob) Name() string     { return "yearly_summary" }
func (j *yearlySummaryJob) Schedule() string { return "30 23 31 12 *" }

func (j *yearlySummaryJob) Run(ctx context.Context) error {
	return j.s.SummarizeYear(ctx, time.Now().Year())
}

// CatchUp runs SummarizeYear for last year if no yearly episode exists.
func (j *yearlySummaryJob) CatchUp(ctx context.Context) error {
	lastYear := time.Now().Year() - 1
	scope := ScopeYearly
	eps, err := j.s.store.SearchEpisodes(ctx, EpisodeQuery{
		Scope: &scope,
		Limit: 1,
	})
	if err != nil {
		return fmt.Errorf("yearly catch-up check: %w", err)
	}
	for _, ep := range eps {
		if ep.StartedAt.Year() == lastYear {
			return nil // already have last year
		}
	}

	logger.Log.Info("summarizer: catch-up yearly", "year", lastYear)
	return j.s.SummarizeYear(ctx, lastYear)
}

// ── Time helpers ──────────────────────────────────────────────────────────────

func truncateToDayInLocation(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

// lastWeekday returns the most recent occurrence of wd at 00:00 in loc.
// If today is wd, returns today at 00:00.
func lastWeekday(t time.Time, wd time.Weekday, loc *time.Location) time.Time {
	day := truncateToDayInLocation(t, loc)
	offset := int(day.Weekday()) - int(wd)
	if offset < 0 {
		offset += 7
	}
	return day.AddDate(0, 0, -offset)
}
