package config

type CouncilConfig struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Enabled     bool     `yaml:"enabled"`
	Agents      []string `yaml:"agents"`
	Skills      []string `yaml:"skills"`
	Connectors  []string `yaml:"connectors"`
}
