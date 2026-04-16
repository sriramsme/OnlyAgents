package memory

import (
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
	MaxPerSource int
	MaxTotal     int
}

type Engine struct {
	store    EngineStore
	sources  []Source
	embedder embedder.Embedder
	llm      llm.Client
	nexus    *nexusResolver
	cfg      engineConfig
}

func defaultEngineConfig() engineConfig {
	return engineConfig{
		MaxPerSource: 10,
		MaxTotal:     10,
	}
}

func newEngine(store EngineStore, emb embedder.Embedder, llm llm.Client, nexus *nexusResolver) *Engine {
	e := &Engine{
		embedder: emb,
		llm:      llm,
		cfg:      defaultEngineConfig(),
		nexus:    nexus,
		store:    store,
	}

	e.sources = []Source{
		&episodeSource{store: store},
		&nexusSource{store: store},
		&praxisSource{store: store},
	}

	return e
}

// AddSource registers an external source — e.g. calendar, notes, tasks.
func (e *Engine) AddSource(s Source) {
	e.sources = append(e.sources, s)
}
