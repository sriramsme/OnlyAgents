package kernel

import (
	"github.com/sriramsme/OnlyAgents/pkg/channels/oaChannel"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// implements KernelReader
// Used by UI/server

func (k *Kernel) OAChannel() *oaChannel.OAChannel {
	ch, err := k.channels.Get("onlyagents")
	if err != nil {
		k.logger.Error("failed to get channel", "name", "oaChannel", "err", err)
		return nil
	}

	oaCh, ok := ch.(*oaChannel.OAChannel)
	if !ok {
		k.logger.Error("channel type mismatch", "expected", "*oaChannel.OAChannel")
		return nil
	}

	return oaCh
}

func (k *Kernel) Subscribe(id string) (<-chan core.UIEvent, func()) {
	ch := make(chan core.UIEvent, 64)
	k.uiSubsMu.Lock()
	k.uiSubs[id] = ch
	k.uiSubsMu.Unlock()

	return ch, func() {
		k.uiSubsMu.Lock()
		delete(k.uiSubs, id)
		k.uiSubsMu.Unlock()
	}
}

func (k *Kernel) AgentsStatus() []core.AgentStatus {
	ids := k.agents.ListAll()
	out := make([]core.AgentStatus, 0, len(ids))
	for _, id := range ids {
		a, err := k.agents.Get(id)
		if err != nil {
			continue
		}
		out = append(out, a.Status())
	}
	return out
}

func (k *Kernel) IsHealthy() bool {
	return k.ctx.Err() == nil
}

// Helpers

func (k *Kernel) wireOAChannel() {
	ch, err := k.channels.Get("onlyagents")
	if err != nil {
		return
	}
	oaCh, ok := ch.(*oaChannel.OAChannel)
	if !ok {
		return
	}
	// Inject Subscribe so each WS connection gets its own UIBus subscription.
	oaCh.SetSubscribe(k.Subscribe)
	oaCh.SetAgentsStatus(k.AgentsStatus)
}
