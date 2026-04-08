package kernel

import (
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/core"
)

func (k *Kernel) GetActiveChannelMetadata() (*core.ChannelMetadata, error) {
	active := *k.channels.GetActive()
	if active == nil {
		return nil, fmt.Errorf("no active channel")
	}
	agentID := k.agents.GetExecutive().ID() // TODO: Get agent ID from active channel
	conv, err := k.cm.GetActiveConversationByChannel(k.ctx, active.PlatformName(), agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session for channel: %s", active.PlatformName())
	}
	metadata := &core.ChannelMetadata{
		Name:      active.PlatformName(),
		SessionID: conv.ID,
		ChatID:    conv.ChatID,
	}
	return metadata, nil
}
