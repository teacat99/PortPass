<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message, Notification } from '@arco-design/web-vue'
import dayjs from 'dayjs'
import { createRule, fetchClientIP, listPresets } from '@/api/rules'
import type { PresetPort, Rule } from '@/api/types'
import { useRulesStore } from '@/stores/rules'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const store = useRulesStore()
const auth = useAuthStore()

const clientIP = ref<string>('')
const presets = ref<PresetPort[]>([])
const loading = ref(false)
const lastResult = ref<Rule | null>(null)

const sourceMode = ref<'current' | 'any' | 'manual'>('current')
const manualSource = ref<string>('')
const port = ref<number | undefined>(undefined)
const protocol = ref<'tcp' | 'udp' | 'both'>('tcp')
const durationPreset = ref<number | undefined>(60 * 60)
const customExpire = ref<string | undefined>(undefined)
const note = ref<string>('')

// Raw duration presets. For non-admin callers we filter out any option
// that exceeds the port's MaxDurationSec (enforced by the backend as well).
const rawDurationOptions = [
  { label: '15m', value: 15 * 60 },
  { label: '1h', value: 60 * 60 },
  { label: '4h', value: 4 * 60 * 60 },
  { label: '12h', value: 12 * 60 * 60 },
  { label: '24h', value: 24 * 60 * 60 }
]

// activePreset is the preset matching the currently typed (port, protocol)
// pair - the user may have picked it via the quick buttons, or typed a
// matching port manually. It drives the user-facing policy hint and the
// duration-button filtering.
const activePreset = computed<PresetPort | null>(() => {
  if (!port.value) return null
  for (const p of presets.value) {
    if (p.port !== port.value) continue
    if (p.protocol === protocol.value || p.protocol === 'both') return p
  }
  return null
})

// Non-admin users cannot pick a duration longer than their preset allows.
// Admins see the raw options unchanged.
const durationOptions = computed(() => {
  const max = auth.isAdmin ? 0 : activePreset.value?.max_duration_sec ?? 0
  if (!max) return rawDurationOptions
  return rawDurationOptions.filter((o) => o.value <= max)
})

// Whenever the filtered list shrinks, ensure the selected preset is still
// present; otherwise snap to the largest option available.
watch(durationOptions, (opts) => {
  if (!durationPreset.value) return
  if (opts.some((o) => o.value === durationPreset.value)) return
  durationPreset.value = opts.length ? opts[opts.length - 1].value : undefined
})

const sourcePreview = computed(() => {
  switch (sourceMode.value) {
    case 'current': return clientIP.value ? `${clientIP.value}/32` : '...'
    case 'any': return '0.0.0.0/0'
    case 'manual': return manualSource.value || '—'
  }
  return '—'
})

onMounted(async () => {
  try {
    clientIP.value = await fetchClientIP()
  } catch { /* already toasted */ }
  try {
    presets.value = await listPresets()
  } catch { /* already toasted */ }
})

function applyPreset(p: PresetPort) {
  port.value = p.port
  protocol.value = (p.protocol as 'tcp' | 'udp' | 'both') || 'tcp'
}

