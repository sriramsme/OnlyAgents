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
	openAIDefaultModel    = "text-embedding-3-small"
	openAIDefaultBaseURL  = "https://api.openai.com"
	openAIEmbedDimensions = 1536
)

type openAIEmbedder struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// newOpenAIEmbedder constructs and validates an OpenAI embedder.
// Returns an error immediately if the API key is empty.
func newOpenAIEmbedder(apiKey, model string) (Embedder, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if model == "" {
		model = openAIDefaultModel
	}
	return &openAIEmbedder{
		apiKey:  apiKey,
		model:   model,
		baseURL: openAIDefaultBaseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (e *openAIEmbedder) Provider() string { return "openai" }
func (e *openAIEmbedder) Dimensions() int  { return openAIEmbedDimensions }

func (e *openAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]any{
		"model": e.model,
		"input": text,
	})
	if err != nil {
		return nil, fmt.Errorf("openai embedder: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai embedder: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embedder: request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("openai embedder: close response: %v\n", err)
		}
	}()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai embedder: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embedder: status %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("openai embedder: parse response: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("openai embedder: empty data in response")
	}

	return result.Data[0].Embedding, nil
}
