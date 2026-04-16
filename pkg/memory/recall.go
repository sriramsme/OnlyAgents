package memory

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// Called by Manager.GetRelevantMemory.
// query is the user's current message or a natural language description
// of what context is needed.
func (re *Engine) Recall(ctx context.Context, query string) (*Context, error) {
	mc := &Context{}

	// Embed once, share across all sources.
	var queryVec []float32
	if re.embedder != nil && query != "" {
		if vec, err := re.embedder.Embed(ctx, query); err == nil {
			queryVec = vec
		}
	}

	// Run all sources, collect results.
	// Each source is independent — one failure doesn't block others.
	var allResults []Result
	for _, src := range re.sources {
		results, err := src.Search(ctx, query, queryVec, re.cfg.MaxPerSource)
		if err != nil {
			logger.Log.Warn("recall: source search failed",
				"source", src.Name(), "err", err)
			continue
		}
		allResults = append(allResults, results...)
	}

	// Re-rank by score, apply total budget.
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})
	if len(allResults) > re.cfg.MaxTotal {
		allResults = allResults[:re.cfg.MaxTotal]
	}

	mc.Results = allResults
	return mc, nil
}

// RecallRecent returns recent episodes for the given scope (session, daily, weekly, monthly, yearly).
// from is the time window for recent episodes.
// Returns recent episodes for the given scope between now and from.
func (re *Engine) RecallRecent(ctx context.Context, scope EpisodeScope, from time.Duration) (*Context, error) {
	var results []Result

	// Recent sessions for conversational continuity.
	now := time.Now()
	recentFrom := now.Add(-from)
	recent, err := re.store.GetEpisodesByScope(ctx, scope, recentFrom, now)
	if err != nil {
		return nil, fmt.Errorf("wake-up: recent sessions: %w", err)
	}

	if len(recent) > 0 {
		for _, ep := range recent {
			results = append(results, Result{
				Content:    fmt.Sprintf("[%s]: %s", ep.StartedAt.Format("Jan 2 3:04PM"), ep.Summary),
				Score:      ep.Importance,
				SourceName: fmt.Sprintf(string(scope), "_memory"),
				Metadata:   map[string]any{"episode_id": ep.ID},
			})
		}
	}

	return &Context{Results: results}, nil
}
