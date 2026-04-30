export interface Rule {
  id: number
  user_id: number
  source_ip: string
  port: number
  ports: string
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
  notify_enabled: boolean
  notify_lead_seconds: number
  notify_sent_browser_at?: string
  notify_sent_ntfy_at?: string
}

export interface PresetPort {
  id: number
  name: string
  port: number
  ports: string
  protocol: string
  sort: number
  user_allowed: boolean
  max_duration_sec: number
  category_id?: number | null
}

// PresetCategory groups preset ports for display purposes. The backend
// seeds six built-in rows (remote/web/db/mq/game/misc) marked
// builtin=true; the UI prevents deletion of those rows but allows
// re-labelling and icon overrides. User-added rows have an empty key
// and builtin=false.
export interface PresetCategory {
  id: number
  // Built-in slug (remote/web/db/mq/game/misc) or empty for user-added.
  key: string
  // User-visible label. Empty on built-ins means "use i18n by key".
  label: string
  // Either an emoji glyph or an http(s):// image URL.
  icon: string
  sort: number
  builtin: boolean
  created_at?: string
  updated_at?: string
}

export interface ProtectedPort {
  id: number
  name: string
  ports: string
  protocol: string
  note: string
}

export interface UserAllowedRange {
  id: number
  user_id: number
  name: string
  ports: string
  protocol: string
  max_duration_sec: number
  note: string
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
  port?: number
  ports?: string
  protocol: string
  duration_sec?: number
  expire_at?: string
  note?: string
  notify_enabled?: boolean
}
