package summarizer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/message"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// Constants

// sessionGap is the idle period between messages that delimits two separate
// conversation sessions within the same calendar day.
const sessionGap = 30 * time.Minute

// tokenBudget is the maximum estimated tokens we send as message content in
// a single daily summarisation call. Leaves headroom for system prompt and
// the model's response. Override via config if your model has a larger window.
const tokenBudget = 80_000

// maxUserMsgLen is the per-message character cap for user content.
// User messages carry the most signal and are given the most room.
const maxUserMsgLen = 800

// maxAssistantMsgLen is the per-message character cap for assistant content.
// We truncate aggressively: what matters most is the user's reaction to the
// assistant, not the full assistant response. The system prompt tells the LLM
// about this truncation so it does not misread shortened messages as errors.
const maxAssistantMsgLen = 300

// Session type

// msgSession is a contiguous block of messages with no inter-message gap
// reaching sessionGap. Sessions are the primary unit of the daily prompt.
type msgSession struct {
	start    time.Time
	end      time.Time
	messages []*message.Message
	agents   []string // deduplicated agent IDs that sent messages in this session
}

// SummarizeDay

// SummarizeDay compresses all messages for the given calendar day into a
// DailySummary and extracts durable facts. It is a no-op if there are no
// messages for the day.
//
// The date argument may be any time.Time within the target day; dayBounds
// derives the precise UTC window using the Summarizer's configured timezone.
func (s *Summarizer) SummarizeDay(ctx context.Context, date time.Time) error {
	from, to := dayBounds(date, s.loc)

	msgs, err := s.store.GetMessagesBetween(ctx, []string{"user", "assistant"}, from, to)
	if err != nil {
		return fmt.Errorf("summarizer: day messages: %w", err)
	}
	if len(msgs) == 0 {
		logger.Log.Info("summarizer: no messages for day, skipping",
			"date", from.In(s.loc).Format("2006-01-02"))
		return nil
	}

	sessions := groupIntoSessions(msgs)

	sessionContent, err := buildDailyInput(ctx, sessions, s)
	if err != nil {
		return fmt.Errorf("summarizer: day input: %w", err)
	}

	userPrompt := buildDailyPrompt(sessionContent, from, s.loc)
	raw, err := s.callLLM(ctx, dailySystemPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("summarizer: day llm: %w", err)
	}

	var resp dailySummaryResponse
	if err := parseJSON(raw, &resp); err != nil {
		return fmt.Errorf("summarizer: day parse: %w", err)
	}

	// Convert LLM topic entries to storage type.
	topics := make(storage.JSONSlice[storage.TopicEntry], len(resp.Topics))
	for i, t := range resp.Topics {
		topics[i] = storage.TopicEntry{
			Topic:        t.Topic,
			MessageShare: t.MessageShare,
			Sentiment:    t.Sentiment,
		}
	}

	dateLabel := from.In(s.loc).Format("2006-01-02") // for fact provenance

	if err := s.store.SaveDailySummary(ctx, &storage.DailySummary{
		ID:              uuid.NewString(),
		Date:            storage.DBTime{Time: from},
		Summary:         resp.Summary,
		KeyEvents:       resp.KeyEvents,
		Topics:          topics,
		ConversationIDs: uniqueConvIDs(msgs),
	}); err != nil {
		return fmt.Errorf("summarizer: save daily: %w", err)
	}

	if err := s.saveFacts(ctx, resp.Facts, firstConvID(msgs), dateLabel); err != nil {
		// Fact errors are non-fatal: the summary is already saved.
		logger.Log.Warn("summarizer: save facts", "err", err)
	}

	logger.Log.Info("summarizer: daily summary saved",
		"date", dateLabel,
		"sessions", len(sessions),
		"messages", len(msgs),
		"facts", len(resp.Facts),
	)
	return nil
}

// Session grouping

