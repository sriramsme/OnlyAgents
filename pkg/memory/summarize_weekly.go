package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// SummarizeWeek reads session episodes for the 7-day window ending on weekEnd
// (inclusive), synthesises a weekly narrative, and runs a Praxis extraction
// pass to update behavioral patterns.
//
// No longer depends on daily episodes being present first.
func (s *Summarizer) SummarizeWeek(ctx context.Context, weekEnd time.Time) error {
	weekStart := weekEnd.AddDate(0, 0, -6)

	sessions, err := s.store.GetEpisodesByScope(ctx, ScopeSession, weekStart, weekEnd)
	if err != nil {
		return fmt.Errorf("summarizer: week sessions: %w", err)
	}
	if len(sessions) == 0 {
		logger.Log.Info("summarizer: no sessions for week, skipping",
			"week_start", weekStart.In(s.loc).Format("2006-01-02"))
		return nil
	}

	prior, err := s.lastEpisodeBefore(ctx, ScopeWeekly, weekStart, 2)
	if err != nil {
		logger.Log.Warn("summarizer: could not fetch prior weekly episodes", "err", err)
	}

	// Build input, chunking if needed (same pattern as daily.go).
	var input string
	if estimateTokens(buildWeeklyInputFromSessions(sessions)) > tokenBudget {
		input, err = buildChunkedWeeklyInput(ctx, sessions, s)
		if err != nil {
			return fmt.Errorf("summarizer: chunk weekly input: %w", err)
		}
	} else {
		input = buildWeeklyInputFromSessions(sessions)
	}

	// 1. Narrative weekly summary.
	raw, err := s.callLLM(ctx, weeklySystemPrompt, buildWeeklyPrompt(input, prior, weekStart, s.loc))
	if err != nil {
		return fmt.Errorf("summarizer: week llm: %w", err)
	}

	ep := &Episode{
		ID:         WeeklyEpisodeID(weekStart),
		Scope:      ScopeWeekly,
		Summary:    strings.TrimSpace(raw),
		Importance: 0.8,
		StartedAt:  weekStart,
		EndedAt:    weekEnd,
		CreatedAt:  time.Now(),
	}
	if err := s.store.SaveEpisode(ctx, ep); err != nil {
		return fmt.Errorf("summarizer: save weekly: %w", err)
	}

	logger.Log.Info("summarizer: weekly episode saved",
		"week_start", weekStart.In(s.loc).Format("2006-01-02"),
		"sessions", len(sessions),
	)

	// 2. Praxis extraction pass over the same sessions.
	if err := s.extractPatterns(ctx, sessions, weekStart); err != nil {
		// Non-fatal: log and continue. Praxis is enhancement, not load-bearing.
		logger.Log.Warn("summarizer: praxis extraction failed", "err", err)
	}

	return nil
}

// extractPatterns runs a Praxis extraction LLM call over the week's sessions,
// then upserts the results into PraxisStore.
func (s *Summarizer) extractPatterns(ctx context.Context, sessions []*Episode, weekStart time.Time) error {
	// Fetch existing patterns to give the LLM context for reinforcement/contradiction.
	existing, err := s.store.GetAllPatterns(ctx)
	if err != nil {
		return fmt.Errorf("praxis: fetch existing patterns: %w", err)
	}

	prompt := buildPraxisPrompt(sessions, existing, weekStart, s.loc)
	raw, err := s.callLLM(ctx, praxisSystemPrompt, prompt)
	if err != nil {
		return fmt.Errorf("praxis: llm: %w", err)
	}

	var result praxisExtraction
	if err := parseSessionJSON(raw, &result); err != nil {
		logger.Log.Warn("summarizer: praxis JSON parse failed", "err", err)
		return nil // degrade gracefully
	}

	now := time.Now()

	for _, p := range result.New {
		pat := &Pattern{
			ID:               uuid.New().String(),
			Description:      p.Description,
			Confidence:       0.5, // starts conservative, rises with reinforcement
			ObservationCount: p.ObservationCount,
			FirstObservedAt:  weekStart,
			LastObservedAt:   now,
			CreatedAt:        now,
		}
		if s.embedder != nil {
			pat.Embedding, err = s.embedder.Embed(ctx, p.Description)
			if err != nil {
				logger.Log.Warn("praxis: embedding failed", "err", err, "description", p.Description)
			}
		}
		if err := s.store.SavePattern(ctx, pat); err != nil {
			logger.Log.Warn("praxis: save pattern failed", "err", err, "description", p.Description)
		}
	}

	for _, r := range result.Reinforced {
		if err := s.store.UpdatePattern(ctx, r.ID, +0.1, now); err != nil {
			logger.Log.Warn("praxis: reinforce pattern failed", "err", err, "id", r.ID)
		}
	}

	for _, c := range result.Contradicted {
		if err := s.store.UpdatePattern(ctx, c.ID, -0.15, now); err != nil {
			logger.Log.Warn("praxis: contradict pattern failed", "err", err, "id", c.ID)
		}
	}

	return nil
}

// praxisExtraction is the structured payload from the Praxis LLM call.
type praxisExtraction struct {
	New []struct {
		Description      string `json:"description"`
		ObservationCount int    `json:"observation_count"`
	} `json:"new"`
	Reinforced []struct {
		ID string `json:"id"`
	} `json:"reinforced"`
	Contradicted []struct {
		ID string `json:"id"`
	} `json:"contradicted"`
}

