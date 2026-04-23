<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message, Modal } from '@arco-design/web-vue'
import dayjs from 'dayjs'
import { duplicateRule, extendRule, terminateRule } from '@/api/rules'
import { useRulesStore } from '@/stores/rules'
import { formatRemaining, useNow } from '@/composables/countdown'
import type { Rule } from '@/api/types'

const { t } = useI18n()
const store = useRulesStore()
const now = useNow(1000)

const extendVisible = ref(false)
const extendTarget = ref<Rule | null>(null)
const extendSec = ref<number>(60 * 60)

let refreshTimer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  await store.reload()
  refreshTimer = setInterval(() => store.reload(), 30_000)
})
onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})

async function onTerminate(rule: Rule) {
  Modal.warning({
    title: t('action.terminate'),
    content: t('rules.terminateConfirm'),
    hideCancel: false,
    onOk: async () => {
      await terminateRule(rule.id)
      Message.success(t('msg.ruleTerminated'))
      await store.reload()
    }
  })
}

function openExtend(rule: Rule) {
  extendTarget.value = rule
  extendSec.value = 60 * 60
  extendVisible.value = true
}

async function submitExtend() {
  if (!extendTarget.value) return
  await extendRule(extendTarget.value.id, extendSec.value)
  Message.success(t('msg.ruleExtended'))
  extendVisible.value = false
  await store.reload()
}

async function onDuplicate(rule: Rule) {
  await duplicateRule(rule.id)
  Message.success(t('msg.ruleDuplicated'))
  await store.reload()
}
</script>

<template>
  <a-card :title="t('rules.title')">
    <template #extra>
      <a-button @click="store.reload()">{{ t('action.refresh') }}</a-button>
    </template>

    <a-table
      :loading="store.loading"
      :data="store.active"
      :pagination="false"
      :scroll="{ x: 880 }"
      row-key="id"
    >
      <template #empty>
        <a-empty :description="t('rules.empty')" />
      </template>
      <template #columns>
        <a-table-column :title="t('rules.id')" data-index="id" :width="70" />
        <a-table-column :title="t('rules.source')" data-index="source_ip" />
        <a-table-column :title="t('rules.port')">
          <template #cell="{ record }">{{ record.port }}</template>
        </a-table-column>
        <a-table-column :title="t('rules.protocol')" data-index="protocol" :width="100" />
        <a-table-column :title="t('rules.remaining')" :width="140">
          <template #cell="{ record }">
            <span class="countdown">{{ formatRemaining(record.expire_at, now) }}</span>
          </template>
        </a-table-column>
        <a-table-column :title="t('rules.createdAt')">
          <template #cell="{ record }">{{ dayjs(record.created_at).format('MM-DD HH:mm') }}</template>
        </a-table-column>
        <a-table-column :title="t('rules.note')" data-index="note" ellipsis tooltip />
        <a-table-column :title="t('rules.actions')" :width="260">
          <template #cell="{ record }">
            <a-space>
              <a-button size="small" status="danger" @click="onTerminate(record)">{{ t('action.terminate') }}</a-button>
              <a-button size="small" @click="openExtend(record)">{{ t('action.extend') }}</a-button>
              <a-button size="small" @click="onDuplicate(record)">{{ t('action.duplicate') }}</a-button>
            </a-space>
          </template>
        </a-table-column>
      </template>
    </a-table>

    <a-modal
      v-model:visible="extendVisible"
      :title="t('rules.extendDialog')"
      @ok="submitExtend"
    >
      <a-form-item :label="t('rules.extendAmount')">
        <a-radio-group v-model="extendSec">
          <a-radio :value="15 * 60">15m</a-radio>
          <a-radio :value="60 * 60">1h</a-radio>
          <a-radio :value="4 * 60 * 60">4h</a-radio>
          <a-radio :value="12 * 60 * 60">12h</a-radio>
        </a-radio-group>
      </a-form-item>
    </a-modal>
  </a-card>
</template>

<style scoped>
.countdown { font-family: ui-monospace, SFMono-Regular, monospace; font-weight: 600; color: var(--color-primary-6); }
</style>
