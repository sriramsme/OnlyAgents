package websearch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

const (
	version = "1.0.0"
)

func init() {
	skills.Register("websearch", NewWebSearchSkill)
}

// WebSearchSkill provides web search capabilities
type WebSearchSkill struct {
	ctx      context.Context
	cancel   context.CancelFunc
	eventBus chan<- core.Event
	*skills.BaseSkill

	// Connectors injected by kernel
	searchConns map[string]connectors.WebSearchConnector
}

// NewWebSearchSkill creates a new web search skill
func NewWebSearchSkill(ctx context.Context, eventBus chan<- core.Event) (skills.Skill, error) {
	base := skills.NewBaseSkill(
		tools.SkillWebSearch,
		"Search the web for current information using various search providers",
		version,
		skills.SkillTypeNative,
	)

	skillCtx, cancel := context.WithCancel(ctx)

	return &WebSearchSkill{
		BaseSkill:   base,
		searchConns: make(map[string]connectors.WebSearchConnector),
		ctx:         skillCtx,
		cancel:      cancel,
		eventBus:    eventBus,
	}, nil
}

// Initialize sets up the web search skill with injected connectors
func (s *WebSearchSkill) Initialize(deps skills.SkillDeps) error {
	s.SetOutbox(deps.Outbox)

	// Extract web search connectors from deps.Connectors
	for name, conn := range deps.Connectors {
		if searchConn, ok := conn.(connectors.WebSearchConnector); ok {
			s.searchConns[name] = searchConn
		}
	}

	if len(s.searchConns) == 0 {
		return fmt.Errorf("websearch skill requires at least one web search connector (brave, duckduckgo, perplexity)")
	}

	return nil
}

// Shutdown cleans up resources
func (s *WebSearchSkill) Shutdown() error {
	s.cancel()
	return nil
}

// RequiredCapabilities declares that this skill needs web search connectors
func (s *WebSearchSkill) RequiredCapabilities() []core.Capability {
	return []core.Capability{core.CapabilityWebSearch}
}

func (s *WebSearchSkill) Tools() []tools.ToolDef {
	return tools.GetWebSearchTools()
}

func (s *WebSearchSkill) Execute(ctx context.Context, toolName string, params []byte) (any, error) {
	switch toolName {
	case "websearch_search":
		return s.search(params)
	case "websearch_fetch":
		return s.fetchURL(ctx, params)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (s *WebSearchSkill) search(args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.WebSearchInput](args)
	if err != nil {
		return nil, err
	}

	var searchConn connectors.WebSearchConnector
	var connectorName string
	for name, conn := range s.searchConns {
		searchConn = conn
		connectorName = name
		break
	}
	if searchConn == nil {
		return nil, fmt.Errorf("no web search connector available")
	}

	maxResults := input.MaxResults
	if maxResults == 0 {
		maxResults = 5
	} else if maxResults < 1 {
		maxResults = 1
	} else if maxResults > 10 {
		maxResults = 10
	}

	resp, err := searchConn.Search(s.ctx, &connectors.SearchRequest{
		Query:      input.Query,
		MaxResults: maxResults,
	})
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	results := make([]map[string]any, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = map[string]any{
			"title":   r.Title,
			"url":     r.URL,
			"snippet": r.Snippet,
		}
	}
	return map[string]any{
		"query":     input.Query,
		"results":   results,
		"count":     len(results),
		"provider":  resp.Provider,
		"connector": connectorName,
	}, nil
}

func (s *WebSearchSkill) fetchURL(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.WebSearchFetchInput](args)
	if err != nil {
		return nil, err
	}

	if input.URL == "" {
		return nil, fmt.Errorf("websearch_fetch: url is required")
	}

	maxLength := input.MaxLength
	switch {
	case maxLength <= 0:
		maxLength = 8000
	case maxLength > 32000:
		maxLength = 32000
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, input.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("websearch_fetch: invalid url %q: %w", input.URL, err)
	}
	req.Header.Set("User-Agent", "OnlyAgents/1.0 (+https://github.com/sriramsme/OnlyAgents)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("websearch_fetch: fetch failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Println("websearch_fetch: error closing response body:", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("websearch_fetch: %s returned HTTP %d", input.URL, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/") &&
		!strings.Contains(contentType, "application/xhtml") {
		return nil, fmt.Errorf("websearch_fetch: unsupported content type %q", contentType)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2MB cap
	if err != nil {
		return nil, fmt.Errorf("websearch_fetch: read body: %w", err)
	}

	title, text := extractText(body)

	if len(text) > maxLength {
		text = text[:maxLength]
	}

	return map[string]any{
		"url":            input.URL,
		"title":          title,
		"content":        text,
		"content_length": len(text),
	}, nil
}

// extractText parses HTML and returns the page title and readable text content.
// Uses golang.org/x/net/html — pure Go, no CGO.
func extractText(body []byte) (title, text string) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		// fallback: strip tags naively
		return "", strings.TrimSpace(stripTags(string(body)))
	}

	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.ElementNode:
			switch n.Data {
			case "script", "style", "noscript", "nav", "footer", "header", "aside":
				return // skip noise nodes entirely
			case "title":
				if n.FirstChild != nil {
					title = strings.TrimSpace(n.FirstChild.Data)
				}
			}
		case html.TextNode:
			t := strings.TrimSpace(n.Data)
			if t != "" {
				sb.WriteString(t)
				sb.WriteByte('\n')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return title, strings.TrimSpace(sb.String())
}

func stripTags(s string) string {
	inTag := false
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