// buildWeeklyInputFromSessions formats session episodes as the text block
// for the weekly prompt.
func buildWeeklyInputFromSessions(sessions []*Episode) string {
	var b strings.Builder
	for _, ep := range sessions {
		fmt.Fprintf(&b, "[%s %s–%s]\n%s\n\n",
			ep.StartedAt.Format("Mon Jan 2"),
			ep.StartedAt.Format("3:04PM"),
			ep.EndedAt.Format("3:04PM"),
			strings.TrimSpace(ep.Summary),
		)
	}
	return b.String()
}

// buildChunkedWeeklyInput pre-summarises each session via a cheap LLM call
// when combined content exceeds tokenBudget. Reuses the same chunk system
// prompt as daily.go.
func buildChunkedWeeklyInput(ctx context.Context, sessions []*Episode, s *Summarizer) (string, error) {
	const chunkSystem = `You are a memory compression assistant for OnlyAgents.
Summarise this conversation session into 2-3 sentences covering the main topics, decisions, and tone.
"user" is the human. Named agents (executive, productivity_agent, etc.) are AI.
Respond with plain prose only — no JSON, no bullet points.`

	var b strings.Builder
	for _, ep := range sessions {
		mini, err := s.callLLM(ctx, chunkSystem, "Session:\n"+ep.Summary)
		if err != nil {
			return "", fmt.Errorf("chunk weekly session %s: %w",
				ep.StartedAt.Format("Mon Jan 2 3:04PM"), err)
		}
		fmt.Fprintf(&b, "[%s %s–%s]\n%s\n\n",
			ep.StartedAt.Format("Mon Jan 2"),
			ep.StartedAt.Format("3:04PM"),
			ep.EndedAt.Format("3:04PM"),
			strings.TrimSpace(mini),
		)
	}
	return b.String(), nil
}

func buildWeeklyPrompt(input string, prior []*Episode, weekStart time.Time, loc *time.Location) string {
	var b strings.Builder

	if len(prior) > 0 {
		b.WriteString("PREVIOUS WEEKLY CONTEXT (most recent first, for continuity):\n")
		for _, p := range prior {
			fmt.Fprintf(&b, "[week of %s]\n%s\n\n",
				p.StartedAt.In(loc).Format("Jan 2"),
				strings.TrimSpace(p.Summary),
			)
		}
		b.WriteString("---\n\n")
	}

	fmt.Fprintf(&b, "WEEK: %s – %s\n\n",
		weekStart.In(loc).Format("Jan 2"),
		weekStart.AddDate(0, 0, 6).In(loc).Format("Jan 2 2006"),
	)
	b.WriteString("Synthesise the following sessions into a weekly overview.\n\n")
	b.WriteString("Sessions:\n")
	b.WriteString(input)
	return b.String()
}

func buildPraxisPrompt(sessions []*Episode, existing []*Pattern, weekStart time.Time, loc *time.Location) string {
	var b strings.Builder

	fmt.Fprintf(&b, "WEEK: %s – %s\n\n",
		weekStart.In(loc).Format("Jan 2"),
		weekStart.AddDate(0, 0, 6).In(loc).Format("Jan 2 2006"),
	)

	b.WriteString("SESSION SUMMARIES:\n")
	for _, ep := range sessions {
		fmt.Fprintf(&b, "[%s]\n%s\n\n",
			ep.StartedAt.Format("Mon Jan 2 3:04PM"),
			strings.TrimSpace(ep.Summary),
		)
	}

	if len(existing) > 0 {
		b.WriteString("---\n\nEXISTING BEHAVIORAL PATTERNS:\n")
		for _, p := range existing {
			fmt.Fprintf(&b, "[id:%s confidence:%.2f] %s\n", p.ID, p.Confidence, p.Description)
		}
	}

	return b.String()
}

func WeeklyEpisodeID(weekStart time.Time) string {
	return "weekly:" + weekStart.UTC().Format("2006-01-02")
}

const weeklySystemPrompt = `You are the memory system for OnlyAgents, a personal AI agent runtime.

YOUR TASK:
Synthesise the provided session summaries into a weekly overview.
If PREVIOUS WEEKLY CONTEXT is provided, use it to track ongoing threads — do NOT
re-summarise it, only let it inform what evolved this week.
Focus on dominant themes and meaningful progressions.

OUTPUT: Respond ONLY with plain text. No JSON, no markdown fences, no explanation.`

const praxisSystemPrompt = `You are the behavioral pattern extractor for OnlyAgents.

Analyze the week's session summaries and identify behavioral patterns about the user.
Patterns describe HOW the user works, communicates, and thinks — not facts about the world.

Examples of good patterns:
- "Prefers code examples before explanation during implementation discussions"
- "Wants to think through architecture verbally before committing to a design"
- "Gets more detailed in questions when frustrated or blocked"

Rules:
- NEW patterns must appear at least 3 times across the sessions to be recorded
- Only record what is clearly demonstrated, not inferred
- Compare against EXISTING PATTERNS: reinforce or contradict them by ID if evidence exists
- Contradicted patterns should have clear counter-evidence, not just absence

Return ONLY a single JSON object, no markdown fences:
{
  "new": [{"description": "string", "observation_count": 3}],
  "reinforced": [{"id": "string"}],
  "contradicted": [{"id": "string"}]
}`
