package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// LoadAllSkillConfigs loads all *.yaml files from the skills config dir.
func LoadAllSkillConfigs() (map[string]*SkillConfig, error) {
	dir := SkillsDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read skill dir: %w", err)
	}

	configs := make(map[string]*SkillConfig)

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
	setSkillDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg SkillConfig
	if err := v.Unmarshal(&cfg, func(dc *mapstructure.DecoderConfig) {
		dc.TagName = "mapstructure"
		dc.WeaklyTypedInput = true
		dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		)
	}); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.RawConfig = v.AllSettings()
	return &cfg, nil
}

func setSkillDefaults(v *viper.Viper) {
	// Common
	v.SetDefault("enabled", true)
	v.SetDefault("version", "1.0.0")
	v.SetDefault("access_level", "read")

	// Executor
	v.SetDefault("executor.allowed_shells", []string{"bash", "sh"})
	v.SetDefault("executor.max_output_size", 1024*1024) // 1MB
	v.SetDefault("executor.max_execution_time", 60)     // seconds
	v.SetDefault("executor.use_sandbox", false)
}
