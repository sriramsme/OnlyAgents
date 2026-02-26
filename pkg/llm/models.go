// pkg/llm/capabilities.go
package llm

import (
	"fmt"
	"sort"
	"strings"
)

// ModelCapabilities defines common capabilities across all LLM providers
type ModelCapabilities struct {
	// Availability
	Available bool
	Provider  Provider // e.g., "openai", "anthropic", "google"

	// Token limits
	MaxTokens        int
	DefaultMaxTokens int
	ContextWindow    int // Total context window size

	// Temperature constraints
	SupportsTemperature bool
	MinTemperature      float64
	MaxTemperature      float64
	DefaultTemperature  float64

	// Common features
	SupportsStreaming   bool
	SupportsToolCalling bool
	SupportsVision      bool
	SupportsJSON        bool // Structured output

	// Cost (USD per 1M tokens)
	InputCostPer1M  float64
	OutputCostPer1M float64

	// Metadata
	Description string
	ReleaseDate string
	Deprecated  bool
	ReplacedBy  string // Model that replaces this if deprecated

	SupportsPromptCaching   bool
	SupportsAudio           bool
	SupportsReasoningEffort bool
	IsReasoningModel        bool
	SupportsFunctionCalling bool

	// Provider-specific extensions (use type assertion to access)
	Extensions interface{}
}

// ModelInfo contains display-friendly model information
type ModelInfo struct {
	Name         string
	Provider     string
	Description  string
	Capabilities ModelCapabilities
	Features     []string // Human-readable feature list
}

// CapabilityFilter defines criteria for filtering models
type CapabilityFilter struct {
	Provider         *Provider
	MinMaxTokens     *int
	MaxCostPer1M     *float64
	RequireStreaming bool
	RequireTools     bool
	RequireVision    bool
}

// ModelRegistry is the interface all providers must implement
type ModelRegistry interface {
	GetSupportedModels() []string
	GetModelCapabilities(model string) (ModelCapabilities, error)
	GetModelInfo(model string) (ModelInfo, error)
	FilterModels(filter CapabilityFilter) []ModelInfo
}

// GetSupportedModels returns list of all available models
func GetSupportedModels(registry map[string]ModelCapabilities) []string {
	models := make([]string, 0, len(registry))
	for model, caps := range registry {
		if caps.Available {
			models = append(models, model)
		}
	}
	sort.Strings(models)
	return models
}

// GetModelCapabilities retrieves capabilities for a model
func GetModelCapabilities(model string, registry map[string]ModelCapabilities) (ModelCapabilities, error) {
	caps, exists := registry[model]
	if !exists {
		return ModelCapabilities{}, fmt.Errorf("unknown model: %s", model)
	}
	if !caps.Available {
		return ModelCapabilities{}, fmt.Errorf("model not available: %s", model)
	}
	return caps, nil
}

// GetModelInfo returns detailed information about a model
func GetModelInfo(model string, registry map[string]ModelCapabilities) (ModelInfo, error) {
	caps, err := GetModelCapabilities(model, registry)
	if err != nil {
		return ModelInfo{}, err
	}

	features := buildFeatureList(caps)

	return ModelInfo{
		Name:         model,
		Provider:     string(ProviderOpenAI),
		Description:  caps.Description,
		Capabilities: caps,
		Features:     features,
	}, nil
}

// GetAllModelsInfo returns info for all available models
func GetAllModelsInfo(registry map[string]ModelCapabilities) []ModelInfo {
	models := GetSupportedModels(registry)
	infos := make([]ModelInfo, 0, len(models))

	for _, model := range models {
		if info, err := GetModelInfo(model, registry); err == nil {
			infos = append(infos, info)
		}
	}

	return infos
}

// FilterModels returns models matching the filter criteria
func FilterModels(filter CapabilityFilter, registry map[string]ModelCapabilities) []ModelInfo {
	allModels := GetAllModelsInfo(registry)
	filtered := make([]ModelInfo, 0)

	for _, info := range allModels {
		if matchesFilter(info.Capabilities, filter) {
			filtered = append(filtered, info)
		}
	}

	return filtered
}

