import { memo } from 'react'
import { Handle, Position } from '@xyflow/react'
import type { NodeProps } from '@xyflow/react'
import type { AgentStatus } from '../../api/types'

const STATE_STYLES: Record<string, { border: string; dot: string; label: string }> = {
  active: {
    border: 'border-green',
    dot:    'bg-green shadow-[0_0_6px_#00d97e]',
    label:  'text-green',
  },
  idle: {
    border: 'border-border',
    dot:    'bg-dim/40',
    label:  'text-dim',
  },
  error: {
    border: 'border-red',
    dot:    'bg-red shadow-[0_0_6px_#f85149]',
    label:  'text-red',
  },
}

interface AgentNodeData extends AgentStatus {
  isCenter?: boolean  // true for executive in the radial layout
}

export const AgentNode = memo(function AgentNode({ data }: NodeProps) {
  const agent = data as unknown as AgentNodeData
  const style = STATE_STYLES[agent.state] ?? STATE_STYLES.idle
  const isCenter = agent.isCenter ?? agent.is_executive

  return (
    <>
      {/* Handles — invisible, used by React Flow for edges */}
      <Handle type="target" position={Position.Top}    style={{ opacity: 0 }} />
      <Handle type="source" position={Position.Bottom} style={{ opacity: 0 }} />

      <div
        className={[
          'relative bg-surface border rounded px-3 py-2.5 transition-all duration-300',
          style.border,
          isCenter
            ? 'w-40 shadow-[0_0_24px_rgba(0,217,126,0.08)]'
            : 'w-36',
          agent.state === 'active' && isCenter
            ? 'shadow-[0_0_32px_rgba(0,217,126,0.15)]'
            : '',
        ].join(' ')}
      >
        {/* Active pulse ring — only for active executive */}
        {agent.state === 'active' && isCenter && (
          <div className="absolute inset-0 rounded border border-green/20 animate-ping" />
        )}

        {/* Header row */}
        <div className="flex items-center gap-1.5 mb-1.5">
          <div className={`status-dot ${style.dot} transition-all duration-300`} />
          <span className="text-bright text-xs font-mono font-semibold truncate">
            {agent.name}
          </span>
          {isCenter && (
            <span className="ml-auto text-[9px] text-green/50 shrink-0">EXEC</span>
          )}
        </div>

        {/* Current task */}
        <div className="text-[10px] text-dim min-h-[14px] truncate leading-relaxed">
          {agent.state === 'active' && agent.current_task ? (
            <span className="text-text/70">{agent.current_task}</span>
          ) : (
            <span className={style.label}>{agent.state}</span>
          )}
        </div>

        {/* Model tag */}
        <div className="mt-1.5 text-[9px] text-dim/40 font-mono truncate">
          {agent.model}
        </div>
      </div>
    </>
  )
})
