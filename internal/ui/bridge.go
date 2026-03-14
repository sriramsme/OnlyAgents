package ui

import (
	"context"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/core"
)

type Bridge struct {
	bus  core.UIBus
	subs map[string]chan core.UIEvent
	mu   sync.RWMutex
}

func NewBridge(bus core.UIBus) *Bridge {
	return &Bridge{
		bus:  bus,
		subs: make(map[string]chan core.UIEvent),
	}
}

func (b *Bridge) Subscribe(id string) (<-chan core.UIEvent, func()) {
	ch := make(chan core.UIEvent, 64)
	b.mu.Lock()
	b.subs[id] = ch
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, id)
		b.mu.Unlock()
	}
}

func (b *Bridge) AgentsStatus() func() []core.AgentStatus {
	return nil // set from outside via SetAgentsStatus
}

func (b *Bridge) Run(ctx context.Context) {
	for {
		select {
		case evt := <-b.bus:
			b.broadcast(evt)
		case <-ctx.Done():
			return
		}
	}
}

func (b *Bridge) broadcast(evt core.UIEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- evt:
		default:
		}
	}
}
