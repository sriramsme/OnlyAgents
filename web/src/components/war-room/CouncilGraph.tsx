import { useEffect, useRef, useMemo, useState } from 'react'
import {
  ReactFlow,
  Background,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { useAgentStore } from '../../store/agentStore'
import { useEventStore } from '../../store/eventStore'
import { AgentNode } from './AgentNode'
import { DelegationEdge } from './DelegationEdge'
import type { AgentStatus, DelegationPayload } from '../../api/types'

const NODE_TYPES = { agentNode: AgentNode }
const EDGE_TYPES = { delegationEdge: DelegationEdge }

function buildLayout(agents: AgentStatus[]) {
  const executive = agents.find((a) => a.is_executive)
  const subs      = agents.filter((a) => !a.is_executive)
  const nodes: Node[] = []
  const edges: Edge[] = []

  // Use 500x400 as the logical canvas — React Flow fitView handles scaling
  const cx = 500, cy = 400, radius = 280

  if (executive) {
    nodes.push({
      id:        executive.id,
      type:      'agentNode',
      position:  { x: cx - 80, y: cy - 40 },
      data:      { ...executive, isCenter: true },
      draggable: true,
    })
  }

  subs.forEach((agent, i) => {
    const angle = (i / Math.max(subs.length, 1)) * 2 * Math.PI - Math.PI / 2
    nodes.push({
      id:        agent.id,
      type:      'agentNode',
      position:  {
        x: cx + Math.cos(angle) * radius - 72,
        y: cy + Math.sin(angle) * radius - 40,
      },
      data:      agent,
      draggable: true,
    })
    if (executive) {
      edges.push({
        id:     `exec-${agent.id}`,
        source: executive.id,
        target: agent.id,
        type:   'delegationEdge',
        data:   { animated: false },
      })
    }
  })

  return { nodes, edges }
}

function agentMemberKey(m: Record<string, AgentStatus>): string {
  return Object.keys(m).sort().join(',')
}

export function CouncilGraph() {
  const containerRef = useRef<HTMLDivElement>(null)
  const [ready, setReady] = useState(false)

  // Wait until the container has real pixel dimensions before mounting React Flow
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect
      if (width > 0 && height > 0) setReady(true)
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const agentsMap = useAgentStore((s) => s.agents)
  const events    = useEventStore((s) => s.events)
  const memberKey = useMemo(() => agentMemberKey(agentsMap), [agentsMap])
  const agentList = useMemo(() => Object.values(agentsMap), [agentsMap])

  const { nodes: initNodes, edges: initEdges } = useMemo(
    () => buildLayout(agentList),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )

  const [nodes, setNodes, onNodesChange] = useNodesState(initNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initEdges)

  // Rebuild layout when agents are added/removed
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    const { nodes: n, edges: e } = buildLayout(agentList)
    setNodes(n)
    setEdges(e)
  }, [memberKey]) // eslint-disable-line react-hooks/exhaustive-deps

  // Sync state/task changes into existing nodes
  const prevAgentsRef = useRef(agentsMap)
  useEffect(() => {
    const prev = prevAgentsRef.current
    prevAgentsRef.current = agentsMap
    const changedIds = Object.keys(agentsMap).filter((id) => {
      const a = agentsMap[id], p = prev[id]
      return !p || p.state !== a.state || p.current_task !== a.current_task
    })
    if (changedIds.length === 0) return
    setNodes((nds) =>
      nds.map((n) => {
        if (!changedIds.includes(n.id)) return n
        const agent = agentsMap[n.id]
        if (!agent) return n
        return { ...n, data: { ...agent, isCenter: agent.is_executive } }
      })
    )
  }, [agentsMap, setNodes])

  // Animate delegation edges
  const lastProcessedIdx = useRef(-1)
  const delegationTimers = useRef<Record<string, ReturnType<typeof setTimeout>>>({})
  useEffect(() => {
    if (events.length === 0) return
    const startIdx = lastProcessedIdx.current + 1
    if (startIdx >= events.length) return
    lastProcessedIdx.current = events.length - 1
    for (const evt of events.slice(startIdx)) {
      if (evt.type !== 'delegation') continue
      const p = evt.payload as DelegationPayload
      if (!p) continue
      const edgeId = `exec-${p.to_agent}`
      setEdges((eds) =>
        eds.map((e) =>
          e.id === edgeId ? { ...e, data: { animated: true, task: p.task } } : e
        )
      )
      if (delegationTimers.current[edgeId]) clearTimeout(delegationTimers.current[edgeId])
      delegationTimers.current[edgeId] = setTimeout(() => {
        setEdges((eds) =>
          eds.map((e) => e.id === edgeId ? { ...e, data: { animated: false } } : e)
        )
      }, 3_000)
    }
  }, [events, setEdges])

  return (
    <div ref={containerRef} style={{ position: 'absolute', inset: 0 }}>
      {ready && (
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          nodeTypes={NODE_TYPES}
          edgeTypes={EDGE_TYPES}
          fitView
          fitViewOptions={{ padding: 0.25 }}
          minZoom={0.3}
          maxZoom={2}
          proOptions={{ hideAttribution: true }}
          style={{ background: 'transparent' }}
        >
          <Background
            variant={BackgroundVariant.Dots}
            gap={32}
            size={1}
            color="#1e2d3d"
          />
        </ReactFlow>
      )}
      {!ready && (
        <div style={{
          position: 'absolute', inset: 0,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }} className="text-dim/30 text-xs font-mono">
          initialising council...
        </div>
      )}
    </div>
  )
}
