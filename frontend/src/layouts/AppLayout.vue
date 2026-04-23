<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { IconHome, IconList, IconHistory, IconSettings, IconLanguage, IconExport, IconMenu } from '@arco-design/web-vue/es/icon'
import { setLocale } from '@/i18n'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const auth = useAuthStore()

const drawerVisible = ref(false)
const isMobile = ref(false)

function checkViewport() {
  isMobile.value = window.innerWidth < 768
}

onMounted(() => {
  checkViewport()
  window.addEventListener('resize', checkViewport)
  auth.refreshStatus()
})

const selectedKeys = computed(() => [String(route.name ?? 'home')])

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
        <a-button v-if="auth.required && auth.token" type="text" shape="round" status="danger" @click="logout">
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
          <a-menu-item key="settings"><template #icon><IconSettings /></template>{{ t('menu.settings') }}</a-menu-item>
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
          <a-menu-item key="settings"><template #icon><IconSettings /></template>{{ t('menu.settings') }}</a-menu-item>
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
.app-sider {
  background: var(--color-bg-2);
  border-right: 1px solid var(--color-border-2);
}
.app-content {
  padding: 24px;
  background: var(--color-bg-1);
}
@media (max-width: 768px) {
  .logo-sub { display: none; }
  .app-content { padding: 16px; }
}
.fade-enter-active, .fade-leave-active { transition: opacity 0.15s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
