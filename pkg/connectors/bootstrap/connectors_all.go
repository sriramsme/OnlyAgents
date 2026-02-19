//go:build !conn_minimal

package bootstrap

import (
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/telegram"
	// _ "github.com/sriramsme/OnlyAgents/pkg/connectors/discord"
	// _ "github.com/sriramsme/OnlyAgents/pkg/connectors/slack"
)
