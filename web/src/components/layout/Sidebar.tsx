import { NavLink } from 'react-router-dom'
import { useEventStore } from '../../store/eventStore'
import { Bot, Zap, Link2, Radio, MessageCircle } from 'lucide-react'

const NAV_PRIMARY = [
  { to: '/', label: 'Council', icon: Bot, exact: true, accent: '#00d97e' },
  { to: '/chat', label: 'Chat', icon: MessageCircle, exact: false, accent: '#00d97e' },
]

const NAV_CONFIG = [
  { to: '/agents', label: 'Agents', icon: Bot, exact: false, accent: '#00d97e' },
  { to: '/skills', label: 'Skills', icon: Zap, exact: false, accent: '#388bfd' },
  { to: '/connectors', label: 'Connectors', icon: Link2, exact: false, accent: '#d29922' },
  { to: '/channels', label: 'Channels', icon: Radio, exact: false, accent: '#bc8cff' },
]

type NavItem = { to: string; label: string; icon: React.ComponentType<{ size?: number; strokeWidth?: number, color?: string }>; exact: boolean; accent: string }

function NavItem({ to, label, icon: Icon, exact, accent }: NavItem) {
  return (
    <NavLink
      to={to}
      end={exact}
      style={({ isActive }) => ({
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '9px 16px',
        fontSize: 11,
        fontFamily: 'inherit',
        textDecoration: 'none',
        color: isActive ? accent : '#8b9eb0',
        background: isActive ? `${accent}0d` : 'transparent',
        borderRight: isActive ? `2px solid ${accent}` : '2px solid transparent',
        letterSpacing: '0.05em',
        transition: 'color 0.1s, background 0.1s',
      })}
    >
      <span style={{ lineHeight: 1 }}>
        <Icon size={13} strokeWidth={1.5} />
      </span>
      <span>{label}</span>
    </NavLink>
  )
}

function SectionLabel({ label }: { label: string }) {
  return (
    <div style={{
      padding: '10px 16px 4px',
      fontSize: 9,
      letterSpacing: '0.12em',
      textTransform: 'uppercase',
      color: '#3d444d',
      fontFamily: 'inherit',
    }}>
      {label}
    </div>
  )
}

export function Sidebar() {
  const connected = useEventStore((s) => s.connected)
  const lastPing = useEventStore((s) => s.lastPing)

  const pingAge = lastPing ? Date.now() - lastPing : Infinity
  const isStale = pingAge > 30_000
  const dotColor = !connected ? '#f85149' : isStale ? '#d29922' : '#00d97e'
  const connLabel = !connected ? 'offline' : isStale ? 'stale' : 'live'

  return (
    <div style={{
      width: 176,
      flexShrink: 0,
      display: 'flex',
      flexDirection: 'column',
      background: '#0d1117',
      borderRight: '1px solid #1e2d3d',
      overflow: 'hidden',
    }}>
      {/* Wordmark */}
      <div style={{
        padding: '16px 16px 12px',
        borderBottom: '1px solid #1e2d3d',
        flexShrink: 0,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{
            width: 6, height: 6, borderRadius: '50%',
            background: dotColor,
            flexShrink: 0,
            boxShadow: `0 0 6px ${dotColor}`,
          }} />
          <span style={{
            color: '#e6edf3',
            fontSize: 11,
            letterSpacing: '0.15em',
            textTransform: 'uppercase',
            fontFamily: 'inherit',
          }}>
            OnlyAgents
          </span>
        </div>
        <div style={{
          color: dotColor,
          fontSize: 10,
          marginTop: 2,
          paddingLeft: 14,
          fontFamily: 'inherit',
        }}>
          {connLabel}
        </div>
      </div>

      {/* Nav */}
      <nav style={{ flex: 1, paddingTop: 8, paddingBottom: 8, overflowY: 'auto' }}>
        {NAV_PRIMARY.map(item => <NavItem key={item.to} {...item} />)}

        <SectionLabel label="Config" />

        {NAV_CONFIG.map(item => <NavItem key={item.to} {...item} />)}
      </nav>

      {/* Footer */}
      <div style={{
        padding: '10px 16px',
        borderTop: '1px solid #1e2d3d',
        flexShrink: 0,
      }}>
        <button
          style={{
            background: 'none',
            border: 'none',
            color: 'rgba(139,158,176,0.4)',
            fontSize: 10,
            fontFamily: 'inherit',
            cursor: 'pointer',
            padding: 0,
          }}
          onMouseEnter={(e) => (e.currentTarget.style.color = '#8b9eb0')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'rgba(139,158,176,0.4)')}
          onClick={() => {
            localStorage.removeItem('onlyagents_connection')
            window.location.reload()
          }}
        >
          ↩ disconnect
        </button>
      </div>
    </div>
  )
}
