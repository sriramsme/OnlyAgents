package perplexity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

const (
	// version   = "1.0.0"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
)

func init() {
	connectors.Register("perplexity", NewConnector)
}

// Config holds Perplexity-specific configuration
type Config struct {
	Platform   string `yaml:"platform"` // "perplexity"
	Enabled    bool   `yaml:"enabled"`
	MaxResults int    `yaml:"max_results"` // Default max results

	Credentials Credentials `yaml:"credentials"`
	Model       string      `yaml:"model"` // Default: "sonar"
}

type Credentials struct {
	APIKey string `yaml:"api_key"` // Vault key
}

// PerplexityConnector implements WebSearchConnector interface
type PerplexityConnector struct {
	config *Config
	vault  vault.Vault
	name   string
	ctx    context.Context
	cancel context.CancelFunc
}

// NewConnector creates a new Perplexity connector
func NewConnector(
	ctx context.Context,
	rawConfig map[string]interface{},
	v vault.Vault,
	bus chan<- core.Event,
) (connectors.Connector, error) {
	var cfg Config

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &cfg,
		WeaklyTypedInput: true,
		TagName:          "yaml",
	})
	if err != nil {
		return nil, fmt.Errorf("create decoder: %w", err)
	}

	if err := decoder.Decode(rawConfig); err != nil {
		return nil, fmt.Errorf("decode perplexity config: %w", err)
	}

	// Extract name from rawConfig if present
	name := "perplexity"
	if n, ok := rawConfig["name"].(string); ok {
		name = n
	}

	if cfg.MaxResults == 0 {
		cfg.MaxResults = 5
	}

	if cfg.Model == "" {
		cfg.Model = "sonar"
	}

	connCtx, cancel := context.WithCancel(ctx)

	return &PerplexityConnector{
		config: &cfg,
		vault:  v,
		name:   name,
		ctx:    connCtx,
		cancel: cancel,
	}, nil
}

// ====================
// Connector Interface
// ====================

func (p *PerplexityConnector) Name() string { return p.name }
func (p *PerplexityConnector) Type() string { return "perplexity" }

func (p *PerplexityConnector) Connect() error {
	// Validate API key exists in vault
	_, err := p.vault.GetSecret(p.ctx, p.config.Credentials.APIKey)
	if err != nil {
		return fmt.Errorf("perplexity API key not found in vault: %w", err)
	}
	return nil
}

func (p *PerplexityConnector) Disconnect() error {
	return nil
}

func (p *PerplexityConnector) Start() error {
	return nil
}

func (p *PerplexityConnector) Stop() error {
	p.cancel()
	return nil
}

func (p *PerplexityConnector) HealthCheck() error {
	// Check if API key is accessible
	// _, err := p.vault.GetSecret(p.config.Credentials.APIKey)
	// return err
	return nil
}

// ====================
// WebSearchConnector Interface
// ====================

func (p *PerplexityConnector) Search(ctx context.Context, req *connectors.SearchRequest) (*connectors.SearchResponse, error) {
	apiKey, err := p.vault.GetSecret(ctx, p.config.Credentials.APIKey)
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}

	maxResults := req.MaxResults
	if maxResults == 0 {
		maxResults = p.config.MaxResults
	}

	searchURL := "https://api.perplexity.ai/chat/completions"

	payload := map[string]any{
		"model": p.config.Model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are a search assistant. Provide concise search results with titles, URLs, and brief descriptions in the following format:\n1. Title\n   URL\n   Description\n\nDo not add extra commentary.",
			},
			{
				"role":    "user",
				"content": fmt.Sprintf("Search for: %s. Provide up to %d relevant results.", req.Query, maxResults),
			},
		},
		"max_tokens": 1000,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", searchURL, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("error closing response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("perplexity API error (status %d): %s", resp.StatusCode, string(body))
	}

	var perplexityResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &perplexityResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(perplexityResp.Choices) == 0 {
		return &connectors.SearchResponse{
			Query:      req.Query,
			Results:    []connectors.SearchResult{},
			TotalCount: 0,
			Provider:   "perplexity",
		}, nil
	}

	// Perplexity returns formatted text, we wrap it as a single result
	// In a production system, you might parse this into structured results
	content := perplexityResp.Choices[0].Message.Content

	results := []connectors.SearchResult{
		{
			Title:       fmt.Sprintf("Search results for: %s", req.Query),
			Snippet:     content,
			Description: content,
		},
	}

	return &connectors.SearchResponse{
		Query:      req.Query,
		Results:    results,
		TotalCount: len(results),
		Provider:   "perplexity",
	}, nil
}
