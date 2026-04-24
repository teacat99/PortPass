<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message, Modal } from '@arco-design/web-vue'
import { IconPlus, IconRefresh, IconDelete, IconLock } from '@arco-design/web-vue/es/icon'
import type { Role, User } from '@/api/types'
import { createUser, deleteUser, listUsers, resetUserPassword, updateUser } from '@/api/users'
import { useAuthStore } from '@/stores/auth'
import { useBreakpoint } from '@/composables/useBreakpoint'
import EmptyState from '@/components/EmptyState.vue'

// Stable colour palette for avatar bubbles, deterministic by username.
const avatarPalette = ['#165dff', '#0fc6c2', '#722ed1', '#f5319d', '#ff7d00', '#00b42a']
function avatarColor(name: string): string {
  let h = 0
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0
  return avatarPalette[h % avatarPalette.length]
}
function avatarLetter(name: string): string {
  return (name.trim()[0] ?? '?').toUpperCase()
}

// Crude password strength signal reused on both Create and Reset modals.
function strengthOf(v: string): { score: number; label: string } {
  if (!v) return { score: 0, label: '' }
  let s = 0
  if (v.length >= 6) s++
  if (v.length >= 10) s++
  if (/[A-Z]/.test(v) && /[a-z]/.test(v)) s++
  if (/\d/.test(v)) s++
  if (/[^A-Za-z0-9]/.test(v)) s++
  return { score: s, label: ['', '太弱', '一般', '中等', '不错', '很强'][s] || '' }
}

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
  { title: t('users.username'), slotName: 'who' },
  { title: t('users.role'), slotName: 'role', width: 160 },
  { title: t('users.disabled'), slotName: 'disabled', width: 140 },
  { title: t('users.createdAt'), slotName: 'createdAt', width: 180 },
  { title: t('users.actions'), slotName: 'actions', width: 240, align: 'right' as const, fixed: 'right' as const }
]

const createStrength = computed(() => strengthOf(createForm.password))
const resetStrength = computed(() => strengthOf(resetForm.new_password))
</script>

