// src/api/endpoints/health.ts
import { apiFetch } from '../client'
import type { HealthResponse, ServerInfo } from '../types'

export const health = {
  check: () => apiFetch<HealthResponse>('/health', { skipAuth: true }),
  me: () => apiFetch<ServerInfo>('/auth/me', { skipAuth: true }),
}

// src/api/endpoints/agents.ts
import { apiFetch as _apiFetch } from '../client'
import type { AgentStatus } from '../types'

export const agents = {
  list: () => _apiFetch<AgentStatus[]>('/v1/agents'),
}
