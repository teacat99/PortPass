import client from './client'
import type {
  CreateRulePayload,
  PresetCategory,
  PresetPort,
  Rule,
  SettingsBundle
} from './types'

export async function fetchClientIP() {
  const { data } = await client.get<{ ip: string }>('/client-ip')
  return data.ip
}

export async function listRules(params?: { status?: string; limit?: number }) {
  const { data } = await client.get<{ rules: Rule[]; total: number }>('/rules', { params })
  return data
}

export async function createRule(payload: CreateRulePayload) {
  const { data } = await client.post<Rule>('/rules', payload)
  return data
}

export async function terminateRule(id: number) {
  const { data } = await client.post<Rule>(`/rules/${id}/terminate`)
  return data
}

export async function extendRule(id: number, durationSec: number) {
  const { data } = await client.post<Rule>(`/rules/${id}/extend`, { duration_sec: durationSec })
  return data
}

export async function duplicateRule(id: number) {
  const { data } = await client.post<Rule>(`/rules/${id}/duplicate`)
  return data
}

// setRuleNotify toggles expiry-notification on an already-created rule.
// Re-enabling re-snapshots the lead time from current settings and
// resets both per-channel sent_at marks (parallels the Extend flow).
export async function setRuleNotify(id: number, enabled: boolean) {
  const { data } = await client.post<Rule>(`/rules/${id}/notify`, { enabled })
  return data
}

export interface HistoryQuery {
  status?: string
  port?: number
  ip?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export async function listHistory(q: HistoryQuery) {
  const { data } = await client.get<{ rules: Rule[]; total: number }>('/history', { params: q })
  return data
}

export async function listPresets() {
  const { data } = await client.get<PresetPort[]>('/preset-ports')
  return data
}

export async function upsertPreset(p: Partial<PresetPort>) {
  const { data } = await client.post<PresetPort>('/preset-ports', p)
  return data
}

export async function deletePreset(id: number) {
  await client.delete(`/preset-ports/${id}`)
}

export async function listPresetCategories() {
  const { data } = await client.get<PresetCategory[]>('/preset-categories')
  return data
}

export async function upsertPresetCategory(c: Partial<PresetCategory>) {
  const { data } = await client.post<PresetCategory>('/preset-categories', c)
  return data
}

export async function deletePresetCategory(id: number) {
  await client.delete(`/preset-categories/${id}`)
}

export async function getSettings() {
  const { data } = await client.get<SettingsBundle>('/settings')
  return data
}

export async function saveSettings(kv: Record<string, string>) {
  await client.put('/settings', kv)
}

// fetchPendingNotifications returns the caller's own rules that are
// inside their pre-expiry notification window and not yet acked. The
// browser polls this every ~30s and pops a Notification per entry.
// The backend already filters by channel selector (returns empty when
// the operator picked ntfy-only).
export async function fetchPendingNotifications() {
  const { data } = await client.get<{ rules: Rule[] }>('/notify/pending')
  return data.rules
}

export interface NotifySettings {
  lead_minutes: number
  channels: 'browser' | 'ntfy' | 'both'
  default_enabled: boolean
}

// fetchNotifySettings exposes the three notify knobs every authenticated
// user can read. The full /runtime-settings is admin-only; this slice
// is what HomeView / RulesView / the polling loop need to render their
// UI and decide whether to call requestPermission().
export async function fetchNotifySettings() {
  const { data } = await client.get<NotifySettings>('/notify/settings')
  return data
}

// ackNotifications stamps notify_sent_browser_at on the listed rule
// IDs so the watcher won't return them again until Extend resets the
// flag. Errors are non-fatal: a missed ack just means the same rule
// may pop one more time on the next poll.
export async function ackNotifications(ruleIds: number[]) {
  if (ruleIds.length === 0) return 0
  const { data } = await client.post<{ updated: number }>('/notify/ack', {
    rule_ids: ruleIds
  })
  return data.updated
}
