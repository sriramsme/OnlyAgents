package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// sessionAgentID is the sentinel used when saving session-scoped summaries.
// Matches the key used by ConversationManager.
const sessionAgentID = "__session__"

// Summarizer runs LLM-based compression passes over raw messages and
// lower-level summaries. Fact extraction happens in the same LLM call as
// daily summarisation to keep token usage down.
type Summarizer struct {
	store     storage.Storage
	llmClient llm.Client
}

func newSummarizer(store storage.Storage, llmClient llm.Client) *Summarizer {
	return &Summarizer{store: store, llmClient: llmClient}
}

// Daily

// SummarizeDay summarises all messages for the given calendar day.
// Facts extracted by the LLM are upserted into the facts table.
// If there are no messages for the day, it is a no-op.
func (s *Summarizer) SummarizeDay(ctx context.Context, date time.Time) error {
	day := truncateToDay(date)
	msgs, err := s.store.GetRecentMessages(ctx, sessionAgentID, day)
	if err != nil {
		return fmt.Errorf("summarizer: day messages: %w", err)
	}
	// Filter to messages within the target day only.
	nextDay := day.Add(24 * time.Hour)
	var dayMsgs []*storage.Message
	for _, m := range msgs {
		if !m.Timestamp.Before(day) && m.Timestamp.Before(nextDay) {
			dayMsgs = append(dayMsgs, m)
		}
	}
	if len(dayMsgs) == 0 {
		logger.Log.Info("summarizer: no messages for day, skipping", "date", day.Format("2006-01-02"))
		return nil
	}

	prompt := buildDailyPrompt(dayMsgs)
	raw, err := s.callLLM(ctx, prompt)
	if err != nil {
		return fmt.Errorf("summarizer: day llm: %w", err)
	}

	var resp dailySummaryResponse
	if err := parseJSON(raw, &resp); err != nil {
		return fmt.Errorf("summarizer: day parse: %w", err)
	}

	// Collect unique conversation IDs from messages.
	convIDs := uniqueConvIDs(dayMsgs)

	if err := s.store.SaveDailySummary(ctx, &storage.DailySummary{
		ID:              uuid.NewString(),
		AgentID:         sessionAgentID,
		Date:            storage.DBTime{Time: day},
		Summary:         resp.Summary,
		KeyEvents:       resp.KeyEvents,
		Topics:          resp.Topics,
		ConversationIDs: convIDs,
	}); err != nil {
		return fmt.Errorf("summarizer: save daily: %w", err)
	}

	// Upsert extracted facts.
	if err := s.saveFacts(ctx, resp.Facts); err != nil {
		// Non-fatal: summary is saved, facts are best-effort.
		logger.Log.Warn("summarizer: save facts", "err", err)
	}

	logger.Log.Info("summarizer: daily summary saved",
		"date", day.Format("2006-01-02"),
		"messages", len(dayMsgs),
		"facts", len(resp.Facts))
	return nil
}

//  Weekly

// SummarizeWeek summarises the 7 daily summaries ending on weekEnd (Sunday).
func (s *Summarizer) SummarizeWeek(ctx context.Context, weekEnd time.Time) error {
	weekStart := weekEnd.AddDate(0, 0, -6)
	dailies, err := s.store.GetDailySummaries(ctx, sessionAgentID, weekStart, weekEnd)
	if err != nil {
		return fmt.Errorf("summarizer: week dailies: %w", err)
	}
	if len(dailies) == 0 {
		logger.Log.Info("summarizer: no daily summaries for week, skipping",
			"week_start", weekStart.Format("2006-01-02"))
		return nil
	}

	prompt := buildWeeklyPrompt(dailies)
	raw, err := s.callLLM(ctx, prompt)
	if err != nil {
		return fmt.Errorf("summarizer: week llm: %w", err)
	}

	var resp weeklySummaryResponse
	if err := parseJSON(raw, &resp); err != nil {
		return fmt.Errorf("summarizer: week parse: %w", err)
	}

	return wrap(s.store.SaveWeeklySummary(ctx, &storage.WeeklySummary{
		ID:           uuid.NewString(),
		AgentID:      sessionAgentID,
		WeekStart:    storage.DBTime{Time: weekStart},
		WeekEnd:      storage.DBTime{Time: weekEnd},
		Summary:      resp.Summary,
		Themes:       resp.Themes,
		Achievements: resp.Achievements,
	}), "save weekly summary")
}

