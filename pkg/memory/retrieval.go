package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// MemoryContext is the enriched context assembled before each LLM call.
// It is separate from raw conversation history — it carries compressed
// longer-term memory and relevant facts rather than recent message turns.
type MemoryContext struct {
	TodaySummary  *storage.DailySummary
	RecentSummary *storage.WeeklySummary // most recent week, if today's summary absent
	RelevantFacts []*storage.Fact
}

// GetRelevantMemory assembles long-term memory context relevant to the given
// query. Called by the agent in execute() before building the messages slice.
// query is typically the user's current message — used for FTS fact search.
func (mm *MemoryManager) GetRelevantMemory(ctx context.Context, sessionAgentID string, query string) (*MemoryContext, error) {
	mc := &MemoryContext{}

	// 1. Today's daily summary (most recent compressed context).
	today, err := mm.store.GetDailySummary(ctx, sessionAgentID, time.Now())
	if err == nil {
		mc.TodaySummary = today
	}

	// 2. If no daily summary yet (early in the day), pull the most recent
	//    weekly summary so the agent still has some longer-term context.
	if mc.TodaySummary == nil {
		now := time.Now()
		weeklies, err := mm.store.GetWeeklySummaries(ctx, sessionAgentID,
			now.AddDate(0, 0, -7), now)
		if err == nil && len(weeklies) > 0 {
			mc.RecentSummary = weeklies[len(weeklies)-1]
		}
	}

	// 3. Facts relevant to the query via FTS5.
	if query != "" {
		facts, err := mm.store.SearchFacts(ctx, sessionAgentID, query)
		if err == nil {
			mc.RelevantFacts = facts
		}
	}

	return mc, nil
}

// FormatMemoryContext converts a MemoryContext into a plain-text block
// suitable for injection as a system message before the conversation history.
// Returns an empty string if there is nothing meaningful to inject.
func FormatMemoryContext(mc *MemoryContext) string {
	if mc == nil {
		return ""
	}

	var b strings.Builder

	if mc.TodaySummary != nil {
		b.WriteString("## Memory: Today So Far\n")
		b.WriteString(mc.TodaySummary.Summary)

		if len(mc.TodaySummary.KeyEvents) > 0 {
			b.WriteString("\nKey events: ")
			b.WriteString(strings.Join(mc.TodaySummary.KeyEvents, ", "))
		}

		b.WriteString("\n\n")

	} else if mc.RecentSummary != nil {
		b.WriteString("## Memory: Recent Week\n")

		fmt.Fprintf(&b, "(%s – %s) ",
			mc.RecentSummary.WeekStart.Format("Jan 2"),
			mc.RecentSummary.WeekEnd.Format("Jan 2"),
		)

		b.WriteString(mc.RecentSummary.Summary)
		b.WriteString("\n\n")
	}

	if len(mc.RelevantFacts) > 0 {
		b.WriteString("## Relevant Facts\n")

		for _, f := range mc.RelevantFacts {
			fmt.Fprintf(&b, "- [%s] %s: %s\n",
				f.EntityType,
				f.Entity,
				f.Fact,
			)
		}

		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}
