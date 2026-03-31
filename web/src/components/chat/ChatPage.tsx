import { useState, useEffect, useRef } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getWSInstance, resetWSInstance } from '../../api/ws'
import { sessions } from '../../api/endpoints/chat'
import { MessageList } from './MessageList'
import { ChatInput } from './ChatInput'
import type { ChatMessage, Session } from '../../api/types'

// ─── Live thinking state ──────────────────────────────────────────────────────

export interface LiveToolEvent {
  id: string
  toolName: string
  input?: string
  success?: boolean
  durationMs?: number
  done: boolean
}

export interface LiveThinkingBlock {
  reasoning: string
  toolEvents: LiveToolEvent[]
  /** Latest live label for the dropdown title */
  latestLabel: string
}

// ─────────────────────────────────────────────────────────────────────────────

export function ChatPage() {
  const { sessionId: urlSessionId } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  // Keep ws in a ref so it never triggers re-renders or effect re-runs
  const wsRef = useRef(getWSInstance())

  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isThinking, setIsThinking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [liveThinking, setLiveThinking] = useState<LiveThinkingBlock | null>(null)

  // ======================
  // Sessions
  // ======================

  const { data: sessionList } = useQuery({
    queryKey: ['sessions'],
    queryFn: () => sessions.list(),
    staleTime: 30_000,
    refetchInterval: 60_000,
  })

  const sessionItems = (sessionList?.sessions ?? []) as Session[]

  // ======================
  // History
  // ======================

  const { data: historyData } = useQuery({
    queryKey: ['chat-history', urlSessionId ?? wsRef.current.sessionId],
    queryFn: () => {
      const id = urlSessionId ?? wsRef.current.sessionId
      return id ? sessions.history(id) : Promise.resolve(null)
    },
    enabled: !!(urlSessionId ?? wsRef.current.sessionId),
    staleTime: 30_000,
  })

  const seededRef = useRef(false)

  useEffect(() => {
    if (historyData?.history && !seededRef.current) {
      seededRef.current = true
      setMessages(historyData.history)
    }
  }, [historyData])

  // ======================
  // Session switch — only when urlSessionId changes
  // ======================

  useEffect(() => {
    if (!urlSessionId) return
    const current = wsRef.current
    if (urlSessionId !== current.sessionId) {
      resetWSInstance()
      const fresh = getWSInstance({ sessionId: urlSessionId })
      fresh.connect()
      wsRef.current = fresh
      seededRef.current = false
    }
  }, [urlSessionId])

  // Connect on first mount if not already connected
  useEffect(() => {
    const ws = wsRef.current
    if (!ws.connected) ws.connect()
  }, [])

  // ======================
  // URL sync
  // ======================

  useEffect(() => {
    const ws = wsRef.current
    if (ws.sessionId && !urlSessionId) {
      navigate(`/chat/${ws.sessionId}`, { replace: true })
    }
  })

  // ======================
  // WS handlers — run once, use refs for stable callbacks
  // ======================

  // Stable refs so closures inside ws handlers always see fresh state setters
  const setMessagesRef = useRef(setMessages)
  const setIsThinkingRef = useRef(setIsThinking)
  const setLiveThinkingRef = useRef(setLiveThinking)
  const navigateRef = useRef(navigate)
  const queryClientRef = useRef(queryClient)

  useEffect(() => {
    navigateRef.current = navigate
  }, [navigate])

  useEffect(() => {
    const ws = wsRef.current

    ws.handlers.onAgentThinking = () => {
      setIsThinkingRef.current(true)
      setError(null)
      setLiveThinkingRef.current({ reasoning: '', toolEvents: [], latestLabel: 'Thinking…' })
    }

    ws.handlers.onAgentText = ({ message_id, content, is_final, agent_id }) => {
      setMessagesRef.current((prev) => {
        const idx = prev.findIndex((m) => m.id === message_id)
        if (idx !== -1) {
          const updated = [...prev]
          updated[idx] = { ...updated[idx], content: updated[idx].content + content }
          return updated
        }
        return [
          ...prev,
          {
            id: message_id,
            role: 'assistant',
            content,
            agentId: agent_id,
            timestamp: new Date().toISOString(),
          },
        ]
      })

      if (is_final) {
        setIsThinkingRef.current(false)
        setLiveThinkingRef.current(null)
      }
    }

    ws.handlers.onNotification = ({ title, body }) => {
      setMessagesRef.current((prev) => [
        ...prev,
        {
          id: crypto.randomUUID(),
          role: 'notification',
          content: `**${title}** ${body}`,
          timestamp: new Date().toISOString(),
        },
      ])
    }

    ws.handlers.onNewSession = ({ session_id }) => {
      void queryClientRef.current.invalidateQueries({ queryKey: ['sessions'] })
      navigateRef.current(`/chat/${session_id}`, { replace: true })
    }

    ws.handlers.onToolCalled = (_agentId, { tool_name, input }) => {
      const eventId = crypto.randomUUID()
      setLiveThinkingRef.current((prev) => {
        const base = prev ?? { reasoning: '', toolEvents: [], latestLabel: 'Thinking…' }
        return {
          ...base,
          latestLabel: tool_name,
          toolEvents: [
            ...base.toolEvents,
            { id: eventId, toolName: tool_name, input, done: false },
          ],
        }
      })
    }

    ws.handlers.onToolResult = (_agentId, { tool_name, success, duration_ms }) => {
      setLiveThinkingRef.current((prev) => {
        if (!prev) return prev
        const events = [...prev.toolEvents]
        // Mark most recent matching pending tool as done
        for (let i = events.length - 1; i >= 0; i--) {
          if (events[i].toolName === tool_name && !events[i].done) {
            events[i] = { ...events[i], success, durationMs: duration_ms, done: true }
            break
          }
        }
        return { ...prev, toolEvents: events }
      })
    }

    return () => {
      ws.handlers.onAgentThinking = undefined
      ws.handlers.onAgentText = undefined
      ws.handlers.onNotification = undefined
      ws.handlers.onNewSession = undefined
      ws.handlers.onToolCalled = undefined
      ws.handlers.onToolResult = undefined
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []) // intentionally empty — ws singleton, setters are stable

  // ======================
  // Actions
  // ======================

  function handleSend(message: string) {
    setError(null)
    setMessages((prev) => [
      ...prev,
      { id: crypto.randomUUID(), role: 'user', content: message, timestamp: new Date().toISOString() },
    ])
    wsRef.current.send(message)
  }

  function handleNewSession() {
    wsRef.current.newSession()
  }

  function handleSelectSession(id: string) {
    if (id === (urlSessionId ?? wsRef.current.sessionId)) return
    navigate(`/chat/${id}`)
  }

  const activeSessionId = urlSessionId ?? wsRef.current.sessionId

  // ======================
  // Render
  // ======================

  return (
    <div style={{
      position: 'absolute', inset: 0,
      display: 'flex', flexDirection: 'column',
      background: '#080c10', overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8,
        padding: '10px 16px', borderBottom: '1px solid #1e2d3d',
        background: 'rgba(13,17,23,0.5)', flexShrink: 0,
      }}>
        <div style={{ width: 8, height: 8, borderRadius: '50%', background: 'rgba(0,217,126,0.5)' }} />

        <span style={{ color: '#8b9eb0', fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.1em' }}>
          Executive Agent
        </span>

        {isThinking && (
          <span style={{ color: '#d29922', fontSize: 10 }}>thinking…</span>
        )}

        {sessionItems.length > 0 && (
          <select
            value={activeSessionId ?? ''}
            onChange={(e) => handleSelectSession(e.target.value)}
            style={{
              marginLeft: 'auto', background: '#0d1117',
              border: '1px solid #1e2d3d', borderRadius: 4,
              color: '#8b9eb0', fontSize: 10, padding: '2px 6px', cursor: 'pointer',
            }}
          >
            {sessionItems.map((s) => (
              <option key={s.id} value={s.id}>
                {s.id.slice(0, 8)}{s.startedAt ? ` — ${new Date(s.startedAt).toLocaleDateString()}` : ''}
              </option>
            ))}
          </select>
        )}

        <button
          onClick={handleNewSession}
          style={{
            marginLeft: sessionItems.length > 0 ? 6 : 'auto',
            background: 'none', border: '1px solid #1e2d3d', borderRadius: 4,
            color: '#8b9eb0', fontSize: 10, padding: '2px 8px', cursor: 'pointer',
          }}
        >
          + new
        </button>
      </div>

      {/* Messages */}
      <div style={{ flex: 1, overflow: 'hidden', minHeight: 0 }}>
        <MessageList messages={messages} isLoading={isThinking} liveThinking={liveThinking} />
      </div>

      {error && (
        <div style={{
          padding: '8px 16px', background: 'rgba(248,81,73,0.05)',
          borderTop: '1px solid rgba(248,81,73,0.2)', color: '#f85149', fontSize: 11,
        }}>
          {error}
        </div>
      )}

      <ChatInput onSend={handleSend} disabled={isThinking} />
    </div>
  )
}