// Monthly

// SummarizeMonth summarises all weekly summaries for the given year/month.
func (s *Summarizer) SummarizeMonth(ctx context.Context, year, month int) error {
	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1)
	weeklies, err := s.store.GetWeeklySummaries(ctx, sessionAgentID, from, to)
	if err != nil {
		return fmt.Errorf("summarizer: month weeklies: %w", err)
	}
	if len(weeklies) == 0 {
		logger.Log.Info("summarizer: no weekly summaries for month, skipping",
			"year", year, "month", month)
		return nil
	}

	prompt := buildMonthlyPrompt(weeklies)
	raw, err := s.callLLM(ctx, prompt)
	if err != nil {
		return fmt.Errorf("summarizer: month llm: %w", err)
	}

	var resp monthlySummaryResponse
	if err := parseJSON(raw, &resp); err != nil {
		return fmt.Errorf("summarizer: month parse: %w", err)
	}

	return wrap(s.store.SaveMonthlySummary(ctx, &storage.MonthlySummary{
		ID:         uuid.NewString(),
		AgentID:    sessionAgentID,
		Year:       year,
		Month:      month,
		Summary:    resp.Summary,
		Highlights: resp.Highlights,
		Statistics: resp.Statistics,
	}), "save monthly summary")
}

// Yearly

// SummarizeYear summarises all monthly summaries for the given year.
func (s *Summarizer) SummarizeYear(ctx context.Context, year int) error {
	monthlies, err := s.store.GetMonthlySummaries(ctx, sessionAgentID, year)
	if err != nil {
		return fmt.Errorf("summarizer: year monthlies: %w", err)
	}
	if len(monthlies) == 0 {
		logger.Log.Info("summarizer: no monthly summaries for year, skipping", "year", year)
		return nil
	}

	prompt := buildYearlyPrompt(monthlies)
	raw, err := s.callLLM(ctx, prompt)
	if err != nil {
		return fmt.Errorf("summarizer: year llm: %w", err)
	}

	var resp yearlySummaryResponse
	if err := parseJSON(raw, &resp); err != nil {
		return fmt.Errorf("summarizer: year parse: %w", err)
	}

	return wrap(s.store.SaveYearlyArchive(ctx, &storage.YearlyArchive{
		ID:          uuid.NewString(),
		AgentID:     sessionAgentID,
		Year:        year,
		Summary:     resp.Summary,
		MajorEvents: resp.MajorEvents,
		Statistics:  resp.Statistics,
	}), "save yearly archive")
}

// Fact persistence

