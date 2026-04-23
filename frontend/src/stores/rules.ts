import { defineStore } from 'pinia'
import { ref } from 'vue'
import { listRules } from '@/api/rules'
import type { Rule } from '@/api/types'

export const useRulesStore = defineStore('rules', () => {
  const active = ref<Rule[]>([])
  const loading = ref(false)

  async function reload() {
    loading.value = true
    try {
      const { rules } = await listRules({ status: 'active,pending', limit: 500 })
      active.value = rules
    } finally {
      loading.value = false
    }
  }

  return { active, loading, reload }
})
