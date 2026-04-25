<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import dayjs from 'dayjs'
import { RefreshCw, Search as SearchIcon } from 'lucide-vue-next'
import { listHistory } from '@/api/rules'
import type { Rule } from '@/api/types'

import DateTimePicker from '@/components/DateTimePicker.vue'
import EmptyState from '@/components/EmptyState.vue'
import StatusTag from '@/components/StatusTag.vue'
import CopyableText from '@/components/CopyableText.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem
} from '@/components/ui/select'
import {
  Table, TableHeader, TableBody, TableRow, TableHead, TableCell
} from '@/components/ui/table'
import {
  Tooltip, TooltipTrigger, TooltipContent
} from '@/components/ui/tooltip'

const { t } = useI18n()

const rules = ref<Rule[]>([])
const total = ref(0)
const loading = ref(false)

const filter = ref({
  port: '' as string,
  ip: '' as string,
  status: '' as string,
  from: '' as string,
  to: '' as string
})

const pageSize = 20
const currentPage = ref(1)

async function reload() {
  loading.value = true
  try {
    const q: Record<string, string | number> = { limit: 200 }
    if (filter.value.port) {
      const n = Number(filter.value.port)
      if (!Number.isNaN(n)) q.port = n
    }
    if (filter.value.ip) q.ip = filter.value.ip
    if (filter.value.status) q.status = filter.value.status
    if (filter.value.from && filter.value.to) {
      q.from = dayjs(filter.value.from).toISOString()
      q.to = dayjs(filter.value.to).toISOString()
    }
    const resp = await listHistory(q)
    rules.value = resp.rules
    total.value = resp.total
    currentPage.value = 1
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
  filter.value = { port: '', ip: '', status: '', from: '', to: '' }
  reload()
}

const paged = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return rules.value.slice(start, start + pageSize)
})
const pageTotal = computed(() => Math.max(1, Math.ceil(rules.value.length / pageSize)))

function protoVariant(p: string) {
  return p === 'udp' ? 'secondary' : 'default'
}

onMounted(reload)
</script>

