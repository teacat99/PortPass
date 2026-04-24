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
      {
        path: 'settings',
        name: 'settings',
        meta: { adminOnly: true },
        component: () => import('@/views/SettingsView.vue')
      },
      // Legacy `/users` URL kept as a redirect so bookmarks keep working
      // after the Users page became a tab inside Settings.
      { path: 'users', redirect: { name: 'settings' } }
    ]
  },
  { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue') }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// Router guard. Two concerns run here: redirecting unauthenticated users
// to /login (password mode only), and blocking non-admins from adminOnly
// routes even if they know the direct URL. The `me` object may not have
// loaded yet on a hard refresh so we lazy-fetch before deciding.
router.beforeEach(async (to, _from, next) => {
  const auth = useAuthStore()
  if (to.name !== 'login' && auth.required && !auth.token) {
    next({ name: 'login', query: { redirect: to.fullPath } })
    return
  }
  // Ensure we know the caller's role before evaluating adminOnly routes.
  if (to.name !== 'login' && auth.token && !auth.me) {
    await auth.fetchMe()
  }
  if (to.meta?.adminOnly && auth.me && auth.me.role !== 'admin') {
    next({ name: 'home' })
    return
  }
  next()
})

export default router
