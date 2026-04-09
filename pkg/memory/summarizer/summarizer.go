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
package summarizer

import (
	"context"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
	"github.com/sriramsme/OnlyAgents/pkg/message"
)

// Type aliases so sibling files don't need to import memory directly.
type (
	Episode      = memory.Episode
	EpisodeScope = memory.EpisodeScope
)

const (
	ScopeSession = memory.ScopeSession
	ScopeDaily   = memory.ScopeDaily
	ScopeWeekly  = memory.ScopeWeekly
	ScopeMonthly = memory.ScopeMonthly
	ScopeYearly  = memory.ScopeYearly
)

// SummarizerStore is the combined store interface required by the Summarizer.
// Implementors must satisfy both EpisodeStore (for reading/writing episodes)
// and message.Store (for reading raw conversation messages).
type SummarizerStore interface {
	memory.EpisodeStore
	memory.PraxisStore
	memory.NexusStore
	message.Store
}

// Summarizer is the single entry-point for all memory compression passes.
type Summarizer struct {
	store     SummarizerStore
	llmClient llm.Client
	loc       *time.Location
	embedder  memory.Embedder
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

// lastEpisodeBefore fetches up to n episodes of the given scope whose window
// ends before cutoff. Used to inject prior-period context into prompts.
// Results are ordered newest-first. Returns nil (no error) when none exist.
func (s *Summarizer) lastEpisodeBefore(ctx context.Context, scope EpisodeScope, cutoff time.Time, n int) ([]*Episode, error) {
	q := memory.EpisodeQuery{
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