<template>
  <div class="pp-page flex flex-col gap-4">
    <!-- Header -->
    <header class="flex items-end justify-between gap-4 flex-wrap">
      <div>
        <h1 class="text-xl font-semibold text-foreground m-0">{{ t('history.title') }}</h1>
        <p class="text-sm text-muted-foreground mt-1 m-0">{{ t('history.subtitle') }}</p>
      </div>
      <Button variant="outline" size="sm" :disabled="loading" @click="reload">
        <RefreshCw :class="['size-4', loading && 'animate-spin']" />
        {{ t('action.refresh') }}
      </Button>
    </header>

    <!-- Filter bar -->
    <div class="rounded-lg border border-border bg-card p-4">
      <div class="grid grid-cols-2 md:grid-cols-6 gap-3 items-end">
        <div class="flex flex-col gap-1.5 col-span-1">
          <label class="text-xs text-muted-foreground font-medium">{{ t('rules.port') }}</label>
          <Input v-model="filter.port" placeholder="80" class="h-9" />
        </div>
        <div class="flex flex-col gap-1.5 col-span-1">
          <label class="text-xs text-muted-foreground font-medium">IP / CIDR</label>
          <Input v-model="filter.ip" placeholder="192.168.1.0/24" class="h-9" />
        </div>
        <div class="flex flex-col gap-1.5 col-span-1">
          <label class="text-xs text-muted-foreground font-medium">{{ t('history.status') }}</label>
          <Select v-model="filter.status">
            <SelectTrigger class="h-9">
              <SelectValue :placeholder="t('common.all')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="expired">{{ t('status.expired') }}</SelectItem>
              <SelectItem value="revoked">{{ t('status.revoked') }}</SelectItem>
              <SelectItem value="failed">{{ t('status.failed') }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="flex flex-col gap-1.5 col-span-1">
          <label class="text-xs text-muted-foreground font-medium">{{ t('history.filterFrom') }}</label>
          <DateTimePicker v-model="filter.from" />
        </div>
        <div class="flex flex-col gap-1.5 col-span-1">
          <label class="text-xs text-muted-foreground font-medium">{{ t('history.filterTo') }}</label>
          <DateTimePicker v-model="filter.to" />
        </div>
        <div class="flex gap-2 col-span-2 md:col-span-1 justify-end">
          <Button variant="outline" size="sm" @click="reset">{{ t('history.reset') }}</Button>
          <Button size="sm" @click="reload">
            <SearchIcon class="size-4" />
            {{ t('action.search') }}
          </Button>
        </div>
      </div>
    </div>

    <!-- List -->
    <div class="rounded-lg border border-border bg-card overflow-hidden">
      <div v-if="loading && !rules.length" class="p-6 flex flex-col gap-4">
        <div v-for="i in 4" :key="i" class="flex gap-3 items-center">
          <div class="h-4 w-16 rounded bg-muted animate-pulse" />
          <div class="h-4 flex-1 rounded bg-muted animate-pulse" />
        </div>
      </div>

      <EmptyState
        v-else-if="!rules.length"
        icon="📜"
        :title="t('history.emptyTitle')"
        :description="t('history.emptyDesc')"
      >
        <template #action>
          <Button variant="outline" @click="reset">{{ t('history.clearFilter') }}</Button>
        </template>
      </EmptyState>

      <!-- Desktop table -->
      <div v-else class="hidden md:block">
        <Table :container-class="'border-0 rounded-none'">
          <TableHeader>
            <TableRow class="bg-muted/50 hover:bg-muted/50">
              <TableHead class="w-[70px]">ID</TableHead>
              <TableHead class="w-[100px]">{{ t('history.status') }}</TableHead>
              <TableHead class="w-[180px]">{{ t('rules.source') }}</TableHead>
              <TableHead class="w-[140px]">{{ t('rules.port') }}</TableHead>
              <TableHead class="w-[160px]">{{ t('history.actor') }}</TableHead>
              <TableHead class="w-[100px]">{{ t('history.user') }}</TableHead>
              <TableHead class="w-[130px]">{{ t('rules.createdAt') }}</TableHead>
              <TableHead class="w-[130px]">{{ t('history.terminatedAt') }}</TableHead>
              <TableHead class="w-[90px]">{{ t('history.duration') }}</TableHead>
              <TableHead>{{ t('rules.note') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="r in paged" :key="r.id">
              <TableCell><CopyableText :value="r.id" mono /></TableCell>
              <TableCell><StatusTag :status="r.status" /></TableCell>
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
              <TableCell class="max-w-[160px]">
                <CopyableText :value="r.created_ip" mono truncate />
              </TableCell>
              <TableCell>
                <Badge variant="outline" class="font-normal">{{ r.created_by || '-' }}</Badge>
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
                <Tooltip v-if="r.terminated_at">
                  <TooltipTrigger as-child>
                    <span class="text-xs text-muted-foreground font-mono">
                      {{ dayjs(r.terminated_at).format('MM-DD HH:mm') }}
                    </span>
                  </TooltipTrigger>
                  <TooltipContent>
                    {{ dayjs(r.terminated_at).format('YYYY-MM-DD HH:mm:ss') }}
                  </TooltipContent>
                </Tooltip>
                <span v-else class="text-muted-foreground text-xs">—</span>
              </TableCell>
              <TableCell>
                <span class="font-mono text-xs">{{ durationOf(r) }}</span>
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
            </TableRow>
          </TableBody>
        </Table>

        <!-- Pagination -->
        <div
          v-if="pageTotal > 1"
          class="flex items-center justify-between px-4 py-3 border-t border-border text-sm"
        >
          <span class="text-muted-foreground text-xs">
            {{ t('history.totalRows', { n: rules.length }) }} · {{ t('history.page', { current: currentPage, total: pageTotal }) }}
          </span>
          <div class="flex gap-1">
            <Button
              variant="outline"
              size="sm"
              :disabled="currentPage <= 1"
              @click="currentPage--"
            >{{ t('history.prevPage') }}</Button>
            <Button
              variant="outline"
              size="sm"
              :disabled="currentPage >= pageTotal"
              @click="currentPage++"
            >{{ t('history.nextPage') }}</Button>
          </div>
        </div>
      </div>

      <!-- Mobile cards -->
      <div v-if="paged.length" class="md:hidden p-3 flex flex-col gap-2.5">
        <div
          v-for="r in paged"
          :key="r.id"
          class="rounded-md border border-border bg-card p-4 flex flex-col gap-3"
        >
          <div class="flex items-center justify-between gap-2">
            <div class="inline-flex items-center gap-1.5 min-w-0">
              <code class="font-mono font-semibold text-base truncate">{{ r.ports || r.port }}</code>
              <Badge :variant="protoVariant(r.protocol)" class="text-[10px]">
                {{ r.protocol.toUpperCase() }}
              </Badge>
            </div>
            <StatusTag :status="r.status" />
          </div>
          <div class="grid grid-cols-2 gap-y-2 gap-x-4 text-sm min-w-0">
            <div class="flex flex-col gap-0.5 min-w-0">
              <span class="text-[11px] text-muted-foreground">{{ t('rules.source') }}</span>
              <CopyableText :value="r.source_ip" mono truncate />
            </div>
            <div class="flex flex-col gap-0.5 min-w-0">
              <span class="text-[11px] text-muted-foreground">{{ t('history.actor') }}</span>
              <CopyableText :value="r.created_ip" mono truncate />
            </div>
            <div class="flex flex-col gap-0.5 min-w-0">
              <span class="text-[11px] text-muted-foreground">{{ t('rules.createdAt') }}</span>
              <span class="font-mono text-xs">{{ dayjs(r.created_at).format('MM-DD HH:mm') }}</span>
            </div>
            <div class="flex flex-col gap-0.5 min-w-0">
              <span class="text-[11px] text-muted-foreground">{{ t('history.duration') }}</span>
              <span class="font-mono text-xs">{{ durationOf(r) }}</span>
            </div>
            <div class="flex flex-col gap-0.5 min-w-0">
              <span class="text-[11px] text-muted-foreground">{{ t('history.user') }}</span>
              <span class="text-xs">{{ r.created_by || '-' }}</span>
            </div>
            <div class="flex flex-col gap-0.5 min-w-0">
              <span class="text-[11px] text-muted-foreground">ID</span>
              <CopyableText :value="r.id" mono />
            </div>
          </div>
          <div
            v-if="r.note"
            class="text-xs text-muted-foreground bg-muted/50 rounded-md px-2.5 py-1.5"
          >
            📝 {{ r.note }}
          </div>
        </div>

        <!-- Mobile pagination -->
        <div
          v-if="pageTotal > 1"
          class="flex items-center justify-between px-2 py-2 text-xs"
        >
          <span class="text-muted-foreground">
            {{ currentPage }} / {{ pageTotal }}
          </span>
          <div class="flex gap-1">
            <Button variant="outline" size="sm" :disabled="currentPage <= 1" @click="currentPage--">{{ t('history.prevPage') }}</Button>
            <Button variant="outline" size="sm" :disabled="currentPage >= pageTotal" @click="currentPage++">{{ t('history.nextPage') }}</Button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
