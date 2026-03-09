// src/api/ws.ts
// Single WebSocket connection replacing:
//   - POST /v1/chat
//   - POST /v1/agents/{id}/chat
//   - GET  /v1/events (SSE)
//
// Usage:
//   const ws = new OAWebSocket({ sessionId, agentId })
//   ws.onMessage = (msg) => { ... }
//   ws.connect()
//   ws.send("hello")
//   ws.disconnect()

import { getConnectionConfig, getBaseUrl } from './client'

// ─── Message types ────────────────────────────────────────────────────────────
// Keep in sync with:
//   pkg/channels/oaChannel/types.go  → WSMessageType constants
//   pkg/core/ui.go                   → UIEventType constants
//   pkg/core/events.go               → OutboundTokenPayload etc.

export type WSMessageType =
  // UI → Server
  | 'chat'
  | 'voice.chunk'
  | 'voice.end'
  | 'session.new'
  | 'ping'
  // Server → UI (chat)
  | 'agent.text'
  | 'agent.voice'
  | 'agent.thinking'
  | 'notification'
  // Server → UI (war room)
  | 'agent.activated'
  | 'agent.idle'
  | 'agent.error'
  | 'agent.busy'
  | 'tool.called'
  | 'tool.result'
  | 'delegation'
  | 'snapshot'
  | 'pong'

export interface WSMessage {
  type: WSMessageType
  session_id?: string
  agent_id?: string
  timestamp: string
  payload?: unknown
}

// ─── Inbound payload types ────────────────────────────────────────────────────

export interface AgentTextPayload {
  content: string
  agent_id: string
  is_final: boolean
}

export interface NotificationPayload {
  title: string
  body: string
  severity: 'info' | 'warning' | 'alert'
  link?: string
}

export interface NewSessionPayload {
  session_id: string
}

// War room payloads — matches core.AgentStatus etc.
export interface AgentActivatedPayload {
  task: string
  model: string
}

export interface AgentIdlePayload {
  duration_ms: number
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

// ─── Event handler map ────────────────────────────────────────────────────────

export interface OAWebSocketHandlers {
  // Chat
  onAgentText?: (payload: AgentTextPayload, msg: WSMessage) => void
  onAgentThinking?: (msg: WSMessage) => void
  onNotification?: (payload: NotificationPayload, msg: WSMessage) => void
  onNewSession?: (payload: NewSessionPayload) => void
  // War room
  onAgentActivated?: (agentId: string, payload: AgentActivatedPayload) => void
  onAgentIdle?: (agentId: string, payload: AgentIdlePayload) => void
  onAgentError?: (agentId: string, payload: { error: string }) => void
  onToolCalled?: (agentId: string, payload: ToolCalledPayload) => void
  onToolResult?: (agentId: string, payload: ToolResultPayload) => void
  onDelegation?: (payload: DelegationPayload) => void
  onSnapshot?: (agentId: string, payload: unknown) => void
  // Connection
  onConnected?: () => void
  onDisconnected?: (wasClean: boolean) => void
  onError?: (err: Event) => void
}

// ─── Connection options ───────────────────────────────────────────────────────

export interface OAWebSocketOptions {
  sessionId?: string   // omit to start a new session
  agentId?: string     // defaults to "executive"
  handlers?: OAWebSocketHandlers
  reconnect?: boolean  // auto-reconnect on unexpected close (default: true)
  reconnectDelayMs?: number // default: 2000
}

// ─── OAWebSocket ──────────────────────────────────────────────────────────────

export class OAWebSocket {
  private ws: WebSocket | null = null
  private options: OAWebSocketOptions
  private currentSessionId: string | undefined
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private manualClose = false
  private pingTimer: ReturnType<typeof setInterval> | null = null
  private pendingMessages: Partial<WSMessage>[] = []

  handlers: OAWebSocketHandlers

  constructor(options: OAWebSocketOptions = {}) {
    this.options = {
      reconnect: true,
      reconnectDelayMs: 2000,
      ...options,
    }
    this.handlers = options.handlers ?? {}
    this.currentSessionId = options.sessionId
        ?? localStorage.getItem('oa_session_id')
        ?? undefined  // ← restored on page reload
  }
  // ── Public API ──────────────────────────────────────────────────────────────

  get sessionId(): string | undefined {
    return this.currentSessionId
  }

  get connected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  connect(): void {
    if (this.ws?.readyState === WebSocket.OPEN) return
    this.manualClose = false
    this._connect()
  }

  disconnect(): void {
    this.manualClose = true
    this._cleanup()
    this.ws?.close(1000, 'user disconnect')
    this.ws = null
  }

  // Send a chat message to the agent
  send(message: string, agentId?: string): void {
    this._sendMsg({
      type: 'chat',
      timestamp: new Date().toISOString(),
      payload: {
        message,
        agent_id: agentId ?? this.options.agentId ?? 'executive',
      },
    })
  }

