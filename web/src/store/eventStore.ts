import { create } from 'zustand'
import type { UIEvent } from '../api/types'

const MAX_EVENTS = 200

interface EventStore {
  events: UIEvent[]
  lastPing: number        // Date.now() of last heartbeat — for connection health
  connected: boolean

  push:         (evt: UIEvent) => void
  setConnected: (v: boolean) => void
  setLastPing:  () => void
  clear:        () => void
}

export const useEventStore = create<EventStore>((set) => ({
  events: [],
  lastPing: 0,
  connected: false,

  push: (evt) =>
    set((s) => ({
      events:
        s.events.length >= MAX_EVENTS
          ? [...s.events.slice(1), evt]   // drop oldest, append newest
          : [...s.events, evt],
    })),

  setConnected: (v) => set({ connected: v }),
  setLastPing:  ()  => set({ lastPing: Date.now() }),
  clear:        ()  => set({ events: [] }),
}))
