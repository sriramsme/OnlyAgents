package summarizer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// SummarizeYear compresses monthly episodes for the given year into a yearly
// episode, with the prior year's episode as continuity context.
func (s *Summarizer) SummarizeYear(ctx context.Context, year int) error {
	from := time.Date(year, 1, 1, 0, 0, 0, 0, s.loc)
	to := time.Date(year, 12, 31, 23, 59, 59, 0, s.loc)

	monthlies, err := s.store.GetEpisodesByScope(ctx, ScopeMonthly, from, to)
	if err != nil {
		return fmt.Errorf("summarizer: year monthlies: %w", err)
	}

	if len(monthlies) == 0 {
		logger.Log.Info("summarizer: no monthly episodes for year, skipping",
			"year", year)
		return nil
	}

	// Fetch the previous 1 yearly episode for long-range continuity.
	prior, err := s.lastEpisodeBefore(ctx, ScopeYearly, from, 1)
	if err != nil {
		logger.Log.Warn("summarizer: could not fetch prior yearly episode", "err", err)
	}

	summary, err := s.callLLM(ctx, yearlySystemPrompt, buildYearlyPrompt(monthlies, prior, year))
	if err != nil {
		return fmt.Errorf("summarizer: year llm: %w", err)
	}

	ep := &Episode{
		ID:         YearlyEpisodeID(year),
		Scope:      ScopeYearly,
		Summary:    strings.TrimSpace(summary),
		Importance: 1.0,
		StartedAt:  from,
		EndedAt:    to,
		CreatedAt:  time.Now(),
	}

	if err := s.store.SaveEpisode(ctx, ep); err != nil {
		return fmt.Errorf("summarizer: save yearly: %w", err)
	}

	logger.Log.Info("summarizer: yearly episode saved",
		"year", year,
		"months", len(monthlies),
	)

	return nil
}

func YearlyEpisodeID(year int) string {
	return fmt.Sprintf("yearly:%04d", year)
}

func buildYearlyPrompt(monthlies []*Episode, prior []*Episode, year int) string {
	var b strings.Builder

	if len(prior) > 0 {
		b.WriteString("PREVIOUS YEAR CONTEXT (for long-range continuity):\n")
		fmt.Fprintf(&b, "[%d]\n%s\n\n",
			prior[0].StartedAt.Year(),
			strings.TrimSpace(prior[0].Summary),
		)
		b.WriteString("---\n\n")
	}

	fmt.Fprintf(&b, "YEAR: %d\n\n", year)
	b.WriteString("Synthesise these monthly summaries into a yearly overview.\n\n")
	b.WriteString("Monthly summaries:\n")

	for _, m := range monthlies {
		fmt.Fprintf(&b, "[%s]\n%s\n\n",
			m.StartedAt.Format("January 2006"),
			strings.TrimSpace(m.Summary),
		)
	}

	return b.String()
}

const yearlySystemPrompt = `You are the memory system for OnlyAgents, a personal AI agent runtime.

YOUR TASK:
Write a 5-7 sentence yearly summary based on the monthly summaries.
If PREVIOUS YEAR CONTEXT is provided, describe how this year continued or diverged from
that trajectory — do NOT re-summarise the prior year.

Focus on:
- major themes across the year
- key milestones or turning points
- overall trajectory or direction relative to the prior year

Be concise but comprehensive.
OUTPUT: Respond ONLY with plain text. No JSON, no markdown fences, no explanation, no preamble.`
