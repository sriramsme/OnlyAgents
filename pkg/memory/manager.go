package memory

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/embedder"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/message"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

type Config struct {
	LLM      llm.Config      `mapstructure:"llm"`
	Embedder embedder.Config `mapstructure:"embedder"`
}

// Manager orchestrates memory operations including summarization and retrieval.
type Manager struct {
	store      EpisodeStore
	engine     *Engine
	summarizer *Summarizer
}

// MemoryStore is the combined store interface required by the Memory and its child components..
type Store interface {
	EpisodeStore
	PraxisStore
	NexusStore
	message.Store
}

// NewManager creates a new memory manager.
func NewManager(store Store, cfg Config, tz string) (*Manager, error) {
	embedder, err := embedder.New(cfg.Embedder)
	if err != nil {
		return nil, fmt.Errorf("new embedder: %w", err)
	}

	llmClient, err := llm.New(cfg.LLM)
	if err != nil {
		return nil, fmt.Errorf("new llm client: %w", err)
	}

	nResolver := newNexusResolver(store, llmClient)

	summarizer := NewSummarizer(store, llmClient, embedder, nResolver, tz)

	engine := newEngine(store, embedder, llmClient, nResolver)

	return &Manager{
		store:      store,
		summarizer: summarizer,
		engine:     engine,
	}, nil
}

func (m *Manager) Jobs() []scheduler.Job {
	return m.summarizer.Jobs()
}

func (m *Manager) Summarizer() *Summarizer {
	return m.summarizer
}

func (m *Manager) Engine() *Engine {
	return m.engine
}

// GetRelevantMemory assembles long-term memory context relevant to the given
// query. Called by the agent in execute() before building the messages slice.
// query is typically the user's current message — used for FTS fact search.
func (mm *Manager) GetRelevantMemory(ctx context.Context, query string) (*Context, error) {
	return mm.engine.Recall(ctx, query)
}

func (mm *Manager) Recall(ctx context.Context, query string) (*Context, error) {
	return mm.engine.Recall(ctx, query)
}

func (mm *Manager) Remember(ctx context.Context, input tools.RememberInput) error {
	return mm.engine.Remember(ctx, input)
}
