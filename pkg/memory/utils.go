package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

// callLLM sends a single-turn chat to the LLM with an explicit system prompt.
// system sets the role/behaviour; user contains the data to process.
func callLLM(ctx context.Context, client llm.Client, system, user string) (string, error) {
	resp, err := client.Chat(ctx, &llm.Request{
		Messages: []llm.Message{
			llm.SystemMessage(system),
			llm.UserMessage(user),
		},
		Metadata: map[string]string{"agent_id": "summarizer"},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
