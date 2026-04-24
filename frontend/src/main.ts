import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ArcoVue from '@arco-design/web-vue'
import ArcoVueIcon from '@arco-design/web-vue/es/icon'
import '@arco-design/web-vue/dist/arco.css'
import './assets/tokens.css'
import './assets/responsive.css'

import App from './App.vue'
import router from './router'
import i18n from './i18n'
import { registerPWA } from './pwa'
import { useThemeStore } from './stores/theme'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(i18n)
app.use(ArcoVue)
app.use(ArcoVueIcon)

// Initialise theme before mounting so the first paint already matches the
// user's preferred / persisted scheme — avoids the white flash.
useThemeStore().init()

app.mount('#app')

registerPWA()
