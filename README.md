
# OnlyAgents

[![CI](https://github.com/sriramsme/onlyagents/workflows/CI/badge.svg)](https://github.com/sriramsme/onlyagents/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/sriramsme/onlyagents)](https://goreportcard.com/report/github.com/sriramsme/onlyagents)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/sriramsme/onlyagents)](https://go.dev/)

Self-hosted, open-source infrastructure for autonomous AI agents. Written in Go. Single binary.

OnlyAgents runs a multi-agent system — one executive agent that understands intent, decomposes it, and routes to a council of specialized sub-agents. Each sub-agent has its own LLM, its own tools, its own capabilities. The executive synthesizes results and responds. Agents communicate, delegate, and collaborate — and when a task spans multiple agents, a workflow engine coordinates the steps.

This is not a wrapper around a single LLM call. It is a runtime for agents that think, act, and remember.

## Why Go
Most agent frameworks are Python — hundreds of megabytes of runtime overhead before your agent does anything. OnlyAgents is a single statically-linked binary with no runtime dependencies.


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
        │                │       │ AGENT (Friday)  │        │                 │
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
│  Native (Go)            System / Built-in        CLI-driven          Sandbox │
│  ───────────            ─────────────────        ──────────          ─────── │
│  calendar               meta tools               gh                  code    │
│  notes                  task_complete            ffmpeg              exec    │
│  reminders              workflows                kubectl             runtime │
│  tasks                                          restic             (isolated)│
│  web_search                                     ...                          │
│  email                                                                       │
│                                                                              │
│                    SKILL.md specifications drive CLI tools                   │
└───────────────────────────────────┬──────────────────────────────────────────┘
                                    │
┌───────────────────────────────────▼──────────────────────────────────────────┐
│                                 CONNECTORS                                   │
│                                                                              │
│  Native (no 3rd party)             External                                  │
│  ─────────────────────             ─────────                                 │
│  CalendarConnector                 DuckDuckGo / Brave                        │
│  NotesConnector                    Gmail                                     │
│  RemindersConnector                [more planned]                            │
│  TasksConnector                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```
Read more [here](docs/architecture.md)

**Skills come in four types:**
- Native skills: implemented in Go with formal connector interfaces - calendar, notes, tasks, web search, email.
- System skills are built-in with no external dependencies.
- CLI skills are SKILL.md definitions that drive installed command-line tools via bash — if `gh` is installed, a GitHub skill just works; if `kubectl` is installed, so does a Kubernetes skill. The installed CLI tool *is* the connector — no Go code required.
- Execution skills run code in sandboxes.

Popular CLI skills get promoted to native over time as the project grows. But the practical implication today is that the skill ecosystem is much larger than what OnlyAgents ships with — anyone with `restic`, `docker`, `ffmpeg`, or any other CLI tool already has the connector; they just need the SKILL.md.

**Executive agent** — top of the hierarchy. Receives all user messages, decides whether to answer directly, delegate to a sub-agent, or decompose into a multi-step workflow. Rewrites requests with full context before delegating.

**Sub-agents** — specialized and capability-bounded. Each has its own LLM client (provider and model configured independently), its own soul, and access only to relevant tools. Trust and cost boundaries live at the agent level — run a cheap fast model for CRUD, a more capable one for reasoning.

**Kernel** — the runtime. Agent lifecycle, event bus, skill registry, connector wiring, graceful shutdown. Agents don't communicate directly — everything routes through the kernel.

**Soul** — agent personality and behavioral config in YAML. Communication style, delegation acknowledgments, routing logic, refusal rules. Shapes LLM behavior without prompt engineering in code.

**Workflow engine** — cross-agent coordination. When a task spans multiple agents, the executive creates a workflow with ordered steps and tracked dependencies. Individual agents handle multi-step operations internally.

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

All summarization runs in-process via a cron scheduler. On startup, catch-up logic runs any missed jobs. The facts database uses SQLite FTS5 for search — no vector embeddings required.

## Default Agents

Some agents that we believe applies for everyone, ship out of the box. Their configurations and souls are tuned — modify them if you want, but the defaults are a solid starting point.

**Executive** (`configs/agents/executive.yaml`) — the orchestrator. Every message goes through here first. Knows all registered sub-agents and their capabilities. Decides routing. Never delegates when it can answer directly.

**Productivity / Friday** (`configs/agents/productivity.yaml`) — handles calendar, notes, reminders, and tasks natively. No third-party integrations required. Uses `claude-haiku` — CRUD operations don't need a heavy model.

**General** (`configs/agents/general.yaml`) — the fallback. When no specialized agent matches a task, General handles it. Has access to all skills in the local registry. If no local skill covers the request, it queries online skill registries (Clawhub and others), downloads and converts matching skills on the fly, and uses them.

Rules:
- Only one executive and one general agent can exist in a deployment.
- Agent IDs must be unique across all config files.
- Additional specialized agents can be added by dropping a config file into `~/.onlyagents/configs/agents/`.

## The Web Interface (OAChannel)

OnlyAgents ships with a built-in web interface connected over a single persistent WebSocket. No polling. One connection carries everything — chat, streaming responses, real-time agent activity, tool call notifications, delegations between agents, and proactive messages like reminders and daily digests.

**Chat panel** — conversations with the executive. Responses stream token by token. Sessions are persistent — the interface resumes your last conversation on return. Reconnects and page reloads are transparent.

**Council room** — live view of the system as it works. Which agents are active, what they're doing, tool calls in flight, delegations as they happen.

OAChannel is architecturally identical to any other channel — agents have no special awareness of it. The REST API and WebSocket protocol are fully documented. Build your own interface if you want.

## Modes

**Agent mode** — headless. No HTTP server. Smallest footprint.

```
onlyagents agent run
```

**Server mode** — full stack. REST API, embedded web UI, WebSocket, Telegram.

```
onlyagents server start
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

Prerequisites: Go 1.21+, Node.js 18+ (for the embedded web UI)

```bash
git clone https://github.com/sriramsme/onlyagents
cd onlyagents
make install
```

The Makefile builds the web UI first, then installs the binary. The interface is embedded at compile time — one binary, nothing else to serve.

### Custom Builds

Pre-built binaries include all channels, all LLM providers, and the `env` vault backend. If you're building from source and want a smaller binary, build tags let you include only what you need.

**Channels** — by default all channels are included. To include only what you use:

```bash
# Telegram only (no OAChannel/web UI)
go install -tags channel_telegram ./cmd/onlyagents/

# OAChannel only (no Telegram). You need oachannel for server mode.
go install -tags channel_onlyagents ./cmd/onlyagents/
```

**LLM providers** — by default all providers are included. For example, to include only Anthropic:

```bash
go install -tags llm_anthropic ./cmd/onlyagents/
```

**Vault backends** — `env` is always included. HashiCorp, AWS, and GCP vault backends are available but not yet tested in production — opt in explicitly:

```bash
go install -tags vault_hashicorp ./cmd/onlyagents/  #untested
go install -tags vault_aws ./cmd/onlyagents/        # untested
go install -tags vault_gcp ./cmd/onlyagents/        # untested
```

Combine tags as needed:

```bash
go install -tags "llm_anthropic channel_telegram vault_hashicorp" ./cmd/onlyagents/
```

## Configuration

All config and runtime data lives in `~/.onlyagents/`.

```
~/.onlyagents/
├── agents
│   ├── executive.yaml
│   ├── general.yaml
│   ├── productivity.yaml
│   └── researcher.yaml
├── cache
│   └── skills
├── channels
│   └── telegram.yaml
├── config.yaml
├── connectors           # Custom/downloaded connectors
│   ├── Brave.yaml
│   ├── DuckDuckGo.yaml
│   └── Perplexity.yaml
├── logs
├── marketplace
├── onlyagents.db        # SQLite — all persistent data
├── server.yaml
├── skills               # Custom/downloaded skill files
│   ├── github.md
│   └── weather.md
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
  api_key_vault: "anthropic/api_key"
```

Each agent specifies its own provider, model, and vault key independently. Full reference: [docs/agents.md](docs/agents.md).

## CLI

```
onlyagents server start            Start server (API + web UI + agent kernel)
onlyagents agent run               Run agent kernel only (headless)

onlyagents auth reset              Generate new password
onlyagents auth set-password       Change password interactively
onlyagents auth status             Show auth configuration

onlyagents models list             List all supported models across providers
onlyagents models info <model>     Detailed model info and pricing
onlyagents models compare m1 m2    Side-by-side comparison
onlyagents models filter           Filter by capability (--tools, --vision, etc.)

onlyagents convert <file> -n <n>   Convert raw skill to canonical SKILL.md format
```

On first `server start`, credentials are generated and printed once. Reset with `onlyagents auth reset`.

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
