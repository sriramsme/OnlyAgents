// Package summarizer runs LLM-based compression passes over conversation
// messages and episode summaries.
//
// Compression hierarchy (each level feeds the next):
//
//	SummarizeSessions — raw messages → ScopeSession episodes (fundamental unit)
//	SummarizeDay      — ScopeSession episodes → ScopeDaily episode
//	SummarizeWeek     — ScopeDaily episodes   → ScopeWeekly episode
//	SummarizeMonth    — ScopeWeekly episodes  → ScopeMonthly episode
//	SummarizeYear     — ScopeMonthly episodes → ScopeYearly episode
//
// Call order within a period matters: each level must complete before the next
// is invoked. All episodes are stored via EpisodeStore with a scope tag.
//
// Cron schedule (all times are local to the configured timezone):
//
//	Daily   — 23:59 every day
//	Weekly  — 23:59 every Sunday
//	Monthly — 23:59 on the last day of each month
//	Yearly  — 23:59 on Dec 31
//
// On startup, catch-up logic in the Manager runs any missed jobs.
package memory

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/embedder"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/message"
)

// SummarizerStore is the combined store interface required by the Summarizer.
// Implementors must satisfy both EpisodeStore (for reading/writing episodes)
// and message.Store (for reading raw conversation messages).
type SummarizerStore interface {
	EpisodeStore
	PraxisStore
	NexusStore
	message.Store
}

// Summarizer is the single entry-point for all memory compression passes.
type Summarizer struct {
	store     SummarizerStore
	llmClient llm.Client
	loc       *time.Location
	embedder  embedder.Embedder
}

// New creates a Summarizer. tz is an IANA timezone string (e.g. "America/New_York").
// If tz is empty or invalid, UTC is used.
func NewSummarizer(store SummarizerStore, llmClient llm.Client, embdeder embedder.Embedder, tz string) *Summarizer {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return &Summarizer{
		store:     store,
		llmClient: llmClient,
		loc:       loc,
		embedder:  embdeder,
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

// lastEpisodeBefore fetches up to n episodes of the given scope whose window
// ends before cutoff. Used to inject prior-period context into prompts.
// Results are ordered newest-first. Returns nil (no error) when none exist.
func (s *Summarizer) lastEpisodeBefore(ctx context.Context, scope EpisodeScope, cutoff time.Time, n int) ([]*Episode, error) {
	q := EpisodeQuery{
		Scope: &scope,
		To:    &cutoff,
		Limit: n,
	}
	eps, err := s.store.SearchEpisodes(ctx, q)
	if err != nil {
		return nil, err
	}
	return eps, nil
}
