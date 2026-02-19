//go:build !conn_minimal && conn_telegram

package bootstrap

import (
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/telegram"
)

// "If conn_telegram tag, import only Telegram"
