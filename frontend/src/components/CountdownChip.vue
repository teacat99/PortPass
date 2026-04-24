<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import dayjs from 'dayjs'

// CountdownChip renders a compact pill that shows the remaining time of a
// rule until expire_at. The colour transitions through 4 states:
//   - >30%   green  (calm)
//   - >10%   amber  (heads-up)
//   - >0%    red    (critical)
//   - 0/past gray   (expired)
//
// We accept either an ISO string or a Date and tick once per second from
// onMounted to onUnmounted to keep the table cheap.

const props = defineProps<{
  expireAt: string | Date
  /** Created-at to compute the % remaining ratio for the colour ramp. */
  createdAt?: string | Date
  /** Visual size: default | small */
  size?: 'default' | 'small'
}>()

const now = ref<number>(Date.now())
let timer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  timer = setInterval(() => { now.value = Date.now() }, 1000)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
})

const expireMs = computed(() => dayjs(props.expireAt).valueOf())
const createdMs = computed(() => props.createdAt ? dayjs(props.createdAt).valueOf() : now.value - 60_000)
const remainingMs = computed(() => Math.max(0, expireMs.value - now.value))
const totalMs = computed(() => Math.max(1, expireMs.value - createdMs.value))
const ratio = computed(() => remainingMs.value / totalMs.value)

const tone = computed<'ok' | 'warn' | 'danger' | 'expired'>(() => {
  if (remainingMs.value <= 0) return 'expired'
  if (ratio.value > 0.3) return 'ok'
  if (ratio.value > 0.1) return 'warn'
  return 'danger'
})

const label = computed(() => {
  const ms = remainingMs.value
  if (ms <= 0) return '已过期'
  const s = Math.floor(ms / 1000)
  if (s < 60) return s + ' 秒'
  const m = Math.floor(s / 60)
  if (m < 60) return m + ' 分 ' + (s % 60).toString().padStart(2, '0')
  const h = Math.floor(m / 60)
  if (h < 24) return h + ' 时 ' + (m % 60).toString().padStart(2, '0') + ' 分'
  const d = Math.floor(h / 24)
  return d + ' 天 ' + (h % 24) + ' 时'
})
</script>

<template>
  <span class="pp-cd" :class="['tone-' + tone, size === 'small' ? 'sm' : '']">
    <span class="pp-cd-bar" :style="{ width: (Math.min(100, ratio * 100)) + '%' }"></span>
    <span class="pp-cd-label">{{ label }}</span>
  </span>
</template>

<style scoped>
.pp-cd {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 10px;
  min-width: 90px;
  border-radius: 999px;
  font-variant-numeric: tabular-nums;
  font-size: 12px;
  font-weight: 500;
  overflow: hidden;
  background: var(--pp-surface-sunken);
  color: var(--color-text-2);
  border: 1px solid var(--pp-border);
}
.pp-cd.sm { min-width: 72px; padding: 1px 8px; font-size: 11px; }
.pp-cd-bar {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  background: currentColor;
  opacity: 0.16;
  transition: width 1s linear;
  z-index: 0;
}
.pp-cd-label { position: relative; z-index: 1; }

.tone-ok { color: var(--pp-status-active); border-color: rgba(0, 180, 42, 0.4); }
.tone-warn { color: var(--pp-status-pending); border-color: rgba(255, 125, 0, 0.4); }
.tone-danger {
  color: var(--pp-status-failed);
  border-color: rgba(245, 63, 63, 0.4);
  animation: pp-cd-pulse 1.4s ease-in-out infinite;
}
.tone-expired { color: var(--color-text-3); }

@keyframes pp-cd-pulse {
  0%, 100% { box-shadow: 0 0 0 0 rgba(245, 63, 63, 0); }
  50% { box-shadow: 0 0 0 4px rgba(245, 63, 63, 0.18); }
}
</style>
