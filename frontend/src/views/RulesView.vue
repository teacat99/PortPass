<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import dayjs from 'dayjs'
import {
  RefreshCw, Plus, XCircle, Clock, Copy, Search as SearchIcon,
  Bell, BellOff
} from 'lucide-vue-next'
import { duplicateRule, extendRule, setRuleNotify, terminateRule } from '@/api/rules'
import { useRulesStore } from '@/stores/rules'
import { useNotifyStore } from '@/stores/notify'
import type { Rule } from '@/api/types'
import { Message } from '@/lib/toast'

import EmptyState from '@/components/EmptyState.vue'
import CountdownChip from '@/components/CountdownChip.vue'
import CopyableText from '@/components/CopyableText.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Table, TableHeader, TableBody, TableRow, TableHead, TableCell
} from '@/components/ui/table'
import {
  Tooltip, TooltipTrigger, TooltipContent
} from '@/components/ui/tooltip'
import {
  Dialog, DialogContent, DialogHeader, DialogFooter, DialogTitle
} from '@/components/ui/dialog'

const { t } = useI18n()
const router = useRouter()
const store = useRulesStore()
const notifyStore = useNotifyStore()

// notifyToggling tracks the per-row in-flight set. We block re-clicks
// on the same rule until the round-trip resolves, otherwise rapid
// double-clicks would spawn two independent permission prompts and
// race the audit log.
const notifyToggling = ref<Set<number>>(new Set())

const extendVisible = ref(false)
const extendTarget = ref<Rule | null>(null)
const extendSec = ref<number>(60 * 60)

const search = ref('')
const confirmTarget = ref<Rule | null>(null)

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
    String(r.ports || r.port).includes(q)
    || r.source_ip.toLowerCase().includes(q)
    || (r.note ?? '').toLowerCase().includes(q)
    || (r.created_by ?? '').toLowerCase().includes(q)
  )
})

function askTerminate(rule: Rule) { confirmTarget.value = rule }

