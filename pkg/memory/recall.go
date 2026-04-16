package memory

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

type sourceResult struct {
	results []Result
	err     error
	source  string
}

func (re *Engine) Recall(ctx context.Context, query string) (*Context, error) {
	var queryVec []float32
	if re.embedder != nil && query != "" {
		if vec, err := re.embedder.Embed(ctx, query); err == nil {
			queryVec = vec
		}
	}

	ch := make(chan sourceResult, len(re.sources))
	for _, src := range re.sources {
		go func() {
			results, err := src.Search(ctx, query, queryVec, re.cfg.MaxPerSource)
			ch <- sourceResult{results: results, err: err, source: src.Name()}
		}()
	}

	var allResults []Result
	for range re.sources {
		sr := <-ch
		if sr.err != nil {
			logger.Log.Warn("recall: source search failed", "source", sr.source, "err", sr.err)
			continue
		}
		allResults = append(allResults, sr.results...)
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})
	if len(allResults) > re.cfg.MaxTotal {
		allResults = allResults[:re.cfg.MaxTotal]
	}
	return &Context{Results: allResults}, nil
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
