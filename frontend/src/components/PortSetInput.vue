<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { parsePortSet, formatRange } from '@/utils/portset'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

interface Props {
  modelValue?: string
  placeholder?: string
  disabled?: boolean
  allowEmpty?: boolean
  quick?: Array<{ label: string; value: string }>
  inputClass?: string
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: '',
  placeholder: '22, 80, 8080-8090',
  disabled: false,
  allowEmpty: false,
  quick: () => []
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'validation', ok: boolean, error: string | null): void
}>()

const inner = ref(props.modelValue)

function computeValidity(value: string): { ok: boolean; error: string | null } {
  const p = parsePortSet(value)
  if (!p.ok) return { ok: false, error: p.error }
  const ok = props.allowEmpty || p.count > 0
  return { ok, error: null }
}

// Whenever the parent pushes a new value (e.g. after clicking a preset
// chip), keep the internal ref in sync AND re-emit validation so the
// parent's `submitDisabled` computed re-evaluates. This is the fix for
// the stuck submit button — the old version updated `inner` but never
// broadcast the new validity.
watch(() => props.modelValue, v => {
  if (v === inner.value) return
  inner.value = v
  const r = computeValidity(v)
  emit('validation', r.ok, r.error)
}, { immediate: true })

const parsed = computed(() => parsePortSet(inner.value))
const error = computed(() => (parsed.value.ok ? null : parsed.value.error))
const validity = computed(() => computeValidity(inner.value))

const chips = computed(() => {
  if (!parsed.value.ok) return []
  return parsed.value.ranges.map(r => ({
    key: `${r.from}-${r.to}`,
    label: formatRange(r),
    count: r.to - r.from + 1
  }))
})

const stats = computed(() => {
  if (!parsed.value.ok) return null
  return { entries: parsed.value.entries, count: parsed.value.count }
})

function emitUpdate(value: string) {
  inner.value = value
  emit('update:modelValue', value)
  const r = computeValidity(value)
  emit('validation', r.ok, r.error)
}

function onInput(value: string | number) {
  emitUpdate(String(value))
}

function onBlur() {
  if (parsed.value.ok && parsed.value.canonical && parsed.value.canonical !== inner.value) {
    emitUpdate(parsed.value.canonical)
  }
}

function applyQuick(v: string) {
  const current = inner.value.trim()
  if (!current) {
    emitUpdate(v)
    return
  }
  const combined = `${current},${v}`
  const p = parsePortSet(combined)
  if (p.ok) emitUpdate(p.canonical)
  else emitUpdate(combined)
}
</script>

<template>
  <div class="flex flex-col gap-2 min-w-0">
    <div class="relative">
      <Input
        :model-value="inner"
        :placeholder="placeholder"
        :disabled="disabled"
        :class="[
          'pr-28',
          error ? 'border-destructive focus-visible:ring-destructive/30' : '',
          inputClass
        ].filter(Boolean).join(' ')"
        @update:model-value="onInput"
        @blur="onBlur"
      />
      <span
        v-if="stats && !error"
        class="absolute right-3 top-1/2 -translate-y-1/2 text-[11px] text-muted-foreground pointer-events-none whitespace-nowrap"
      >
        {{ stats.entries }} 段 · {{ stats.count }} 端口
      </span>
    </div>

    <div v-if="error" class="text-xs text-destructive">
      {{ error }}
    </div>
    <div v-else-if="chips.length" class="flex flex-wrap gap-1">
      <Badge v-for="c in chips" :key="c.key" variant="default">
        {{ c.label }}
        <span v-if="c.count > 1" class="opacity-70 ml-0.5 text-[10px]">×{{ c.count }}</span>
      </Badge>
    </div>

    <div v-if="quick && quick.length" class="flex flex-wrap gap-1.5">
      <Button
        v-for="q in quick"
        :key="q.value"
        type="button"
        variant="outline"
        size="sm"
        :disabled="disabled"
        class="h-7 text-xs"
        @click="applyQuick(q.value)"
      >
        {{ q.label }}
      </Button>
    </div>

    <!-- Propagated ok flag for TypeScript to flag unused (kept for parent emit) -->
    <span v-if="false">{{ validity.ok }}</span>
  </div>
</template>
