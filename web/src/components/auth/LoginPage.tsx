import { useState } from 'react'
import { apiFetch, ApiError, getConnectionConfig } from '../../api/client'

interface Props {
  onAuthenticated: () => void
}

export function LoginPage({ onAuthenticated }: Props) {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [status, setStatus]     = useState<'idle' | 'loading' | 'error'>('idle')
  const [errorMsg, setErrorMsg] = useState('')

  const cfg = getConnectionConfig()
  const isInsecureRemote =
    cfg?.serverUrl.startsWith('http://') &&
    !cfg.serverUrl.includes('localhost') &&
    !cfg.serverUrl.includes('127.0.0.1')

  async function handleLogin() {
    if (!username || !password) return
    setStatus('loading')
    setErrorMsg('')

    try {
      await apiFetch('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
        skipAuth: true,
        credentials: 'include', // include cookies in cross-origin requests
      })
      onAuthenticated()
    } catch (err) {
      setStatus('error')
      if (err instanceof ApiError) {
        switch (err.status) {
          case 401: setErrorMsg('Invalid username or password.'); break
          case 429: setErrorMsg('Too many attempts. Wait a minute and try again.'); break
          default:  setErrorMsg(`Server error (${err.status}). Check the server logs.`)
        }
      } else {
        setErrorMsg('Cannot reach server.')
      }
    }
  }

  return (
    <div style={{
      position: 'fixed', inset: 0,
      background: '#080c10',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      fontFamily: 'JetBrains Mono, Fira Mono, monospace',
    }}>
      {/* Background grid */}
      <div style={{
        position: 'absolute', inset: 0, opacity: 0.03,
        backgroundImage: `
          linear-gradient(rgba(0,217,126,0.8) 1px, transparent 1px),
          linear-gradient(90deg, rgba(0,217,126,0.8) 1px, transparent 1px)
        `,
        backgroundSize: '40px 40px',
      }} />

      {/* Corner labels */}
      <div style={{ position: 'absolute', top: 20, left: 20, color: 'rgba(0,217,126,0.25)', fontSize: 10 }}>
        ONLYAGENTS // AUTH
      </div>
      <div style={{ position: 'absolute', top: 20, right: 20, color: 'rgba(139,158,176,0.2)', fontSize: 10 }}>
        {cfg?.serverUrl ?? 'localhost'}
      </div>

      {/* Panel */}
      <div style={{
        position: 'relative', zIndex: 1,
        width: '100%', maxWidth: 340,
        padding: '0 16px',
      }}>
        {/* Title */}
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <div style={{
            display: 'inline-flex', alignItems: 'center', gap: 8,
            marginBottom: 10,
          }}>
            <span style={{
              width: 6, height: 6, borderRadius: '50%',
              background: '#00d97e',
              boxShadow: '0 0 8px #00d97e',
              animation: 'pulse 2s infinite',
            }} />
            <span style={{ color: '#00d97e', fontSize: 10, letterSpacing: '0.3em', textTransform: 'uppercase' }}>
              OnlyAgents
            </span>
            <span style={{
              width: 6, height: 6, borderRadius: '50%',
              background: '#00d97e',
              boxShadow: '0 0 8px #00d97e',
            }} />
          </div>
          <div style={{ color: '#e6edf3', fontSize: 16, fontWeight: 600 }}>
            Sign In
          </div>
          <div style={{ color: '#8b9eb0', fontSize: 11, marginTop: 4 }}>
            Your personal agent council awaits
          </div>
        </div>

        {/* Form */}
        <div style={{
          background: '#0d1117',
          border: '1px solid #1e2d3d',
          borderRadius: 8,
          padding: 20,
          display: 'flex',
          flexDirection: 'column',
          gap: 14,
        }}>
          {/* Insecure warning */}
          {isInsecureRemote && (
            <div style={{
              background: 'rgba(210,153,34,0.05)',
              border: '1px solid rgba(210,153,34,0.3)',
              borderRadius: 4,
              padding: '8px 10px',
              color: '#d29922',
              fontSize: 11,
              display: 'flex',
              gap: 6,
            }}>
              <span style={{ flexShrink: 0 }}>⚠</span>
              <span>Unencrypted remote connection. Use HTTPS in production.</span>
            </div>
          )}

          {/* Username */}
          <div>
            <label style={{
              display: 'block', color: '#8b9eb0',
              fontSize: 10, letterSpacing: '0.1em',
              textTransform: 'uppercase', marginBottom: 6,
            }}>
              Username
            </label>
            <input
              style={{
                width: '100%', background: '#080c10',
                border: '1px solid #1e2d3d', borderRadius: 4,
                padding: '8px 10px', color: '#cdd9e5',
                fontFamily: 'inherit', fontSize: 12,
                outline: 'none', boxSizing: 'border-box',
                transition: 'border-color 0.15s',
              }}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              onFocus={(e) => e.currentTarget.style.borderColor = '#00d97e'}
              onBlur={(e) => e.currentTarget.style.borderColor = '#1e2d3d'}
              onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
              autoComplete="username"
            />
          </div>

          {/* Password */}
          <div>
            <label style={{
              display: 'block', color: '#8b9eb0',
              fontSize: 10, letterSpacing: '0.1em',
              textTransform: 'uppercase', marginBottom: 6,
            }}>
              Password
            </label>
            <input
              type="password"
              style={{
                width: '100%', background: '#080c10',
                border: '1px solid #1e2d3d', borderRadius: 4,
                padding: '8px 10px', color: '#cdd9e5',
                fontFamily: 'inherit', fontSize: 12,
                outline: 'none', boxSizing: 'border-box',
                transition: 'border-color 0.15s',
              }}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              onFocus={(e) => e.currentTarget.style.borderColor = '#00d97e'}
              onBlur={(e) => e.currentTarget.style.borderColor = '#1e2d3d'}
              onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
              autoComplete="current-password"
              autoFocus
            />
          </div>

          {/* Error */}
          {status === 'error' && (
            <div style={{
              background: 'rgba(248,81,73,0.05)',
              border: '1px solid rgba(248,81,73,0.25)',
              borderRadius: 4, padding: '8px 10px',
              color: '#f85149', fontSize: 11,
            }}>
              {errorMsg}
            </div>
          )}

          {/* Submit */}
          <button
            onClick={handleLogin}
            disabled={status === 'loading' || !username || !password}
            style={{
              background: status === 'loading' ? 'rgba(0,217,126,0.05)' : 'rgba(0,217,126,0.08)',
              border: '1px solid #00d97e',
              borderRadius: 4, padding: '9px 16px',
              color: '#00d97e', fontFamily: 'inherit',
              fontSize: 12, cursor: 'pointer',
              display: 'flex', alignItems: 'center',
              justifyContent: 'center', gap: 8,
              transition: 'background 0.15s',
              opacity: (!username || !password) ? 0.4 : 1,
            }}
            onMouseEnter={(e) => {
              if (status !== 'loading') e.currentTarget.style.background = 'rgba(0,217,126,0.15)'
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.background = 'rgba(0,217,126,0.08)'
            }}
          >
            {status === 'loading' ? (
              <>
                <span style={{ animation: 'blink 1.2s step-end infinite' }}>▋</span>
                <span>Authenticating...</span>
              </>
            ) : (
              <>
                <span>Sign In</span>
                <span style={{ opacity: 0.5 }}>→</span>
              </>
            )}
          </button>
        </div>

        {/* Forgot password hint */}
        <div style={{
          textAlign: 'center', marginTop: 14,
          color: 'rgba(139,158,176,0.35)', fontSize: 10,
          lineHeight: 1.6,
        }}>
          Forgot password? Run{' '}
          <code style={{ color: 'rgba(139,158,176,0.6)', fontSize: 10 }}>
            onlyagents auth reset
          </code>
          {' '}on your server.
        </div>
      </div>

      <style>{`
        @keyframes pulse { 0%, 100% { opacity: 1 } 50% { opacity: 0.4 } }
        @keyframes blink { 0%, 100% { opacity: 1 } 50% { opacity: 0 } }
      `}</style>
    </div>
  )
}
