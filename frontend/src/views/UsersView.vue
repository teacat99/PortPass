<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message, Modal } from '@arco-design/web-vue'
import { IconPlus, IconRefresh, IconDelete, IconLock, IconEdit } from '@arco-design/web-vue/es/icon'
import type { Role, User } from '@/api/types'
import { createUser, deleteUser, listUsers, resetUserPassword, updateUser } from '@/api/users'
import { useAuthStore } from '@/stores/auth'
import { useBreakpoint } from '@/composables/useBreakpoint'

const { t } = useI18n()
const auth = useAuthStore()
const { isMobile } = useBreakpoint()

const loading = ref(false)
const users = ref<User[]>([])

// Create/edit modal state.
const createModal = ref(false)
const createForm = reactive({ username: '', password: '', role: 'user' as Role })
const createSubmitting = ref(false)

// Reset password modal state.
const resetModal = ref(false)
const resetTarget = ref<User | null>(null)
const resetForm = reactive({ new_password: '', confirm: '' })
const resetSubmitting = ref(false)

const activeAdminCount = computed(
  () => users.value.filter((u) => u.role === 'admin' && !u.disabled).length
)

onMounted(load)

async function load() {
  loading.value = true
  try {
    users.value = await listUsers()
  } catch {
    // Interceptor shows the error.
  } finally {
    loading.value = false
  }
}

function openCreate() {
  createForm.username = ''
  createForm.password = ''
  createForm.role = 'user'
  createModal.value = true
}

async function submitCreate() {
  if (!createForm.username.trim() || createForm.password.length < 6) {
    Message.warning(t('password.too_short'))
    return false
  }
  createSubmitting.value = true
  try {
    await createUser({
      username: createForm.username.trim(),
      password: createForm.password,
      role: createForm.role
    })
    Message.success(t('msg.userCreated'))
    createModal.value = false
    await load()
  } catch {
    return false
  } finally {
    createSubmitting.value = false
  }
  return true
}

function isSelf(u: User) {
  return auth.me?.id === u.id
}

// Role change is executed inline: the row's select emits the new role and
// we immediately persist. The backend enforces the "at least one active
// admin" invariant; we show a friendly hint before sending when possible.
async function changeRole(u: User, newRole: Role) {
  if (isSelf(u)) {
    Message.warning(t('users.selfCannotModify'))
    await load()
    return
  }
  if (u.role === 'admin' && newRole !== 'admin' && !u.disabled && activeAdminCount.value <= 1) {
    Message.warning(t('users.lastAdminWarn'))
    await load()
    return
  }
  try {
    await updateUser(u.id, { role: newRole })
    Message.success(t('msg.userUpdated'))
    await load()
  } catch {
    await load()
  }
}

async function toggleDisabled(u: User, disabled: boolean) {
  if (isSelf(u)) {
    Message.warning(t('users.selfCannotModify'))
    await load()
    return
  }
  if (u.role === 'admin' && disabled && !u.disabled && activeAdminCount.value <= 1) {
    Message.warning(t('users.lastAdminWarn'))
    await load()
    return
  }
  try {
    await updateUser(u.id, { disabled })
    Message.success(t('msg.userUpdated'))
    await load()
  } catch {
    await load()
  }
}

function openReset(u: User) {
  resetTarget.value = u
  resetForm.new_password = ''
  resetForm.confirm = ''
  resetModal.value = true
}

async function submitReset() {
  if (!resetTarget.value) return false
  if (resetForm.new_password.length < 6) {
    Message.warning(t('password.too_short'))
    return false
  }
  if (resetForm.new_password !== resetForm.confirm) {
    Message.warning(t('password.mismatch'))
    return false
  }
  resetSubmitting.value = true
  try {
    await resetUserPassword(resetTarget.value.id, resetForm.new_password)
    Message.success(t('password.changed'))
    resetModal.value = false
    return true
  } catch {
    return false
  } finally {
    resetSubmitting.value = false
  }
}

function confirmDelete(u: User) {
  if (isSelf(u)) {
    Message.warning(t('users.selfCannotModify'))
    return
  }
  Modal.warning({
    title: t('action.delete'),
    content: t('users.deleteConfirm'),
    hideCancel: false,
    onOk: async () => {
      try {
        await deleteUser(u.id)
        Message.success(t('msg.userDeleted'))
        await load()
      } catch {
        /* interceptor */
      }
    }
  })
}

const columns = [
  { title: 'ID', dataIndex: 'id', width: 70 },
  { title: t('users.username'), dataIndex: 'username' },
  { title: t('users.role'), slotName: 'role', width: 140 },
  { title: t('users.disabled'), slotName: 'disabled', width: 140 },
  { title: t('users.createdAt'), slotName: 'createdAt', width: 180 },
  { title: t('users.actions'), slotName: 'actions', width: 240, fixed: 'right' as const }
]
</script>

