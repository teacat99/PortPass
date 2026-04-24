<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import dayjs from 'dayjs'
import { IconRefresh, IconSearch } from '@arco-design/web-vue/es/icon'
import { listHistory } from '@/api/rules'
import { useBreakpoint } from '@/composables/useBreakpoint'
import type { Rule } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import StatusTag from '@/components/StatusTag.vue'
import CopyableText from '@/components/CopyableText.vue'

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
  if (s < 60) return s + 's'
  const m = Math.floor(s / 60)
  if (m < 60) return m + 'm'
  const h = Math.floor(m / 60)
  return h + 'h' + (m % 60) + 'm'
}

function reset() {
  filter.value = { port: undefined, ip: '', status: '', range: [] }
  reload()
}

onMounted(reload)
</script>

<template>
  <div class="pp-page history-wrap">
    <header class="pp-card-head">
      <div>
        <h1 class="pp-page-title">{{ t('history.title') }}</h1>
        <p class="pp-page-sub">查看过去的全部规则操作（含到期、撤销、失败）</p>
      </div>
      <div class="pp-head-actions">
        <a-button @click="reload" :loading="loading">
          <template #icon><IconRefresh /></template>
          {{ t('action.refresh') }}
        </a-button>
      </div>
    </header>

    <a-card class="filter-card">
      <div class="filter-bar">
        <a-input-number
          v-model="filter.port"
          :min="1"
          :max="65535"
          :placeholder="t('rules.port')"
          allow-clear
          hide-button
          class="f-port"
        />
        <a-input v-model="filter.ip" placeholder="IP / CIDR" allow-clear class="f-ip" />
        <a-select v-model="filter.status" :placeholder="t('history.status')" allow-clear class="f-status">
          <a-option value="expired">{{ t('status.expired') }}</a-option>
          <a-option value="revoked">{{ t('status.revoked') }}</a-option>
          <a-option value="failed">{{ t('status.failed') }}</a-option>
        </a-select>
        <a-range-picker v-model="filter.range" show-time class="f-range" />
        <div class="f-actions">
          <a-button @click="reset">重置</a-button>
          <a-button type="primary" @click="reload">
            <template #icon><IconSearch /></template>
            {{ t('action.search') }}
          </a-button>
        </div>
      </div>
    </a-card>

    <a-card class="list-card">
      <div v-if="loading && !rules.length" class="loading-skel">
        <a-skeleton :animation="true" v-for="i in 4" :key="i">
          <a-skeleton-line :rows="2" :widths="['50%', '90%']" />
        </a-skeleton>
      </div>

      <EmptyState
        v-else-if="!rules.length"
        icon="📜"
        title="暂无符合条件的历史"
        description="尝试调整时间范围或清空筛选条件，重新搜索。"
      >
        <template #action>
          <a-button @click="reset">清空筛选</a-button>
        </template>
      </EmptyState>

      <a-table
        v-else-if="!isMobile"
        :data="rules"
        :scroll="{ x: 1000 }"
        :pagination="{ pageSize: 20, total, showTotal: true }"
        :hoverable="true"
        :bordered="false"
        size="medium"
        row-key="id"
      >
        <template #columns>
          <a-table-column title="ID" :width="80">
            <template #cell="{ record }">
              <CopyableText :value="record.id" mono />
            </template>
          </a-table-column>
          <a-table-column :title="t('history.status')" :width="100">
            <template #cell="{ record }">
              <StatusTag :status="record.status" />
            </template>
          </a-table-column>
          <a-table-column :title="t('rules.source')" :width="170">
            <template #cell="{ record }">
              <CopyableText :value="record.source_ip" mono />
            </template>
          </a-table-column>
          <a-table-column :title="t('rules.port')" :width="120">
            <template #cell="{ record }">
              <span class="port-cell">
                <code>:{{ record.port }}</code>
                <a-tag size="small" :color="record.protocol === 'udp' ? 'purple' : 'arcoblue'">{{ record.protocol.toUpperCase() }}</a-tag>
              </span>
            </template>
          </a-table-column>
          <a-table-column :title="t('history.actor')" :width="160">
            <template #cell="{ record }">
              <CopyableText :value="record.created_ip" mono />
            </template>
          </a-table-column>
          <a-table-column title="用户" :width="110">
            <template #cell="{ record }">
              <a-tag size="small" color="gray">{{ record.created_by || '-' }}</a-tag>
            </template>
          </a-table-column>
          <a-table-column :title="t('rules.createdAt')" :width="140">
            <template #cell="{ record }">
              <a-tooltip :content="dayjs(record.created_at).format('YYYY-MM-DD HH:mm:ss')">
                <span class="muted">{{ dayjs(record.created_at).format('MM-DD HH:mm') }}</span>
              </a-tooltip>
            </template>
          </a-table-column>
          <a-table-column :title="t('history.terminatedAt')" :width="140">
            <template #cell="{ record }">
              <span v-if="record.terminated_at" class="muted">{{ dayjs(record.terminated_at).format('MM-DD HH:mm') }}</span>
              <span v-else class="muted">—</span>
            </template>
          </a-table-column>
          <a-table-column :title="t('history.duration')" :width="90">
            <template #cell="{ record }"><span class="mono">{{ durationOf(record) }}</span></template>
          </a-table-column>
          <a-table-column :title="t('rules.note')" data-index="note" ellipsis tooltip />
        </template>
      </a-table>

      <div v-else class="m-list">
        <div v-for="r in rules" :key="r.id" class="m-card">
          <div class="m-card-head">
            <span class="m-port">
              <code>:{{ r.port }}</code>
              <a-tag size="small" :color="r.protocol === 'udp' ? 'purple' : 'arcoblue'">{{ r.protocol.toUpperCase() }}</a-tag>
            </span>
            <StatusTag :status="r.status" />
          </div>
          <div class="m-grid">
            <div class="m-cell"><span class="muted">{{ t('rules.source') }}</span>
              <CopyableText :value="r.source_ip" mono />
            </div>
            <div class="m-cell"><span class="muted">{{ t('history.actor') }}</span>
              <CopyableText :value="r.created_ip" mono />
            </div>
            <div class="m-cell"><span class="muted">{{ t('rules.createdAt') }}</span>
              <span class="mono">{{ dayjs(r.created_at).format('MM-DD HH:mm') }}</span>
            </div>
            <div class="m-cell"><span class="muted">{{ t('history.duration') }}</span>
              <span class="mono">{{ durationOf(r) }}</span>
            </div>
            <div class="m-cell"><span class="muted">用户</span>
              <span>{{ r.created_by || '-' }}</span>
            </div>
            <div class="m-cell"><span class="muted">ID</span>
              <CopyableText :value="r.id" mono />
            </div>
          </div>
          <div v-if="r.note" class="m-note">📝 {{ r.note }}</div>
        </div>
      </div>
    </a-card>
  </div>
