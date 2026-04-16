package memory

import (
	"context"
)

// Source is a single retrieval backend. Each store type that can contribute
// to a recall query implements this interface.
// This is the extension point — add new sources without touching Engine.
type Source interface {
	// Name identifies this source in logs and Context output.
	Name() string
	// Search returns raw results for the given query.
	// queryVec is nil when no embedder is configured — sources must handle this.
	Search(ctx context.Context, query string, queryVec []float32, limit int) ([]Result, error)
}

// Result is a normalized retrieval result from any source.
type Result struct {
	// Content is the human-readable text injected into the prompt.
	Content string
	// Score is the relevance score (0.0-1.0). Used for final re-ranking.
	Score float32
	// SourceName identifies where this came from (for logging/debugging).
	SourceName string
	// Metadata carries source-specific data (entity IDs, episode IDs, etc.)
	// for cross-referencing between sources.
	Metadata map[string]any
}
