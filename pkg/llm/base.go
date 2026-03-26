package llm

import (
	"fmt"
	"time"
)

type BaseClient struct {
	capabilities ModelCapabilities

	// Resolved configuration
	maxTokens     int
	temperature   float64
	enableCaching bool
	cacheKey      string
}

func NewBaseClient(caps ModelCapabilities, cfg ProviderConfig) (BaseClient, error) {
	maxTokens := caps.DefaultMaxTokens
	temperature := caps.DefaultTemperature

	opts := cfg.Options
	if opts != nil {
		if opts.MaxTokens > 0 {
			maxTokens = opts.MaxTokens
		}
		if opts.Temperature > 0 {
			temperature = opts.Temperature
		}
	}

	if maxTokens > caps.MaxTokens {
		return BaseClient{}, fmt.Errorf("max_tokens %d exceeds model limit %d", maxTokens, caps.MaxTokens)
	}

	if caps.SupportsTemperature {
		if temperature < caps.MinTemperature || temperature > caps.MaxTemperature {
			return BaseClient{}, fmt.Errorf(
				"temperature %.2f outside valid range [%.2f, %.2f]",
				temperature, caps.MinTemperature, caps.MaxTemperature,
			)
		}
	}

	return BaseClient{
		capabilities:  caps,
		maxTokens:     maxTokens,
		temperature:   temperature,
		enableCaching: caps.SupportsPromptCaching,
		cacheKey:      fmt.Sprintf("agent-%s-%d", cfg.Model, time.Now().Unix()/3600),
	}, nil
}

// SetCaching controls prompt caching
func (c *BaseClient) SetCaching(enabled bool) {
	if c.capabilities.SupportsPromptCaching {
		c.enableCaching = enabled
	}
}

// SetCacheKey sets a custom cache key
func (c *BaseClient) SetCacheKey(key string) {
	c.cacheKey = key
}

func (c *BaseClient) Capabilities() ModelCapabilities {
	return c.capabilities
}

func (c *BaseClient) CachingEnabled() bool {
	return c.enableCaching
}

func (c *BaseClient) CacheKey() string {
	return c.cacheKey
}

func (c *BaseClient) MaxTokens() int {
	return c.maxTokens
}

func (c *BaseClient) Temperature() float64 {
	return c.temperature
}

func (c *BaseClient) ContextWindow() int {
	return c.capabilities.ContextWindow
}