// groupIntoSessions splits a chronologically ordered message slice into
// sessions delimited by gaps of at least sessionGap.
func groupIntoSessions(msgs []*message.Message) []msgSession {
	if len(msgs) == 0 {
		return nil
	}

	var sessions []msgSession
	cur := msgSession{
		start:    msgs[0].Timestamp.Time,
		messages: []*message.Message{msgs[0]},
	}

	for _, m := range msgs[1:] {
		prev := cur.messages[len(cur.messages)-1]
		if m.Timestamp.Sub(prev.Timestamp.Time) >= sessionGap {
			cur.end = prev.Timestamp.Time
			cur.agents = sessionAgents(cur.messages)
			sessions = append(sessions, cur)
			cur = msgSession{
				start:    m.Timestamp.Time,
				messages: []*message.Message{m},
			}
		} else {
			cur.messages = append(cur.messages, m)
		}
	}
	cur.end = cur.messages[len(cur.messages)-1].Timestamp.Time
	cur.agents = sessionAgents(cur.messages)
	sessions = append(sessions, cur)
	return sessions
}

// sessionAgents returns deduplicated agent IDs from assistant-role messages.
func sessionAgents(msgs []*message.Message) []string {
	seen := make(map[string]bool)
	var agents []string
	for _, m := range msgs {
		if m.Role == "assistant" && m.AgentID != "" && !seen[m.AgentID] {
			seen[m.AgentID] = true
			agents = append(agents, m.AgentID)
		}
	}
	return agents
}

// Prompt input construction

// buildDailyInput renders sessions into a prompt-ready string.
// If the rendered content fits within tokenBudget it is returned as-is.
// If it overflows, each session is pre-summarised individually and those
// mini-summaries are combined — facts are only extracted in the final pass.
func buildDailyInput(ctx context.Context, sessions []msgSession, s *Summarizer) (string, error) {
	full := renderSessions(sessions)
	if estimateTokens(full) <= tokenBudget {
		return full, nil
	}
	return buildChunkedDailyInput(ctx, sessions, s)
}

