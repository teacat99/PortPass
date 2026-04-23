import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { authStatus, getMe, login as apiLogin } from '@/api/auth'
import type { Me, Role } from '@/api/types'

// useAuthStore centralises everything the UI needs about the current
// session: the mode enforced by the backend, the bearer token, and the
// authenticated principal (id / username / role). Consumers should treat
// the `me` ref as the single source of truth for role-based gating.
export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('portpass.token') ?? '')
  const mode = ref<string>('password')
  const required = ref<boolean>(true)
  const me = ref<Me | null>(null)

  const isAdmin = computed<boolean>(() => me.value?.role === 'admin')
  const role = computed<Role | null>(() => me.value?.role ?? null)

  async function refreshStatus() {
    try {
      const s = await authStatus()
      mode.value = s.mode
      required.value = s.required
    } catch {
      // Leave defaults; unauthenticated users will be redirected by guard.
    }
  }

  async function fetchMe() {
    try {
      me.value = await getMe()
    } catch {
      me.value = null
    }
  }

  async function login(username: string, password: string) {
    const resp = await apiLogin(username, password)
    token.value = resp.token
    localStorage.setItem('portpass.token', resp.token)
    me.value = {
      id: 0, // real id comes from /auth/me; login response has no id yet
      username: resp.username,
      role: resp.role,
      auth_mode: 'password'
    }
    await fetchMe()
  }

  function logout() {
    token.value = ''
    me.value = null
    localStorage.removeItem('portpass.token')
  }

  return { token, mode, required, me, isAdmin, role, refreshStatus, fetchMe, login, logout }
})
