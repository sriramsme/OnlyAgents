package websearch

import (
	"context"
	"fmt"

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
		"websearch",
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

// Tools returns the LLM function calling tools for web search
func (s *WebSearchSkill) Tools() []tools.ToolDef {
	return []tools.ToolDef{
		tools.NewToolDef(
			"websearch_search",
			"Search the web for current information. Returns titles, URLs, and snippets from search results.",
			tools.BuildParams(
				map[string]tools.Property{
					"query": tools.StringProp("Search query (what to search for)"),
					"max_results": tools.Property{
						Type:        "integer",
						Description: "Number of results to return (1-10, default: 5)",
						Default:     5,
					},
				},
				[]string{"query"},
			),
		),
	}
}

// Execute runs a tool
func (s *WebSearchSkill) Execute(ctx context.Context, toolName string, params map[string]any) (any, error) {
	switch toolName {
	case "websearch_search":
		return s.search(params)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// ====================
// Tool Implementations
// ====================

func (s *WebSearchSkill) search(params map[string]any) (any, error) {
	// Use first available search connector
	var searchConn connectors.WebSearchConnector
	var connectorName string
	for name, conn := range s.searchConns {
		searchConn = conn
		connectorName = name
		break
	}

	query, ok := params["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query parameter is required")
	}

	maxResults := 5
	if mr, ok := params["max_results"].(float64); ok {
		maxResults = int(mr)
		if maxResults < 1 {
			maxResults = 1
		}
		if maxResults > 10 {
			maxResults = 10
		}
	}

	req := &connectors.SearchRequest{
		Query:      query,
		MaxResults: maxResults,
	}

	// Use skill's context for the search operation
	resp, err := searchConn.Search(s.ctx, req)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Format results for LLM
	results := make([]map[string]any, len(resp.Results))
	for i, result := range resp.Results {
		results[i] = map[string]any{
			"title":   result.Title,
			"url":     result.URL,
			"snippet": result.Snippet,
		}
	}

	return map[string]any{
		"query":     query,
		"results":   results,
		"count":     len(results),
		"provider":  resp.Provider,
		"connector": connectorName,
	}, nil
}
