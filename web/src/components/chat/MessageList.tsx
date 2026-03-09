// ─── MessageList ──────────────────────────────────────────────────────────────
import { useEffect, useRef } from 'react'
import type { ChatMessage } from '../../api/types'

interface MessageListProps {
  messages: ChatMessage[]
  isLoading: boolean
}

export function MessageList({ messages, isLoading }: MessageListProps) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages.length, isLoading])

  return (
    <div className="flex-1 overflow-y-auto px-4 py-4 space-y-4 font-mono text-sm">
      {messages.length === 0 && !isLoading && (
        <div className="text-center text-dim/30 mt-16 text-xs">
          <div className="text-2xl mb-2">⬡</div>
          <div>Send a message to the executive agent</div>
        </div>
      )}

      {messages.map((msg, i) => (
        <div key={i} className={`flex gap-3 ${msg.role === 'user' ? 'justify-end' : ''}`}>
          {msg.role === 'assistant' && (
            <div className="w-6 h-6 rounded border border-green/30 flex items-center
                           justify-center text-green text-[10px] shrink-0 mt-0.5">
              ⬡
            </div>
          )}

          <div
            className={[
              'max-w-[75%] rounded px-3 py-2 text-xs leading-relaxed',
              msg.role === 'user'
                ? 'bg-surface border border-border text-text/80 ml-auto'
                : 'bg-green/5 border border-green/20 text-text',
            ].join(' ')}
          >
            <div className="whitespace-pre-wrap break-words">{msg.content}</div>
            <div className="text-[9px] text-dim/30 mt-1 text-right">
              {new Date(msg.timestamp).toLocaleTimeString('en-US', {
                hour12: false, hour: '2-digit', minute: '2-digit',
              })}
            </div>
          </div>

          {msg.role === 'user' && (
            <div className="w-6 h-6 rounded border border-border flex items-center
                           justify-center text-dim text-[10px] shrink-0 mt-0.5">
              U
            </div>
          )}
        </div>
      ))}

      {/* Thinking indicator */}
      {isLoading && (
        <div className="flex gap-3">
          <div className="w-6 h-6 rounded border border-green/30 flex items-center
                         justify-center text-green text-[10px] shrink-0 mt-0.5">
            ⬡
          </div>
          <div className="bg-green/5 border border-green/20 rounded px-3 py-2">
            <span className="text-green text-xs animate-pulse">thinking</span>
            <span className="text-green animate-blink ml-0.5">▋</span>
          </div>
        </div>
      )}

      <div ref={bottomRef} />
    </div>
  )
}
