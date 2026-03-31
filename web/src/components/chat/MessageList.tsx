import { useEffect, useRef, useState } from 'react'
import remarkGfm from 'remark-gfm'
import type { ChatMessage } from '../../api/types'
import type { LiveThinkingBlock } from './ChatPage'
import { lazy, Suspense } from 'react'

const ReactMarkdown = lazy(() => import('react-markdown'))

// ─── Props ────────────────────────────────────────────────────────────────────

interface MessageListProps {
  messages: ChatMessage[]
  isLoading: boolean
  liveThinking: LiveThinkingBlock | null
}

// ─── Grouping ─────────────────────────────────────────────────────────────────

interface MessageGroup {
  assistant: ChatMessage
  toolMessages: ChatMessage[]
}

function groupMessages(messages: ChatMessage[]): Array<ChatMessage | MessageGroup> {
  const result: Array<ChatMessage | MessageGroup> = []
  for (let i = 0; i < messages.length; i++) {
    const msg = messages[i]
    if (msg.role === 'assistant') {
      const tools: ChatMessage[] = []
      while (i + 1 < messages.length && messages[i + 1].role === 'tool') {
        i++
        tools.push(messages[i])
      }
      result.push({ assistant: msg, toolMessages: tools })
    } else {
      result.push(msg)
    }
  }
  return result
}

// ─── Shared thinking shell ────────────────────────────────────────────────────

