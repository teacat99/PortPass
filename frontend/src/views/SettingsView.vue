<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message, Modal } from '@arco-design/web-vue'
import { deletePreset, getSettings, listPresets, upsertPreset } from '@/api/rules'
import type { PresetPort, SettingsBundle } from '@/api/types'
import { useBreakpoint } from '@/composables/useBreakpoint'

const { t } = useI18n()
const { isMobile, isNarrow } = useBreakpoint()

const settings = ref<SettingsBundle | null>(null)
const presets = ref<PresetPort[]>([])

const editVisible = ref(false)
const editing = ref<Partial<PresetPort>>({})

async function reload() {
  const [s, p] = await Promise.all([getSettings(), listPresets()])
  settings.value = s
  presets.value = p
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
  if (!editing.value.name || !editing.value.port) {
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
    title: t('action.delete'),
    content: `${p.name} (${p.port}/${p.protocol})`,
    hideCancel: false,
    onOk: async () => {
      await deletePreset(p.id)
      Message.success(t('msg.presetDeleted'))
      await reload()
    }
  })
}
</script>

<template>
  <a-card :title="t('settings.title')">
    <a-tabs :position="isNarrow ? 'top' : 'top'" :type="isNarrow ? 'line' : 'line'">
      <a-tab-pane key="presets" :title="t('settings.tabPresets')">
        <a-space style="margin-bottom: 12px" wrap>
          <a-button type="primary" @click="openCreate">+ {{ t('action.new_user') === '新建用户' ? '新建预设' : 'New preset' }}</a-button>
        </a-space>

        <!-- Desktop table -->
        <a-table v-if="!isMobile" :data="presets" :pagination="false">
          <template #columns>
            <a-table-column title="Name" data-index="name" />
            <a-table-column title="Port" data-index="port" :width="100" />
            <a-table-column title="Proto" data-index="protocol" :width="100" />
            <a-table-column :title="t('settings.userAllowed')" :width="140">
              <template #cell="{ record }">
                <a-tag :color="record.user_allowed ? 'green' : 'gray'">
                  {{ record.user_allowed ? '✓' : '×' }}
                </a-tag>
              </template>
            </a-table-column>
            <a-table-column :title="t('settings.maxDurationSec')" :width="180">
              <template #cell="{ record }">
                {{ record.max_duration_sec ? record.max_duration_sec + 's' : '-' }}
              </template>
            </a-table-column>
            <a-table-column title="Sort" data-index="sort" :width="80" />
            <a-table-column :title="t('rules.actions')" :width="200">
              <template #cell="{ record }">
                <a-space>
                  <a-button size="small" @click="openEdit(record)">{{ t('action.edit') }}</a-button>
                  <a-button size="small" status="danger" @click="removePreset(record)">{{ t('action.delete') }}</a-button>
                </a-space>
              </template>
            </a-table-column>
          </template>
        </a-table>

        <!-- Mobile cards -->
        <div v-else class="portpass-card-list">
          <div v-for="p in presets" :key="p.id" class="portpass-card">
            <h4>{{ p.name }} <span class="mono">{{ p.port }}/{{ p.protocol }}</span></h4>
            <div class="row">
              <span class="label">{{ t('settings.userAllowed') }}</span>
              <a-tag :color="p.user_allowed ? 'green' : 'gray'">{{ p.user_allowed ? '✓' : '×' }}</a-tag>
            </div>
            <div class="row">
              <span class="label">{{ t('settings.maxDurationSec') }}</span>
              <span>{{ p.max_duration_sec ? p.max_duration_sec + 's' : '-' }}</span>
            </div>
            <div class="row"><span class="label">Sort</span><span>{{ p.sort }}</span></div>
            <div class="actions">
              <a-button size="medium" @click="openEdit(p)">{{ t('action.edit') }}</a-button>
              <a-button size="medium" status="danger" @click="removePreset(p)">{{ t('action.delete') }}</a-button>
            </div>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="defaults" :title="t('settings.tabDefaults')">
        <a-descriptions v-if="settings" :column="1" size="medium" :data="[
          { label: t('settings.firewallDriver'), value: settings.firewall_driver },
          { label: t('settings.maxDuration'), value: String(settings.max_duration_hours) },
          { label: t('settings.historyRetention'), value: String(settings.history_retention_days) }
        ]" />
        <a-alert type="info" style="margin-top: 12px">
          {{ 'Runtime defaults are sourced from environment variables — edit your deployment to change them.' }}
        </a-alert>
      </a-tab-pane>

      <a-tab-pane key="proxies" :title="t('settings.tabProxies')">
        <a-descriptions v-if="settings" :column="1" :data="[
          { label: t('settings.trustedProxies'), value: settings.trusted_proxies.join(', ') || '—' }
        ]" />
      </a-tab-pane>

      <a-tab-pane key="auth" :title="t('settings.tabAuth')">
        <a-descriptions v-if="settings" :column="1" :data="[
          { label: t('settings.authMode'), value: settings.auth_mode }
        ]" />
      </a-tab-pane>
    </a-tabs>

    <a-modal v-model:visible="editVisible" :title="t('action.edit')" :on-before-ok="saveEdit" unmount-on-close>
      <a-form :model="editing" layout="vertical">
        <a-form-item label="Name">
          <a-input v-model="editing.name" />
        </a-form-item>
        <a-form-item label="Port">
          <a-input-number v-model="editing.port" :min="1" :max="65535" />
        </a-form-item>
        <a-form-item label="Protocol">
          <a-select v-model="editing.protocol">
            <a-option value="tcp">TCP</a-option>
            <a-option value="udp">UDP</a-option>
            <a-option value="both">TCP+UDP</a-option>
          </a-select>
        </a-form-item>
        <a-form-item label="Sort">
          <a-input-number v-model="editing.sort" :min="0" :max="999" />
        </a-form-item>
        <a-form-item :label="t('settings.userAllowed')" :help="t('settings.userAllowedHelp')">
          <a-switch v-model="editing.user_allowed" />
        </a-form-item>
        <a-form-item :label="t('settings.maxDurationSec')" :help="t('settings.maxDurationSecHelp')">
          <a-input-number v-model="editing.max_duration_sec" :min="0" :max="24 * 3600" :step="300" />
        </a-form-item>
      </a-form>
    </a-modal>
  </a-card>
</template>

<style scoped>
.mono { font-family: ui-monospace, SFMono-Regular, monospace; }
</style>
