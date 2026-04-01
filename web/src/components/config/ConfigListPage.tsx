// src/components/config/ConfigListPage.tsx
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { RefreshCw, Bot, Zap, Link2, Radio, ChevronRight } from 'lucide-react'
import { agentsApi, skillsApi, connectorsApi, channelsApi } from '../../api/endpoints/config'
import type { ConfigSummary } from '../../api/types'
import { ConfigDetail } from './ConfigDetail'

// ─── Resource map ─────────────────────────────────────────────────────────────

type Resource = 'agents' | 'skills' | 'connectors' | 'channels'

interface ResourceMeta {
    label: string
    Icon: React.ComponentType<{ size?: number; strokeWidth?: number, color?: string }>
    accent: string
    listKey: string
    listFn: () => Promise<Record<string, ConfigSummary[]>>
    getFn: (id: string) => Promise<unknown>
}

const RESOURCES: Record<Resource, ResourceMeta> = {
    agents: {
        label: 'Agents',
        Icon: Bot,
        accent: '#00d97e',
        listKey: 'agents',
        listFn: agentsApi.list as () => Promise<Record<string, ConfigSummary[]>>,
        getFn: agentsApi.get,
    },
    skills: {
        label: 'Skills',
        Icon: Zap,
        accent: '#388bfd',
        listKey: 'skills',
        listFn: skillsApi.list as () => Promise<Record<string, ConfigSummary[]>>,
        getFn: skillsApi.get,
    },
    connectors: {
        label: 'Connectors',
        Icon: Link2,
        accent: '#d29922',
        listKey: 'connectors',
        listFn: connectorsApi.list as () => Promise<Record<string, ConfigSummary[]>>,
        getFn: connectorsApi.get,
    },
    channels: {
        label: 'Channels',
        Icon: Radio,
        accent: '#bc8cff',
        listKey: 'channels',
        listFn: channelsApi.list as () => Promise<Record<string, ConfigSummary[]>>,
        getFn: channelsApi.get,
    },
}

// ─── Props ────────────────────────────────────────────────────────────────────

interface Props {
    resource: Resource
}

// ─── Component ────────────────────────────────────────────────────────────────

export function ConfigListPage({ resource }: Props) {
    const meta = RESOURCES[resource]
    const [selectedId, setSelectedId] = useState<string | null>(null)

    const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
        queryKey: [resource],
        queryFn: meta.listFn,
    })

    // Filter out zero-value rows (backend safety net: id and name both empty)
    const raw: ConfigSummary[] = (data as Record<string, ConfigSummary[]>)?.[meta.listKey] ?? []
    const items = raw.filter(item => item.id || item.name)

    return (
        <div style={{ display: 'flex', height: '100%', overflow: 'hidden' }}>
            {/* ── List panel ── */}
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>

                {/* Header */}
                <div style={{
                    padding: '18px 24px',
                    borderBottom: '1px solid #1e2d3d',
                    flexShrink: 0,
                    display: 'flex',
                    alignItems: 'center',
                    gap: 10,
                }}>
                    <meta.Icon size={16} strokeWidth={1.5} color={meta.accent} />
                    <div style={{ flex: 1 }}>
                        <h1 style={{
                            fontSize: 13,
                            fontWeight: 600,
                            color: '#e6edf3',
                            letterSpacing: '0.06em',
                            textTransform: 'uppercase',
                            margin: 0,
                        }}>
                            {meta.label}
                        </h1>
                        <p style={{ fontSize: 10, color: '#8b9eb0', marginTop: 2 }}>
                            {isLoading ? 'loading…' : isError ? 'error loading' : `${items.length} registered`}
                        </p>
                    </div>

                    <button
                        title="Refresh"
                        onClick={() => void refetch()}
                        style={{
                            background: 'none',
                            border: '1px solid #1e2d3d',
                            borderRadius: 4,
                            color: isFetching ? meta.accent : '#8b9eb0',
                            display: 'flex',
                            alignItems: 'center',
                            gap: 5,
                            fontSize: 10,
                            fontFamily: 'inherit',
                            padding: '4px 10px',
                            cursor: 'pointer',
                            transition: 'border-color 0.15s, color 0.15s',
                        }}
                        onMouseEnter={e => {
                            e.currentTarget.style.borderColor = meta.accent
                            e.currentTarget.style.color = meta.accent
                        }}
                        onMouseLeave={e => {
                            if (!isFetching) {
                                e.currentTarget.style.borderColor = '#1e2d3d'
                                e.currentTarget.style.color = '#8b9eb0'
                            }
                        }}
                    >
                        <RefreshCw
                            size={11}
                            strokeWidth={2}
                            style={{ animation: isFetching ? 'spin 0.8s linear infinite' : 'none' }}
                        />
                        refresh
                    </button>
                </div>

                {/* Body */}
                <div style={{ flex: 1, overflowY: 'auto', padding: '10px 14px' }}>
                    {isLoading && <LoadingSkeleton />}

                    {isError && (
                        <div style={{ margin: '40px auto', textAlign: 'center', color: '#f85149', fontSize: 11 }}>
                            <div style={{ fontSize: 22, marginBottom: 8 }}>✗</div>
                            {(error as Error)?.message ?? 'Failed to load'}
                        </div>
                    )}

                    {!isLoading && !isError && items.length === 0 && (
                        <EmptyState label={meta.label} Icon={meta.Icon} accent={meta.accent} />
                    )}

                    {!isLoading && !isError && items.length > 0 && (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                            {items.map(item => (
                                <ConfigRow
                                    key={item.id || item.name}
                                    item={item}
                                    accent={meta.accent}
                                    selected={selectedId === (item.id || item.name)}
                                    onClick={() => {
                                        const key = item.id || item.name
                                        setSelectedId(selectedId === key ? null : key)
                                    }}
                                />
                            ))}
                        </div>
                    )}
                </div>
            </div>

            {/* ── Detail panel ── */}
            {selectedId && (
                <ConfigDetail
                    resource={resource}
                    id={selectedId}
                    getFn={meta.getFn}
                    accent={meta.accent}
                    onClose={() => setSelectedId(null)}
                />
            )}

            <style>{`@keyframes spin { to { transform: rotate(360deg) } }`}</style>
        </div>
    )
}

