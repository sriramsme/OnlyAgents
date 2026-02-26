package gemini

import (
	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

// GoogleExtensions contains Google-specific capabilities
type GoogleExtensions struct {
	SupportsVideoInput      bool
	SupportsPDFInput        bool
	SupportsCodeExecution   bool
	SupportsComputerUse     bool
	SupportsFileSearch      bool
	SupportsSearchGrounding bool
	SupportsMapsGrounding   bool
	SupportsURLContext      bool
	SupportsBatchAPI        bool
}

// ModelRegistry maps model names to their capabilities
var ModelRegistry = map[string]llm.ModelCapabilities{
	"gemini-2.5-flash": {
		Available:               true,
		Provider:                llm.ProviderGemini,
		MaxTokens:               65536,
		DefaultMaxTokens:        32768,
		ContextWindow:           1048576,
		SupportsTemperature:     true,
		MinTemperature:          0.0,
		MaxTemperature:          2.0,
		DefaultTemperature:      0.7,
		SupportsStreaming:       true,
		SupportsToolCalling:     true,
		SupportsVision:          true,
		SupportsJSON:            true,
		InputCostPer1M:          0.30,
		OutputCostPer1M:         2.50,
		Description:             "Hybrid reasoning multimodal model with 1M context window and thinking budgets",
		ReleaseDate:             "2025-06",
		Deprecated:              false,
		SupportsPromptCaching:   true,
		SupportsAudio:           true,
		SupportsReasoningEffort: true,
		IsReasoningModel:        true,
		SupportsFunctionCalling: true,

		Extensions: GoogleExtensions{
			SupportsVideoInput:      true,
			SupportsCodeExecution:   true,
			SupportsFileSearch:      true,
			SupportsSearchGrounding: true,
			SupportsMapsGrounding:   true,
			SupportsURLContext:      true,
			SupportsBatchAPI:        true,
		},
	},

	"gemini-3-flash-preview": {
		Available:               true,
		Provider:                llm.ProviderGemini,
		MaxTokens:               65536,
		DefaultMaxTokens:        32768,
		ContextWindow:           1048576,
		SupportsTemperature:     true,
		MinTemperature:          0.0,
		MaxTemperature:          2.0,
		DefaultTemperature:      0.7,
		SupportsStreaming:       true,
		SupportsToolCalling:     true,
		SupportsVision:          true,
		SupportsJSON:            true,
		InputCostPer1M:          0.50,
		OutputCostPer1M:         3.00,
		Description:             "Preview Gemini 3 Flash model combining frontier reasoning, multimodal input, and search grounding optimized for speed",
		ReleaseDate:             "2025-12",
		Deprecated:              false,
		SupportsPromptCaching:   true,
		SupportsAudio:           true,
		SupportsReasoningEffort: true,
		IsReasoningModel:        true,
		SupportsFunctionCalling: true,

		Extensions: GoogleExtensions{
			SupportsVideoInput:      true,
			SupportsPDFInput:        true,
			SupportsCodeExecution:   true,
			SupportsComputerUse:     true,
			SupportsFileSearch:      true,
			SupportsSearchGrounding: true,
			SupportsMapsGrounding:   false,
			SupportsURLContext:      true,
			SupportsBatchAPI:        true,
		},
	},

	"gemini-3-pro-preview": {
		Available:               true,
		Provider:                llm.ProviderGemini,
		MaxTokens:               65536,
		DefaultMaxTokens:        32768,
		ContextWindow:           1048576,
		SupportsTemperature:     true,
		MinTemperature:          0.0,
		MaxTemperature:          2.0,
		DefaultTemperature:      0.7,
		SupportsStreaming:       true,
		SupportsToolCalling:     true,
		SupportsVision:          true,
		SupportsJSON:            true,
		InputCostPer1M:          2.00,
		OutputCostPer1M:         12.00,
		Description:             "Preview Gemini 3 Pro multimodal reasoning model optimized for advanced agentic and coding workflows",
		ReleaseDate:             "2025-11",
		Deprecated:              false,
		SupportsPromptCaching:   true,
		SupportsAudio:           true,
		SupportsReasoningEffort: true,
		IsReasoningModel:        true,
		SupportsFunctionCalling: true,

		Extensions: GoogleExtensions{
			SupportsVideoInput:      true,
			SupportsPDFInput:        true,
			SupportsCodeExecution:   true,
			SupportsComputerUse:     false,
			SupportsFileSearch:      true,
			SupportsSearchGrounding: true,
			SupportsMapsGrounding:   false,
			SupportsURLContext:      true,
			SupportsBatchAPI:        true,
		},
	},
}
