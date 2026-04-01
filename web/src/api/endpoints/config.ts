// src/api/endpoints/config.ts
// Read-only endpoints for the four config-backed resources.
import { apiFetch } from '../client'
import type { ConfigSummary, AgentConfig, SkillConfig, ConnectorConfig, ChannelConfig } from '../types'

export const agentsApi = {
    list: () => apiFetch<{ agents: ConfigSummary[] }>('/v1/agents'),
    get: (id: string) => apiFetch<AgentConfig>(`/v1/agents/${id}`),
}

export const skillsApi = {
    list: () => apiFetch<{ skills: ConfigSummary[] }>('/v1/skills'),
    get: (id: string) => apiFetch<SkillConfig>(`/v1/skills/${id}`),
}

export const connectorsApi = {
    list: () => apiFetch<{ connectors: ConfigSummary[] }>('/v1/connectors'),
    get: (id: string) => apiFetch<ConnectorConfig>(`/v1/connectors/${id}`),
}

export const channelsApi = {
    list: () => apiFetch<{ channels: ConfigSummary[] }>('/v1/channels'),
    get: (id: string) => apiFetch<ChannelConfig>(`/v1/channels/${id}`),
}