// ─── Row ─────────────────────────────────────────────────────────────────────

function ConfigRow({
    item,
    accent,
    selected,
    onClick,
}: {
    item: ConfigSummary
    accent: string
    selected: boolean
    onClick: () => void
}) {
    return (
        <button
            onClick={onClick}
            style={{
                display: 'flex',
                alignItems: 'center',
                gap: 12,
                width: '100%',
                background: selected ? `${accent}0d` : 'transparent',
                border: `1px solid ${selected ? accent : '#1e2d3d'}`,
                borderRadius: 6,
                padding: '10px 14px',
                cursor: 'pointer',
                textAlign: 'left',
                transition: 'background 0.15s, border-color 0.15s',
                fontFamily: 'inherit',
            }}
            onMouseEnter={e => {
                if (!selected) {
                    e.currentTarget.style.borderColor = `${accent}55`
                    e.currentTarget.style.background = `${accent}07`
                }
            }}
            onMouseLeave={e => {
                if (!selected) {
                    e.currentTarget.style.borderColor = '#1e2d3d'
                    e.currentTarget.style.background = 'transparent'
                }
            }}
        >
            {/* Status dot */}
            <span style={{
                width: 6,
                height: 6,
                borderRadius: '50%',
                background: item.enabled ? accent : '#3d444d',
                boxShadow: item.enabled ? `0 0 5px ${accent}` : 'none',
                flexShrink: 0,
                transition: 'background 0.2s',
            }} />

            {/* Content */}
            <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <span style={{
                        fontSize: 12,
                        color: selected ? accent : '#cdd9e5',
                        fontWeight: 500,
                        letterSpacing: '0.03em',
                        whiteSpace: 'nowrap',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        transition: 'color 0.15s',
                    }}>
                        {item.name}
                    </span>
                    <EnabledBadge enabled={item.enabled} accent={accent} />
                </div>
                {item.description && (
                    <p style={{
                        fontSize: 10,
                        color: '#8b9eb0',
                        marginTop: 3,
                        lineHeight: 1.5,
                        overflow: 'hidden',
                        display: '-webkit-box',
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: 'vertical',
                    }}>
                        {item.description}
                    </p>
                )}
            </div>

            <ChevronRight
                size={13}
                strokeWidth={1.5}
                color={selected ? accent : '#3d444d'}
                style={{ flexShrink: 0, transition: 'color 0.15s' }}
            />
        </button>
    )
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function EnabledBadge({ enabled, accent }: { enabled: boolean; accent: string }) {
    return (
        <span style={{
            fontSize: 9,
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
            padding: '2px 6px',
            borderRadius: 3,
            border: `1px solid ${enabled ? `${accent}55` : '#3d444d'}`,
            color: enabled ? accent : '#8b9eb0',
            background: enabled ? `${accent}10` : 'transparent',
            flexShrink: 0,
            fontFamily: 'inherit',
        }}>
            {enabled ? 'on' : 'off'}
        </span>
    )
}

function LoadingSkeleton() {
    return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {[0.9, 0.75, 0.6, 0.45].map((opacity, i) => (
                <div key={i} style={{
                    height: 58,
                    borderRadius: 6,
                    border: '1px solid #1e2d3d',
                    background: '#0d1117',
                    animation: 'pulse 1.4s ease-in-out infinite',
                    opacity,
                }} />
            ))}
            <style>{`@keyframes pulse { 0%,100%{opacity:0.5} 50%{opacity:1} }`}</style>
        </div>
    )
}

function EmptyState({
    label,
    Icon,
    accent,
}: {
    label: string
    Icon: React.ComponentType<{ size?: number; strokeWidth?: number; color?: string }>
    accent: string
}) {
    return (
        <div style={{ margin: '60px auto', textAlign: 'center', color: '#8b9eb0', fontSize: 11 }}>
            <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 12, opacity: 0.3 }}>
                <Icon size={32} strokeWidth={1} color={accent} />
            </div>
            <div style={{ color: accent, opacity: 0.7, marginBottom: 4 }}>no {label.toLowerCase()} found</div>
            <div style={{ fontSize: 10, opacity: 0.5 }}>add config files to register {label.toLowerCase()}</div>
        </div>
    )
}

import type React from 'react'
