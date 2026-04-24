<script setup lang="ts">
import { onMounted, ref } from 'vue'
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

// Kept minimal; login page only exposes locale toggle (no theme / user menu)
// because the page is reached pre-auth. The toggle mirrors the header button
// in AppLayout so returning users see a familiar control.
function toggleLocale() {
  setLocale(locale.value === 'zh-CN' ? 'en-US' : 'zh-CN')
}

const username = ref('')
const password = ref('')
const loading = ref(false)

onMounted(async () => {
  await auth.refreshStatus()
})

async function submit(e?: Event) {
  e?.preventDefault()
  if (!username.value.trim() || !password.value) return
  loading.value = true
  try {
    await auth.login(username.value.trim(), password.value)
    const redirect = (route.query.redirect as string) || '/'
    router.replace(redirect)
  } catch (err: any) {
    // Backend returns `{code, error}` for auth/login failures. We prefer
    // the localised `login.error.<code>` bundle so that toggling locale
    // translates the message immediately; the English `error` string is
    // only used as a final fallback for unrecognised codes.
    const code = err?.response?.data?.code as string | undefined
    const english = err?.response?.data?.error as string | undefined
    const localisedKey = code ? `login.error.${code}` : ''
    const translated = localisedKey && t(localisedKey) !== localisedKey
      ? t(localisedKey)
      : undefined
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
          :disabled="loading || !username.trim() || !password"
        >
          <span v-if="loading" class="inline-block size-4 rounded-full border-2 border-primary-foreground/50 border-t-transparent animate-spin" />
          <span>{{ t('action.login') }}</span>
        </Button>
      </form>
    </div>
  </div>
</template>