function ThinkingShell({
  title,
  streaming,
  children,
}: {
  title: string
  streaming: boolean
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(streaming) // live = open, historical = closed

  return (
    <div style={{ marginBottom: 6 }}>
      <button
        onClick={() => setOpen((o) => !o)}
        style={{
          display: 'inline-flex', alignItems: 'center', gap: 5,
          background: 'none', border: 'none', cursor: 'pointer',
          color: streaming ? '#d29922' : '#4a6070',
          fontSize: 11, padding: '2px 0', letterSpacing: '0.03em',
          maxWidth: '100%', textAlign: 'left',
        }}
      >
        <span style={{
          display: 'inline-block', flexShrink: 0,
          transition: 'transform 0.18s ease',
          transform: open ? 'rotate(90deg)' : 'rotate(0deg)',
          fontSize: 9, opacity: 0.7,
        }}>
          ▶
        </span>

        {/* Pulsing dot while streaming */}
        {streaming && (
          <span style={{
            width: 5, height: 5, borderRadius: '50%', flexShrink: 0,
            background: '#d29922', display: 'inline-block',
            animation: 'oa-pulse 1.2s ease-in-out infinite',
          }} />
        )}

        <span style={{
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          maxWidth: 400,
        }}>
          {title}
        </span>
      </button>

      {open && (
        <div style={{
          marginTop: 6,
          borderLeft: '2px solid #1a2d3d',
          paddingLeft: 12,
          display: 'flex', flexDirection: 'column', gap: 5,
        }}>
          {children}
        </div>
      )}
    </div>
  )
}

// ─── Tool event row (no extra nesting — just one row) ────────────────────────

function ToolRow({
  toolName,
  input,
  done,
  success,
  durationMs,
}: {
  toolName: string
  input?: string
  done: boolean
  success?: boolean
  durationMs?: number
}) {
  const [expanded, setExpanded] = useState(false)

  // Pretty-print JSON input
  let prettyInput = input ?? ''
  try {
    if (prettyInput) prettyInput = JSON.stringify(JSON.parse(prettyInput), null, 2)
  } catch { /* leave raw */ }

  return (
    <div style={{ fontSize: 10, fontFamily: "'JetBrains Mono','Fira Code',monospace" }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
        {/* Status indicator */}
        <span style={{
          width: 5, height: 5, borderRadius: '50%', display: 'inline-block', flexShrink: 0,
          background: !done ? 'transparent' : success ? '#00bfa5' : '#f85149',
          border: !done ? '1px solid #4a7a94' : 'none',
          animation: !done ? 'oa-pulse 1.2s ease-in-out infinite' : 'none',
        }} />

        {/* Tool name */}
        <span style={{ color: '#00bfa5' }}>{toolName}</span>

        {/* Duration */}
        {durationMs !== undefined && (
          <span style={{ color: '#2a4050' }}>{durationMs}ms</span>
        )}

        {/* Expand toggle — only if there's input */}
        {prettyInput && (
          <button
            onClick={() => setExpanded((o) => !o)}
            style={{
              background: 'none', border: 'none', cursor: 'pointer',
              color: '#3a5060', fontSize: 9, padding: '0 2px',
            }}
          >
            {expanded ? '▲' : '▼'}
          </button>
        )}
      </div>

      {expanded && prettyInput && (
        <pre style={{
          margin: '4px 0 0 10px',
          background: 'rgba(0,0,0,0.25)', borderRadius: 4,
          padding: '5px 8px', overflowX: 'auto',
          color: '#5d8a9a', fontSize: 10,
          whiteSpace: 'pre-wrap', wordBreak: 'break-all',
        }}>
          {prettyInput}
        </pre>
      )}
    </div>
  )
}

// ─── Live thinking block ──────────────────────────────────────────────────────

function LiveThinkingDropdown({ block }: { block: LiveThinkingBlock }) {
  const title = block.latestLabel || 'Thinking…'
  const hasContent = block.toolEvents.length > 0 || !!block.reasoning

  return (
    <ThinkingShell title={title} streaming>
      {!hasContent && (
        <span style={{ color: '#3a5060', fontSize: 10, fontStyle: 'italic' }}>working…</span>
      )}
      {block.reasoning && (
        <div style={{
          fontSize: 11, color: '#5d7a8a', lineHeight: 1.6,
          fontStyle: 'italic', whiteSpace: 'pre-wrap',
        }}>
          {block.reasoning}
        </div>
      )}
      {block.toolEvents.map((evt) => (
        <ToolRow
          key={evt.id}
          toolName={evt.toolName}
          input={evt.input}
          done={evt.done}
          success={evt.success}
          durationMs={evt.durationMs}
        />
      ))}
    </ThinkingShell>
  )
}

// ─── Historical thinking block ────────────────────────────────────────────────

function HistoricalThinkingDropdown({
  reasoning,
  toolMessages,
}: {
  reasoning?: string
  toolMessages: ChatMessage[]
}) {
  if (!reasoning && toolMessages.length === 0) return null

  // Build the title: first line of reasoning, or list of tool names
  let title = 'Thinking'
  if (reasoning?.trim()) {
    const firstLine = reasoning.trim().split('\n')[0]
    title = firstLine.length > 60 ? firstLine.slice(0, 60) + '…' : firstLine
  } else if (toolMessages.length > 0) {
    const names = toolMessages.map((t) => {
      if (!t.toolCalls) return t.toolCallId ?? 'tool'
      try {
        const p = JSON.parse(t.toolCalls)
        const first = Array.isArray(p) ? p[0] : p
        return first?.function?.name ?? 'tool'
      } catch { return 'tool' }
    })
    const joined = names.join(', ')
    title = joined.length > 60 ? joined.slice(0, 60) + '…' : joined
  }

  return (
    <ThinkingShell title={title} streaming={false}>
      {reasoning && (
        <div style={{
          fontSize: 11, color: '#5d7a8a', lineHeight: 1.6,
          fontStyle: 'italic', whiteSpace: 'pre-wrap',
        }}>
          {reasoning}
        </div>
      )}
      {toolMessages.map((tm) => {
        let toolName = tm.toolCallId ?? 'tool'
        let input = tm.content
        if (tm.toolCalls) {
          try {
            const p = JSON.parse(tm.toolCalls)
            const first = Array.isArray(p) ? p[0] : p
            if (first?.function?.name) toolName = first.function.name
            if (first?.function?.arguments) input = first.function.arguments
          } catch { /* ignore */ }
        }
        return (
          <ToolRow
            key={tm.id ?? tm.timestamp}
            toolName={toolName}
            input={input}
            done
            success
          />
        )
      })}
    </ThinkingShell>
  )
}

// ─── Assistant message group ──────────────────────────────────────────────────

function AssistantGroup({ group }: { group: MessageGroup }) {
  const hasThinking = !!(group.assistant.reasoningContent || group.toolMessages.length > 0)

  return (
    <div className="flex gap-3">
      <div className="w-6 h-6 rounded border border-green/30 flex items-center justify-center text-green text-[10px] shrink-0 mt-0.5">
        ⬡
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        {hasThinking && (
          <HistoricalThinkingDropdown
            reasoning={group.assistant.reasoningContent}
            toolMessages={group.toolMessages}
          />
        )}
        {group.assistant.content && (
          <div className="bg-green/5 border border-green/20 rounded px-3 py-2 text-xs leading-relaxed text-text max-w-[75%]">
            <div className="prose prose-invert prose-sm max-w-none break-words">
              <Suspense fallback={<div className="text-dim/40">...</div>}>
                <ReactMarkdown remarkPlugins={[remarkGfm]}>
                  {group.assistant.content}
                </ReactMarkdown>
              </Suspense>
            </div>
            <div className="text-[9px] text-dim/30 mt-2 text-right">
              {new Date(group.assistant.timestamp).toLocaleTimeString('en-US', {
                hour12: false, hour: '2-digit', minute: '2-digit',
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// ─── Main component ───────────────────────────────────────────────────────────

export function MessageList({ messages, isLoading, liveThinking }: MessageListProps) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [messages, isLoading, liveThinking])

  const groups = groupMessages(messages)

  return (
    <>
      <style>{`
        @keyframes oa-pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.25; }
        }
      `}</style>

      <div
        ref={containerRef}
        className="flex-1 overflow-y-auto px-4 py-4 space-y-4 text-sm"
        style={{ height: '100%' }}
      >
        {messages.length === 0 && !isLoading && (
          <div className="text-center text-dim/30 mt-16 text-xs">
            <div className="text-2xl mb-2">⬡</div>
            <div>Send a message to the executive agent</div>
          </div>
        )}

        {groups.map((item) => {
          if ('assistant' in item) {
            return <AssistantGroup key={item.assistant.id ?? item.assistant.timestamp} group={item} />
          }

          if (item.role === 'user') {
            return (
              <div key={item.id ?? item.timestamp} className="flex gap-3 justify-end">
                <div className="max-w-[75%] rounded px-3 py-2 text-xs leading-relaxed bg-surface border border-border text-text/80 ml-auto">
                  <div className="prose prose-invert prose-sm max-w-none break-words">
                    <Suspense fallback={<div className="text-dim/40">...</div>}>
                      <ReactMarkdown remarkPlugins={[remarkGfm]}>{item.content}</ReactMarkdown>
                    </Suspense>
                  </div>
                  <div className="text-[9px] text-dim/30 mt-2 text-right">
                    {new Date(item.timestamp).toLocaleTimeString('en-US', {
                      hour12: false, hour: '2-digit', minute: '2-digit',
                    })}
                  </div>
                </div>
                <div className="w-6 h-6 rounded border border-border flex items-center justify-center text-dim text-[10px] shrink-0 mt-0.5">
                  U
                </div>
              </div>
            )
          }

          if (item.role === 'notification') {
            return (
              <div
                key={item.id ?? item.timestamp}
                style={{ textAlign: 'center', fontSize: 10, color: '#5d7a8a', padding: '4px 0' }}
              >
                <Suspense fallback={null}>
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{item.content}</ReactMarkdown>
                </Suspense>
              </div>
            )
          }

          return null
        })}

        {/* Live thinking block — streamed while agent works */}
        {liveThinking && (
          <div className="flex gap-3">
            <div className="w-6 h-6 rounded border border-green/30 flex items-center justify-center text-green text-[10px] shrink-0 mt-0.5">
              ⬡
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <LiveThinkingDropdown block={liveThinking} />
            </div>
          </div>
        )}

        {/* Fallback spinner if thinking but no live block yet */}
        {isLoading && !liveThinking && (
          <div className="flex gap-3">
            <div className="w-6 h-6 rounded border border-green/30 flex items-center justify-center text-green text-[10px] shrink-0 mt-0.5">
              ⬡
            </div>
            <div className="bg-green/5 border border-green/20 rounded px-3 py-2">
              <span className="text-green text-xs animate-pulse">thinking</span>
              <span className="text-green animate-blink ml-0.5">▋</span>
            </div>
          </div>
        )}

        <div />
      </div>
    </>
  )
}
