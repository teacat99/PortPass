import { defineStore } from 'pinia'
import { ref } from 'vue'
import { authStatus, login as apiLogin } from '@/api/auth'

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('portpass.token') ?? '')
  const mode = ref<string>('password')
  const required = ref<boolean>(true)

  async function refreshStatus() {
    try {
      const s = await authStatus()
      mode.value = s.mode
      required.value = s.required
    } catch {
      // Leave defaults; unauthenticated users will be redirected by guard.
    }
  }

  async function login(password: string) {
    const { token: t } = await apiLogin(password)
    token.value = t
    localStorage.setItem('portpass.token', t)
  }

  function logout() {
    token.value = ''
    localStorage.removeItem('portpass.token')
  }

  return { token, mode, required, refreshStatus, login, logout }
})
