package memory

import (
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/embedder"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/message"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
)

type Config struct {
	LLM      llm.Config      `mapstructure:"llm"`
	Embedder embedder.Config `mapstructure:"embedder"`
}

// Manager orchestrates memory operations including summarization and retrieval.
type Manager struct {
	store      EpisodeStore
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

	summarizer := NewSummarizer(store, llmClient, embedder, tz)

	return &Manager{
		store:      store,
		summarizer: summarizer,
	}, nil
}

func (m *Manager) Jobs() []scheduler.Job {
	return m.summarizer.Jobs()
}