<template>
  <div class="pp-page users-wrap">
    <header class="pp-card-head">
      <div>
        <h1 class="pp-page-title">{{ t('users.title') }}</h1>
        <p class="pp-page-sub">
          共 <strong>{{ users.length }}</strong> 个账号 · 启用中管理员 <strong>{{ activeAdminCount }}</strong> 位
        </p>
      </div>
      <div class="pp-head-actions">
        <a-button @click="load" :loading="loading">
          <template #icon><IconRefresh /></template>
          {{ t('action.refresh') }}
        </a-button>
        <a-button type="primary" @click="openCreate">
          <template #icon><IconPlus /></template>
          {{ t('action.new_user') }}
        </a-button>
      </div>
    </header>

    <a-card class="list-card">
      <div v-if="loading && !users.length" class="loading-skel">
        <a-skeleton :animation="true" v-for="i in 3" :key="i">
          <a-skeleton-line :rows="2" :widths="['40%', '85%']" />
        </a-skeleton>
      </div>

      <EmptyState
        v-else-if="!users.length"
        icon="👥"
        title="还没有任何用户"
        description="点击右上角“新建用户”创建第一个普通用户。"
      />

      <a-table
        v-else-if="!isMobile"
        :columns="columns"
        :data="users"
        :pagination="false"
        row-key="id"
        :scroll="{ x: 900 }"
        :hoverable="true"
        :bordered="false"
        size="medium"
      >
        <template #who="{ record }">
          <div class="who-cell">
            <span class="avatar" :style="{ background: avatarColor(record.username) }">
              {{ avatarLetter(record.username) }}
            </span>
            <div class="who-meta">
              <div class="who-name">
                {{ record.username }}
                <span v-if="isSelf(record)" class="self-tag">你</span>
              </div>
              <div class="who-id">ID #{{ record.id }}</div>
            </div>
          </div>
        </template>
        <template #role="{ record }">
          <a-select
            :model-value="record.role"
            :disabled="isSelf(record)"
            size="small"
            class="role-select"
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
        <template #createdAt="{ record }">
          <a-tooltip :content="new Date(record.created_at).toLocaleString()">
            <span class="muted">{{ new Date(record.created_at).toLocaleDateString() }}</span>
          </a-tooltip>
        </template>
        <template #actions="{ record }">
          <a-space :size="4">
            <a-tooltip :content="t('action.reset_password')">
              <a-button size="small" type="text" @click="openReset(record)">
                <template #icon><IconLock /></template>
              </a-button>
            </a-tooltip>
            <a-tooltip :content="isSelf(record) ? '不能删除自己' : t('action.delete')">
              <a-button size="small" type="text" status="danger" :disabled="isSelf(record)" @click="confirmDelete(record)">
                <template #icon><IconDelete /></template>
              </a-button>
            </a-tooltip>
          </a-space>
        </template>
      </a-table>

      <div v-else class="m-list">
        <div v-for="u in users" :key="u.id" class="m-card">
          <div class="m-card-head">
            <span class="avatar" :style="{ background: avatarColor(u.username) }">{{ avatarLetter(u.username) }}</span>
            <div class="who-meta">
              <div class="who-name">
                {{ u.username }}
                <span v-if="isSelf(u)" class="self-tag">你</span>
              </div>
              <div class="who-id">
                <a-tag v-if="u.role === 'admin'" color="arcoblue" size="small">{{ t('users.roleAdmin') }}</a-tag>
                <a-tag v-else size="small">{{ t('users.roleUser') }}</a-tag>
                <a-tag v-if="u.disabled" color="red" size="small">{{ t('users.disabled') }}</a-tag>
              </div>
            </div>
          </div>
          <div class="m-grid">
            <div class="m-cell"><span class="muted">{{ t('users.role') }}</span>
              <a-select
                :model-value="u.role"
                :disabled="isSelf(u)"
                size="small"
                @change="(v: unknown) => changeRole(u, v as Role)"
              >
                <a-option value="admin">{{ t('users.roleAdmin') }}</a-option>
                <a-option value="user">{{ t('users.roleUser') }}</a-option>
              </a-select>
            </div>
            <div class="m-cell"><span class="muted">{{ t('users.enabled') }}</span>
              <a-switch
                :model-value="!u.disabled"
                :disabled="isSelf(u)"
                @change="(v: unknown) => toggleDisabled(u, !(v as boolean))"
              />
            </div>
          </div>
          <div class="m-actions">
            <a-button size="small" @click="openReset(u)">
              <template #icon><IconLock /></template>
              {{ t('action.reset_password') }}
            </a-button>
            <a-button size="small" status="danger" :disabled="isSelf(u)" @click="confirmDelete(u)">
              <template #icon><IconDelete /></template>
              {{ t('action.delete') }}
            </a-button>
          </div>
        </div>
      </div>
    </a-card>

    <a-modal
      v-model:visible="createModal"
      :title="t('users.newUser')"
      :on-before-ok="submitCreate"
      :confirm-loading="createSubmitting"
      unmount-on-close
    >
      <a-form :model="createForm" layout="vertical">
        <a-form-item :label="t('users.username')">
          <a-input v-model="createForm.username" autocomplete="off" placeholder="例如 alice" />
        </a-form-item>
        <a-form-item :label="t('password.new')">
          <a-input-password v-model="createForm.password" autocomplete="new-password" />
          <div class="pwd-strength" v-if="createForm.password">
            <div class="pwd-bars">
              <span v-for="i in 5" :key="i" class="pwd-bar"
                :class="['s' + Math.min(createStrength.score, 5), i <= createStrength.score ? 'on' : '']" />
            </div>
            <span class="pwd-label">{{ createStrength.label }}</span>
          </div>
        </a-form-item>
        <a-form-item :label="t('users.role')">
          <a-radio-group v-model="createForm.role" type="button">
            <a-radio value="user">{{ t('users.roleUser') }}</a-radio>
            <a-radio value="admin">{{ t('users.roleAdmin') }}</a-radio>
          </a-radio-group>
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:visible="resetModal"
      :title="t('users.resetPwd') + (resetTarget ? ' · ' + resetTarget.username : '')"
      :on-before-ok="submitReset"
      :confirm-loading="resetSubmitting"
      unmount-on-close
    >
      <a-form :model="resetForm" layout="vertical">
        <a-form-item :label="t('password.new')">
          <a-input-password v-model="resetForm.new_password" autocomplete="new-password" />
          <div class="pwd-strength" v-if="resetForm.new_password">
            <div class="pwd-bars">
              <span v-for="i in 5" :key="i" class="pwd-bar"
                :class="['s' + Math.min(resetStrength.score, 5), i <= resetStrength.score ? 'on' : '']" />
            </div>
            <span class="pwd-label">{{ resetStrength.label }}</span>
          </div>
        </a-form-item>
        <a-form-item :label="t('password.confirm')">
          <a-input-password v-model="resetForm.confirm" autocomplete="new-password" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.users-wrap { display: flex; flex-direction: column; gap: 16px; }
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

