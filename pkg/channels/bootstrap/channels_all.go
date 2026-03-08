//go:build !channel_minimal

package bootstrap

import (
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/oaChannel"
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/telegram"
	// _ "github.com/sriramsme/OnlyAgents/pkg/connectors/discord"
	// _ "github.com/sriramsme/OnlyAgents/pkg/connectors/slack"
)
