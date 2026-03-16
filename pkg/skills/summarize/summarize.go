package summarize

import (
	"context"
	"fmt"
	"strings"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

// maxChunkChars is the max content size sent to the LLM in a single call.
// Content larger than this is chunked, summarized in parts, then merged.
const maxChunkChars = 24000

func init() {
	skills.Register("summarize", NewSummarizeSkill)
}

type SummarizeSkill struct {
	ctx    context.Context
	cancel context.CancelFunc
	*skills.BaseSkill
	llmClient llm.Client
}

func NewSummarizeSkill(ctx context.Context, cfg config.SkillConfig,
	conn connectors.Connector, security config.SecurityConfig,
) (skills.Skill, error) {
	if conn != nil {
		return nil, fmt.Errorf("summarize: connector should be nil")
	}

	base := skills.NewBaseSkill(cfg, skills.SkillTypeNative)

	if cfg.LLM == nil {
		return nil, fmt.Errorf("summarize: llm config required")
	}

	client, err := llm.NewFromConfig(*cfg.LLM)
	if err != nil {
		return nil, fmt.Errorf("summarize: llm init: %w", err)
	}

	skillCtx, cancel := context.WithCancel(ctx)
	return &SummarizeSkill{
		BaseSkill: base,
		llmClient: client,
		ctx:       skillCtx,
		cancel:    cancel,
	}, nil
}

func (s *SummarizeSkill) Initialize() error {
	return nil
}

func (s *SummarizeSkill) Shutdown() error {
	s.cancel()
	return nil
}

func (s *SummarizeSkill) Tools() []tools.ToolDef {
	return tools.GetSummarizeTools()
}

func (s *SummarizeSkill) Execute(ctx context.Context, toolName string, args []byte) (any, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("summarize: LLM client not initialized")
	}
	switch toolName {
	case "summarize_text":
		return s.summarizeText(ctx, args)
	case "summarize_url":
		return s.summarizeURL(ctx, args)
	case "summarize_file":
		return s.summarizeFile(ctx, args)
	case "summarize_youtube":
		return s.summarizeYouTube(ctx, args)
	default:
		return nil, fmt.Errorf("summarize: unknown tool %q", toolName)
	}
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
		return nil, fmt.Errorf("summarize_url: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("summarize_url: no readable content found at %s", input.URL)
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

// summarize handles chunking for long content and calls the LLM.
func (s *SummarizeSkill) summarize(
	ctx context.Context,
	content, sourceDesc string,
	length tools.SummarizeLength,
	language, focus string,
) (string, error) {
	// Short content: single call
	if len(content) <= maxChunkChars {
		return s.callLLM(ctx, buildPrompt(content, sourceDesc, length, language, focus))
	}

	// Long content: map-reduce — summarize each chunk, then merge
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

	// Single merge pass over all partial summaries
	return s.callLLM(ctx, mergePrompt(partials, length, language, focus))
}

func (s *SummarizeSkill) callLLM(ctx context.Context, prompt string) (string, error) {
	req := &llm.Request{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
	}
	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}
	if len(resp.Content) == 0 {
		return "", fmt.Errorf("LLM returned no response")
	}
	return strings.TrimSpace(resp.Content), nil
}
