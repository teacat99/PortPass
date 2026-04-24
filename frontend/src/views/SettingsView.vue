<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message, Modal } from '@arco-design/web-vue'
import {
  IconPlus, IconRefresh, IconEdit, IconDelete,
  IconSafe, IconClockCircle, IconStorage, IconCommon, IconLock
} from '@arco-design/web-vue/es/icon'
import { deletePreset, getSettings, listPresets, upsertPreset } from '@/api/rules'
import type { PresetPort, SettingsBundle } from '@/api/types'
import { useBreakpoint } from '@/composables/useBreakpoint'
import { categorize } from '@/utils/presetCategory'
import EmptyState from '@/components/EmptyState.vue'

const { t, locale } = useI18n()
const { isMobile } = useBreakpoint()

const settings = ref<SettingsBundle | null>(null)
const presets = ref<PresetPort[]>([])
const loading = ref(false)

const editVisible = ref(false)
const editing = ref<Partial<PresetPort>>({})
const isEditingExisting = computed(() => !!editing.value.id)

const newPresetLabel = computed(() => locale.value === 'zh-CN' ? '新建预设' : 'New preset')
const editPresetLabel = computed(() => locale.value === 'zh-CN' ? '编辑预设' : 'Edit preset')

async function reload() {
  loading.value = true
  try {
    const [s, p] = await Promise.all([getSettings(), listPresets()])
    settings.value = s
    presets.value = p
  } finally {
    loading.value = false
  }
}

onMounted(reload)

function openCreate() {
  editing.value = {
    name: '',
    port: undefined,
    protocol: 'tcp',
    sort: 99,
    user_allowed: false,
    max_duration_sec: 0
  }
  editVisible.value = true
}

function openEdit(p: PresetPort) {
  editing.value = { ...p }
  editVisible.value = true
}

async function saveEdit() {
  if (!editing.value.name?.trim() || !editing.value.port) {
    Message.warning(t('msg.invalidInput'))
    return false
  }
  await upsertPreset(editing.value)
  Message.success(t('msg.presetSaved'))
  editVisible.value = false
  await reload()
  return true
}

async function removePreset(p: PresetPort) {
  Modal.warning({
    title: t('action.delete') + ' · ' + p.name,
    content: `${p.port}/${p.protocol}`,
    hideCancel: false,
    okButtonProps: { status: 'danger' },
    onOk: async () => {
      await deletePreset(p.id)
      Message.success(t('msg.presetDeleted'))
      await reload()
    }
  })
}

function fmtDuration(sec: number): string {
  if (!sec) return '—'
  if (sec < 60) return sec + ' 秒'
  if (sec < 3600) return Math.floor(sec / 60) + ' 分钟'
  return Math.floor(sec / 3600) + ' 小时' + (sec % 3600 > 0 ? Math.floor((sec % 3600) / 60) + '分' : '')
}

const presetCount = computed(() => presets.value.length)
const userAllowedCount = computed(() => presets.value.filter((p) => p.user_allowed).length)
</script>

