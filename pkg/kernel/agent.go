package kernel

import (
   "context"
   "fmt"
   "log/slog"
   "sync"
   "time"

   "github.com/sriramsme/OnlyAgents/pkg/llm"
)

type Agent struct {
	id          string
	skills   	*SkillRegistry
	connectors  *ConnectorRegistry
    security    *SecurityManager
    state       *StateManager
	llm         llm.Client

    // Message handling
    incoming    chan Message
    outgoing    chan Message

    // Lifecycle
    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup

    // Config
    config      Config
	logger      *slog.Logger
}

// Config holds agent configuration
type Config struct {
	ID             string
	MaxConcurrency int
	BufferSize     int
	LLMClient      llm.Client
}

// NewAgent creates a new agent instance
func NewAgent(config Config)( *Agent,error) {
   ctx, cancel := context.WithCancel(context.Background())

   logger := slog.With("agent_id", config.ID)

   agent := &Agent{
        id:          config.ID,
        skills:      NewSkillRegistry(),
        connectors:  NewConnectorRegistry(),
        security:    NewSecurityManager(),
        state:       NewStateManager(),
        llm:         config.LLMClient,
		incoming:    make(chan Message, config.BufferSize),
        outgoing:    make(chan Message, config.BufferSize),
        ctx:         ctx,
		cancel:      cancel,
        config:      config,
		logger:      logger,
    }

	return agent, nil
}

// Start starts the agent
func (a *Agent) Start() error {
    a.logger.Info("Starting agent...")

	// start message processing
	a.wg.Add(1)
	go a.processMessages()

	// start health check
	a.wg.Add(1)
	go a.healthCheck()

	a.logger.Info("Agent started successfully")
	return nil
}

// Stop gracefully shuts down the agent
func (a *Agent) Stop() error {
	a.logger.Info("Stopping agent...")
	a.cancel()

	// wait fo goroutines to finish
	done := make(chan struct{})
	go func(){
		a.wg.Wait()
		close(done)
	}()

	// wait with timeout
	select {
	case <-done:
		a.logger.Info("Agent stopped successfully")
		return nil

	case <-time.After(time.Second * 5):
		a.logger.Error("Agent stop timeout, forcefully shutting down...")
	}

	return nil
}

// processMessages is the main event loop of the agent
func (a *Agent) processMessages() {
    defer a.wg.Done()

	for {
		select {
		case msg := <-a.incoming:
			a.handleMessage(msg)
		case <-a.ctx.Done():
			return
		}
	}
}


// handleMessage processes a message
func (a *Agent) handleMessage(msg Message) {
    a.logger.Info("Received message",
        "message_id", msg.ID,
        "from", msg.FromAgent,
        "action", msg.Action)

	// TODO: Full message handling pipeline
    // 1. Security verification
    // 2. Intent classification
    // 3. Skill selection
    // 4. Execution
    // 5. Response signing
}


// healthCheck periodically checks the agent's health
func (a *Agent) healthCheck() {
	defer a.wg.Done()

	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// TODO: Health check

			a.logger.Info("Health check successful")
		case <-a.ctx.Done():
			return
		}
	}
}

// AskLLM is a helper method for skills to use
func (a *Agent) AskLLM(ctx context.Context, system, prompt string) (string, error) {
    if a.llm == nil {
        return "", fmt.Errorf("LLM client not configured")
    }

    resp, err := a.llm.Complete(ctx, llm.CompletionRequest{
        System: system,
        Messages: []llm.Message{
            {Role: llm.RoleUser, Content: prompt},
        },
    })

    if err != nil {
        return "", err
    }

    a.logger.Debug("LLM completion",
        "input_tokens", resp.InputTokens,
        "output_tokens", resp.OutputTokens,
        "model", resp.Model)

    return resp.Content, nil
}
// RegisterSkill registers a new skill to the agent
func (a *Agent) RegisterSkill(skill Skill) error {
	return a.skills.Register(skill)
}

// SendMessage sends a message to another agent
func (a *Agent) SendMessage(msg Message) error {
	select {
	case a.outgoing <- msg:
		return nil
	case <-time.After(time.Second * 1):
		return fmt.Errorf("send message timeout")
	}
}
