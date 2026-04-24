<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ status: string }>()
const { t } = useI18n()

// Single source of truth for the status colour palette so every page that
// shows a rule status uses the exact same vocabulary.
const palette: Record<string, { color: string; label: string; dot: string }> = {
  active:   { color: 'green',   label: 'status.active',   dot: 'var(--pp-status-active)' },
  pending:  { color: 'orange',  label: 'status.pending',  dot: 'var(--pp-status-pending)' },
  expired:  { color: 'gray',    label: 'status.expired',  dot: 'var(--pp-status-expired)' },
  revoked:  { color: 'red',     label: 'status.revoked',  dot: 'var(--pp-status-revoked)' },
  failed:   { color: 'red',     label: 'status.failed',   dot: 'var(--pp-status-failed)' }
}

const meta = computed(() => palette[props.status] ?? { color: 'gray', label: props.status, dot: 'var(--color-text-3)' })
</script>

<template>
  <a-tag :color="meta.color" size="small" class="pp-status-tag">
    <span class="pp-status-dot" :style="{ background: meta.dot }"></span>
    {{ meta.label.startsWith('status.') ? t(meta.label) : meta.label }}
  </a-tag>
</template>

<style scoped>
.pp-status-tag { display: inline-flex; align-items: center; gap: 6px; }
.pp-status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  display: inline-block;
}
</style>
