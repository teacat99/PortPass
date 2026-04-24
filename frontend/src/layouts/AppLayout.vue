<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  IconHome,
  IconList,
  IconHistory,
  IconSettings,
  IconLanguage,
  IconExport,
  IconMenu,
  IconUser,
  IconUserGroup,
  IconLock,
  IconMoon,
  IconSun
} from '@arco-design/web-vue/es/icon'
import { Message } from '@arco-design/web-vue'
import { setLocale } from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { useBreakpoint } from '@/composables/useBreakpoint'
import { changeOwnPassword } from '@/api/auth'
import logoUrl from '@/assets/logo.svg'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const auth = useAuthStore()
const theme = useThemeStore()
const { isMobile } = useBreakpoint()

const drawerVisible = ref(false)

// Change-password modal state — surfaced in the user dropdown.
const pwdModal = ref(false)
const pwdSubmitting = ref(false)
const pwdForm = ref({ old_password: '', new_password: '', confirm: '' })

onMounted(async () => {
  await auth.refreshStatus()
  if (auth.token || !auth.required) {
    await auth.fetchMe()
  }
})

const selectedKeys = computed(() => [String(route.name ?? 'home')])
const isAdmin = computed(() => auth.isAdmin)
const canChangePassword = computed(
  () => auth.required && auth.mode === 'password' && !!auth.me
)

const navItems = computed(() => {
  const base = [
    { key: 'home',    label: t('menu.home'),    icon: IconHome },
    { key: 'rules',   label: t('menu.rules'),   icon: IconList },
    { key: 'history', label: t('menu.history'), icon: IconHistory }
  ]
  if (isAdmin.value) {
    base.push({ key: 'users',    label: t('menu.users'),    icon: IconUserGroup })
    base.push({ key: 'settings', label: t('menu.settings'), icon: IconSettings })
  }
  return base
})

function navigate(key: string) {
  router.push({ name: key })
  drawerVisible.value = false
}

function toggleLocale() {
  const next = locale.value === 'zh-CN' ? 'en-US' : 'zh-CN'
  setLocale(next as 'zh-CN' | 'en-US')
}

function logout() {
  auth.logout()
  router.push({ name: 'login' })
}

function openPasswordModal() {
  pwdForm.value = { old_password: '', new_password: '', confirm: '' }
  pwdModal.value = true
}

// Crude but effective password strength: length × character-class mix.
const pwdStrength = computed(() => {
  const v = pwdForm.value.new_password
  if (!v) return { score: 0, label: '', tone: 'gray' as const }
  let score = 0
  if (v.length >= 6) score++
  if (v.length >= 10) score++
  if (/[A-Z]/.test(v) && /[a-z]/.test(v)) score++
  if (/\d/.test(v)) score++
  if (/[^A-Za-z0-9]/.test(v)) score++
  const map = ['', '太弱', '一般', '中等', '不错', '很强']
  const tones: Array<'red' | 'orange' | 'gold' | 'green'> = ['red', 'red', 'orange', 'gold', 'green']
  return { score, label: map[score] || '', tone: tones[Math.min(4, Math.max(0, score - 1))] }
})

async function submitPassword() {
  if (pwdForm.value.new_password.length < 6) {
    Message.warning(t('password.too_short'))
    return false
  }
  if (pwdForm.value.new_password !== pwdForm.value.confirm) {
    Message.warning(t('password.mismatch'))
    return false
  }
  pwdSubmitting.value = true
  try {
    await changeOwnPassword(pwdForm.value.old_password, pwdForm.value.new_password)
    Message.success(t('password.changed'))
    pwdModal.value = false
    return true
  } catch {
    return false
  } finally {
    pwdSubmitting.value = false
  }
}
</script>

