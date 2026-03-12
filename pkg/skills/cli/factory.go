// pkg/skills/cli/factory.go
package cli

import (
	"context"
	"fmt"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
)

func init() {
	// Auto-register CLI skill loader
	skills.RegisterLoader("cli", LoadCLISkillsFromConfig)
}

// LoadCLISkillsFromConfig is called by the registry to load CLI skills
func LoadCLISkillsFromConfig(ctx context.Context, configDir string, executor interface{}) ([]skills.Skill, error) {
	// Type assert the executor
	cliExecutor, ok := executor.(*CLIExecutor)
	if !ok {
		return nil, fmt.Errorf("invalid executor type for CLI skills")
	}

	// Load all .md files
	cliSkills, err := LoadCLISkillsFromDirectory(configDir, cliExecutor)
	if err != nil {
		return nil, err
	}

	// Convert []*CLISkill to []skills.Skill
	result := make([]skills.Skill, len(cliSkills))
	for i, s := range cliSkills {
		if s.Enabled {
			result[i] = s
		}
	}

	logger.Log.Info("CLI loader registered skills",
		"count", len(result),
		"directory", configDir)

	return result, nil
}
