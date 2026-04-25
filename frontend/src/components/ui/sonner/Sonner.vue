<script setup lang="ts">
import { Toaster, type ToasterProps } from 'vue-sonner'
import { useThemeStore } from '@/stores/theme'
import { computed } from 'vue'

// Thin wrapper around vue-sonner's <Toaster /> so the rest of the app can
// always import from @/components/ui/sonner (matches shadcn-vue layout).
// The theme is mirrored from our theme store so toasts adopt dark colours
// in sync with the rest of the UI.
const props = defineProps<ToasterProps>()
const theme = useThemeStore()
const resolvedTheme = computed<ToasterProps['theme']>(() =>
  theme.isDark ? 'dark' : 'light'
)
</script>

<template>
  <Toaster
    v-bind="props"
    :theme="resolvedTheme"
    position="top-left"
    rich-colors
    close-button
    :toast-options="{
      classes: {
        toast: 'rounded-md border-border shadow-float',
      }
    }"
  />
</template>
