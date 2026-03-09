import { useState, useRef } from 'react'

interface ChatInputProps {
  onSend: (message: string) => void
  disabled: boolean
}

export function ChatInput({ onSend, disabled }: ChatInputProps) {
  const [value, setValue] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  function handleSubmit() {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
    // Reset textarea height
    if (textareaRef.current) textareaRef.current.style.height = 'auto'
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }

  function handleInput() {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`
  }

  return (
    <div className="border-t border-border bg-surface px-4 py-3">
      <div className="flex gap-2 items-end">
        <div className="flex-1 relative">
          <span className="absolute left-3 top-2.5 text-green/50 text-xs select-none">
            &gt;
          </span>
          <textarea
            ref={textareaRef}
            className="w-full bg-canvas border border-border rounded px-3 py-2 pl-6
                       text-text font-mono text-xs resize-none min-h-[38px] max-h-40
                       focus:outline-none focus:border-green/50
                       placeholder-dim/30 transition-colors duration-150"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={handleKeyDown}
            onInput={handleInput}
            placeholder="Message the executive agent…  (Shift+Enter for newline)"
            disabled={disabled}
            rows={1}
          />
        </div>

        <button
          className="btn-primary shrink-0 h-[38px] px-3"
          onClick={handleSubmit}
          disabled={disabled || !value.trim()}
        >
          {disabled ? (
            <span className="animate-blink">▋</span>
          ) : (
            <span>⏎</span>
          )}
        </button>
      </div>
    </div>
  )
}
