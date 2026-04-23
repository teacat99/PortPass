import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    component: () => import('@/layouts/AppLayout.vue'),
    children: [
      { path: '', name: 'home', component: () => import('@/views/HomeView.vue') },
      { path: 'rules', name: 'rules', component: () => import('@/views/RulesView.vue') },
      { path: 'history', name: 'history', component: () => import('@/views/HistoryView.vue') },
      { path: 'settings', name: 'settings', component: () => import('@/views/SettingsView.vue') }
    ]
  },
  { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue') }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// Minimal guard: when password auth is enforced and we have no token, divert
// everything except /login to the login screen. In IP-whitelist / none modes
// the backend enforces authorisation so the guard stays out of the way.
router.beforeEach((to, _from, next) => {
  const auth = useAuthStore()
  if (to.name !== 'login' && auth.required && !auth.token) {
    next({ name: 'login', query: { redirect: to.fullPath } })
    return
  }
  next()
})

export default router