<template>
  <a-layout class="app-layout">
    <a-layout-header class="app-header">
      <div class="header-left">
        <a-button v-if="isMobile" type="text" shape="circle" @click="drawerVisible = true">
          <template #icon><IconMenu /></template>
        </a-button>
        <div class="logo">
          <img :src="logoUrl" alt="PortPass" class="logo-img" />
          <div class="logo-text">
            <div class="logo-title">{{ t('app.title') }}</div>
            <div class="logo-sub">{{ t('app.subtitle') }}</div>
          </div>
        </div>
      </div>
      <div class="header-right">
        <a-tooltip :content="theme.isDark ? '切换到浅色' : '切换到深色'" position="bottom">
          <a-button type="text" shape="circle" @click="theme.toggle()">
            <template #icon>
              <IconSun v-if="theme.isDark" />
              <IconMoon v-else />
            </template>
          </a-button>
        </a-tooltip>
        <a-tooltip :content="locale === 'zh-CN' ? 'Switch to English' : '切换到中文'" position="bottom">
          <a-button type="text" shape="circle" @click="toggleLocale">
            <template #icon><IconLanguage /></template>
          </a-button>
        </a-tooltip>
        <a-dropdown v-if="auth.me" trigger="click" position="br">
          <a-button type="text" shape="round" class="user-btn">
            <template #icon><IconUser /></template>
            <span class="user-name">{{ auth.me.username }}</span>
            <a-tag v-if="isAdmin" color="arcoblue" size="small" class="role-tag">admin</a-tag>
          </a-button>
          <template #content>
            <a-doption v-if="canChangePassword" @click="openPasswordModal">
              <template #icon><IconLock /></template>
              {{ t('action.change_password') }}
            </a-doption>
            <a-doption v-if="auth.required && auth.token" @click="logout">
              <template #icon><IconExport /></template>
              {{ t('action.logout') }}
            </a-doption>
          </template>
        </a-dropdown>
        <a-button
          v-else-if="auth.required && auth.token"
          type="text"
          shape="round"
          status="danger"
          @click="logout"
        >
          <template #icon><IconExport /></template>
          {{ t('action.logout') }}
        </a-button>
      </div>
    </a-layout-header>

    <a-layout class="app-body">
      <a-layout-sider v-if="!isMobile" :width="216" class="app-sider" breakpoint="lg">
        <nav class="side-nav">
          <button
            v-for="n in navItems"
            :key="n.key"
            type="button"
            class="side-nav-item"
            :class="{ active: selectedKeys[0] === n.key }"
            @click="navigate(n.key)"
          >
            <component :is="n.icon" class="side-nav-icon" />
            <span class="side-nav-label">{{ n.label }}</span>
          </button>
        </nav>
        <div class="side-foot">
          <span class="pp-muted">v0.1 · {{ auth.mode }}</span>
        </div>
      </a-layout-sider>

      <a-drawer
        v-model:visible="drawerVisible"
        placement="left"
        :width="276"
        :footer="false"
        :header="false"
        unmount-on-close
      >
        <div class="drawer-head">
          <img :src="logoUrl" alt="" class="logo-img" />
          <div>
            <div class="logo-title">{{ t('app.title') }}</div>
            <div class="logo-sub">{{ auth.me?.username || '' }}</div>
          </div>
        </div>
        <nav class="side-nav drawer-nav">
          <button
            v-for="n in navItems"
            :key="n.key"
            type="button"
            class="side-nav-item"
            :class="{ active: selectedKeys[0] === n.key }"
            @click="navigate(n.key)"
          >
            <component :is="n.icon" class="side-nav-icon" />
            <span class="side-nav-label">{{ n.label }}</span>
          </button>
        </nav>
      </a-drawer>

      <a-layout-content class="app-content">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </a-layout-content>
    </a-layout>

    <a-modal
      v-model:visible="pwdModal"
      :title="t('action.change_password')"
      :on-before-ok="submitPassword"
      :confirm-loading="pwdSubmitting"
      unmount-on-close
    >
      <a-form :model="pwdForm" layout="vertical">
        <a-form-item :label="t('password.old')">
          <a-input-password v-model="pwdForm.old_password" autocomplete="current-password" />
        </a-form-item>
        <a-form-item :label="t('password.new')">
          <a-input-password v-model="pwdForm.new_password" autocomplete="new-password" />
          <div class="pwd-strength" v-if="pwdForm.new_password">
            <div class="pwd-bars">
              <span
                v-for="i in 5"
                :key="i"
                class="pwd-bar"
                :class="['s' + Math.min(pwdStrength.score, 5), i <= pwdStrength.score ? 'on' : '']"
              />
            </div>
            <span class="pwd-label">{{ pwdStrength.label }}</span>
          </div>
        </a-form-item>
        <a-form-item :label="t('password.confirm')">
          <a-input-password v-model="pwdForm.confirm" autocomplete="new-password" />
        </a-form-item>
      </a-form>
    </a-modal>
  </a-layout>
