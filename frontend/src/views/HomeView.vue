<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Message, Notification } from '@arco-design/web-vue'
import dayjs from 'dayjs'
import { IconCheckCircleFill, IconLock, IconRight, IconSwap, IconClockCircle } from '@arco-design/web-vue/es/icon'
import { createRule, fetchClientIP, listPresets } from '@/api/rules'
import type { CreateRulePayload, PresetPort, Rule } from '@/api/types'
import { useRulesStore } from '@/stores/rules'
import { useAuthStore } from '@/stores/auth'
import { useBreakpoint } from '@/composables/useBreakpoint'
import { groupPresets } from '@/utils/presetCategory'
import CopyableText from '@/components/CopyableText.vue'
import CountdownChip from '@/components/CountdownChip.vue'

const { t, locale } = useI18n()
const router = useRouter()
const store = useRulesStore()
const auth = useAuthStore()
const { isMobile } = useBreakpoint()

const clientIP = ref<string>('')
const ipLoading = ref(true)
const presets = ref<PresetPort[]>([])
const presetsLoading = ref(true)
const submitting = ref(false)
const lastResult = ref<Rule | null>(null)

// Form state. We bind the entire object to a-form for validation.
const form = ref({
  sourceMode: 'current' as 'current' | 'any' | 'manual',
  manualSource: '',
  port: undefined as number | undefined,
  protocol: 'tcp' as 'tcp' | 'udp' | 'both',
  durationPreset: 60 * 60 as number | undefined,
  customExpire: undefined as string | undefined,
  note: ''
})

const rawDurationOptions = [
  { label: '15m', minutes: 15, value: 15 * 60 },
  { label: '1h',  minutes: 60, value: 60 * 60 },
  { label: '4h',  minutes: 240, value: 4 * 60 * 60 },
  { label: '12h', minutes: 720, value: 12 * 60 * 60 },
  { label: '24h', minutes: 1440, value: 24 * 60 * 60 }
]

// Greeting changes by hour for a tiny human touch.
const greeting = computed(() => {
  const h = dayjs().hour()
  if (h < 6) return t('home.helloNight')
  if (h < 12) return t('home.helloMorning')
  if (h < 18) return t('home.helloAfternoon')
  return t('home.helloEvening')
})

const groupedPresets = computed(() => groupPresets(presets.value))

