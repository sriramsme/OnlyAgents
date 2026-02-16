# OnlyAgents: Technical Summary


[![CI](https://github.com/sriramsme/onlyagents/workflows/CI/badge.svg)](https://github.com/sriramsme/onlyagents/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/sriramsme/onlyagents)](https://goreportcard.com/report/github.com/sriramsme/onlyagents)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/sriramsme/onlyagents)](https://go.dev/)

**Modular, secure, open infrastructure for autonomous AI agents**

---

## Vision

* Foundation for agent ecosystem: modular, interoperable, edge-first.
* Agents run anywhere (phones, IoT, cloud) and communicate seamlessly.
* Open protocols, user data sovereignty, developer freedom.

---

## Core Problems in Current Agents

* Monolithic, siloed, proprietary, insecure, cloud-heavy.
* No standard: communication, identity, modularity, security.
* Edge deployment and cryptographic verification missing.

---

## Architecture Overview

**Fundamental Units:**

| Unit      | Role                                                                   |
| --------- | ---------------------------------------------------------------------- |
| Kernel    | Runtime, agent lifecycle, message routing, state, skill execution      |
| ASec      | Security: cryptographic identity, capabilities, signed messages, audit |
| A2A       | Agent-to-Agent protocol: discovery, messages, multi-turn conversations |
| Skills    | Domain specialization modules (plug-and-play)                          |
| Connector | Platform adapters (APIs, services)                                     |
| Soul      | Agent persona, preferences, behavior, memory                           |

**Executive Agent Pattern:**

* Decomposes user intent → orchestrates multiple agents → synthesizes results.
* Example: booking dinner + scheduling review → Calendar + Restaurant + Research agents.

---

## Kernel (Runtime)

* Language: Go 1.21+, cross-platform, lightweight (5-10MB memory, <10ms startup)
* Features: lifecycle management, message routing, context/state, hot-reload skills
* Observability: Prometheus metrics, structured logging, tracing via OpenTelemetry

**Example (Go):**

```go
agent := kernel.NewAgent(kernel.Config{
    ID: "user.assistant.calendar",
    Skills: []kernel.Skill{skills.NewCalendarSkill()},
    Connectors: []kernel.Connector{connectors.NewGoogleCalendar()},
    Security: asec.DefaultConfig(),
})
agent.Start()
```

---

## ASec (Security Protocol)

* Zero-trust model: auth, capability-based authorization, audit logs
* Identity: Ed25519 keys, certificates
* Messages: cryptographically signed, tamper-proof
* Credential handling: AES-256 encrypted, vault integration
* Sandbox execution: memory/file limits, network restrictions
* Immutable audit trail

---

## A2A (Agent-to-Agent Communication)

* Open, typed, async, reliable, extensible
* Agent discovery via manifest (`/.well-known/a2a-manifest.json`)
* Supports WebSocket, HTTP/2, gRPC, message queues (RabbitMQ, NATS)
* Multi-turn conversations tracked by `ConversationID`
* Example message schema:

```go
type A2AMessage struct {
    ProtocolVersion string
    FromAgent       string
    ToAgent         string
    MessageID       string
    ConversationID  string
    Action          string
    Payload         map[string]any
    Context         map[string]any
    Signature       []byte
    PublicKey       []byte
    TTL             int
    Priority        string
}
```

---

## Skills (Domain Specialization)

* Provide domain-specific capabilities to generalist agents
* Interface:

```go
type Skill interface {
    Name() string
    Execute(ctx context.Context, intent string, params map[string]any) (Result, error)
    Initialize() error
    Shutdown() error
}
```

* Skills are sandboxed, versioned, and installable via CLI.

---

## Connector (Platform Integration)

* Standardized API interface for external services
* Example: CalendarConnector with CRUD, availability, invites
* Multi-platform support: Google, Outlook, Apple, Slack, Notion, etc.

---

## Soul (Agent Persona)

* Defines identity, personality, values, behavior, memory
* Guides prompt generation, decision-making, proactive actions
* Evolves through feedback and interaction
* Configuration via YAML; applied at runtime to influence LLM prompts

---

## Performance & Benchmarks

* Cold start <10ms, message routing <1ms, memory 5-10MB
* Message signing <0.1ms, capability check <0.01ms
* Throughput: 10k+ msgs/sec, p99 latency <5ms local, <50ms remote
* Supports 10k+ concurrent connections per agent

---

## Tech Stack

* **Kernel**: Go, Goroutines, Protocol Buffers
* **Security**: Ed25519, AES-256, TLS 1.3, Vault
* **Communication**: WebSocket, HTTP/2, gRPC, MQ
* **Storage**: BadgerDB (state), PostgreSQL (audit), Redis (cache)
* **Observability**: Prometheus, Grafana, Zap, OpenTelemetry
* **LLM Integration**: Anthropic Claude, GPT-4, Gemini, Mistral, vLLM
* **SDKs**: Go, Python, TypeScript, Rust

---

## Use Cases

1. **Personal Assistant**: Calendar, email, tasks, research
2. **Home Automation**: Climate, security, energy, comfort
3. **Developer Productivity**: Code, DevOps, PM, research agents
4. **Enterprise Sales**: Prospecting, outreach, meetings, follow-up

---

## Roadmap (Concise)

1. **Foundation**: Kernel, ASec, A2A, CLI, single agent examples
2. **Skills & Connectors**: Calendar, email, task mgmt, research
3. **Multi-Agent Orchestration**: Executive agent pattern, dependency scheduling
4. **Developer Platform**: SDKs, skill dev kit, marketplace
5. **Production Hardening**: Kubernetes, HA, scaling, monitoring
6. **Ecosystem Growth**: Mobile SDKs, browser extension, public marketplace, partnerships

---

## Project Structure (Key Modules)

Below is the planned project structure. It is not final, and may change as the project evolves.
```
OnlyAgents/
├── cmd/
│   ├── agent/              # Main agent binary
│   └── cli/                # CLI tool
├── pkg/
│   ├── kernel/               # Agent runtime
│   │   ├── agent.go
│   │   ├── lifecycle.go
│   │   ├── state.go
│   │   └── router.go
│   ├── asec/               # Security protocol
│   │   ├── identity.go
│   │   ├── signing.go
│   │   ├── capabilities.go
│   │   ├── vault.go
│   │   └── audit.go
│   ├── a2a/                # Agent-to-agent protocol
│   │   ├── message.go
│   │   ├── discovery.go
│   │   ├── transport.go
│   │   └── conversation.go
│   ├── skills/             # Skill framework
│   │   ├── interface.go
│   │   ├── registry.go
│   │   ├── calendar/
│   │   ├── email/
│   │   └── research/
│   ├── connectors/         # Platform connectors
│   │   ├── interface.go
│   │   ├── google/
│   │   ├── microsoft/
│   │   └── slack/
│   └── soul/               # Agent persona
│       ├── soul.go
│       ├── memory.go
│       └── adaptation.go
├── sdk/
│   ├── python/             # Python bindings
│   ├── typescript/         # TypeScript bindings
│   └── rust/               # Rust bindings
├── examples/
│   ├── simple-agent/
│   ├── multi-agent/
│   └── custom-skill/
├── docs/
│   ├── architecture/
│   ├── protocol/
│   ├── security/
│   ├── tutorials/
│   └── api-reference/
├── deployments/
│   ├── docker/
│   ├── kubernetes/
│   └── systemd/
├── scripts/
│   ├── build.sh
│   ├── test.sh
│   └── release.sh
└── tests/
    ├── unit/
    ├── integration/
    └── e2e/
```
