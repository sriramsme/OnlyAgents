package notes

import (
	"context"
	"fmt"
	"strings"

	"github.com/sriramsme/OnlyAgents/pkg/memory"
)

type Source struct{ store Store }

func NewSource(s Store) memory.Source {
	return &Source{store: s}
}

func (s *Source) Name() string { return "notes" }

func (s *Source) Search(ctx context.Context, query string, _ []float32, limit int) ([]memory.Result, error) {
	all, err := s.store.SearchNotes(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("notes source: %w", err)
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}

	results := make([]memory.Result, len(all))
	for i, n := range all {
		results[i] = memory.Result{
			Content:    formatNote(n),
			Score:      noteScore(n),
			SourceName: s.Name(),
			Metadata: map[string]any{
				"note_id": n.ID,
				"pinned":  n.Pinned,
			},
		}
	}
	return results, nil
}

// formatNote renders a note as a compact LLM-readable string.
// Truncates long content so a single note can't dominate the context window.
func formatNote(n *Note) string {
	const maxContent = 400

	var sb strings.Builder
	fmt.Fprintf(&sb, "[%s] %s", n.UpdatedAt.Format("Jan 2 2006"), n.Title)

	if len(n.Tags) > 0 {
		fmt.Fprintf(&sb, " #%s", strings.Join(n.Tags, " #"))
	}
	if n.Content != "" {
		body := n.Content
		if len(body) > maxContent {
			body = body[:maxContent] + "…"
		}
		sb.WriteString("\n")
		sb.WriteString(body)
	}
	return sb.String()
}

// noteScore gives pinned notes a baseline boost; unpinned notes score 0.5.
// FTS rank ordering already handles relative relevance, so scores here are
// just for cross-source comparison in the Engine.
func noteScore(n *Note) float32 {
	if n.Pinned {
		return 0.85
	}
	return 0.5
}
