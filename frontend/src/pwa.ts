import { registerSW } from 'virtual:pwa-register'
import { Notification } from '@arco-design/web-vue'

// registerPWA wires Service Worker update events to Arco notifications. We
// use manual dismiss so the user isn't interrupted mid-task; they can click
// "Refresh" on their own time.
export function registerPWA() {
  const updateSW = registerSW({
    onNeedRefresh() {
      Notification.info({
        id: 'portpass-sw-update',
        title: 'Update available',
        content: 'A new version is ready. Reload to apply.',
        closable: true,
        duration: 0,
        btnText: 'Reload',
        onClick: () => updateSW(true)
      } as any)
    },
    onOfflineReady() {
      Notification.success({ title: 'PortPass', content: 'Ready to work offline', duration: 2000 })
    }
  })
}
