export interface Rule {
  id: number
  user_id: number
  source_ip: string
  port: number
  protocol: string
  note: string
  status: string
  expire_at: string
  created_by: string
  created_ip: string
  created_at: string
  terminated_at?: string
  driver_name: string
  driver_ref: string
  comment_tag: string
}

export interface PresetPort {
  id: number
  name: string
  port: number
  protocol: string
  sort: number
  user_allowed: boolean
  max_duration_sec: number
}

export type Role = 'admin' | 'user'

export interface User {
  id: number
  username: string
  role: Role
  disabled: boolean
  created_at: string
  updated_at: string
}

export interface Me {
  id: number
  username: string
  role: Role
  auth_mode: 'password' | 'ipwhitelist' | 'none'
}

export interface Setting {
  key: string
  value: string
  updated_at: string
}

export interface SettingsBundle {
  auth_mode: 'password' | 'ipwhitelist' | 'none'
  max_duration_hours: number
  history_retention_days: number
  firewall_driver: string
  trusted_proxies: string[]
  kv: Setting[]
}

export interface CreateRulePayload {
  source_ip?: string
  use_client_ip?: boolean
  port: number
  protocol: string
  duration_sec?: number
  expire_at?: string
  note?: string
}
