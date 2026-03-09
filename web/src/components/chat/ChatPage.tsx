import { useState, useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getWSInstance } from '../../api/ws'
import { sessions } from '../../api/endpoints/chat'
import { MessageList } from './MessageList'
import { ChatInput } from './ChatInput'
import type { ChatMessage } from '../../api/types'

export function ChatPage() {
  const ws = getWSInstance()

  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isThinking, setIsThinking] = useState(false)
  const [streamingContent, setStreamingContent] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const sessionId = ws.sessionId

  // Load history for current session on mount
  const { data: historyData } = useQuery({
    queryKey: ['chat-history', sessionId],
    queryFn: () => (sessionId ? sessions.history(sessionId) : Promise.resolve(null)),
    enabled: !!sessionId,
    staleTime: 30_000,
  })

  // Seed messages from history on first load
  const seededRef = useRef(false)
  useEffect(() => {
    if (historyData?.history && !seededRef.current) {
      seededRef.current = true
      setMessages(historyData.history)
    }
  }, [historyData])

  // Register chat handlers on this WS instance
  useEffect(() => {
    ws.handlers.onAgentThinking = () => {
      setIsThinking(true)
      setError(null)
    }

    ws.handlers.onAgentText = ({ content, is_final }) => {
      if (!is_final) {
        // Streaming token — accumulate into streamingContent
        setStreamingContent((prev) => (prev ?? '') + content)
        setIsThinking(false)
      } else {
        // Final response — commit to messages, clear streaming state
        setStreamingContent(null)
        setIsThinking(false)
        setMessages((prev) => [
          ...prev,
          {
            role: 'assistant',
            content,
            timestamp: new Date().toISOString(),
          },
        ])
      }
    }

    ws.handlers.onNotification = ({ title, body }) => {
      // Proactive agent message — render as assistant message
      setMessages((prev) => [
        ...prev,
        {
          role: 'assistant',
          content: `**${title}** ${body}`,
          timestamp: new Date().toISOString(),
        },
      ])
    }

    ws.handlers.onNewSession = () => {
      // Session was reset — clear local state
      setMessages([])
      setStreamingContent(null)
      setIsThinking(false)
      seededRef.current = false
    }

    return () => {
      ws.handlers.onAgentThinking = undefined
      ws.handlers.onAgentText = undefined
      ws.handlers.onNotification = undefined
      ws.handlers.onNewSession = undefined
    }
  }, [ws])

  function handleSend(message: string) {
    setError(null)
    // Optimistic user message
    setMessages((prev) => [
      ...prev,
      { role: 'user', content: message, timestamp: new Date().toISOString() },
    ])
    ws.send(message)
  }

  function handleNewSession() {
    ws.newSession()
  }

  // Merge committed messages with in-progress streaming message
  const displayMessages: ChatMessage[] = streamingContent != null
    ? [
        ...messages,
        {
          role: 'assistant',
          content: streamingContent,
          timestamp: new Date().toISOString(),
        },
      ]
    : messages

  const isPending = isThinking || streamingContent != null

  return (
    <div style={{
      position: 'absolute',
      inset: 0,
      display: 'flex',
      flexDirection: 'column',
      background: '#080c10',
      overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '10px 16px',
        borderBottom: '1px solid #1e2d3d',
        background: 'rgba(13,17,23,0.5)',
        flexShrink: 0,
      }}>
        <div style={{
          width: 8, height: 8, borderRadius: '50%',
          background: 'rgba(0,217,126,0.5)',
        }} />
        <span style={{
          color: '#8b9eb0', fontSize: 11,
          textTransform: 'uppercase', letterSpacing: '0.1em',
          fontFamily: 'inherit',
        }}>
          Executive Agent
        </span>
        {isPending && (
          <span style={{
            color: '#d29922',
            fontSize: 10,
            fontFamily: 'inherit',
          }}>
            {isThinking ? 'thinking…' : 'responding…'}
          </span>
        )}
        {/* New session button */}
        <button
          onClick={handleNewSession}
          style={{
            marginLeft: 'auto',
            background: 'none',
            border: '1px solid #1e2d3d',
            borderRadius: 4,
            color: '#8b9eb0',
            fontSize: 10,
            padding: '2px 8px',
            cursor: 'pointer',
            fontFamily: 'inherit',
          }}
        >
          new session
        </button>
      </div>

      {/* Messages */}
      <div style={{ flex: 1, overflow: 'hidden', minHeight: 0 }}>
        <MessageList messages={displayMessages} isLoading={isThinking} />
      </div>

      {/* Error */}
      {error && (
        <div style={{
          padding: '8px 16px',
          background: 'rgba(248,81,73,0.05)',
          borderTop: '1px solid rgba(248,81,73,0.2)',
          color: '#f85149',
          fontSize: 11,
          fontFamily: 'inherit',
          flexShrink: 0,
        }}>
          {error}
        </div>
      )}

      {/* Input */}
      <ChatInput onSend={handleSend} disabled={isPending} />
    </div>
  )
}
