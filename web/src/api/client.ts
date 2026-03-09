// src/api/client.ts
import type { ConnectionConfig } from './types'

// ─── Storage keys ─────────────────────────────────────────────────────────────

const STORAGE_KEY = 'onlyagents_connection'

export function getConnectionConfig(): ConnectionConfig | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    return JSON.parse(raw) as ConnectionConfig
  } catch {
    return null
  }
}

export function setConnectionConfig(cfg: ConnectionConfig): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(cfg))
}

export function clearConnectionConfig(): void {
  localStorage.removeItem(STORAGE_KEY)
}

// ─── Base URL ─────────────────────────────────────────────────────────────────

export function getBaseUrl(): string {
  const cfg = getConnectionConfig()

  if (cfg?.serverUrl) {
    return cfg.serverUrl.replace(/\/$/, '')
  }

  if (typeof window !== 'undefined') {
    if (window.location.port === '5173') {
      return '' // Vite dev proxy — relative paths
    }
  }

  return 'http://localhost:8080'
}

// ─── Core fetch wrapper ───────────────────────────────────────────────────────

export class ApiError extends Error {
  public status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

interface FetchOptions extends RequestInit {
  skipAuth?: boolean
}

export async function apiFetch<T>(path: string, options: FetchOptions = {}): Promise<T> {
  const cfg = getConnectionConfig()
  const baseUrl = getBaseUrl()

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  if (!options.skipAuth && cfg?.apiKey) {
    headers['X-API-Key'] = cfg.apiKey
  }

  const res = await fetch(`${baseUrl}${path}`, {
    credentials: 'include',
    ...options,
    headers: { ...headers, ...(options.headers as Record<string, string> | undefined) },
  })

  if (!res.ok) {
    let message = `HTTP ${res.status}`
    try {
      const body = await res.json() as { error?: string }
      if (body.error) message = body.error
    } catch { /* ignore */ }
    throw new ApiError(res.status, message)
  }

  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}