<template>
  <div class="users-view">
    <div class="page-header">
      <h2>{{ t('users.title') }}</h2>
      <a-space wrap>
        <a-button @click="load"><template #icon><IconRefresh /></template>{{ t('action.refresh') }}</a-button>
        <a-button type="primary" @click="openCreate"><template #icon><IconPlus /></template>{{ t('action.new_user') }}</a-button>
      </a-space>
    </div>

    <!-- Desktop: a-table. Mobile: card layout. -->
    <a-table
      v-if="!isMobile"
      :columns="columns"
      :data="users"
      :loading="loading"
      :pagination="false"
      row-key="id"
      :scroll="{ x: 900 }"
    >
      <template #role="{ record }">
        <a-select
          :model-value="record.role"
          :disabled="isSelf(record)"
          size="small"
          @change="(v: unknown) => changeRole(record, v as Role)"
        >
          <a-option value="admin">{{ t('users.roleAdmin') }}</a-option>
          <a-option value="user">{{ t('users.roleUser') }}</a-option>
        </a-select>
      </template>
      <template #disabled="{ record }">
        <a-switch
          :model-value="!record.disabled"
          :disabled="isSelf(record)"
          @change="(v: unknown) => toggleDisabled(record, !(v as boolean))"
        />
        <span class="state-label">{{ record.disabled ? t('users.disabled') : t('users.enabled') }}</span>
      </template>
      <template #createdAt="{ record }">{{ new Date(record.created_at).toLocaleString() }}</template>
      <template #actions="{ record }">
        <a-space>
          <a-button size="small" @click="openReset(record)">
            <template #icon><IconLock /></template>
            {{ t('action.reset_password') }}
          </a-button>
          <a-button size="small" status="danger" :disabled="isSelf(record)" @click="confirmDelete(record)">
            <template #icon><IconDelete /></template>
            {{ t('action.delete') }}
          </a-button>
        </a-space>
      </template>
    </a-table>

    <div v-else class="portpass-card-list">
      <div v-if="loading" class="loading-hint">{{ t('action.refresh') }}...</div>
      <div v-for="u in users" :key="u.id" class="portpass-card">
        <h4>
          {{ u.username }}
          <a-tag v-if="u.role === 'admin'" color="arcoblue" size="small">{{ t('users.roleAdmin') }}</a-tag>
          <a-tag v-else size="small">{{ t('users.roleUser') }}</a-tag>
          <a-tag v-if="u.disabled" color="red" size="small">{{ t('users.disabled') }}</a-tag>
        </h4>
        <div class="row"><span class="label">ID</span><span>{{ u.id }}</span></div>
        <div class="row"><span class="label">{{ t('users.createdAt') }}</span><span>{{ new Date(u.created_at).toLocaleString() }}</span></div>
        <div class="row">
          <span class="label">{{ t('users.role') }}</span>
          <a-select
            :model-value="u.role"
            :disabled="isSelf(u)"
            size="small"
            style="width: 140px"
            @change="(v: unknown) => changeRole(u, v as Role)"
          >
            <a-option value="admin">{{ t('users.roleAdmin') }}</a-option>
            <a-option value="user">{{ t('users.roleUser') }}</a-option>
          </a-select>
        </div>
        <div class="row">
          <span class="label">{{ t('users.enabled') }}</span>
          <a-switch
            :model-value="!u.disabled"
            :disabled="isSelf(u)"
            @change="(v: unknown) => toggleDisabled(u, !(v as boolean))"
          />
        </div>
        <div class="actions">
          <a-button size="medium" @click="openReset(u)">
            <template #icon><IconLock /></template>
            {{ t('action.reset_password') }}
          </a-button>
          <a-button size="medium" status="danger" :disabled="isSelf(u)" @click="confirmDelete(u)">
            <template #icon><IconDelete /></template>
            {{ t('action.delete') }}
          </a-button>
        </div>
      </div>
    </div>

    <!-- Create user modal. -->
    <a-modal
      v-model:visible="createModal"
      :title="t('users.newUser')"
      :on-before-ok="submitCreate"
      :confirm-loading="createSubmitting"
      unmount-on-close
    >
      <a-form :model="createForm" layout="vertical">
        <a-form-item :label="t('users.username')">
          <a-input v-model="createForm.username" autocomplete="off" />
        </a-form-item>
        <a-form-item :label="t('password.new')">
          <a-input-password v-model="createForm.password" autocomplete="new-password" />
        </a-form-item>
        <a-form-item :label="t('users.role')">
          <a-radio-group v-model="createForm.role">
            <a-radio value="user">{{ t('users.roleUser') }}</a-radio>
            <a-radio value="admin">{{ t('users.roleAdmin') }}</a-radio>
          </a-radio-group>
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- Reset-password modal. -->
    <a-modal
      v-model:visible="resetModal"
      :title="t('users.resetPwd') + (resetTarget ? ' - ' + resetTarget.username : '')"
      :on-before-ok="submitReset"
      :confirm-loading="resetSubmitting"
      unmount-on-close
    >
      <a-form :model="resetForm" layout="vertical">
        <a-form-item :label="t('password.new')">
          <a-input-password v-model="resetForm.new_password" autocomplete="new-password" />
        </a-form-item>
        <a-form-item :label="t('password.confirm')">
          <a-input-password v-model="resetForm.confirm" autocomplete="new-password" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.users-view { display: flex; flex-direction: column; gap: 16px; }
.page-header { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 8px; }
.page-header h2 { margin: 0; font-size: 20px; }
.state-label { margin-left: 8px; color: var(--color-text-3); font-size: 12px; }
.loading-hint { color: var(--color-text-3); text-align: center; padding: 20px; }
@media (max-width: 640px) {
  .page-header h2 { font-size: 18px; }
}
</style>
