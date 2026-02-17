# OnlyAgents: Complete Architecture Overview

**Version:** 0.1.0
**Last Updated:** 2025-02-16
**Purpose:** Master reference document for the OnlyAgents framework

---

## Table of Contents

1. [Vision & Goals](#vision--goals)
2. [High-Level Architecture](#high-level-architecture)
3. [Core Components](#core-components)
4. [Memory System](#memory-system)
5. [Skills vs Connectors](#skills-vs-connectors)
6. [Agent Types](#agent-types)
7. [Data Flow](#data-flow)
8. [Project Structure](#project-structure)
9. [Technology Stack](#technology-stack)
10. [Implementation Phases](#implementation-phases)

---

## Vision & Goals

### What is OnlyAgents?

OnlyAgents is a **modular, secure, open infrastructure for autonomous AI agents**. It enables:

- 🤖 **Multi-agent orchestration** - Executive agents coordinate specialized sub-agents
- 🧠 **Human-like memory** - Hierarchical memory from short-term to long-term
- 🔧 **Extensible skills** - Plugin architecture for domain capabilities
- 🔌 **Universal connectors** - Standardized integration with external services
- 🔒 **Enterprise security** - Cryptographic identity, capabilities, audit trails
- 📡 **Agent-to-agent communication** - Open protocol for agent discovery and messaging

### Core Principles

1. **Modular Architecture** - Each component is independent and composable
2. **Separation of Concerns** - Clear boundaries between layers
3. **Human-like Memory** - Hierarchical compression mimicking biological memory
4. **Specialized Intelligence** - Each agent is an expert in its domain
5. **Open Standards** - Interoperable protocols, no vendor lock-in

---

## High-Level Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                          USER INTERFACE                              │
│                     (CLI, API, Web, Mobile)                          │
└─────────────────────────────────────────────────────────────────────┘
                                  ↓
┌─────────────────────────────────────────────────────────────────────┐
│                      EXECUTIVE AGENT (Orchestrator)                  │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ LLM (Claude Opus 4.6) - Strategic Planning & Coordination    │  │
│  │ Memory: Full context, goals, past orchestrations            │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                    ↓              ↓              ↓
      ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
      │ Calendar Agent  │  │  Email Agent    │  │ Research Agent  │
      │                 │  │                 │  │                 │
      │ LLM: Sonnet 4.5 │  │ LLM: Sonnet 4.5 │  │ LLM: GPT-4     │
      │ Skills:         │  │ Skills:         │  │ Skills:         │
      │ - Scheduling    │  │ - Email Mgmt    │  │ - Web Search    │
      │ - Availability  │  │ - Drafting      │  │ - Analysis      │
      │                 │  │                 │  │                 │
      │ Memory:         │  │ Memory:         │  │ Memory:         │
      │ - Schedule      │  │ - Conversations │  │ - Research      │
      │ - Preferences   │  │ - Email style   │  │ - Findings      │
      └────────┬────────┘  └────────┬────────┘  └────────┬────────┘
               ↓                     ↓                     ↓
      ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
      │    SKILLS       │  │    SKILLS       │  │    SKILLS       │
      │  (Business      │  │  (Business      │  │  (Business      │
      │   Logic)        │  │   Logic)        │  │   Logic)        │
      └────────┬────────┘  └────────┬────────┘  └────────┬────────┘
               ↓                     ↓                     ↓
      ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
      │  CONNECTORS     │  │  CONNECTORS     │  │  CONNECTORS     │
      │  (API Wrappers) │  │  (API Wrappers) │  │  (API Wrappers) │
      └────────┬────────┘  └────────┬────────┘  └────────┬────────┘
               ↓                     ↓                     ↓
      ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
      │ Google Calendar │  │      Gmail      │  │   Web APIs      │
      │      API        │  │       API       │  │  (Search, etc)  │
      └─────────────────┘  └─────────────────┘  └─────────────────┘
```

### Layer Responsibilities

| Layer | Purpose | Examples |
|-------|---------|----------|
| **Executive Agent** | Task decomposition, orchestration, synthesis | Breaks "plan dinner party" into calendar + email + research tasks |
| **Specialized Agents** | Domain expertise, focused execution | Calendar Agent only handles scheduling |
| **Skills** | Business logic, multi-connector orchestration | Email skill uses both Gmail + Calendar connectors |
| **Connectors** | API integration, auth, retries, rate limiting | Gmail connector wraps Gmail API with OAuth |
| **External Services** | Actual platforms | Google Calendar, Gmail, Discord, etc. |

---

## Core Components

### 1. Kernel (Agent Runtime)

**Location:** `pkg/kernel/`

**Purpose:** Core agent execution engine

**Key Features:**
- Agent lifecycle management (start/stop/health checks)
- Message routing and processing
- Skill registration and execution
- LLM integration with memory
- Tool calling orchestration
- Conversation state management

**Main Types:**
```go
type Agent struct {
    id         string
    llmClient  llm.Client
    memory     *memory.MemoryManager
    skills     *SkillRegistry
    connectors *ConnectorRegistry
    state      *StateManager
    security   *SecurityManager
}
```

**Key Methods:**
```go
// Execute user request with full memory context
ExecuteWithMemory(ctx, userMessage string) (string, error)

// Execute with explicit context (for specialized agents)
ExecuteWithContext(ctx, instruction, context string) (string, error)

// Helper for skills to use LLM
AskLLM(ctx, system, prompt string) (string, error)

// Register skills
RegisterSkill(skill Skill) error

// End conversation and save state
EndConversation(ctx) error
```

---


### 2. ASec — Agent Security Protocol

Zero-trust security framework for autonomous agents. Every interaction requires:

* **Authentication** — cryptographic agent identity (Ed25519 keypairs + CA-signed certs)
* **Authorization** — capability-based access control (fine-grained, resource/action scoped)
* **Audit** — immutable, signed event logs

**Key Features**

* **Signed Messages** — all payloads cryptographically signed (tamper-proof, non-repudiable)
* **Capability Model** — agents declare what they *can do*, not roles
* **Prompt Injection Defense** — sanitization, pattern detection, isolation, limits
* **Credential Security** — encrypted at rest (AES-256), vault-backed (e.g., HashiCorp Vault, AWS Secrets Manager), rotation + audit trails
* **Sandboxed Execution** — isolated runtime, network/file boundaries, memory + time limits
* **Tamper-Proof Audit Trail** — signed `AuditEvent` records for compliance and debugging

Security is foundational—not optional.

---

### 3. A2A — Agent-to-Agent Protocol

Open protocol for agent discovery, communication, and collaboration — the HTTP/SMTP layer for agents.

**Design Principles**

* **Discoverable** — agents publish manifests (`/.well-known/a2a-manifest.json`)
* **Typed** — strict input/output schemas
* **Asynchronous** — non-blocking, multi-turn capable
* **Reliable** — message IDs, retries, TTLs, acknowledgments
* **Extensible** — forward-compatible versioning

**Core Elements**

* **Agent Manifest** — declares capabilities, schemas, endpoints, and security requirements
* **Structured Messages** — versioned, signed, conversation-linked (`ConversationID`)
* **Multi-Turn Conversations** — first-class workflow chaining across agents
* **Transport-Agnostic** — WebSocket, HTTP/2, gRPC, or queues (e.g., RabbitMQ, Apache Kafka, NATS)

A2A enables secure, interoperable, and scalable agent ecosystems.

---

### 4. LLM Package (Language Model Interface)

**Location:** `pkg/llm/`

**Purpose:** Unified interface to multiple LLM providers

**Key Features:**
- Provider abstraction (Anthropic, OpenAI, Gemini, Local)
- Factory pattern for dynamic provider creation
- Tool calling support (function calling)
- Extended thinking support (Sonnet 4.5, Opus 4.6)
- Message format conversion (OpenAI-style canonical format)
- Provider registry for extensibility

**Architecture:**
```
┌──────────────────────────────────────────────────┐
│              LLM Factory                          │
│  - Creates clients from config                   │
│  - Manages multiple providers                    │
│  - Handles API key resolution                    │
└──────────────────────────────────────────────────┘
                        ↓
        ┌───────────────┴───────────────┐
        ↓                               ↓
┌──────────────────┐           ┌──────────────────┐
│ Anthropic Client │           │  OpenAI Client   │
│ - Claude Opus    │           │  - GPT-4         │
│ - Claude Sonnet  │           │  - GPT-4 Turbo   │
│ - Claude Haiku   │           │                  │
└──────────────────┘           └──────────────────┘
```

**Core Types:**
```go
type Client interface {
    Chat(ctx context.Context, req *Request) (*Response, error)
    Provider() Provider
    Model() string
}

type Request struct {
    Messages    []Message
    Tools       []ToolDef      // For function calling
    MaxTokens   int
    Temperature float64
}

type Response struct {
    Content          string      // Final text response
    ReasoningContent string      // Extended thinking (Sonnet 4.5)
    ToolCalls        []ToolCall  // Function calls
    Usage            Usage       // Token counts
}
```

**Tool Calling:**
```go
// LLM can call functions (skills)
type ToolDef struct {
    Type     string      // "function"
    Function FunctionDef
}

type FunctionDef struct {
    Name        string
    Description string
    Parameters  map[string]any  // JSON Schema
}
```

---

### 5. Memory System (Hierarchical Memory)

**Location:** `pkg/memory/`

**Purpose:** Human-like memory with automatic compression

**Architecture:**

```
┌─────────────────────────────────────────────────────────────────┐
│                    MEMORY HIERARCHY                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Working Memory (Current Conversation)                           │
│  ├─ Messages: Individual user/assistant messages                │
│  ├─ Context: Active conversation state                          │
│  └─ Retention: Current session                                  │
│                          ↓ (save on message)                    │
├─────────────────────────────────────────────────────────────────┤
│  Short-term Memory (Recent History)                             │
│  ├─ Messages: Last 4 hours of conversation                      │
│  ├─ Storage: messages table in SQLite                           │
│  └─ Retention: 30 days                                          │
│                          ↓ (summarize at 11:59 PM daily)        │
├─────────────────────────────────────────────────────────────────┤
│  Daily Summaries (Medium-term)                                  │
│  ├─ Content: LLM-generated daily summary                        │
│  ├─ Includes: Key events, topics, main activities               │
│  ├─ Storage: daily_summaries table                              │
│  └─ Retention: 90 days                                          │
│                          ↓ (summarize every Sunday)             │
├─────────────────────────────────────────────────────────────────┤
│  Weekly Summaries                                               │
│  ├─ Content: Themes, patterns, achievements                     │
│  ├─ Aggregated from: 7 daily summaries                          │
│  ├─ Storage: weekly_summaries table                             │
│  └─ Retention: 1 year                                           │
│                          ↓ (summarize on 1st of month)          │
├─────────────────────────────────────────────────────────────────┤
│  Monthly Summaries (Long-term)                                  │
│  ├─ Content: Major highlights, progress, statistics             │
│  ├─ Aggregated from: 4-5 weekly summaries                       │
│  ├─ Storage: monthly_summaries table                            │
│  └─ Retention: 5 years                                          │
│                          ↓ (archive at end of year)             │
├─────────────────────────────────────────────────────────────────┤
│  Yearly Archives (Permanent)                                    │
│  ├─ Content: Compressed yearly summary                          │
│  ├─ Aggregated from: 12 monthly summaries                       │
│  ├─ Storage: yearly_archives table                              │
│  └─ Retention: Forever (highly compressed)                      │
├─────────────────────────────────────────────────────────────────┤
│  Facts Database (Entity Memory)                                 │
│  ├─ Content: Permanent facts about entities                     │
│  ├─ Examples: "Alice prefers morning meetings"                  │
│  ├─ Storage: facts table                                        │
│  └─ Retention: Forever (with confidence scores)                 │
└─────────────────────────────────────────────────────────────────┘
```

**Database Schema:**

```sql
-- Working memory (current conversations)
conversations (
    id, agent_id, started_at, ended_at,
    context, summary
)

-- Short-term memory (raw messages, 30 days)
messages (
    id, conversation_id, role, content,
    reasoning_content, tool_calls, timestamp
)

-- Daily summaries (90 days)
daily_summaries (
    id, agent_id, date, summary,
    key_events, topics, conversation_ids
)

-- Weekly summaries (1 year)
weekly_summaries (
    id, agent_id, week_start, week_end,
    summary, themes, achievements
)

-- Monthly summaries (5 years)
monthly_summaries (
    id, agent_id, year, month,
    summary, highlights, statistics
)

-- Yearly archives (permanent)
yearly_archives (
    id, agent_id, year,
    summary, major_events, statistics
)

-- Entity facts (permanent knowledge)
facts (
    id, agent_id, entity, entity_type, fact,
    confidence, source_conversation_id,
    first_seen, last_confirmed
)

-- Agent state (current state)
agent_state (
    agent_id, current_conversation_id,
    context, preferences, goals, last_active
)
```

**Automatic Summarization (Cron Jobs):**

```
11:59 PM Daily:
  ├─ Collect all messages from today
  ├─ Use LLM to generate summary
  ├─ Extract key events, topics
  └─ Save to daily_summaries

Sunday 12:00 AM:
  ├─ Collect past 7 daily summaries
  ├─ Use LLM to generate weekly summary
  ├─ Identify themes and patterns
  └─ Save to weekly_summaries

1st of Month 12:00 AM:
  ├─ Collect past 4-5 weekly summaries
  ├─ Use LLM to generate monthly summary
  ├─ Calculate statistics
  └─ Save to monthly_summaries

December 31st:
  ├─ Collect all 12 monthly summaries
  ├─ Use LLM to generate yearly archive
  ├─ Highly compress information
  └─ Save to yearly_archives
```

**Memory Retrieval:**

```go
// When user asks a question:
func (m *MemoryManager) GetRelevantMemory(ctx, query string) Memory {
    // 1. Get recent context (last 4 hours)
    recentMessages := m.GetRecentMessages(ctx, 4)

    // 2. Get today's summary
    todaySummary := m.GetDailySummary(ctx, time.Now())

    // 3. Search past memories (semantic search)
    relevantPast := m.SearchMemory(ctx, query, limit=5)

    // 4. Get relevant facts
    facts := m.GetRelevantFacts(ctx, query)

    // 5. Combine into context
    return CombineMemory(recentMessages, todaySummary, relevantPast, facts)
}
```

**Benefits:**
- ✅ **Constant memory usage** - Old data is compressed
- ✅ **Fast retrieval** - Indexed by time + semantic search
- ✅ **Human-like** - Mimics biological memory
- ✅ **Intelligent** - Learns facts over time
- ✅ **Scalable** - Works for years of data

---

## Skills vs Connectors

### The Pattern

**Connectors** = Infrastructure Layer (Low-level)
**Skills** = Business Logic Layer (High-level)

```
┌─────────────────────────────────────────────────────────┐
│                        SKILLS                            │
│                   (Business Logic)                       │
│                                                          │
│  - Understand user intent                               │
│  - Orchestrate multiple connectors                      │
│  - Use LLM for intelligence                             │
│  - Return structured results                            │
│                                                          │
│  Example: email_manager skill                           │
│  ├─ Action: "draft_response"                           │
│  ├─ Uses: gmail connector (read email)                 │
│  ├─ Uses: LLM (draft response)                         │
│  ├─ Uses: calendar connector (schedule follow-up)      │
│  └─ Returns: {draft, scheduled}                        │
└─────────────────────────────────────────────────────────┘
                         ↓ uses
┌─────────────────────────────────────────────────────────┐
│                     CONNECTORS                           │
│                 (Infrastructure)                         │
│                                                          │
│  - Direct API integration                               │
│  - Authentication (OAuth, API keys)                     │
│  - Rate limiting                                        │
│  - Retry logic                                          │
│  - Error handling                                       │
│                                                          │
│  Example: gmail connector                               │
│  ├─ SendEmail(to, subject, body)                       │
│  ├─ GetEmails(query, max)                              │
│  ├─ DeleteEmail(id)                                     │
│  └─ Handles: OAuth, rate limits, errors                │
└─────────────────────────────────────────────────────────┘
                         ↓ calls
┌─────────────────────────────────────────────────────────┐
│                  EXTERNAL SERVICES                       │
│                                                          │
│  Gmail API, Calendar API, Discord API,                  │
│  Smart Home APIs, etc.                                  │
└─────────────────────────────────────────────────────────┘
```

### Example: Email Management

**Connector (gmail/gmail.go):**
```go
type GmailConnector struct {
    service *gmail.Service
    creds   *oauth2.Config
}

// Low-level: Just wraps the Gmail API
func (g *GmailConnector) SendEmail(to, subject, body string) error {
    message := &gmail.Message{...}
    _, err := g.service.Users.Messages.Send("me", message).Do()
    return err
}

func (g *GmailConnector) GetEmails(query string, max int) ([]Email, error) {
    resp, err := g.service.Users.Messages.List("me").Q(query).Do()
    // Parse and return
}
```

**Skill (email/email_skill.go):**
```go
type EmailSkill struct {
    gmail    *GmailConnector
    calendar *CalendarConnector
    agent    *kernel.Agent  // Can ask LLM for help
}

// High-level: Orchestrates connectors + LLM
func (e *EmailSkill) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
    action := params["action"].(string)

    switch action {
    case "draft_response":
        // 1. Get email via connector
        email := e.gmail.GetEmail(params["email_id"])

        // 2. Use LLM to draft response
        draft := e.agent.AskLLM(ctx,
            "You draft professional emails",
            fmt.Sprintf("Draft response to: %s", email.Body))

        // 3. Optionally schedule follow-up via calendar connector
        if params["schedule_followup"] == true {
            e.calendar.CreateEvent(Event{
                Title: "Follow up on email",
                Start: time.Now().Add(3 * 24 * time.Hour),
            })
        }

        return map[string]any{
            "draft": draft,
            "scheduled": true,
        }, nil
    }
}
```

### Connector Interface

```go
// pkg/connectors/connector.go
type Connector interface {
    Initialize() error
    Shutdown() error
    Name() string
    HealthCheck() error
}

// Specific connector types
type EmailConnector interface {
    Connector
    SendEmail(to, subject, body string) error
    GetEmails(query string, max int) ([]Email, error)
}

type CalendarConnector interface {
    Connector
    CreateEvent(event Event) error
    GetEvents(start, end time.Time) ([]Event, error)
}
```

### Skill Interface

```go
// pkg/skills/skill.go
type Skill interface {
    Name() string
    Description() string
    Parameters() map[string]any  // JSON Schema for LLM
    Execute(ctx context.Context, params map[string]any) (map[string]any, error)
    Initialize() error
    Shutdown() error
}
```

### Configuration

```yaml
connectors:
  - name: gmail
    type: gmail
    enabled: true
    credentials:
      client_id: "${GOOGLE_CLIENT_ID}"
      client_secret: "${GOOGLE_CLIENT_SECRET}"

  - name: calendar
    type: google_calendar
    enabled: true
    credentials:
      client_id: "${GOOGLE_CLIENT_ID}"

skills:
  - name: email_manager
    enabled: true
    connectors:
      - gmail      # Can use gmail connector
      - calendar   # Can use calendar connector
    config:
      auto_draft: true
      style: "professional"
```

---

## Agent Types

### 1. Standard Agent

**Purpose:** Single-domain agent with skills and memory

**Example:** Personal assistant agent

```go
agent := kernel.NewAgent(config, memory)
agent.RegisterSkill(emailSkill)
agent.RegisterSkill(calendarSkill)

response := agent.ExecuteWithMemory(ctx, "Schedule a meeting")
```

**Memory:** Full memory access (all summaries, facts, context)

**Skills:** All registered skills

**Use case:** General-purpose agent, personal assistant

---

### 2. Executive Agent

**Purpose:** Orchestrates multiple specialized agents

**Architecture:**
```
Executive Agent
    ├─ Full memory and context
    ├─ Task decomposition (using LLM)
    ├─ Manages specialized sub-agents
    └─ Synthesizes results
```

**Example:**
```go
executive := kernel.NewExecutiveAgent(config, memory)

// Executive decomposes: "Plan dinner party"
// Into:
//   1. Calendar Agent: Find available date
//   2. Research Agent: Find recipes
//   3. Email Agent: Send invites

response := executive.Orchestrate(ctx, "Plan dinner party for 8 people")
```

**Workflow:**
```
User Request: "Plan dinner party for 8 people"
    ↓
Executive Agent (Claude Opus):
    ├─ Analyzes request
    ├─ Decomposes into subtasks:
    │   ├─ Find available date (→ Calendar Agent)
    │   ├─ Research recipes (→ Research Agent)
    │   └─ Send invites (→ Email Agent)
    ├─ Executes each subtask with context
    └─ Synthesizes results
    ↓
Response: "Scheduled for March 15th, menu planned,
           invites sent to 8 guests"
```

---

### 3. Specialized Agent

**Purpose:** Expert in one domain with focused context

**Examples:**
- Calendar Agent: Only scheduling
- Email Agent: Only email management
- Research Agent: Only information gathering
- Code Agent: Only coding tasks

**Key Features:**
- Focused skills (only relevant ones)
- Focused memory (only domain-specific context)
- Optimized model (cheaper model for simple tasks)
- Fast execution (less context = faster)

**Memory Filtering:**
```go
// Calendar Agent only gets calendar-related memory
calendarAgent.Execute(ctx, "Find free slot")
    ↓
Memory Context:
    ✅ "User prefers morning meetings"
    ✅ "Always buffer 15 min between meetings"
    ✅ "Avoid lunch hours (12-1 PM)"
    ❌ Email writing preferences (filtered out)
    ❌ Research topics (filtered out)
```

---

## Data Flow

### Complete Request Flow

```
┌────────────────────────────────────────────────────────────────┐
│ 1. USER REQUEST                                                 │
│    "Schedule a meeting with Alice about the project tomorrow"  │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 2. AGENT RECEIVES REQUEST                                       │
│    agent.ExecuteWithMemory(ctx, userMessage)                   │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 3. MEMORY RETRIEVAL                                             │
│    ├─ Short-term: Recent messages (last 4 hours)              │
│    ├─ Daily: Today's summary                                  │
│    ├─ Semantic: Past relevant memories                        │
│    └─ Facts: "Alice prefers morning meetings"                 │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 4. CONTEXT BUILDING                                             │
│    System Prompt:                                              │
│    "You are a calendar assistant.                              │
│     Skills: calendar_manager, email_manager                    │
│                                                                │
│     Context from memory:                                       │
│     - Alice prefers morning meetings                           │
│     - Today's schedule: 2 meetings                             │
│     - Recent: Discussed project deadline"                      │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 5. LLM REQUEST                                                  │
│    Request to Claude:                                          │
│    Messages: [SystemPrompt, UserMessage]                       │
│    Tools: [calendar_manager, email_manager]                    │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 6. LLM RESPONSE (with tool calls)                               │
│    ToolCalls: [                                                │
│      {                                                         │
│        name: "calendar_manager",                               │
│        args: {                                                 │
│          action: "find_slot",                                  │
│          date: "tomorrow",                                     │
│          participant: "Alice",                                 │
│          prefer_time: "morning"  // From memory!              │
│        }                                                       │
│      },                                                        │
│      {                                                         │
│        name: "calendar_manager",                               │
│        args: {action: "create_event", ...}                     │
│      }                                                         │
│    ]                                                           │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 7. SKILL EXECUTION                                              │
│    For each tool call:                                         │
│    ├─ Find skill: calendar_manager                            │
│    ├─ Parse arguments                                         │
│    └─ Execute: skill.Execute(ctx, args)                       │
│                   ↓                                            │
│         Skill uses Connector:                                  │
│         calendarConnector.FindSlot(...)                        │
│         calendarConnector.CreateEvent(...)                     │
│                   ↓                                            │
│         Connector calls API:                                   │
│         Google Calendar API                                    │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 8. TOOL RESULTS                                                 │
│    Results: [                                                  │
│      {                                                         │
│        tool_call_id: "call_1",                                 │
│        result: {                                               │
│          available_slots: ["9:00 AM", "10:30 AM"],            │
│          selected: "9:00 AM"                                   │
│        }                                                       │
│      },                                                        │
│      {                                                         │
│        tool_call_id: "call_2",                                 │
│        result: {status: "created", event_id: "evt_123"}       │
│      }                                                         │
│    ]                                                           │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 9. SECOND LLM REQUEST (with tool results)                      │
│    Messages: [SystemPrompt, UserMessage,                       │
│              AssistantMessage(with tool_calls),                │
│              ToolResult("Available slots..."),                 │
│              ToolResult("Event created...")]                   │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 10. FINAL LLM RESPONSE                                          │
│     "I've scheduled a meeting with Alice tomorrow at 9:00 AM   │
│      to discuss the project. The meeting is on your calendar." │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 11. MEMORY SAVING                                               │
│     ├─ Save user message                                      │
│     ├─ Save assistant response                                │
│     ├─ Extract facts: "Discussed project with Alice"          │
│     └─ Update conversation state                              │
└────────────────────────────────────────────────────────────────┘
                           ↓
┌────────────────────────────────────────────────────────────────┐
│ 12. RETURN TO USER                                              │
│     "I've scheduled a meeting with Alice tomorrow at 9:00 AM   │
│      to discuss the project. The meeting is on your calendar." │
└────────────────────────────────────────────────────────────────┘
```

### Background Jobs (Automated)

```
Every Day at 11:59 PM:
├─ Collect all messages from today
├─ Send to LLM: "Summarize this day"
├─ LLM returns: Summary + Key events + Topics
└─ Save to daily_summaries table

Every Sunday at 12:00 AM:
├─ Collect past 7 daily summaries
├─ Send to LLM: "Summarize this week"
├─ LLM returns: Weekly summary + Themes
└─ Save to weekly_summaries table

Every 1st of Month at 12:00 AM:
├─ Collect past 4-5 weekly summaries
├─ Send to LLM: "Summarize this month"
├─ LLM returns: Monthly summary + Highlights
└─ Save to monthly_summaries table

Every December 31st:
├─ Collect all 12 monthly summaries
├─ Send to LLM: "Summarize this year"
├─ LLM returns: Yearly summary (compressed)
└─ Save to yearly_archives table
```

---

## Project Structure

```
OnlyAgents/
├── cmd/
│   ├── agent/              # Main agent binary
│   │   └── main.go
│   ├── executive/          # Executive agent binary
│   │   └── main.go
│   └── cli/                # CLI tool
│       └── main.go
│
├── pkg/
│   ├── config/             # Configuration management
│   │   ├── config.go       # Load config from YAML
│   │   └── validation.go   # Validate config
│   │
│   ├── logger/             # Structured logging
│   │   └── logger.go       # slog wrapper
│   │
│   ├── llm/                # LLM abstraction layer
│   │   ├── llm.go          # Core types & interfaces
│   │   ├── factory.go      # Provider factory
│   │   ├── mock.go         # Mock for testing
│   │   └── providers/
│   │       ├── anthropic.go
│   │       ├── openai.go
│   │       └── gemini.go
│   │
│   ├── memory/             # Memory system
│   │   ├── memory.go       # MemoryManager
│   │   ├── schema.sql      # Database schema
│   │   ├── scheduler.go    # Cron jobs for summarization
│   │   ├── types.go        # Memory types
│   │   └── memory_test.go
│   │
│   ├── kernel/             # Agent runtime
│   │   ├── agent.go        # Core Agent
│   │   ├── executive.go    # Executive Agent
│   │   ├── lifecycle.go    # Start/Stop/Health
│   │   ├── message.go      # Message types
│   │   ├── router.go       # Message routing
│   │   ├── state.go        # State management
│   │   ├── skill_registry.go
│   │   └── connector_registry.go
│   │
│   ├── skills/             # Skill implementations
│   │   ├── skill.go        # Skill interface
│   │   ├── email/
│   │   │   ├── email_skill.go
│   │   │   └── email_skill_test.go
│   │   ├── calendar/
│   │   │   ├── calendar_skill.go
│   │   │   └── calendar_skill_test.go
│   │   ├── research/
│   │   │   └── research_skill.go
│   │   └── smarthome/
│   │       └── smarthome_skill.go
│   │
│   ├── connectors/         # Connector implementations
│   │   ├── connector.go    # Connector interface
│   │   ├── gmail/
│   │   │   ├── gmail.go
│   │   │   ├── auth.go
│   │   │   └── types.go
│   │   ├── calendar/
│   │   │   ├── calendar.go
│   │   │   └── types.go
│   │   ├── discord/
│   │   │   └── discord.go
│   │   └── smarthome/
│   │       ├── hue.go
│   │       ├── nest.go
│   │       └── ring.go
│   │
│   ├── asec/               # Security layer
│   │   ├── identity.go     # Cryptographic identity
│   │   ├── signing.go      # Message signing
│   │   ├── capabilities.go # Capability system
│   │   └── audit.go        # Audit logging
│   │
│   ├── a2a/                # Agent-to-agent protocol
│   │   ├── message.go      # A2A message format
│   │   ├── discovery.go    # Agent discovery
│   │   ├── transport.go    # Transport layer
│   │   └── conversation.go # Conversation tracking
│   │
│   └── soul/               # Agent persona
│       ├── soul.go         # Personality/preferences
│       ├── memory.go       # Personal memory
│       └── adaptation.go   # Learning/evolution
│
├── data/
│   ├── memory.db           # SQLite database
│   ├── embeddings/         # Vector embeddings cache
│   └── logs/               # Log files
│
├── credentials/
│   ├── gmail_token.json
│   ├── calendar_token.json
│   └── keys/
│       └── agent.key
│
├── examples/
│   ├── simple-agent/
│   ├── multi-agent/
│   └── custom-skill/
│
├── docs/
│   ├── ARCHITECTURE.md     # This file
│   ├── README.md           # Project README
│   ├── api/                # API documentation
│   ├── skills/             # Skill development guide
│   └── deployment/         # Deployment guides
│
├── scripts/
│   ├── build.sh
│   ├── test.sh
│   ├── migrate-db.sh
│   └── setup-dev.sh
│
├── tests/
│   ├── integration/
│   └── e2e/
│
├── agent.yaml              # Main config file
├── go.mod
└── go.sum
```

---

## Technology Stack

### Core Technologies

| Component | Technology | Purpose |
|-----------|-----------|---------|
| **Language** | Go 1.25 | Runtime, concurrency, performance |
| **Database** | SQLite | Local memory storage |
| **LLM SDK** | Anthropic SDK, OpenAI SDK | LLM integration |
| **Logging** | slog | Structured logging |
| **Config** | Viper | YAML configuration |
| **HTTP** | net/http, WebSocket | API, A2A communication |
| **Security** | Ed25519, AES-256 | Cryptographic identity |

### External Services

| Service | Purpose | Connector |
|---------|---------|-----------|
| Google Gmail | Email management | `connectors/gmail` |
| Google Calendar | Calendar management | `connectors/calendar` |
| Discord | Chat platform | `connectors/discord` |
| Slack | Team communication | `connectors/slack` |
| Philips Hue | Smart lighting | `connectors/smarthome` |
| Nest | Smart thermostat | `connectors/smarthome` |

### LLM Providers

| Provider | Models | Use Case |
|----------|--------|----------|
| **Anthropic** | Claude Opus 4.6 | Executive agent (strategic) |
| | Claude Sonnet 4.5 | Specialized agents (efficient) |
| | Claude Haiku 4.5 | Simple tasks (fast/cheap) |
| **OpenAI** | GPT-4 Turbo | Alternative provider |
| **Google** | Gemini Pro | Alternative provider |
| **Local** | Llama, Mistral | Privacy, cost savings |

---

## Implementation Phases

### Phase 1: Foundation (Weeks 1-2)

**Goal:** Core infrastructure working

**Tasks:**
- [x] LLM package with tool calling
- [x] Factory pattern
- [x] Anthropic provider
- [ ] Memory database schema
- [ ] Basic MemoryManager
- [ ] Agent with LLM integration
- [ ] Configuration system

**Deliverable:** Agent can chat with LLM and basic memory

---

### Phase 2: Memory System (Weeks 3-4)

**Goal:** Hierarchical memory working

**Tasks:**
- [ ] Daily summarization
- [ ] Cron scheduler
- [ ] Memory retrieval
- [ ] Fact learning
- [ ] Context building
- [ ] Agent memory integration

**Deliverable:** Agent remembers and learns

---

### Phase 3: Skills & Connectors (Weeks 5-6)

**Goal:** First skill working end-to-end

**Tasks:**
- [ ] Connector interface
- [ ] Skill interface
- [ ] Gmail connector
- [ ] Calendar connector
- [ ] Email skill
- [ ] Calendar skill
- [ ] Tool calling integration

**Deliverable:** Agent can manage email and calendar

---

### Phase 4: Executive Agent (Week 7)

**Goal:** Multi-agent orchestration

**Tasks:**
- [ ] Executive agent implementation
- [ ] Task decomposition with LLM
- [ ] Specialized agent creation
- [ ] Context passing
- [ ] Result synthesis

**Deliverable:** Executive orchestrates sub-agents

---

### Phase 5: Additional Features (Weeks 8-10)

**Goal:** Production-ready features

**Tasks:**
- [ ] OpenAI provider
- [ ] Gemini provider
- [ ] Weekly/monthly summarization
- [ ] Vector embeddings for semantic search
- [ ] More skills (research, code, etc.)
- [ ] More connectors (Discord, Slack, etc.)
- [ ] Security layer (ASec)
- [ ] A2A protocol basics

**Deliverable:** Multi-provider, multi-skill system

---

### Phase 6: Polish & Deploy (Weeks 11-12)

**Goal:** Production deployment

**Tasks:**
- [ ] Comprehensive testing
- [ ] Performance optimization
- [ ] Error handling
- [ ] Documentation
- [ ] CLI tool
- [ ] Deployment scripts
- [ ] Monitoring/metrics

**Deliverable:** Production-ready agent system

---

## Key Design Decisions

### 1. Why Hierarchical Memory?

**Decision:** Compress old memories into summaries

**Rationale:**
- Human brains do this naturally
- Constant memory usage regardless of time
- Fast retrieval (don't search years of messages)
- Intelligent (LLM does compression, not simple truncation)

**Trade-off:** Some detail lost in compression, but major facts preserved

---

### 2. Why Skills vs Connectors?

**Decision:** Separate business logic from infrastructure

**Rationale:**
- Reusability (multiple skills use same connector)
- Testability (mock connectors easily)
- Maintainability (change API without changing logic)
- Flexibility (swap implementations)

**Trade-off:** More layers, but cleaner architecture

---

### 3. Why Executive Agent Pattern?

**Decision:** One orchestrator, many specialists

**Rationale:**
- Scalability (parallel execution)
- Cost optimization (use cheaper models for simple tasks)
- Expertise (each agent is focused)
- Flexibility (add new specialists easily)

**Trade-off:** More complexity, but more powerful

---

### 4. Why SQLite?

**Decision:** Local SQLite database

**Rationale:**
- No external dependencies
- Fast for local queries
- ACID transactions
- Easy backup/migration
- Sufficient for single-agent use

**Trade-off:** Not distributed, but can migrate to Postgres later if needed

---

### 5. Why Tool Calling?

**Decision:** LLM calls skills as functions

**Rationale:**
- Natural integration (LLM decides when to use skills)
- Structured (JSON schema for parameters)
- Reliable (LLM is very good at function calling)
- Standard (OpenAI function calling format)

**Trade-off:** Requires LLM with tool calling support

---

## Configuration Example

```yaml
# agent.yaml

agent:
  id: "user.alice.assistant"
  name: "Alice's Assistant"
  role: "personal-assistant"
  max_concurrency: 10
  buffer_size: 100

logging:
  level: "info"
  format: "json"

llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  api_key: ""  # Uses ANTHROPIC_API_KEY env var
  options:
    max_tokens: "4096"
    temperature: "0.7"

memory:
  enabled: true
  database_path: "./data/memory.db"
  summarization:
    enabled: true
    daily_schedule: "59 23 * * *"      # 11:59 PM
    weekly_schedule: "0 0 * * 0"       # Sunday
    monthly_schedule: "0 0 1 * *"      # 1st of month
  retention:
    messages: "30d"
    daily_summaries: "90d"
    weekly_summaries: "1y"
    monthly_summaries: "5y"

connectors:
  - name: gmail
    type: gmail
    enabled: true
    credentials:
      client_id: "${GOOGLE_CLIENT_ID}"
      client_secret: "${GOOGLE_CLIENT_SECRET}"

  - name: calendar
    type: google_calendar
    enabled: true

skills:
  - name: email_manager
    enabled: true
    connectors: [gmail, calendar]

  - name: calendar_assistant
    enabled: true
    connectors: [calendar, gmail]

executive:
  enabled: false
  model: "claude-opus-4-5-20251101"
  specialized_agents:
    - type: calendar
      model: "claude-sonnet-4-20250514"
    - type: email
      model: "claude-sonnet-4-20250514"
```

---

## Summary

OnlyAgents is a **modular agent framework** with:

1. **Hierarchical Memory** - Daily → Weekly → Monthly → Yearly compression
2. **Skills & Connectors** - Separation of business logic and infrastructure
3. **Executive Pattern** - One orchestrator, many specialists
4. **Multi-LLM Support** - Anthropic, OpenAI, Gemini, Local
5. **Tool Calling** - Skills exposed as LLM functions
6. **Security** - Cryptographic identity, capabilities, audit
7. **A2A Protocol** - Agent-to-agent communication

This architecture enables:
- ✅ Long-term memory (years of data)
- ✅ Intelligent learning (fact extraction)
- ✅ Domain expertise (specialized agents)
- ✅ Cost optimization (right model for right task)
- ✅ Extensibility (plugin architecture)
- ✅ Security (enterprise-grade)

**Start simple, scale gradually.**

---

**Last Updated:** 2025-02-16
**Version:** 0.1.0
**Maintainer:** OnlyAgents Team
