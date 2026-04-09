package embedder

import "context"

// Noop is a no-op embedder. It always returns nil vectors and
// signals Dimensions()==0 so RecallEngine skips vector search
// and falls back to FTS.
type Noop struct{}

func (Noop) Embed(_ context.Context, _ string) ([]float32, error) { return nil, nil }
func (Noop) Dimensions() int                                      { return 0 }
func (Noop) Provider() string                                     { return "none" }
