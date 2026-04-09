package summarizer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/message"
)

// sessionGap is the idle period between messages that delimits two separate
// conversation sessions within the same calendar day.
const sessionGap = 30 * time.Minute

// tokenBudget is the maximum estimated tokens we send as message content in
// a single daily summarisation call.
const tokenBudget = 80_000

// maxUserMsgLen is the per-message character cap for user content.
const maxUserMsgLen = 800

// maxAssistantMsgLen is the per-message character cap for assistant content.
// We truncate aggressively — what matters is the user's reaction.
const maxAssistantMsgLen = 300

// msgSession is a contiguous block of messages with no inter-message gap
// reaching sessionGap. Sessions are the primary unit of the daily prompt.
type msgSession struct {
	start    time.Time
	end      time.Time
	messages []*message.Message
	agents   []string // deduplicated agent IDs that sent messages in this session
}

// SummarizeDay compresses all session episodes for the given calendar day into
// a daily episode. It fetches the previous daily episode to give the LLM
// continuity context.
//
// Call order: SummarizeSessions must be run first.
func (s *Summarizer) SummarizeDay(ctx context.Context, date time.Time) error {
	from, to := dayBounds(date, s.loc)

	sessions, err := s.store.GetEpisodesByScope(ctx, ScopeSession, from, to)
	if err != nil {
		return fmt.Errorf("summarizer: fetch sessions: %w", err)
	}

	if len(sessions) == 0 {
		logger.Log.Info("summarizer: no sessions for day, skipping",
			"date", from.In(s.loc).Format("2006-01-02"))
		return nil
	}

	// Fetch the previous 2 daily episodes for continuity.
	prior, err := s.lastEpisodeBefore(ctx, ScopeDaily, from, 2)
	if err != nil {
		logger.Log.Warn("summarizer: could not fetch prior daily episodes", "err", err)
	}

	var input string
	if estimateTokens(buildDailyInputFromSessions(sessions)) > tokenBudget {
		input, err = buildChunkedDailyInput(ctx, sessions, s)
		if err != nil {
			return fmt.Errorf("summarizer: chunk daily input: %w", err)
		}
	} else {
		input = buildDailyInputFromSessions(sessions)
	}

	userPrompt := buildDailyPrompt(input, prior, from, s.loc)

	raw, err := s.callLLM(ctx, dailySystemPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("summarizer: day llm: %w", err)
	}

	ep := &Episode{
		ID:         DailyEpisodeID(from),
		Scope:      ScopeDaily,
		Summary:    strings.TrimSpace(raw),
		Importance: 0.7,
		StartedAt:  from,
		EndedAt:    to,
		CreatedAt:  time.Now(),
	}

	if err := s.store.SaveEpisode(ctx, ep); err != nil {
		return fmt.Errorf("summarizer: save daily: %w", err)
	}

	logger.Log.Info("summarizer: daily episode saved",
		"date", from.In(s.loc).Format("2006-01-02"),
		"sessions", len(sessions),
	)

	return nil
}

func DailyEpisodeID(from time.Time) string {
	return "daily:" + from.UTC().Format("2006-01-02")
}

// buildDailyInputFromSessions formats session episodes into the text block
// injected into the daily prompt.
func buildDailyInputFromSessions(sessions []*Episode) string {
	var b strings.Builder
	for _, ep := range sessions {
		fmt.Fprintf(&b, "[%s – %s]\n%s\n\n",
			ep.StartedAt.Format("3:04PM"),
			ep.EndedAt.Format("3:04PM"),
			strings.TrimSpace(ep.Summary),
		)
	}
	return b.String()
}

// renderSession formats a single raw msgSession (live messages) for the
// session-level summarisation prompt.
func renderSession(sess msgSession) string {
	var b strings.Builder

	fmt.Fprintf(&b, "[%s – %s]\n",
		sess.start.Format("3:04PM"),
		sess.end.Format("3:04PM"),
	)

	for _, m := range sess.messages {
		label := m.Role
		if m.Role == "assistant" && m.AgentID != "" {
			label = m.AgentID
		}
		fmt.Fprintf(&b, "%s: %s\n", label, truncateMsg(m.Role, m.Content))
	}

	return b.String()
}

// buildChunkedDailyInput pre-summarises each session episode via a cheap LLM
// call, then concatenates those mini-summaries. Used when full content exceeds
// tokenBudget.
func buildChunkedDailyInput(ctx context.Context, sessions []*Episode, s *Summarizer) (string, error) {
	const chunkSystem = `You are a memory compression assistant for OnlyAgents.
Summarise this conversation session into 2-3 sentences covering the main topics, decisions, and tone.
"user" is the human. Named agents (executive, productivity_agent, etc.) are AI.
Respond with plain prose only — no JSON, no bullet points.`

	var b strings.Builder

	for _, ep := range sessions {
		mini, err := s.callLLM(ctx, chunkSystem, "Session:\n"+ep.Summary)
		if err != nil {
			return "", fmt.Errorf("chunk summarise session %s: %w",
				ep.StartedAt.Format("3:04PM"), err)
		}

		fmt.Fprintf(&b, "[%s – %s]\n%s\n\n",
			ep.StartedAt.Format("3:04PM"),
			ep.EndedAt.Format("3:04PM"),
			strings.TrimSpace(mini),
		)
	}
	return b.String(), nil
}

const dailySystemPrompt = `You are the memory system for OnlyAgents, a personal AI agent runtime.

SYSTEM ARCHITECTURE:
OnlyAgents runs a multi-agent system. One executive agent handles all user-facing
conversation — it understands intent, decomposes tasks, and delegates to specialised
sub-agents (e.g. productivity_agent, researcher_agent). In the message history below,
"user" is always the human. All other labels are AI agents.

YOUR TASK:
Compress today's conversation sessions into a daily summary.
If PREVIOUS DAILY CONTEXT is provided, use it to understand ongoing threads and
trajectories — do NOT re-summarise it, only let it inform what is new or changed today.

OUTPUT: Respond ONLY with plain text. No JSON, no markdown fences, no explanation, no preamble.`

// buildDailyPrompt assembles the user-turn, prepending prior daily context
// (most recent first) when available.
func buildDailyPrompt(sessionContent string, prior []*Episode, date time.Time, loc *time.Location) string {
	var b strings.Builder

	if len(prior) > 0 {
		b.WriteString("PREVIOUS DAILY CONTEXT (most recent first, for continuity):\n")
		for _, p := range prior {
			fmt.Fprintf(&b, "[%s]\n%s\n\n",
				p.StartedAt.In(loc).Format("Monday, Jan 2"),
				strings.TrimSpace(p.Summary),
			)
		}
		b.WriteString("---\n\n")
	}

	fmt.Fprintf(&b, "DATE: %s\n\n", date.In(loc).Format("Monday, January 2 2006"))
	b.WriteString("Summarise the following conversation sessions.\n\n")
	b.WriteString("Sessions:\n")
	b.WriteString(sessionContent)
	return b.String()
}

// truncateMsg applies role-specific length caps before prompt injection.
func truncateMsg(role, content string) string {
	limit := maxUserMsgLen
	if role == "assistant" {
		limit = maxAssistantMsgLen
	}
	if len(content) <= limit {
		return content
	}
	return content[:limit] + "…[truncated]"
}
