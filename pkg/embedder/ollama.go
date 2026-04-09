package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	ollamaDefaultBaseURL  = "http://localhost:11434"
	ollamaDefaultModel    = "nomic-embed-text"
	ollamaEmbedDimensions = 768
)

type ollamaEmbedder struct {
	model   string
	baseURL string
	client  *http.Client
}

// newOllamaEmbedder constructs an Ollama embedder and performs a
// hard-fail connectivity check so misconfiguration is caught at init.
func newOllamaEmbedder(baseURL, model string) (Embedder, error) {
	if baseURL == "" {
		baseURL = ollamaDefaultBaseURL
	}
	if model == "" {
		model = ollamaDefaultModel
	}

	e := &ollamaEmbedder{
		model:   model,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}

	if err := e.ping(); err != nil {
		return nil, fmt.Errorf("ollama embedder: cannot reach Ollama at %s: %w (is Ollama running?)", baseURL, err)
	}

	return e, nil
}

func (e *ollamaEmbedder) Provider() string { return "ollama" }
func (e *ollamaEmbedder) Dimensions() int  { return ollamaEmbedDimensions }

func (e *ollamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]any{
		"model":  e.model,
		"prompt": text,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("ollama embedder: close response: %v\n", err)
		}
	}()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embedder: status %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("ollama embedder: parse response: %w", err)
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama embedder: empty embedding in response")
	}

	return result.Embedding, nil
}

// ping hits GET /api/tags — Ollama's lightest health endpoint.
func (e *ollamaEmbedder) ping() error {
	resp, err := e.client.Get(e.baseURL + "/api/tags")
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("ollama embedder: close response: %v\n", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
