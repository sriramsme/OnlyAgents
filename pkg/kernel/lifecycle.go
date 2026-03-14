package kernel

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/core"
)

func (k *Kernel) Run() error {
	k.logger.Info("starting kernel")

	// Start all connectors
	k.logger.Info("starting connectors")
	for _, c := range k.connectors.All() {
		if err := c.Start(); err != nil {
			return fmt.Errorf("failed to start connector %s: %w", c.Name(), err)
		}
	}

	// Start all agents
	k.logger.Info("starting agents")
	for _, a := range k.agents.All() {
		if err := a.Start(); err != nil {
			return fmt.Errorf("failed to start agent %s: %w", a.ID(), err)
		}
	}

	// Start workflow engine
	k.logger.Info("starting workflow engine")
	if err := k.workflow.Start(); err != nil {
		return fmt.Errorf("failed to start workflow engine: %w", err)
	}

	// Start all channels — they'll write MessageReceived events to the bus
	k.logger.Info("starting channels")
	for _, ch := range k.channels.All() {
		if err := ch.Start(); err != nil {
			return fmt.Errorf("failed to start channel %s: %w", ch.PlatformName(), err)
		}
	}

	// Start event router
	k.wg.Add(1)
	go k.run()

	if k.uiBus != nil {
		k.wg.Add(1)
		go k.runUI()
	}

	k.logger.Info("kernel started")
	fmt.Println("kernel started")
	return nil
}

func (k *Kernel) Shutdown() error {
	k.logger.Info("stopping kernel - beginning graceful shutdown")

	// Step 1: Stop accepting new events - stop all channels first
	k.logger.Info("stopping channels to prevent new messages")
	for _, ch := range k.channels.All() {
		if err := ch.Stop(); err != nil {
			k.logger.Error("failed to stop channel",
				"channel", ch.PlatformName(),
				"error", err)
		}
	}

	// Step 2: Allow time for in-flight events to be processed
	k.logger.Info("draining event bus")
	drainTimer := time.NewTimer(2 * time.Second)
	select {
	case <-drainTimer.C:
		k.logger.Info("drain period complete", "pending_events", len(k.bus))
	case <-k.ctx.Done():
		// Already cancelled from outside
	}

	// Step 3: Cancel context to signal shutdown to router and components
	k.logger.Info("cancelling context")
	k.cancel()

	// Step 4: Wait for event router and tool call goroutines
	k.logger.Info("waiting for event router and goroutines to complete")
	done := make(chan struct{})
	go func() {
		k.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		k.logger.Info("all goroutines stopped gracefully")
	case <-time.After(10 * time.Second):
		k.logger.Error("kernel shutdown timeout - some goroutines may be leaked",
			"warning", "check for blocked channels or infinite loops")
	}

	// Step 5: Stop agents
	k.logger.Info("stopping agents")
	for _, a := range k.agents.All() {
		if err := a.Stop(); err != nil {
			k.logger.Error("failed to stop agent",
				"agent_id", a.ID(),
				"error", err)
		}
	}

	// Step 6: Stop connectors
	k.logger.Info("stopping connectors")
	for _, c := range k.connectors.All() {
		if err := c.Stop(); err != nil {
			k.logger.Error("failed to stop connector",
				"connector", c.Name(),
				"error", err)
		}
	}

	defer func() {
		if err := k.store.Close(); err != nil {
			k.logger.Error("storage close failed", "err", err)
		}
	}()

	k.logger.Info("kernel stopped")
	return nil
}

// --- Event router ---

func (k *Kernel) run() {
	defer k.wg.Done()

	for {
		select {
		case evt := <-k.bus:
			// Wrap route() in panic recovery
			func() {
				defer func() {
					if r := recover(); r != nil {
						k.logger.Error("PANIC in event router",
							"panic", r,
							"event_type", evt.Type,
							"correlation_id", evt.CorrelationID,
							"agent_id", evt.AgentID,
							"stack", string(debug.Stack()))

						// Try to send error response if this was a request
						if evt.ReplyTo != nil {
							errorEvt := core.Event{
								Type:          evt.Type,
								CorrelationID: evt.CorrelationID,
								Payload: core.ErrorPayload{
									Error: "internal error processing event",
								},
							}

							select {
							case evt.ReplyTo <- errorEvt:
								k.logger.Info("sent error response to requester")
							default:
								k.logger.Warn("could not send error response - channel full or closed")
							}
						}
					}
				}()

				k.route(evt)
			}()

		case <-k.ctx.Done():
			k.logger.Info("event router shutting down")
			return
		}
	}
}

// reads UIBus, fans out to all SSE subscribers
func (k *Kernel) runUI() {
	defer k.wg.Done()
	for {
		select {
		case evt := <-k.uiBus:
			k.broadcastUI(evt)
		case <-k.ctx.Done():
			k.logger.Info("ui event router shutting down")
			return
		}
	}
}

// non-blocking fan-out to all SSE subscriber channels
func (k *Kernel) broadcastUI(evt core.UIEvent) {
	k.uiSubsMu.RLock()
	defer k.uiSubsMu.RUnlock()
	for _, ch := range k.uiSubs {
		select {
		case ch <- evt:
		default: // slow client — drop rather than block the fan-out loop
		}
	}
}
