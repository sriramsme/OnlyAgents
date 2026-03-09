import { memo } from 'react'
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
  type EdgeProps,
} from '@xyflow/react'

interface DelegationEdgeData {
  animated?: boolean
  task?: string
}

export const DelegationEdge = memo(function DelegationEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  data,
}: EdgeProps) {
  const edgeData = (data ?? {}) as DelegationEdgeData
  const isActive = edgeData.animated === true

  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  })

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        style={{
          stroke: isActive ? '#00d97e' : '#1e2d3d',
          strokeWidth: isActive ? 1.5 : 1,
          strokeDasharray: isActive ? '6 3' : undefined,
          transition: 'stroke 0.3s ease, stroke-width 0.3s ease',
          filter: isActive ? 'drop-shadow(0 0 4px #00d97e66)' : undefined,
        }}
      />

      {/* Show task label when active */}
      {isActive && edgeData.task && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
              pointerEvents: 'none',
            }}
            className="bg-canvas border border-green/30 text-green text-[9px] font-mono
                       px-1.5 py-0.5 rounded max-w-[120px] truncate"
          >
            {edgeData.task}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
})