func (s *Summarizer) saveFacts(ctx context.Context, facts []extractedFact) error {
	now := storage.DBTime{Time: time.Now()}
	for _, f := range facts {
		if err := s.store.UpsertFact(ctx, &storage.Fact{
			ID:            uuid.NewString(),
			AgentID:       sessionAgentID,
			Entity:        f.Entity,
			EntityType:    f.EntityType,
			Fact:          f.Fact,
			Confidence:    f.Confidence,
			FirstSeen:     now,
			LastConfirmed: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

// LLM call

func (s *Summarizer) callLLM(ctx context.Context, prompt string) (string, error) {
	resp, err := s.llmClient.Chat(ctx, &llm.Request{
		Messages: []llm.Message{
			llm.UserMessage(prompt),
		},
		Metadata: map[string]string{"agent_id": "summarizer"},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Prompts

func buildDailyPrompt(msgs []*storage.Message) string {
	var b strings.Builder
	b.WriteString(`You are a memory compression assistant for a personal AI agent.
Summarise the following conversation messages from today into a concise daily summary.
Extract: key events, main topics discussed, and any facts learned about the user or entities they mentioned.

Respond ONLY with valid JSON, no markdown, no explanation. Use this exact schema:
{
  "summary": "2-4 sentence narrative of the day",
  "key_events": ["event 1", "event 2"],
  "topics": ["topic 1", "topic 2"],
  "facts": [
    {"entity": "entity name", "entity_type": "person|place|preference|project|other", "fact": "fact about entity", "confidence": 0.9}
  ]
}

Messages:
`)
	for _, m := range msgs {
		fmt.Fprintf(&b, "[%s] %s: %s\n", m.Timestamp.Format("15:04"), m.Role, m.Content)
	}
	return b.String()
}

func buildWeeklyPrompt(dailies []*storage.DailySummary) string {
	var b strings.Builder
	b.WriteString(`You are a memory compression assistant.
Summarise the following daily summaries into a weekly overview.

Respond ONLY with valid JSON, no markdown, no explanation. Use this exact schema:
{
  "summary": "3-5 sentence narrative of the week",
  "themes": ["recurring theme 1", "theme 2"],
  "achievements": ["thing completed or accomplished 1", "achievement 2"]
}

Daily summaries:
`)
	for _, d := range dailies {
		fmt.Fprintf(&b, "[%s] %s\n", d.Date.Format("2006-01-02"), d.Summary)
	}
	return b.String()
}

func buildMonthlyPrompt(weeklies []*storage.WeeklySummary) string {
	var b strings.Builder
	b.WriteString(`You are a memory compression assistant.
Summarise the following weekly summaries into a monthly overview.

Respond ONLY with valid JSON, no markdown, no explanation. Use this exact schema:
{
  "summary": "3-5 sentence narrative of the month",
  "highlights": ["highlight 1", "highlight 2"],
  "statistics": {"weeks_active": 4, "dominant_theme": "work"}
}

Weekly summaries:
`)
	for _, w := range weeklies {
		fmt.Fprintf(&b, "[%s – %s] %s\n",
			w.WeekStart.Format("Jan 2"),
			w.WeekEnd.Format("Jan 2"),
			w.Summary)
	}
	return b.String()
}

func buildYearlyPrompt(monthlies []*storage.MonthlySummary) string {
	var b strings.Builder
	b.WriteString(`You are a memory compression assistant.
Summarise the following monthly summaries into a yearly archive.

Respond ONLY with valid JSON, no markdown, no explanation. Use this exact schema:
{
  "summary": "5-7 sentence narrative of the year",
  "major_events": ["major event 1", "major event 2"],
  "statistics": {"months_active": 12, "dominant_theme": "growth"}
}

Monthly summaries:
`)
	for _, m := range monthlies {
		fmt.Fprintf(&b, "[%d-%02d] %s\n", m.Year, m.Month, m.Summary)
	}
	return b.String()
}

// Response type

type extractedFact struct {
	Entity     string  `json:"entity"`
	EntityType string  `json:"entity_type"`
	Fact       string  `json:"fact"`
	Confidence float64 `json:"confidence"`
}

type dailySummaryResponse struct {
	Summary   string          `json:"summary"`
	KeyEvents []string        `json:"key_events"`
	Topics    []string        `json:"topics"`
	Facts     []extractedFact `json:"facts"`
}

type weeklySummaryResponse struct {
	Summary      string   `json:"summary"`
	Themes       []string `json:"themes"`
	Achievements []string `json:"achievements"`
}

type monthlySummaryResponse struct {
	Summary    string          `json:"summary"`
	Highlights []string        `json:"highlights"`
	Statistics storage.JSONMap `json:"statistics"`
}

type yearlySummaryResponse struct {
	Summary     string          `json:"summary"`
	MajorEvents []string        `json:"major_events"`
	Statistics  storage.JSONMap `json:"statistics"`
}

// parseJSON strips markdown fences the LLM sometimes adds before unmarshalling.
func parseJSON(raw string, v any) error {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if i := strings.LastIndex(s, "}"); i >= 0 && i < len(s)-1 {
		s = s[:i+1]
	}
	return json.Unmarshal([]byte(s), v)
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func uniqueConvIDs(msgs []*storage.Message) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, m := range msgs {
		if !seen[m.ConversationID] {
			seen[m.ConversationID] = true
			ids = append(ids, m.ConversationID)
		}
	}
	return ids
}

// wrap is re-declared here to avoid importing sqlite internals.
// It matches the pattern from pkg/storage/sqlite/sqlite.go.
func wrap(err error, op string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("memory: %s: %w", op, err)
}
