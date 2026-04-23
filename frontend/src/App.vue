<script setup lang="ts">
import { computed } from 'vue'
import zhCN from '@arco-design/web-vue/es/locale/lang/zh-cn'
import enUS from '@arco-design/web-vue/es/locale/lang/en-us'
import { useI18n } from 'vue-i18n'

const { locale } = useI18n()
const arcoLocale = computed(() => (locale.value === 'en-US' ? enUS : zhCN))
</script>

<template>
  <a-config-provider :locale="arcoLocale">
    <router-view />
  </a-config-provider>
</template>
