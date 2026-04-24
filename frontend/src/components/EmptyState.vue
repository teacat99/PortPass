<script setup lang="ts">
defineProps<{
  title?: string
  description?: string
  /** Lucide-style emoji or single character used as a soft illustration. */
  icon?: string
}>()
</script>

<template>
  <div class="pp-empty">
    <div class="pp-empty-illu">{{ icon ?? '✨' }}</div>
    <div class="pp-empty-title">{{ title }}</div>
    <div v-if="description" class="pp-empty-desc">{{ description }}</div>
    <div v-if="$slots.action" class="pp-empty-action">
      <slot name="action" />
    </div>
  </div>
</template>

<style scoped>
.pp-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px 16px;
  color: var(--color-text-3);
  gap: 8px;
}
.pp-empty-illu {
  width: 72px;
  height: 72px;
  border-radius: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 32px;
  background: var(--pp-surface-sunken);
  color: var(--pp-brand-6);
  margin-bottom: 6px;
}
.pp-empty-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-1);
}
.pp-empty-desc {
  max-width: 320px;
  text-align: center;
  font-size: 13px;
  line-height: 1.6;
}
.pp-empty-action {
  margin-top: 12px;
}
</style>
