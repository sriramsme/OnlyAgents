# OnlyAgents

[![CI](https://github.com/sriramsme/onlyagents/workflows/CI/badge.svg)](https://github.com/sriramsme/onlyagents/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/sriramsme/onlyagents)](https://goreportcard.com/report/github.com/sriramsme/onlyagents)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/sriramsme/onlyagents)](https://go.dev/)

Self-hosted, open-source infrastructure for autonomous AI agents. Written in Go. Single binary.

OnlyAgents runs a multi-agent system. One executive agent that understands intent, decomposes it, and routes to a council of specialized sub-agents.

This is not a wrapper around a single LLM call. It is a runtime for agents.

[Go to Installation](#installation)

## Why Go

Many agent frameworks depend on high-level runtimes and large dependency trees. OnlyAgents compiles to a single statically linked binary with no runtime dependencies, keeping the system lightweight and easy to run.

| Metric / Package                | **OnlyAgents**        | **OpenClaw**     | **NanoBot**            |
| ------------------------------- | --------------------- | ---------------- | ---------------------- |
| **Language**                    | Go                    | TypeScript       | Python                 |
| **Binary / Package Size**       | 36 MB ✅              | —                | —                      |
| **RAM (idle)**                  | ~27 MB ✅             | >1 GB ⚠️         | >100 MB ⚠️             |
| **Startup Time** (p50)          | 31 ms ✅              | >500 s ⚠️        | >30 s ⚠️               |
| **Deploy Target / Cost**        | Any Linux, macOS, ARM | Mac Mini 💰 $599 | Most Linux SBC 💰 ~$50 |


> A full multi-agent runtime with embedded web UI, SQLite, and a cron scheduler — in 36MB and 31ms. Binary includes embedded web UI and full server stack. Telegram channel adds ~4MB.

Same binary on a $5/mo VPS, a spare Mac Mini, a Raspberry Pi, or a rack server. No runtime to install. No dependency hell.

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                                USER INTERFACE                                │
│             OAChannel (WebSocket/SSE) · Telegram · REST API                  │
└───────────────────────────────────┬──────────────────────────────────────────┘
                                    │
┌───────────────────────────────────▼──────────────────────────────────────────┐
│                               KERNEL (Runtime)                               │
│          Agent lifecycle · Event bus · Skill registry · Connector wiring     │
└───────────────┬────────────────────────┬──────────────────────────┬──────────┘
                │                        │                          │
        ┌───────▼────────┐       ┌───────▼─────────┐        ┌───────▼─────────┐
        │ EXECUTIVE AGENT│       │ PRODUCTIVITY    │        │ GENERAL AGENT   │
        │                │       │ AGENT           │        │                 │
        │ Orchestration  │       │ Calendar        │        │ Fallback handler│
        │ Delegation     │       │ Notes           │        │ Searches local  │
        │ Workflow coord │       │ Reminders       │        │ skill registry  │
        │ Synthesis      │       │ Tasks           │        │ then Clawhub +  │
        │                │       │                 │        │ online registris│
        └───────┬────────┘       └───────┬─────────┘        └───────┬─────────┘
                │                        │                          │
                └──────────── routes via Kernel Event Bus ──────────┘
                                    │
┌───────────────────────────────────▼──────────────────────────────────────────┐
│                                    SKILLS                                    │
│                                                                              │
│  Native (Go)               System / Internal            CLI                  │
│  ───────────               ─────────────────            ───                  │
│  calendar                  meta tools                   gh                   │
│  notes                     task_complete                kubectl              │
│  reminders                 workflows                    ffmpeg               │
│  tasks                                                   restic              │
│  web_search                                             ...                  │
│  email                                                                       │
│                                                                              │
│                 SKILL.md specifications drive CLI-based skills               │
└───────────────────────────────────┬──────────────────────────────────────────┘
                                    │
┌───────────────────────────────────▼──────────────────────────────────────────┐
│                                 CONNECTORS                                   │
│                                                                              │
│  Local (no external service)        Service (external APIs)                  │
│  ───────────────────────────        ───────────────────────                  │
│  CalendarConnector                  GmailConnector                           │
│  NotesConnector                     NotionConnector                          │
│  RemindersConnector                 DuckDuckGo / Brave                       │
│  TasksConnector                     [more planned]                           │
└──────────────────────────────────────────────────────────────────────────────┘
```
Read more [here](docs/architecture.md)

**Executive agent:** top of the hierarchy. Receives all user messages, decides whether to answer directly, delegate to a sub-agent, or decompose into a multi-step workflow. Rewrites requests with full context before delegating.

**Sub-agents:** specialized and capability-bounded. Each has independent models, its own soul, and access only to relevant tools. Trust and cost boundaries live at the agent level - run a cheap fast model for CRUD, a more capable one for reasoning.

**Kernel:** the runtime. Agent lifecycle, event bus, skill registry, connector wiring, graceful shutdown. Agents don't communicate directly — everything routes through the kernel.

**Soul:** configuration that defines an agent’s behavior, tone, and decision rules.

**Workflow engine:** cross-agent coordination. When a task spans multiple agents, the executive creates a workflow with ordered steps and tracked dependencies. Individual agents handle multi-step operations internally.


**Skills:** defines agent capabilities.

* **Native** - implemented in Go. You can install them standalone for your own use with `go install github.com/sriramsme/OnlyAgents/cmd/<skill_name>`.
* **CLI** - defined by `<skill>.yaml` files and executed through installed command-line tools.
* **System** - internal framework skills used for orchestration (e.g., `delegate_to_agent` ).

  Popular CLI skills may later be promoted to native skills as the project evolves.

**Connectors:** handle integrations with data sources and services.

* **Local connectors** operate entirely on the local system (e.g., SQLite-backed calendar, notes, tasks).
* **Service connectors** integrate with external APIs such as Gmail, Notion, Brave.

A **skill defines the capability**, while a **connector defines where the data or service comes from**.

## Memory

> Implemented. Integration testing in progress.

Agents remember. Automatically. No configuration required.

```
┌────────────────────────────────────────────────────────────────┐
│  Working Memory  ·  current conversation messages              │
│                          ↓  saved on every message             │
│  Short-term      ·  last 4 hours  ·  30-day retention          │
│                          ↓  LLM summarization at 23:59 daily   │
│  Daily Summaries ·  key events, topics  ·  90-day retention    │
│                          ↓  every Sunday                       │
│  Weekly Summaries  ·  themes, patterns  ·  1-year retention    │
│                          ↓  1st of month                       │
│  Monthly Summaries  ·  highlights, stats  ·  5-year retention  │
│                          ↓  Dec 31                             │
│  Yearly Archives  ·  compressed  ·  permanent                  │
├────────────────────────────────────────────────────────────────┤
│  Facts Database  ·  entity knowledge  ·  permanent             │
│  Extracted during daily summarization. Confidence-scored.      │
│  Example: "prefers morning meetings", "works in Go"            │
└────────────────────────────────────────────────────────────────┘
```

All summarization runs in-process via a cron scheduler. On startup, catch-up logic runs any missed jobs. The facts database uses SQLite FTS5 for search - no vector embeddings required.

## Default Agents

Some agents that we believe applies for everyone, ship out of the box. Their configurations and souls are tuned - modify them if you want, but the defaults are a solid starting point.

Rules:
- Only one executive and one general agent can exist in a deployment.
- Agent IDs must be unique across all config files.
- Additional specialized agents can be added by dropping a config file into `~/.onlyagents/configs/agents/`.

## The Web Interface (OAChannel)

> Work in progress.

OnlyAgents ships with a built-in web interface connected over a single persistent WebSocket. No polling. One connection carries everything like chat, streaming responses, real-time agent activity, tool call notifications, delegations between agents, and proactive messages like reminders and daily digests.

**Chat panel:** conversations with the executive. Responses stream token by token. Sessions are persistent - the interface resumes your last conversation on return. Reconnects and page reloads are transparent.

**Council room:** live view of the system as it works. Which agents are active, what they're doing, tool calls in flight, delegations as they happen.

OAChannel is architecturally identical to any other channel - agents have no special awareness of it. The REST API and WebSocket protocol are fully documented. Build your own interface if you want.

## Modes

**Agent mode:** headless. No HTTP server. Smallest footprint.

```
onlyagents start --no-server
```

**Server mode:** full stack. REST API, embedded web UI, WebSocket, Telegram.

```
onlyagents start
```

## Installation

### Pre-built binaries

Download from [GitHub Releases](https://github.com/sriramsme/onlyagents/releases).

```bash
# macOS (Apple Silicon)
curl -L https://github.com/sriramsme/onlyagents/releases/latest/download/onlyagents_darwin_arm64.tar.gz | tar xz
sudo mv onlyagents /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/sriramsme/onlyagents/releases/latest/download/onlyagents_linux_amd64.tar.gz | tar xz
sudo mv onlyagents /usr/local/bin/
```

### Build from source

Prerequisites: Go 1.25+, Node.js 18+ (for the embedded web UI)

```bash
git clone https://github.com/sriramsme/onlyagents
cd onlyagents
make install
```

The Makefile builds the web UI first, then installs the binary. The interface is embedded at compile time — one binary, nothing else to serve.

### Custom Builds

Pre-built binaries include all channels, all LLM providers, and the `env` vault backend. If you're building from source and want a smaller binary, build tags let you include only what you need.

**Channels:** by default all channels are included. To include only what you use:

```bash
# Telegram only (no OAChannel/web UI)
go install -tags channel_telegram ./cmd/onlyagents/

# OAChannel only (no Telegram). You need oachannel for server mode.
go install -tags channel_onlyagents ./cmd/onlyagents/
```

**LLM providers:** by default all providers are included. For example, to include only Anthropic:

```bash
go install -tags llm_anthropic ./cmd/onlyagents/
```

**Vault backends:** `env` is always included. HashiCorp, AWS, and GCP vault backends are available but not yet tested — opt in explicitly:

```bash
go install -tags vault_hashicorp ./cmd/onlyagents/  #untested
go install -tags vault_aws ./cmd/onlyagents/        # untested
go install -tags vault_gcp ./cmd/onlyagents/        # untested
```

Combine tags as needed:

```bash
go install -tags "llm_anthropic channel_telegram vault_hashicorp" ./cmd/onlyagents/
```
## Getting Started

After installing, run the setup wizard:

```bash
onlyagents setup
```

This walks through creating your config directory, setting your user profile,
configuring an LLM provider, choosing a channel, and setting your auth password.
Safe to re-run. already configured steps can be skipped.

[Available commands](#cli)

## Councils

Councils are preconfigured agent teams for specific domains. Instead of assembling an agent system manually, activate a council with a single command and get a working, curated setup immediately.

```bash
onlyagents council enable software_dev
```

OnlyAgents ships with various built-in councils:

| Council                | Description                                              |
| ---------------------- | -------------------------------------------------------- |
| `personal_productivity`| Calendar, tasks, reminders, and notes. No setup needed.  |
| `research`             | Web search, summarization, and note-taking.              |
| `software_dev`         | Engineering team - coding, review, testing, GitHub.      |
| `devops`               | Infrastructure - Docker, Kubernetes, deployments.        |
| `content_creation`     | Writing, drafting, and research-backed content.          |
| `home_life`            | Household tasks, weather, travel, and home automation.   |

Councils don't change the runtime architecture. All they do is enable the right agents, skills, and connectors for a domain. The executive agent and kernel work exactly the same way.

## Configuration

All config and runtime data lives in `~/.onlyagents/`.

```
~/.onlyagents/
├── agents
│   ├── executive.yaml
│   ├── general.yaml
│   ├── productivity.yaml
│   └── researcher.yaml
├── channels
│   └── telegram.yaml
├── connectors           # Custom/downloaded connectors
│   ├── Brave.yaml
│   ├── DuckDuckGo.yaml
│   └── Perplexity.yaml
├── councils
│   ├── personal_productivity.yaml
│   ├── research.yaml
│   ├── software_dev.yaml
│   ├── devops.yaml
├── skills               # Custom/downloaded skill files
│   ├── github.md
│   └── weather.md
├── logs
├── cache
├── marketplace
├── onlyagents.db        # SQLite — all persistent data
├── server.yaml
├── config.yaml
├── user.yaml            # User config
└── vault.yaml

```

### Vault

```yaml
# ~/.onlyagents/vault.yaml
type: env
dotenv_path: '.env'  # exlcude this or keep it empty if using system environments.
enable_cache: true
audit_log: false
```

For `.env` files, `ANTHROPIC_API_KEY` is referenced in agent configs as `anthropic/api_key`. Other supported backends: `hashicorp`, `aws`, `gcp`. See [docs/vault.md](docs/vault.md).

### Agent config

```yaml
# ~/.onlyagents/configs/agents/executive.yaml
id: "executive"
name: "Dragon"
description: "Executive assistant. Handles delegation, coordination, synthesis."
is_executive: true
max_concurrency: 4
buffer_size: 20
llm:
  provider: "anthropic"
  model: "claude-haiku-4-5-20251001"
  api_key_path: "anthropic/api_key"
```

Each agent specifies its own provider, model, and vault key independently. Full reference: [docs/agents.md](docs/agents.md).

## CLI

```bash
onlyagents setup                   Interactive setup wizard — run this first

onlyagents start                   Start server (API + web UI + agent kernel)
onlyagents start --no-server       Run agent kernel only (headless)

onlyagents agent list              List all configured agents
onlyagents agent enable <id>       Enable an agent
onlyagents agent disable <id>      Disable an agent
onlyagents agent view <id>         View agent config (--raw, --field flags)
onlyagents agent edit <id>         Edit agent config interactively

onlyagents channel list            List all configured channels
onlyagents channel setup <name>    Run interactive setup for a channel
onlyagents channel enable <name>   Enable a channel
onlyagents channel disable <name>  Disable a channel
onlyagents channel view <name>     View channel config
onlyagents channel edit <name>     Edit channel config interactively

onlyagents connector list          List all configured connectors
onlyagents connector setup <name>  Run interactive setup for a connector
onlyagents connector enable <name> Enable a connector
onlyagents connector disable <name> Disable a connector
onlyagents connector view <name>   View connector config
onlyagents connector edit <name>   Edit connector config interactively

onlyagents skill list              List all skills
onlyagents skill install <name>    Install required binaries for a skill
onlyagents skill enable <name>     Enable a skill
onlyagents skill disable <name>    Disable a skill
onlyagents skill validate <name>   Validate a skill's requirements (bins, env vars)
onlyagents skill view <name>       View skill config
onlyagents skill edit <name>       Edit skill config interactively
onlyagents skill tools <name>      List tools provided by a skill

onlyagents council list            List all councils and their status
onlyagents council info <name>     Show agents, skills, and connectors in a council
onlyagents council enable <name>   Activate a council
onlyagents council disable <name>  Deactivate a council

onlyagents auth reset              Generate new password
onlyagents auth set-password       Change password interactively
onlyagents auth status             Show auth configuration

onlyagents models list             List all supported models across providers
onlyagents models info <model>     Detailed model info and pricing
onlyagents models compare m1 m2    Side-by-side comparison
onlyagents models filter           Filter by capability (interactive or --flags)

onlyagents convert <file> -n <n>   Convert raw skill to canonical SKILL.md format
```
Auth credentials are configured during onlyagents setup. Reset anytime with onlyagents auth reset.

## Custom Skills

Drop a SKILL.md file into `~/.onlyagents/skills/` and restart. For raw or unformatted skill definitions:

```bash
onlyagents convert raw_skill.md -n weather -p anthropic
# Saved to ~/.onlyagents/skills/weather.md
```

CLI skills work with whatever tools are installed on the host — `gh`, `kubectl`, `ffmpeg`, `restic`, `docker`. Community skill files from Clawhub or other registries can be converted and dropped the same way. The General agent will also download and convert skills automatically at runtime when no local skill covers a request.

Skill format reference: [docs/skills.md](docs/skills.md).

## A2A — Agent-to-Agent Communication

> In development. Protocol is being finalized.

OnlyAgents will support direct, signed, peer-to-peer communication between separate agent instances — across machines, across users, without a relay.

- No relay server. Agents communicate directly over HTTPS.
- Ed25519 keypairs per instance. All envelopes cryptographically signed.
- A lightweight keyserver handles discovery — public key resolution only. Never sees message content.
- The protocol is a standalone Go module (`github.com/onlyagents/a2a`) with no OnlyAgents-specific dependencies. Any agent framework can implement it.

This makes it possible for agents on different machines, owned by different people, to delegate tasks and share results — with all content private and end-to-end verifiable.

Setup instructions will be added here once the implementation ships.

## Backup

Back up the entire `~/.onlyagents/` directory — it contains your database, agent configs, custom skills, vault config, and credentials. Everything needed to restore a working instance.

```bash
cp -r ~/.onlyagents/ ~/backups/onlyagents-$(date +%Y%m%d)/
```

## Remote Deployment

OnlyAgents runs on any Linux server — VPS, bare metal, ARM. No separate database to provision. Single binary, SQLite, point at your config and run.

Systemd and Docker configurations are not yet documented. Remote deployment PRs and working configurations are welcome.

## Contributing

Issues, feature requests, and pull requests are open.

High-value contributions: new channels (Slack, Discord), new connectors (Google Calendar sync, Notion, Linear), remote deployment configurations, A2A protocol feedback, and skill definitions for Clawhub.

Open an issue before building a new channel or connector — interface contracts are worth aligning on first.

## License

MIT