// renderSessions formats sessions for injection into the daily prompt.
//
// Each session header carries:
//   - local time span  (e.g. "2:30PM – 4:00PM")
//   - message count and percentage of the day's total
//   - participating agent IDs
//
// The percentage is the volume-weighting signal the LLM acts on.
// Assistant messages are truncated to maxAssistantMsgLen and labelled with
// the agent ID rather than the generic "assistant" role.
func renderSessions(sessions []msgSession) string {
	var b strings.Builder

	totalMsgs := 0
	for _, sess := range sessions {
		totalMsgs += len(sess.messages)
	}

	for _, sess := range sessions {
		pct := 0
		if totalMsgs > 0 {
			pct = (len(sess.messages) * 100) / totalMsgs
		}
		agentStr := "executive"
		if len(sess.agents) > 0 {
			agentStr = strings.Join(sess.agents, ", ")
		}
		fmt.Fprintf(&b, "[%s – %s · %d messages · %d%% of day · agents: %s]\n",
			sess.start.Format("3:04PM"),
			sess.end.Format("3:04PM"),
			len(sess.messages),
			pct,
			agentStr,
		)
		for _, m := range sess.messages {
			label := m.Role
			if m.Role == "assistant" && m.AgentID != "" {
				label = m.AgentID
			}
			fmt.Fprintf(&b, "  %s: %s\n", label, truncateMsg(m.Role, m.Content))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// buildChunkedDailyInput pre-summarises each session with a cheap plain-text
// LLM call, then concatenates those mini-summaries for the final JSON pass.
// Called only when the full day's content exceeds tokenBudget.
func buildChunkedDailyInput(ctx context.Context, sessions []msgSession, s *Summarizer) (string, error) {
	const chunkSystem = `You are a memory compression assistant for OnlyAgents.
Summarise this conversation session into 2-3 sentences covering the main topics, decisions, and tone.
"user" is the human. Named agents (executive, productivity_agent, etc.) are AI.
Respond with plain prose only — no JSON, no bullet points.`

	var b strings.Builder
	totalMsgs := 0
	for _, sess := range sessions {
		totalMsgs += len(sess.messages)
	}

	for _, sess := range sessions {
		rendered := renderSessions([]msgSession{sess})
		mini, err := s.callLLM(ctx, chunkSystem, "Session:\n"+rendered)
		if err != nil {
			return "", fmt.Errorf("chunk summarise session %s: %w",
				sess.start.Format("3:04PM"), err)
		}

		pct := 0
		if totalMsgs > 0 {
			pct = (len(sess.messages) * 100) / totalMsgs
		}
		fmt.Fprintf(&b, "[%s – %s · %d messages · %d%% of day]\n%s\n\n",
			sess.start.Format("3:04PM"),
			sess.end.Format("3:04PM"),
			len(sess.messages),
			pct,
			strings.TrimSpace(mini),
		)
	}
	return b.String(), nil
}

// System prompt
const dailySystemPrompt = `You are the memory system for OnlyAgents, a personal AI agent runtime.

SYSTEM ARCHITECTURE:
OnlyAgents runs a multi-agent system. One executive agent handles all user-facing
conversation — it understands intent, decomposes tasks, and delegates to specialised
sub-agents (e.g. productivity_agent, researcher_agent). Agents communicate via an
internal event bus called the kernel. In the message history below, "user" is always
the human. All other labels (executive, productivity_agent, etc.) are AI agents.

YOUR TASK:
Compress today's conversation sessions into a structured daily summary and extract
durable facts about the user and entities they mentioned.

TRUNCATION NOTE:
Agent (assistant) messages are truncated to ~300 characters. This is intentional for
efficiency. Do NOT treat truncated messages as incomplete answers. Do NOT speculate
about or attempt to complete truncated content.

WEIGHTING RULE:
Each session header shows its share of the day's total messages (e.g. "62% of day").
Weight your summary, key events, topics, and facts proportionally to session volume.
A session with 60% of the day's messages should dominate the summary narrative and
produce more facts than a 10% session.

FACT EXTRACTION RULES:

- Do NOT extract facts that describe task configuration — delivery times, output
  formats for a specific request, preferred sources for a single workflow. These
  belong in the workflow system, not the facts database.
- Do NOT extract organizations, tools, or places merely because they were mentioned.
  Only extract them if they reveal something durable about the user's preferences,
  relationships, or context (e.g. "works at Acme Corp" is a fact, "mentioned Acme
  Corp once" is not).
- A fact must answer: "would this be useful context in a future unrelated
  conversation?" If no, skip it.
- Prefer one strong fact over three weak ones. Fewer, higher-confidence facts
  are better than exhaustive extraction.

OUTPUT: Respond ONLY with valid JSON. No markdown fences, no explanation, no preamble.`

// User prompt

const dailyJSONSchema = `{
  "summary": "A brief, around 5-7 sentence, narrative of the day, weighted by session volume",
  "key_events": ["notable event 1", "notable event 2"],
  "topics": [
    {"topic": "cricket", "message_share": 0.62, "sentiment": "enthusiastic"},
    {"topic": "geopolitics", "message_share": 0.18, "sentiment": "neutral"}
  ],
  "facts": [
    {
      "entity": "entity name",
      "entity_type": "person|place|preference|other",
      "fact": "specific durable fact",
      "confidence": 0.9
    }
  ]
}`

func buildDailyPrompt(sessionContent string, date time.Time, loc *time.Location) string {
	var b strings.Builder
	fmt.Fprintf(&b, "DATE: %s\n\n", date.In(loc).Format("Monday, January 2 2006"))
	b.WriteString("Summarise the following conversation sessions.\n")
	b.WriteString("Weight the summary proportionally to each session's message share.\n\n")
	b.WriteString("Required JSON schema:\n")
	b.WriteString(dailyJSONSchema)
	b.WriteString("\n\nSessions:\n")
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
