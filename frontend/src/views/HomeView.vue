<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import dayjs from 'dayjs'
import { CheckCircle2, Lock, ArrowRight, RotateCcw, Clock } from 'lucide-vue-next'
import { createRule, fetchClientIP, listPresets } from '@/api/rules'
import type { CreateRulePayload, PresetPort, Rule } from '@/api/types'
import { useRulesStore } from '@/stores/rules'
import { useAuthStore } from '@/stores/auth'
import { groupPresets } from '@/utils/presetCategory'
import { parsePortSet } from '@/utils/portset'
import { toast } from 'vue-sonner'
import { Message } from '@/lib/toast'

import CopyableText from '@/components/CopyableText.vue'
import CountdownChip from '@/components/CountdownChip.vue'
import DateTimePicker from '@/components/DateTimePicker.vue'
import PortSetInput from '@/components/PortSetInput.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const { t } = useI18n()
const router = useRouter()
const store = useRulesStore()
const auth = useAuthStore()

const clientIP = ref<string>('')
const ipLoading = ref(true)
const presets = ref<PresetPort[]>([])
const presetsLoading = ref(true)
const submitting = ref(false)
const lastResult = ref<Rule | null>(null)

const form = ref({
  sourceMode: 'current' as 'current' | 'any' | 'manual',
  manualSource: '',
  ports: '' as string,
  protocol: 'tcp' as 'tcp' | 'udp' | 'both',
  durationPreset: 60 * 60 as number | undefined,
  customExpire: undefined as string | undefined,
  note: ''
})
// Initial validity reflects an empty ports string (invalid unless allowEmpty).
const portsValidation = ref<{ ok: boolean, error: string | null }>({ ok: false, error: null })

const rawDurationOptions = [
  { label: '15m', value: 15 * 60 },
  { label: '1h',  value: 60 * 60 },
  { label: '4h',  value: 4 * 60 * 60 },
  { label: '12h', value: 12 * 60 * 60 },
  { label: '24h', value: 24 * 60 * 60 }
]

const greeting = computed(() => {
  const h = dayjs().hour()
  if (h < 6) return t('home.helloNight')
  if (h < 12) return t('home.helloMorning')
  if (h < 18) return t('home.helloAfternoon')
  return t('home.helloEvening')
})

const groupedPresets = computed(() => groupPresets(presets.value))

// activePreset locates the preset matching the current port set + protocol
// so we can surface per-preset constraints (e.g. max duration cap).
const activePreset = computed<PresetPort | null>(() => {
  const parsed = parsePortSet(form.value.ports)
  if (!parsed.ok || parsed.count === 0) return null
  for (const p of presets.value) {
    const pp = parsePortSet(p.ports || String(p.port || ''))
    if (!pp.ok) continue
    if (pp.canonical !== parsed.canonical) continue
    if (p.protocol === form.value.protocol || p.protocol === 'both') return p
  }
  return null
})

const durationOptions = computed(() => {
  const max = auth.isAdmin ? 0 : activePreset.value?.max_duration_sec ?? 0
  if (!max) return rawDurationOptions
  return rawDurationOptions.filter((o) => o.value <= max)
})

watch(durationOptions, (opts) => {
  if (form.value.customExpire) return
  if (form.value.durationPreset && opts.some((o) => o.value === form.value.durationPreset)) return
  form.value.durationPreset = opts.length ? opts[opts.length - 1].value : undefined
})

const sourcePreview = computed(() => {
  switch (form.value.sourceMode) {
    case 'current': return clientIP.value ? `${clientIP.value}/32` : '...'
    case 'any':     return '0.0.0.0/0'
    case 'manual':  return form.value.manualSource || '—'
  }
  return '—'
})

const expirePreview = computed(() => {
  const base = dayjs()
  const expire = form.value.customExpire
    ? dayjs(form.value.customExpire)
    : (form.value.durationPreset ? base.add(form.value.durationPreset, 'second') : null)
  if (!expire) return null
  const sameDay = expire.isSame(base, 'day')
  return { abs: expire.format(sameDay ? 'HH:mm' : 'MM-DD HH:mm') }
})

