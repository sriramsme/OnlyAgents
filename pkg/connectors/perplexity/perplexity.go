package perplexity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
)

const (
	// version   = "1.0.0"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
)

// Config holds Perplexity-specific configuration
type Config struct {
	maxResults  int
	model       string
	credentials Credentials
}

type Credentials struct {
	APIKey string
}

// PerplexityConnector implements WebSearchConnector interface
type PerplexityConnector struct {
	config *Config

	*connectors.BaseConnector

	ctx    context.Context
	cancel context.CancelFunc
}

// NewConnector creates a new Perplexity connector
func New(ctx context.Context, cfg Config) (*PerplexityConnector, error) {
	if cfg.maxResults == 0 {
		cfg.maxResults = 5
	}

	if cfg.model == "" {
		cfg.model = "sonar"
	}

	connCtx, cancel := context.WithCancel(ctx)

	return &PerplexityConnector{
		BaseConnector: connectors.NewBaseConnector(connectors.BaseConnectorInfo{
			ID:           "perplexity",
			Name:         "perplexity",
			Description:  "Perplexity AI web search connector",
			Instructions: "Search the web using Perplexity AI",
			Type:         "websearch",
			Enabled:      true,
		}),

		config: &cfg,
		ctx:    connCtx,
		cancel: cancel,
	}, nil
}

func init() {
	connectors.Register("perplexity", func(
		ctx context.Context,
		cfg config.Connector,
	) (connectors.Connector, error) {
		v, err := vault.Load()
		if err != nil {
			return nil, fmt.Errorf("perplexity: vault: %w", err)
		}

		ppCfg := Config{
			maxResults: 5,
			model:      "sonar",
		}
		ppCfg.credentials.APIKey, err = v.GetSecret(ctx, cfg.VaultPaths["api_key"].Path)
		if err != nil {
			return nil, fmt.Errorf("perplexity: get api_key: %w", err)
		}

		if v, ok := cfg.RawConfig["max_results"].(int); ok {
			ppCfg.maxResults = v
		}

		if v, ok := cfg.RawConfig["model"].(string); ok {
			ppCfg.model = v
		}

		conn, err := New(ctx, ppCfg)
		if err != nil {
			return nil, err
		}

		// override metadata from config
		conn.BaseConnector = connectors.NewBaseConnectorFromConfig(cfg)

		return conn, nil
	})
}

// ====================
// Connector Interface
// ====================

func (p *PerplexityConnector) Kind() string { return "websearch" }

func (p *PerplexityConnector) Connect() error {
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
	maxResults := req.MaxResults
	if maxResults == 0 {
		maxResults = p.config.maxResults
	}

	searchURL := "https://api.perplexity.ai/chat/completions"

	payload := map[string]any{
		"model": p.config.model,
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
	httpReq.Header.Set("Authorization", "Bearer "+p.config.credentials.APIKey)
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
