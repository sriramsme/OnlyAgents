//go:build !channel_minimal && channel_onlyagents

package bootstrap

import (
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/onlyagents"
)
