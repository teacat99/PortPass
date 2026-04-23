<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Message } from '@arco-design/web-vue'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const password = ref('')
const loading = ref(false)

async function submit() {
  if (!password.value) return
  loading.value = true
  try {
    await auth.login(password.value)
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
        <div class="mark">P</div>
        <div>
          <div class="title">{{ t('login.title') }}</div>
          <div class="sub">{{ t('app.subtitle') }}</div>
        </div>
      </div>
      <a-form :model="{ password }" layout="vertical" @submit="submit">
        <a-form-item :label="t('login.password')">
          <a-input-password
            v-model="password"
            :placeholder="t('login.passwordPlaceholder')"
            @press-enter="submit"
            allow-clear
          />
        </a-form-item>
        <a-form-item>
          <a-button long type="primary" :loading="loading" @click="submit">
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
  background: linear-gradient(135deg, #eaf2ff 0%, #f5f7fa 100%);
}
.login-card { width: 100%; max-width: 380px; }
.brand { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
.mark {
  width: 42px; height: 42px; border-radius: 10px;
  background: linear-gradient(135deg, #165dff, #3e8cff);
  color: #fff; display: flex; align-items: center; justify-content: center;
  font-weight: 700; font-size: 20px;
}
.title { font-weight: 600; font-size: 18px; }
.sub { color: var(--color-text-3); font-size: 12px; }
</style>
