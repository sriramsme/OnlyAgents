import { useState } from 'react'
import { setConnectionConfig, apiFetch, getBaseUrl } from '../../api/client'

interface Props {
  onConnected: () => void
}

export function ConnectScreen({ onConnected }: Props) {
const [serverUrl, setServerUrl] = useState(getBaseUrl())
  const [apiKey, setApiKey]       = useState('')
  const [status, setStatus]       = useState<'idle' | 'checking' | 'error'>('idle')
  const [errorMsg, setErrorMsg]   = useState('')

  const isInsecure =
    serverUrl.startsWith('http://') &&
    !serverUrl.includes('localhost') &&
    !serverUrl.includes('127.0.0.1') &&
    !serverUrl.includes('::1')

  async function handleConnect() {
    setStatus('checking')
    // Store URL with empty apiKey — browser auth uses session cookie
    setConnectionConfig({ serverUrl: serverUrl.trim(), apiKey: '' })
    try {
      await apiFetch('/health', { skipAuth: true })
      setStatus('idle')
      onConnected()
    } catch {
      setStatus('error')
      setErrorMsg('Cannot reach server. Is OnlyAgents running?')
    }
  }

  return (
    <div className="min-h-screen bg-canvas flex items-center justify-center relative overflow-hidden">
      {/* Background grid */}
      <div
        className="absolute inset-0 opacity-[0.03]"
        style={{
          backgroundImage: `
            linear-gradient(rgba(0,217,126,0.8) 1px, transparent 1px),
            linear-gradient(90deg, rgba(0,217,126,0.8) 1px, transparent 1px)
          `,
          backgroundSize: '40px 40px',
        }}
      />

      {/* Corner decorations */}
      <div className="absolute top-6 left-6 text-green/30 text-xs font-mono">
        ONLYAGENTS_UI v0.1.0
      </div>
      <div className="absolute top-6 right-6 text-dim/30 text-xs font-mono">
        AWAITING CONNECTION
      </div>
      <div className="absolute bottom-6 left-6 text-dim/20 text-xs font-mono">
        SYS:INIT
      </div>

      {/* Main panel */}
      <div className="relative z-10 w-full max-w-sm">
        {/* Title */}
        <div className="mb-8 text-center">
          <div className="inline-flex items-center gap-2 mb-3">
            <div className="w-2 h-2 rounded-full bg-green animate-pulse-slow" />
            <span className="text-green text-xs tracking-[0.3em] uppercase">
              OnlyAgents
            </span>
            <div className="w-2 h-2 rounded-full bg-green animate-pulse-slow" />
          </div>
          <h1 className="text-bright text-xl font-mono tracking-tight">
            Connect to Server
          </h1>
          <p className="text-dim text-xs mt-1.5">
            Local or remote — same interface
          </p>
        </div>

        {/* Form */}
        <div className="bg-surface border border-border rounded-lg p-6 space-y-4">
          {/* HTTP warning */}
          {isInsecure && (
            <div className="flex gap-2 p-3 bg-amber/5 border border-amber/30 rounded text-amber text-xs">
              <span className="shrink-0">⚠</span>
              <span>
                Unencrypted connection to a remote server. API keys and messages
                will be sent in plaintext. Use HTTPS in production.
              </span>
            </div>
          )}

          <div>
            <label className="block text-dim text-xs mb-1.5 uppercase tracking-wider">
              Server URL
            </label>
            <input
              className="terminal-input w-full"
              type="url"
              value={serverUrl}
              onChange={(e) => setServerUrl(e.target.value)}
              placeholder={getBaseUrl()}
              onKeyDown={(e) => e.key === 'Enter' && handleConnect()}
            />
          </div>

          <div>
            <label className="block text-dim text-xs mb-1.5 uppercase tracking-wider">
              API Key
              <span className="ml-2 normal-case text-dim/50">(leave empty if not configured)</span>
            </label>
            <input
              className="terminal-input w-full"
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder="••••••••••••••••"
              onKeyDown={(e) => e.key === 'Enter' && handleConnect()}
              autoComplete="current-password"
            />
          </div>

          {/* Error message */}
          {status === 'error' && (
            <div className="p-3 bg-red/5 border border-red/30 rounded text-red text-xs">
              {errorMsg}
            </div>
          )}

          <button
            className="btn-primary w-full mt-2 flex items-center justify-center gap-2"
            onClick={handleConnect}
            disabled={status === 'checking' || !serverUrl.trim()}
          >
            {status === 'checking' ? (
              <>
                <span className="animate-blink">▋</span>
                <span>Connecting...</span>
              </>
            ) : (
              <>
                <span>Connect</span>
                <span className="text-green/50">→</span>
              </>
            )}
          </button>
        </div>

        <p className="text-center text-dim/40 text-xs mt-4">
          Connection config stored locally in your browser
        </p>
      </div>
    </div>
  )
}
