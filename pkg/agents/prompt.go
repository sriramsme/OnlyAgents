package agents

import (
	"context"
	"strings"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

func (a *Agent) SetSystemPrompt(userSection string, availableAgents string) {
	parts := []string{
		a.soul.SystemPrompt(availableAgents),
	}
	parts = append(parts, userSection)
	a.systemPrompt = strings.Join(parts, "\n\n")
}

func (a *Agent) GetSystemPrompt() string {
	return a.systemPrompt
}

// AskLLM is a helper for skills that need LLM assistance (e.g. drafting text).
func (a *Agent) AskLLM(ctx context.Context, system, prompt string) (string, error) {
	resp, err := a.llmClient.Chat(ctx, &llm.Request{
		Messages: []llm.Message{
			llm.SystemMessage(system),
			llm.UserMessage(prompt),
		},
		Metadata: map[string]string{"agent_id": a.id, "context": "skill_helper"},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
