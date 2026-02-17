//go:build minimal && openai

package bootstrap

import (
	_ "github.com/sriramsme/OnlyAgents/pkg/llm/providers/openai"
)
