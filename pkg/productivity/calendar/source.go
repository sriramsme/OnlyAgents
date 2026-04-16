package calendar

import (
	"context"
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/memory"
)

type Source struct{ store Store }

func NewSource(s Store) memory.Source {
	return &Source{store: s}
}

func (s *Source) Name() string { return "calendar" }

func (s *Source) Search(ctx context.Context, query string, _ []float32, limit int) ([]memory.Result, error) {
	events, err := s.store.SearchEvents(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("calendar source: %w", err)
	}

	results := make([]memory.Result, 0, len(events))
	now := time.Now()
	for _, ev := range events {
		results = append(results, memory.Result{
			Content:    formatEvent(ev),
			Score:      timeProximityScore(ev.StartTime.Time, now),
			SourceName: s.Name(),
			Metadata:   map[string]any{"event_id": ev.ID},
		})
	}
	return results, nil
}

// formatEvent renders an event as a compact, LLM-readable string.
func formatEvent(ev *CalendarEvent) string {
	start := ev.StartTime.Time
	end := ev.EndTime.Time

	timeRange := fmt.Sprintf("%s – %s", start.Format("Mon Jan 2 3:04PM"), end.Format("3:04PM"))
	if ev.AllDay {
		timeRange = fmt.Sprintf("%s (all day)", start.Format("Mon Jan 2"))
	}

	base := fmt.Sprintf("[%s] %s", timeRange, ev.Title)
	if ev.Location != "" {
		base += " @ " + ev.Location
	}
	if ev.Description != "" {
		base += " — " + ev.Description
	}
	return base
}

// timeProximityScore returns a 0–1 score, highest for events nearest to now.
// Events within 24h score ≥ 0.5; score decays over 30 days.
func timeProximityScore(t, now time.Time) float32 {
	diff := t.Sub(now)
	if diff < 0 {
		diff = -diff
	}
	const decay = float64(30 * 24 * time.Hour)
	score := 1.0 - (float64(diff) / decay)
	if score < 0 {
		score = 0
	}
	return float32(score)
}