// activePreset reflects the preset matching the typed port + protocol.
// Drives the per-preset duration cap shown to non-admin users.
const activePreset = computed<PresetPort | null>(() => {
  if (!form.value.port) return null
  for (const p of presets.value) {
    if (p.port !== form.value.port) continue
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

// Live preview of the resolved CIDR before submission.
const sourcePreview = computed(() => {
  switch (form.value.sourceMode) {
    case 'current': return clientIP.value ? `${clientIP.value}/32` : '...'
    case 'any':     return '0.0.0.0/0'
    case 'manual':  return form.value.manualSource || '—'
  }
  return '—'
})

// Live preview of the auto-close moment, expressed both as wall-clock and
// relative ("in 1h") so users can sanity-check the choice.
const expirePreview = computed(() => {
  const base = dayjs()
  const expire = form.value.customExpire
    ? dayjs(form.value.customExpire)
    : (form.value.durationPreset ? base.add(form.value.durationPreset, 'second') : null)
  if (!expire) return null
  const sameDay = expire.isSame(base, 'day')
  return {
    abs: expire.format(sameDay ? 'HH:mm' : 'MM-DD HH:mm')
  }
})

const portValid = computed(() => {
  const p = form.value.port
  return typeof p === 'number' && p >= 1 && p <= 65535
})

const submitDisabled = computed(() =>
  !portValid.value
  || (form.value.sourceMode === 'manual' && !form.value.manualSource.trim())
)

const userOnlyAllowed = computed(() => {
  // For non-admin users, every preset they see is already filtered by the
  // backend. We only highlight admin-only presets when the current account
  // *is* admin to indicate "regular users won't see this".
  return auth.isAdmin
})

onMounted(async () => {
  ipLoading.value = true
  try { clientIP.value = await fetchClientIP() }
  catch { /* toast already handled by interceptor */ }
  finally { ipLoading.value = false }

  presetsLoading.value = true
  try { presets.value = await listPresets() }
  catch { /* toasted */ }
  finally { presetsLoading.value = false }
})

function applyPreset(p: PresetPort) {
  form.value.port = p.port
  form.value.protocol = (p.protocol as typeof form.value.protocol) || 'tcp'
}

function pickDuration(value: number) {
  form.value.durationPreset = value
  form.value.customExpire = undefined
}

function resetForNext() {
  // Keep source/protocol/preset choice; clear note + result so the user can
  // immediately fire another similar rule (typical SSH + curl + commit flow).
  form.value.note = ''
  lastResult.value = null
}

function goRules() {
  router.push({ name: 'rules' })
}

async function copySshCommand() {
  if (!lastResult.value) return
  const ip = lastResult.value.source_ip.split('/')[0]
  const port = lastResult.value.port
  const cmd = port === 22
    ? `ssh -p ${port} <user>@<server>`
    : `# 端口 ${port}/${lastResult.value.protocol} 已对 ${ip} 开放\nnc -vz <server> ${port}`
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
      port: form.value.port!,
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
    Notification.success({
      title: t('msg.ruleCreated'),
      content: `${r.source_ip} :${r.port}/${r.protocol}`,
      duration: 2400
    })
    await store.reload()
    // Scroll the success panel into view so the user sees the confirmation.
    requestAnimationFrame(() => {
      document.querySelector('.pp-success-card')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="pp-page home-wrap" :class="{ 'is-mobile': isMobile }">
    <!-- Hero -->
    <section class="hero">
      <div class="hero-inner">
        <div class="hero-headline">
          <div class="hero-greeting">
            {{ greeting }}{{ auth.me ? '，' + auth.me.username : '' }} 👋
          </div>
          <div class="hero-sub">{{ t('home.welcomeSub') }}</div>
        </div>
        <div class="hero-ip">
          <div class="hero-ip-label">{{ t('home.clientIP') }}</div>
          <div class="hero-ip-value">
            <a-skeleton v-if="ipLoading" :animation="true" class="hero-ip-skel">
              <a-skeleton-line :rows="1" :widths="['180px']" />
            </a-skeleton>
            <template v-else>
              <CopyableText :value="clientIP || '—'" mono />
            </template>
          </div>
          <div class="hero-ip-sub">{{ t('home.clientIPSub') }}</div>
        </div>
      </div>
    </section>

    <!-- Form card -->
    <section class="form-card">
      <header class="form-header">
        <div>
          <h2 class="form-title">{{ t('home.createTitle') }}</h2>
          <p class="form-sub">{{ t('home.createSub') }}</p>
        </div>
      </header>

      <!-- Step ① Source -->
      <div class="step">
        <h3 class="pp-section-title">{{ t('home.stepWho') }}</h3>
        <div class="source-grid">
          <button
            v-for="opt in [
              { v: 'current', label: t('home.sourceCurrent'), icon: '👤' },
              { v: 'any',     label: t('home.sourceAny'),     icon: '🌍' },
              { v: 'manual',  label: t('home.sourceManual'),  icon: '✏️' }
            ]"
            :key="opt.v"
            type="button"
            class="source-tile"
            :class="{ active: form.sourceMode === opt.v }"
            @click="form.sourceMode = opt.v as any"
          >
            <span class="source-icon">{{ opt.icon }}</span>
            <span class="source-label">{{ opt.label }}</span>
          </button>
        </div>
        <div v-if="form.sourceMode === 'manual'" class="source-manual">
          <a-input
            v-model="form.manualSource"
            :placeholder="t('home.sourceManualPlaceholder')"
            allow-clear
            size="large"
          />
        </div>
        <div class="preview-line">
          <span class="preview-label">{{ t('home.sourcePreviewLabel') }}</span>
          <code class="preview-value">{{ sourcePreview }}</code>
        </div>
      </div>

      <!-- Step ② Service / port -->
      <div class="step">
        <h3 class="pp-section-title">{{ t('home.stepWhat') }}</h3>

        <a-skeleton v-if="presetsLoading" :animation="true">
          <a-skeleton-line :rows="2" :widths="['60%', '80%']" />
        </a-skeleton>

        <div v-else class="preset-groups">
          <div v-for="g in groupedPresets" :key="g.key" class="preset-group">
            <div class="preset-group-title">
              <span class="preset-group-icon">{{ g.icon }}</span>
              {{ t('home.cat' + g.key.charAt(0).toUpperCase() + g.key.slice(1)) }}
            </div>
            <div class="preset-chip-row">
              <button
                v-for="p in g.items"
                :key="p.id"
                type="button"
                class="preset-chip"
                :class="{ active: activePreset?.id === p.id }"
                :title="userOnlyAllowed && !p.user_allowed ? '该端口仅管理员可开放（普通用户不可见）' : ''"
                @click="applyPreset(p)"
              >
                <span class="preset-chip-name">{{ p.name }}</span>
                <span class="preset-chip-port">:{{ p.port }}/{{ p.protocol }}</span>
                <IconLock v-if="userOnlyAllowed && !p.user_allowed" class="preset-admin-only-badge" />
              </button>
            </div>
          </div>
        </div>

        <div class="custom-port-row">
          <div class="custom-port-field">
            <label class="custom-port-label">{{ t('home.customPort') }}</label>
            <a-input-number
              v-model="form.port"
              :min="1"
              :max="65535"
              :placeholder="t('home.portPlaceholder')"
              size="large"
              class="custom-port-input"
              hide-button
            />
            <div v-if="form.port && !portValid" class="field-error">{{ t('home.portInvalid') }}</div>
          </div>
          <div class="custom-port-field">
            <label class="custom-port-label">{{ t('home.protocol') }}</label>
            <a-radio-group v-model="form.protocol" type="button" size="large">
              <a-radio value="tcp">{{ t('home.protoTcp') }}</a-radio>
              <a-radio value="udp">{{ t('home.protoUdp') }}</a-radio>
              <a-radio value="both">{{ t('home.protoBoth') }}</a-radio>
            </a-radio-group>
          </div>
        </div>
      </div>

      <!-- Step ③ Duration -->
      <div class="step">
        <h3 class="pp-section-title">{{ t('home.stepHowLong') }}</h3>
        <div class="duration-row">
          <button
            v-for="opt in durationOptions"
            :key="opt.value"
            type="button"
            class="duration-chip"
            :class="{ active: form.durationPreset === opt.value && !form.customExpire }"
            @click="pickDuration(opt.value)"
          >
            {{ opt.label }}
          </button>
          <a-date-picker
            v-model="form.customExpire"
            show-time
            size="large"
            :placeholder="t('home.durationCustom')"
            class="duration-custom"
          />
        </div>
        <div v-if="!auth.isAdmin && activePreset?.max_duration_sec" class="preview-line warn">
          <IconClockCircle />
          <span>{{ t('home.durationMaxHint', { n: Math.floor(activePreset.max_duration_sec / 60) }) }}</span>
        </div>
        <div v-if="expirePreview" class="preview-line">
          <span class="preview-label">{{ t('home.durationPreviewPrefix') }}</span>
          <code class="preview-value">{{ expirePreview.abs }}</code>
          <span class="preview-label">{{ t('home.durationPreviewSuffix') }}</span>
        </div>
      </div>

      <!-- Note -->
      <div class="step note-step">
        <h3 class="pp-section-title">{{ t('home.note') }}</h3>
        <a-textarea
          v-model="form.note"
          :placeholder="t('home.notePlaceholder')"
          :max-length="255"
          allow-clear
          show-word-limit
          :auto-size="{ minRows: 1, maxRows: 3 }"
        />
      </div>

      <!-- Submit (desktop) -->
      <div class="submit-row" v-if="!isMobile">
        <a-button
          type="primary"
          size="large"
          long
          :loading="submitting"
          :disabled="submitDisabled"
          @click="submit"
        >
          <template #icon><IconCheckCircleFill /></template>
          {{ submitting ? t('home.submitting') : t('home.submit') }}
        </a-button>
      </div>
    </section>

    <!-- Success panel -->
    <section v-if="lastResult" class="pp-success-card success-card">
      <div class="success-head">
        <div class="success-mark">
          <IconCheckCircleFill />
        </div>
        <div>
          <div class="success-title">{{ t('home.submittedTitle') }}</div>
          <div class="success-sub">{{ t('home.submittedSub') }}</div>
        </div>
        <CountdownChip
          class="success-countdown"
          :expire-at="lastResult.expire_at"
          :created-at="lastResult.created_at"
        />
      </div>

      <div class="success-grid">
        <div class="success-cell">
          <div class="success-cell-label">{{ t('rules.source') }}</div>
          <CopyableText :value="lastResult.source_ip" mono />
        </div>
        <div class="success-cell">
          <div class="success-cell-label">{{ t('rules.port') }}</div>
          <span class="pp-mono">{{ lastResult.port }}/{{ lastResult.protocol }}</span>
        </div>
        <div class="success-cell">
          <div class="success-cell-label">ID</div>
          <CopyableText :value="lastResult.id" mono />
        </div>
        <div class="success-cell">
          <div class="success-cell-label">{{ t('rules.createdAt') }}</div>
          <span class="pp-mono">{{ dayjs(lastResult.created_at).format('YYYY-MM-DD HH:mm:ss') }}</span>
        </div>
      </div>

      <div class="success-actions">
        <a-button @click="resetForNext">
          <template #icon><IconSwap /></template>
          {{ t('home.submittedAgain') }}
        </a-button>
        <a-button @click="copySshCommand">{{ t('home.submittedSshHint') }}</a-button>
        <a-button type="primary" @click="goRules">
          {{ t('home.submittedView') }}
          <template #icon><IconRight /></template>
        </a-button>
      </div>
    </section>

    <!-- Mobile sticky submit bar -->
    <div v-if="isMobile" class="mobile-bar-spacer"></div>
    <div v-if="isMobile" class="mobile-submit-bar">
      <a-button
        type="primary"
        long
        size="large"
        :loading="submitting"
        :disabled="submitDisabled"
        @click="submit"
      >
        <template #icon><IconCheckCircleFill /></template>
        {{ submitting ? t('home.submitting') : t('home.submit') }}
      </a-button>
    </div>
  </div>
</template>

<style scoped>
.home-wrap {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding-bottom: 8px;
}

/* ---------------- Hero ---------------- */
.hero {
  background: var(--pp-hero-bg);
  color: var(--pp-hero-fg);
  border-radius: 16px;
  padding: 24px 28px;
  box-shadow: var(--pp-shadow-2);
  position: relative;
  overflow: hidden;
}
.hero::after {
  content: '';
  position: absolute;
  right: -60px;
  top: -60px;
  width: 220px;
  height: 220px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(255,255,255,0.18), rgba(255,255,255,0));
}
.hero-inner {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 24px;
  align-items: center;
  position: relative;
  z-index: 1;
}
.hero-greeting { font-size: 22px; font-weight: 600; line-height: 1.2; }
.hero-sub { margin-top: 6px; opacity: 0.85; font-size: 13px; line-height: 1.6; max-width: 480px; }
.hero-ip {
  background: rgba(255, 255, 255, 0.13);
  backdrop-filter: blur(6px);
  border-radius: 12px;
  padding: 12px 16px;
  min-width: 240px;
}
.hero-ip-label { font-size: 11px; opacity: 0.8; text-transform: uppercase; letter-spacing: 0.05em; }
.hero-ip-value {
  font-size: 22px;
  font-weight: 700;
  font-family: ui-monospace, SFMono-Regular, monospace;
  margin: 4px 0 2px;
  display: flex;
  align-items: center;
}
.hero-ip-value :deep(.pp-copyable) { color: #fff; }
.hero-ip-value :deep(.pp-copyable:hover) { background: rgba(255,255,255,0.16); }
.hero-ip-value :deep(.pp-copyable-icon) { color: rgba(255,255,255,0.85); }
.hero-ip-skel :deep(.arco-skeleton-line div) { background: rgba(255,255,255,0.25) !important; }
.hero-ip-sub { font-size: 11px; opacity: 0.75; }

/* ---------------- Form card ---------------- */
.form-card {
  background: var(--pp-surface);
  border-radius: 16px;
  padding: 24px 28px 20px;
  box-shadow: var(--pp-shadow-1);
  display: flex;
  flex-direction: column;
  gap: 20px;
}
.form-header { padding-bottom: 4px; border-bottom: 1px solid var(--pp-border); margin-bottom: 4px; }
.form-title { font-size: 17px; font-weight: 600; margin: 0 0 4px 0; color: var(--color-text-1); }
.form-sub { margin: 0; font-size: 13px; color: var(--color-text-3); }

.step { display: flex; flex-direction: column; gap: 10px; }

/* Source tiles */
.source-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
}
.source-tile {
  appearance: none;
  background: var(--pp-surface-soft);
  border: 1.5px solid var(--pp-border);
  border-radius: 10px;
  padding: 14px 10px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  transition: all 0.15s ease;
  font-size: 13px;
  color: var(--color-text-2);
}
.source-tile:hover { border-color: var(--pp-brand-3); background: var(--pp-brand-1); }
.source-tile.active {
  border-color: var(--pp-brand-6);
  background: var(--pp-brand-1);
  color: var(--pp-brand-7);
  font-weight: 600;
  box-shadow: 0 0 0 3px rgba(22, 93, 255, 0.12);
}
.source-icon { font-size: 22px; line-height: 1; }
.source-manual { margin-top: 4px; }

.preview-line {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--color-text-3);
  background: var(--pp-surface-sunken);
  padding: 8px 12px;
  border-radius: 8px;
}
.preview-line.warn {
  color: var(--pp-status-pending);
  background: rgba(255, 125, 0, 0.08);
}
.preview-label { font-size: 12px; }
.preview-value {
  font-family: ui-monospace, SFMono-Regular, monospace;
  font-size: 13px;
  color: var(--color-text-1);
  font-weight: 600;
  background: transparent;
  padding: 0;
}

/* Preset groups */
.preset-groups { display: flex; flex-direction: column; gap: 12px; }
.preset-group-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--color-text-3);
  margin-bottom: 6px;
  font-weight: 500;
}
.preset-group-icon { font-size: 14px; }
.preset-chip-row { display: flex; flex-wrap: wrap; gap: 8px; }
.preset-chip {
  appearance: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-radius: 999px;
  border: 1px solid var(--pp-border);
  background: var(--pp-surface-soft);
  cursor: pointer;
  font-size: 13px;
  transition: all 0.15s ease;
  color: var(--color-text-2);
}
.preset-chip:hover {
  border-color: var(--pp-brand-3);
  background: var(--pp-brand-1);
  color: var(--pp-brand-7);
}
.preset-chip.active {
  border-color: var(--pp-brand-6);
  background: var(--pp-brand-6);
  color: #fff;
  font-weight: 600;
}
.preset-chip-name { font-weight: 500; }
.preset-chip-port { font-family: ui-monospace, monospace; font-size: 11px; opacity: 0.78; }
.preset-chip.active .preset-chip-port { color: rgba(255,255,255,0.85); }
.preset-admin-only-badge {
  font-size: 10px;
  color: var(--color-text-3);
  background: var(--pp-surface);
  border-radius: 999px;
  padding: 2px;
  margin-left: 2px;
  opacity: 0.7;
}
.preset-chip.active .preset-admin-only-badge { color: rgba(255,255,255,0.9); background: rgba(255,255,255,0.16); opacity: 1; }

/* Custom port row */
.custom-port-row {
  display: grid;
  grid-template-columns: 220px 1fr;
  gap: 16px;
  margin-top: 8px;
  padding-top: 12px;
  border-top: 1px dashed var(--pp-border);
}
.custom-port-field { display: flex; flex-direction: column; gap: 6px; }
.custom-port-label { font-size: 12px; color: var(--color-text-3); font-weight: 500; }
.custom-port-input { width: 100%; }
.field-error { font-size: 12px; color: var(--pp-status-failed); }

/* Duration */
.duration-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.duration-chip {
  appearance: none;
  padding: 8px 16px;
  border-radius: 8px;
  border: 1px solid var(--pp-border);
  background: var(--pp-surface-soft);
  cursor: pointer;
  font-weight: 500;
  font-size: 13px;
  color: var(--color-text-2);
  transition: all 0.15s ease;
  min-width: 60px;
}
.duration-chip:hover { border-color: var(--pp-brand-3); }
.duration-chip.active {
  background: var(--pp-brand-6);
  color: #fff;
  border-color: var(--pp-brand-6);
}
.duration-custom { width: 240px; }

.note-step :deep(.arco-textarea-wrapper) { border-radius: 10px; }

/* Submit (desktop) */
.submit-row { padding-top: 8px; }
.submit-row :deep(.arco-btn) { font-size: 15px; height: 48px; border-radius: 10px; }

/* ---------------- Success card ---------------- */
.success-card {
  background: var(--pp-surface);
  border-radius: 16px;
  padding: 22px 26px;
  box-shadow: var(--pp-shadow-2);
  border: 1px solid rgba(0, 180, 42, 0.25);
  position: relative;
  overflow: hidden;
}
.success-card::before {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(135deg, rgba(0, 180, 42, 0.06), transparent 50%);
  pointer-events: none;
}
.success-head { display: flex; align-items: center; gap: 14px; position: relative; z-index: 1; }
.success-mark {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--pp-status-active);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  flex: 0 0 40px;
}
.success-title { font-size: 16px; font-weight: 600; color: var(--color-text-1); }
.success-sub { font-size: 12px; color: var(--color-text-3); margin-top: 2px; }
.success-countdown { margin-left: auto; }

