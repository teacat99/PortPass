<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Message } from '@arco-design/web-vue'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

// Pre-fill username with the conventional default so fresh installs can be
// signed into on a single tap. Real deployments will override this after
// rotating the seed password anyway.
const username = ref('admin')
const password = ref('')
const loading = ref(false)

onMounted(async () => {
  await auth.refreshStatus()
})

async function submit() {
  if (!username.value.trim() || !password.value) return
  loading.value = true
  try {
    await auth.login(username.value.trim(), password.value)
    const redirect = (route.query.redirect as string) || '/'
    router.replace(redirect)
  } catch (err: any) {
    Message.error(err?.response?.data?.error ?? t('login.failed'))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <a-card class="login-card">
      <div class="brand">
        <img src="@/assets/logo.svg" class="mark" alt="PortPass" />
        <div>
          <div class="title">{{ t('login.title') }}</div>
          <div class="sub">{{ t('app.subtitle') }}</div>
        </div>
      </div>
      <a-form :model="{ username, password }" layout="vertical" @submit="submit">
        <a-form-item :label="t('login.username')">
          <a-input
            v-model="username"
            size="large"
            :placeholder="t('login.usernamePlaceholder')"
            autocomplete="username"
            allow-clear
          />
        </a-form-item>
        <a-form-item :label="t('login.password')">
          <a-input-password
            v-model="password"
            size="large"
            :placeholder="t('login.passwordPlaceholder')"
            autocomplete="current-password"
            @press-enter="submit"
            allow-clear
          />
        </a-form-item>
        <a-form-item>
          <a-button long size="large" type="primary" :loading="loading" @click="submit">
            {{ t('action.login') }}
          </a-button>
        </a-form-item>
      </a-form>
    </a-card>
  </div>
</template>

<style scoped>
.login-wrap {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background:
    radial-gradient(800px 500px at 0% 0%, rgba(64, 128, 255, 0.18), transparent 60%),
    radial-gradient(700px 500px at 100% 100%, rgba(108, 177, 255, 0.18), transparent 60%),
    var(--pp-surface-soft);
  position: relative;
}
.login-wrap::before {
  /* Decorative grid backdrop. */
  content: '';
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(22, 93, 255, 0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(22, 93, 255, 0.04) 1px, transparent 1px);
  background-size: 32px 32px;
  pointer-events: none;
  mask-image: radial-gradient(ellipse at center, #000 30%, transparent 75%);
}
.login-card {
  width: 100%;
  max-width: min(92vw, 400px);
  border-radius: 16px;
  box-shadow: var(--pp-shadow-3);
  position: relative;
  z-index: 1;
}
.login-card :deep(.arco-card-body) { padding: 28px 28px 20px; }
.brand { display: flex; align-items: center; gap: 14px; margin-bottom: 24px; }
.mark { width: 44px; height: 44px; border-radius: 12px; }
.title { font-weight: 600; font-size: 19px; color: var(--color-text-1); }
.sub { color: var(--color-text-3); font-size: 12px; margin-top: 2px; }

@media (max-width: 640px) {
  .login-wrap { padding: 12px; align-items: flex-start; padding-top: 56px; }
  .login-card :deep(.arco-card-body) { padding: 22px 20px 14px; }
  .title { font-size: 17px; }
}
</style>
