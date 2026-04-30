import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  ackNotifications,
  fetchNotifySettings,
  fetchPendingNotifications,
  type NotifySettings
} from '@/api/rules'
import type { Rule } from '@/api/types'
import i18n from '@/i18n'

// useNotifyStore owns the global expiry-reminder pipeline:
//   1. Caches the three notify settings (lead minutes / channels /
//      default enabled) so HomeView can render the bell with the right
//      defaults without re-fetching on every navigation.
//   2. Drives a 30-second polling loop against /api/notify/pending,
//      pops a browser Notification per result, then ACKs back to the
//      server so the same rule won't fire again until Extend resets
//      the flag.
//
// The polling is opt-in via startPolling() so non-authenticated views
// (login page) don't waste a request. The interval is hard-coded at
// 30s to match the backend reconcile cadence; smaller values would
// burn server CPU without buying timing precision because the lead
// time is configured in minutes.
export const useNotifyStore = defineStore('notify', () => {
  const settings = ref<NotifySettings | null>(null)
  let pollTimer: number | null = null

  async function loadSettings() {
    try {
      settings.value = await fetchNotifySettings()
    } catch {
      settings.value = null
    }
  }

  function startPolling(intervalMs = 30000) {
    stopPolling()
    pollTimer = window.setInterval(() => {
      void tick()
    }, intervalMs)
    void tick()
  }

  function stopPolling() {
    if (pollTimer != null) {
      window.clearInterval(pollTimer)
      pollTimer = null
    }
  }

  // tick is exported so callers (HomeView after creating a rule) can
  // run a one-shot poll without waiting for the next interval. It is
  // safe to invoke concurrently — the backend ack is idempotent for
  // already-stamped rules.
  async function tick() {
    if (typeof Notification === 'undefined') return
    if (Notification.permission !== 'granted') return
    if (settings.value == null) return
    if (settings.value.channels !== 'browser' && settings.value.channels !== 'both') return
    let rules: Rule[]
    try {
      rules = await fetchPendingNotifications()
    } catch {
      return
    }
    if (rules.length === 0) return
    const shown: number[] = []
    for (const r of rules) {
      try {
        showOne(r)
        shown.push(r.id)
      } catch {
        // Ignore individual show failures (browser may throw on tab
        // backgrounded etc.); the ack list will just be a subset.
      }
    }
    if (shown.length > 0) {
      try {
        await ackNotifications(shown)
      } catch {
        // Non-fatal: the same rule may pop one extra time on the next
        // poll, but we never want a transient HTTP error to crash the
        // whole watcher.
      }
    }
  }

  function showOne(rule: Rule) {
    const t = i18n.global.t
    const remainMs = new Date(rule.expire_at).getTime() - Date.now()
    const remainSec = Math.max(0, Math.round(remainMs / 1000))
    const mins = Math.floor(remainSec / 60)
    const secs = remainSec % 60
    const remaining = mins > 0
      ? t('notify.expiryRemainingMin', { m: mins, s: String(secs).padStart(2, '0') })
      : t('notify.expiryRemainingSec', { s: secs })
    const ports = rule.ports || String(rule.port || '')
    const body = t('notify.expiryBody', {
      source: rule.source_ip,
      ports,
      protocol: rule.protocol,
      remaining
    })
    new Notification(t('notify.expiryTitle'), {
      body,
      tag: `portpass-rule-${rule.id}`,
      icon: '/icons/icon-192.png',
      badge: '/icons/icon-192.png'
    })
  }

  return {
    settings,
    loadSettings,
    startPolling,
    stopPolling,
    tick
  }
})
