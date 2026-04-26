import type { PresetCategory, PresetPort } from '@/api/types'

// Heuristic mapping from a preset's name/port to a built-in category
// key. Used as a fallback when a preset has no manual category_id, and
// when the home page needs to group presets that predate the manual
// taxonomy. The keys here mirror the seed rows the backend creates on
// first boot (see store.SeedPresetCategories).

export interface HeuristicMatch {
  key: string
  match: (p: PresetPort) => boolean
}

export const CATEGORY_HEURISTICS: HeuristicMatch[] = [
  {
    key: 'remote',
    match: (p) => /ssh|rdp|vnc|telnet/i.test(p.name) || p.port === 22 || p.port === 3389 || p.port === 5900
  },
  {
    key: 'web',
    match: (p) => /http|https|nginx|caddy|web/i.test(p.name) || p.port === 80 || p.port === 443 || p.port === 8080 || p.port === 8443
  },
  {
    key: 'db',
    match: (p) =>
      /mysql|postgres|mongo|redis|mariadb|sqlserver|clickhouse|elastic/i.test(p.name)
      || [3306, 5432, 27017, 6379, 1433, 9200, 9000, 8123].includes(p.port)
  },
  {
    key: 'mq',
    match: (p) => /kafka|rabbitmq|amqp|nats|mqtt|nsq/i.test(p.name) || [5672, 9092, 1883, 4222, 4150].includes(p.port)
  },
  {
    key: 'game',
    match: (p) => /game|minecraft|steam|valheim|terraria/i.test(p.name)
  }
]

// Built-in fallback ordering when grouping presets by category. User-
// added categories are appended after these in their stored sort order.
const BUILTIN_ORDER = ['remote', 'web', 'db', 'mq', 'game', 'misc']

// autoCategoryKey runs the heuristic and returns one of the built-in
// keys, defaulting to 'misc' when no rule fires. Pure function with no
// dependency on the categories list so callers can use it before the
// categories API resolves.
export function autoCategoryKey(p: PresetPort): string {
  for (const c of CATEGORY_HEURISTICS) {
    if (c.match(p)) return c.key
  }
  return 'misc'
}

// resolveCategory picks the PresetCategory row that should render a
// given preset. Manual category_id wins; otherwise we use the heuristic
// to find the matching built-in row by key. Returns null only when the
// categories list is empty (e.g. before initial fetch resolves).
export function resolveCategory(
  p: PresetPort,
  categories: PresetCategory[]
): PresetCategory | null {
  if (p.category_id) {
    const hit = categories.find((c) => c.id === p.category_id)
    if (hit) return hit
  }
  const key = autoCategoryKey(p)
  const byKey = categories.find((c) => c.key === key)
  if (byKey) return byKey
  return categories.find((c) => c.key === 'misc') ?? null
}

export interface PresetGroup {
  category: PresetCategory
  items: PresetPort[]
}

// groupPresetsBy buckets presets under their resolved category and
// returns the buckets in the canonical order: built-ins first (using
// BUILTIN_ORDER), then user-added rows ordered by the category's sort
// column. Empty buckets are dropped so the home page does not render
// orphan group headers.
export function groupPresetsBy(
  presets: PresetPort[],
  categories: PresetCategory[]
): PresetGroup[] {
  if (!categories.length) return []
  const buckets = new Map<number, PresetGroup>()
  for (const p of presets) {
    const cat = resolveCategory(p, categories)
    if (!cat) continue
    if (!buckets.has(cat.id)) buckets.set(cat.id, { category: cat, items: [] })
    buckets.get(cat.id)!.items.push(p)
  }
  const seen = new Set<number>()
  const out: PresetGroup[] = []
  for (const key of BUILTIN_ORDER) {
    const cat = categories.find((c) => c.builtin && c.key === key)
    if (!cat) continue
    const bucket = buckets.get(cat.id)
    if (bucket) {
      out.push(bucket)
      seen.add(cat.id)
    }
  }
  const userAdded = categories
    .filter((c) => !c.builtin)
    .sort((a, b) => a.sort - b.sort || a.id - b.id)
  for (const cat of userAdded) {
    const bucket = buckets.get(cat.id)
    if (bucket && !seen.has(cat.id)) {
      out.push(bucket)
      seen.add(cat.id)
    }
  }
  for (const g of out) {
    g.items.sort((a, z) => a.sort - z.sort || a.port - z.port)
  }
  return out
}

// Legacy compatibility: existing call sites that imported `categorize`
// or `groupPresets` (pre-categories migration) can keep working when
// the categories list has not loaded yet. They just get the heuristic
// result with built-in-style icons.
const LEGACY_ICONS: Record<string, string> = {
  remote: '🔐',
  web: '🌐',
  db: '🗄️',
  mq: '📬',
  game: '🎮',
  misc: '🔌'
}

export function categorize(p: PresetPort): { key: string; icon: string } {
  const key = autoCategoryKey(p)
  return { key, icon: LEGACY_ICONS[key] || '🔌' }
}

export function groupPresets(presets: PresetPort[]) {
  const buckets = new Map<string, { key: string; icon: string; items: PresetPort[] }>()
  for (const p of presets) {
    const c = categorize(p)
    if (!buckets.has(c.key)) buckets.set(c.key, { key: c.key, icon: c.icon, items: [] })
    buckets.get(c.key)!.items.push(p)
  }
  return BUILTIN_ORDER
    .map((k) => buckets.get(k))
    .filter((b): b is { key: string; icon: string; items: PresetPort[] } => !!b)
    .map((b) => ({ ...b, items: b.items.sort((a, z) => a.sort - z.sort || a.port - z.port) }))
}
