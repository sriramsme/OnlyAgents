import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { useEventStream } from '../../hooks/useEventStream'

export function Shell() {
  useEventStream(true)

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      display: 'flex',
      flexDirection: 'row',
      overflow: 'hidden',
      background: '#080c10',
    }}>
      <Sidebar />
      {/* main fills remaining width, position:relative so children can use absolute inset:0 */}
      <div style={{
        flex: 1,
        position: 'relative',
        overflow: 'hidden',
        minWidth: 0,
        minHeight: 0,
      }}>
        <Outlet />
      </div>
    </div>
  )
}
