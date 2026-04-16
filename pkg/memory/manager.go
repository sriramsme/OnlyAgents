package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/embedder"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/message"
	"github.com/sriramsme/OnlyAgents/pkg/scheduler"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// RecentWindow is the time window for the wake-up prompt.
const RECENT_WINDOW = 24 * time.Hour

type Config struct {
	LLM      llm.Config      `mapstructure:"llm"`
	Embedder embedder.Config `mapstructure:"embedder"`
}

type cachedValue struct {
	value     string
	expiresAt time.Time
}

// Manager orchestrates memory operations including summarization and retrieval.
type Manager struct {
	store      EpisodeStore
	engine     *Engine
	summarizer *Summarizer

	cache sync.Map
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
		cache:      sync.Map{},
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

func (mm *Manager) GetRecentMemory(ctx context.Context, scope EpisodeScope, from time.Duration) (*Context, error) {
	return mm.engine.RecallRecent(ctx, scope, from)
}

// Gets cached recent memory scoped by sessions (past 24 hours) for the LLM to use in the system prompt.
// If the cache is expired, it will be refreshed. By default, the cache expires after 1 hour.
// Returns the cached result or an error if it cannot be retrieved.
func (mm *Manager) GetRecentMemoryForLLM(ctx context.Context, sessionID string) (string, error) {
	key := "recentActivity:" + sessionID

	if cached, ok := mm.cache.Load(key); ok {
		if cv, ok := cached.(cachedValue); ok {
			if cv.expiresAt.After(time.Now()) {
				return cv.value, nil
			}
		}
		mm.cache.Delete(key) // cleanup bad/expired
	}

	result, err := mm.engine.RecallRecent(ctx, ScopeSession, 24*time.Hour)
	if err != nil {
		return "", err
	}

	formatted := result.Render()

	mm.cache.Store(key, cachedValue{
		value:     formatted,
		expiresAt: time.Now().Add(1 * time.Hour),
	})

	return formatted, nil
}

func (mm *Manager) Recall(ctx context.Context, query string) (*Context, error) {
	return mm.engine.Recall(ctx, query)
}

func (mm *Manager) Remember(ctx context.Context, input tools.RememberInput) error {
	return mm.engine.Remember(ctx, input)
}
