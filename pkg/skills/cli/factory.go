// pkg/skills/cli/factory.go
package cli

import (
	"context"
	"fmt"
	"path/filepath"

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

	// Build path to CLI skills directory
	cliSkillsDir := filepath.Join(configDir, "skills")

	// Load all .md files
	cliSkills, err := LoadCLISkillsFromDirectory(cliSkillsDir, cliExecutor)
	if err != nil {
		return nil, err
	}

	// Convert []*CLISkill to []skills.Skill
	result := make([]skills.Skill, len(cliSkills))
	for i, s := range cliSkills {
		result[i] = s
	}

	logger.Log.Info("CLI loader registered skills",
		"count", len(result),
		"directory", cliSkillsDir)

	return result, nil
}
