package memory

import (
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/embedder"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

// EngineStore is the storage interface Engine needs.
// Subset of the full Storage interface — only what retrieval requires.
type EngineStore interface {
	EpisodeStore
	NexusStore
	PraxisStore
}

// engineConfig controls retrieval budgets. Sensible defaults, all overridable.
type engineConfig struct {
	MaxEpisodes  int           // max session episodes to return
	MaxFacts     int           // max Nexus facts to return
	MaxPatterns  int           // max Praxis patterns to return
	RecentWindow time.Duration // how far back "recent" episodes go for wake-up
}

type Engine struct {
	store    EngineStore
	embedder embedder.Embedder
	llm      llm.Client
	cfg      engineConfig
}

func defaultEngineConfig() engineConfig {
	return engineConfig{
		MaxEpisodes:  5,
		MaxFacts:     20,
		MaxPatterns:  10,
		RecentWindow: 48 * time.Hour,
	}
}

func newEngine(store EngineStore, emb embedder.Embedder, llm llm.Client) *Engine {
	return &Engine{
		store:    store,
		embedder: emb,
		llm:      llm,
		cfg:      defaultEngineConfig(),
	}
}
