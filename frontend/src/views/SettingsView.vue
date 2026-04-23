<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message, Modal } from '@arco-design/web-vue'
import { deletePreset, getSettings, listPresets, upsertPreset } from '@/api/rules'
import type { PresetPort, SettingsBundle } from '@/api/types'

const { t } = useI18n()

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
  editing.value = { name: '', port: undefined, protocol: 'tcp', sort: 99 }
  editVisible.value = true
}
function openEdit(p: PresetPort) {
  editing.value = { ...p }
  editVisible.value = true
}
async function saveEdit() {
  if (!editing.value.name || !editing.value.port) {
    Message.warning(t('msg.invalidInput'))
    return
  }
  await upsertPreset(editing.value)
  Message.success(t('msg.presetSaved'))
  editVisible.value = false
  await reload()
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
    <a-tabs>
      <a-tab-pane key="presets" :title="t('settings.tabPresets')">
        <a-space style="margin-bottom: 12px">
          <a-button type="primary" @click="openCreate">+</a-button>
        </a-space>
        <a-table :data="presets" :pagination="false">
          <template #columns>
            <a-table-column title="Name" data-index="name" />
            <a-table-column title="Port" data-index="port" :width="100" />
            <a-table-column title="Proto" data-index="protocol" :width="100" />
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

    <a-modal v-model:visible="editVisible" :title="t('action.edit')" @ok="saveEdit">
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
      </a-form>
    </a-modal>
  </a-card>
</template>
