// pkg/skills/marketplace/manager.go
package marketplace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// Manager orchestrates multiple skill marketplaces
type Manager struct {
	marketplaces []Marketplace
	cache        *SearchCache
	cacheDir     string
	mu           sync.RWMutex
}

// NewManager creates a marketplace manager
func NewManager(cacheDir string) *Manager {
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		logger.Log.Error("failed to create cache dir", "error", err)
	}

	return &Manager{
		marketplaces: make([]Marketplace, 0),
		cache:        NewSearchCache(50, 5*time.Minute),
		cacheDir:     cacheDir,
	}
}

// RegisterMarketplace adds a marketplace to the manager
func (m *Manager) RegisterMarketplace(marketplace Marketplace) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.marketplaces = append(m.marketplaces, marketplace)

	logger.Log.Info("registered skill marketplace",
		"name", marketplace.Name())
}

// Search searches all registered marketplaces
func (m *Manager) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	// Check cache first
	if cached, ok := m.cache.Get(query); ok {
		logger.Log.Debug("marketplace search cache hit", "query", query)
		return cached, nil
	}

	m.mu.RLock()
	marketplaces := m.marketplaces
	m.mu.RUnlock()

	if len(marketplaces) == 0 {
		return nil, fmt.Errorf("no marketplaces registered")
	}

	// Search all marketplaces in parallel
	type result struct {
		results []SearchResult
		err     error
	}

	resultsCh := make(chan result, len(marketplaces))

	for _, mp := range marketplaces {
		go func(marketplace Marketplace) {
			results, err := marketplace.Search(ctx, query, opts)
			resultsCh <- result{results: results, err: err}
		}(mp)
	}

	// Collect results
	var allResults []SearchResult
	for range marketplaces {
		res := <-resultsCh
		if res.err != nil {
			logger.Log.Warn("marketplace search failed",
				"error", res.err)
			continue
		}
		allResults = append(allResults, res.results...)
	}

	if len(allResults) == 0 {
		return nil, fmt.Errorf("no results found for query: %s", query)
	}

	// Sort by score (descending)
	sortByScore(allResults)

	// Cache results
	m.cache.Put(query, allResults)

	return allResults, nil
}

// FindByCapability searches for skills providing a specific capability
func (m *Manager) FindByCapability(ctx context.Context, capability string) ([]SearchResult, error) {
	// Use capability as search query
	opts := SearchOptions{
		Limit:        10,
		Capabilities: []string{capability},
		Verified:     false, // Don't require verified, but prefer them
	}

	return m.Search(ctx, capability, opts)
}

// GetMetadata gets metadata from the appropriate marketplace
func (m *Manager) GetMetadata(ctx context.Context, slug, marketplace string) (*SkillMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, mp := range m.marketplaces {
		if mp.Name() == marketplace {
			return mp.GetMetadata(ctx, slug)
		}
	}

	return nil, fmt.Errorf("marketplace not found: %s", marketplace)
}

// DownloadSkill downloads a skill and returns the local file path
// This is where we download the SKILL.md file to disk
func (m *Manager) DownloadSkill(ctx context.Context, slug, version, marketplace string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Find marketplace
	var mp Marketplace
	for _, m := range m.marketplaces {
		if m.Name() == marketplace {
			mp = m
			break
		}
	}
	if mp == nil {
		return "", fmt.Errorf("marketplace not found: %s", marketplace)
	}

	// Download
	logger.Log.Info("downloading skill from marketplace",
		"slug", slug,
		"version", version,
		"marketplace", marketplace)

	reader, err := mp.Download(ctx, slug, version)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer func() {
		err := reader.Close()
		if err != nil {
			fmt.Printf("error closing reader: %s", err)
		}
	}()

	// Save to cache directory
	filename := fmt.Sprintf(
		"%s_%s_%s.md",
		safeName(marketplace),
		safeName(slug),
		safeName(version),
	)

	fullPath := filepath.Join(m.cacheDir, filename)
	fullPath = filepath.Clean(fullPath)

	if !strings.HasPrefix(fullPath, filepath.Clean(m.cacheDir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid cache path")
	}

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			fmt.Printf("error closing file: %s", err)
		}
	}()

	if _, err := io.Copy(file, reader); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	logger.Log.Info("skill downloaded",
		"slug", slug,
		"path", fullPath)

	return fullPath, nil
}

func safeName(s string) string {
	s = filepath.Base(s) // remove path traversal
	s = strings.ReplaceAll(s, "..", "")
	return s
}

// sortByScore sorts results by score descending
func sortByScore(results []SearchResult) {
	slices.SortFunc(results, func(a, b SearchResult) int {
		switch {
		case a.Score > b.Score:
			return -1 // descending
		case a.Score < b.Score:
			return 1
		default:
			return 0
		}
	})
}
