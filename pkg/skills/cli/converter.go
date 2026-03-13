// pkg/skills/cli/converter.go
package cli

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

// ConvertOptions controls how the conversion is performed.
type ConvertOptions struct {
	SkillName string
}

// ConvertResult is returned by ConvertSKILL.
type ConvertResult struct {
	// Content is the fully-formatted YAML ready to be written to disk.
	Content string
	// Parsed is the validated in-memory representation of Content.
	Parsed *config.SkillConfig
}

// ConvertSKILL takes arbitrary skill content and uses the LLM to rewrite
// it into the canonical YAML format. Validates by parsing; retries once
// with the parse error fed back to the model.
func ConvertSKILL(ctx context.Context, client llm.Client, raw string, opts ConvertOptions) (*ConvertResult, error) {
	systemPrompt := buildSystemPrompt(opts.SkillName)
	userMsg := buildUserMessage(raw)

	converted, err := callLLM(ctx, client, systemPrompt, []llm.Message{
		llm.UserMessage(userMsg),
	})
	if err != nil {
		return nil, fmt.Errorf("llm conversion: %w", err)
	}

	sf, parseErr := validateYAMLOutput(converted)
	if parseErr != nil {
		retryMessages := []llm.Message{
			llm.UserMessage(userMsg),
			llm.AssistantMessage(converted),
			llm.UserMessage(buildRetryMessage(parseErr)),
		}
		logger.Log.Debug("Parsing YAML failed, retrying with error message", "error", parseErr)
		converted, err = callLLM(ctx, client, systemPrompt, retryMessages)
		if err != nil {
			return nil, fmt.Errorf("llm retry: %w", err)
		}
		sf, parseErr = validateYAMLOutput(converted)
		if parseErr != nil {
			return nil, fmt.Errorf("converted skill failed validation after retry: %w", parseErr)
		}
	}

	parsed, err := ParseSkillFile(sf)
	if err != nil {
		return nil, fmt.Errorf("parse converted skill: %w", err)
	}

	return &ConvertResult{
		Content: converted,
		Parsed:  parsed,
	}, nil
}

// ParseSkillFile converts a loaded SkillFile into a config.SkillConfig.
func ParseSkillFile(sf *config.SkillConfig) (*config.SkillConfig, error) {
	if len(sf.Tools) == 0 {
		return nil, fmt.Errorf("skill %q has no tools defined", sf.Name)
	}
	for i, t := range sf.Tools {
		if t.Name == "" {
			return nil, fmt.Errorf("skill %q: tool[%d] missing name", sf.Name, i)
		}
		if t.Command == "" {
			return nil, fmt.Errorf("skill %q tool %q: missing command", sf.Name, t.Name)
		}
		if sf.Tools[i].Timeout == 0 {
			sf.Tools[i].Timeout = 30
		}
		if sf.Tools[i].Access == "" {
			sf.Tools[i].Access = "read"
		}
	}
	return sf, nil
}

// validateYAMLOutput parses and validates the LLM's YAML output.
func validateYAMLOutput(content string) (*config.SkillConfig, error) {
	sf, err := parseYAMLString(content)
	if err != nil {
		return nil, err
	}
	if sf.Name == "" {
		return nil, fmt.Errorf("missing name field")
	}
	if sf.Type != "cli" {
		return nil, fmt.Errorf("type must be 'cli', got %q", sf.Type)
	}
	if len(sf.Tools) == 0 {
		return nil, fmt.Errorf("no tools defined")
	}
	for i, t := range sf.Tools {
		if t.Name == "" {
			return nil, fmt.Errorf("tool[%d]: missing name", i)
		}
		if t.Command == "" {
			return nil, fmt.Errorf("tool %q: missing command", t.Name)
		}
	}
	return sf, nil
}

func parseYAMLString(content string) (*config.SkillConfig, error) {
	// Strip yaml fences if model wrapped output
	content = stripYAMLFence(content)
	var sf config.SkillConfig
	if err := yaml.Unmarshal([]byte(content), &sf); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	return &sf, nil
}