.list-card { border-radius: 14px; }
.list-card :deep(.arco-card-body) { padding: 0; }
.list-card :deep(.arco-table-th) { background: var(--pp-surface-soft); font-weight: 600; }
.loading-skel { padding: 24px; display: flex; flex-direction: column; gap: 16px; }

.who-cell { display: inline-flex; align-items: center; gap: 10px; }
.avatar {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 600;
  flex: 0 0 34px;
  font-size: 14px;
}
.who-meta { display: flex; flex-direction: column; line-height: 1.2; }
.who-name { font-weight: 500; color: var(--color-text-1); display: flex; align-items: center; gap: 6px; }
.who-id { font-size: 11px; color: var(--color-text-3); margin-top: 2px; display: flex; gap: 4px; align-items: center; }
.self-tag {
  font-size: 10px;
  background: var(--pp-brand-1);
  color: var(--pp-brand-7);
  padding: 1px 6px;
  border-radius: 999px;
  font-weight: 500;
}

.role-select { width: 130px; }
.state-label { margin-left: 8px; color: var(--color-text-3); font-size: 12px; }
.muted { color: var(--color-text-3); font-size: 12px; }

.m-list { padding: 12px; display: flex; flex-direction: column; gap: 10px; }
.m-card {
  background: var(--pp-surface);
  border: 1px solid var(--pp-border);
  border-radius: 12px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.m-card-head { display: flex; align-items: center; gap: 10px; }
.m-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 8px 16px; }
.m-cell { display: flex; flex-direction: column; gap: 4px; font-size: 13px; }
.m-cell .muted { font-size: 11px; }
.m-actions { display: flex; gap: 6px; }
.m-actions :deep(.arco-btn) { flex: 1; }

/* Password strength */
.pwd-strength { display: flex; align-items: center; gap: 8px; margin-top: 6px; }
.pwd-bars { display: flex; gap: 3px; flex: 1; }
.pwd-bar {
  flex: 1; height: 4px; border-radius: 2px;
  background: var(--pp-border);
  transition: background 0.15s ease;
}
.pwd-bar.on.s1 { background: #f53f3f; }
.pwd-bar.on.s2 { background: #f53f3f; }
.pwd-bar.on.s3 { background: #ff7d00; }
.pwd-bar.on.s4 { background: #fadb14; }
.pwd-bar.on.s5 { background: var(--pp-status-active); }
.pwd-label { font-size: 11px; color: var(--color-text-3); width: 32px; text-align: right; }

@media (max-width: 768px) {
  .pp-head-actions { width: 100%; }
  .pp-head-actions :deep(.arco-btn) { flex: 1; }
}
</style>
