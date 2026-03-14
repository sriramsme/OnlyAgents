package kernel

import (
	"github.com/sriramsme/OnlyAgents/pkg/channels/oaChannel"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

// KernelReader is the read-only interface exposed to the server.
type KernelReader interface {
	AgentsStatus() []core.AgentStatus
	IsHealthy() bool
	UIBus() core.UIBus
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

func (k *Kernel) UIBus() core.UIBus {
	return k.uiBus
}

func (k *Kernel) OAChannel() *oaChannel.OAChannel {
	ch, err := k.channels.Get("onlyagents")
	if err != nil {
		return nil
	}

	oaCh, ok := ch.(*oaChannel.OAChannel)
	if !ok {
		return nil
	}

	return oaCh
}
