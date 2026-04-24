<script setup lang="ts">
import { ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { IconCopy, IconCheck } from '@arco-design/web-vue/es/icon'

const props = defineProps<{
  value: string | number
  /** Override what gets put on the clipboard (vs displayed). */
  copyValue?: string
  /** Render value in monospace */
  mono?: boolean
  /** Hide the displayed value, show only the copy icon. */
  iconOnly?: boolean
}>()

const copied = ref(false)

async function copy() {
  const v = String(props.copyValue ?? props.value ?? '')
  if (!v) return
  try {
    await navigator.clipboard.writeText(v)
    copied.value = true
    Message.success({ content: '已复制', duration: 1200 })
    setTimeout(() => { copied.value = false }, 1500)
  } catch {
    Message.error('复制失败，请手动选择文本')
  }
}
</script>

<template>
  <span class="pp-copyable" :class="{ 'mono': mono }" @click="copy">
    <span v-if="!iconOnly" class="pp-copyable-text">{{ value }}</span>
    <component :is="copied ? IconCheck : IconCopy" class="pp-copyable-icon" />
  </span>
</template>

<style scoped>
.pp-copyable {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 6px;
  transition: background 0.15s ease;
  user-select: text;
}
.pp-copyable:hover {
  background: var(--pp-surface-sunken);
}
.pp-copyable.mono .pp-copyable-text {
  font-family: ui-monospace, SFMono-Regular, monospace;
}
.pp-copyable-icon {
  font-size: 13px;
  color: var(--color-text-3);
  opacity: 0.6;
}
.pp-copyable:hover .pp-copyable-icon {
  opacity: 1;
  color: var(--pp-brand-6);
}
</style>
