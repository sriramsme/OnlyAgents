# OnlyAgents — Architecture

**Version:** 0.2.0
**Updated:** 2026-03

---

## Table of Contents

1. [Overview](#overview)
2. [System Diagram](#system-diagram)
3. [Kernel](#kernel)
4. [Agents](#agents)
5. [Skills](#skills)
6. [Connectors](#connectors)
7. [Memory](#memory)
8. [Storage](#storage)
9. [Channels & OAChannel](#channels--oachannel)
10. [LLM Layer](#llm-layer)
11. [Vault & Secrets](#vault--secrets)
12. [Soul](#soul)
13. [Workflow Engine](#workflow-engine)
14. [A2A — Agent-to-Agent Protocol](#a2a--agent-to-agent-protocol)
15. [Data Flow](#data-flow)
16. [Project Structure](#project-structure)

---

## Overview

OnlyAgents is a multi-agent runtime written in Go. A single binary starts the entire system — kernel, agents, memory scheduler, channel listeners, and (in server mode) an HTTP/WebSocket server.

**Core design decisions:**

- **Single-user, local-first data model.** One SQLite database. No agentID on user-facing tables (calendar, notes, tasks, reminders) — the database belongs to one person. AgentID scoping only appears where it genuinely matters: conversation history, agent state, memory summaries.
- **Agent = LLM client + system prompt + tool subset + limits.** Trust and cost boundaries live at the agent level. Sub-agents access only the tools relevant to their domain. A cheap fast model for CRUD; a more capable one for reasoning.
- **Skills call connectors, not APIs.** A skill owns business logic. A connector owns the API integration. A native skill paired with a native connector requires no external service.
- **CLI skills make any installed tool a skill.** The installed binary is the connector. No Go code required.
- **Memory is automatic.** Conversation history rolls up through daily → weekly → monthly → yearly summaries via in-process cron. Agents always have relevant context without manual configuration.

---

## System Diagram

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

---

## Kernel

`pkg/kernel/`

The kernel is the runtime. It owns nothing domain-specific — it wires everything together and manages lifecycle.

**Responsibilities:**
- Start and stop all agents in dependency order
- Own the internal event bus (`core.UIBus`) — all inter-agent communication flows through it
- Initialize and inject the storage layer (`pkg/storage/sqlite`)
- Instantiate native connectors directly (bypassing the factory — see [Connectors](#connectors))
- Register skills and wire them to agents
- Start the memory scheduler
- Route inbound channel messages to the executive agent
- Expose the bus and kernel reference to the API layer (server mode)

Agents do not hold references to each other. Everything routes through the kernel event bus.

---

## Agents

`pkg/agents/`

An agent is: LLM client + system prompt + soul + tool subset + execution limits.

### Executive Agent

One per deployment. Every message enters here. The executive determines whether to answer directly, delegate to a sub-agent, or decompose into a workflow. Before delegating, it rewrites the user's request with full context — the sub-agent receives a precise, enriched instruction, not the raw user message.

The executive knows the capability set of every registered sub-agent. It never delegates when it can answer directly, and never creates a workflow when a single sub-agent can handle everything.

### Productivity Agent (Friday)

Handles calendar, notes, reminders, and tasks. All four domains are native — no external service required. Configured with `claude-haiku` and `enable_early_stopping: false`. CRUD operations don't benefit from early stopping or heavy models.

### General Agent

The fallback. When no specialist matches a request, General handles it with access to the full local skill registry. If no local skill covers the task, it queries Clawhub and other online registries, downloads and converts matching skill definitions, and uses them in the same request.

### Rules

- One executive. One general. Any number of specialized agents.
- Agent IDs must be unique across all config files.
- Add a specialized agent by dropping a config file into `~/.onlyagents/configs/agents/`.

### Execution Loop

Each agent runs a tool call loop:

1. Build messages: system prompt → memory context → conversation history
2. LLM call with registered tools
3. For each tool call: look up skill via `toolSkillMap`, execute, persist result, append to messages
4. If `ExecHalt` returned: send `DirectMessage`, stop loop
5. If no tool calls: final LLM response, persist, return

**`ToolExecution` and `ExecControl`** — every tool call returns a `ToolExecution` struct with a control signal:
- `ExecContinue` — normal, keep looping
- `ExecHalt` — stop the loop, optionally send a direct message to the user

`ExecHalt` is used in two places: delegation acknowledgments (when the executive hands off to a sub-agent) and workflow submission (when a workflow is created and the step list is sent to the user). In both cases, the loop halts immediately after the ack is sent — no second LLM call.

**History windowing** — `GetHistory` uses turn-based windowing (`DefaultHistoryTurns = 10`). A turn is one user message plus all subsequent assistant/tool messages until the next user message. An agent making five tool calls in one turn still counts as one turn. Tool results are truncated to 500 chars on retrieval; full content is preserved in the database for the memory summarizer.

## Skills

`pkg/skills/`

Skills implement the business logic exposed to the LLM. They define tools, handle tool execution, and interact with connectors for data access or external services.

### Types

**Native**

Implemented in Go and integrated through connector interfaces. Native skills provide stable integrations with core capabilities and can operate with either local or service connectors.

Examples:

* `calendar`
* `notes`
* `reminders`
* `tasks`
* `web_search`
* `email`

---

**System**

Internal framework skills used for orchestration and agent coordination. These are not external integrations but core capabilities of the runtime.

Examples:

* `task_complete`
* `delegate_to_agent`
* `create_workflow`
* `find_best_agent`

These are primarily used by the executive agent as meta-tools.

---

**CLI**

Defined by a `SKILL.md` specification and executed through installed command-line tools.

If the CLI tool exists on the system, the skill works automatically. The CLI executable effectively acts as the connector.

Examples:

* `gh` → GitHub automation
* `kubectl` → Kubernetes operations
* `ffmpeg` → media processing
* `restic` → backups

This design allows the skill ecosystem to grow far beyond what ships with OnlyAgents. Over time, widely used CLI skills may be promoted to native implementations.

### SkillName

```go
type SkillName string
```

A typed constant used for internal routing.

`ToolDef.Skill` carries `json:"-"` so the skill identifier is never exposed to the LLM. The agent's `toolSkillMap` is constructed at initialization from tool definitions, replacing the previous string-splitting approach.

### Custom Skills

Users can extend the system by adding new CLI-based skills.

Place a `SKILL.md` file in:

```
~/.onlyagents/skills/
```

The kernel loads these definitions on startup.

The `onlyagents convert` command can convert raw skill descriptions into canonical `SKILL.md` format using the configured LLM provider.

## Connectors

`pkg/connectors/`

Connectors handle integrations with data sources and services. They are responsible for authentication, rate limiting, retries, and error handling.

### Local Connectors

Local connectors operate entirely on the local machine and do not depend on external services.

They are instantiated directly by the kernel and receive the storage layer as a dependency.

Example initialization:

```
kernel.initLocalConnectors(store) →
    local.CalendarConnector(store)
    local.NotesConnector(store)
    local.RemindersConnector(store)
    local.TasksConnector(store)
```

These connectors interact directly with SQLite-backed storage. Business logic specific to the connector (for example `FindAvailableSlots`) lives inside the connector, while the storage layer remains simple.

### Service Connectors

Service connectors integrate with external APIs such as search providers or email services.

These connectors are constructed through the factory pattern. The factory receives configuration and vault access, resolves credentials, and initializes the connector.

Examples:

* DuckDuckGo
* Brave Search
* Gmail

### Connector Interfaces

Four productivity interfaces are defined in `pkg/connectors/`:

* `CalendarConnector`
* `NotesConnector`
* `RemindersConnector`
* `TasksConnector`

Skills cast `deps.Connectors` to the appropriate interface during `Initialize()`.

Each connector implementation includes compile-time interface checks to ensure compatibility.

## Memory

`pkg/memory/`

Memory is automatic. No configuration required. The `MemoryManager` starts with the kernel and runs in the background.

### Hierarchy

```
┌────────────────────────────────────────────────────────────────┐
│  Working Memory  ·  current conversation                       │
│                          ↓  persisted on every message         │
│  Short-term      ·  last 4 hours  ·  30-day retention          │
│                          ↓  LLM summarization at 23:59 daily   │
│  Daily Summaries ·  key events, topics  ·  90-day retention    │
│                          ↓  every Sunday 00:00                 │
│  Weekly Summaries  ·  themes, patterns  ·  1-year retention    │
│                          ↓  1st of month 00:00                 │
│  Monthly Summaries  ·  highlights, stats  ·  5-year retention  │
│                          ↓  Dec 31 23:30                       │
│  Yearly Archives  ·  compressed  ·  permanent                  │
├────────────────────────────────────────────────────────────────┤
│  Facts Database  ·  entity knowledge  ·  permanent             │
│  Extracted during daily summarization with confidence scores.  │
└────────────────────────────────────────────────────────────────┘
```

All summaries are tagged `agent_id = "__session__"` — one summary per session, not per sub-agent.

### Scheduler

`robfig/cron/v3`, in-process. On startup, catch-up logic checks the `job_runs` table and immediately runs any missed jobs (handles the machine-was-off-at-midnight case).

An additional job runs nightly at 20:00: the daily digest. Fetches tomorrow's tasks and reminders, formats as Markdown, and delivers via the active channel. Skipped on restart (stale digest is useless).

### Retrieval

`GetRelevantMemory` combines: last 4 hours of messages, today's daily summary (or most recent weekly as fallback), and FTS5-matched facts for the current query. Returns a `MemoryContext` injected as a second system message in the agent's execution loop.

### Facts

Extracted from daily messages in the same LLM pass as the daily summary. Stored with confidence scores. Conflicting facts set `superseded_by` on the old record. FTS5 virtual table on `facts` for search — no vector embeddings required.

---

## Storage

`pkg/storage/`

### Interface

Master `Storage` interface composed from eleven sub-interfaces:

```
ConversationStore   MessageStore     MemoryStore
FactStore           AgentStateStore  CalendarStore
NoteStore           ReminderStore    ProjectStore
TaskStore           JobRunStore
```

### SQLite Backend

`pkg/storage/sqlite/` using `modernc.org/sqlite` (pure Go, no CGO) and `github.com/jmoiron/sqlx`.

Pragmas on every open:
```sql
PRAGMA journal_mode=WAL
PRAGMA foreign_keys=ON
```

Migrations run on startup from an embedded `migrations/` folder, tracked in `schema_migrations`. Never modify applied migrations — always add new ones.

```
001_init.sql          conversations, messages, agent_state
002_memory.sql        summary tables, facts + FTS5 triggers
003_productivity.sql  calendar_events, notes, reminders + FTS5
004_job_runs.sql      job_runs
005_tasks.sql         projects, tasks + FTS5
```

### Key Design Decision — No AgentID on User Tables

`CalendarEvent`, `Note`, `Reminder`, `Project`, `Task` have no `agent_id`. The database belongs to one user. AgentID only appears on `messages`, `agent_state`, and memory summary tables — where it genuinely scopes data to a specific agent within the multi-agent system. A2A is a communication protocol, not a data-sharing model. No other user's data enters this database.

### Custom Types

`pkg/storage/sqltypes.go` — native Go types at call sites, no manual marshal/unmarshal:

- `DBTime` — non-nullable `time.Time` as RFC3339Nano TEXT
- `NullDBTime` — nullable `time.Time`
- `JSONSlice[T]` — generic slice as JSON array TEXT
- `JSONMap` — `map[string]any` as JSON object TEXT

---

## Channels & OAChannel

`pkg/channels/`

Channels are how users and agents communicate. All channels are equal — agents have no special awareness of which channel a message arrived from. A message from OAChannel is handled identically to one from Telegram.

**Telegram** — primary external channel. Responses and streaming where the platform supports it.

**OAChannel** — the native channel for the web interface. Delivers over a single persistent WebSocket. Carries everything: streaming responses, proactive messages, the council event stream. Not a special case — just a channel that happens to support more of the platform's capabilities.

**Council room** — OAChannel-exclusive. A live event feed of agent activity: which agents are running, tool calls in progress, delegations as they happen, workflow step status. Powered by the same `UIBus` events that drive the kernel internally.

**Sessions** — OAChannel sessions are persistent. The interface resumes the correct conversation on reconnect or page reload using a session ID stored in the browser. No visible delay.

---

## LLM Layer

`pkg/llm/`

Unified interface over multiple LLM providers. Each agent holds its own client — provider, model, and API key are configured per agent.

**Providers:** Anthropic, OpenAI, Gemini. Registered at startup via blank imports from `pkg/llm/bootstrap`.

**Model registry** — each provider exports a `ModelRegistry` with capabilities (context window, max tokens, cost, tool support, vision support, etc.). The `onlyagents models` commands query this registry directly, no API calls required.

**Message format** — canonical OpenAI-style format internally. Provider clients convert to/from their native formats.

**Tool calling** — all providers normalized to the same interface. `AssistantMessageWithTools`, `ToolResultMessage` helpers handle the conversion.

---

## Vault & Secrets

`pkg/asec/vault/`

All secrets go through the vault abstraction. No hardcoded credentials anywhere.

**Backends:** `env` (`.env` file or exported environment variables), `hashicorp` (HashiCorp Vault), `aws` (AWS Secrets Manager), `gcp` (GCP Secret Manager).

**Key path convention:** `anthropic/api_key`, `openai/api_key`, `telegram/bot_token`. For the `env` backend, `ANTHROPIC_API_KEY` maps to `anthropic/api_key`.

The vault is loaded once at startup and injected into the connector factory and LLM client constructors. Skills and agents never access the vault directly.

---

## Soul

`pkg/soul/`

Agent personality and behavioral configuration in YAML. Applied at runtime to shape LLM behavior — communication style, delegation acknowledgment templates, routing decision rules, refusals.

**Delegation acknowledgments** are drawn from a soul-configured list with `{agent_name}` substitution, randomized per delegation. This keeps the executive's responses from feeling mechanical.

**Executive soul** encodes the routing decision tree: answer directly if no specialist needed; delegate if one agent covers everything; create a workflow only if the task genuinely spans multiple agents. These are rules, not suggestions — the soul is the source of truth for routing behavior.

Sub-agent souls include relationship context (`relationship.to_executive`) that shapes how they format responses for relay versus direct delivery.

---

## Workflow Engine

`pkg/core/`

When a task spans multiple agents, the executive creates a `Workflow` — a named, ordered set of `WorkflowStep` records with dependency tracking.

**`WorkflowStep`** (not `WorkflowTask` — the name "task" is reserved for the productivity to-do system):

```
WorkflowStep {
    ID
    Name
    Description
    RequiredCapabilities []string
    DependsOn            []string
}
```

On workflow submission, `requestWorkflow` returns `ExecDone` with a formatted step-list acknowledgment. The loop halts immediately — the user sees the plan, the kernel begins execution. No second LLM call after submission.

Single-agent multi-step operations are never workflows. The executive delegates the full request to the sub-agent, which handles multi-step execution internally.

---

## A2A — Agent-to-Agent Protocol

> In development. Protocol is being finalized.

### Architecture

A2A splits into two layers:

**Protocol layer** — standalone Go module (`github.com/onlyagents/a2a`). No OnlyAgents-specific dependencies. Envelope format, Ed25519 signing and verification, keypair management, the HTTP client that sends signed envelopes, the HTTP handler that receives and verifies them. Any agent framework can import this.

**Integration layer** — `pkg/a2a/` in OnlyAgents. Thin wrapper. Routes verified inbound envelopes to the kernel's agent dispatcher. Wires the A2A server into the existing HTTP server.

```
a2a/                          standalone module
├── envelope.go               Envelope type, Sign(), Verify()
├── keypair.go                Ed25519 generate, load, save
├── client.go                 Send signed envelopes to any endpoint
├── server.go                 HTTP handler: verify + callback
├── discovery.go              Discovery network API client
└── types.go

pkg/a2a/                      OnlyAgents integration
├── dispatcher.go             Routes verified envelopes to agents
└── handler.go                Wires a2a.Server into HTTP server
```

The protocol server calls a callback on verified receipt. OnlyAgents provides that callback. The protocol module never imports kernel.

### Message Format

All messages are signed JSON envelopes. The payload is optionally encrypted with the recipient's public key.

```
Envelope {
    Version    "1.0"
    ID         UUID (deduplication)
    From       sender agent_id
    To         recipient agent_id
    Type       message | request | response | event
    Payload    JSON, optionally encrypted
    Encrypted  bool
    Timestamp  Unix (anti-replay)
    ExpiresAt  Unix
    Signature  base64 Ed25519 over canonical fields
}
```

### Trust Model

- Ed25519 keypairs generated on first run, stored in `~/.onlyagents/agent.key` and `agent.pub`. Never regenerated — the keypair is the agent's identity.
- A lightweight discovery network (separate hosted service) stores agent public keys and endpoint URLs. It acts as a keyserver only — it never sees message content.
- Connections require mutual acceptance through the discovery network before messages are exchanged.
- Inbound messages are verified against the sender's public key fetched from the discovery network. Expired or duplicate envelopes are rejected.

### Requirement

A2A requires a publicly reachable HTTPS endpoint. Options: VPS, Cloudflare Tunnel, or similar. Built-in tunnel support is planned for a future release.

---

## Data Flow

A complete request from user input to response, showing how all layers interact.

```
User: "Schedule a meeting with Alice tomorrow morning"
        │
        ▼
  Channel (OAChannel / Telegram)
  Delivers AgentExecutePayload to kernel event bus
        │
        ▼
  Executive Agent
  1. Retrieves memory context (recent messages + daily summary + facts)
  2. Builds messages: [system prompt] [memory context] [history] [user message]
  3. LLM call with tools: delegate_to_agent, create_workflow, answer tools
  4. LLM returns: delegate_to_agent → productivity agent
  5. Executive rewrites request with context:
     "Schedule a meeting with Alice tomorrow. Alice prefers mornings.
      User has a 10am slot open."
  6. ExecHalt → sends delegation ack to user, loop stops
        │
        ▼
  Kernel event bus routes to Productivity Agent (Friday)
  1. Retrieves memory context
  2. LLM call with calendar/task tools
  3. LLM calls: calendar_list_events (check tomorrow), calendar_create_event
  4. CalendarSkill → CalendarConnector (native) → SQLite write
  5. Tool results persisted, loop continues
  6. LLM final response: "Scheduled for 9am tomorrow with Alice."
  7. ConversationManager persists assistant message
        │
        ▼
  Response routed back through kernel to originating channel
  OAChannel: streamed token by token over WebSocket
  Telegram: sent as message
        │
        ▼
  Memory (async, non-blocking)
  User and assistant messages saved to messages table
  Facts extracted at 23:59 in nightly summarization pass
```

---

## Project Structure

```
cmd/
└── onlyagents/
    └── main.go

internal/
├── api/                    HTTP server, handlers, WebSocket
├── auth/                   bcrypt auth, sessions, IP rate limiting
├── bootstrap/              internal wiring, defaults, setsup ~/.onlyagents dir if not present
└── config/                 ServerConfig, vault loading, paths

pkg/
├── a2a/                    A2A integration layer (dispatcher, handler)
├── agents/                 Agent struct, execute loop, tool routing
├── asec/
│   └── vault/              Secret provider abstraction
├── channels/               Channel interface, Telegram, OAChannel
├── connectors/             Connector interfaces + factory
│   └── native/             CalendarConnector, NotesConnector, etc.
├── core/                   Event types, UIBus, Workflow, Capability
├── kernel/                 Runtime, wiring, lifecycle
├── llm/                    LLM client interface + providers
│   └── providers/
│       ├── anthropic/
│       ├── openai/
│       └── gemini/
├── memory/                 ConversationManager, MemoryManager,
│                           summarizer, scheduler, retrieval
├── skills/                 Skill interface, registry, base skill
│   ├── calendar/
│   ├── notes/
│   ├── reminders/
│   ├── tasks/
│   ├── websearch/
│   ├── email/
│   └── cli/                CLI skill runner + convert
├── soul/                   Soul definitions, YAML loader
├── storage/                Storage interface, types, sqltypes
│   └── sqlite/             SQLite backend, migrations
└── tools/                  ToolDef, ToolCall, SkillName


web/                        Vite frontend source
ui/                         embed.go (embeds web/dist at build time)

~/.onlyagents/              Runtime data directory
├── vault.yaml
├── configs/agents/         User agent configs
├── skills/                 User skill files
├── onlyagents.db           SQLite database
├── agent.key               Ed25519 private key (A2A identity)
└── agent.pub               Ed25519 public key
```