.success-grid {
  margin-top: 16px;
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px 24px;
  position: relative;
  z-index: 1;
}
.success-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 13px;
  color: var(--color-text-1);
}
.success-cell-label { font-size: 11px; color: var(--color-text-3); text-transform: uppercase; letter-spacing: 0.04em; }

.success-actions {
  margin-top: 16px;
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  position: relative;
  z-index: 1;
}

/* ---------------- Mobile ---------------- */
.mobile-bar-spacer { height: 64px; }
.mobile-submit-bar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  padding: 10px 14px calc(10px + env(safe-area-inset-bottom));
  background: var(--pp-surface);
  border-top: 1px solid var(--pp-border);
  z-index: 50;
  box-shadow: 0 -4px 12px rgba(15, 23, 42, 0.06);
}
.mobile-submit-bar :deep(.arco-btn) { height: 48px; font-size: 15px; border-radius: 10px; }

.is-mobile .hero { padding: 18px 18px; border-radius: 14px; }
.is-mobile .hero-inner { grid-template-columns: 1fr; gap: 12px; }
.is-mobile .hero-ip { min-width: 0; }
.is-mobile .form-card { padding: 18px 16px; border-radius: 14px; }
.is-mobile .source-grid { gap: 8px; }
.is-mobile .source-tile { padding: 10px 6px; font-size: 12px; }
.is-mobile .source-icon { font-size: 18px; }
.is-mobile .custom-port-row { grid-template-columns: 1fr; gap: 12px; }
.is-mobile .duration-custom { width: 100%; }
.is-mobile .success-card { padding: 18px 16px; border-radius: 14px; }
.is-mobile .success-grid { grid-template-columns: 1fr 1fr; gap: 10px; }
.is-mobile .success-actions { flex-direction: column; }
.is-mobile .success-actions :deep(.arco-btn) { width: 100%; }

@media (max-width: 480px) {
  .source-grid { grid-template-columns: 1fr; }
  .duration-row .duration-chip { flex: 1; min-width: 0; }
}
</style>
