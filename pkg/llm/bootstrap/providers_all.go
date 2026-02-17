//go:build !minimal

package bootstrap

import (
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/providers/anthropic"
)
