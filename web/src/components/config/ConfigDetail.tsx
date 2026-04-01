// src/components/config/ConfigDetail.tsx
// Generic detail panel — renders any config object as a formatted key/value view.
import type React from 'react'
import { useQuery } from '@tanstack/react-query'

interface Props {
    resource: string
    id: string
    getFn: (id: string) => Promise<unknown>
    accent: string
    onClose: () => void
}

export function ConfigDetail({ resource, id, getFn, accent, onClose }: Props) {
    const { data, isLoading, isError, error } = useQuery<Record<string, unknown>>({
        queryKey: [resource, id],
        queryFn: () => getFn(id) as Promise<Record<string, unknown>>,
        enabled: Boolean(id),
    })

    return (
        <div style={{
            width: 340,
            flexShrink: 0,
            borderLeft: `1px solid ${accent}33`,
            display: 'flex',
            flexDirection: 'column',
            background: '#0a0f14',
            overflow: 'hidden',
        }}>
            {/* Panel header */}
            <div style={{
                padding: '14px 16px',
                borderBottom: `1px solid ${accent}22`,
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                flexShrink: 0,
            }}>
                <span style={{ flex: 1, fontSize: 11, color: accent, letterSpacing: '0.08em' }}>
                    {id}
                </span>
                <button
                    onClick={onClose}
                    title="Close"
                    style={{
                        background: 'none',
                        border: 'none',
                        color: '#8b9eb0',
                        fontSize: 14,
                        cursor: 'pointer',
                        padding: '0 2px',
                        lineHeight: 1,
                        fontFamily: 'inherit',
                        transition: 'color 0.15s',
                    }}
                    onMouseEnter={e => (e.currentTarget.style.color = '#cdd9e5')}
                    onMouseLeave={e => (e.currentTarget.style.color = '#8b9eb0')}
                >
                    ✕
                </button>
            </div>

            {/* Panel body */}
            <div style={{ flex: 1, overflowY: 'auto', padding: '12px 16px' }}>
                {isLoading && (
                    <div style={{ color: '#8b9eb0', fontSize: 11, paddingTop: 20, textAlign: 'center' }}>
                        <span style={{ animation: 'blink 1.2s step-end infinite' }}>▋</span>
                        {' '}loading…
                        <style>{`@keyframes blink { 0%,100%{opacity:1} 50%{opacity:0} }`}</style>
                    </div>
                )}

                {isError && (
                    <div style={{ color: '#f85149', fontSize: 11, paddingTop: 20, textAlign: 'center' }}>
                        {(error as Error)?.message ?? 'Failed to load'}
                    </div>
                )}

                {!isLoading && !isError && data && (
                    <ConfigFields data={data} accent={accent} />
                )}
            </div>
        </div>
    )
}

// ─── Recursive field renderer ─────────────────────────────────────────────────

function ConfigFields({
    data,
    accent,
    depth = 0,
}: {
    data: Record<string, unknown>
    accent: string
    depth?: number
}) {
    const entries = Object.entries(data).filter(([, v]) => v !== undefined && v !== null)

    return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {entries.map(([key, value]) => (
                <FieldRow key={key} fieldKey={key} value={value} accent={accent} depth={depth} />
            ))}
        </div>
    )
}

function FieldRow({
    fieldKey,
    value,
    accent,
    depth,
}: {
    fieldKey: string
    value: unknown
    accent: string
    depth: number
}) {
    const label = fieldKey.replace(/_/g, ' ')
    const indent = depth * 12

    // ── Object / array → recurse ──────────────────────────────────────────
    if (typeof value === 'object' && value !== null) {
        const isArr = Array.isArray(value)
        const entries: [string, unknown][] = isArr
            ? (value as unknown[]).map((v, i) => [String(i), v])
            : Object.entries(value as Record<string, unknown>)

        return (
            <div style={{ marginLeft: indent }}>
                <div style={{
                    fontSize: 9,
                    color: '#8b9eb0',
                    letterSpacing: '0.1em',
                    textTransform: 'uppercase',
                    padding: '8px 0 4px',
                    borderBottom: '1px solid #1e2d3d',
                    marginBottom: 4,
                }}>
                    {label}{isArr && <span style={{ color: accent, opacity: 0.6 }}> [ {entries.length} ]</span>}
                </div>
                {entries.map(([k, v]) => (
                    <FieldRow
                        key={k}
                        fieldKey={isArr ? ((v as { name?: string })?.name ?? k) : k}
                        value={v}
                        accent={accent}
                        depth={depth + 1}
                    />
                ))}
            </div>
        )
    }

    // ── Boolean ───────────────────────────────────────────────────────────
    if (typeof value === 'boolean') {
        return (
            <div style={rowStyle(indent)}>
                <span style={keyStyle}>{label}</span>
                <span style={{
                    fontSize: 10,
                    color: value ? accent : '#8b9eb0',
                    fontFamily: 'inherit',
                }}>
                    {value ? '✓ true' : '✗ false'}
                </span>
            </div>
        )
    }

    // ── Primitive ─────────────────────────────────────────────────────────
    const strVal = String(value)
    return (
        <div style={rowStyle(indent)}>
            <span style={keyStyle}>{label}</span>
            <span style={{
                fontSize: 11,
                color: '#cdd9e5',
                wordBreak: 'break-word',
                lineHeight: 1.5,
                flex: 1,
                textAlign: 'right',
                fontFamily: 'inherit',
            }}>
                {strVal}
            </span>
        </div>
    )
}

const rowStyle = (indent: number): React.CSSProperties => ({
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'flex-start',
    gap: 12,
    padding: '5px 0',
    borderBottom: '1px solid #1a2333',
    marginLeft: indent,
})

const keyStyle: React.CSSProperties = {
    fontSize: 10,
    color: '#8b9eb0',
    letterSpacing: '0.05em',
    flexShrink: 0,
    maxWidth: '45%',
    textTransform: 'lowercase',
}
