//go:build !channel_minimal && channel_telegram

package bootstrap

import (
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/oaChannel"
	_ "github.com/sriramsme/OnlyAgents/pkg/channels/telegram"
)

// "If conn_telegram tag, import only Telegram"
