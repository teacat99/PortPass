<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import LanguageIcon from '@/components/LanguageIcon.vue'
import { useAuthStore } from '@/stores/auth'
import { setLocale } from '@/i18n'
import { Message } from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'

const { t, locale } = useI18n()
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

function toggleLocale() {
  setLocale(locale.value === 'zh-CN' ? 'en-US' : 'zh-CN')
}

const username = ref('')
const password = ref('')
const loading = ref(false)

// Brute-force lockout state. When the backend returns 429 with
// retry_after_secs we surface an in-page banner + countdown so the user
// understands why the submit button is disabled, and we don't send any
// further requests that would just fail again.
const lockoutSecs = ref(0)
const lockoutMessage = ref('')
let lockoutTimer: ReturnType<typeof setInterval> | null = null

const submitDisabled = computed(
  () => loading.value || !username.value.trim() || !password.value || lockoutSecs.value > 0,
)

function startLockout(secs: number, code: string) {
  if (lockoutTimer) clearInterval(lockoutTimer)
  lockoutSecs.value = secs
  lockoutMessage.value = t(`login.error.${code}`)
  lockoutTimer = setInterval(() => {
    lockoutSecs.value = Math.max(0, lockoutSecs.value - 1)
    if (lockoutSecs.value === 0 && lockoutTimer) {
      clearInterval(lockoutTimer)
      lockoutTimer = null
      lockoutMessage.value = ''
    }
  }, 1000)
}

onBeforeUnmount(() => {
  if (lockoutTimer) clearInterval(lockoutTimer)
})

onMounted(async () => {
  await auth.refreshStatus()
})

async function submit(e?: Event) {
  e?.preventDefault()
  if (submitDisabled.value) return
  loading.value = true
  try {
    await auth.login(username.value.trim(), password.value)
    const redirect = (route.query.redirect as string) || '/'
    router.replace(redirect)
  } catch (err: any) {
    const status = err?.response?.status as number | undefined
    const code = err?.response?.data?.code as string | undefined
    const english = err?.response?.data?.error as string | undefined
    const retryAfter = err?.response?.data?.retry_after_secs as number | undefined

    if (status === 429 && retryAfter && code) {
      startLockout(retryAfter, code)
      Message.error(t(`login.error.${code}`))
      return
    }

    // Backend returns `{code, error}` for auth/login failures. Prefer
    // the localised `login.error.<code>` bundle so toggling locale
    // translates the message immediately; English `error` is a fallback.
    const localisedKey = code ? `login.error.${code}` : ''
    const translated =
      localisedKey && t(localisedKey) !== localisedKey ? t(localisedKey) : undefined
    Message.error(translated ?? english ?? t('login.failed'))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center p-5 relative overflow-hidden bg-background">
    <!-- Ambient brand gradient. Kept as an absolutely-positioned layer so
         the form card sits on a neutral surface while still picking up the
         brand identity. -->
    <div class="absolute inset-0 pointer-events-none" aria-hidden="true">
      <div class="absolute -top-40 -left-40 size-[600px] rounded-full bg-brand-500/20 blur-3xl"></div>
      <div class="absolute -bottom-40 -right-40 size-[600px] rounded-full bg-brand-300/20 blur-3xl"></div>
    </div>

    <div class="absolute top-3 right-3 z-10">
      <Tooltip>
        <TooltipTrigger as-child>
          <Button variant="ghost" size="icon" aria-label="locale" @click="toggleLocale">
            <LanguageIcon />
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {{ locale === 'zh-CN' ? 'Switch to English' : '切换到中文' }}
        </TooltipContent>
      </Tooltip>
    </div>

    <div
      class="relative w-full max-w-sm rounded-lg border border-border bg-card shadow-modal p-7"
    >
      <div class="flex items-center gap-3 mb-6">
        <img src="@/assets/logo.svg" alt="PortPass" class="size-11 rounded-xl" />
        <div class="flex flex-col">
          <span class="font-semibold text-lg">{{ t('login.title') }}</span>
          <span class="text-xs text-muted-foreground mt-0.5">{{ t('app.subtitle') }}</span>
        </div>
      </div>

      <div
        v-if="lockoutSecs > 0"
        class="mb-4 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
        role="alert"
      >
        <div class="font-medium">{{ lockoutMessage || t('login.lockedUntil') }}</div>
        <div class="mt-0.5 text-xs opacity-80">
          {{ t('login.retryIn', { seconds: lockoutSecs }) }}
        </div>
      </div>

      <form class="flex flex-col gap-4" @submit="submit">
        <div class="flex flex-col gap-1.5">
          <Label for="login-username">{{ t('login.username') }}</Label>
          <Input
            id="login-username"
            v-model="username"
            :placeholder="t('login.usernamePlaceholder')"
            autocomplete="username"
            class="h-11 text-base"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <Label for="login-password">{{ t('login.password') }}</Label>
          <Input
            id="login-password"
            v-model="password"
            type="password"
            :placeholder="t('login.passwordPlaceholder')"
            autocomplete="current-password"
            class="h-11 text-base"
            @keydown.enter="submit"
          />
        </div>

        <Button
          type="submit"
          size="lg"
          class="w-full mt-2"
          :disabled="submitDisabled"
        >
          <span v-if="loading" class="inline-block size-4 rounded-full border-2 border-primary-foreground/50 border-t-transparent animate-spin" />
          <span v-if="lockoutSecs > 0">{{ t('login.retryIn', { seconds: lockoutSecs }) }}</span>
          <span v-else>{{ t('action.login') }}</span>
        </Button>
      </form>
    </div>
  </div>
</template>
