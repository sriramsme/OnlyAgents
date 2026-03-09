import { create } from 'zustand'
import type { AgentStatus, AgentState } from '../api/types'

export interface AgentRecord extends AgentStatus {
  activeSince?: number  // Date.now() when became active
}

interface AgentStore {
  agents: Record<string, AgentRecord>

  // Initialise from snapshot.agent event on SSE connect
  initAgent: (status: AgentStatus) => void

  // Live updates from SSE
  setAgentActive: (id: string, task: string, model: string) => void
  setAgentIdle:   (id: string, durationMs: number) => void
  setAgentError:  (id: string, error: string) => void
  setAgentState:  (id: string, state: AgentState) => void

  // Ordered list for rendering
  agentList: () => AgentRecord[]
}

export const useAgentStore = create<AgentStore>((set, get) => ({
  agents: {},

  initAgent: (status) =>
    set((s) => ({
      agents: {
        ...s.agents,
        [status.id]: { ...status },
      },
    })),

  setAgentActive: (id, task, model) =>
    set((s) => ({
      agents: {
        ...s.agents,
        [id]: {
          ...(s.agents[id] ?? { id, name: id, is_executive: false }),
          state: 'active',
          current_task: task,
          model,
          last_active: new Date().toISOString(),
          activeSince: Date.now(),
        },
      },
    })),

  setAgentIdle: (id) =>
    set((s) => ({
      agents: {
        ...s.agents,
        [id]: {
          ...(s.agents[id] ?? { id, name: id, is_executive: false }),
          state: 'idle',
          current_task: '',
          last_active: new Date().toISOString(),
          activeSince: undefined,
        },
      },
    })),

  setAgentError: (id, _error) =>
    set((s) => ({
      agents: {
        ...s.agents,
        [id]: {
          ...(s.agents[id] ?? { id, name: id, is_executive: false }),
          state: 'error',
          last_active: new Date().toISOString(),
          activeSince: undefined,
        },
      },
    })),

  setAgentState: (id, state) =>
    set((s) => ({
      agents: {
        ...s.agents,
        [id]: {
          ...(s.agents[id] ?? { id, name: id, is_executive: false }),
          state,
        },
      },
    })),

  agentList: () => Object.values(get().agents),
}))
