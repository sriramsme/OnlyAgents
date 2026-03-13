package brave

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

func init() {
	// Register factory with connectors package
	connectors.Register("brave", NewConnector)
}

// Config holds Brave-specific configuration
type Config struct {
	config.ConnectorConfig
	MaxResults int `yaml:"max_results"` // Default max results
}

type Credentials struct {
	APIKey string `yaml:"api_key"` // Vault key
}

// BraveConnector implements WebSearchConnector interface
type BraveConnector struct {
	config *Config
	vault  vault.Vault
	ctx    context.Context
	cancel context.CancelFunc
}

// NewConnector creates a new Brave connector
func NewConnector(
	ctx context.Context,
	cfg config.ConnectorConfig,
	v vault.Vault,
	bus chan<- core.Event,
) (connectors.WebSearchConnector, error) {
	braveCfg := &Config{
		ConnectorConfig: cfg,
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &braveCfg,
		WeaklyTypedInput: true,
		TagName:          "mapstructure",
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("create decoder: %w", err)
	}
	if err := decoder.Decode(cfg.RawConfig); err != nil {
		return nil, fmt.Errorf("decode brave config: %w", err)
	}

	if braveCfg.MaxResults == 0 {
		braveCfg.MaxResults = 5
	}

	connCtx, cancel := context.WithCancel(ctx)

	return &BraveConnector{
		config: braveCfg,
		vault:  v,
		ctx:    connCtx,
		cancel: cancel,
	}, nil
}

// ====================
// Connector Interface
// ====================

func (b *BraveConnector) Name() string                   { return b.config.Name }
func (b *BraveConnector) ID() string                     { return b.config.ID }
func (b *BraveConnector) Type() connectors.ConnectorType { return connectors.ConnectorTypeService }
func (b *BraveConnector) Kind() string                   { return "websearch" }

func (b *BraveConnector) Connect() error {
	// Validate API key exists in vault
	_, err := b.vault.GetSecret(b.ctx, b.config.VaultPaths["api_key"].Path)
	if err != nil {
		return fmt.Errorf("brave API key not found in vault: %w", err)
	}
	return nil
}

func (b *BraveConnector) Disconnect() error {
	return nil
}

func (b *BraveConnector) Start() error {
	return nil
}

func (b *BraveConnector) Stop() error {
	b.cancel()
	return nil
}

func (b *BraveConnector) HealthCheck() error {
	// Check if API key is accessible
	// _, err := b.vault.GetSecret(ctx,b.config.Credentials.APIKey)
	// return err
	return nil
}

// ====================
// WebSearchConnector Interface
// ====================

func (b *BraveConnector) Search(ctx context.Context, req *connectors.SearchRequest) (*connectors.SearchResponse, error) {
	apiKey, err := b.vault.GetSecret(ctx, b.config.VaultPaths["api_key"].Path)
	if err != nil {
		return nil, fmt.Errorf("get API key: %w", err)
	}

	maxResults := req.MaxResults
	if maxResults == 0 {
		maxResults = b.config.MaxResults
	}

	searchURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(req.Query), maxResults)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Subscription-Token", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brave API error (status %d): %s\n %s", resp.StatusCode, string(body), err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var braveResp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.Unmarshal(body, &braveResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	results := make([]connectors.SearchResult, 0, len(braveResp.Web.Results))
	for _, item := range braveResp.Web.Results {
		results = append(results, connectors.SearchResult{
			Title:       item.Title,
			URL:         item.URL,
			Snippet:     item.Description,
			Description: item.Description,
		})
	}

	return &connectors.SearchResponse{
		Query:      req.Query,
		Results:    results,
		TotalCount: len(results),
		Provider:   "brave",
	}, nil
}
