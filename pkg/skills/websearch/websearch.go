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