</template>

<style scoped>
.app-layout {
  min-height: 100vh;
  background: var(--pp-surface-soft);
}
.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  background: var(--pp-surface);
  border-bottom: 1px solid var(--pp-border);
  height: 60px;
  position: sticky;
  top: 0;
  z-index: 30;
  backdrop-filter: blur(8px);
}
.header-left { display: flex; align-items: center; gap: 12px; }
.logo { display: flex; align-items: center; gap: 10px; }
.logo-img { width: 32px; height: 32px; border-radius: 8px; }
.logo-text { display: flex; flex-direction: column; line-height: 1.15; }
.logo-title { font-weight: 600; font-size: 15px; color: var(--color-text-1); }
.logo-sub { color: var(--color-text-3); font-size: 11px; }
.header-right { display: flex; gap: 4px; align-items: center; }
.user-btn { padding: 0 10px; }
.user-name { margin-left: 4px; font-weight: 500; }
.role-tag { margin-left: 6px; }

.app-body { background: var(--pp-surface-soft); }
.app-sider {
  background: var(--pp-surface);
  border-right: 1px solid var(--pp-border);
  display: flex;
  flex-direction: column;
}
.app-sider :deep(.arco-layout-sider-children) {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.app-content {
  padding: 24px;
  background: var(--pp-surface-soft);
  min-height: calc(100vh - 60px);
}

/* Custom side nav (we don't use a-menu so we can fully control hover/active visuals). */
.side-nav { padding: 12px 10px; display: flex; flex-direction: column; gap: 2px; flex: 1; }
.side-nav-item {
  appearance: none;
  background: transparent;
  border: 0;
  border-radius: 8px;
  padding: 10px 12px;
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  font-size: 14px;
  color: var(--color-text-2);
  transition: all 0.15s ease;
  text-align: left;
}
.side-nav-item:hover {
  background: var(--pp-surface-sunken);
  color: var(--color-text-1);
}
.side-nav-item.active {
  background: var(--pp-brand-1);
  color: var(--pp-brand-7);
  font-weight: 600;
  position: relative;
}
.side-nav-item.active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 8px;
  bottom: 8px;
  width: 3px;
  background: var(--pp-brand-6);
  border-radius: 0 3px 3px 0;
}
.side-nav-icon { font-size: 16px; }
.side-nav-label { flex: 1; }
.side-foot { padding: 12px 16px; border-top: 1px solid var(--pp-border); font-size: 11px; }

.drawer-head { padding: 18px 18px 8px; display: flex; align-items: center; gap: 10px; }
.drawer-nav { padding-top: 4px; }

.fade-enter-active, .fade-leave-active { transition: opacity 0.15s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

@media (max-width: 768px) {
  .app-header { padding: 0 10px; height: 56px; }
  .logo-sub { display: none; }
  .logo-title { font-size: 14px; }
  .app-content { padding: 14px; min-height: calc(100vh - 56px); }
  .user-name { display: none; }
}

/* Password strength indicator */
.pwd-strength { display: flex; align-items: center; gap: 8px; margin-top: 6px; }
.pwd-bars { display: flex; gap: 3px; flex: 1; }
.pwd-bar {
  flex: 1;
  height: 4px;
  border-radius: 2px;
  background: var(--pp-border);
  transition: background 0.15s ease;
}
.pwd-bar.on.s1 { background: #f53f3f; }
.pwd-bar.on.s2 { background: #f53f3f; }
.pwd-bar.on.s3 { background: #ff7d00; }
.pwd-bar.on.s4 { background: #fadb14; }
.pwd-bar.on.s5 { background: var(--pp-status-active); }
.pwd-label { font-size: 11px; color: var(--color-text-3); width: 32px; text-align: right; }
</style>
