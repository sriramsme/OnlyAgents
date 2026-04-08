package summarizer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// SummarizeWeek compresses the daily summaries for the 7-day window ending on
// weekEnd (inclusive) into a WeeklySummary.
//
// weekEnd should be a Sunday in the Summarizer's configured timezone. The cron
// scheduler is responsible for passing the correct date.
//
// It is a no-op if there are no daily summaries for the window.
func (s *Summarizer) SummarizeWeek(ctx context.Context, weekEnd time.Time) error {
	weekStart := weekEnd.AddDate(0, 0, -6)

	dailies, err := s.store.GetDailySummaries(ctx, weekStart, weekEnd)
	if err != nil {
		return fmt.Errorf("summarizer: week dailies: %w", err)
	}
	if len(dailies) == 0 {
		logger.Log.Info("summarizer: no daily summaries for week, skipping",
			"week_start", weekStart.In(s.loc).Format("2006-01-02"))
		return nil
	}

	raw, err := s.callLLM(ctx, weeklySystemPrompt, buildWeeklyPrompt(dailies))
	if err != nil {
		return fmt.Errorf("summarizer: week llm: %w", err)
	}

	var resp weeklySummaryResponse
	if err := parseJSON(raw, &resp); err != nil {
		return fmt.Errorf("summarizer: week parse: %w", err)
	}

	if err := s.store.SaveWeeklySummary(ctx, &storage.WeeklySummary{
		ID:           uuid.NewString(),
		WeekStart:    dbtypes.DBTime{Time: weekStart.UTC()},
		WeekEnd:      dbtypes.DBTime{Time: weekEnd.UTC()},
		Summary:      resp.Summary,
		Themes:       resp.Themes,
		Achievements: resp.Achievements,
	}); err != nil {
		return fmt.Errorf("summarizer: save weekly: %w", err)
	}

	logger.Log.Info("summarizer: weekly summary saved",
		"week_start", weekStart.In(s.loc).Format("2006-01-02"),
		"days", len(dailies),
	)
	return nil
}

const weeklySystemPrompt = `You are the memory system for OnlyAgents, a personal AI agent runtime.

YOUR TASK:
Synthesise the provided daily summaries into a weekly overview.
Each daily entry includes a prose summary, key events, and topics with message-share
weights. Use those weights to identify dominant themes across the week.

OUTPUT: Respond ONLY with valid JSON. No markdown fences, no explanation, no preamble.`

const weeklyJSONSchema = `{
  "summary": "3-5 sentence narrative of the week",
  "themes": ["dominant recurring theme", "secondary theme"],
  "achievements": ["concrete thing completed or decided"]
}`

func buildWeeklyPrompt(dailies []*storage.DailySummary) string {
	var b strings.Builder
	b.WriteString("Synthesise these daily summaries into a weekly overview.\n\n")
	b.WriteString("Required JSON schema:\n")
	b.WriteString(weeklyJSONSchema)
	b.WriteString("\n\nDaily summaries:\n")

	for _, d := range dailies {
		// Render topics as "topic(share%)" for compact but informative input.
		var topicParts []string
		for _, t := range d.Topics {
			topicParts = append(topicParts,
				fmt.Sprintf("%s(%.0f%%,%s)", t.Topic, t.MessageShare*100, t.Sentiment))
		}

		fmt.Fprintf(&b, "[%s]\nSummary: %s\nKey events: %s\nTopics: %s\n\n",
			d.Date.Format("2006-01-02"),
			d.Summary,
			strings.Join(d.KeyEvents, "; "),
			strings.Join(topicParts, ", "),
		)
	}
	return b.String()
}
