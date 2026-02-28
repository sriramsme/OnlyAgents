// pkg/skills/marketplace/clawhub.go
package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ClawHubMarketplace implements Marketplace for ClawHub
type ClawHubMarketplace struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// NewClawHubMarketplace creates a ClawHub marketplace client
func NewClawHubMarketplace(baseURL, authToken string) *ClawHubMarketplace {
	if baseURL == "" {
		baseURL = "https://clawhub.ai"
	}

	return &ClawHubMarketplace{
		baseURL:   baseURL,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *ClawHubMarketplace) Name() string {
	return "clawhub"
}

func (c *ClawHubMarketplace) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/search")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", query)
	if opts.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	u.RawQuery = q.Encode()

	body, err := c.doGet(ctx, u.String())
	if err != nil {
		return nil, err
	}

	var resp struct {
		Results []struct {
			Score       float64 `json:"score"`
			Slug        *string `json:"slug"`
			DisplayName *string `json:"displayName"`
			Summary     *string `json:"summary"`
			Version     *string `json:"version"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	results := make([]SearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		if r.Slug == nil {
			continue
		}

		results = append(results, SearchResult{
			Slug:        *r.Slug,
			DisplayName: derefString(r.DisplayName, *r.Slug),
			Summary:     derefString(r.Summary, ""),
			Version:     derefString(r.Version, "latest"),
			Score:       r.Score,
			Marketplace: c.Name(),
		})
	}

	return results, nil
}

func (c *ClawHubMarketplace) GetMetadata(ctx context.Context, slug string) (*SkillMetadata, error) {
	u := fmt.Sprintf("%s/api/v1/skills/%s", c.baseURL, url.PathEscape(slug))

	body, err := c.doGet(ctx, u)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Slug          string `json:"slug"`
		DisplayName   string `json:"displayName"`
		Summary       string `json:"summary"`
		LatestVersion *struct {
			Version string `json:"version"`
		} `json:"latestVersion"`
		Moderation *struct {
			IsMalwareBlocked bool `json:"isMalwareBlocked"`
			IsSuspicious     bool `json:"isSuspicious"`
		} `json:"moderation"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	meta := &SkillMetadata{
		Slug:        resp.Slug,
		DisplayName: resp.DisplayName,
		Description: resp.Summary,
		Marketplace: c.Name(),
	}

	if resp.LatestVersion != nil {
		meta.Version = resp.LatestVersion.Version
	}
	if resp.Moderation != nil {
		meta.IsMalwareBlocked = resp.Moderation.IsMalwareBlocked
		meta.IsSuspicious = resp.Moderation.IsSuspicious
	}

	return meta, nil
}

func (c *ClawHubMarketplace) Download(ctx context.Context, slug, version string) (io.ReadCloser, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/download")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("slug", slug)
	if version != "" && version != "latest" {
		q.Set("version", version)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		err := resp.Body.Close()
		return nil, fmt.Errorf("download failed: HTTP %d %s", resp.StatusCode, err)
	}

	return resp.Body, nil
}

func (c *ClawHubMarketplace) doGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			fmt.Printf("error closing response body: %s", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func derefString(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}
