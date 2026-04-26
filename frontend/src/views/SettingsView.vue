<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Plus, RefreshCw, Pencil, Trash2,
  ShieldCheck, Clock, Database, Cog,
  Lock, Users as UsersIcon, Settings as SettingsIcon,
  AlertTriangle, Package, History, Check, X as XIcon
} from 'lucide-vue-next'
import {
  deletePreset, getSettings, listPresetCategories, listPresets, upsertPreset
} from '@/api/rules'
import {
  listProtectedPorts, upsertProtectedPort, deleteProtectedPort, listUserRanges
} from '@/api/policy'
import {
  createUser, deleteUser, listUsers, resetUserPassword, updateUser
} from '@/api/users'
import { fetchLoginHistory, type LoginAttempt } from '@/api/auth'
import type {
  PresetCategory, PresetPort, ProtectedPort, SettingsBundle, User, Role
} from '@/api/types'
import dayjs from 'dayjs'
import { useAuthStore } from '@/stores/auth'
import { resolveCategory } from '@/utils/presetCategory'
import { isImageIcon } from '@/utils/presetIcon'
import { Message } from '@/lib/toast'

import EmptyState from '@/components/EmptyState.vue'
import PortSetInput from '@/components/PortSetInput.vue'
import UserRangesDrawer from '@/components/UserRangesDrawer.vue'
import RuntimeSettingsForm from '@/components/RuntimeSettingsForm.vue'
import PresetCategorySelect from '@/components/PresetCategorySelect.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import {
  Tabs, TabsList, TabsTrigger, TabsContent
} from '@/components/ui/tabs'
import {
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem
} from '@/components/ui/select'
import {
  Table, TableHeader, TableBody, TableRow, TableHead, TableCell
} from '@/components/ui/table'
import {
  Dialog, DialogContent, DialogHeader, DialogFooter, DialogTitle
} from '@/components/ui/dialog'
import {
  Tooltip, TooltipTrigger, TooltipContent
} from '@/components/ui/tooltip'

const { t, locale } = useI18n()
const auth = useAuthStore()

const activeTab = ref<'users' | 'presets' | 'protected' | 'security' | 'runtime'>('users')

// Login history (Security tab). We load on first visit and when the user
// hits refresh; this avoids paying for the query on every Settings visit.
const loginAttempts = ref<LoginAttempt[]>([])
const loginAttemptsLoading = ref(false)
const loginAttemptsFilter = ref('')

async function loadLoginHistory() {
  loginAttemptsLoading.value = true
  try {
    loginAttempts.value = await fetchLoginHistory({
      username: loginAttemptsFilter.value.trim() || undefined,
      limit: 200,
    })
  } catch {
    loginAttempts.value = []
  } finally {
    loginAttemptsLoading.value = false
  }
}

const settings = ref<SettingsBundle | null>(null)
const presets = ref<PresetPort[]>([])
const presetCategories = ref<PresetCategory[]>([])
const protectedPorts = ref<ProtectedPort[]>([])
const users = ref<User[]>([])
const userRangeCounts = ref<Record<number, number>>({})
const loading = ref(false)

async function reload() {
  loading.value = true
  try {
    const [s, p, cats, pp, us] = await Promise.all([
      getSettings(),
      listPresets(),
      listPresetCategories().catch(() => [] as PresetCategory[]),
      listProtectedPorts().catch(() => []),
      listUsers().catch(() => [])
    ])
    settings.value = s
    presets.value = p
    presetCategories.value = cats
    protectedPorts.value = pp
    users.value = us
    await refreshRangeCounts()
  } finally {
    loading.value = false
  }
}

// Resolve a preset's category icon for table/card rendering. When
// categories haven't loaded yet (or the heuristic returns nothing) we
// fall back to a neutral plug emoji so the row never renders empty.
function presetIcon(p: PresetPort): string {
  const cat = resolveCategory(p, presetCategories.value)
  return cat?.icon || '🔌'
}

async function refreshRangeCounts() {
  const next: Record<number, number> = {}
  for (const u of users.value) {
    try {
      const r = await listUserRanges(u.id)
      next[u.id] = r.length
    } catch {
      next[u.id] = 0
    }
  }
  userRangeCounts.value = next
}

onMounted(reload)

// ─────────── Presets ───────────
const presetEditVisible = ref(false)
const presetEditing = ref<Partial<PresetPort>>({})
const presetPortsValid = ref({ ok: false, error: null as string | null })
const isEditingPreset = computed(() => !!presetEditing.value.id)
const confirmPresetTarget = ref<PresetPort | null>(null)

function openPresetCreate() {
  presetEditing.value = {
    name: '',
    ports: '',
    protocol: 'tcp',
    sort: 99,
    user_allowed: false,
    max_duration_sec: 0,
    category_id: null
  }
  presetPortsValid.value = { ok: false, error: null }
  presetEditVisible.value = true
}

function openPresetEdit(p: PresetPort) {
  presetEditing.value = {
    ...p,
    ports: p.ports || String(p.port || ''),
    category_id: p.category_id ?? null
  }
  presetPortsValid.value = { ok: true, error: null }
  presetEditVisible.value = true
}

async function savePreset() {
  if (!presetEditing.value.name?.trim()) {
    Message.warning(t('msg.invalidInput'))
    return
  }
  if (!presetPortsValid.value.ok) {
    Message.warning(presetPortsValid.value.error || t('msg.invalidInput'))
    return
  }
  try {
    await upsertPreset(presetEditing.value)
    Message.success(t('msg.presetSaved'))
    presetEditVisible.value = false
    await reload()
  } catch {
    /* interceptor */
  }
}

async function doRemovePreset() {
  if (!confirmPresetTarget.value) return
  const id = confirmPresetTarget.value.id
  confirmPresetTarget.value = null
  await deletePreset(id)
  Message.success(t('msg.presetDeleted'))
  await reload()
}

function fmtDuration(sec: number): string {
  if (!sec) return '—'
  if (sec < 60) return sec + t('unit.seconds')
  if (sec < 3600) return Math.floor(sec / 60) + t('unit.minutes')
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return h + t('unit.hours') + (m > 0 ? m + t('unit.minutes') : '')
}

