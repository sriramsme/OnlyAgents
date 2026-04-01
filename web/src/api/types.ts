// ─── Server / auth ────────────────────────────────────────────────────────────

export interface ServerInfo {
  version: string
  name: string
  authenticated: boolean
}

export interface HealthResponse {
  status: string
  timestamp: string
}

// ─── Agents ───────────────────────────────────────────────────────────────────

export type AgentState = 'idle' | 'active' | 'error'

export interface AgentStatus {
  id: string
  name: string
  state: AgentState
  current_task: string
  last_active: string   // RFC3339
  model: string
  is_executive: boolean
}

// ─── UI Events (mirrors pkg/core/ui_events.go) ────────────────────────────────

export type UIEventType =
  | 'agent.activated'
  | 'agent.idle'
  | 'agent.error'
  | 'tool.called'
  | 'tool.result'
  | 'delegation'
  | 'heartbeat'
  | 'snapshot.agent'

export interface UIEvent {
  type: UIEventType
  timestamp: string   // RFC3339
  agent_id?: string
  payload?: unknown
}

export interface AgentActivatedPayload {
  task: string
  model: string
}

export interface AgentIdlePayload {
  duration_ms: number
}

export interface AgentErrorPayload {
  error: string
}

export interface ToolCalledPayload {
  tool_name: string
  input: string
}

export interface ToolResultPayload {
  tool_name: string
  success: boolean
  duration_ms: number
}

export interface DelegationPayload {
  from_agent: string
  to_agent: string
  task: string
}

// ─── Chat ─────────────────────────────────────────────────────────────────────

// should match pkg/storage/types.go
export type Session = {
  id: string
  channel: string
  agentId: string
  chatId: string

  startedAt: string
  endedAt: string

  context: Record<string, any>
  summary: string
  peerAgentId: string
}

export type ChatMessage = {
  id?: string
  conversationId?: string
  agentId?: string

  role: 'user' | 'assistant' | 'tool' | 'notification'
  content: string

  reasoningContent?: string
  toolCalls?: string
  toolCallId?: string

  timestamp: string
}

export type ChatHistoryResponse = {
  history: ChatMessage[]
  count: number
}

export type ChatSessionResponse = {
  session_id: string
  session: Session
}

export type ChatSessionListResponse = {
  sessions: ChatSessionResponse[]
  count: number
}

export interface SendMessageRequest {
  message: string
  agent_id?: string
}

export interface SendMessageResponse {
  response: string
  agent_id: string
  timestamp: string
  latency_ms: number
}

// ─── Connection config (stored in localStorage) ───────────────────────────────

export interface ConnectionConfig {
  serverUrl: string   // e.g. "http://localhost:19965" or "https://myserver.com"
  apiKey: string      // empty string if no auth configured
}

// ─── Config resources (agents / skills / connectors / channels) ───────────────

/** Shared summary shape returned by all four List endpoints */
export interface ConfigSummary {
  id: string
  name: string
  description: string
  enabled: boolean
}

/** Full agent config returned by GET /v1/agents/:id */
export interface AgentConfig extends ConfigSummary {
  model?: string
  system_prompt?: string
  skills?: string[]
  tools?: string[]
  [key: string]: unknown
}

/** Full skill config returned by GET /v1/skills/:id */
export interface SkillConfig extends ConfigSummary {
  type?: string
  config?: Record<string, unknown>
  [key: string]: unknown
}

/** Full connector config returned by GET /v1/connectors/:id */
export interface ConnectorConfig extends ConfigSummary {
  type?: string
  url?: string
  auth?: Record<string, unknown>
  [key: string]: unknown
}

/** Full channel config returned by GET /v1/channels/:id */
export interface ChannelConfig extends ConfigSummary {
  type?: string
  webhook_url?: string
  agent_id?: string
  [key: string]: unknown
}
