package skills

import "context"

// Skill interface that all skills must implement
type Skill interface {
	// Metadata
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Version() string
	RequiredCapabilities() []string

	// Execution
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)

	// LLM Integration
	GetSystemPrompt() string

	// Lifecycle
	Initialize() error
	Shutdown() error
}
