// pkg/skills/cli/converter.go
package cli

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/logger"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
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
	Parsed *skills.Config
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

// ParseSkillFile converts a loaded SkillFile into a skills.Config.
func ParseSkillFile(sf *skills.Config) (*skills.Config, error) {
	if len(sf.Tools) == 0 {
		return nil, fmt.Errorf("skill %q has no tools defined", sf.Name)
	}
	for i, t := range sf.Tools {
		if t.Name == "" {
			return nil, fmt.Errorf("skill %q: tool[%d] missing name", sf.Name, i)
		}
		if t.Exec.Command == "" {
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
func validateYAMLOutput(content string) (*skills.Config, error) {
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
		if t.Exec.Command == "" {
			return nil, fmt.Errorf("tool %q: missing command", t.Name)
		}
	}
	return sf, nil
}

func parseYAMLString(content string) (*skills.Config, error) {
	// Strip yaml fences if model wrapped output
	content = stripYAMLFence(content)
	var sf skills.Config
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
     Get it at: <where to obtain the value>

capabilities:
  - <domain_outcome>
  - <domain_outcome>

requires:
  bins:
    - name: <binary>
      install:
        brew: brew install <binary>
        apt: sudo apt install <binary>
        manual: <https://install-url>
  env:
    - ENV_VAR_NAME

groups:
  <skill>_<capability>: "<one-line description of what tools in this group do>"
  <entity>_<capability>: "<description — use entity prefix when skill has sub-domains>"

tools:
  - name: <tool_name>
    description: <what the tool does>
    group: <group_name>
    access: <read|write|admin>
    timeout: 30
    exec:
	  command: <binary>
	  args: ["<arg1>", "<arg2>", "{{param}}"]
	  stdin_param: <optional param name whose value is piped to stdin>
    parameters:
      - name: <param_name>
        type: <string|number|integer|boolean|array>
        description: <param description>
    validation:
      max_output_size: 102400
	  require_confirm: true
  - name: <next_tool_name>
    ...`

const groupRules = `
TOOL GROUPS:
Groups are intra-agent: the sub-agent reads these to load ONLY the tools needed for a task.
Think "what operation type is the agent performing right now?" not "what domain is this skill in?"

Naming convention:
  - Default: <skill_name>_<capability>  e.g. git_inspect, git_sync
  - Sub-domain: <entity>_<capability>   e.g. project_read, project_write
    Use sub-domain prefix when the skill manages multiple distinct entities
    (e.g. tasks + projects, repos + PRs) where skill_* would be ambiguous.

Rules:
  - 3-6 groups per skill. Too few = no routing benefit. Too many = fragmentation.
  - Group names should reflect tool access level:
    read tools  → <domain>_read or <domain>_inspect
    write tools → <domain>_write or <domain>_<verb>
    admin tools → <domain>_manage
  - Split read vs write for each domain: git_inspect (read) vs git_commit (write)
  - Group names are snake_case slugs, no spaces
  - Group descriptions describe the OPERATION TYPE across all tools in the group,
    not a single tool. Start with a verb or noun phrase.
    Good: "View, list, search, and filter tasks"
    Bad:  "get_task tool"
  - Every tool must belong to exactly one group — no ungrouped tools
  - A passthrough/escape-hatch tool gets its own group: <skill>_passthrough
  - Do NOT create a group per tool — that defeats the purpose
  - Do NOT use generic names like "misc", "other", "utilities"
`

const capabilityRules = `
CAPABILITIES:
Capabilities are inter-agent: the executive reads these to decide WHICH skill to route to.
Think "what would an orchestrator call this domain?" not "what does this tool do?"

Rules:
  - 3-5 capabilities max
  - Describe the OUTCOME or DOMAIN, not the implementation
  - Use phrases a non-technical user would say to describe what they need
    Good: "version_control", "code_review", "ci_cd"
    Bad:  "git_inspect", "run_tests" (too operational — those are groups)
  - Do NOT use generic cross-domain terms like "read", "write", "manage" —
    these tell the executive nothing about which skill to route to
  - Do NOT repeat group names as capabilities — they operate at different levels
`

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
- Set access_level on the skill to the highest access level of any of its tools.
- For each tool, set access:
  - read: only retrieves or lists data, no side effects
  - write: creates, updates, or sends data
  - admin: deletes, destroys, or irreversible system effects
  - When a command can be read or write depending on flags, mark it write.
- requires.bins: each bin is an object with name and install map.
  - Include install commands for common package managers (brew, apt etc) when known.
  - Always include manual with the official install URL.
  - Only include package managers you are confident about — omit uncertain ones.
- Always include instructions if requires.bins or requires.env are non-empty:
  - bins: one install step per binary referencing the manual URL
  - env: one step per env var telling user to add to ~/.onlyagents/.env with note on where to get the value
  - If neither bins nor env, omit instructions entirely.
%s
%s
%s
CANONICAL FORMAT:

%s`, nameConstraint, capabilityRules, groupRules, canonicalFormat)
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
