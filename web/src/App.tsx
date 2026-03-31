import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, useParams } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { getConnectionConfig } from './api/client'
import { apiFetch, ApiError } from './api/client'
import { ConnectScreen } from './components/connection/ConnectScreen'
import { LoginPage } from './components/auth/LoginPage'
import { Shell } from './components/layout/Shell'
import { WarRoom } from './components/war-room/WarRoom'
import { ChatPage } from './components/chat/ChatPage'
import { getWSInstance } from './api/ws'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false },
  },
})

// Three-stage app state:
//   disconnected → no server config stored or server unreachable
//   unauthenticated → server reachable but not logged in
//   authenticated → session valid, render the app
type AppState = 'checking' | 'disconnected' | 'unauthenticated' | 'authenticated'

export default function App() {
  const [appState, setAppState] = useState<AppState>('checking')

  useEffect(() => {
    void checkSession()
  }, [])

  async function checkSession() {
    const cfg = getConnectionConfig()
    if (!cfg) {
      setAppState('disconnected')
      return
    }

    try {
      // /health is open — confirms server is reachable
      await apiFetch('/health', { skipAuth: true })
    } catch {
      setAppState('disconnected')
      return
    }

    try {
      // /auth/me is authed — confirms session cookie is valid
      // credentials:'include' sends the cookie cross-origin if needed
      await apiFetch('/auth/me', {
        credentials: 'include' as RequestCredentials,
      })
      setAppState('authenticated')
      getWSInstance().connect()
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setAppState('unauthenticated')
      } else {
        // /auth/me errored for non-auth reason — assume unauthenticated
        setAppState('unauthenticated')
      }
    }
  }

  if (appState === 'checking') {
    return (
      <div style={{
        position: 'fixed', inset: 0, background: '#080c10',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontFamily: 'JetBrains Mono, monospace',
      }}>
        <div style={{ color: '#00d97e', fontSize: 12, display: 'flex', gap: 8 }}>
          <span style={{ animation: 'blink 1.2s step-end infinite' }}>▋</span>
          <span>Initialising...</span>
        </div>
        <style>{`@keyframes blink { 0%,100%{opacity:1} 50%{opacity:0} }`}</style>
      </div>
    )
  }

  if (appState === 'disconnected') {
    return (
      <ConnectScreen onConnected={() => setAppState('unauthenticated')} />
    )
  }

  if (appState === 'unauthenticated') {
    return (
      <LoginPage onAuthenticated={() => setAppState('authenticated')} />
    )
  }

  function ChatPageWithKey() {
      const { sessionId } = useParams()
      return <ChatPage key={sessionId ?? 'new'} />
  }
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route element={<Shell />}>
            <Route index element={<WarRoom />} />
<Route path="chat" element={<ChatPageWithKey />} />
<Route path="chat/:sessionId" element={<ChatPageWithKey />} />
            {/* Phase C routes — uncomment as panels are built */}
            {/* <Route path="memory"   element={<MemoryPage />} /> */}
            {/* <Route path="tasks"    element={<TasksPage />} /> */}
            {/* <Route path="calendar" element={<CalendarPage />} /> */}
            {/* <Route path="notes"    element={<NotesPage />} /> */}
            {/* <Route path="config"   element={<ConfigPage />} /> */}
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
