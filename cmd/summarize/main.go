package main

import (
	"context"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/skills/runner"
	"github.com/sriramsme/OnlyAgents/pkg/skills/summarize"
	"github.com/sriramsme/OnlyAgents/pkg/tools"

	_ "github.com/sriramsme/OnlyAgents/pkg/llm/bootstrap"
)

func main() {
	opts := llm.Options{}
	runner.Run(
		"summarize",
		"Summarize text, URLs, files, and YouTube videos",
		tools.GetSummarizeEntries(),
		runner.RegisterLLMFlags,
		runner.LLMSetup(func(client llm.Client) (skills.Skill, error) {
			return summarize.New(context.Background(), client, opts)
		}),
	)
}
