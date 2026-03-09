import { useEffect, useRef } from 'react'
import { getWSInstance } from '../api/ws'
import { useAgentStore } from '../store/agentStore'
import { useEventStore } from '../store/eventStore'
import type {
  AgentErrorPayload,
  AgentStatus,
} from '../api/types'
import type {
  AgentActivatedPayload as WSAgentActivatedPayload,
  AgentIdlePayload as WSAgentIdlePayload,
  DelegationPayload as WSDelegationPayload,
} from '../api/ws'

export function useEventStream(enabled: boolean) {
  const { initAgent, setAgentActive, setAgentIdle, setAgentError } = useAgentStore()
  const { push, setConnected, setLastPing } = useEventStore()

  // Keep store refs stable so WS handlers don't need to re-register on rerenders
  const storeRef = useRef({ initAgent, setAgentActive, setAgentIdle, setAgentError, push, setConnected, setLastPing })
  useEffect(() => {
    storeRef.current = { initAgent, setAgentActive, setAgentIdle, setAgentError, push, setConnected, setLastPing }
  })

  useEffect(() => {
    if (!enabled) return

    const ws = getWSInstance()

    ws.handlers.onConnected = () => {
      storeRef.current.setConnected(true)
    }

    ws.handlers.onDisconnected = () => {
      storeRef.current.setConnected(false)
    }

    // ── War room events ───────────────────────────────────────────────────────

    ws.handlers.onSnapshot = (agentId, payload) => {
      const status = payload as AgentStatus
      if (status) storeRef.current.initAgent(status)
      // Push to event log so war room sees it
      storeRef.current.push({
        type: 'snapshot.agent',
        agent_id: agentId,
        timestamp: new Date().toISOString(),
        payload: status,
      })
    }

    ws.handlers.onAgentActivated = (agentId, payload) => {
      const p = payload as WSAgentActivatedPayload
      if (agentId && p) storeRef.current.setAgentActive(agentId, p.task, p.model)
      storeRef.current.push({
        type: 'agent.activated',
        agent_id: agentId,
        timestamp: new Date().toISOString(),
        payload: p,
      })
    }

    ws.handlers.onAgentIdle = (agentId, payload) => {
      const p = payload as WSAgentIdlePayload
      if (agentId) storeRef.current.setAgentIdle(agentId, p?.duration_ms ?? 0)
      storeRef.current.push({
        type: 'agent.idle',
        agent_id: agentId,
        timestamp: new Date().toISOString(),
        payload: p,
      })
    }

    ws.handlers.onAgentError = (agentId, payload) => {
      const p = payload as AgentErrorPayload
      if (agentId) storeRef.current.setAgentError(agentId, p?.error ?? 'unknown error')
      storeRef.current.push({
        type: 'agent.error',
        agent_id: agentId,
        timestamp: new Date().toISOString(),
        payload: p,
      })
    }

    ws.handlers.onDelegation = (payload) => {
      const p = payload as WSDelegationPayload
      storeRef.current.push({
        type: 'delegation',
        timestamp: new Date().toISOString(),
        payload: p,
      })
    }

    ws.handlers.onToolCalled = (agentId, payload) => {
      storeRef.current.push({
        type: 'tool.called',
        agent_id: agentId,
        timestamp: new Date().toISOString(),
        payload,
      })
    }

    ws.handlers.onToolResult = (agentId, payload) => {
      storeRef.current.push({
        type: 'tool.result',
        agent_id: agentId,
        timestamp: new Date().toISOString(),
        payload,
      })
    }

    // Heartbeat — pong is handled internally by OAWebSocket,
    // but we still want to update last ping timestamp
    ws.handlers.onConnected = () => {
      storeRef.current.setConnected(true)
      storeRef.current.setLastPing()
      // Persist session ID so reconnects skip session creation round trip
      const sessionId = ws.sessionId
      if (sessionId) {
        localStorage.setItem('oa_session_id', sessionId)
      }
    }

    ws.connect()

    return () => {
      // Clear war room handlers only — chat handlers (onAgentText etc.)
      // are owned by the chat hook, don't touch them here
      ws.handlers.onConnected = undefined
      ws.handlers.onDisconnected = undefined
      ws.handlers.onSnapshot = undefined
      ws.handlers.onAgentActivated = undefined
      ws.handlers.onAgentIdle = undefined
      ws.handlers.onAgentError = undefined
      ws.handlers.onDelegation = undefined
      ws.handlers.onToolCalled = undefined
      ws.handlers.onToolResult = undefined
      storeRef.current.setConnected(false)
    }
  }, [enabled]) // eslint-disable-line react-hooks/exhaustive-deps
}