// CompareModels returns a side-by-side comparison
func CompareModels(models []string, registry map[string]ModelCapabilities) (string, error) {
	if len(models) == 0 {
		return "", fmt.Errorf("no models provided")
	}

	var sb strings.Builder
	sb.WriteString("Model Comparison\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	infos := make([]ModelInfo, 0, len(models))
	for _, model := range models {
		info, err := GetModelInfo(model, registry)
		if err != nil {
			return "", fmt.Errorf("model %s: %w", model, err)
		}
		infos = append(infos, info)
	}

	// Header
	sb.WriteString(fmt.Sprintf("%-20s", "Feature"))
	for _, info := range infos {
		sb.WriteString(fmt.Sprintf("%-20s", truncate(info.Name, 18)))
	}
	sb.WriteString("\n" + strings.Repeat("-", 80) + "\n")

	// Rows
	writeComparisonRow(&sb, "Max Tokens", infos, func(c ModelCapabilities) string {
		return fmt.Sprintf("%d", c.MaxTokens)
	})
	writeComparisonRow(&sb, "Context Window", infos, func(c ModelCapabilities) string {
		return fmt.Sprintf("%d", c.ContextWindow)
	})
	writeComparisonRow(&sb, "Temperature Range", infos, func(c ModelCapabilities) string {
		if !c.SupportsTemperature {
			return "N/A"
		}
		return fmt.Sprintf("%.1f-%.1f", c.MinTemperature, c.MaxTemperature)
	})
	writeComparisonRow(&sb, "Streaming", infos, func(c ModelCapabilities) string {
		return yesNo(c.SupportsStreaming)
	})
	writeComparisonRow(&sb, "Tools", infos, func(c ModelCapabilities) string {
		return yesNo(c.SupportsToolCalling)
	})
	writeComparisonRow(&sb, "Vision", infos, func(c ModelCapabilities) string {
		return yesNo(c.SupportsVision)
	})
	writeComparisonRow(&sb, "Input Cost/1M", infos, func(c ModelCapabilities) string {
		return fmt.Sprintf("$%.2f", c.InputCostPer1M)
	})
	writeComparisonRow(&sb, "Output Cost/1M", infos, func(c ModelCapabilities) string {
		return fmt.Sprintf("$%.2f", c.OutputCostPer1M)
	})

	// OpenAI-specific features
	writeComparisonRow(&sb, "Prompt Caching", infos, func(c ModelCapabilities) string {
		return yesNo(c.SupportsPromptCaching)
	})
	writeComparisonRow(&sb, "Audio", infos, func(c ModelCapabilities) string {
		return yesNo(c.SupportsAudio)
	})
	writeComparisonRow(&sb, "Reasoning Model", infos, func(c ModelCapabilities) string {
		return yesNo(c.IsReasoningModel)
	})

	return sb.String(), nil
}

// Helper functions

func buildFeatureList(caps ModelCapabilities) []string {
	features := []string{}

	if caps.SupportsStreaming {
		features = append(features, "Streaming")
	}
	if caps.SupportsToolCalling {
		features = append(features, "Tool Calling")
	}
	if caps.SupportsVision {
		features = append(features, "Vision")
	}
	if caps.SupportsJSON {
		features = append(features, "JSON Output")
	}

	if caps.SupportsPromptCaching {
		features = append(features, "Prompt Caching")
	}
	if caps.SupportsAudio {
		features = append(features, "Audio")
	}
	if caps.IsReasoningModel {
		features = append(features, "Advanced Reasoning")
	}

	return features
}

func matchesFilter(caps ModelCapabilities, filter CapabilityFilter) bool {
	if filter.Provider != nil && caps.Provider != *filter.Provider {
		return false
	}
	if filter.MinMaxTokens != nil && caps.MaxTokens < *filter.MinMaxTokens {
		return false
	}
	if filter.MaxCostPer1M != nil && caps.OutputCostPer1M > *filter.MaxCostPer1M {
		return false
	}
	if filter.RequireStreaming && !caps.SupportsStreaming {
		return false
	}
	if filter.RequireTools && !caps.SupportsToolCalling {
		return false
	}
	if filter.RequireVision && !caps.SupportsVision {
		return false
	}
	return true
}

func writeComparisonRow(
	sb *strings.Builder,
	label string,
	infos []ModelInfo,
	getValue func(ModelCapabilities) string,
) {
	fmt.Fprintf(sb, "%-20s", label)
	for _, info := range infos {
		fmt.Fprintf(sb, "%-20s",
			truncate(getValue(info.Capabilities), 18),
		)
	}
	sb.WriteString("\n")
}
func yesNo(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ValidateModelConfig validates configuration against model capabilities
type featureCheck struct {
	enabled   bool
	supported bool
	message   string
}

func ValidateModelConfig(
	model string,
	registry map[string]ModelCapabilities,
	maxTokens int,
	temperature float64,
	enableCaching,
	enableStreaming,
	enableTools,
	enablePromptCaching,
	enableAudio,
	enableReasoning,
	enableFunctionCalling bool,
) error {

	caps, err := GetModelCapabilities(model, registry)
	if err != nil {
		return err
	}

	if maxTokens > caps.MaxTokens {
		return fmt.Errorf("max_tokens %d exceeds model limit %d for %s",
			maxTokens, caps.MaxTokens, model)
	}

	if caps.SupportsTemperature {
		if temperature < caps.MinTemperature || temperature > caps.MaxTemperature {
			return fmt.Errorf(
				"temperature %.2f outside valid range [%.2f, %.2f] for %s",
				temperature, caps.MinTemperature, caps.MaxTemperature, model,
			)
		}
	}

	features := []featureCheck{
		{enableStreaming, caps.SupportsStreaming, "streaming"},
		{enableTools, caps.SupportsToolCalling, "tool calling"},
		{enablePromptCaching, caps.SupportsPromptCaching, "prompt caching"},
		{enableAudio, caps.SupportsAudio, "audio"},
		{enableReasoning, caps.IsReasoningModel, "reasoning"},
		{enableFunctionCalling, caps.SupportsFunctionCalling, "function calling"},
	}

	for _, f := range features {
		if f.enabled && !f.supported {
			return fmt.Errorf("%s not supported for model: %s", f.message, model)
		}
	}

	return nil
}