const presetCount = computed(() => presets.value.length)
const userAllowedCount = computed(() => presets.value.filter((p) => p.user_allowed).length)

// ─────────── Protected ───────────
const protectedEditVisible = ref(false)
const protectedEditing = ref<Partial<ProtectedPort>>({})
const protectedPortsValid = ref({ ok: false, error: null as string | null })
const confirmProtectedTarget = ref<ProtectedPort | null>(null)

function openProtectedCreate() {
  protectedEditing.value = { name: '', ports: '', protocol: 'tcp', note: '' }
  protectedPortsValid.value = { ok: false, error: null }
  protectedEditVisible.value = true
}

function openProtectedEdit(p: ProtectedPort) {
  protectedEditing.value = { ...p }
  protectedPortsValid.value = { ok: true, error: null }
  protectedEditVisible.value = true
}

async function saveProtected() {
  if (!protectedEditing.value.name?.trim()) {
    Message.warning(t('msg.invalidInput'))
    return
  }
  if (!protectedPortsValid.value.ok) {
    Message.warning(protectedPortsValid.value.error || t('msg.invalidInput'))
    return
  }
  try {
    await upsertProtectedPort(protectedEditing.value)
    Message.success(t('msg.saved'))
    protectedEditVisible.value = false
    await reload()
  } catch {
    /* interceptor */
  }
}

async function doRemoveProtected() {
  if (!confirmProtectedTarget.value) return
  const id = confirmProtectedTarget.value.id
  confirmProtectedTarget.value = null
  await deleteProtectedPort(id)
  Message.success(t('msg.deleted'))
  await reload()
}

// ─────────── Users ───────────
const avatarPalette = ['#165dff', '#0fc6c2', '#722ed1', '#f5319d', '#ff7d00', '#00b42a']
function avatarColor(name: string): string {
  let h = 0
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0
  return avatarPalette[h % avatarPalette.length]
}
function avatarLetter(name: string): string {
  return (name.trim()[0] ?? '?').toUpperCase()
}
function strengthOf(v: string): { score: number; label: string } {
  if (!v) return { score: 0, label: '' }
  let s = 0
  if (v.length >= 6) s++
  if (v.length >= 10) s++
  if (/[A-Z]/.test(v) && /[a-z]/.test(v)) s++
  if (/\d/.test(v)) s++
  if (/[^A-Za-z0-9]/.test(v)) s++
  const labels = [
    '',
    t('password.strength.weak'),
    t('password.strength.fair'),
    t('password.strength.medium'),
    t('password.strength.good'),
    t('password.strength.strong')
  ]
  return { score: s, label: labels[s] || '' }
}

const userCreateModal = ref(false)
const userCreateForm = reactive({ username: '', password: '', role: 'user' as Role })
const userCreateSubmitting = ref(false)
const userResetModal = ref(false)
const userResetTarget = ref<User | null>(null)
const userResetForm = reactive({ new_password: '', confirm: '' })
const userResetSubmitting = ref(false)
const rangesDrawer = ref(false)
const rangesDrawerUser = ref<User | null>(null)
const confirmUserTarget = ref<User | null>(null)

const activeAdminCount = computed(
  () => users.value.filter((u) => u.role === 'admin' && !u.disabled).length
)

function isSelf(u: User) {
  return auth.me?.id === u.id
}

function openUserCreate() {
  userCreateForm.username = ''
  userCreateForm.password = ''
  userCreateForm.role = 'user'
  userCreateModal.value = true
}

async function submitUserCreate() {
  if (!userCreateForm.username.trim() || userCreateForm.password.length < 6) {
    Message.warning(t('password.too_short'))
    return
  }
  userCreateSubmitting.value = true
  try {
    await createUser({
      username: userCreateForm.username.trim(),
      password: userCreateForm.password,
      role: userCreateForm.role
    })
    Message.success(t('msg.userCreated'))
    userCreateModal.value = false
    await reload()
  } catch {
    /* interceptor */
  } finally {
    userCreateSubmitting.value = false
  }
}

async function changeRole(u: User, newRole: Role) {
  if (isSelf(u)) {
    Message.warning(t('users.selfCannotModify'))
    await reload()
    return
  }
  if (u.role === 'admin' && newRole !== 'admin' && !u.disabled && activeAdminCount.value <= 1) {
    Message.warning(t('users.lastAdminWarn'))
    await reload()
    return
  }
  try {
    await updateUser(u.id, { role: newRole })
    Message.success(t('msg.userUpdated'))
    await reload()
  } catch {
    await reload()
  }
}

async function toggleDisabled(u: User, enabled: boolean) {
  const disabled = !enabled
  if (isSelf(u)) {
    Message.warning(t('users.selfCannotModify'))
    await reload()
    return
  }
  if (u.role === 'admin' && disabled && !u.disabled && activeAdminCount.value <= 1) {
    Message.warning(t('users.lastAdminWarn'))
    await reload()
    return
  }
  try {
    await updateUser(u.id, { disabled })
    Message.success(t('msg.userUpdated'))
    await reload()
  } catch {
    await reload()
  }
}

function openUserReset(u: User) {
  userResetTarget.value = u
  userResetForm.new_password = ''
  userResetForm.confirm = ''
  userResetModal.value = true
}

async function submitUserReset() {
  if (!userResetTarget.value) return
  if (userResetForm.new_password.length < 6) {
    Message.warning(t('password.too_short'))
    return
  }
  if (userResetForm.new_password !== userResetForm.confirm) {
    Message.warning(t('password.mismatch'))
    return
  }
  userResetSubmitting.value = true
  try {
    await resetUserPassword(userResetTarget.value.id, userResetForm.new_password)
    Message.success(t('password.changed'))
    userResetModal.value = false
  } catch {
    /* interceptor */
  } finally {
    userResetSubmitting.value = false
  }
}

