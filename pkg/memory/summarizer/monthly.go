package summarizer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// SummarizeMonth compresses all weekly summaries for the given year/month into
// a MonthlySummary. It is a no-op if there are no weekly summaries for the period.
func (s *Summarizer) SummarizeMonth(ctx context.Context, year, month int) error {
	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, s.loc)
	to := from.AddDate(0, 1, -1) // last day of month

	weeklies, err := s.store.GetWeeklySummaries(ctx, from, to)
	if err != nil {
		return fmt.Errorf("summarizer: month weeklies: %w", err)
	}
	if len(weeklies) == 0 {
		logger.Log.Info("summarizer: no weekly summaries for month, skipping",
			"year", year, "month", month)
		return nil
	}

	raw, err := s.callLLM(ctx, monthlySystemPrompt, buildMonthlyPrompt(weeklies))
	if err != nil {
		return fmt.Errorf("summarizer: month llm: %w", err)
	}

	var resp monthlySummaryResponse
	if err := parseJSON(raw, &resp); err != nil {
		return fmt.Errorf("summarizer: month parse: %w", err)
	}

	if err := s.store.SaveMonthlySummary(ctx, &storage.MonthlySummary{
		ID:         uuid.NewString(),
		Year:       year,
		Month:      month,
		Summary:    resp.Summary,
		Highlights: resp.Highlights,
		Statistics: resp.Statistics,
	}); err != nil {
		return fmt.Errorf("summarizer: save monthly: %w", err)
	}

	logger.Log.Info("summarizer: monthly summary saved",
		"year", year, "month", month, "weeks", len(weeklies))
	return nil
}

const monthlySystemPrompt = `You are the memory system for OnlyAgents, a personal AI agent runtime.

YOUR TASK:
Synthesise the provided weekly summaries into a monthly overview.
Each weekly entry includes themes and achievements. Surface the month's dominant
patterns, what was accomplished, and any notable shifts in activity or focus.

OUTPUT: Respond ONLY with valid JSON. No markdown fences, no explanation, no preamble.`

const monthlyJSONSchema = `{
  "summary": "3-5 sentence narrative of the month",
  "highlights": ["most notable event or accomplishment"],
  "statistics": {"weeks_active": 4, "dominant_theme": "work"}
}`

func buildMonthlyPrompt(weeklies []*storage.WeeklySummary) string {
	var b strings.Builder
	b.WriteString("Synthesise these weekly summaries into a monthly overview.\n\n")
	b.WriteString("Required JSON schema:\n")
	b.WriteString(monthlyJSONSchema)
	b.WriteString("\n\nWeekly summaries:\n")

	for _, w := range weeklies {
		fmt.Fprintf(&b, "[%s – %s]\nSummary: %s\nThemes: %s\nAchievements: %s\n\n",
			w.WeekStart.Format("Jan 2"),
			w.WeekEnd.Format("Jan 2"),
			w.Summary,
			strings.Join(w.Themes, "; "),
			strings.Join(w.Achievements, "; "),
		)
	}
	return b.String()
}
