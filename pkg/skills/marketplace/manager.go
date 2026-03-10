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

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	skillcli "github.com/sriramsme/OnlyAgents/pkg/skills/cli"
)

// Manager orchestrates multiple skill marketplaces.
type Manager struct {
	marketplaces []Marketplace
	cache        *SearchCache
	cacheDir     string // raw downloads land here (untrusted)
	installDir   string // converted + validated skills land here
	mu           sync.RWMutex
}

// NewManager creates a marketplace manager.
//
//   - cacheDir  – where raw downloaded files are written (untrusted scratch space)
//   - installDir – where sanitised SKILL.md files are written (loaded by kernel)
func NewManager(cacheDir, installDir string) *Manager {
	for _, d := range []string{cacheDir, installDir} {
		if err := os.MkdirAll(d, 0700); err != nil {
			logger.Log.Error("failed to create directory", "dir", d, "error", err)
		}
	}

	return &Manager{
		marketplaces: make([]Marketplace, 0),
		cache:        NewSearchCache(50, 5*time.Minute),
		cacheDir:     cacheDir,
		installDir:   installDir,
	}
}

// RegisterMarketplace adds a marketplace to the manager.
func (m *Manager) RegisterMarketplace(marketplace Marketplace) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.marketplaces = append(m.marketplaces, marketplace)
	logger.Log.Info("registered skill marketplace", "name", marketplace.Name())
}

// Search searches all registered marketplaces (cached).
func (m *Manager) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
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

	var allResults []SearchResult
	for range marketplaces {
		res := <-resultsCh
		if res.err != nil {
			logger.Log.Warn("marketplace search failed", "error", res.err)
			continue
		}
		allResults = append(allResults, res.results...)
	}

	if len(allResults) == 0 {
		return nil, fmt.Errorf("no results found for query: %s", query)
	}

	sortByScore(allResults)
	m.cache.Put(query, allResults)
	return allResults, nil
}

// FindByCapability searches for skills providing a specific capability.
func (m *Manager) FindByCapability(ctx context.Context, capability string) ([]SearchResult, error) {
	return m.Search(ctx, capability, SearchOptions{
		Limit:        10,
		Capabilities: []string{capability},
	})
}

// GetMetadata fetches metadata from the named marketplace.
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

// DownloadAndInstall downloads a skill, converts it with an LLM, validates it,
// and writes the sanitised SKILL.md to the install directory.
//
// It returns the path of the installed file.
func (m *Manager) DownloadAndInstall(
	ctx context.Context,
	slug, version, marketplace string,
	client llm.Client,
) (string, error) {
	// ── 1. Download raw file to cache dir ─────────────────────────────────────
	rawPath, err := m.downloadRaw(ctx, slug, version, marketplace)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	logger.Log.Info("raw skill cached", "path", rawPath)

	// ── 2. Read raw content ───────────────────────────────────────────────────
	raw, err := os.ReadFile(rawPath) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("read cached file: %w", err)
	}

	// ── 3. Convert + validate ─────────────────────────────────────────────────
	logger.Log.Info("converting skill via LLM",
		"slug", slug, "marketplace", marketplace)

	result, err := skillcli.ConvertSKILL(ctx, client, string(raw), skillcli.ConvertOptions{
		SkillName: slug,
	})
	if err != nil {
		return "", fmt.Errorf("convert skill %q: %w", slug, err)
	}

	// ── 4. Write to install dir ───────────────────────────────────────────────
	installPath := m.installPath(slug, version)
	if err := os.WriteFile(installPath, []byte(result.Content), 0600); err != nil { // nolint:gosec
		return "", fmt.Errorf("write install file: %w", err)
	}

	logger.Log.Info("skill installed",
		"slug", slug,
		"tools", len(result.Parsed.Commands),
		"path", installPath)

	return installPath, nil
}

func (m *Manager) downloadRaw(ctx context.Context, slug, version, marketplace string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var mp Marketplace
	for _, candidate := range m.marketplaces {
		if candidate.Name() == marketplace {
			mp = candidate
			break
		}
	}
	if mp == nil {
		return "", fmt.Errorf("marketplace not found: %s", marketplace)
	}

	reader, err := mp.Download(ctx, slug, version)
	if err != nil {
		return "", fmt.Errorf("marketplace download: %w", err)
	}
	defer reader.Close() //nolint:errcheck

	filename := fmt.Sprintf("%s_%s_%s_raw.md", safeName(marketplace), safeName(slug), safeName(version))
	fullPath := m.safePath(m.cacheDir, filename)
	if fullPath == "" {
		return "", fmt.Errorf("invalid cache path")
	}

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("create cache file: %w", err)
	}
	defer file.Close() //nolint:errcheck

	if _, err := io.Copy(file, reader); err != nil {
		return "", fmt.Errorf("write cache file: %w", err)
	}

	return fullPath, nil
}

func (m *Manager) installPath(slug, version string) string {
	safeSlug := filepath.Base(slug)
	safeVersion := filepath.Base(version)

	installPath := filepath.Join(m.installDir, safeSlug+"-"+safeVersion+".skill")
	installPath = filepath.Clean(installPath)
	return installPath
}

// safePath joins dir + filename and verifies the result stays inside dir.
func (m *Manager) safePath(dir, filename string) string {
	full := filepath.Clean(filepath.Join(dir, filename))
	if !strings.HasPrefix(full, filepath.Clean(dir)+string(os.PathSeparator)) {
		return ""
	}
	return full
}

func safeName(s string) string {
	s = filepath.Base(s)
	s = strings.ReplaceAll(s, "..", "")
	return s
}

func sortByScore(results []SearchResult) {
	slices.SortFunc(results, func(a, b SearchResult) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		default:
			return 0
		}
	})
}
