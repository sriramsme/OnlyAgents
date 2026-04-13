package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// SummarizeMonth compresses weekly episodes for the given year/month into a
// monthly episode, with the previous month's episode as continuity context.
func (s *Summarizer) SummarizeMonth(ctx context.Context, year, month int) error {
	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, s.loc)
	to := from.AddDate(0, 1, -1)

	weeklies, err := s.store.GetEpisodesByScope(ctx, ScopeWeekly, from, to)
	if err != nil {
		return fmt.Errorf("summarizer: month weeklies: %w", err)
	}

	if len(weeklies) == 0 {
		logger.Log.Info("summarizer: no weekly episodes for month, skipping",
			"year", year, "month", month)
		return nil
	}

	// Fetch the previous 1 monthly episode for continuity.
	prior, err := s.lastEpisodeBefore(ctx, ScopeMonthly, from, 1)
	if err != nil {
		logger.Log.Warn("summarizer: could not fetch prior monthly episode", "err", err)
	}

	raw, err := s.callLLM(ctx, monthlySystemPrompt, buildMonthlyPrompt(weeklies, prior, from, s.loc))
	if err != nil {
		return fmt.Errorf("summarizer: month llm: %w", err)
	}

	ep := &Episode{
		ID:         MonthlyEpisodeID(year, month),
		Scope:      ScopeMonthly,
		Summary:    strings.TrimSpace(raw),
		Importance: 0.9,
		StartedAt:  dbtypes.DBTime{Time: from},
		EndedAt:    dbtypes.DBTime{Time: to},
		CreatedAt:  dbtypes.DBTime{Time: time.Now()},
	}

	if err := s.store.SaveEpisode(ctx, ep); err != nil {
		return fmt.Errorf("summarizer: save monthly: %w", err)
	}

	logger.Log.Info("summarizer: monthly episode saved",
		"year", year,
		"month", month,
		"weeks", len(weeklies),
	)

	return nil
}

func MonthlyEpisodeID(year, month int) string {
	return fmt.Sprintf("monthly:%04d-%02d", year, month)
}

func buildMonthlyPrompt(weeklies []*Episode, prior []*Episode, monthStart time.Time, loc *time.Location) string {
	var b strings.Builder

	if len(prior) > 0 {
		b.WriteString("PREVIOUS MONTH CONTEXT (for continuity):\n")
		fmt.Fprintf(&b, "[%s]\n%s\n\n",
			prior[0].StartedAt.In(loc).Format("January 2006"),
			strings.TrimSpace(prior[0].Summary),
		)
		b.WriteString("---\n\n")
	}

	fmt.Fprintf(&b, "MONTH: %s\n\n", monthStart.In(loc).Format("January 2006"))
	b.WriteString("Synthesise these weekly summaries into a monthly overview.\n\n")
	b.WriteString("Weekly summaries:\n")

	// Weight each week's share of the month by relative importance.
	total := float32(0)
	for _, w := range weeklies {
		total += w.Importance
	}

	for _, w := range weeklies {
		pct := 0
		if total > 0 {
			pct = int((w.Importance / total) * 100)
		}
		fmt.Fprintf(&b, "[%s – %s · %d%%]\n%s\n\n",
			w.StartedAt.In(loc).Format("Jan 2"),
			w.EndedAt.In(loc).Format("Jan 2"),
			pct,
			strings.TrimSpace(w.Summary),
		)
	}

	return b.String()
}

const monthlySystemPrompt = `You are the memory system for OnlyAgents, a personal AI agent runtime.

YOUR TASK:
Write a concise 3-5 sentence monthly summary based on the weekly summaries.
If PREVIOUS MONTH CONTEXT is provided, note how this month continued, diverged from,
or resolved threads from that prior period — do NOT re-summarise the previous month.

Focus on:
- dominant patterns across the month
- major outcomes and decisions
- shifts in direction or focus

OUTPUT: Respond ONLY with plain text. No JSON, no markdown fences, no explanation, no preamble.`
