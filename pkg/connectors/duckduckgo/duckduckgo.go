package duckduckgo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

const (
	version   = "1.0.0"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

func init() {
	connectors.Register("duckduckgo", NewConnector)
}

// Config holds DuckDuckGo-specific configuration
type Config struct {
	Platform   string `yaml:"platform"` // "duckduckgo"
	Enabled    bool   `yaml:"enabled"`
	MaxResults int    `yaml:"max_results"` // Default max results
}

// DuckDuckGoConnector implements WebSearchConnector interface
type DuckDuckGoConnector struct {
	config *Config
	name   string
	ctx    context.Context
	cancel context.CancelFunc
}

// NewConnector creates a new DuckDuckGo connector
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
		return nil, fmt.Errorf("decode duckduckgo config: %w", err)
	}

	// Extract name from rawConfig if present
	name := "duckduckgo"
	if n, ok := rawConfig["name"].(string); ok {
		name = n
	}

	if cfg.MaxResults == 0 {
		cfg.MaxResults = 5
	}

	connCtx, cancel := context.WithCancel(ctx)

	return &DuckDuckGoConnector{
		config: &cfg,
		name:   name,
		ctx:    connCtx,
		cancel: cancel,
	}, nil
}

// ====================
// Connector Interface
// ====================

func (d *DuckDuckGoConnector) Name() string { return d.name }
func (d *DuckDuckGoConnector) Type() string { return "duckduckgo" }

func (d *DuckDuckGoConnector) Connect() error {
	return nil // No authentication needed
}

func (d *DuckDuckGoConnector) Disconnect() error {
	return nil
}

func (d *DuckDuckGoConnector) Start() error {
	return nil
}

func (d *DuckDuckGoConnector) Stop() error {
	d.cancel()
	return nil
}

func (d *DuckDuckGoConnector) HealthCheck() error {
	// Simple health check - try to reach DDG
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://html.duckduckgo.com/html/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("error closing response body: %v", err)
		}
	}()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("DDG service unavailable: %d", resp.StatusCode)
	}

	return nil
}

// ====================
// WebSearchConnector Interface
// ====================

func (d *DuckDuckGoConnector) Search(ctx context.Context, req *connectors.SearchRequest) (*connectors.SearchResponse, error) {
	maxResults := req.MaxResults
	if maxResults == 0 {
		maxResults = d.config.MaxResults
	}

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(req.Query))

	httpReq, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("User-Agent", userAgent)

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	results, err := d.extractResults(string(body), maxResults, req.Query)
	if err != nil {
		return nil, err
	}

	return &connectors.SearchResponse{
		Query:      req.Query,
		Results:    results,
		TotalCount: len(results),
		Provider:   "duckduckgo",
	}, nil
}

func (d *DuckDuckGoConnector) extractResults(html string, maxResults int, query string) ([]connectors.SearchResult, error) {
	// Extract result links
	reLink := regexp.MustCompile(`<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	linkMatches := reLink.FindAllStringSubmatch(html, maxResults+5)

	if len(linkMatches) == 0 {
		return []connectors.SearchResult{}, nil
	}

	// Extract snippets
	reSnippet := regexp.MustCompile(`<a class="result__snippet[^"]*".*?>([\s\S]*?)</a>`)
	snippetMatches := reSnippet.FindAllStringSubmatch(html, maxResults+5)

	results := make([]connectors.SearchResult, 0, maxResults)
	maxItems := min(len(linkMatches), maxResults)

	for i := 0; i < maxItems; i++ {
		urlStr := linkMatches[i][1]
		title := stripTags(linkMatches[i][2])
		title = strings.TrimSpace(title)

		// URL decoding if needed
		if strings.Contains(urlStr, "uddg=") {
			if u, err := url.QueryUnescape(urlStr); err == nil {
				idx := strings.Index(u, "uddg=")
				if idx != -1 {
					urlStr = u[idx+5:]
				}
			}
		}

		result := connectors.SearchResult{
			Title: title,
			URL:   urlStr,
		}

		// Attach snippet if available
		if i < len(snippetMatches) {
			snippet := stripTags(snippetMatches[i][1])
			snippet = strings.TrimSpace(snippet)
			result.Snippet = snippet
			result.Description = snippet
		}

		results = append(results, result)
	}

	return results, nil
}

func stripTags(content string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	return re.ReplaceAllString(content, "")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