<template>
  <div class="pp-page settings-wrap">
    <header class="pp-card-head">
      <div>
        <h1 class="pp-page-title">{{ t('settings.title') }}</h1>
        <p class="pp-page-sub">管理预设端口、查看运行时默认参数与鉴权策略</p>
      </div>
      <div class="pp-head-actions">
        <a-button @click="reload" :loading="loading">
          <template #icon><IconRefresh /></template>
          {{ t('action.refresh') }}
        </a-button>
      </div>
    </header>

    <!-- Runtime overview cards: a quick "system status" snapshot. -->
    <section v-if="settings" class="overview">
      <div class="ov-card">
        <div class="ov-icon" style="background: rgba(22,93,255,0.1); color: var(--pp-brand-6)"><IconSafe /></div>
        <div class="ov-meta">
          <div class="ov-label">鉴权模式</div>
          <div class="ov-value">{{ settings.auth_mode }}</div>
        </div>
      </div>
      <div class="ov-card">
        <div class="ov-icon" style="background: rgba(0,180,42,0.1); color: var(--pp-status-active)"><IconCommon /></div>
        <div class="ov-meta">
          <div class="ov-label">防火墙驱动</div>
          <div class="ov-value">{{ settings.firewall_driver }}</div>
        </div>
      </div>
      <div class="ov-card">
        <div class="ov-icon" style="background: rgba(255,125,0,0.1); color: var(--pp-status-pending)"><IconClockCircle /></div>
        <div class="ov-meta">
          <div class="ov-label">单条规则上限</div>
          <div class="ov-value">{{ settings.max_duration_hours }} 小时</div>
        </div>
      </div>
      <div class="ov-card">
        <div class="ov-icon" style="background: rgba(114,46,209,0.1); color: #722ed1"><IconStorage /></div>
        <div class="ov-meta">
          <div class="ov-label">历史保留</div>
          <div class="ov-value">{{ settings.history_retention_days }} 天</div>
        </div>
      </div>
    </section>

    <a-card class="settings-card">
      <a-tabs default-active-key="presets" :auto-switch="false">
        <a-tab-pane key="presets">
          <template #title>
            <span class="tab-title">📦 {{ t('settings.tabPresets') }}
              <a-tag size="small" color="arcoblue">{{ presetCount }}</a-tag>
            </span>
          </template>

          <div class="tab-body">
            <div class="tab-toolbar">
              <p class="tab-help">
                普通用户仅能在勾选了「普通用户可用」的端口上创建规则，
                <strong>{{ userAllowedCount }}</strong> / {{ presetCount }} 个预设当前对普通用户可见。
              </p>
              <a-button type="primary" @click="openCreate">
                <template #icon><IconPlus /></template>
                {{ newPresetLabel }}
              </a-button>
            </div>

            <EmptyState
              v-if="!presets.length && !loading"
              icon="📭"
              title="还没有预设端口"
              description="预设可以让首页的用户一键选择常用端口，建议至少配置 SSH / HTTP / HTTPS。"
            />

            <a-table
              v-else-if="!isMobile"
              :data="presets"
              :pagination="false"
              :hoverable="true"
              :bordered="false"
              size="medium"
              row-key="id"
            >
              <template #columns>
                <a-table-column title="名称" :width="220">
                  <template #cell="{ record }">
                    <span class="preset-name">
                      <span class="preset-icon">{{ categorize(record).icon }}</span>
                      <span>{{ record.name }}</span>
                    </span>
                  </template>
                </a-table-column>
                <a-table-column title="端口 / 协议" :width="160">
                  <template #cell="{ record }">
                    <code class="mono">:{{ record.port }}</code>
                    <a-tag size="small" :color="record.protocol === 'udp' ? 'purple' : 'arcoblue'" style="margin-left: 6px">
                      {{ record.protocol.toUpperCase() }}
                    </a-tag>
                  </template>
                </a-table-column>
                <a-table-column :title="t('settings.userAllowed')" :width="140">
                  <template #cell="{ record }">
                    <a-tag :color="record.user_allowed ? 'green' : 'gray'" size="small">
                      <IconLock v-if="!record.user_allowed" /> {{ record.user_allowed ? '可用' : '仅管理员' }}
                    </a-tag>
                  </template>
                </a-table-column>
                <a-table-column :title="t('settings.maxDurationSec')" :width="160">
                  <template #cell="{ record }">
                    <span :class="record.max_duration_sec ? 'mono' : 'muted'">
                      {{ fmtDuration(record.max_duration_sec) }}
                    </span>
                  </template>
                </a-table-column>
                <a-table-column title="排序" :width="80" data-index="sort" align="center" />
                <a-table-column :title="t('rules.actions')" :width="140" align="right" fixed="right">
                  <template #cell="{ record }">
                    <a-space :size="4">
                      <a-tooltip :content="t('action.edit')">
                        <a-button size="small" type="text" @click="openEdit(record)">
                          <template #icon><IconEdit /></template>
                        </a-button>
                      </a-tooltip>
                      <a-tooltip :content="t('action.delete')">
                        <a-button size="small" type="text" status="danger" @click="removePreset(record)">
                          <template #icon><IconDelete /></template>
                        </a-button>
                      </a-tooltip>
                    </a-space>
                  </template>
                </a-table-column>
              </template>
            </a-table>

            <div v-else class="m-list">
              <div v-for="p in presets" :key="p.id" class="m-card">
                <div class="m-card-head">
                  <span class="preset-name">
                    <span class="preset-icon">{{ categorize(p).icon }}</span>
                    <strong>{{ p.name }}</strong>
                  </span>
                  <span><code class="mono">:{{ p.port }}/{{ p.protocol }}</code></span>
                </div>
                <div class="m-grid">
                  <div class="m-cell"><span class="muted">{{ t('settings.userAllowed') }}</span>
                    <a-tag :color="p.user_allowed ? 'green' : 'gray'" size="small">{{ p.user_allowed ? '可用' : '仅管理员' }}</a-tag>
                  </div>
                  <div class="m-cell"><span class="muted">{{ t('settings.maxDurationSec') }}</span>
                    <span class="mono">{{ fmtDuration(p.max_duration_sec) }}</span>
                  </div>
                </div>
                <div class="m-actions">
                  <a-button size="small" @click="openEdit(p)">
                    <template #icon><IconEdit /></template>
                    {{ t('action.edit') }}
                  </a-button>
                  <a-button size="small" status="danger" @click="removePreset(p)">
                    <template #icon><IconDelete /></template>
                    {{ t('action.delete') }}
                  </a-button>
                </div>
              </div>
            </div>
          </div>
        </a-tab-pane>

        <a-tab-pane key="defaults">
          <template #title>
            <span class="tab-title">⚙️ {{ t('settings.tabDefaults') }}</span>
          </template>
          <div class="tab-body">
            <a-alert type="info" closable>
              这些参数由启动时的环境变量决定（PORTPASS_* 系列），需修改请编辑容器配置后重启。
            </a-alert>
            <div class="kv-grid" v-if="settings">
              <div class="kv">
                <span class="kv-k">PORTPASS_FIREWALL_DRIVER</span>
                <code class="kv-v">{{ settings.firewall_driver }}</code>
              </div>
              <div class="kv">
                <span class="kv-k">PORTPASS_MAX_DURATION_HOURS</span>
                <code class="kv-v">{{ settings.max_duration_hours }}</code>
              </div>
              <div class="kv">
                <span class="kv-k">PORTPASS_HISTORY_RETENTION_DAYS</span>
                <code class="kv-v">{{ settings.history_retention_days }}</code>
              </div>
              <div class="kv">
                <span class="kv-k">PORTPASS_AUTH_MODE</span>
                <code class="kv-v">{{ settings.auth_mode }}</code>
              </div>
            </div>
          </div>
        </a-tab-pane>

        <a-tab-pane key="proxies">
          <template #title>
            <span class="tab-title">🌐 {{ t('settings.tabProxies') }}
              <a-tag size="small">{{ settings?.trusted_proxies?.length ?? 0 }}</a-tag>
            </span>
          </template>
          <div class="tab-body">
            <a-alert type="warning" closable>
              客户端真实 IP 通过 <code>X-Forwarded-For</code> 解析。仅当请求来自这些 CIDR 时，PortPass 才会信任 XFF 头部。
            </a-alert>
            <div v-if="settings?.trusted_proxies?.length" class="proxy-list">
              <a-tag v-for="p in settings.trusted_proxies" :key="p" size="medium" color="arcoblue">
                {{ p }}
              </a-tag>
            </div>
            <EmptyState
              v-else
              icon="🛡️"
              title="未配置可信反代"
              description="如需识别 NAT 后的真实客户端 IP，请通过 PORTPASS_TRUSTED_PROXIES 环境变量配置。"
            />
          </div>
        </a-tab-pane>

        <a-tab-pane key="auth">
          <template #title>
            <span class="tab-title">🔑 {{ t('settings.tabAuth') }}</span>
          </template>
          <div class="tab-body">
            <div v-if="settings" class="auth-current">
              <div class="auth-mode">
                <span class="muted">当前鉴权模式</span>
                <a-tag color="arcoblue" size="medium" style="font-size: 14px">{{ settings.auth_mode }}</a-tag>
              </div>
              <div class="auth-explain">
                <p v-if="settings.auth_mode === 'password'">用户名 + 密码登录，签发 JWT Token。所有账号信息持久化在 SQLite。</p>
                <p v-else-if="settings.auth_mode === 'ipwhitelist'">仅根据来源 IP 是否在白名单内放行，UI 跳过登录步骤。</p>
                <p v-else>关闭鉴权，任何人都可以访问。<strong>仅适合内网或开发环境使用。</strong></p>
              </div>
            </div>
          </div>
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <a-modal
      v-model:visible="editVisible"
      :title="isEditingExisting ? editPresetLabel : newPresetLabel"
      :on-before-ok="saveEdit"
      unmount-on-close
    >
      <a-form :model="editing" layout="vertical">
        <div class="form-row">
          <a-form-item label="名称" class="grow">
            <a-input v-model="editing.name" placeholder="例如 SSH" />
          </a-form-item>
          <a-form-item label="排序优先级">
            <a-input-number v-model="editing.sort" :min="0" :max="999" hide-button class="num-w" />
          </a-form-item>
        </div>
        <div class="form-row">
          <a-form-item label="端口" class="grow">
            <a-input-number v-model="editing.port" :min="1" :max="65535" hide-button placeholder="1-65535" />
          </a-form-item>
          <a-form-item label="协议">
            <a-radio-group v-model="editing.protocol" type="button">
              <a-radio value="tcp">TCP</a-radio>
              <a-radio value="udp">UDP</a-radio>
              <a-radio value="both">TCP+UDP</a-radio>
            </a-radio-group>
          </a-form-item>
        </div>
        <a-divider style="margin: 8px 0" />
        <a-form-item :label="t('settings.userAllowed')" :help="t('settings.userAllowedHelp')">
          <a-switch v-model="editing.user_allowed" />
        </a-form-item>
        <a-form-item :label="t('settings.maxDurationSec')" :help="t('settings.maxDurationSecHelp')">
          <a-input-number v-model="editing.max_duration_sec" :min="0" :max="24 * 3600" :step="300" hide-button class="num-w" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.settings-wrap { display: flex; flex-direction: column; gap: 16px; }
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