function askUserDelete(u: User) {
  if (isSelf(u)) {
    Message.warning(t('users.selfCannotModify'))
    return
  }
  confirmUserTarget.value = u
}

async function doUserDelete() {
  if (!confirmUserTarget.value) return
  const id = confirmUserTarget.value.id
  confirmUserTarget.value = null
  try {
    await deleteUser(id)
    Message.success(t('msg.userDeleted'))
    await reload()
  } catch {
    /* interceptor */
  }
}

function openRangesDrawer(u: User) {
  rangesDrawerUser.value = u
  rangesDrawer.value = true
}

const createStrength = computed(() => strengthOf(userCreateForm.password))
const resetStrength = computed(() => strengthOf(userResetForm.new_password))

function protoVariant(p: string) {
  return p === 'udp' ? 'secondary' : 'default'
}

const protocolOptions = ['tcp', 'udp', 'both'] as const
</script>

<template>
  <div class="pp-page flex flex-col gap-4">
    <!-- Header -->
    <header class="flex items-end justify-between gap-4 flex-wrap">
      <div>
        <h1 class="text-xl font-semibold text-foreground m-0">{{ t('settings.title') }}</h1>
        <p class="text-sm text-muted-foreground mt-1 m-0">{{ t('settings.subtitle') }}</p>
      </div>
      <Button variant="outline" size="sm" :disabled="loading" @click="reload">
        <RefreshCw :class="['size-4', loading && 'animate-spin']" />
        {{ t('action.refresh') }}
      </Button>
    </header>

    <!-- Runtime overview -->
    <section v-if="settings" class="grid grid-cols-2 md:grid-cols-4 gap-3">
      <div class="rounded-md border border-border bg-card p-3 md:p-4 flex items-center gap-3">
        <div class="size-10 rounded-md bg-primary/10 text-primary flex items-center justify-center shrink-0">
          <ShieldCheck class="size-5" />
        </div>
        <div class="min-w-0">
          <div class="text-xs text-muted-foreground">{{ t('settings.overviewAuth') }}</div>
          <div class="text-sm md:text-base font-semibold truncate">{{ settings.auth_mode }}</div>
        </div>
      </div>
      <div class="rounded-md border border-border bg-card p-3 md:p-4 flex items-center gap-3">
        <div class="size-10 rounded-md bg-emerald-500/10 text-emerald-600 flex items-center justify-center shrink-0">
          <Cog class="size-5" />
        </div>
        <div class="min-w-0">
          <div class="text-xs text-muted-foreground">{{ t('settings.overviewFirewall') }}</div>
          <div class="text-sm md:text-base font-semibold truncate">{{ settings.firewall_driver }}</div>
        </div>
      </div>
      <div class="rounded-md border border-border bg-card p-3 md:p-4 flex items-center gap-3">
        <div class="size-10 rounded-md bg-amber-500/10 text-amber-600 flex items-center justify-center shrink-0">
          <Clock class="size-5" />
        </div>
        <div class="min-w-0">
          <div class="text-xs text-muted-foreground">{{ t('settings.overviewMaxDuration') }}</div>
          <div class="text-sm md:text-base font-semibold truncate">{{ settings.max_duration_hours }} {{ t('settings.overviewHours') }}</div>
        </div>
      </div>
      <div class="rounded-md border border-border bg-card p-3 md:p-4 flex items-center gap-3">
        <div class="size-10 rounded-md bg-violet-500/10 text-violet-600 flex items-center justify-center shrink-0">
          <Database class="size-5" />
        </div>
        <div class="min-w-0">
          <div class="text-xs text-muted-foreground">{{ t('settings.overviewRetention') }}</div>
          <div class="text-sm md:text-base font-semibold truncate">{{ settings.history_retention_days }} {{ t('settings.overviewDays') }}</div>
        </div>
      </div>
    </section>

    <!-- Tabs -->
    <Tabs v-model="activeTab" class="w-full" @update:model-value="(v) => { if (v === 'security' && loginAttempts.length === 0) loadLoginHistory() }">
      <TabsList class="grid grid-cols-5 w-full md:w-auto md:inline-grid bg-muted/60 p-1 rounded-md">
        <TabsTrigger value="users" class="gap-1.5">
          <UsersIcon class="size-3.5" />
          <span class="hidden sm:inline">{{ t('settings.tabUsers') }}</span>
          <span class="sm:hidden">{{ t('settings.tabUsersMobile') }}</span>
          <Badge variant="default" class="text-[10px] h-4 px-1.5 ml-0.5">{{ users.length }}</Badge>
        </TabsTrigger>
        <TabsTrigger value="presets" class="gap-1.5">
          <Package class="size-3.5" />
          <span class="hidden sm:inline">{{ t('settings.tabPresets') }}</span>
          <span class="sm:hidden">{{ t('settings.tabPresetsMobile') }}</span>
          <Badge variant="default" class="text-[10px] h-4 px-1.5 ml-0.5">{{ presetCount }}</Badge>
        </TabsTrigger>
        <TabsTrigger value="protected" class="gap-1.5">
          <AlertTriangle class="size-3.5" />
          <span class="hidden sm:inline">{{ t('settings.tabProtected') }}</span>
          <span class="sm:hidden">{{ t('settings.tabProtectedMobile') }}</span>
          <Badge variant="destructive" class="text-[10px] h-4 px-1.5 ml-0.5">{{ protectedPorts.length }}</Badge>
        </TabsTrigger>
        <TabsTrigger value="security" class="gap-1.5">
          <History class="size-3.5" />
          <span class="hidden sm:inline">{{ t('security.title') }}</span>
          <span class="sm:hidden">{{ t('settings.tabSecurityMobile') }}</span>
        </TabsTrigger>
        <TabsTrigger value="runtime" class="gap-1.5">
          <SettingsIcon class="size-3.5" />
          <span class="hidden sm:inline">{{ t('settings.tabRuntime') }}</span>
          <span class="sm:hidden">{{ t('settings.tabRuntimeMobile') }}</span>
        </TabsTrigger>
      </TabsList>

      <!-- Users -->
      <TabsContent value="users" class="mt-4 rounded-lg border border-border bg-card p-4 md:p-6 flex flex-col gap-4">
        <div class="flex justify-between items-center gap-3 flex-wrap">
          <p class="text-sm text-muted-foreground m-0">
            {{ t('settings.usersCount', { n: users.length }) }} ·
            {{ t('settings.activeAdmins', { n: activeAdminCount }) }}
          </p>
          <Button @click="openUserCreate">
            <Plus class="size-4" />
            {{ t('action.new_user') }}
          </Button>
        </div>

        <EmptyState
          v-if="!users.length && !loading"
          icon="👥"
          :title="t('settings.usersEmpty')"
          :description="t('settings.usersEmptyDesc')"
        />

        <!-- Desktop table -->
        <div v-else class="hidden md:block">
          <Table>
            <TableHeader>
              <TableRow class="bg-muted/50 hover:bg-muted/50">
                <TableHead>{{ t('users.username') }}</TableHead>
                <TableHead class="w-[160px]">{{ t('users.role') }}</TableHead>
                <TableHead class="w-[100px]">{{ t('users.disabled') }}</TableHead>
                <TableHead class="w-[180px]">{{ t('userRanges.column') }}</TableHead>
                <TableHead class="w-[120px] text-right">{{ t('users.actions') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="u in users" :key="u.id">
                <TableCell>
                  <div class="flex items-center gap-3 min-w-0">
                    <span
                      class="size-9 rounded-full inline-flex items-center justify-center text-white font-semibold text-sm shrink-0"
                      :style="{ background: avatarColor(u.username) }"
                    >
                      {{ avatarLetter(u.username) }}
                    </span>
                    <div class="flex flex-col min-w-0">
                      <div class="text-sm font-medium text-foreground flex items-center gap-1.5">
                        <span class="truncate">{{ u.username }}</span>
                        <Badge v-if="isSelf(u)" variant="default" class="text-[10px] h-4 px-1.5">{{ t('users.you') }}</Badge>
                      </div>
                      <div class="text-[11px] text-muted-foreground">ID #{{ u.id }}</div>
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  <Select
                    :model-value="u.role"
                    :disabled="isSelf(u)"
                    @update:model-value="(v: string) => changeRole(u, v as Role)"
                  >
                    <SelectTrigger class="h-8 w-[130px]">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="admin">{{ t('users.roleAdmin') }}</SelectItem>
                      <SelectItem value="user">{{ t('users.roleUser') }}</SelectItem>
                    </SelectContent>
                  </Select>
                </TableCell>
                <TableCell>
                  <Switch
                    :model-value="!u.disabled"
                    :disabled="isSelf(u)"
                    @update:model-value="(v: boolean) => toggleDisabled(u, v)"
                  />
                </TableCell>
                <TableCell>
                  <Button
                    v-if="u.role !== 'admin'"
                    variant="ghost"
                    size="sm"
                    class="h-8"
                    @click="openRangesDrawer(u)"
                  >
                    <span v-if="userRangeCounts[u.id]" class="text-foreground">
                      {{ t('userRanges.colCount', { n: userRangeCounts[u.id] }) }}
                    </span>
                    <span v-else class="text-muted-foreground">{{ t('userRanges.colDefault') }}</span>
                  </Button>
                  <span v-else class="text-muted-foreground text-sm">—</span>
                </TableCell>
                <TableCell class="text-right whitespace-nowrap">
                  <div class="inline-flex gap-0.5">
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <Button variant="ghost" size="icon" class="size-8" @click="openUserReset(u)">
                          <Lock class="size-4" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{{ t('action.reset_password') }}</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <Button
                          variant="ghost"
                          size="icon"
                          class="size-8 text-destructive hover:bg-destructive/10 hover:text-destructive"
                          :disabled="isSelf(u)"
                          @click="askUserDelete(u)"
                        >
                          <Trash2 class="size-4" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>
                        {{ isSelf(u) ? t('users.selfCannotModify') : t('action.delete') }}
                      </TooltipContent>
                    </Tooltip>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>

        <!-- Mobile cards -->
        <div v-if="users.length" class="md:hidden flex flex-col gap-2.5">
          <div
            v-for="u in users"
            :key="u.id"
            class="rounded-md border border-border bg-card p-4 flex flex-col gap-3"
          >
            <div class="flex items-center gap-3">
              <span
                class="size-10 rounded-full inline-flex items-center justify-center text-white font-semibold text-sm shrink-0"
                :style="{ background: avatarColor(u.username) }"
              >
                {{ avatarLetter(u.username) }}
              </span>
              <div class="flex flex-col min-w-0 flex-1">
                <div class="flex items-center gap-1.5">
                  <span class="font-medium text-foreground truncate">{{ u.username }}</span>
                  <Badge v-if="isSelf(u)" variant="default" class="text-[10px] h-4 px-1.5">{{ t('users.you') }}</Badge>
                </div>
                <div class="flex items-center gap-1.5 mt-1">
                  <Badge
                    v-if="u.role === 'admin'"
                    variant="default"
                    class="text-[10px]"
                  >{{ t('users.roleAdmin') }}</Badge>
                  <Badge
                    v-else
                    variant="muted"
                    class="text-[10px]"
                  >{{ t('users.roleUser') }}</Badge>
                  <Badge
                    v-if="u.disabled"
                    variant="destructive"
                    class="text-[10px]"
                  >{{ t('users.disabled') }}</Badge>
                </div>
              </div>
            </div>
            <div class="flex gap-1.5 flex-wrap">
              <Button
                v-if="u.role !== 'admin'"
                variant="outline"
                size="sm"
                class="flex-1 text-xs"
                @click="openRangesDrawer(u)"
              >
                {{ userRangeCounts[u.id]
                  ? t('userRanges.colCount', { n: userRangeCounts[u.id] })
                  : t('userRanges.colDefault') }}
              </Button>
              <Button variant="outline" size="sm" class="text-xs" @click="openUserReset(u)">
                <Lock class="size-3.5" />
                {{ t('action.reset_password') }}
              </Button>
              <Button
                variant="outline"
                size="sm"
                class="text-xs text-destructive border-destructive/30 hover:bg-destructive/10"
                :disabled="isSelf(u)"
                @click="askUserDelete(u)"
              >
                <Trash2 class="size-3.5" />
                {{ t('action.delete') }}
              </Button>
            </div>
          </div>
        </div>
      </TabsContent>

      <!-- Presets -->
      <TabsContent value="presets" class="mt-4 rounded-lg border border-border bg-card p-4 md:p-6 flex flex-col gap-4">
        <div class="flex justify-between items-center gap-3 flex-wrap">
          <p class="text-sm text-muted-foreground m-0">
            {{ t('settings.presetHint') }}，{{ t('settings.presetVisibility', { allowed: userAllowedCount, total: presetCount }) }}
          </p>
          <Button @click="openPresetCreate">
            <Plus class="size-4" />
            {{ t('settings.newPreset') }}
          </Button>
        </div>

        <EmptyState
          v-if="!presets.length && !loading"
          icon="📭"
          :title="t('settings.presetsEmpty')"
          :description="t('settings.presetsEmptyDesc')"
        />

        <!-- Desktop table -->
        <div v-else class="hidden md:block">
          <Table>
            <TableHeader>
              <TableRow class="bg-muted/50 hover:bg-muted/50">
                <TableHead class="w-[220px]">{{ t('settings.presetName') }}</TableHead>
                <TableHead>{{ t('settings.presetPortsProto') }}</TableHead>
                <TableHead class="w-[140px]">{{ t('settings.userAllowed') }}</TableHead>
                <TableHead class="w-[160px]">{{ t('settings.maxDurationSec') }}</TableHead>
                <TableHead class="w-[80px] text-center">{{ t('settings.presetSort') }}</TableHead>
                <TableHead class="w-[100px] text-right">{{ t('rules.actions') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="p in presets" :key="p.id">
                <TableCell>
                  <span class="inline-flex items-center gap-2">
                    <span class="inline-flex items-center justify-center size-5 shrink-0">
                      <img
                        v-if="isImageIcon(presetIcon(p))"
                        :src="presetIcon(p)"
                        class="size-5 rounded-md object-cover"
                        referrerpolicy="no-referrer"
                      />
                      <span v-else class="text-base">{{ presetIcon(p) }}</span>
                    </span>
                    <span class="font-medium">{{ p.name }}</span>
                  </span>
                </TableCell>
                <TableCell>
                  <div class="inline-flex items-center gap-2">
                    <code class="font-mono font-semibold text-sm text-foreground">{{ p.ports || p.port }}</code>
                    <Badge :variant="protoVariant(p.protocol)" class="text-[10px] px-1.5 py-0">
                      {{ p.protocol.toUpperCase() }}
                    </Badge>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge v-if="p.user_allowed" variant="success" class="text-[11px]">{{ t('settings.presetAvailable') }}</Badge>
                  <Badge v-else variant="muted" class="text-[11px] gap-1">
                    <Lock class="size-3" />
                    {{ t('settings.presetAdminOnly') }}
                  </Badge>
                </TableCell>
                <TableCell>
                  <span :class="p.max_duration_sec ? 'font-mono text-sm text-foreground' : 'text-muted-foreground'">
                    {{ fmtDuration(p.max_duration_sec) }}
                  </span>
                </TableCell>
                <TableCell class="text-center text-sm text-muted-foreground">{{ p.sort }}</TableCell>
                <TableCell class="text-right whitespace-nowrap">
                  <div class="inline-flex gap-0.5">
                    <Button variant="ghost" size="icon" class="size-8" @click="openPresetEdit(p)">
                      <Pencil class="size-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      class="size-8 text-destructive hover:bg-destructive/10 hover:text-destructive"
                      @click="confirmPresetTarget = p"
                    >
                      <Trash2 class="size-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>

        <!-- Mobile cards -->
        <div v-if="presets.length" class="md:hidden flex flex-col gap-2.5">
          <div
            v-for="p in presets"
            :key="p.id"
            class="rounded-md border border-border bg-card p-4 flex flex-col gap-2.5"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="inline-flex items-center gap-2 min-w-0">
                <span class="inline-flex items-center justify-center size-5 shrink-0">
                  <img
                    v-if="isImageIcon(presetIcon(p))"
                    :src="presetIcon(p)"
                    class="size-5 rounded-md object-cover"
                    referrerpolicy="no-referrer"
                  />
                  <span v-else class="text-base">{{ presetIcon(p) }}</span>
                </span>
                <strong class="truncate">{{ p.name }}</strong>
              </span>
              <code class="font-mono text-xs text-foreground">{{ p.ports || p.port }}/{{ p.protocol }}</code>
            </div>
            <div class="grid grid-cols-2 gap-y-1.5 gap-x-4 text-sm">
              <div class="flex flex-col gap-0.5">
                <span class="text-[11px] text-muted-foreground">{{ t('settings.userAllowed') }}</span>
                <Badge v-if="p.user_allowed" variant="success" class="text-[10px] w-fit">{{ t('settings.presetAvailable') }}</Badge>
                <Badge v-else variant="muted" class="text-[10px] w-fit">{{ t('settings.presetAdminOnly') }}</Badge>
              </div>
              <div class="flex flex-col gap-0.5">
                <span class="text-[11px] text-muted-foreground">{{ t('settings.maxDurationSec') }}</span>
                <span :class="p.max_duration_sec ? 'font-mono text-xs' : 'text-xs text-muted-foreground'">
                  {{ fmtDuration(p.max_duration_sec) }}
                </span>
              </div>
            </div>
            <div class="flex gap-1.5">
              <Button variant="outline" size="sm" class="flex-1 text-xs" @click="openPresetEdit(p)">
                <Pencil class="size-3.5" />
                {{ t('action.edit') }}
              </Button>
              <Button
                variant="outline"
                size="sm"
                class="flex-1 text-xs text-destructive border-destructive/30 hover:bg-destructive/10"
                @click="confirmPresetTarget = p"
              >
                <Trash2 class="size-3.5" />
                {{ t('action.delete') }}
              </Button>
            </div>
          </div>
        </div>
      </TabsContent>

      <!-- Protected ports -->
      <TabsContent value="protected" class="mt-4 rounded-lg border border-border bg-card p-4 md:p-6 flex flex-col gap-4">
        <div class="flex justify-between items-center gap-3 flex-wrap">
          <p class="text-sm text-muted-foreground m-0">
            {{ t('settings.protectedHint') }}
          </p>
          <Button @click="openProtectedCreate">
            <Plus class="size-4" />
            {{ t('protected.new') }}
          </Button>
        </div>

        <EmptyState
          v-if="!protectedPorts.length && !loading"
          icon="🛡️"
          :title="t('protected.empty')"
          :description="t('protected.subtitle')"
        />

        <!-- Desktop table -->
        <div v-else class="hidden md:block">
          <Table>
            <TableHeader>
              <TableRow class="bg-muted/50 hover:bg-muted/50">
                <TableHead class="w-[220px]">{{ t('protected.name') }}</TableHead>
                <TableHead>{{ t('protected.ports') }}</TableHead>
                <TableHead>{{ t('protected.note') }}</TableHead>
                <TableHead class="w-[100px] text-right">{{ t('rules.actions') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="p in protectedPorts" :key="p.id">
                <TableCell class="font-medium">{{ p.name }}</TableCell>
                <TableCell>
                  <div class="inline-flex items-center gap-2">
                    <code class="font-mono font-semibold text-sm">{{ p.ports }}</code>
                    <Badge :variant="protoVariant(p.protocol)" class="text-[10px] px-1.5 py-0">
                      {{ p.protocol.toUpperCase() }}
                    </Badge>
                  </div>
                </TableCell>
                <TableCell class="max-w-0">
                  <Tooltip v-if="p.note">
                    <TooltipTrigger as-child>
                      <span class="block truncate text-sm text-muted-foreground">{{ p.note }}</span>
                    </TooltipTrigger>
                    <TooltipContent class="max-w-xs whitespace-pre-wrap">{{ p.note }}</TooltipContent>
                  </Tooltip>
                  <span v-else class="text-muted-foreground text-sm">—</span>
                </TableCell>
                <TableCell class="text-right whitespace-nowrap">
                  <div class="inline-flex gap-0.5">
                    <Button variant="ghost" size="icon" class="size-8" @click="openProtectedEdit(p)">
                      <Pencil class="size-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      class="size-8 text-destructive hover:bg-destructive/10 hover:text-destructive"
                      @click="confirmProtectedTarget = p"
                    >
                      <Trash2 class="size-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>

        <!-- Mobile cards -->
        <div v-if="protectedPorts.length" class="md:hidden flex flex-col gap-2.5">
          <div
            v-for="p in protectedPorts"
            :key="p.id"
            class="rounded-md border border-border bg-card p-4 flex flex-col gap-2.5"
          >
            <div class="flex items-center justify-between gap-2">
              <strong class="truncate">{{ p.name }}</strong>
              <code class="font-mono text-xs">{{ p.ports }}/{{ p.protocol }}</code>
            </div>
            <div v-if="p.note" class="text-xs text-muted-foreground">{{ p.note }}</div>
            <div class="flex gap-1.5">
              <Button variant="outline" size="sm" class="flex-1 text-xs" @click="openProtectedEdit(p)">
                <Pencil class="size-3.5" />
                {{ t('action.edit') }}
              </Button>
              <Button
                variant="outline"
                size="sm"
                class="flex-1 text-xs text-destructive border-destructive/30 hover:bg-destructive/10"
                @click="confirmProtectedTarget = p"
              >
                <Trash2 class="size-3.5" />
                {{ t('action.delete') }}
              </Button>
            </div>
          </div>
        </div>
      </TabsContent>

      <!-- Security / Login history -->
      <TabsContent value="security" class="mt-4 rounded-lg border border-border bg-card p-4 md:p-6 flex flex-col gap-4">
        <div class="flex justify-between items-center gap-3 flex-wrap">
          <p class="text-sm text-muted-foreground m-0">{{ t('security.subtitle') }}</p>
          <div class="flex items-center gap-2">
            <Input
              v-model="loginAttemptsFilter"
              :placeholder="t('security.usernameFilter')"
              class="h-9 w-44"
              @keydown.enter="loadLoginHistory"
            />
            <Button variant="outline" :disabled="loginAttemptsLoading" @click="loadLoginHistory">
              <RefreshCw class="size-4" :class="{ 'animate-spin': loginAttemptsLoading }" />
              {{ t('action.refresh') }}
            </Button>
          </div>
        </div>

        <EmptyState
          v-if="!loginAttemptsLoading && loginAttempts.length === 0"
          icon="🛡️"
          :title="t('security.empty')"
          :description="t('security.subtitle')"
        />

        <div v-else class="hidden md:block">
          <Table>
            <TableHeader>
              <TableRow class="bg-muted/50 hover:bg-muted/50">
                <TableHead class="w-[180px]">{{ t('security.columns.time') }}</TableHead>
                <TableHead class="w-[140px]">{{ t('security.columns.username') }}</TableHead>
                <TableHead class="w-[160px]">{{ t('security.columns.ip') }}</TableHead>
                <TableHead class="w-[110px]">{{ t('security.columns.result') }}</TableHead>
                <TableHead class="w-[180px]">{{ t('security.columns.reason') }}</TableHead>
                <TableHead>{{ t('security.columns.ua') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="a in loginAttempts" :key="a.id">
                <TableCell class="font-mono text-xs tabular-nums">
                  {{ dayjs(a.created_at).format('YYYY-MM-DD HH:mm:ss') }}
                </TableCell>
                <TableCell class="font-medium">{{ a.username || '—' }}</TableCell>
                <TableCell class="font-mono text-xs">{{ a.client_ip || '—' }}</TableCell>
                <TableCell>
                  <Badge v-if="a.success" variant="default" class="gap-1">
                    <Check class="size-3" />
                    {{ t('security.success') }}
                  </Badge>
                  <Badge v-else variant="destructive" class="gap-1">
                    <XIcon class="size-3" />
                    {{ t('security.failure') }}
                  </Badge>
                </TableCell>
                <TableCell class="text-xs text-muted-foreground">{{ a.reason || '—' }}</TableCell>
                <TableCell class="text-xs text-muted-foreground max-w-[320px] truncate" :title="a.user_agent">
                  {{ a.user_agent || '—' }}
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>

        <!-- Mobile list -->
        <div class="md:hidden flex flex-col gap-2">
          <div
            v-for="a in loginAttempts"
            :key="a.id"
            class="rounded-md border border-border bg-card p-3 flex flex-col gap-1.5"
          >
            <div class="flex items-center justify-between gap-2">
              <div class="text-sm font-medium truncate">{{ a.username || '—' }}</div>
              <Badge v-if="a.success" variant="default" class="gap-1">
                <Check class="size-3" />
                {{ t('security.success') }}
              </Badge>
              <Badge v-else variant="destructive" class="gap-1">
                <XIcon class="size-3" />
                {{ t('security.failure') }}
              </Badge>
            </div>
            <div class="text-xs font-mono text-muted-foreground">{{ a.client_ip || '—' }}</div>
            <div class="text-xs text-muted-foreground">{{ dayjs(a.created_at).format('YYYY-MM-DD HH:mm:ss') }}</div>
            <div v-if="a.reason" class="text-xs text-muted-foreground">{{ a.reason }}</div>
          </div>
        </div>
      </TabsContent>

      <!-- Runtime -->
      <TabsContent value="runtime" class="mt-4 rounded-lg border border-border bg-card p-4 md:p-6 flex flex-col gap-4">
        <RuntimeSettingsForm @saved="reload" />
      </TabsContent>
    </Tabs>

    <!-- Preset modal -->
    <Dialog v-model:open="presetEditVisible">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {{ isEditingPreset ? t('settings.editPreset') : t('settings.newPreset') }}
          </DialogTitle>
        </DialogHeader>
        <div class="flex flex-col gap-4">
          <div class="grid grid-cols-3 gap-3">
            <div class="col-span-2 flex flex-col gap-1.5">
              <Label>{{ t('settings.presetName') }}</Label>
              <Input v-model="presetEditing.name" :placeholder="locale === 'zh-CN' ? '例如 SSH' : 'e.g. SSH'" />
            </div>
            <div class="flex flex-col gap-1.5">
              <Label>{{ t('settings.presetSort') }}</Label>
              <Input
                v-model="presetEditing.sort"
                type="number"
                :min="0"
                :max="999"
              />
            </div>
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('settings.presetPorts') }}</Label>
            <PortSetInput
              v-model="presetEditing.ports as string"
              :placeholder="t('portSet.placeholder')"
              @validation="(ok: boolean, error: string | null) => (presetPortsValid = { ok, error })"
            />
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('settings.presetProtocol') }}</Label>
            <div class="inline-flex p-1 rounded-md bg-muted/60 border border-border w-fit">
              <button
                v-for="p in protocolOptions"
                :key="p"
                type="button"
                class="px-3 h-8 rounded text-xs font-medium transition-colors"
                :class="presetEditing.protocol === p
                  ? 'bg-card text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'"
                @click="presetEditing.protocol = p"
              >
                {{ p === 'both' ? 'TCP+UDP' : p.toUpperCase() }}
              </button>
            </div>
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('settings.presetCategory') }}</Label>
            <PresetCategorySelect
              :model-value="presetEditing.category_id ?? null"
              :categories="presetCategories"
              @update:model-value="(v: number | null) => (presetEditing.category_id = v)"
              @update:categories="(v: PresetCategory[]) => (presetCategories = v)"
            />
          </div>
          <div class="flex items-center justify-between rounded-md bg-muted/40 p-3">
            <div class="flex flex-col gap-0.5">
              <Label class="cursor-pointer">{{ t('settings.userAllowed') }}</Label>
              <span class="text-xs text-muted-foreground">{{ t('settings.userAllowedHelp') }}</span>
            </div>
            <Switch v-model="presetEditing.user_allowed" />
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('settings.maxDurationSec') }}</Label>
            <Input
              v-model="presetEditing.max_duration_sec"
              type="number"
              :min="0"
              :max="24 * 3600"
              :step="300"
            />
            <span class="text-xs text-muted-foreground">{{ t('settings.maxDurationSecHelp') }}</span>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="presetEditVisible = false">{{ t('common.cancel') }}</Button>
          <Button @click="savePreset">{{ t('common.save') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Protected modal -->
    <Dialog v-model:open="protectedEditVisible">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {{ protectedEditing.id ? t('protected.edit') : t('protected.new') }}
          </DialogTitle>
        </DialogHeader>
        <div class="flex flex-col gap-4">
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('protected.name') }}</Label>
            <Input v-model="protectedEditing.name" placeholder="App backend" />
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('protected.ports') }}</Label>
            <PortSetInput
              v-model="protectedEditing.ports as string"
              :placeholder="t('portSet.placeholder')"
              @validation="(ok: boolean, error: string | null) => (protectedPortsValid = { ok, error })"
            />
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('protected.protocol') }}</Label>
            <div class="inline-flex p-1 rounded-md bg-muted/60 border border-border w-fit">
              <button
                v-for="p in protocolOptions"
                :key="p"
                type="button"
                class="px-3 h-8 rounded text-xs font-medium transition-colors"
                :class="protectedEditing.protocol === p
                  ? 'bg-card text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'"
                @click="protectedEditing.protocol = p"
              >
                {{ p === 'both' ? 'TCP+UDP' : p.toUpperCase() }}
              </button>
            </div>
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('protected.note') }}</Label>
            <Input v-model="protectedEditing.note" placeholder="Optional" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="protectedEditVisible = false">{{ t('common.cancel') }}</Button>
          <Button @click="saveProtected">{{ t('common.save') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- User create modal -->
    <Dialog v-model:open="userCreateModal">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('users.newUser') }}</DialogTitle>
        </DialogHeader>
        <div class="flex flex-col gap-4">
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('users.username') }}</Label>
            <Input v-model="userCreateForm.username" autocomplete="off" :placeholder="locale === 'zh-CN' ? '例如 alice' : 'e.g. alice'" />
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('password.new') }}</Label>
            <Input v-model="userCreateForm.password" type="password" autocomplete="new-password" />
            <div v-if="userCreateForm.password" class="flex items-center gap-2 mt-1">
              <div class="flex gap-1 flex-1">
                <span
                  v-for="i in 5"
                  :key="i"
                  class="flex-1 h-1 rounded-full"
                  :class="i <= createStrength.score
                    ? createStrength.score <= 2 ? 'bg-red-500' : createStrength.score === 3 ? 'bg-amber-500' : createStrength.score === 4 ? 'bg-yellow-400' : 'bg-emerald-500'
                    : 'bg-muted'"
                />
              </div>
              <span class="text-xs text-muted-foreground w-8 text-right">{{ createStrength.label }}</span>
            </div>
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('users.role') }}</Label>
            <div class="inline-flex p-1 rounded-md bg-muted/60 border border-border w-fit">
              <button
                type="button"
                class="px-3 h-8 rounded text-xs font-medium transition-colors"
                :class="userCreateForm.role === 'user'
                  ? 'bg-card text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'"
                @click="userCreateForm.role = 'user'"
              >
                {{ t('users.roleUser') }}
              </button>
              <button
                type="button"
                class="px-3 h-8 rounded text-xs font-medium transition-colors"
                :class="userCreateForm.role === 'admin'
                  ? 'bg-card text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'"
                @click="userCreateForm.role = 'admin'"
              >
                {{ t('users.roleAdmin') }}
              </button>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="userCreateModal = false">{{ t('common.cancel') }}</Button>
          <Button :disabled="userCreateSubmitting" @click="submitUserCreate">
            {{ t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- User reset password -->
    <Dialog v-model:open="userResetModal">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {{ t('users.resetPwd') }}
            <span v-if="userResetTarget" class="text-muted-foreground font-normal">
              · {{ userResetTarget.username }}
            </span>
          </DialogTitle>
        </DialogHeader>
        <div class="flex flex-col gap-4">
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('password.new') }}</Label>
            <Input v-model="userResetForm.new_password" type="password" autocomplete="new-password" />
            <div v-if="userResetForm.new_password" class="flex items-center gap-2 mt-1">
              <div class="flex gap-1 flex-1">
                <span
                  v-for="i in 5"
                  :key="i"
                  class="flex-1 h-1 rounded-full"
                  :class="i <= resetStrength.score
                    ? resetStrength.score <= 2 ? 'bg-red-500' : resetStrength.score === 3 ? 'bg-amber-500' : resetStrength.score === 4 ? 'bg-yellow-400' : 'bg-emerald-500'
                    : 'bg-muted'"
                />
              </div>
              <span class="text-xs text-muted-foreground w-8 text-right">{{ resetStrength.label }}</span>
            </div>
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('password.confirm') }}</Label>
            <Input v-model="userResetForm.confirm" type="password" autocomplete="new-password" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="userResetModal = false">{{ t('common.cancel') }}</Button>
          <Button :disabled="userResetSubmitting" @click="submitUserReset">
            {{ t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Preset delete confirm -->
    <Dialog :open="!!confirmPresetTarget" @update:open="(v: boolean) => !v && (confirmPresetTarget = null)">
      <DialogContent class="max-w-sm">
        <DialogHeader>
          <DialogTitle>
            {{ t('action.delete') }}
            <span v-if="confirmPresetTarget" class="text-muted-foreground font-normal">
              · {{ confirmPresetTarget.name }}
            </span>
          </DialogTitle>
        </DialogHeader>
        <div v-if="confirmPresetTarget" class="text-sm text-muted-foreground">
          {{ confirmPresetTarget.ports || confirmPresetTarget.port }}/{{ confirmPresetTarget.protocol }}
        </div>
        <DialogFooter>
          <Button variant="outline" @click="confirmPresetTarget = null">{{ t('common.cancel') }}</Button>
          <Button variant="destructive" @click="doRemovePreset">{{ t('action.delete') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Protected delete confirm -->
    <Dialog :open="!!confirmProtectedTarget" @update:open="(v: boolean) => !v && (confirmProtectedTarget = null)">
      <DialogContent class="max-w-sm">
        <DialogHeader>
          <DialogTitle>
            {{ t('action.delete') }}
            <span v-if="confirmProtectedTarget" class="text-muted-foreground font-normal">
              · {{ confirmProtectedTarget.name }}
            </span>
          </DialogTitle>
        </DialogHeader>
        <div v-if="confirmProtectedTarget" class="text-sm text-muted-foreground">
          {{ confirmProtectedTarget.ports }}/{{ confirmProtectedTarget.protocol }}
        </div>
        <DialogFooter>
          <Button variant="outline" @click="confirmProtectedTarget = null">{{ t('common.cancel') }}</Button>
          <Button variant="destructive" @click="doRemoveProtected">{{ t('action.delete') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- User delete confirm -->
    <Dialog :open="!!confirmUserTarget" @update:open="(v: boolean) => !v && (confirmUserTarget = null)">
      <DialogContent class="max-w-sm">
        <DialogHeader>
          <DialogTitle>{{ t('action.delete') }}</DialogTitle>
        </DialogHeader>
        <div class="text-sm text-muted-foreground">
          {{ t('users.deleteConfirm') }}
        </div>
        <DialogFooter>
          <Button variant="outline" @click="confirmUserTarget = null">{{ t('common.cancel') }}</Button>
          <Button variant="destructive" @click="doUserDelete">{{ t('action.delete') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <UserRangesDrawer
      v-model:visible="rangesDrawer"
      :user="rangesDrawerUser"
      @changed="refreshRangeCounts"
    />
  </div>
</template>
