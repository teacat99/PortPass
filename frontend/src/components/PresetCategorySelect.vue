<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Pencil, Plus, Sparkles, X } from 'lucide-vue-next'
import {
  deletePresetCategory,
  listPresetCategories,
  upsertPresetCategory
} from '@/api/rules'
import type { PresetCategory } from '@/api/types'
import { isImageIcon } from '@/utils/presetIcon'
import { Message } from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from '@/components/ui/popover'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import IconPickerPanel from '@/components/IconPickerPanel.vue'

// Selector + manager for preset categories.
// modelValue is the bound category_id (null/undefined means "auto-detect").
// categories is the live list owned by the parent (SettingsView). Any
// CRUD operation triggers an update:categories so siblings (HomeView,
// preset table cells) stay in sync without a full refetch.

const props = defineProps<{
  modelValue: number | null | undefined
  categories: PresetCategory[]
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: number | null): void
  (e: 'update:categories', v: PresetCategory[]): void
}>()

const { t, locale } = useI18n()

const popoverOpen = ref(false)

// Display label for built-in categories: when the row's label is blank
// the frontend i18n key (home.cat<Pascal>) supplies the localized text.
function categoryLabel(c: PresetCategory): string {
  if (c.label) return c.label
  if (c.builtin && c.key) {
    const k = 'home.cat' + c.key.charAt(0).toUpperCase() + c.key.slice(1)
    return t(k)
  }
  return c.key || '—'
}

const selected = computed(() => {
  if (!props.modelValue) return null
  return props.categories.find((c) => c.id === props.modelValue) ?? null
})

const triggerLabel = computed(() => {
  if (selected.value) return categoryLabel(selected.value)
  return t('settings.presetCategoryAuto')
})

function pick(id: number | null) {
  emit('update:modelValue', id)
  popoverOpen.value = false
}

// ─────────── Edit / Create dialog ───────────
const editOpen = ref(false)
const editTarget = ref<PresetCategory | null>(null)
const editForm = ref({ label: '', icon: '' })
const editSaving = ref(false)
const isEditing = computed(() => !!editTarget.value)
const editingBuiltin = computed(() => !!editTarget.value?.builtin)

function startCreate() {
  editTarget.value = null
  editForm.value = { label: '', icon: '' }
  editOpen.value = true
  popoverOpen.value = false
}

function startEdit(c: PresetCategory) {
  editTarget.value = c
  editForm.value = { label: c.label, icon: c.icon }
  editOpen.value = true
  popoverOpen.value = false
}

async function saveEdit() {
  // Built-ins permit empty label (i18n fallback). User-defined need a
  // non-empty label so the dropdown row is identifiable at a glance.
  if (!editingBuiltin.value && !editForm.value.label.trim()) {
    Message.warning(t('settings.presetCategoryEmptyName'))
    return
  }
  editSaving.value = true
  try {
    const payload: Partial<PresetCategory> = {
      label: editForm.value.label.trim(),
      icon: editForm.value.icon.trim()
    }
    if (editTarget.value) {
      payload.id = editTarget.value.id
      payload.sort = editTarget.value.sort
    } else {
      // New rows land after the highest existing sort so they appear
      // at the bottom of the user-added section without renumbering.
      const maxSort = props.categories.reduce((m, c) => Math.max(m, c.sort), 0)
      payload.sort = maxSort + 1
    }
    const saved = await upsertPresetCategory(payload)
    const next = [...props.categories]
    const idx = next.findIndex((c) => c.id === saved.id)
    if (idx >= 0) next[idx] = saved
    else next.push(saved)
    next.sort((a, b) => a.sort - b.sort || a.id - b.id)
    emit('update:categories', next)
    Message.success(t('settings.presetCategorySaved'))
    editOpen.value = false
    if (!editTarget.value) {
      // Auto-select newly created category so the user does not have
      // to reopen the dropdown to pick it.
      emit('update:modelValue', saved.id)
    }
  } catch (e: unknown) {
    Message.error(e instanceof Error ? e.message : String(e))
  } finally {
    editSaving.value = false
  }
}

