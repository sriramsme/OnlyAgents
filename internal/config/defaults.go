package config

import (
	"os"
	"path/filepath"
)

const (
	DefaultHomeDirName = ".onlyagents"

	DefaultOnlyAgentsConfig = "config.yaml"
	DefaultUserConfig       = "user.yaml"
	DefaultServerConfig     = "server.yaml"
	DefaultAgentsDir        = "agents"
	DefaultSkillsDir        = "skills"
	DefaultChannelsDir      = "channels"
	DefaultConnectorsDir    = "connectors"

	DefaultVaultFile = "vault.yaml"
	DefaultDBFile    = "onlyagents.db"
)

func HomeDir() string {
	if v := os.Getenv("ONLYAGENTS_HOME"); v != "" {
		return v
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "." // fallback
	}

	return filepath.Join(home, DefaultHomeDirName)
}

func OnlyAgentsConfigPath() string {
	return filepath.Join(HomeDir(), DefaultOnlyAgentsConfig)
}

func UserConfigPath() string {
	return filepath.Join(HomeDir(), DefaultUserConfig)
}

func ServerConfigPath() string {
	return filepath.Join(HomeDir(), DefaultServerConfig)
}

func AgentsDir() string {
	return filepath.Join(HomeDir(), DefaultAgentsDir)
}

func SkillsDir() string {
	return filepath.Join(HomeDir(), DefaultSkillsDir)
}

func ChannelsDir() string {
	return filepath.Join(HomeDir(), DefaultChannelsDir)
}

func ConnectorsDir() string {
	return filepath.Join(HomeDir(), DefaultConnectorsDir)
}

func VaultPath() string {
	return filepath.Join(HomeDir(), DefaultVaultFile)
}

func DBPath() string {
	return filepath.Join(HomeDir(), DefaultDBFile)
}
