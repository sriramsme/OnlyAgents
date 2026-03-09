import { useEffect, useRef } from 'react'
import { useEventStore } from '../../store/eventStore'
import type {
  UIEvent,
  ToolCalledPayload,
  ToolResultPayload,
  DelegationPayload,
  AgentActivatedPayload,
} from '../../api/types'

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleTimeString('en-US', {
      hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit',
    })
  } catch { return '--:--:--' }
}

function EventRow({ evt }: { evt: UIEvent }) {
  const time  = formatTime(evt.timestamp)
  const agent = evt.agent_id ?? '—'

  const base: React.CSSProperties = {
    display: 'flex',
    gap: 6,
    padding: '2px 0',
    alignItems: 'baseline',
    fontSize: 11,
    fontFamily: 'inherit',
    lineHeight: 1.6,
  }

  const dim   = { color: 'rgba(139,158,176,0.4)', flexShrink: 0 }
  const agent_: React.CSSProperties = { color: '#8b9eb0', flexShrink: 0, width: 76, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }

  switch (evt.type) {
    case 'agent.activated': {
      const p = evt.payload as AgentActivatedPayload
      return (
        <div style={base}>
          <span style={dim}>{time}</span>
          <span style={agent_}>{agent}</span>
          <span style={{ color: '#00d97e', flexShrink: 0 }}>▶</span>
          <span style={{ color: 'rgba(205,217,229,0.7)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {p?.task ?? 'started'}
          </span>
        </div>
      )
    }
    case 'agent.idle': {
      return (
        <div style={base}>
          <span style={dim}>{time}</span>
          <span style={agent_}>{agent}</span>
          <span style={{ color: 'rgba(139,158,176,0.5)', flexShrink: 0 }}>◼</span>
          <span style={{ color: 'rgba(139,158,176,0.5)' }}>idle</span>
        </div>
      )
    }
    case 'agent.error': {
      return (
        <div style={base}>
          <span style={dim}>{time}</span>
          <span style={agent_}>{agent}</span>
          <span style={{ color: '#f85149', flexShrink: 0 }}>✕</span>
          <span style={{ color: 'rgba(248,81,73,0.8)' }}>error</span>
        </div>
      )
    }
    case 'tool.called': {
      const p = evt.payload as ToolCalledPayload
      return (
        <div style={base}>
          <span style={dim}>{time}</span>
          <span style={agent_}>{agent}</span>
          <span style={{ color: '#388bfd', flexShrink: 0 }}>⟳</span>
          <span style={{ color: 'rgba(56,139,253,0.9)', flexShrink: 0 }}>{p?.tool_name}</span>
          {p?.input && (
            <span style={{ color: 'rgba(139,158,176,0.4)', fontSize: 10, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              ({p.input.slice(0, 50)}{p.input.length > 50 ? '…' : ''})
            </span>
          )}
        </div>
      )
    }
    case 'tool.result': {
      const p = evt.payload as ToolResultPayload
      return (
        <div style={base}>
          <span style={dim}>{time}</span>
          <span style={agent_}>{agent}</span>
          <span style={{ color: p?.success ? 'rgba(0,217,126,0.6)' : 'rgba(248,81,73,0.6)', flexShrink: 0 }}>
            {p?.success ? '✓' : '✕'}
          </span>
          <span style={{ color: 'rgba(139,158,176,0.6)', flexShrink: 0 }}>{p?.tool_name}</span>
          <span style={{ color: 'rgba(139,158,176,0.25)', fontSize: 10 }}>{p?.duration_ms}ms</span>
        </div>
      )
    }
    case 'delegation': {
      const p = evt.payload as DelegationPayload
      return (
        <div style={base}>
          <span style={dim}>{time}</span>
          <span style={agent_}>{p?.from_agent}</span>
          <span style={{ color: '#d29922', flexShrink: 0 }}>⇢</span>
          <span style={{ color: 'rgba(210,153,34,0.9)', flexShrink: 0 }}>{p?.to_agent}</span>
          <span style={{ color: 'rgba(139,158,176,0.4)', fontSize: 10, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {p?.task?.slice(0, 40)}
          </span>
        </div>
      )
    }
    default:
      return null
  }
}

export function EventLog() {
  const events    = useEventStore((s) => s.events)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [events.length])

  return (
    <div style={{
      width: 280,
      flexShrink: 0,
      display: 'flex',
      flexDirection: 'column',
      background: '#0d1117',
      borderLeft: '1px solid #1e2d3d',
      overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '8px 12px',
        borderBottom: '1px solid #1e2d3d',
        flexShrink: 0,
        fontSize: 10,
        letterSpacing: '0.1em',
      }}>
        <span style={{ color: '#8b9eb0', textTransform: 'uppercase' }}>Event Log</span>
        <span style={{ color: 'rgba(139,158,176,0.3)' }}>{events.length}/200</span>
      </div>

      {/* Scrollable events */}
      <div style={{
        flex: 1,
        overflowY: 'auto',
        padding: '6px 12px',
        minHeight: 0,
      }}>
        {events.length === 0 ? (
          <div style={{
            color: 'rgba(139,158,176,0.25)',
            fontSize: 11,
            textAlign: 'center',
            marginTop: 32,
            fontFamily: 'inherit',
          }}>
            <div>no events yet</div>
            <div style={{ fontSize: 10, marginTop: 4 }}>waiting for activity...</div>
          </div>
        ) : (
          events.map((evt, i) => <EventRow key={i} evt={evt} />)
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}