// ─────────── Delete confirm ───────────
const deleteTarget = ref<PresetCategory | null>(null)
const deleteOpen = ref(false)
const deleteSaving = ref(false)

function startDelete(c: PresetCategory) {
  deleteTarget.value = c
  deleteOpen.value = true
  popoverOpen.value = false
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  deleteSaving.value = true
  try {
    const id = deleteTarget.value.id
    await deletePresetCategory(id)
    const next = props.categories.filter((c) => c.id !== id)
    emit('update:categories', next)
    if (props.modelValue === id) {
      emit('update:modelValue', null)
    }
    Message.success(t('settings.presetCategoryDeleted'))
    deleteOpen.value = false
    deleteTarget.value = null
  } catch (e: unknown) {
    Message.error(e instanceof Error ? e.message : String(e))
  } finally {
    deleteSaving.value = false
  }
}

// Refresh from server when the popover opens — keeps the manager in
// sync if another tab/window edited categories meanwhile.
async function onPopoverOpenChange(v: boolean) {
  popoverOpen.value = v
  if (v) {
    try {
      const list = await listPresetCategories()
      emit('update:categories', list)
    } catch {
      // ignore — we still have the prop snapshot to render
    }
  }
}

// Build the list as a single sorted array so the template is simple.
const orderedCategories = computed(() => {
  const builtinOrder = ['remote', 'web', 'db', 'mq', 'game', 'misc']
  const builtins = props.categories
    .filter((c) => c.builtin)
    .sort((a, b) => {
      const ai = builtinOrder.indexOf(a.key)
      const bi = builtinOrder.indexOf(b.key)
      return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi) || a.sort - b.sort
    })
  const userAdded = props.categories
    .filter((c) => !c.builtin)
    .sort((a, b) => a.sort - b.sort || a.id - b.id)
  return [...builtins, ...userAdded]
})

// Tiny no-op consumer for `locale` so vue-i18n keeps the watcher
// active and labels re-render when the user flips zh ↔ en.
void locale
</script>

