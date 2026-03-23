package kernel

import (
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

func (k *Kernel) GetActiveChannelMetadata() *core.ChannelMetadata {
	active := *k.channels.GetActive()
	if active == nil {
		return nil
	}
	agentID := k.agents.GetExecutive().ID() // TODO: Get agent ID from active channel
	sessionID, err := k.store.GetOrCreateSession(k.ctx, active.PlatformName(), agentID)
	if err != nil {
		k.logger.Error("failed to get session", "err", err)
		return nil
	}
	metadata := &core.ChannelMetadata{
		Name:      active.PlatformName(),
		SessionID: sessionID,
	}
	return metadata
}
