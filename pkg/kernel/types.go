package kernel

import (
	"context"
	"log/slog"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/a2a"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

type AgentRegistry struct {
	agents map[string]*Agent
	mu     sync.RWMutex
}

type Agent struct {
	id          string
	isExecutive bool
	skills      *SkillRegistry
	connectors  *ConnectorRegistry
	state       *StateManager
	llmClient   llm.Client

	// Message handling
	incoming chan a2a.Message
	outgoing chan a2a.Message

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Config
	config Config
	logger *slog.Logger
}

// Config holds agent configuration
type Config struct {
	ID             string
	IsExecutive    bool
	MaxConcurrency int
	BufferSize     int
	LLMClient      llm.Client
}
