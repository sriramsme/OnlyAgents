package memory

import "context"

// Embedder is optional. If nil, stores fall back to FTS5.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