<template>
  <div>
    <Popover :open="popoverOpen" @update:open="onPopoverOpenChange">
      <PopoverTrigger as-child>
        <button
          type="button"
          class="flex items-center justify-between w-full h-10 px-3 rounded-md border border-border bg-card text-sm hover:bg-muted/50 transition-colors"
        >
          <span class="flex items-center gap-2 min-w-0">
            <span class="inline-flex items-center justify-center size-5 shrink-0">
              <template v-if="selected">
                <img
                  v-if="isImageIcon(selected.icon)"
                  :src="selected.icon"
                  class="size-5 rounded-md object-cover"
                  referrerpolicy="no-referrer"
                />
                <span v-else-if="selected.icon" class="text-base leading-none">{{ selected.icon }}</span>
                <span v-else class="text-muted-foreground">·</span>
              </template>
              <Sparkles v-else class="size-4 text-muted-foreground" />
            </span>
            <span class="truncate">{{ triggerLabel }}</span>
          </span>
          <span class="text-xs text-muted-foreground ml-2">▾</span>
        </button>
      </PopoverTrigger>

      <PopoverContent align="start" class="w-72 p-0">
        <div class="max-h-[60vh] overflow-y-auto py-1">
          <!-- Auto-detect always sits at the top so the user can clear
               a manual override with one click. -->
          <button
            type="button"
            class="flex items-center w-full px-3 py-2 text-left hover:bg-muted/60 transition-colors"
            @click="pick(null)"
          >
            <span class="inline-flex items-center justify-center size-5 mr-2">
              <Sparkles class="size-4 text-muted-foreground" />
            </span>
            <span class="flex-1 min-w-0">
              <span class="text-sm">{{ t('settings.presetCategoryAuto') }}</span>
              <span class="block text-[11px] text-muted-foreground truncate">
                {{ t('settings.presetCategoryAutoHint') }}
              </span>
            </span>
            <Check
              v-if="!modelValue"
              class="size-4 text-primary ml-2 shrink-0"
            />
          </button>

          <div class="h-px bg-border mx-2 my-1" />

          <button
            v-for="c in orderedCategories"
            :key="c.id"
            type="button"
            class="group flex items-center w-full px-3 py-2 text-left hover:bg-muted/60 transition-colors"
            @click="pick(c.id)"
          >
            <span class="inline-flex items-center justify-center size-5 mr-2">
              <img
                v-if="isImageIcon(c.icon)"
                :src="c.icon"
                class="size-5 rounded-md object-cover"
                referrerpolicy="no-referrer"
              />
              <span v-else-if="c.icon" class="text-base leading-none">{{ c.icon }}</span>
              <span v-else class="text-muted-foreground">·</span>
            </span>
            <span class="flex-1 min-w-0">
              <span class="text-sm truncate">{{ categoryLabel(c) }}</span>
              <span v-if="c.builtin" class="block text-[10px] text-muted-foreground">
                {{ t('settings.presetCategoryBuiltinHint') }}
              </span>
            </span>
            <span class="flex items-center gap-1 ml-2 shrink-0">
              <Check
                v-if="modelValue === c.id"
                class="size-4 text-primary"
              />
              <button
                type="button"
                class="size-7 rounded-md inline-flex items-center justify-center text-muted-foreground hover:bg-muted hover:text-foreground"
                :title="t('settings.presetCategoryEdit')"
                @click.stop="startEdit(c)"
              >
                <Pencil class="size-3.5" />
              </button>
              <button
                v-if="!c.builtin"
                type="button"
                class="size-7 rounded-md inline-flex items-center justify-center text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                :title="t('settings.presetCategoryDelete')"
                @click.stop="startDelete(c)"
              >
                <X class="size-3.5" />
              </button>
            </span>
          </button>

          <div class="h-px bg-border mx-2 my-1" />

          <button
            type="button"
            class="flex items-center w-full px-3 py-2 text-left hover:bg-muted/60 transition-colors text-primary"
            @click="startCreate"
          >
            <span class="inline-flex items-center justify-center size-5 mr-2">
              <Plus class="size-4" />
            </span>
            <span class="text-sm">{{ t('settings.presetCategoryNew') }}</span>
          </button>
        </div>
      </PopoverContent>
    </Popover>

    <!-- Create / edit dialog (shared form) -->
    <Dialog v-model:open="editOpen">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {{ isEditing ? t('settings.presetCategoryEdit') : t('settings.presetCategoryNew') }}
          </DialogTitle>
        </DialogHeader>
        <div class="flex flex-col gap-4">
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('settings.presetCategoryLabel') }}</Label>
            <Input
              v-model="editForm.label"
              :placeholder="editingBuiltin
                ? categoryLabel(editTarget!)
                : t('settings.presetCategoryLabelPlaceholder')"
            />
            <span v-if="editingBuiltin" class="text-[11px] text-muted-foreground">
              {{ t('settings.presetCategoryBuiltinHint') }}
            </span>
          </div>
          <div class="flex flex-col gap-1.5">
            <Label>{{ t('settings.presetCategoryIcon') }}</Label>
            <IconPickerPanel v-model="editForm.icon" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" :disabled="editSaving" @click="editOpen = false">
            {{ t('common.cancel') }}
          </Button>
          <Button :disabled="editSaving" @click="saveEdit">
            {{ t('common.save') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Delete confirm -->
    <Dialog v-model:open="deleteOpen">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('settings.presetCategoryDelete') }}</DialogTitle>
        </DialogHeader>
        <p v-if="deleteTarget" class="text-sm text-muted-foreground">
          {{ t('settings.presetCategoryDeleteConfirm', { name: categoryLabel(deleteTarget) }) }}
        </p>
        <DialogFooter>
          <Button variant="outline" :disabled="deleteSaving" @click="deleteOpen = false">
            {{ t('common.cancel') }}
          </Button>
          <Button variant="destructive" :disabled="deleteSaving" @click="confirmDelete">
            {{ t('common.delete') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