</template>

<style scoped>
.history-wrap { display: flex; flex-direction: column; gap: 16px; }
.pp-card-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
  flex-wrap: wrap;
}
.pp-page-title { margin: 0; font-size: 20px; font-weight: 600; color: var(--color-text-1); }
.pp-page-sub { margin: 4px 0 0; color: var(--color-text-3); font-size: 13px; }
.pp-head-actions { display: flex; gap: 8px; }

.filter-card { border-radius: 14px; }
.filter-bar {
  display: grid;
  grid-template-columns: 120px 180px 160px 1fr auto;
  gap: 10px;
  align-items: center;
}
.f-port { width: 100%; }
.f-ip, .f-status { width: 100%; }
.f-range { width: 100%; min-width: 240px; }
.f-actions { display: flex; gap: 8px; }

.list-card { border-radius: 14px; }
.list-card :deep(.arco-card-body) { padding: 0; }
.list-card :deep(.arco-table-th) { background: var(--pp-surface-soft); font-weight: 600; }
.loading-skel { padding: 24px; display: flex; flex-direction: column; gap: 16px; }

.port-cell { display: inline-flex; align-items: center; gap: 6px; }
.port-cell code { font-family: ui-monospace, monospace; font-weight: 600; color: var(--color-text-1); }
.muted { color: var(--color-text-3); font-size: 12px; }
.mono { font-family: ui-monospace, monospace; }

.m-list { padding: 12px; display: flex; flex-direction: column; gap: 10px; }
.m-card {
  background: var(--pp-surface);
  border: 1px solid var(--pp-border);
  border-radius: 12px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.m-card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.m-port { display: inline-flex; align-items: center; gap: 6px; font-weight: 600; }
.m-port code { font-family: ui-monospace, monospace; font-size: 15px; }
.m-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 16px;
  font-size: 13px;
}
.m-cell { display: flex; flex-direction: column; gap: 2px; }
.m-cell .muted { font-size: 11px; }
.m-note {
  background: var(--pp-surface-sunken);
  padding: 8px 10px;
  border-radius: 6px;
  font-size: 12px;
  color: var(--color-text-2);
}

@media (max-width: 768px) {
  .filter-bar { grid-template-columns: 1fr 1fr; }
  .f-range { grid-column: 1 / -1; }
  .f-actions { grid-column: 1 / -1; justify-content: flex-end; }
}
</style>
