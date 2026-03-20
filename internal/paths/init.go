package paths

import (
	"fmt"
	"os"
)

// Init ensures the home directory exists, creates subdirectories,
// and seeds default files on first run.
func Init() (*Paths, error) {
	paths := NewPaths()

	dirs := []string{
		paths.Home,
		paths.Agents,
		paths.Connectors,
		paths.Channels,
		paths.Skills,
		paths.Councils,
		paths.Logs,
		paths.Cache,
		paths.Marketplace,
		paths.Media,
		paths.SkillCache,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	return paths, nil
}
