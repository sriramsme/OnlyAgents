// src/api/endpoints/chat.ts
// Chat handled over WebSocket (src/api/ws.ts).
// This file only contains REST endpoints for session management (non-realtime).
import { apiFetch } from '../client'
import type { ChatHistoryResponse, Session } from '../types'

export const sessions = {
  // List all sessions for the current user
  list: () => apiFetch<{ sessions: Session[]; count: number }>('/v1/sessions'),

  // Get message history for a specific session
  history: (sessionId: string) =>
    apiFetch<ChatHistoryResponse>(`/v1/sessions/${sessionId}/history`),

  // End a session (next message starts fresh)
  end: (sessionId: string) =>
    apiFetch<{ status: string }>(`/v1/sessions/${sessionId}`, { method: 'DELETE' }),
}
