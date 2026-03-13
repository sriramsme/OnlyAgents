package marketplace

import (
	"context"
	"io"
)

// Marketplace defines the interface for skill discovery services
// (ClawHub, custom registries, etc.)
type Marketplace interface {
	// Name returns the marketplace identifier
	Name() string

	// Search finds skills matching a query
	Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)

	// GetMetadata retrieves detailed metadata for a skill
	GetMetadata(ctx context.Context, slug string) (*SkillMetadata, error)

	// Download downloads a skill file (SKILL.md or package)
	// Returns a reader for the content
	Download(ctx context.Context, slug, version string) (io.ReadCloser, error)
}

// SearchOptions for filtering search results
type SearchOptions struct {
	Limit     int
	SkillName string
	Verified  bool
	MinRating float64
}

// SearchResult represents a search hit
type SearchResult struct {
	Slug         string
	DisplayName  string
	Summary      string
	Version      string
	Score        float64
	Marketplace  string // Which marketplace it's from
	Capabilities []string
	Verified     bool
	Rating       float64
}

// SkillMetadata contains detailed skill information
type SkillMetadata struct {
	Slug             string
	DisplayName      string
	Description      string
	Version          string
	Author           string
	Capabilities     []string
	Tags             []string
	Verified         bool
	Rating           float64
	Downloads        int
	IsMalwareBlocked bool
	IsSuspicious     bool
	Marketplace      string
}
