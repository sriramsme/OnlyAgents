package llm

import (
	"fmt"
)

// registry holds all registered providers
var registry = make(map[Provider]*ProviderRegistration)

// RegisterProvider registers a new provider
func RegisterProvider(provider Provider, reg ProviderRegistration) {
	registry[provider] = &reg
}

// SupportedProviders returns all registered provider names
func SupportedProviders() []Provider {
	providers := make([]Provider, 0, len(registry))
	for p := range registry {
		providers = append(providers, p)
	}
	return providers
}

// SupportedModels returns models supported by a provider
func SupportedModels(provider Provider) []string {
	if reg, ok := registry[provider]; ok {
		return reg.Models
	}
	return nil
}

// ValidateProviderModel checks if a model is valid for a provider
func ValidateProviderModel(provider Provider, model string) error {
	reg, ok := registry[provider]
	if !ok {
		return fmt.Errorf("unsupported provider: %s", provider)
	}

	if len(reg.Models) == 0 {
		return nil // No validation if models list is empty
	}

	for _, m := range reg.Models {
		if m == model {
			return nil
		}
	}

	return fmt.Errorf("model %s not supported by provider %s", model, provider)
}

func GetProviderEnvKey(provider Provider) (string, bool) {
	reg, ok := registry[provider]
	return reg.EnvKey, ok
}
