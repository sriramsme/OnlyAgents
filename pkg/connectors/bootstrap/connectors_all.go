//go:build !conn_minimal

package bootstrap

import (
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/brave"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/duckduckgo"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/gmail"
	_ "github.com/sriramsme/OnlyAgents/pkg/connectors/perplexity"
)
