import type { PresetPort } from '@/api/types'

// Light heuristic mapping from a preset's name/port to a category. Used by
// the Home page to render preset chips into nicely-grouped cards with an
// icon glyph.
//
// We deliberately keep this client-side and tolerant: if nothing matches,
// the preset falls into the "Misc" group with a generic icon.

export interface PresetCategory {
  /** i18n key under "preset.cat.*" */
  key: string
  /** Emoji glyph rendered as the chip icon. Browser-native, no asset cost. */
  icon: string
  match: (p: PresetPort) => boolean
}

export const CATEGORIES: PresetCategory[] = [
  {
    key: 'remote',
    icon: '🔐',
    match: (p) => /ssh|rdp|vnc|telnet/i.test(p.name) || p.port === 22 || p.port === 3389 || p.port === 5900
  },
  {
    key: 'web',
    icon: '🌐',
    match: (p) => /http|https|nginx|caddy|web/i.test(p.name) || p.port === 80 || p.port === 443 || p.port === 8080 || p.port === 8443
  },
  {
    key: 'db',
    icon: '🗄️',
    match: (p) =>
      /mysql|postgres|mongo|redis|mariadb|sqlserver|clickhouse|elastic/i.test(p.name)
      || [3306, 5432, 27017, 6379, 1433, 9200, 9000, 8123].includes(p.port)
  },
  {
    key: 'mq',
    icon: '📬',
    match: (p) => /kafka|rabbitmq|amqp|nats|mqtt|nsq/i.test(p.name) || [5672, 9092, 1883, 4222, 4150].includes(p.port)
  },
  {
    key: 'game',
    icon: '🎮',
    match: (p) => /game|minecraft|steam|valheim|terraria/i.test(p.name)
  }
]

export function categorize(p: PresetPort): { key: string; icon: string } {
  for (const c of CATEGORIES) {
    if (c.match(p)) return { key: c.key, icon: c.icon }
  }
  return { key: 'misc', icon: '🔌' }
}

export function groupPresets(presets: PresetPort[]) {
  const buckets = new Map<string, { key: string; icon: string; items: PresetPort[] }>()
  for (const p of presets) {
    const c = categorize(p)
    if (!buckets.has(c.key)) buckets.set(c.key, { key: c.key, icon: c.icon, items: [] })
    buckets.get(c.key)!.items.push(p)
  }
  // Stable ordering: remote, web, db, mq, game, misc
  const order = ['remote', 'web', 'db', 'mq', 'game', 'misc']
  return order
    .map((k) => buckets.get(k))
    .filter((b): b is { key: string; icon: string; items: PresetPort[] } => !!b)
    .map((b) => ({ ...b, items: b.items.sort((a, z) => a.sort - z.sort || a.port - z.port) }))
}
