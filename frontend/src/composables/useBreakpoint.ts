import { onMounted, onUnmounted, readonly, ref } from 'vue'

// useBreakpoint watches window width and exposes two booleans used across
// the app to switch between tablet/desktop table layouts and mobile card
// layouts. Centralising the breakpoints avoids subtly different media
// queries drifting between views.
//
// - isNarrow : <= 640px (single-column forms, full-screen modals)
// - isMobile : <= 768px (table -> card transformation, sidebar collapses)
export function useBreakpoint() {
  const width = ref<number>(typeof window !== 'undefined' ? window.innerWidth : 1024)
  const isMobile = ref<boolean>(width.value <= 768)
  const isNarrow = ref<boolean>(width.value <= 640)

  function update() {
    width.value = window.innerWidth
    isMobile.value = width.value <= 768
    isNarrow.value = width.value <= 640
  }

  onMounted(() => {
    update()
    window.addEventListener('resize', update, { passive: true })
  })
  onUnmounted(() => {
    window.removeEventListener('resize', update)
  })

  return {
    width: readonly(width),
    isMobile: readonly(isMobile),
    isNarrow: readonly(isNarrow)
  }
}