.overview {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}
.ov-card {
  background: var(--pp-surface);
  border-radius: 12px;
  padding: 14px 16px;
  display: flex;
  align-items: center;
  gap: 12px;
  box-shadow: var(--pp-shadow-1);
}
.ov-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex: 0 0 40px;
}
.ov-label { font-size: 12px; color: var(--color-text-3); }
.ov-value { font-size: 16px; font-weight: 600; color: var(--color-text-1); margin-top: 2px; }

.settings-card { border-radius: 14px; }
.settings-card :deep(.arco-card-body) { padding: 0; }
.settings-card :deep(.arco-tabs-nav) { padding: 0 20px; border-bottom: 1px solid var(--pp-border); }
.tab-title { display: inline-flex; align-items: center; gap: 6px; }
.tab-body { padding: 20px 24px 24px; display: flex; flex-direction: column; gap: 16px; }
.tab-toolbar { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.tab-help { margin: 0; color: var(--color-text-2); font-size: 13px; flex: 1; }

.preset-name { display: inline-flex; align-items: center; gap: 8px; }
.preset-icon { font-size: 16px; }
.muted { color: var(--color-text-3); }
.mono { font-family: ui-monospace, SFMono-Regular, monospace; font-weight: 500; color: var(--color-text-1); }

.kv-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 8px;
}
.kv {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  background: var(--pp-surface-soft);
  border-radius: 8px;
}
.kv-k { font-family: ui-monospace, monospace; font-size: 12px; color: var(--color-text-2); }
.kv-v { font-family: ui-monospace, monospace; font-weight: 600; color: var(--pp-brand-6); }

.proxy-list { display: flex; flex-wrap: wrap; gap: 8px; }
.auth-current { display: flex; flex-direction: column; gap: 12px; }
.auth-mode { display: flex; align-items: center; gap: 10px; }
.auth-explain { color: var(--color-text-2); line-height: 1.7; font-size: 13px; }

.form-row { display: flex; gap: 12px; }
.form-row .grow { flex: 1; }
.num-w :deep(.arco-input-number) { width: 100%; }

.m-list { display: flex; flex-direction: column; gap: 10px; }
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
  gap: 8px;
}
.m-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.m-cell { display: flex; flex-direction: column; gap: 4px; font-size: 13px; }
.m-cell .muted { font-size: 11px; }
.m-actions { display: flex; gap: 6px; }
.m-actions :deep(.arco-btn) { flex: 1; }

@media (max-width: 768px) {
  .form-row { flex-direction: column; gap: 0; }
  .tab-body { padding: 16px; }
  .settings-card :deep(.arco-tabs-nav) { padding: 0 8px; }
}
</style>
