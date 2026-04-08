// Package summarizer runs LLM-based compression passes over conversation
// messages and lower-level summaries.
//
// Summary tables (daily/weekly/monthly/yearly) are system-wide — they are not
// scoped to individual agents. agentID on the Summarizer is used only for
// fact attribution (which agent extracted the fact).
//
// Cron schedule (all times are local to the configured timezone):
//
//	Daily   — 23:59 every day
//	Weekly  — 23:59 every Sunday
//	Monthly — 23:59 on the last day of each month
//	Yearly  — 23:59 on Dec 31
//
// On startup, catch-up logic in the Manager runs any missed jobs.
package summarizer

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sriramsme/OnlyAgents/pkg/dbtypes"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/message"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
)

// Summarizer is the single entry-point for all memory compression passes.
type Summarizer struct {
	store     SummarizerStore
	llmClient llm.Client
	loc       *time.Location
}

type SummarizerStore interface {
	storage.MemoryStore
	message.Store
	storage.FactStore
}

// New creates a Summarizer. tz is an IANA timezone string (e.g. "America/New_York").
// If tz is empty or invalid, UTC is used.
func New(store SummarizerStore, llmClient llm.Client, tz string) *Summarizer {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return &Summarizer{
		store:     store,
		llmClient: llmClient,
		loc:       loc,
	}
}

func (s *Summarizer) Loc() *time.Location {
	return s.loc
}

// callLLM sends a single-turn chat to the LLM with an explicit system prompt.
// system sets the role/behaviour; user contains the data to process.
func (s *Summarizer) callLLM(ctx context.Context, system, user string) (string, error) {
	resp, err := s.llmClient.Chat(ctx, &llm.Request{
		Messages: []llm.Message{
			llm.SystemMessage(system),
			llm.UserMessage(user),
		},
		Metadata: map[string]string{"agent_id": "summarizer"},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// saveFacts upserts extracted facts.
//
// TODO: replace with semantic similarity once embeddings are available.
// Per-fact logic:
//   - Same (agentID, entity, fact) seen again → reinforce: bump times_seen, nudge
//     confidence toward 1.0, update last_confirmed.
//   - New fact, but a conflicting active fact exists for the same entity with the
//     same leading verb phrase (e.g. two "prefers …" facts) → mark the old one
//     superseded and lower its confidence.
//   - Otherwise → insert new.
//
// entity_type is normalised to the canonical set before storage.
// confidence is clamped to [0.0, 1.0].
// sourceConvID and sourceSummaryDate are best-effort provenance.
func (s *Summarizer) saveFacts(ctx context.Context, facts []extractedFact, sourceConvID, sourceSummaryDate string) error {
	now := dbtypes.DBTime{Time: time.Now()}
	for _, f := range facts {
		f.EntityType = normalizeEntityType(f.EntityType)
		f.Confidence = clampConfidence(f.Confidence)
		if err := s.store.InsertFact(ctx, &storage.Fact{
			ID:                   uuid.NewString(),
			Entity:               f.Entity,
			EntityType:           f.EntityType,
			Fact:                 f.Fact,
			Confidence:           f.Confidence,
			TimesSeen:            1,
			SourceConversationID: sourceConvID,
			SourceSummaryDate:    sourceSummaryDate,
			SupersededBy:         "",
			FirstSeen:            now,
			LastConfirmed:        now,
		}); err != nil {
			logger.Log.Warn("summarizer: insert fact", "entity", f.Entity, "err", err)
		}
	}
	return nil
}

// extractedFact is the JSON shape returned by the LLM for individual facts.
type extractedFact struct {
	Entity     string  `json:"entity"`
	EntityType string  `json:"entity_type"`
	Fact       string  `json:"fact"`
	Confidence float64 `json:"confidence"`
}

// Response types for each summarisation tier.

type dailySummaryResponse struct {
	Summary   string          `json:"summary"`
	KeyEvents []string        `json:"key_events"`
	Topics    []topicEntry    `json:"topics"`
	Facts     []extractedFact `json:"facts"`
}

// topicEntry mirrors storage.TopicEntry and is used for JSON decode.
type topicEntry struct {
	Topic        string  `json:"topic"`
	MessageShare float64 `json:"message_share"`
	Sentiment    string  `json:"sentiment"`
}

type weeklySummaryResponse struct {
	Summary      string   `json:"summary"`
	Themes       []string `json:"themes"`
	Achievements []string `json:"achievements"`
}

type monthlySummaryResponse struct {
	Summary    string          `json:"summary"`
	Highlights []string        `json:"highlights"`
	Statistics dbtypes.JSONMap `json:"statistics"`
}

type yearlySummaryResponse struct {
	Summary     string          `json:"summary"`
	MajorEvents []string        `json:"major_events"`
	Statistics  dbtypes.JSONMap `json:"statistics"`
}
