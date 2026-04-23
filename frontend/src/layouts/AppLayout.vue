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
  IconLock
} from '@arco-design/web-vue/es/icon'
import { Message } from '@arco-design/web-vue'
import { setLocale } from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import { useBreakpoint } from '@/composables/useBreakpoint'
import { changeOwnPassword } from '@/api/auth'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const auth = useAuthStore()
const { isMobile } = useBreakpoint()

const drawerVisible = ref(false)

// Change-password modal state. Surfaced in the user dropdown in the top
// bar; disabled in none/ipwhitelist modes where the principal is the
// implicit system admin and has no real credentials.
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

async function submitPassword() {
  if (pwdForm.value.new_password.length < 6) {
    Message.warning(t('password.too_short'))
    return
  }
  if (pwdForm.value.new_password !== pwdForm.value.confirm) {
    Message.warning(t('password.mismatch'))
    return
  }
  pwdSubmitting.value = true
  try {
    await changeOwnPassword(pwdForm.value.old_password, pwdForm.value.new_password)
    Message.success(t('password.changed'))
    pwdModal.value = false
  } catch {
    // Interceptor already surfaced the server message.
  } finally {
    pwdSubmitting.value = false
  }
}
</script>

<template>
  <a-layout class="app-layout">
    <a-layout-header class="app-header">
      <div class="header-left">
        <a-button v-if="isMobile" type="text" @click="drawerVisible = true">
          <template #icon><IconMenu /></template>
        </a-button>
        <div class="logo">
          <div class="logo-mark">P</div>
          <div class="logo-text">
            <div class="logo-title">{{ t('app.title') }}</div>
            <div class="logo-sub">{{ t('app.subtitle') }}</div>
          </div>
        </div>
      </div>
      <div class="header-right">
        <a-button type="text" shape="round" @click="toggleLocale">
          <template #icon><IconLanguage /></template>
          {{ locale === 'zh-CN' ? 'EN' : '中' }}
        </a-button>
        <a-dropdown v-if="auth.me" trigger="click" position="br">
          <a-button type="text" shape="round">
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

    <a-layout>
      <a-layout-sider v-if="!isMobile" :width="220" class="app-sider">
        <a-menu :selected-keys="selectedKeys" @menu-item-click="navigate">
          <a-menu-item key="home"><template #icon><IconHome /></template>{{ t('menu.home') }}</a-menu-item>
          <a-menu-item key="rules"><template #icon><IconList /></template>{{ t('menu.rules') }}</a-menu-item>
          <a-menu-item key="history"><template #icon><IconHistory /></template>{{ t('menu.history') }}</a-menu-item>
          <a-menu-item v-if="isAdmin" key="users"><template #icon><IconUserGroup /></template>{{ t('menu.users') }}</a-menu-item>
          <a-menu-item v-if="isAdmin" key="settings"><template #icon><IconSettings /></template>{{ t('menu.settings') }}</a-menu-item>
        </a-menu>
      </a-layout-sider>

      <a-drawer
        v-model:visible="drawerVisible"
        placement="left"
        :width="240"
        :footer="false"
        :header="false"
        unmount-on-close
      >
        <a-menu :selected-keys="selectedKeys" @menu-item-click="navigate">
          <a-menu-item key="home"><template #icon><IconHome /></template>{{ t('menu.home') }}</a-menu-item>
          <a-menu-item key="rules"><template #icon><IconList /></template>{{ t('menu.rules') }}</a-menu-item>
          <a-menu-item key="history"><template #icon><IconHistory /></template>{{ t('menu.history') }}</a-menu-item>
          <a-menu-item v-if="isAdmin" key="users"><template #icon><IconUserGroup /></template>{{ t('menu.users') }}</a-menu-item>
          <a-menu-item v-if="isAdmin" key="settings"><template #icon><IconSettings /></template>{{ t('menu.settings') }}</a-menu-item>
        </a-menu>
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
}
.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  background: var(--color-bg-2);
  border-bottom: 1px solid var(--color-border-2);
  height: 64px;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.logo {
  display: flex;
  align-items: center;
  gap: 10px;
}
.logo-mark {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: linear-gradient(135deg, #165dff, #3e8cff);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 20px;
}
.logo-text {
  display: flex;
  flex-direction: column;
  line-height: 1.15;
}
.logo-title { font-weight: 600; font-size: 16px; }
.logo-sub { color: var(--color-text-3); font-size: 12px; }
.header-right { display: flex; gap: 8px; align-items: center; }
.user-name { margin-left: 4px; font-weight: 500; }
.role-tag { margin-left: 6px; }
.app-sider {
  background: var(--color-bg-2);
  border-right: 1px solid var(--color-border-2);
}
.app-content {
  padding: 24px;
  background: var(--color-bg-1);
}
@media (max-width: 768px) {
  .app-header { padding: 0 12px; }
  .logo-sub { display: none; }
  .logo-title { font-size: 15px; }
  .app-content { padding: 14px; }
  .user-name { display: none; }
}
.fade-enter-active, .fade-leave-active { transition: opacity 0.15s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
