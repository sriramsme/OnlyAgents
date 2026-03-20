package paths

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
	DefaultEnvFile   = ".env"
)

// Paths holds all the canonical paths in ~/.onlyagents
type Paths struct {
	Home        string
	Agents      string
	Connectors  string
	Channels    string
	Skills      string
	Councils    string
	Logs        string
	Cache       string
	Marketplace string
	DBPath      string
	ConfigPath  string
	ServerPath  string
	UserPath    string
	VaultPath   string
	SkillCache  string
	Media       string
	EnvPath     string
}

func NewPaths() *Paths {
	return &Paths{
		Home:        HomeDir(),
		Agents:      AgentsDir(),
		Connectors:  ConnectorsDir(),
		Channels:    ChannelsDir(),
		Skills:      SkillsDir(),
		Councils:    CouncilsDir(),
		Logs:        LogsDir(),
		Cache:       CacheDir(),
		Marketplace: MarketplaceDir(),
		DBPath:      DBPath(),
		ConfigPath:  ConfigPath(),
		ServerPath:  ServerConfigPath(),
		UserPath:    UserConfigPath(),
		VaultPath:   VaultPath(),
		SkillCache:  SkillCacheDir(),
		Media:       MediaDir(),
		EnvPath:     EnvPath(),
	}
}

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

func ConfigPath() string {
	return filepath.Join(HomeDir(), DefaultOnlyAgentsConfig)
}

func EnvPath() string {
	return filepath.Join(HomeDir(), DefaultEnvFile)
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

func CouncilsDir() string {
	return filepath.Join(HomeDir(), "councils")
}

func LogsDir() string {
	return filepath.Join(HomeDir(), "logs")
}

func CacheDir() string {
	return filepath.Join(HomeDir(), "cache")
}

func SkillCacheDir() string {
	return filepath.Join(CacheDir(), "skills")
}

func MarketplaceDir() string {
	return filepath.Join(HomeDir(), "marketplace")
}

func MediaDir() string {
	return filepath.Join(HomeDir(), "media")
}
