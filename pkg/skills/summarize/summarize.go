package summarize

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

const (
	maxChunkChars         = 12000 // ~3000 tokens in practice, works across all models
	intermediateMaxTokens = 250
	mergeBatchSize        = 5
)

type SummarizeSkill struct {
	ctx    context.Context
	cancel context.CancelFunc
	*skills.BaseSkill
	llmClient  llm.Client
	llmOptions llm.Options
}

// external path — defaults baked in
func New(ctx context.Context, client llm.Client, opts llm.Options) (*SummarizeSkill, error) {
	if client == nil {
		return nil, fmt.Errorf("summarize: llm client required")
	}

	skillCtx, cancel := context.WithCancel(ctx)

	return &SummarizeSkill{
		BaseSkill: skills.NewBaseSkill(skills.BaseSkillInfo{
			Name:        "summarize",
			Description: "Summarizes text using an LLM",
			Version:     "1.0.0",
			Enabled:     true,
			AccessLevel: "read",
			Tools:       tools.GetSummarizeTools(),
			Groups:      tools.GetSummarizeGroups(),
		}, skills.SkillTypeNative),
		llmClient:  client,
		llmOptions: opts,
		ctx:        skillCtx,
		cancel:     cancel,
	}, nil
}

// internal path — config drives everything, never touches New()
func init() {
	skills.Register("summarize", func(
		ctx context.Context,
		cfg skills.Config,
		conn connectors.Connector,
	) (skills.Skill, error) {
		if conn != nil {
			return nil, fmt.Errorf("summarize: connector should be nil")
		}

		if cfg.LLM == nil {
			return nil, fmt.Errorf("summarize: llm config required")
		}

		client, err := llm.New(*cfg.LLM)
		if err != nil {
			return nil, fmt.Errorf("summarize: llm init: %w", err)
		}

		skillCtx, cancel := context.WithCancel(ctx)

		if cfg.LLM.Options == nil {
			cfg.LLM.Options = &llm.Options{}
		}
		return &SummarizeSkill{
			BaseSkill: skills.NewBaseSkillFromConfig(
				cfg,
				skills.SkillTypeNative,
				tools.GetSummarizeTools(),
				tools.GetSummarizeGroups(),
			),
			llmClient:  client,
			llmOptions: *cfg.LLM.Options,
			ctx:        skillCtx,
			cancel:     cancel,
		}, nil
	})
}

func (s *SummarizeSkill) Initialize() error {
	return nil
}

func (s *SummarizeSkill) Shutdown() error {
	s.cancel()
	return nil
}

func (s *SummarizeSkill) Execute(ctx context.Context, toolName string, args []byte) tools.ToolExecution {
	if s.llmClient == nil {
		return tools.ExecErr(fmt.Errorf("summarize: LLM client not initialized"))
	}

	var result any
	var err error

	switch toolName {
	case "summarize_text":
		result, err = s.summarizeText(ctx, args)
	case "summarize_url":
		result, err = s.summarizeURL(ctx, args)
	case "summarize_file":
		result, err = s.summarizeFile(ctx, args)
	case "summarize_youtube":
		result, err = s.summarizeYouTube(ctx, args)
	default:
		return tools.ExecErr(fmt.Errorf("summarize: unknown tool %q", toolName))
	}

	if err != nil {
		return tools.ExecErr(err)
	}
	return tools.ExecOK(result)
}

// ── tool handlers ─────────────────────────────────────────────────────────────

func (s *SummarizeSkill) summarizeText(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.SummarizeTextInput](args)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Text) == "" {
		return nil, fmt.Errorf("summarize_text: text is required")
	}
	summary, err := s.summarize(ctx, input.Text, "", input.Length, input.Language, input.Focus)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"summary":        summary,
		"input_length":   len(input.Text),
		"summary_length": len(summary),
	}, nil
}

func (s *SummarizeSkill) summarizeURL(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.SummarizeURLInput](args)
	if err != nil {
		return nil, err
	}
	if input.URL == "" {
		return nil, fmt.Errorf("summarize_url: url is required")
	}

	title, text, err := fetchURL(input.URL)
	if err != nil {
		// Surface structured context so the agent can reason about next steps.
		fe := &FetchError{}
		if errors.As(err, &fe) {
			return map[string]any{
				"url":        input.URL,
				"accessible": false,
				"error_type": httpErrorType(fe.StatusCode),
				"error":      err.Error(),
			}, nil
		}
		return map[string]any{
			"url":        input.URL,
			"accessible": false,
			"error_type": "fetch_failed",
			"error":      err.Error(),
		}, nil
	}

	if strings.TrimSpace(text) == "" {
		return map[string]any{
			"url":        input.URL,
			"accessible": true,
			"error_type": "no_content",
			"error":      "no readable content found at URL",
		}, nil
	}

	sourceDesc := input.URL
	if title != "" {
		sourceDesc = fmt.Sprintf("%s (%s)", title, input.URL)
	}
	summary, err := s.summarize(ctx, text, sourceDesc, input.Length, input.Language, input.Focus)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"url":            input.URL,
		"title":          title,
		"summary":        summary,
		"accessible":     true,
		"summary_length": len(summary),
	}, nil
}

