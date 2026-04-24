<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, Trash2, Pencil, AlertTriangle } from 'lucide-vue-next'
import type { UserAllowedRange, User } from '@/api/types'
import {
  listUserRanges,
  upsertUserRange,
  deleteUserRange,
  clearUserRanges
} from '@/api/policy'
import { Message } from '@/lib/toast'

import PortSetInput from '@/components/PortSetInput.vue'
import EmptyState from '@/components/EmptyState.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription
} from '@/components/ui/sheet'
import {
  Dialog, DialogContent, DialogHeader, DialogFooter, DialogTitle
} from '@/components/ui/dialog'

const props = defineProps<{ visible: boolean; user: User | null }>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'changed'): void
}>()
const { t } = useI18n()

const localVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v)
})

const ranges = ref<UserAllowedRange[]>([])
const loading = ref(false)

async function reload() {
  if (!props.user) return
  loading.value = true
  try {
    ranges.value = await listUserRanges(props.user.id)
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.visible, props.user?.id],
  () => {
    if (props.visible && props.user) reload()
  },
  { immediate: true }
)

const editVisible = ref(false)
const editing = ref<Partial<UserAllowedRange>>({})
const portsValid = ref({ ok: false, error: null as string | null })

function openCreate() {
  editing.value = {
    name: '',
    ports: '',
    protocol: 'tcp',
    max_duration_sec: 0,
    note: ''
  }
  portsValid.value = { ok: false, error: null }
  editVisible.value = true
}

function openEdit(r: UserAllowedRange) {
  editing.value = { ...r }
  portsValid.value = { ok: true, error: null }
  editVisible.value = true
}

async function save() {
  if (!props.user) return
  if (!editing.value.name?.trim()) {
    Message.warning(t('msg.invalidInput'))
    return
  }
  if (!portsValid.value.ok) {
    Message.warning(portsValid.value.error || t('msg.invalidInput'))
    return
  }
  try {
    await upsertUserRange(props.user.id, editing.value)
    Message.success(t('msg.saved'))
    editVisible.value = false
    await reload()
    emit('changed')
  } catch {
    /* interceptor */
  }
}

const confirmTarget = ref<UserAllowedRange | null>(null)
const confirmClearAll = ref(false)

async function doRemove() {
  if (!props.user || !confirmTarget.value) return
  const id = confirmTarget.value.id
  confirmTarget.value = null
  await deleteUserRange(props.user.id, id)
  Message.success(t('msg.deleted'))
  await reload()
  emit('changed')
}

async function doClearAll() {
  if (!props.user) return
  confirmClearAll.value = false
  await clearUserRanges(props.user.id)
  Message.success(t('msg.deleted'))
  await reload()
  emit('changed')
}
</script>