async function doTerminate() {
  if (!confirmTarget.value) return
  const id = confirmTarget.value.id
  confirmTarget.value = null
  await terminateRule(id)
  Message.success(t('msg.ruleTerminated'))
  await store.reload()
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

function protoVariant(p: string) {
  return p === 'udp' ? 'secondary' : 'default'
}

// notifyTooltip composes the tooltip text for the per-row bell icon.
// We prefer "already notified at <time>" when one of the channels has
// fired so the operator can correlate a recent toast/ntfy push back to
// the rule; otherwise we surface the lead time so they know when the
// next ping will go out.
function notifyTooltip(r: Rule): string {
  const sentAt = r.notify_sent_browser_at || r.notify_sent_ntfy_at
  if (sentAt) {
    return t('rules.notifyAlreadySent', { time: dayjs(sentAt).format('HH:mm:ss') })
  }
  const leadMin = Math.max(1, Math.round((r.notify_lead_seconds || 0) / 60))
  return t('rules.notifyOn', { n: leadMin })
}

// ensureBrowserNotifyPermission mirrors the HomeView gating logic but
// keeps the result structured so callers can decide how to surface the
// failure (HomeView shows an inline hint, RulesView uses a toast).
async function ensureBrowserNotifyPermission(): Promise<{ ok: boolean; reason?: string }> {
  if (typeof Notification === 'undefined') {
    return { ok: false, reason: t('home.notifyPermissionUnsupported') }
  }
  if (!window.isSecureContext && location.hostname !== 'localhost') {
    return { ok: false, reason: t('home.notifyContextInsecure') }
  }
  if (Notification.permission === 'granted') return { ok: true }
  if (Notification.permission === 'denied') {
    return { ok: false, reason: t('home.notifyPermissionDenied') }
  }
  let result: NotificationPermission = 'default'
  try {
    result = await Notification.requestPermission()
  } catch {
    return { ok: false, reason: t('home.notifyPermissionUnsupported') }
  }
  return result === 'granted'
    ? { ok: true }
    : { ok: false, reason: t('home.notifyPermissionDenied') }
}

// toggleNotify flips notify_enabled on a rule via the backend, gating
// the UI on a per-row in-flight set so rapid double-clicks don't spawn
// two permission prompts. When turning on under the browser channel we
// prompt for permission first; if the channel is "both" we still allow
// the flip even if the browser permission is denied (ntfy will deliver),
// and just surface a warning toast so the operator knows.
async function toggleNotify(r: Rule) {
  if (notifyToggling.value.has(r.id)) return
  notifyToggling.value.add(r.id)
  try {
    const next = !r.notify_enabled
    if (next) {
      const channels = notifyStore.settings?.channels ?? 'browser'
      if (channels === 'browser' || channels === 'both') {
        const perm = await ensureBrowserNotifyPermission()
        if (!perm.ok) {
          if (channels === 'browser') {
            Message.warning(perm.reason || t('home.notifyPermissionDenied'))
            return
          }
          Message.warning(perm.reason || t('home.notifyPermissionDenied'))
        }
      }
    }
    await setRuleNotify(r.id, next)
    Message.success(next ? t('msg.ruleNotifyEnabled') : t('msg.ruleNotifyDisabled'))
    await store.reload()
  } finally {
    notifyToggling.value.delete(r.id)
  }
}
</script>

<template>
  <div class="pp-page flex flex-col gap-4">
    <!-- Header -->
    <header class="flex items-end justify-between gap-4 flex-wrap">
      <div>
        <h1 class="text-xl font-semibold text-foreground m-0">{{ t('rules.title') }}</h1>
        <p class="text-sm text-muted-foreground mt-1 m-0">
          {{ t('rules.subtitle', { n: store.active.length }) }}
          <span v-if="search && filtered.length !== store.active.length">
            · {{ t('rules.filteredHint', { n: filtered.length }) }}
          </span>
        </p>
      </div>
      <div class="flex gap-2 items-center flex-wrap w-full md:w-auto">
        <div class="relative w-full md:w-60">
          <SearchIcon class="absolute left-2.5 top-1/2 -translate-y-1/2 size-4 text-muted-foreground pointer-events-none" />
          <Input
            v-model="search"
            :placeholder="t('rules.searchPlaceholder')"
            class="pl-8 h-9 bg-card"
          />
        </div>
        <Button variant="outline" size="sm" class="bg-card" :disabled="store.loading" @click="store.reload()">
          <RefreshCw :class="['size-4', store.loading && 'animate-spin']" />
          <span class="hidden sm:inline">{{ t('action.refresh') }}</span>
        </Button>
        <Button size="sm" @click="router.push({ name: 'home' })">
          <Plus class="size-4" />
          <span class="hidden sm:inline">{{ t('action.create') }}</span>
        </Button>
      </div>
    </header>

    <!-- Card wrapper -->
    <div class="rounded-lg border border-border bg-card overflow-hidden">
      <!-- Loading skeleton -->
      <div v-if="store.loading && !store.active.length" class="p-6 flex flex-col gap-4">
        <div v-for="i in 3" :key="i" class="flex gap-3 items-center">
          <div class="h-4 w-16 rounded bg-muted animate-pulse" />
          <div class="h-4 flex-1 rounded bg-muted animate-pulse" />
        </div>
      </div>

      <!-- Empty -->
      <EmptyState
        v-else-if="!filtered.length && !search"
          icon="🛡️"
          :title="t('rules.emptyTitle')"
          :description="t('rules.emptyDesc')"
      >
        <template #action>
          <Button @click="router.push({ name: 'home' })">
            <Plus class="size-4" />
            {{ t('action.create') }}
          </Button>
        </template>
      </EmptyState>

      <EmptyState
        v-else-if="!filtered.length && search"
          icon="🔍"
          :title="t('rules.noMatch')"
          :description="t('rules.noMatchDesc', { q: search })"
      />

      <!-- Desktop table -->
      <div v-else class="hidden md:block">
        <Table :container-class="'border-0 rounded-none'">
          <TableHeader>
            <TableRow class="bg-muted/50 hover:bg-muted/50">
              <TableHead class="w-[72px]">{{ t('rules.id') }}</TableHead>
              <TableHead class="w-[180px]">{{ t('rules.source') }}</TableHead>
              <TableHead class="w-[170px]">{{ t('rules.port') }} / {{ t('rules.protocol') }}</TableHead>
              <TableHead class="w-[140px]">{{ t('rules.remaining') }}</TableHead>
              <TableHead class="w-[130px]">{{ t('rules.createdAt') }}</TableHead>
              <TableHead class="w-[100px]">{{ t('rules.user') }}</TableHead>
              <TableHead>{{ t('rules.note') }}</TableHead>
              <TableHead class="w-[160px] text-right">{{ t('rules.actions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="r in filtered" :key="r.id">
              <TableCell><CopyableText :value="r.id" mono /></TableCell>
              <TableCell class="max-w-[180px]">
                <CopyableText :value="r.source_ip" mono truncate />
              </TableCell>
              <TableCell>
                <div class="inline-flex items-center gap-1.5 min-w-0">
                  <code class="font-mono font-semibold text-sm">{{ r.ports || r.port }}</code>
                  <Badge :variant="protoVariant(r.protocol)" class="text-[10px] px-1.5 py-0">
                    {{ r.protocol.toUpperCase() }}
                  </Badge>
                </div>
              </TableCell>
              <TableCell>
                <CountdownChip :expire-at="r.expire_at" :created-at="r.created_at" />
              </TableCell>
              <TableCell>
                <Tooltip>
                  <TooltipTrigger as-child>
                    <span class="text-xs text-muted-foreground font-mono">
                      {{ dayjs(r.created_at).format('MM-DD HH:mm') }}
                    </span>
                  </TooltipTrigger>
                  <TooltipContent>
                    {{ dayjs(r.created_at).format('YYYY-MM-DD HH:mm:ss') }}
                  </TooltipContent>
                </Tooltip>
              </TableCell>
              <TableCell>
                <Badge variant="outline" class="font-normal">{{ r.created_by || '-' }}</Badge>
              </TableCell>
              <TableCell class="max-w-0">
                <Tooltip v-if="r.note">
                  <TooltipTrigger as-child>
                    <span class="block truncate text-sm text-foreground/80" :title="r.note">
                      {{ r.note }}
                    </span>
                  </TooltipTrigger>
                  <TooltipContent class="max-w-xs whitespace-pre-wrap">
                    {{ r.note }}
                  </TooltipContent>
                </Tooltip>
                <span v-else class="text-muted-foreground text-sm">—</span>
              </TableCell>
              <TableCell class="text-right whitespace-nowrap">
                <div class="inline-flex gap-0.5 items-center">
                  <!--
                    Notification toggle. Same size-8 footprint as the
                    other action buttons; clicking flips notify_enabled
                    via PATCH /api/rules/:id/notify. When turning on
                    under a browser-capable channel we first request the
                    Notification permission so the next push window
                    actually fires.
                  -->
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <Button
                        variant="ghost"
                        size="icon"
                        class="size-8"
                        :class="r.notify_enabled ? 'text-primary hover:text-primary' : 'text-muted-foreground/60'"
                        :disabled="notifyToggling.has(r.id)"
                        :aria-label="r.notify_enabled ? 'notify-on' : 'notify-off'"
                        @click="toggleNotify(r)"
                      >
                        <Bell v-if="r.notify_enabled" class="size-4" />
                        <BellOff v-else class="size-4" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent class="max-w-xs">
                      <div>{{ r.notify_enabled ? notifyTooltip(r) : t('rules.notifyOff') }}</div>
                      <div class="text-[11px] mt-0.5 opacity-90">
                        {{ r.notify_enabled
                          ? t('rules.notifyClickToDisable')
                          : t('rules.notifyClickToEnable') }}
                      </div>
                    </TooltipContent>
                  </Tooltip>
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <Button variant="ghost" size="icon" class="size-8" @click="openExtend(r)">
                        <Clock class="size-4" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{{ t('action.extend') }}</TooltipContent>
                  </Tooltip>
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <Button variant="ghost" size="icon" class="size-8" @click="onDuplicate(r)">
                        <Copy class="size-4" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{{ t('action.duplicate') }}</TooltipContent>
                  </Tooltip>
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <Button
                        variant="ghost"
                        size="icon"
                        class="size-8 text-destructive hover:bg-destructive/10 hover:text-destructive"
                        @click="askTerminate(r)"
                      >
                        <XCircle class="size-4" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{{ t('action.terminate') }}</TooltipContent>
                  </Tooltip>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>

      <!-- Mobile cards -->
      <div v-if="filtered.length" class="md:hidden p-3 flex flex-col gap-2.5">
        <div
          v-for="r in filtered"
          :key="r.id"
          class="rounded-md border border-border bg-card p-4 flex flex-col gap-3"
        >
          <div class="flex items-center justify-between gap-2">
            <div class="inline-flex items-center gap-1.5 min-w-0">
              <code class="font-mono font-semibold text-base truncate">{{ r.ports || r.port }}</code>
              <Badge :variant="protoVariant(r.protocol)" class="text-[10px]">
                {{ r.protocol.toUpperCase() }}
              </Badge>
              <!--
                Mobile bell is a tap-target rather than a static icon so
                operators can flip the reminder mid-rule from a phone.
                Visual treatment kept compact (size-7) to fit alongside
                the port/protocol cluster instead of stealing the action
                row at the bottom of the card.
              -->
              <button
                type="button"
                class="inline-flex size-7 items-center justify-center rounded-md transition-colors disabled:opacity-50"
                :class="r.notify_enabled
                  ? 'text-primary hover:bg-primary/10'
                  : 'text-muted-foreground/60 hover:bg-muted'"
                :disabled="notifyToggling.has(r.id)"
                :aria-label="r.notify_enabled ? 'notify-on' : 'notify-off'"
                @click="toggleNotify(r)"
              >
                <Bell v-if="r.notify_enabled" class="size-3.5" />
                <BellOff v-else class="size-3.5" />
              </button>
            </div>
            <CountdownChip :expire-at="r.expire_at" :created-at="r.created_at" size="small" />
          </div>
          <div class="grid grid-cols-2 gap-y-2 gap-x-4 text-sm min-w-0">
            <div class="flex flex-col gap-0.5 min-w-0">
              <span class="text-[11px] text-muted-foreground">{{ t('rules.source') }}</span>
              <CopyableText :value="r.source_ip" mono truncate />
            </div>
            <div class="flex flex-col gap-0.5 min-w-0">
              <span class="text-[11px] text-muted-foreground">ID</span>
              <CopyableText :value="r.id" mono />
            </div>
            <div class="flex flex-col gap-0.5 min-w-0">
              <span class="text-[11px] text-muted-foreground">{{ t('rules.createdAt') }}</span>
              <span class="font-mono text-xs">{{ dayjs(r.created_at).format('MM-DD HH:mm') }}</span>
            </div>
            <div class="flex flex-col gap-0.5 min-w-0">
              <span class="text-[11px] text-muted-foreground">{{ t('rules.user') }}</span>
              <span class="text-xs">{{ r.created_by || '-' }}</span>
            </div>
          </div>
          <div
            v-if="r.note"
            class="text-xs text-muted-foreground bg-muted/50 rounded-md px-2.5 py-1.5"
          >
            📝 {{ r.note }}
          </div>
          <div class="grid grid-cols-3 gap-1.5">
            <Button variant="outline" size="sm" @click="openExtend(r)">
              <Clock class="size-3.5" />
              <span class="text-xs">{{ t('action.extend') }}</span>
            </Button>
            <Button variant="outline" size="sm" @click="onDuplicate(r)">
              <Copy class="size-3.5" />
              <span class="text-xs">{{ t('action.duplicate') }}</span>
            </Button>
            <Button variant="outline" size="sm" class="text-destructive border-destructive/30 hover:bg-destructive/10" @click="askTerminate(r)">
              <XCircle class="size-3.5" />
              <span class="text-xs">{{ t('action.terminate') }}</span>
            </Button>
          </div>
        </div>
      </div>
    </div>

    <!-- Extend dialog -->
    <Dialog v-model:open="extendVisible">
      <DialogContent class="max-w-sm">
        <DialogHeader>
          <DialogTitle>{{ t('rules.extendDialog') }}</DialogTitle>
        </DialogHeader>

        <div class="flex flex-col gap-3">
          <div class="text-sm text-muted-foreground">{{ t('rules.extendAmount') }}</div>
          <div class="grid grid-cols-4 gap-2">
            <button
              v-for="opt in [
                { label: '15m', value: 15 * 60 },
                { label: '1h', value: 60 * 60 },
                { label: '4h', value: 4 * 60 * 60 },
                { label: '12h', value: 12 * 60 * 60 }
              ]"
              :key="opt.value"
              type="button"
              class="h-10 rounded-md border text-sm font-medium transition-colors"
              :class="extendSec === opt.value
                ? 'bg-primary text-primary-foreground border-primary'
                : 'border-input text-muted-foreground hover:bg-accent hover:text-accent-foreground'"
              @click="extendSec = opt.value"
            >
              {{ opt.label }}
            </button>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" @click="extendVisible = false">{{ t('common.cancel') }}</Button>
          <Button @click="submitExtend">{{ t('common.confirm') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Terminate confirm -->
    <Dialog :open="!!confirmTarget" @update:open="(v: boolean) => !v && (confirmTarget = null)">
      <DialogContent class="max-w-sm">
        <DialogHeader>
          <DialogTitle>{{ t('action.terminate') }}</DialogTitle>
        </DialogHeader>
        <div class="text-sm text-muted-foreground">
          {{ t('rules.terminateConfirm') }}
        </div>
        <DialogFooter>
          <Button variant="outline" @click="confirmTarget = null">{{ t('common.cancel') }}</Button>
          <Button variant="destructive" @click="doTerminate">{{ t('action.terminate') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