func (s *SummarizeSkill) summarizeFile(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.SummarizeFileInput](args)
	if err != nil {
		return nil, err
	}
	if input.Path == "" {
		return nil, fmt.Errorf("summarize_file: path is required")
	}

	text, err := readFile(input.Path)
	if err != nil {
		return nil, fmt.Errorf("summarize_file: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("summarize_file: no readable content in %s", input.Path)
	}

	summary, err := s.summarize(ctx, text, input.Path, input.Length, input.Language, input.Focus)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path":           input.Path,
		"summary":        summary,
		"summary_length": len(summary),
	}, nil
}

func (s *SummarizeSkill) summarizeYouTube(ctx context.Context, args []byte) (any, error) {
	input, err := tools.UnmarshalParams[tools.SummarizeYouTubeInput](args)
	if err != nil {
		return nil, err
	}
	if input.URL == "" {
		return nil, fmt.Errorf("summarize_youtube: url is required")
	}

	transcript, found, err := fetchYouTubeTranscript(input.URL)
	if err != nil {
		return nil, fmt.Errorf("summarize_youtube: %w", err)
	}
	if !found || strings.TrimSpace(transcript) == "" {
		return map[string]any{
			"url":     input.URL,
			"summary": "",
			"note":    "No transcript available for this video. Automatic captions may be disabled or the video may not have captions.",
		}, nil
	}

	summary, err := s.summarize(ctx, transcript, "YouTube transcript: "+input.URL, input.Length, input.Language, "")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"url":             input.URL,
		"summary":         summary,
		"transcript_used": true,
		"summary_length":  len(summary),
	}, nil
}

// ── core summarization logic ──────────────────────────────────────────────────

// summarize handles token-aware chunking and batched hierarchical merge.
func (s *SummarizeSkill) summarize(
	ctx context.Context,
	content, sourceDesc string,
	length tools.SummarizeLength,
	language, focus string,
) (string, error) {
	if len(content) <= maxChunkChars {
		return s.callLLM(ctx, buildPrompt(content, sourceDesc, length, language, focus))
	}

	chunks := chunkText(content, maxChunkChars)
	partials := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		desc := fmt.Sprintf("%s (part %d/%d)", sourceDesc, i+1, len(chunks))
		partial, err := s.callLLM(ctx,
			buildPrompt(chunk, desc, tools.SummarizeLengthMedium, language, focus))
		if err != nil {
			return "", fmt.Errorf("summarize chunk %d: %w", i+1, err)
		}
		partials = append(partials, partial)
	}

	// Batched merge — iterate until one summary remains.
	// Each pass reduces count by mergeBatchSize factor.
	// Two passes handle documents of any realistic size.
	for len(partials) > 1 {
		var next []string
		for i := 0; i < len(partials); i += mergeBatchSize {
			end := i + mergeBatchSize
			end = min(end, len(partials))
			batch := partials[i:end]
			// Only the final batch gets the requested length —
			// intermediate merges stay medium to preserve detail.
			batchLength := tools.SummarizeLengthMedium
			if end == len(partials) && len(next) == 0 {
				batchLength = length
			}
			merged, err := s.callLLM(ctx, mergePrompt(batch, batchLength, language, focus))
			if err != nil {
				return "", fmt.Errorf("merge batch %d: %w", i/mergeBatchSize+1, err)
			}
			next = append(next, merged)
		}
		partials = next
	}

	return partials[0], nil
}

func (s *SummarizeSkill) callLLM(ctx context.Context, prompt string) (string, error) {
	req := &llm.Request{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
	}
	if s.llmOptions.MaxTokens > 0 {
		req.MaxTokens = s.llmOptions.MaxTokens
	}

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}
	if resp.Content == "" {
		return "", fmt.Errorf("LLM returned no response")
	}
	return strings.TrimSpace(resp.Content), nil
}

// httpErrorType maps status codes to agent-readable labels.
func httpErrorType(code int) string {
	switch {
	case code == 403:
		return "forbidden"
	case code == 404:
		return "not_found"
	case code == 429:
		return "rate_limited"
	case code == 451:
		return "legal_block"
	case code >= 500:
		return "server_error"
	default:
		return "http_error"
	}
}
