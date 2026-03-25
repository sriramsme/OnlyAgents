package summarizer

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// SummarizeYear compresses all monthly summaries for the given year into a
// YearlyArchive. It is a no-op if there are no monthly summaries for the year.
func (s *Summarizer) SummarizeYear(ctx context.Context, year int) error {
	monthlies, err := s.store.GetMonthlySummaries(ctx, year)
	if err != nil {
		return fmt.Errorf("summarizer: year monthlies: %w", err)
	}
	if len(monthlies) == 0 {
		logger.Log.Info("summarizer: no monthly summaries for year, skipping", "year", year)
		return nil
	}

	raw, err := s.callLLM(ctx, yearlySystemPrompt, buildYearlyPrompt(monthlies))
	if err != nil {
		return fmt.Errorf("summarizer: year llm: %w", err)
	}

	var resp yearlySummaryResponse
	if err := parseJSON(raw, &resp); err != nil {
		return fmt.Errorf("summarizer: year parse: %w", err)
	}

	if err := s.store.SaveYearlyArchive(ctx, &storage.YearlyArchive{
		ID:          uuid.NewString(),
		Year:        year,
		Summary:     resp.Summary,
		MajorEvents: resp.MajorEvents,
		Statistics:  resp.Statistics,
	}); err != nil {
		return fmt.Errorf("summarizer: save yearly: %w", err)
	}

	logger.Log.Info("summarizer: yearly archive saved", "year", year, "months", len(monthlies))
	return nil
}

const yearlySystemPrompt = `You are the memory system for OnlyAgents, a personal AI agent runtime.

YOUR TASK:
Synthesise the provided monthly summaries into a yearly archive.
Identify major life/work events, long-arc themes, and meaningful statistics.
This archive is the permanent long-term record — be comprehensive but concise.

OUTPUT: Respond ONLY with valid JSON. No markdown fences, no explanation, no preamble.`

const yearlyJSONSchema = `{
  "summary": "5-7 sentence narrative of the year",
  "major_events": ["significant event or milestone"],
  "statistics": {"months_active": 12, "dominant_theme": "growth"}
}`

func buildYearlyPrompt(monthlies []*storage.MonthlySummary) string {
	var b strings.Builder
	b.WriteString("Synthesise these monthly summaries into a yearly archive.\n\n")
	b.WriteString("Required JSON schema:\n")
	b.WriteString(yearlyJSONSchema)
	b.WriteString("\n\nMonthly summaries:\n")

	for _, m := range monthlies {
		fmt.Fprintf(&b, "[%d-%02d]\nSummary: %s\nHighlights: %s\n\n",
			m.Year, m.Month,
			m.Summary,
			strings.Join(m.Highlights, "; "),
		)
	}
	return b.String()
}
