// Maps environment variable names to human-readable descriptions in both
// Chinese and English. Rendered on the Settings > "运行时参数" tab so
// operators understand what each variable controls without leaving the UI.
export interface EnvHint {
  zh: string
  en: string
}

export const ENV_META: Record<string, EnvHint> = {
  PORTPASS_LISTEN: {
    zh: '监听地址（格式 :8080 或 0.0.0.0:8080）',
    en: 'Listen address (e.g. :8080 or 0.0.0.0:8080)'
  },
  PORTPASS_DATA_DIR: {
    zh: 'SQLite 数据目录，需挂载持久化卷',
    en: 'SQLite data directory; mount a persistent volume'
  },
  PORTPASS_FIREWALL_DRIVER: {
    zh: '防火墙驱动：iptables / nftables / ufw / firewalld / mock',
    en: 'Firewall backend: iptables / nftables / ufw / firewalld / mock'
  },
  PORTPASS_MAX_DURATION_HOURS: {
    zh: '单条规则最大持续时长（小时），兜底上限',
    en: 'Global max duration per rule (hours), hard cap'
  },
  PORTPASS_HISTORY_RETENTION_DAYS: {
    zh: '历史记录保留天数，到期自动清理',
    en: 'History retention window (days), auto-purged'
  },
  PORTPASS_MAX_RULES_PER_IP: {
    zh: '单个 (用户,客户端 IP) 并发规则数上限',
    en: 'Max concurrent rules per (user, client-IP) pair'
  },
  PORTPASS_RATE_LIMIT_PER_MINUTE_PER_IP: {
    zh: '每 IP 每分钟的写操作频控上限',
    en: 'Write-ops rate cap per minute per client IP'
  },
  PORTPASS_AUTH_MODE: {
    zh: '鉴权模式：password / ipwhitelist / none',
    en: 'Auth mode: password / ipwhitelist / none'
  },
  PORTPASS_TRUSTED_PROXIES: {
    zh: '可信反代 CIDR 列表，用于解析真实客户端 IP',
    en: 'Trusted proxy CIDRs used to recover the real client IP'
  },
  PORTPASS_ADMIN_USERNAME: {
    zh: '首次启动播种的管理员用户名（落库后可忽略）',
    en: 'Seeded admin username on first boot (ignored afterwards)'
  },
  PORTPASS_ADMIN_PASSWORD: {
    zh: '首次启动播种的管理员密码（不传则默认 passwd）',
    en: 'Seeded admin password on first boot (defaults to "passwd")'
  },
  PORTPASS_JWT_SECRET: {
    zh: 'JWT 签名密钥，部署时请自行生成 32+ 字节',
    en: 'JWT signing secret; set to ≥32 random bytes in production'
  },
  PORTPASS_IP_WHITELIST: {
    zh: 'ipwhitelist 模式下允许直通的 CIDR 列表',
    en: 'CIDRs allowed to bypass password in ipwhitelist mode'
  }
}

export function envHint(key: string, locale: 'zh-CN' | 'en-US'): string {
  const meta = ENV_META[key]
  if (!meta) return ''
  return locale === 'zh-CN' ? meta.zh : meta.en
}
