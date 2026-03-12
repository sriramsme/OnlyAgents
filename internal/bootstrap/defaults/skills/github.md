---
name: github
description: "Interact with GitHub using the `gh` CLI. Use `gh issue`, `gh pr`, `gh run`, and `gh api` for issues, PRs, CI runs, and advanced queries."
version: 1.0.0
enabled: true
capabilities:
  - github
requires:
  bins:
    - gh
  env:
    - GITHUB_TOKEN
instructions: |
  1. Install the GitHub CLI: https://cli.github.com
  2. Set GITHUB_TOKEN in ~/.onlyagents/.env
     Get a token at: https://github.com/settings/tokens
security:
  sanitized: true
  sanitized_at: 2026-02-28T00:00:00Z
  sanitized_by: converter
---
# GitHub Skill
Use the `gh` CLI to interact with GitHub. Always specify `--repo owner/repo` when not in a git directory, or use URLs directly.

## Pull Requests

Check CI status on a PR:
```bash
gh pr checks {{pr_id}} --repo {{repo}}
```

List recent workflow runs:
```bash
gh run list --repo {{repo}} --limit {{limit}}
```

View a run and see which steps failed:
```bash
gh run view {{run_id}} --repo {{repo}}
```

View logs for failed steps only:
```bash
gh run view {{run_id}} --repo {{repo}} --log-failed
```

## API for Advanced Queries

The `gh api` command is useful for accessing data not available through other subcommands.

Get PR with specific fields:
```bash
gh api repos/{{owner}}/{{repo}}/pulls/{{pull_id}} --jq '.title, .state, .user.login'
```

## JSON Output

Most commands support `--json` for structured output.  You can use `--jq` to filter:

```bash
gh issue list --repo {{repo}} --json number,title --jq '.[] | "\(.number): \(.title)"'
```

## Tools

---
### gh_pr_checks
**Description:** Check CI status on a PR
**Command:**
```bash
gh pr checks {{pr_id}} --repo {{repo}}
```
**Parameters:**
- `pr_id` (integer): PR number
- `repo` (string): repository in owner/repo format
**Timeout:** 10
**Validation:**
```yaml
allowed_commands:
  - gh
denied_patterns:
  - "rm -rf"
max_output_size: 102400
```
---
### gh_run_list
**Description:** List recent workflow runs
**Command:**
```bash
gh run list --repo {{repo}} --limit {{limit}}
```
**Parameters:**
- `repo` (string): repository in owner/repo format
- `limit` (integer): number of runs to list
**Timeout:** 10
**Validation:**
```yaml
allowed_commands:
  - gh
denied_patterns:
  - "rm -rf"
max_output_size: 102400
```
---
### gh_run_view
**Description:** View a run and see which steps failed
**Command:**
```bash
gh run view {{run_id}} --repo {{repo}}
```
**Parameters:**
- `run_id` (string|integer): run identifier
- `repo` (string): repository
**Timeout:** 10
**Validation:**
```yaml
allowed_commands:
  - gh
denied_patterns:
  - "rm -rf"
max_output_size: 102400
```
---
### gh_run_view_log
**Description:** View logs for failed steps only
**Command:**
```bash
gh run view {{run_id}} --repo {{repo}} --log-failed
```
**Parameters:**
- `run_id` (string|integer): run identifier
- `repo` (string): repository
**Timeout:** 10
**Validation:**
```yaml
allowed_commands:
  - gh
denied_patterns:
  - "rm -rf"
max_output_size: 102400
```
---
### gh_api_pull_fields
**Description:** Get PR with specific fields via the GitHub API
**Command:**
```bash
gh api repos/{{owner}}/{{repo}}/pulls/{{pull_id}} --jq '.title, .state, .user.login'
```
**Parameters:**
- `owner` (string): repository owner
- `repo` (string): repository name
- `pull_id` (integer): pull request number
**Timeout:** 10
**Validation:**
```yaml
allowed_commands:
  - gh
denied_patterns:
  - "rm -rf"
max_output_size: 102400
```
---
### gh_issue_list_json
**Description:** List issues with JSON and optional jq filtering
**Command:**
```bash
gh issue list --repo {{repo}} --json number,title --jq '.[] | "\(.number): \(.title)"'
```
**Parameters:**
- `repo` (string): repository
**Timeout:** 10
**Validation:**
```yaml
allowed_commands:
  - gh
denied_patterns:
  - "rm -rf"
max_output_size: 102400
```
---
