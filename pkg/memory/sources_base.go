package memory

import (
	"context"
	"fmt"
)

type episodeSource struct{ store EpisodeStore }

func (s *episodeSource) Name() string { return "episodes" }
func (s *episodeSource) Search(ctx context.Context, query string, queryVec []float32, limit int) ([]Result, error) {
	episodes, err := s.store.SearchEpisodes(ctx, EpisodeQuery{
		Embedding: queryVec,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	results := make([]Result, len(episodes))
	for i, ep := range episodes {
		results[i] = Result{
			Content:    fmt.Sprintf("[%s] %s", ep.StartedAt.Format("Jan 2 3:04PM"), ep.Summary),
			Score:      ep.Importance,
			SourceName: fmt.Sprintf("%s (%s)", s.Name(), ep.Scope),
			Metadata:   map[string]any{"episode_id": ep.ID},
		}
	}
	return results, nil
}

type nexusSource struct{ store NexusStore }

func (s *nexusSource) Name() string { return "nexus" }
func (s *nexusSource) Search(ctx context.Context, query string, queryVec []float32, limit int) ([]Result, error) {
	// FTS on entity_aliases + canonical names first.
	entities, err := s.store.FindSimilarEntities(ctx, query)
	if err != nil || len(entities) == 0 {
		return nil, err
	}
	var results []Result
	for _, entity := range entities {
		if len(results) >= limit {
			break
		}
		relations, err := s.store.QueryEntity(ctx, entity.ID, nil)
		if err != nil {
			continue
		}
		for _, rel := range relations {
			object := ""
			if rel.ObjectLiteral != nil {
				object = *rel.ObjectLiteral
			} else {
				object = rel.ObjectName
			}
			results = append(results, Result{
				Content:    fmt.Sprintf("%s %s %s", rel.SubjectName, rel.Predicate, object),
				Score:      rel.Confidence,
				SourceName: s.Name(),
				Metadata:   map[string]any{"entity_id": entity.ID},
			})
		}
	}
	return results, nil
}

type praxisSource struct{ store PraxisStore }

func (s *praxisSource) Name() string { return "praxis" }
func (s *praxisSource) Search(ctx context.Context, query string, queryVec []float32, limit int) ([]Result, error) {
	patterns, err := s.store.SearchPatterns(ctx, queryVec, limit)
	if err != nil {
		return nil, err
	}
	results := make([]Result, len(patterns))
	for i, p := range patterns {
		results[i] = Result{
			Content:    p.Description,
			Score:      p.Confidence,
			SourceName: s.Name(),
		}
	}
	return results, nil
}
