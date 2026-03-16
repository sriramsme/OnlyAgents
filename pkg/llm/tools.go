// Package llm provides LLM client abstractions for OnlyAgents
package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// HasToolCalls returns true if the response contains tool calls
func (r *Response) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// ParseToolArguments safely parses tool arguments JSON
func ParseToolArguments(arguments string) (map[string]any, error) {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return map[string]any{}, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(arguments), &parsed); err != nil {
		return nil, fmt.Errorf("invalid tool arguments JSON: %w", err)
	}
	return parsed, nil
}
