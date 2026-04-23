import client from './client'
import type { CreateRulePayload, PresetPort, Rule, SettingsBundle } from './types'

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

export async function getSettings() {
  const { data } = await client.get<SettingsBundle>('/settings')
  return data
}

export async function saveSettings(kv: Record<string, string>) {
  await client.put('/settings', kv)
}
