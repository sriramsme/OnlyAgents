package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/embedder"
)

// RecallStore is the storage interface RecallEngine needs.
// Subset of the full Storage interface — only what retrieval requires.
type RecallStore interface {
	EpisodeStore
	NexusStore
	PraxisStore
}

// MemoryContext is the structured output of a recall query.
// Rendered into the agent's system prompt before each LLM call.
type MemoryContext struct {
	WakeUp   string      // identity snapshot — always included
	Episodes []*Episode  // relevant session summaries
	Facts    []*Relation // current Nexus facts for relevant entities
	Patterns []*Pattern  // applicable behavioral patterns
}

// Render formats MemoryContext into a prompt-injectable string.
// Returns empty string if there is nothing meaningful to inject.
func (mc *MemoryContext) Render() string {
	if mc == nil {
		return ""
	}
	var b strings.Builder

	if mc.WakeUp != "" {
		b.WriteString("## Context\n")
		b.WriteString(mc.WakeUp)
		b.WriteString("\n\n")
	}

	if len(mc.Facts) > 0 {
		b.WriteString("## Current Facts\n")
		for _, f := range mc.Facts {
			if f.ObjectLiteral != nil {
				fmt.Fprintf(&b, "- %s %s %s\n", f.SubjectID, f.Predicate, *f.ObjectLiteral)
			} else if f.ObjectID != nil {
				fmt.Fprintf(&b, "- %s %s %s\n", f.SubjectID, f.Predicate, *f.ObjectID)
			}
		}
		b.WriteString("\n")
	}

	if len(mc.Episodes) > 0 {
		b.WriteString("## Relevant History\n")
		for _, ep := range mc.Episodes {
			fmt.Fprintf(&b, "[%s] %s\n\n",
				ep.StartedAt.Format("Jan 2 3:04PM"),
				ep.Summary,
			)
		}
	}

	if len(mc.Patterns) > 0 {
		b.WriteString("## Behavioral Patterns\n")
		for _, p := range mc.Patterns {
			fmt.Fprintf(&b, "- %s\n", p.Description)
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

// recallConfig controls retrieval budgets. Sensible defaults, all overridable.
type recallConfig struct {
	MaxEpisodes  int           // max session episodes to return
	MaxFacts     int           // max Nexus facts to return
	MaxPatterns  int           // max Praxis patterns to return
	RecentWindow time.Duration // how far back "recent" episodes go for wake-up
}

func defaultRecallConfig() recallConfig {
	return recallConfig{
		MaxEpisodes:  5,
		MaxFacts:     20,
		MaxPatterns:  10,
		RecentWindow: 48 * time.Hour,
	}
}

// RecallEngine assembles MemoryContext for a given query.
type RecallEngine struct {
	store    RecallStore
	embedder embedder.Embedder // nil-safe — falls back to FTS/filter
	cfg      recallConfig
}

func newRecallEngine(store RecallStore, emb embedder.Embedder) *RecallEngine {
	return &RecallEngine{
		store:    store,
		embedder: emb,
		cfg:      defaultRecallConfig(),
	}
}

// Recall is the main entry point. Called by Manager.GetRelevantMemory.
// query is the user's current message or a natural language description
// of what context is needed.
func (re *RecallEngine) Recall(ctx context.Context, query string) (*MemoryContext, error) {
	mc := &MemoryContext{}

	// Step 1: Wake-up snapshot — always included regardless of query.
	wakeUp, err := re.buildWakeUp(ctx)
	if err != nil {
		// Non-fatal: log and continue without wake-up.
		// Engine should never fail to return a MemoryContext.
	} else {
		mc.WakeUp = wakeUp
	}

	// Step 2: Embed the query for semantic episode search.
	var queryVec []float32
	if re.embedder != nil && query != "" {
		queryVec, err = re.embedder.Embed(ctx, query)
		if err != nil {
			// Non-fatal: fall through to filter-based search.
			queryVec = nil
		}
	}

	// Step 3: Retrieve relevant episodes.
	episodes, err := re.store.SearchEpisodes(ctx, EpisodeQuery{
		Scope:     scopePtr(ScopeSession),
		Embedding: queryVec,
		Limit:     re.cfg.MaxEpisodes,
	})
	if err != nil {
		return mc, fmt.Errorf("recall: search episodes: %w", err)
	}
	mc.Episodes = episodes

	// Step 4: Pull entity IDs from retrieved episodes,
	// then fetch current Nexus facts for those entities.
	if len(episodes) > 0 {
		facts, err := re.factsForEpisodes(ctx, episodes)
		if err != nil {
			// Non-fatal: return what we have.
		} else {
			mc.Facts = facts
		}
	}

	// Step 5: Retrieve applicable behavioral patterns.
	patterns, err := re.store.SearchPatterns(ctx, queryVec, re.cfg.MaxPatterns)
	if err != nil {
		// Non-fatal.
	} else {
		mc.Patterns = patterns
	}

	return mc, nil
}

// buildWakeUp assembles the lightweight identity snapshot loaded every session.
// Reads recent episodes + high-confidence facts — no query embedding needed.
func (re *RecallEngine) buildWakeUp(ctx context.Context) (string, error) {
	var b strings.Builder

	// Recent sessions for conversational continuity.
	now := time.Now()
	recentFrom := now.Add(-re.cfg.RecentWindow)
	recent, err := re.store.GetEpisodesByScope(ctx, ScopeSession, recentFrom, now)
	if err != nil {
		return "", fmt.Errorf("wake-up: recent sessions: %w", err)
	}

	if len(recent) > 0 {
		b.WriteString("Recent activity:\n")
		// Show last 2 sessions max in wake-up — keep it tight.
		start := len(recent) - 2
		if start < 0 {
			start = 0
		}
		for _, ep := range recent[start:] {
			fmt.Fprintf(&b, "- [%s] %s\n",
				ep.StartedAt.Format("Jan 2 3:04PM"),
				ep.Summary,
			)
		}
	}

	return strings.TrimSpace(b.String()), nil
}

// factsForEpisodes collects entity IDs from the episode_entities join table
// for each retrieved episode, then fetches current Nexus facts for those entities.
func (re *RecallEngine) factsForEpisodes(ctx context.Context, episodes []*Episode) ([]*Relation, error) {
	// Collect unique entity IDs across all retrieved episodes.
	seen := make(map[string]struct{})
	var entityIDs []string
	for _, ep := range episodes {
		ids, err := re.store.GetEpisodeEntityIDs(ctx, ep.ID)
		if err != nil {
			continue // best-effort
		}
		for _, id := range ids {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				entityIDs = append(entityIDs, id)
			}
		}
	}

	if len(entityIDs) == 0 {
		return nil, nil
	}

	// Fetch current facts for each entity, up to MaxFacts total.
	var facts []*Relation
	for _, eid := range entityIDs {
		remaining := re.cfg.MaxFacts - len(facts)
		if remaining <= 0 {
			break
		}

		relations, err := re.store.QueryEntity(ctx, eid, nil)
		if err != nil {
			continue
		}

		// Take only up to remaining slots, relations already ordered
		// by valid_from DESC from QueryEntity
		if len(relations) > remaining {
			relations = relations[:remaining]
		}
		facts = append(facts, relations...)
	}

	return facts, nil
}

// scopePtr is a helper since EpisodeQuery.Scope is a pointer.
func scopePtr(s EpisodeScope) *EpisodeScope {
	return &s
}
