package agents

import (
	"fmt"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
)

func (a *Agent) Start() error {
	for _, s := range a.skills {
		if err := s.Initialize(); err != nil {
			return fmt.Errorf("skill %s: failed to initialize: %w", s.Name(), err)
		}
	}
	a.logger.Info("starting agent", "model", a.llmClient.Model())
	a.wg.Add(2)
	go a.processEvents()
	go a.healthCheck()
	return nil
}

func (a *Agent) Stop() error {
	a.logger.Info("stopping agent")

	for _, s := range a.skills {
		if err := s.Shutdown(); err != nil {
			return fmt.Errorf("skill %s: failed to shutdown: %w", s.Name(), err)
		}
	}
	// Cancel context to signal shutdown
	a.cancel()

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	// Wait with timeout
	timeout := 10 * time.Second
	select {
	case <-done:
		a.logger.Info("agent stopped gracefully")
		return nil
	case <-time.After(timeout):
		a.logger.Error("agent shutdown timeout",
			"timeout", timeout,
			"warning", "goroutines may still be running - check for blocked LLM calls or stuck tool executions")
		return fmt.Errorf("agent %s shutdown timeout after %v", a.id, timeout)
	}
}

func (a *Agent) Status() core.AgentStatus {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return core.AgentStatus{
		ID:          a.id,
		Name:        a.name,
		State:       a.state,
		CurrentTask: a.currentTask,
		LastActive:  a.lastActive,
		Model:       a.llmClient.Model(),
		IsExecutive: a.isExecutive,
	}
}

func (a *Agent) setState(state core.AgentState, task string) {
	a.stateMu.Lock()
	a.state = state
	a.currentTask = task
	a.lastActive = time.Now()
	a.stateMu.Unlock()
}

func (a *Agent) healthCheck() {
	defer a.wg.Done()
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.logger.Debug("health check ok")
		case <-a.ctx.Done():
			a.logger.Info("health check shutting down")
			return
		}
	}
}
