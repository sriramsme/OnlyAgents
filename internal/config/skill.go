package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// LoadAllSkillConfigs loads all *.yaml files from the skills config dir.
func LoadAllSkillConfigs() (map[tools.SkillName]*SkillConfig, error) {
	dir := SkillsDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read skill dir: %w", err)
	}

	configs := make(map[tools.SkillName]*SkillConfig)

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}

		cfg, err := LoadSkillConfig(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", f.Name(), err)
		}

		configs[cfg.Name] = cfg
	}

	return configs, nil
}

func LoadSkillConfig(configPath string) (*SkillConfig, error) {
	if configPath == "" {
		return nil, fmt.Errorf("config path empty")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("skill config not found: %s", configPath)
	}

	v := viper.New()
	v.SetConfigFile(configPath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg SkillConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Store raw config for platform-specific unmarshaling
	cfg.RawConfig = v.AllSettings()

	return &cfg, nil
}