  // Request a new session — server responds with session.new containing new ID
  newSession(): void {
    this._sendMsg({ type: 'session.new', timestamp: new Date().toISOString() })
  }

  ping(): void {
    this._sendMsg({ type: 'ping', timestamp: new Date().toISOString() })
  }

  // ── Internal ────────────────────────────────────────────────────────────────

  private _connect(): void {
    const url = this._buildURL()
    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      this._startPing()
      // Flush any messages queued before connection was ready
      this.pendingMessages.forEach(msg => {
        this.ws!.send(JSON.stringify(msg))
      })
      this.pendingMessages = []
      this.handlers.onConnected?.()
    }

    this.ws.onmessage = (evt) => {
      try {
        const msg = JSON.parse(evt.data as string) as WSMessage
        this._dispatch(msg)
      } catch (e) {
        console.error('[OAWebSocket] parse error', e)
      }
    }

    this.ws.onerror = (evt) => {
      this.handlers.onError?.(evt)
    }

    this.ws.onclose = (evt) => {
      this._cleanup()
      this.handlers.onDisconnected?.(evt.wasClean)
      if (!this.manualClose && this.options.reconnect) {
        this.reconnectTimer = setTimeout(
          () => this._connect(),
          this.options.reconnectDelayMs,
        )
      }
    }
  }

  private _dispatch(msg: WSMessage): void {
    switch (msg.type) {
      // ── Chat ──
      case 'agent.text':
        this.handlers.onAgentText?.(msg.payload as AgentTextPayload, msg)
        break
      case 'agent.thinking':
        this.handlers.onAgentThinking?.(msg)
        break
      case 'notification':
        this.handlers.onNotification?.(msg.payload as NotificationPayload, msg)
        break
      case 'session.new': {
        const p = msg.payload as NewSessionPayload
        this.currentSessionId = p.session_id
        this.handlers.onNewSession?.(p)
        break
      }
      // ── War room ──
      case 'agent.activated':
        this.handlers.onAgentActivated?.(msg.agent_id ?? '', msg.payload as AgentActivatedPayload)
        break
      case 'agent.idle':
        this.handlers.onAgentIdle?.(msg.agent_id ?? '', msg.payload as AgentIdlePayload)
        break
      case 'agent.error':
        this.handlers.onAgentError?.(msg.agent_id ?? '', msg.payload as { error: string })
        break
      case 'tool.called':
        this.handlers.onToolCalled?.(msg.agent_id ?? '', msg.payload as ToolCalledPayload)
        break
      case 'tool.result':
        this.handlers.onToolResult?.(msg.agent_id ?? '', msg.payload as ToolResultPayload)
        break
      case 'delegation':
        this.handlers.onDelegation?.(msg.payload as DelegationPayload)
        break
      case 'snapshot':
        this.handlers.onSnapshot?.(msg.agent_id ?? '', msg.payload)
        break
      case 'pong':
        break // heartbeat acknowledged
      default:
        console.debug('[OAWebSocket] unhandled message type:', msg.type)
    }
  }

  private _sendMsg(msg: Partial<WSMessage>): void {
    if (this.ws?.readyState !== WebSocket.OPEN) {
      this.ws?.send(JSON.stringify(msg))

    }else {
        // Queue until connected instead of dropping
        this.pendingMessages.push(msg)
    }
      }

  private _buildURL(): string {
    const cfg = getConnectionConfig()
    const base = getBaseUrl()

    // Convert http(s) → ws(s)
    const wsBase = base
      .replace(/^http:\/\//, 'ws://')
      .replace(/^https:\/\//, 'wss://')
      // Dev mode (relative base '') → derive from window.location
      || `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}`

    const params = new URLSearchParams()
    if (this.currentSessionId) params.set('session_id', this.currentSessionId)
    if (this.options.agentId) params.set('agent_id', this.options.agentId)
    if (cfg?.apiKey) params.set('key', cfg.apiKey) // WS can't send headers

    const query = params.toString()
    return `${wsBase}/v1/ws${query ? `?${query}` : ''}`
  }

  private _startPing(): void {
    this.pingTimer = setInterval(() => this.ping(), 30_000)
  }

  private _cleanup(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.pingTimer) {
      clearInterval(this.pingTimer)
      this.pingTimer = null
    }
  }
}

// ─── Singleton helpers (mirrors old chat/events pattern) ─────────────────────
// One shared WS instance for the app. Import and use directly.

let _instance: OAWebSocket | null = null

export function getWSInstance(options?: OAWebSocketOptions): OAWebSocket {
  if (!_instance) {
    _instance = new OAWebSocket(options)
  }
  return _instance
}

export function resetWSInstance(): void {
  _instance?.disconnect()
  _instance = null
}
