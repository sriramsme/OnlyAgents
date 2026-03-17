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
)

// Config holds Brave-specific configuration
type Config struct {
	APIKey     string `json:"api_key"              desc:"Brave Search API key" cli_req:"true"`
	MaxResults int    `json:"max_results,omitempty" desc:"Max results to return"`
}

// BraveConnector implements WebSearchConnector interface
type BraveConnector struct {
	ctx    context.Context
	cancel context.CancelFunc

	*connectors.BaseConnector

	config *Config
}

// NewConnector creates a new Brave connector
func New(
	ctx context.Context,
	cfg Config,
) (*BraveConnector, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("brave: missing api key")
	}

	if cfg.MaxResults == 0 {
		cfg.MaxResults = 5
	}

	connCtx, cancel := context.WithCancel(ctx)

	return &BraveConnector{
		BaseConnector: connectors.NewBaseConnector(connectors.BaseConnectorInfo{
			ID:           "brave",
			Name:         "brave",
			Description:  "Brave search connector",
			Instructions: "Search the web using Brave Search API",
			Type:         "websearch",
			Enabled:      true,
		}),

		config: &cfg,
		ctx:    connCtx,
		cancel: cancel,
	}, nil
}

func init() {
	connectors.Register("brave", func(
		ctx context.Context,
		cfg config.Connector,
	) (connectors.Connector, error) {
		var braveCfg Config

		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:           &braveCfg,
			WeaklyTypedInput: true,
			TagName:          "mapstructure",
		})
		if err != nil {
			return nil, fmt.Errorf("create decoder: %w", err)
		}

		if err := decoder.Decode(cfg.RawConfig); err != nil {
			return nil, fmt.Errorf("decode brave config: %w", err)
		}

		v, err := vault.Load()
		if err != nil {
			return nil, fmt.Errorf("brave: vault: %w", err)
		}

		braveCfg.APIKey, err = v.GetSecret(ctx, cfg.VaultPaths["api_key"].Path)
		if err != nil {
			return nil, fmt.Errorf("brave: get api_key: %w", err)
		}

		conn, err := New(
			ctx,
			braveCfg,
		)
		if err != nil {
			return nil, err
		}

		// override base connector info from config
		conn.BaseConnector = connectors.NewBaseConnectorFromConfig(cfg)

		return conn, nil
	})
}

// ====================
// Connector Interface
// ====================
func (b *BraveConnector) Kind() string { return "websearch" }

func (b *BraveConnector) Connect() error {
	return nil
}

func (b *BraveConnector) Disconnect() error {
	b.cancel()
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
	APIKey := b.config.APIKey

	MaxResults := req.MaxResults
	if MaxResults == 0 {
		MaxResults = b.config.MaxResults
	}

	searchURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(req.Query), MaxResults)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Subscription-Token", APIKey)

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
