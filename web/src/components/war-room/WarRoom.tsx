import { ReactFlowProvider } from '@xyflow/react'
import { CouncilGraph } from './CouncilGraph'
import { EventLog } from './EventLog'
import { useAgentStore } from '../../store/agentStore'
import { useEventStore } from '../../store/eventStore'

function StatusBar() {
  const agentsCount = useAgentStore((s) => Object.keys(s.agents).length)
  const activeCount = useAgentStore((s) =>
    Object.values(s.agents).filter((a) => a.state === 'active').length
  )
  const connected = useEventStore((s) => Object.keys(s.events).length > 0)



  return (
    <div style={{ height: 32, flexShrink: 0 }}
         className="border-t border-border flex items-center px-4 gap-6 text-[10px] font-mono bg-surface/50">
      <span className={connected ? 'text-green' : 'text-red'}>
        {connected ? '● LIVE' : '○ OFFLINE'}
      </span>
      <span className="text-dim">
        {agentsCount} agent{agentsCount !== 1 ? 's' : ''}
      </span>
      {activeCount > 0 && (
        <span className="text-amber">{activeCount} active</span>
      )}
      <span className="ml-auto text-dim/30">OnlyAgents</span>
    </div>
  )
}


export function WarRoom() {
  return (
    <div style={{
      position: 'absolute',
      inset: 0,
      display: 'flex',
      flexDirection: 'column',
      background: '#080c10',
      overflow: 'hidden',
    }}>
      {/* Graph row */}
      <div style={{
        flex: 1,
        display: 'flex',
        flexDirection: 'row',
        overflow: 'hidden',
        minHeight: 0,
      }}>
        {/* Council graph */}
        <div style={{
          flex: 1,
          position: 'relative',
          overflow: 'hidden',
          minWidth: 0,
        }}>
          <ReactFlowProvider>
            <CouncilGraph />
          </ReactFlowProvider>
        </div>

        {/* Event log */}
        <EventLog />
      </div>

      <StatusBar />
    </div>
  )
}
