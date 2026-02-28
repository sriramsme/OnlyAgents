// pkg/skills/cli/converter.go
package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
)

// ConvertOptions controls how the conversion is performed.
type ConvertOptions struct {
	// SkillName overrides the name field in the generated frontmatter.
	// If empty, the LLM infers it from the content.
	SkillName string
}

// ConvertResult is returned by ConvertSKILL.
type ConvertResult struct {
	// Content is the fully-formatted SKILL.md ready to be written to disk.
	Content string
	// Parsed is the validated, in-memory representation of Content.
	Parsed *ParsedSkill
}

// ConvertSKILL takes arbitrary skill content (any format, any quality) and uses
// the supplied LLM client to rewrite it into the canonical SKILL.md format.
// It validates the output by parsing it; if parsing fails it retries once with
// the parse error fed back to the model as a follow-up user message.
func ConvertSKILL(ctx context.Context, client llm.Client, raw string, opts ConvertOptions) (*ConvertResult, error) {
	systemPrompt := buildSystemPrompt(opts.SkillName)
	userMsg := buildUserMessage(raw)

	converted, err := callLLM(ctx, client, systemPrompt, []llm.Message{
		llm.UserMessage(userMsg),
	})
	if err != nil {
		return nil, fmt.Errorf("llm conversion: %w", err)
	}

	// Validate by parsing.
	parsed, parseErr := ParseSKILLMD(converted)
	if parseErr != nil {
		// One retry: extend the conversation so the model has full context.
		// assistant turn = what it produced, user turn = the parse error.
		retryMessages := []llm.Message{
			llm.UserMessage(userMsg),
			llm.AssistantMessage(converted),
			llm.UserMessage(buildRetryMessage(parseErr)),
		}

		converted, err = callLLM(ctx, client, systemPrompt, retryMessages)
		if err != nil {
			return nil, fmt.Errorf("llm retry: %w", err)
		}

		parsed, parseErr = ParseSKILLMD(converted)
		if parseErr != nil {
			return nil, fmt.Errorf("converted skill failed validation after retry: %w", parseErr)
		}
	}

	return &ConvertResult{
		Content: converted,
		Parsed:  parsed,
	}, nil
}

// ──────────────────────────────────────────────────────────────
// LLM call
// ──────────────────────────────────────────────────────────────

// callLLM sends systemPrompt + messages to the client and returns the text response.
func callLLM(ctx context.Context, client llm.Client, systemPrompt string, messages []llm.Message) (string, error) {
	req := &llm.Request{
		Messages:    append([]llm.Message{llm.SystemMessage(systemPrompt)}, messages...),
		Temperature: 0.1, // low temperature: formatting task, not creative
		Metadata: map[string]string{
			"task": "skill-conversion",
		},
	}

	resp, err := client.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	fmt.Println("LLM response:", resp.Content)
	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	// Strip markdown fences if the model wrapped its output
	content := strings.TrimSpace(resp.Content)
	content = stripMarkdownFence(content)
	return content, nil
}

func stripMarkdownFence(s string) string {
	for _, lang := range []string{"```markdown", "```md", "```"} {
		if strings.HasPrefix(s, lang) {
			s = strings.TrimPrefix(s, lang)
			s = strings.TrimSuffix(s, "```")
			s = strings.TrimSpace(s)
			break
		}
	}
	return s
}

// ──────────────────────────────────────────────────────────────
// Prompt builders
// ──────────────────────────────────────────────────────────────

const canonicalFormat = `---
name: <short-slug>
description: <one-line description>
version: 1.0.0
capabilities:
  - <capability1>
  - <capability2>
requires:
  bins:
    - <binary1>
  env: []
security:
  sanitized: true
  sanitized_at: <RFC3339 timestamp>
  sanitized_by: converter
---
# <Human-readable title>
<Optional short prose description>

## Tools

### <tool_name>
**Description:** <what the tool does>
**Command:**
` + "```bash" + `
<the actual shell command with {{param}} placeholders>
` + "```" + `
**Parameters:**
- ` + "`param_name`" + ` (string|number|integer|boolean): <description>
**Timeout:** <seconds>
**Validation:**
` + "```yaml" + `
allowed_commands:
  - <base command e.g. curl>
denied_patterns:
  - "rm -rf"
max_output_size: 102400
` + "```" + `
---
### <next_tool_name>
...`

// buildSystemPrompt returns the stable system prompt.
// The optional nameHint is baked in here because it is a hard constraint,
// not something that varies per user turn.
func buildSystemPrompt(nameHint string) string {
	nameConstraint := ""
	if nameHint != "" {
		nameConstraint = fmt.Sprintf(
			"\nThe `name` field in the frontmatter MUST be exactly: %q\n",
			nameHint,
		)
	}

	return fmt.Sprintf(`You are a skill formatter. Your only job is to convert skill definitions into the exact canonical SKILL.md format shown below.

RULES:
- Output ONLY the converted SKILL.md file. No explanation, no preamble, no markdown fences around the whole output.
- Preserve ALL commands exactly as-is. Do NOT alter shell commands, flags, or URLs.
- Use {{param}} placeholders for any dynamic values in commands.
- Each parameter bullet must follow: - `+"`name`"+` (type): description
  Valid types: string, number, integer, boolean, array
- Tools are separated by a line containing only: ---
- Include a **Validation:** block only if there are meaningful restrictions.
- Set security.sanitized_at to the current UTC time in RFC3339 format.
%s
CANONICAL FORMAT:
%s`, nameConstraint, canonicalFormat)
}

// buildUserMessage wraps the raw input for the first user turn.
func buildUserMessage(raw string) string {
	return fmt.Sprintf("Convert this skill definition to the canonical SKILL.md format:\n\n%s", raw)
}

// buildRetryMessage is sent as a follow-up user turn when the first attempt
// fails validation. The model already has its previous attempt in context as
// the assistant turn, so we only need to state what went wrong.
func buildRetryMessage(parseErr error) string {
	return fmt.Sprintf(`Your output failed validation with this error:

  %s

Fix the issue and return the corrected SKILL.md. Output ONLY the file content, no explanation.`,
		parseErr.Error())
}
