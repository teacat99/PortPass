<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import dayjs from 'dayjs'
import { listHistory } from '@/api/rules'
import { useBreakpoint } from '@/composables/useBreakpoint'
import type { Rule } from '@/api/types'

const { t } = useI18n()
const { isMobile } = useBreakpoint()

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

function statusColor(s: string) {
  return s === 'failed' ? 'red' : s === 'revoked' ? 'orange' : 'gray'
}

onMounted(reload)
</script>

<template>
  <a-card :title="t('history.title')">
    <template #extra>
      <a-button type="primary" @click="reload">{{ t('action.refresh') }}</a-button>
    </template>

    <!-- Filter bar: inline on desktop, stacked on mobile (see responsive.css
         for the .portpass-filter-bar rule). -->
    <div class="portpass-filter-bar filter-bar">
      <a-input-number
        v-model="filter.port"
        :min="1"
        :max="65535"
        :placeholder="t('rules.port')"
        allow-clear
        style="width: 140px"
      />
      <a-input v-model="filter.ip" placeholder="IP" allow-clear style="width: 180px" />
      <a-select v-model="filter.status" :placeholder="t('history.status')" allow-clear style="width: 160px">
        <a-option value="expired">{{ t('status.expired') }}</a-option>
        <a-option value="revoked">{{ t('status.revoked') }}</a-option>
        <a-option value="failed">{{ t('status.failed') }}</a-option>
      </a-select>
      <a-range-picker v-model="filter.range" show-time style="min-width: 280px; flex: 1 1 280px" />
      <a-button type="primary" @click="reload">{{ t('action.search') }}</a-button>
    </div>

    <a-table
      v-if="!isMobile"
      :loading="loading"
      :data="rules"
      :scroll="{ x: 880 }"
      :pagination="{ pageSize: 20, total }"
    >
      <template #columns>
        <a-table-column title="ID" data-index="id" :width="70" />
        <a-table-column :title="t('history.status')" data-index="status" :width="100">
          <template #cell="{ record }">
            <a-tag :color="statusColor(record.status)">{{ t(`status.${record.status}`) }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column :title="t('rules.source')" data-index="source_ip" />
        <a-table-column :title="t('rules.port')">
          <template #cell="{ record }">{{ record.port }}/{{ record.protocol }}</template>
        </a-table-column>
        <a-table-column :title="t('history.actor')" data-index="created_ip" />
        <a-table-column title="User" data-index="created_by" :width="120" />
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

    <div v-else class="portpass-card-list">
      <a-empty v-if="!rules.length && !loading" />
      <div v-for="r in rules" :key="r.id" class="portpass-card">
        <h4>
          #{{ r.id }}
          <a-tag :color="statusColor(r.status)" size="small">{{ t(`status.${r.status}`) }}</a-tag>
        </h4>
        <div class="row"><span class="label">{{ t('rules.port') }}</span><span class="mono">{{ r.port }}/{{ r.protocol }}</span></div>
        <div class="row"><span class="label">{{ t('rules.source') }}</span><span class="mono">{{ r.source_ip }}</span></div>
        <div class="row"><span class="label">{{ t('history.actor') }}</span><span class="mono">{{ r.created_ip }}</span></div>
        <div class="row"><span class="label">User</span><span>{{ r.created_by || '-' }}</span></div>
        <div class="row"><span class="label">{{ t('rules.createdAt') }}</span><span>{{ dayjs(r.created_at).format('MM-DD HH:mm') }}</span></div>
        <div class="row"><span class="label">{{ t('history.duration') }}</span><span>{{ durationOf(r) }}</span></div>
        <div v-if="r.note" class="row"><span class="label">{{ t('rules.note') }}</span><span>{{ r.note }}</span></div>
      </div>
    </div>
  </a-card>
</template>

<style scoped>
.filter-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.mono { font-family: ui-monospace, SFMono-Regular, monospace; }
</style>