async function submit() {
  if (!port.value) {
    Message.warning(t('msg.invalidInput'))
    return
  }
  loading.value = true
  try {
    const payload = {
      port: port.value,
      protocol: protocol.value,
      note: note.value,
      use_client_ip: sourceMode.value === 'current',
      source_ip: sourceMode.value === 'any' ? '0.0.0.0/0' : (sourceMode.value === 'manual' ? manualSource.value : undefined),
      duration_sec: customExpire.value ? undefined : (durationPreset.value ?? undefined),
      expire_at: customExpire.value ? dayjs(customExpire.value as string).toISOString() : undefined
    }
    const r = await createRule(payload)
    lastResult.value = r
    Notification.success({ title: t('msg.ruleCreated'), content: `${r.source_ip} :${r.port}/${r.protocol}` })
    await store.reload()
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="home-wrap">
    <a-card class="info-card">
      <div class="ip-row">
        <div class="ip-label">{{ t('home.clientIP') }}</div>
        <div class="ip-value">{{ clientIP || '—' }}</div>
      </div>
    </a-card>

    <a-card :title="t('action.create')" class="form-card">
      <a-form :model="{}" layout="vertical" :disabled="loading">
        <a-form-item :label="t('home.sourceMode')">
          <a-radio-group v-model="sourceMode" type="button">
            <a-radio value="current">{{ t('home.sourceCurrent') }}</a-radio>
            <a-radio value="any">{{ t('home.sourceAny') }}</a-radio>
            <a-radio value="manual">{{ t('home.sourceManual') }}</a-radio>
          </a-radio-group>
          <div v-if="sourceMode === 'manual'" style="margin-top: 8px">
            <a-input v-model="manualSource" placeholder="1.2.3.4/32" allow-clear />
          </div>
          <div class="preview">→ {{ sourcePreview }}</div>
        </a-form-item>

        <a-form-item :label="t('home.port')">
          <a-input-number v-model="port" :min="1" :max="65535" :placeholder="t('home.portPlaceholder')" style="max-width: 240px" />
          <div class="preset-list">
            <a-button v-for="p in presets" :key="p.id" size="small" @click="applyPreset(p)">
              {{ p.name }} ({{ p.port }}/{{ p.protocol }})
            </a-button>
          </div>
          <div v-if="!auth.isAdmin && activePreset?.max_duration_sec" class="preview">
            Max {{ Math.floor(activePreset.max_duration_sec / 60) }} min
          </div>
        </a-form-item>

        <a-form-item :label="t('home.protocol')">
          <a-radio-group v-model="protocol" type="button">
            <a-radio value="tcp">TCP</a-radio>
            <a-radio value="udp">UDP</a-radio>
            <a-radio value="both">TCP+UDP</a-radio>
          </a-radio-group>
        </a-form-item>

        <a-form-item :label="t('home.duration')">
          <a-space wrap>
            <a-button
              v-for="opt in durationOptions"
              :key="opt.value"
              :type="durationPreset === opt.value && !customExpire ? 'primary' : 'outline'"
              size="small"
              @click="() => { durationPreset = opt.value; customExpire = undefined }"
            >{{ opt.label }}</a-button>
          </a-space>
          <div style="margin-top: 8px">
            <a-date-picker
              v-model="customExpire"
              show-time
              :placeholder="t('home.durationCustom')"
              style="width: 260px"
            />
          </div>
        </a-form-item>

        <a-form-item :label="t('home.note')">
          <a-textarea v-model="note" :placeholder="t('home.notePlaceholder')" :max-length="255" allow-clear auto-size />
        </a-form-item>

        <a-form-item>
          <a-button type="primary" :loading="loading" @click="submit">
            {{ t('action.submit') }}
          </a-button>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card v-if="lastResult" :title="t('home.submitted')" class="result-card">
      <a-descriptions :column="{ xs: 1, md: 2 }" layout="inline-horizontal" size="small" :data="[
        { label: 'ID', value: String(lastResult.id) },
        { label: t('rules.source'), value: lastResult.source_ip },
        { label: t('rules.port'), value: `${lastResult.port}/${lastResult.protocol}` },
        { label: t('rules.createdAt'), value: dayjs(lastResult.created_at).format('YYYY-MM-DD HH:mm:ss') },
        { label: 'Expire', value: dayjs(lastResult.expire_at).format('YYYY-MM-DD HH:mm:ss') }
      ]" />
    </a-card>
  </div>
</template>

<style scoped>
.home-wrap { display: flex; flex-direction: column; gap: 16px; max-width: 960px; margin: 0 auto; }
.ip-row { display: flex; align-items: center; justify-content: space-between; }
.ip-label { color: var(--color-text-3); font-size: 13px; }
.ip-value { font-size: 22px; font-weight: 600; font-family: ui-monospace, SFMono-Regular, monospace; color: var(--color-primary-6); }
.preview { color: var(--color-text-3); font-size: 12px; margin-top: 4px; font-family: ui-monospace, monospace; }
.preset-list { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
</style>
