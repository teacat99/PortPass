<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import dayjs from 'dayjs'
import { listHistory } from '@/api/rules'
import type { Rule } from '@/api/types'

const { t } = useI18n()

const rules = ref<Rule[]>([])
const total = ref(0)
const loading = ref(false)

const filter = ref({
  port: undefined as number | undefined,
  ip: '' as string,
  status: '' as string,
  range: [] as string[]
})

async function reload() {
  loading.value = true
  try {
    const q: Record<string, string | number> = { limit: 200 }
    if (filter.value.port) q.port = filter.value.port
    if (filter.value.ip) q.ip = filter.value.ip
    if (filter.value.status) q.status = filter.value.status
    if (filter.value.range?.length === 2) {
      q.from = dayjs(filter.value.range[0]).toISOString()
      q.to = dayjs(filter.value.range[1]).toISOString()
    }
    const resp = await listHistory(q)
    rules.value = resp.rules
    total.value = resp.total
  } finally {
    loading.value = false
  }
}

function durationOf(r: Rule): string {
  const end = r.terminated_at ? new Date(r.terminated_at).getTime() : new Date(r.expire_at).getTime()
  const start = new Date(r.created_at).getTime()
  const s = Math.max(0, Math.floor((end - start) / 1000))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  return h > 0 ? `${h}h${m}m` : `${m}m`
}

onMounted(reload)
</script>

<template>
  <a-card :title="t('history.title')">
    <template #extra>
      <a-button type="primary" @click="reload">{{ t('action.refresh') }}</a-button>
    </template>

    <a-form :model="filter" layout="inline" style="margin-bottom: 12px">
      <a-form-item :label="t('rules.port')">
        <a-input-number v-model="filter.port" :min="1" :max="65535" allow-clear style="width: 140px" />
      </a-form-item>
      <a-form-item label="IP">
        <a-input v-model="filter.ip" allow-clear style="width: 180px" />
      </a-form-item>
      <a-form-item :label="t('history.status')">
        <a-select v-model="filter.status" allow-clear style="width: 160px">
          <a-option value="expired">{{ t('status.expired') }}</a-option>
          <a-option value="revoked">{{ t('status.revoked') }}</a-option>
          <a-option value="failed">{{ t('status.failed') }}</a-option>
        </a-select>
      </a-form-item>
      <a-form-item :label="t('history.filterFrom')">
        <a-range-picker v-model="filter.range" show-time style="width: 360px" />
      </a-form-item>
      <a-form-item>
        <a-button type="primary" @click="reload">{{ t('action.search') }}</a-button>
      </a-form-item>
    </a-form>

    <a-table :loading="loading" :data="rules" :scroll="{ x: 880 }" :pagination="{ pageSize: 20, total }">
      <template #columns>
        <a-table-column title="ID" data-index="id" :width="70" />
        <a-table-column :title="t('history.status')" data-index="status" :width="100">
          <template #cell="{ record }">
            <a-tag :color="record.status === 'failed' ? 'red' : record.status === 'revoked' ? 'orange' : 'gray'">
              {{ t(`status.${record.status}`) }}
            </a-tag>
          </template>
        </a-table-column>
        <a-table-column :title="t('rules.source')" data-index="source_ip" />
        <a-table-column :title="t('rules.port')">
          <template #cell="{ record }">{{ record.port }}/{{ record.protocol }}</template>
        </a-table-column>
        <a-table-column :title="t('history.actor')" data-index="created_ip" />
        <a-table-column :title="t('rules.createdAt')">
          <template #cell="{ record }">{{ dayjs(record.created_at).format('MM-DD HH:mm') }}</template>
        </a-table-column>
        <a-table-column :title="t('history.terminatedAt')">
          <template #cell="{ record }">{{ record.terminated_at ? dayjs(record.terminated_at).format('MM-DD HH:mm') : '—' }}</template>
        </a-table-column>
        <a-table-column :title="t('history.duration')" :width="100">
          <template #cell="{ record }">{{ durationOf(record) }}</template>
        </a-table-column>
        <a-table-column :title="t('rules.note')" data-index="note" ellipsis tooltip />
      </template>
    </a-table>
  </a-card>
</template>