const portsValid = computed(() => portsValidation.value.ok)

const submitDisabled = computed(() =>
  !portsValid.value
  || (form.value.sourceMode === 'manual' && !form.value.manualSource.trim())
)

const userCanSeePresetLocks = computed(() => auth.isAdmin)

onMounted(async () => {
  ipLoading.value = true
  try { clientIP.value = await fetchClientIP() }
  catch { /* handled by axios interceptor */ }
  finally { ipLoading.value = false }

  presetsLoading.value = true
  try { presets.value = await listPresets() }
  catch { /* ditto */ }
  finally { presetsLoading.value = false }
})

function applyPreset(p: PresetPort) {
  form.value.ports = p.ports || (p.port ? String(p.port) : '')
  form.value.protocol = (p.protocol as typeof form.value.protocol) || 'tcp'
}

function pickDuration(value: number) {
  form.value.durationPreset = value
  form.value.customExpire = undefined
}

function resetForNext() {
  form.value.note = ''
  lastResult.value = null
}

function goRules() { router.push({ name: 'rules' }) }

async function copySshCommand() {
  if (!lastResult.value) return
  const ip = lastResult.value.source_ip.split('/')[0]
  const portsStr = lastResult.value.ports || String(lastResult.value.port || '')
  const parsed = parsePortSet(portsStr)
  const firstPort = parsed.ok && parsed.ranges.length ? parsed.ranges[0].from : lastResult.value.port
  const cmd = firstPort === 22
    ? `ssh -p 22 <user>@<server>`
    : `# 端口 ${portsStr}/${lastResult.value.protocol} 已对 ${ip} 开放\nnc -vz <server> ${firstPort}`
  try {
    await navigator.clipboard.writeText(cmd)
    Message.success(t('home.submittedSshHintCopied'))
  } catch {
    Message.warning(cmd)
  }
}

