package memory

import (
	"context"
	"strings"
)

// GetRelevantMemory assembles long-term memory context relevant to the given
// query. Called by the agent in execute() before building the messages slice.
// query is typically the user's current message — used for FTS fact search.
func (mm *Manager) GetRelevantMemory(ctx context.Context, query string) (*MemoryContext, error) {
	return mm.recallEngine.Recall(ctx, query)
}

// formatMemoryContext converts a MemoryContext into a plain-text block
// suitable for injection as a system message before the conversation history.
// Returns an empty string if there is nothing meaningful to inject.
func FormatMemoryContext(mc *MemoryContext) string {
	if mc == nil {
		return ""
	}

	var b strings.Builder

	return strings.TrimSpace(b.String())
}
