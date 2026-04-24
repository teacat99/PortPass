// Frontend mirror of internal/portset/portset.go — keeps validation and
// canonicalisation identical between the form input and the backend so
// users see the same errors / canonical strings on both sides.

export const MIN_PORT = 1
export const MAX_PORT = 65535
export const MAX_ENTRIES = 15

export interface PortRange {
  from: number
  to: number
}

export interface ParseOk {
  ok: true
  ranges: PortRange[]
  canonical: string
  count: number
  entries: number
}

export interface ParseErr {
  ok: false
  error: string
}

export type ParseResult = ParseOk | ParseErr

function parsePort(raw: string): number | string {
  const s = raw.trim()
  if (!/^\d+$/.test(s)) {
    return `无效端口号 "${s}"`
  }
  const n = parseInt(s, 10)
  if (n < MIN_PORT || n > MAX_PORT) {
    return `端口 ${n} 超出范围 ${MIN_PORT}-${MAX_PORT}`
  }
  return n
}

function parseOne(part: string): PortRange | string {
  if (part.includes('-')) {
    const [a, b] = part.split('-', 2)
    const from = parsePort(a)
    if (typeof from === 'string') return from
    const to = parsePort(b)
    if (typeof to === 'string') return to
    if (from > to) return `无效区间 "${part}" (起始大于结束)`
    return { from, to }
  }
  const p = parsePort(part)
  if (typeof p === 'string') return p
  return { from: p, to: p }
}

function canonicalise(ranges: PortRange[]): PortRange[] | string {
  const sorted = [...ranges].sort((a, b) =>
    a.from !== b.from ? a.from - b.from : a.to - b.to
  )
  const merged: PortRange[] = []
  for (const r of sorted) {
    if (merged.length === 0) {
      merged.push({ ...r })
      continue
    }
    const last = merged[merged.length - 1]
    if (r.from <= last.to + 1) {
      if (r.to > last.to) last.to = r.to
    } else {
      merged.push({ ...r })
    }
  }
  if (merged.length > MAX_ENTRIES) {
    return `端口段过多（${merged.length}）, 最多 ${MAX_ENTRIES} 段`
  }
  return merged
}

export function parsePortSet(input: string): ParseResult {
  const trimmed = (input ?? '').trim()
  if (!trimmed) {
    return { ok: true, ranges: [], canonical: '', count: 0, entries: 0 }
  }
  const pieces = trimmed.split(',').map(p => p.trim()).filter(Boolean)
  if (pieces.length === 0) {
    return { ok: false, error: '端口列表为空' }
  }
  const ranges: PortRange[] = []
  for (const p of pieces) {
    const r = parseOne(p)
    if (typeof r === 'string') return { ok: false, error: r }
    ranges.push(r)
  }
  const canon = canonicalise(ranges)
  if (typeof canon === 'string') return { ok: false, error: canon }
  return {
    ok: true,
    ranges: canon,
    canonical: formatRanges(canon),
    count: canon.reduce((acc, r) => acc + (r.to - r.from + 1), 0),
    entries: canon.length
  }
}

export function formatRanges(ranges: PortRange[]): string {
  return ranges
    .map(r => (r.from === r.to ? String(r.from) : `${r.from}-${r.to}`))
    .join(',')
}

// Returns true when `outer` fully contains every port in `inner`. Used
// to display "disabled" styling on preset chips that fall outside the
// current user's allowed ranges.
export function containsSet(outer: PortRange[], inner: PortRange[]): boolean {
  for (const r of inner) {
    let covered = false
    for (const x of outer) {
      if (x.from <= r.from && r.to <= x.to) {
        covered = true
        break
      }
    }
    if (!covered) return false
  }
  return true
}

// Returns true when any port is shared between the two sets.
export function overlaps(a: PortRange[], b: PortRange[]): boolean {
  for (const x of a) {
    for (const y of b) {
      if (x.from <= y.to && y.from <= x.to) return true
    }
  }
  return false
}

// Returns the first port of the set (0 if empty). Used to feed the
// legacy `port` field on request payloads during the transition period.
export function firstPort(ranges: PortRange[]): number {
  return ranges.length === 0 ? 0 : ranges[0].from
}

// Helper: format a single range for chip display.
export function formatRange(r: PortRange): string {
  return r.from === r.to ? String(r.from) : `${r.from}-${r.to}`
}