async function submit() {
  if (submitDisabled.value) {
    Message.warning(t('home.portInvalid'))
    return
  }
  submitting.value = true
  try {
    const payload: CreateRulePayload = {
      ports: form.value.ports,
      protocol: form.value.protocol,
      note: form.value.note,
      use_client_ip: form.value.sourceMode === 'current',
      source_ip: form.value.sourceMode === 'any'
        ? '0.0.0.0/0'
        : (form.value.sourceMode === 'manual' ? form.value.manualSource.trim() : undefined),
      duration_sec: form.value.customExpire ? undefined : form.value.durationPreset,
      expire_at: form.value.customExpire ? dayjs(form.value.customExpire).toISOString() : undefined
    }
    const r = await createRule(payload)
    lastResult.value = r
    toast.success(t('msg.ruleCreated'), {
      description: `${r.source_ip} :${r.ports || r.port}/${r.protocol}`,
      duration: 2400
    })
    await store.reload()
    requestAnimationFrame(() => {
      document.getElementById('pp-success-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  } finally {
    submitting.value = false
  }
}

type SourceMode = 'current' | 'any' | 'manual'
interface SourceOpt { v: SourceMode; label: string; icon: string }
const sourceOptions = computed<SourceOpt[]>(() => [
  { v: 'current', label: t('home.sourceCurrent'), icon: '👤' },
  { v: 'any',     label: t('home.sourceAny'),     icon: '🌍' },
  { v: 'manual',  label: t('home.sourceManual'),  icon: '✏️' }
])
</script>

<template>
  <div class="pp-page flex flex-col gap-5 pb-2">
    <!--
      Hero — refined per the Phase-2 feedback: cleaner flat surface with a
      single brand stripe accent, no gradients.
    -->
    <section class="relative overflow-hidden rounded-lg border border-border bg-card px-5 md:px-7 py-5">
      <span class="absolute left-0 top-4 bottom-4 w-[3px] rounded-r bg-primary" aria-hidden="true" />
      <div class="grid grid-cols-1 md:grid-cols-[1fr_auto] gap-4 md:gap-6 items-center">
        <div>
          <div class="text-lg md:text-xl font-semibold text-foreground leading-tight">
            {{ greeting }}<span v-if="auth.me">，{{ auth.me.username }}</span> 👋
          </div>
          <div class="mt-1 text-xs md:text-sm text-muted-foreground leading-relaxed max-w-prose">
            {{ t('home.welcomeSub') }}
          </div>
        </div>
        <div class="rounded-md border border-border bg-muted/60 px-3.5 py-2.5 min-w-0 md:min-w-[220px]">
          <div class="text-[11px] uppercase tracking-wider text-muted-foreground font-medium">
            {{ t('home.clientIP') }}
          </div>
          <div class="text-base md:text-lg font-semibold font-mono tabular-nums text-foreground mt-0.5 min-h-[22px] flex items-center">
            <span v-if="ipLoading" class="inline-block w-28 h-4 rounded bg-muted animate-pulse" />
            <CopyableText v-else :value="clientIP || '—'" mono />
          </div>
          <div class="text-[11px] text-muted-foreground mt-0.5">
            {{ t('home.clientIPSub') }}
          </div>
        </div>
      </div>
    </section>

    <!-- Form card -->
    <section class="rounded-lg bg-card shadow-card flex flex-col gap-5 p-5 md:p-7">
      <header class="pb-3 border-b border-border">
        <h2 class="text-base md:text-lg font-semibold text-foreground mb-1">
          {{ t('home.createTitle') }}
        </h2>
        <p class="text-xs md:text-sm text-muted-foreground m-0">
          {{ t('home.createSub') }}
        </p>
      </header>

      <!-- Step ① Source -->
      <div class="flex flex-col gap-3">
        <h3 class="pp-section-title">{{ t('home.stepWho') }}</h3>
        <div class="grid grid-cols-3 gap-2 md:gap-2.5">
          <button
            v-for="opt in sourceOptions"
            :key="opt.v"
            type="button"
            class="flex flex-col items-center justify-center gap-1.5 py-3 px-2 rounded-md border-[1.5px] text-xs md:text-sm transition-all"
            :class="form.sourceMode === opt.v
              ? 'border-primary bg-primary/10 text-primary font-semibold shadow-[0_0_0_3px_rgba(22,93,255,0.08)]'
              : 'border-border bg-muted/50 text-muted-foreground hover:border-primary/50 hover:bg-primary/5'"
            @click="form.sourceMode = opt.v"
          >
            <span class="text-lg md:text-xl leading-none">{{ opt.icon }}</span>
            <span class="leading-tight text-center">{{ opt.label }}</span>
          </button>
        </div>
        <div v-if="form.sourceMode === 'manual'">
          <Input
            v-model="form.manualSource"
            :placeholder="t('home.sourceManualPlaceholder')"
            class="h-10"
          />
        </div>
        <div class="flex items-center gap-2 text-xs bg-muted/50 px-3 py-2 rounded-md">
          <span class="text-muted-foreground">{{ t('home.sourcePreviewLabel') }}</span>
          <code class="font-mono font-semibold text-foreground">{{ sourcePreview }}</code>
        </div>
      </div>

      <!-- Step ② Port / service -->
      <div class="flex flex-col gap-3">
        <h3 class="pp-section-title">{{ t('home.stepWhat') }}</h3>

        <div v-if="presetsLoading" class="flex flex-col gap-2">
          <div class="h-4 rounded bg-muted animate-pulse w-3/5" />
          <div class="h-4 rounded bg-muted animate-pulse w-4/5" />
        </div>

        <div v-else class="flex flex-col gap-3">
          <div v-for="g in groupedPresets" :key="g.key" class="flex flex-col gap-1.5">
            <div class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
              <span class="text-sm">{{ g.icon }}</span>
              {{ t('home.cat' + g.key.charAt(0).toUpperCase() + g.key.slice(1)) }}
            </div>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="p in g.items"
                :key="p.id"
                type="button"
                class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full border text-xs md:text-sm transition-all"
                :class="activePreset?.id === p.id
                  ? 'border-primary bg-primary text-primary-foreground font-semibold'
                  : 'border-border bg-muted/50 text-muted-foreground hover:border-primary/50 hover:bg-primary/5 hover:text-primary'"
                :title="userCanSeePresetLocks && !p.user_allowed ? '该端口仅管理员可开放' : ''"
                @click="applyPreset(p)"
              >
                <span class="font-medium">{{ p.name }}</span>
                <span class="font-mono text-[11px] opacity-75">{{ p.ports || p.port }}/{{ p.protocol }}</span>
                <Lock v-if="userCanSeePresetLocks && !p.user_allowed" class="size-3 opacity-60" />
              </button>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-[1fr_auto] gap-3 md:gap-4 mt-2 pt-3 border-t border-dashed border-border items-start">
          <div class="flex flex-col gap-1.5 min-w-0">
            <Label>{{ t('home.customPort') }}</Label>
            <PortSetInput
              v-model="form.ports"
              :placeholder="t('portSet.placeholder')"
              input-class="h-10"
              :quick="[
                { label: '+22',  value: '22' },
                { label: '+80',  value: '80' },
                { label: '+443', value: '443' }
              ]"
              @validation="(ok: boolean, error: string | null) => (portsValidation = { ok, error })"
            />
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('home.protocol') }}</Label>
            <div class="inline-flex p-1 rounded-md bg-muted/60 border border-border">
              <button
                v-for="p in ['tcp', 'udp', 'both']"
                :key="p"
                type="button"
                class="px-3 md:px-4 h-8 rounded text-xs md:text-sm font-medium transition-colors"
                :class="form.protocol === p
                  ? 'bg-card text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'"
                @click="form.protocol = p as any"
              >
                {{ t('home.proto' + (p === 'tcp' ? 'Tcp' : p === 'udp' ? 'Udp' : 'Both')) }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Step ③ Duration -->
      <div class="flex flex-col gap-3">
        <h3 class="pp-section-title">{{ t('home.stepHowLong') }}</h3>
        <div class="flex flex-wrap gap-2 items-center">
          <button
            v-for="opt in durationOptions"
            :key="opt.value"
            type="button"
            class="min-w-[60px] h-9 px-4 rounded-md border text-sm font-medium transition-all"
            :class="form.durationPreset === opt.value && !form.customExpire
              ? 'bg-primary text-primary-foreground border-primary'
              : 'border-border bg-muted/50 text-muted-foreground hover:border-primary/50 hover:text-foreground'"
            @click="pickDuration(opt.value)"
          >{{ opt.label }}</button>

          <DateTimePicker
            v-model="form.customExpire"
            class="w-[220px]"
            :placeholder="t('home.durationCustom')"
          />
        </div>
        <div
          v-if="!auth.isAdmin && activePreset?.max_duration_sec"
          class="flex items-center gap-2 text-xs rounded-md px-3 py-2 bg-amber-500/10 text-amber-700 dark:text-amber-300"
        >
          <Clock class="size-3.5" />
          <span>{{ t('home.durationMaxHint', { n: Math.floor(activePreset.max_duration_sec / 60) }) }}</span>
        </div>
        <div
          v-if="expirePreview"
          class="flex items-center gap-2 text-xs rounded-md px-3 py-2 bg-muted/50 text-muted-foreground"
        >
          <span>{{ t('home.durationPreviewPrefix') }}</span>
          <code class="font-mono font-semibold text-foreground">{{ expirePreview.abs }}</code>
          <span>{{ t('home.durationPreviewSuffix') }}</span>
        </div>
      </div>

      <!-- Note -->
      <div class="flex flex-col gap-3">
        <h3 class="pp-section-title">{{ t('home.note') }}</h3>
        <textarea
          v-model="form.note"
          rows="2"
          maxlength="255"
          :placeholder="t('home.notePlaceholder')"
          class="min-h-[60px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring resize-y"
        />
      </div>

      <!-- Submit (desktop inline) -->
      <div class="hidden md:block pt-2">
        <Button
          size="lg"
          class="w-full h-12 text-base"
          :disabled="submitDisabled || submitting"
          @click="submit"
        >
          <CheckCircle2 v-if="!submitting" class="size-5" />
          <span v-else class="inline-block size-5 rounded-full border-2 border-primary-foreground/50 border-t-transparent animate-spin" />
          <span>{{ submitting ? t('home.submitting') : t('home.submit') }}</span>
        </Button>
      </div>
    </section>

    <!-- Success panel -->
    <section
      v-if="lastResult"
      id="pp-success-panel"
      class="relative overflow-hidden rounded-lg border border-emerald-500/30 bg-card shadow-card p-5 md:p-6"
    >
      <div class="absolute inset-0 pointer-events-none bg-gradient-to-br from-emerald-500/5 to-transparent" aria-hidden="true" />
      <div class="relative flex items-center gap-3.5">
        <div class="size-10 rounded-full bg-emerald-500 text-white flex items-center justify-center shrink-0">
          <CheckCircle2 class="size-6" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="font-semibold text-base text-foreground">{{ t('home.submittedTitle') }}</div>
          <div class="text-xs text-muted-foreground mt-0.5">{{ t('home.submittedSub') }}</div>
        </div>
        <CountdownChip
          :expire-at="lastResult.expire_at"
          :created-at="lastResult.created_at"
        />
      </div>

      <div class="relative grid grid-cols-2 gap-y-3 gap-x-6 mt-4">
        <div class="flex flex-col gap-0.5 min-w-0">
          <div class="text-[11px] uppercase tracking-wide text-muted-foreground">{{ t('rules.source') }}</div>
          <CopyableText :value="lastResult.source_ip" mono />
        </div>
        <div class="flex flex-col gap-0.5 min-w-0">
          <div class="text-[11px] uppercase tracking-wide text-muted-foreground">{{ t('rules.port') }}</div>
          <span class="font-mono text-sm">{{ lastResult.ports || lastResult.port }}/{{ lastResult.protocol }}</span>
        </div>
        <div class="flex flex-col gap-0.5 min-w-0">
          <div class="text-[11px] uppercase tracking-wide text-muted-foreground">ID</div>
          <CopyableText :value="lastResult.id" mono />
        </div>
        <div class="flex flex-col gap-0.5 min-w-0">
          <div class="text-[11px] uppercase tracking-wide text-muted-foreground">{{ t('rules.createdAt') }}</div>
          <span class="font-mono text-sm">{{ dayjs(lastResult.created_at).format('YYYY-MM-DD HH:mm:ss') }}</span>
        </div>
      </div>

      <div class="relative flex flex-wrap gap-2 mt-4">
        <Button variant="outline" @click="resetForNext">
          <RotateCcw class="size-4" />
          {{ t('home.submittedAgain') }}
        </Button>
        <Button variant="outline" @click="copySshCommand">
          {{ t('home.submittedSshHint') }}
        </Button>
        <Button @click="goRules" class="ml-auto">
          {{ t('home.submittedView') }}
          <ArrowRight class="size-4" />
        </Button>
      </div>
    </section>

    <!-- Mobile sticky submit bar -->
    <div class="md:hidden h-16" aria-hidden="true" />
    <div
      class="md:hidden fixed inset-x-0 bottom-[calc(4.5rem+env(safe-area-inset-bottom,0px))] z-40 p-3 bg-card border-t border-border shadow-[0_-4px_12px_rgba(15,23,42,0.06)]"
    >
      <Button
        size="lg"
        class="w-full h-12 text-base"
        :disabled="submitDisabled || submitting"
        @click="submit"
      >
        <CheckCircle2 v-if="!submitting" class="size-5" />
        <span v-else class="inline-block size-5 rounded-full border-2 border-primary-foreground/50 border-t-transparent animate-spin" />
        <span>{{ submitting ? t('home.submitting') : t('home.submit') }}</span>
      </Button>
    </div>
  </div>
</template>