<template>
  <Sheet :open="localVisible" @update:open="localVisible = $event">
    <SheetContent side="right" class="w-full sm:max-w-[560px] flex flex-col gap-0 p-0">
      <SheetHeader class="px-6 py-5 border-b border-border">
        <SheetTitle>
          {{ t('userRanges.drawerTitle') }}
          <span v-if="user" class="text-muted-foreground font-normal">· {{ user.username }}</span>
        </SheetTitle>
        <SheetDescription>{{ t('userRanges.drawerSub') }}</SheetDescription>
      </SheetHeader>

      <div class="flex-1 overflow-y-auto px-6 py-5 flex flex-col gap-4">
        <Alert v-if="ranges.length" variant="warning">
          <AlertDescription>{{ t('userRanges.overrideNote') }}</AlertDescription>
        </Alert>
        <Alert v-else variant="info">
          <AlertDescription>{{ t('userRanges.emptyNote') }}</AlertDescription>
        </Alert>

        <div class="flex gap-2">
          <Button @click="openCreate">
            <Plus class="size-4" />
            {{ t('userRanges.addRange') }}
          </Button>
          <Button
            v-if="ranges.length"
            variant="outline"
            class="text-destructive border-destructive/30 hover:bg-destructive/10"
            @click="confirmClearAll = true"
          >
            <AlertTriangle class="size-4" />
            {{ t('userRanges.clearAll') }}
          </Button>
        </div>

        <EmptyState
          v-if="!loading && !ranges.length"
          icon="🔓"
          :title="t('userRanges.colDefault')"
          :description="t('userRanges.emptyNote')"
        />

        <div v-else class="flex flex-col gap-2">
          <div
            v-for="r in ranges"
            :key="r.id"
            class="rounded-md border border-border bg-card p-4 flex flex-col gap-2"
          >
            <div class="flex items-center gap-2">
              <strong class="text-foreground">{{ r.name }}</strong>
              <Badge :variant="r.protocol === 'udp' ? 'secondary' : 'default'" class="text-[10px]">
                {{ r.protocol.toUpperCase() }}
              </Badge>
              <div class="ml-auto inline-flex gap-0.5">
                <Button variant="ghost" size="icon" class="size-7" @click="openEdit(r)">
                  <Pencil class="size-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  class="size-7 text-destructive hover:bg-destructive/10"
                  @click="confirmTarget = r"
                >
                  <Trash2 class="size-3.5" />
                </Button>
              </div>
            </div>
            <div class="font-mono text-sm text-foreground">{{ r.ports }}</div>
            <div
              v-if="r.max_duration_sec || r.note"
              class="flex gap-4 flex-wrap text-xs text-muted-foreground"
            >
              <span v-if="r.max_duration_sec">⏱ {{ Math.floor(r.max_duration_sec / 60) }} min</span>
              <span v-if="r.note">📝 {{ r.note }}</span>
            </div>
          </div>
        </div>
      </div>
    </SheetContent>
  </Sheet>

  <Dialog v-model:open="editVisible">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>
          {{ editing.id ? t('userRanges.editRange') : t('userRanges.addRange') }}
        </DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4">
        <div class="flex flex-col gap-1.5">
          <Label>{{ t('userRanges.name') }}</Label>
          <Input v-model="editing.name" placeholder="HTTP cluster" />
        </div>
        <div class="flex flex-col gap-1.5">
          <Label>{{ t('userRanges.ports') }}</Label>
          <PortSetInput
            v-model="editing.ports as string"
            :placeholder="t('portSet.placeholder')"
            @validation="(ok: boolean, error: string | null) => (portsValid = { ok, error })"
          />
        </div>
        <div class="flex flex-col gap-1.5">
          <Label>{{ t('userRanges.protocol') }}</Label>
          <div class="inline-flex p-1 rounded-md bg-muted/60 border border-border w-fit">
            <button
              v-for="p in ['tcp', 'udp', 'both']"
              :key="p"
              type="button"
              class="px-3 h-8 rounded text-xs font-medium transition-colors"
              :class="editing.protocol === p
                ? 'bg-card text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'"
              @click="editing.protocol = p"
            >
              {{ p === 'both' ? 'TCP+UDP' : p.toUpperCase() }}
            </button>
          </div>
        </div>
        <div class="flex flex-col gap-1.5">
          <Label>{{ t('userRanges.maxDuration') }}</Label>
          <Input
            v-model="editing.max_duration_sec"
            type="number"
            :min="0"
            :max="24 * 3600"
            :step="300"
          />
        </div>
        <div class="flex flex-col gap-1.5">
          <Label>{{ t('userRanges.note') }}</Label>
          <Input v-model="editing.note" />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" @click="editVisible = false">{{ t('common.cancel') }}</Button>
        <Button @click="save">{{ t('common.save') }}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Remove confirm -->
  <Dialog :open="!!confirmTarget" @update:open="(v: boolean) => !v && (confirmTarget = null)">
    <DialogContent class="max-w-sm">
      <DialogHeader>
        <DialogTitle>{{ t('action.delete') }}</DialogTitle>
      </DialogHeader>
      <div v-if="confirmTarget" class="text-sm text-muted-foreground">
        {{ confirmTarget.name }} ({{ confirmTarget.ports }})
      </div>
      <DialogFooter>
        <Button variant="outline" @click="confirmTarget = null">{{ t('common.cancel') }}</Button>
        <Button variant="destructive" @click="doRemove">{{ t('action.delete') }}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>

  <!-- Clear all confirm -->
  <Dialog v-model:open="confirmClearAll">
    <DialogContent class="max-w-sm">
      <DialogHeader>
        <DialogTitle>{{ t('userRanges.clearAll') }}</DialogTitle>
      </DialogHeader>
      <div class="text-sm text-muted-foreground">
        {{ t('userRanges.clearAllConfirm') }}
      </div>
      <DialogFooter>
        <Button variant="outline" @click="confirmClearAll = false">{{ t('common.cancel') }}</Button>
        <Button variant="destructive" @click="doClearAll">{{ t('userRanges.clearAll') }}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
