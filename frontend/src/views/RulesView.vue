<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Message, Modal } from '@arco-design/web-vue'
import dayjs from 'dayjs'
import { IconRefresh, IconPlus, IconCloseCircle, IconClockCircle, IconCopy } from '@arco-design/web-vue/es/icon'
import { duplicateRule, extendRule, terminateRule } from '@/api/rules'
import { useRulesStore } from '@/stores/rules'
import { useBreakpoint } from '@/composables/useBreakpoint'
import type { Rule } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import CountdownChip from '@/components/CountdownChip.vue'
import CopyableText from '@/components/CopyableText.vue'

const { t } = useI18n()
const router = useRouter()
const store = useRulesStore()
const { isMobile } = useBreakpoint()

const extendVisible = ref(false)
const extendTarget = ref<Rule | null>(null)
const extendSec = ref<number>(60 * 60)

const search = ref('')

let refreshTimer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  await store.reload()
  refreshTimer = setInterval(() => store.reload(), 30_000)
})
onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return store.active
  return store.active.filter((r) =>
    String(r.port).includes(q)
    || r.source_ip.toLowerCase().includes(q)
    || (r.note ?? '').toLowerCase().includes(q)
    || (r.created_by ?? '').toLowerCase().includes(q)
  )
})

async function onTerminate(rule: Rule) {
  Modal.warning({
    title: t('action.terminate'),
    content: t('rules.terminateConfirm'),
    hideCancel: false,
    okButtonProps: { status: 'danger' },
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
  <div class="pp-page rules-wrap">
    <header class="pp-card-head">
      <div>
        <h1 class="pp-page-title">{{ t('rules.title') }}</h1>
        <p class="pp-page-sub">
          共 <strong>{{ store.active.length }}</strong> 条生效中
          <span v-if="search && filtered.length !== store.active.length">
            · 当前显示 {{ filtered.length }} 条
          </span>
        </p>
      </div>
      <div class="pp-head-actions">
        <a-input-search
          v-model="search"
          :placeholder="'按端口 / IP / 备注 / 用户名搜索'"
          allow-clear
          class="search-box"
        />
        <a-button @click="store.reload()" :loading="store.loading">
          <template #icon><IconRefresh /></template>
          {{ t('action.refresh') }}
        </a-button>
        <a-button type="primary" @click="router.push({ name: 'home' })">
          <template #icon><IconPlus /></template>
          {{ t('action.create') }}
        </a-button>
      </div>
    </header>

    <a-card class="rules-card">
      <!-- Loading skeleton -->
      <div v-if="store.loading && !store.active.length" class="loading-skel">
        <a-skeleton :animation="true" v-for="i in 3" :key="i">
          <a-skeleton-line :rows="2" :widths="['60%', '90%']" />
        </a-skeleton>
      </div>

      <!-- Empty state -->
      <EmptyState
        v-else-if="!filtered.length && !search"
        icon="🛡️"
        title="暂时没有生效中的规则"
        description="你可以从首页快速创建一条临时端口规则，到期后会自动撤销。"
      >
        <template #action>
          <a-button type="primary" @click="router.push({ name: 'home' })">
            <template #icon><IconPlus /></template>
            {{ t('action.create') }}
          </a-button>
        </template>
      </EmptyState>

      <EmptyState
        v-else-if="!filtered.length && search"
        icon="🔍"
        title="没有匹配的规则"
        :description="`没有规则匹配 “${search}”，试试换个关键词。`"
      />

      <!-- Desktop table -->
      <a-table
        v-else-if="!isMobile"
        :data="filtered"
        :pagination="false"
        :scroll="{ x: 980 }"
        row-key="id"
        :bordered="false"
        :hoverable="true"
        size="medium"
      >
        <template #columns>
          <a-table-column :title="t('rules.id')" :width="76" data-index="id">
            <template #cell="{ record }">
              <CopyableText :value="record.id" mono />
            </template>
          </a-table-column>
          <a-table-column :title="t('rules.source')" data-index="source_ip" :width="170">
            <template #cell="{ record }">
              <CopyableText :value="record.source_ip" mono />
            </template>
          </a-table-column>
          <a-table-column :title="t('rules.port') + ' / ' + t('rules.protocol')" :width="140">
            <template #cell="{ record }">
              <span class="port-cell">
                <code>:{{ record.port }}</code>
                <a-tag size="small" :color="record.protocol === 'udp' ? 'purple' : 'arcoblue'">
                  {{ record.protocol.toUpperCase() }}
                </a-tag>
              </span>
            </template>
          </a-table-column>
          <a-table-column :title="t('rules.remaining')" :width="160">
            <template #cell="{ record }">
              <CountdownChip :expire-at="record.expire_at" :created-at="record.created_at" />
            </template>
          </a-table-column>
          <a-table-column :title="t('rules.createdAt')" :width="140">
            <template #cell="{ record }">
              <a-tooltip :content="dayjs(record.created_at).format('YYYY-MM-DD HH:mm:ss')">
                <span class="muted">{{ dayjs(record.created_at).format('MM-DD HH:mm') }}</span>
              </a-tooltip>
            </template>
          </a-table-column>
          <a-table-column title="用户" data-index="created_by" :width="120">
            <template #cell="{ record }">
              <a-tag size="small" color="gray">{{ record.created_by || '-' }}</a-tag>
            </template>
          </a-table-column>
          <a-table-column :title="t('rules.note')" data-index="note" ellipsis tooltip />
          <a-table-column :title="t('rules.actions')" :width="220" align="right" fixed="right">
            <template #cell="{ record }">
              <a-space :size="4">
                <a-tooltip :content="t('action.extend')">
                  <a-button size="small" type="text" @click="openExtend(record)">
                    <template #icon><IconClockCircle /></template>
                  </a-button>
                </a-tooltip>
                <a-tooltip :content="t('action.duplicate')">
                  <a-button size="small" type="text" @click="onDuplicate(record)">
                    <template #icon><IconCopy /></template>
                  </a-button>
                </a-tooltip>
                <a-tooltip :content="t('action.terminate')">
                  <a-button size="small" type="text" status="danger" @click="onTerminate(record)">
                    <template #icon><IconCloseCircle /></template>
                  </a-button>
                </a-tooltip>
              </a-space>
            </template>
          </a-table-column>
        </template>
      </a-table>

      <!-- Mobile card list -->
      <div v-else class="m-list">
        <div v-for="r in filtered" :key="r.id" class="m-card">
          <div class="m-card-head">
            <span class="m-port"><code>:{{ r.port }}</code>
              <a-tag size="small" :color="r.protocol === 'udp' ? 'purple' : 'arcoblue'">
                {{ r.protocol.toUpperCase() }}
              </a-tag>
            </span>
            <CountdownChip :expire-at="r.expire_at" :created-at="r.created_at" size="small" />
          </div>
          <div class="m-grid">
            <div class="m-cell"><span class="muted">{{ t('rules.source') }}</span>
              <CopyableText :value="r.source_ip" mono />
            </div>
            <div class="m-cell"><span class="muted">{{ t('rules.id') }}</span>
              <CopyableText :value="r.id" mono />
            </div>
            <div class="m-cell"><span class="muted">{{ t('rules.createdAt') }}</span>
              <span class="mono">{{ dayjs(r.created_at).format('MM-DD HH:mm') }}</span>
            </div>
            <div class="m-cell"><span class="muted">用户</span>
              <span>{{ r.created_by || '-' }}</span>
            </div>
          </div>
          <div v-if="r.note" class="m-note">📝 {{ r.note }}</div>
          <div class="m-actions">
            <a-button size="small" @click="openExtend(r)">
              <template #icon><IconClockCircle /></template>
              {{ t('action.extend') }}
            </a-button>
            <a-button size="small" @click="onDuplicate(r)">
              <template #icon><IconCopy /></template>
              {{ t('action.duplicate') }}
            </a-button>
            <a-button size="small" status="danger" @click="onTerminate(r)">
              <template #icon><IconCloseCircle /></template>
              {{ t('action.terminate') }}
            </a-button>
          </div>
        </div>
      </div>
    </a-card>

    <a-modal v-model:visible="extendVisible" :title="t('rules.extendDialog')" @ok="submitExtend" unmount-on-close>
      <a-form-item :label="t('rules.extendAmount')">
        <a-radio-group v-model="extendSec" type="button">
          <a-radio :value="15 * 60">15m</a-radio>
          <a-radio :value="60 * 60">1h</a-radio>
          <a-radio :value="4 * 60 * 60">4h</a-radio>
          <a-radio :value="12 * 60 * 60">12h</a-radio>
        </a-radio-group>
      </a-form-item>
    </a-modal>
  </div>
</template>

<style scoped>
.rules-wrap { display: flex; flex-direction: column; gap: 16px; }
.pp-card-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
  flex-wrap: wrap;
}
.pp-page-title { margin: 0; font-size: 20px; font-weight: 600; color: var(--color-text-1); }
.pp-page-sub { margin: 4px 0 0; color: var(--color-text-3); font-size: 13px; }
.pp-head-actions { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.search-box { width: 240px; }

.rules-card { border-radius: 14px; }
.rules-card :deep(.arco-card-body) { padding: 0; }
.rules-card :deep(.arco-table-th) { background: var(--pp-surface-soft); font-weight: 600; }
.rules-card :deep(.arco-table-tr-hover .arco-table-td) { background: var(--pp-brand-1) !important; }

.port-cell { display: inline-flex; align-items: center; gap: 6px; }
.port-cell code {
  font-family: ui-monospace, monospace;
  font-weight: 600;
  color: var(--color-text-1);
}
.muted { color: var(--color-text-3); font-size: 12px; }
.mono { font-family: ui-monospace, monospace; }
.loading-skel { padding: 24px; display: flex; flex-direction: column; gap: 16px; }

/* Mobile cards */
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
.m-port { display: inline-flex; align-items: center; gap: 6px; }
.m-port code { font-family: ui-monospace, monospace; font-weight: 600; font-size: 15px; }
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
.m-actions { display: flex; gap: 6px; flex-wrap: wrap; }
.m-actions :deep(.arco-btn) { flex: 1; min-width: 0; }

@media (max-width: 768px) {
  .search-box { width: 100%; flex: 1; }
  .pp-head-actions { width: 100%; }
}
</style>
