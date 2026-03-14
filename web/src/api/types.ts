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

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  timestamp: string
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

export interface ChatHistoryResponse {
  history: ChatMessage[]
  count: number
}

// ─── Connection config (stored in localStorage) ───────────────────────────────

export interface ConnectionConfig {
  serverUrl: string   // e.g. "http://localhost:19965" or "https://myserver.com"
  apiKey: string      // empty string if no auth configured
}