func stripYAMLFence(s string) string {
	s = strings.TrimSpace(s)
	for _, fence := range []string{"```yaml", "```yml", "```"} {
		if rest, ok := strings.CutPrefix(s, fence); ok {
			s = strings.TrimSpace(strings.TrimSuffix(rest, "```"))
			break
		}
	}
	return s
}

// ── LLM call ──────────────────────────────────────────────────────────────────

func callLLM(ctx context.Context, client llm.Client, systemPrompt string, messages []llm.Message) (string, error) {
	req := &llm.Request{
		Messages:    append([]llm.Message{llm.SystemMessage(systemPrompt)}, messages...),
		Temperature: 0.1,
		Metadata:    map[string]string{"task": "skill-conversion"},
	}
	resp, err := client.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}
	return strings.TrimSpace(resp.Content), nil
}

// ── Prompt builders ───────────────────────────────────────────────────────────

const canonicalFormat = `name: <short-slug>
type: cli
enabled: true
access_level: <read|write|admin>
description: <one-line description>
version: 1.0.0
instructions: |
  1. Install <binary>: <install URL or command>
  2. Set <ENV_VAR_NAME> in ~/.onlyagents/.env
capabilities:
  - web_search       # primary capability
  - arxiv            # sub-capability
  - fact_checking    # what it enables, not what it is
requires:
  bins:
    - <binary>
  env:
    - ENV_VAR_NAME
security:
  sanitized: true
  sanitized_at: <RFC3339 timestamp>
  sanitized_by: converter
tools:
  - name: <tool_name>
    description: <what the tool does>
    access: <read|write|admin>
    timeout: 30
    command: <shell command with {{param}} placeholders>
    parameters:
      - name: <param_name>
        type: <string|number|integer|boolean|array>
        description: <param description>
    validation:
      allowed_commands:
        - <base command>
      denied_patterns:
        - "rm -rf"
      max_output_size: 102400
  - name: <next_tool_name>
    ...`

func buildSystemPrompt(nameHint string) string {
	nameConstraint := ""
	if nameHint != "" {
		nameConstraint = fmt.Sprintf(
			"\nThe `name` field MUST be exactly: %q\n",
			nameHint,
		)
	}

	return fmt.Sprintf(`You are a skill formatter. Convert skill definitions into the exact canonical YAML format shown below.

RULES:
- Output ONLY the YAML file. No explanation, no preamble, no markdown fences.
- type must always be: cli
- Preserve ALL commands exactly as-is. Do NOT alter shell commands, flags, or URLs.
- Use {{param}} placeholders for dynamic values in commands.
- Each parameter needs name, type, and description.
  Valid types: string, number, integer, boolean, array
- Include validation block only if there are meaningful restrictions.
- Set security.sanitized_at to current UTC time in RFC3339 format.
- Set access_level on the skill to the highest access level of any of its tools.
- For each tool, set access:
  - read: only retrieves or lists data, no side effects
  - write: creates, updates, or sends data
  - admin: deletes, destroys, or irreversible system effects
  - When a command can be read or write depending on flags, mark it write.
- Always include instructions if requires.bins or requires.env are non-empty:
  - bins: include install step with URL
  - env: include step to add to ~/.onlyagents/.env with note on where to get the value
  - If neither, omit instructions entirely.
- Capabilities: domain-level slugs describing what workflows this skill enables.
  Think from an executive agent's perspective: "what kind of task would I route here?"
  Rules:
  - Describe the OUTCOME or DOMAIN, not the implementation (not how it works, what it produces)
  - 3-5 capabilities max
  - Ask: "would a non-technical user use this phrase to describe what they need?"
    Yes → good capability. No → too technical or too granular.
  - Examples by domain:
    technical:   github, repository_management, issue_tracking, ci_cd
    creative:    creative_writing, content_editing, storytelling
    productivity: task_management, scheduling, note_taking

%s

CANONICAL FORMAT:\n\n
%s`, nameConstraint, canonicalFormat)
}

func buildUserMessage(raw string) string {
	return fmt.Sprintf("Convert this skill definition to the canonical YAML format:\n\n%s", raw)
}

func buildRetryMessage(parseErr error) string {
	return fmt.Sprintf(`Your output failed validation:

  %s

Fix the issue and return the corrected YAML. Output ONLY the file content, no explanation.`,
		parseErr.Error())
}
