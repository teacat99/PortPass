import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'

// useThemeStore controls the dark/light/auto colour scheme. The user's
// explicit choice is persisted to localStorage; "auto" follows the system
// preference and reacts live to OS-level changes (prefers-color-scheme).
//
// We intentionally avoid Arco's dark-mode helper because we want the same
// CSS variable swap to drive both Arco components AND our own custom CSS.
// Toggling the `arco-theme="dark"` attribute on <body> achieves both.

export type ThemeMode = 'light' | 'dark' | 'auto'

const STORAGE_KEY = 'portpass.theme'

function readPersisted(): ThemeMode {
  const v = localStorage.getItem(STORAGE_KEY)
  if (v === 'light' || v === 'dark' || v === 'auto') return v
  return 'auto'
}

function systemPrefersDark(): boolean {
  return typeof window !== 'undefined'
    && window.matchMedia
    && window.matchMedia('(prefers-color-scheme: dark)').matches
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(readPersisted())
  const systemDark = ref<boolean>(systemPrefersDark())

  // Actual rendered scheme after resolving "auto".
  const isDark = computed<boolean>(() =>
    mode.value === 'dark' || (mode.value === 'auto' && systemDark.value)
  )

  function apply() {
    const body = document.body
    if (isDark.value) {
      body.setAttribute('arco-theme', 'dark')
      body.classList.add('pp-dark')
    } else {
      body.removeAttribute('arco-theme')
      body.classList.remove('pp-dark')
    }
    // Drive the browser chrome colour (status bar / address bar) too.
    const meta = document.querySelector('meta[name="theme-color"]')
    if (meta) meta.setAttribute('content', isDark.value ? '#0f1216' : '#165dff')
  }

  function setMode(next: ThemeMode) {
    mode.value = next
    localStorage.setItem(STORAGE_KEY, next)
  }

  function toggle() {
    // Two-state toggle ignores "auto" and snaps to the opposite of what
    // is currently rendered.
    setMode(isDark.value ? 'light' : 'dark')
  }

  function init() {
    if (typeof window !== 'undefined' && window.matchMedia) {
      const mql = window.matchMedia('(prefers-color-scheme: dark)')
      const handler = (e: MediaQueryListEvent) => { systemDark.value = e.matches }
      try { mql.addEventListener('change', handler) }
      catch { mql.addListener(handler) /* Safari < 14 */ }
    }
    apply()
  }

  watch(isDark, () => apply())

  return { mode, isDark, setMode, toggle, init }
})
