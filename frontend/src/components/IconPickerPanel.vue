<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Input } from '@/components/ui/input'
import { ICON_EMOJI_PRESETS, isImageIcon } from '@/utils/presetIcon'

const props = defineProps<{
  modelValue: string
}>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const { t } = useI18n()

const isUrl = computed(() => isImageIcon(props.modelValue))
// Active tab follows the value's shape so reopening the panel returns
// the user to the row they were last editing.
const activeTab = computed<'emoji' | 'url'>(() => (isUrl.value ? 'url' : 'emoji'))

function pick(v: string) {
  emit('update:modelValue', v)
}

function setUrl(v: string) {
  emit('update:modelValue', v)
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- Live preview so the user sees what will land in the dropdown
         without having to close and reopen the picker. -->
    <div class="flex items-center gap-3 rounded-md border border-border bg-muted/40 px-3 py-2">
      <span class="inline-flex items-center justify-center size-8 rounded-md bg-card border border-border">
        <img
          v-if="isUrl && modelValue"
          :src="modelValue"
          class="size-7 rounded-md object-cover"
          referrerpolicy="no-referrer"
        />
        <span v-else-if="modelValue" class="text-xl leading-none">{{ modelValue }}</span>
        <span v-else class="text-xs text-muted-foreground">—</span>
      </span>
      <span class="text-xs text-muted-foreground truncate flex-1">
        {{ modelValue || t('settings.iconNoneHint') }}
      </span>
    </div>

    <Tabs :model-value="activeTab" class="w-full">
      <TabsList class="grid w-full grid-cols-2">
        <TabsTrigger value="emoji" @click="activeTab !== 'emoji' && pick('')">
          {{ t('settings.iconTabEmoji') }}
        </TabsTrigger>
        <TabsTrigger value="url" @click="activeTab !== 'url' && setUrl('https://')">
          {{ t('settings.iconTabUrl') }}
        </TabsTrigger>
      </TabsList>

      <TabsContent value="emoji" class="mt-3">
        <div class="grid grid-cols-6 gap-1.5">
          <button
            v-for="e in ICON_EMOJI_PRESETS"
            :key="e"
            type="button"
            class="size-9 rounded-md border text-lg flex items-center justify-center transition-colors"
            :class="modelValue === e
              ? 'border-primary bg-primary/10'
              : 'border-border bg-card hover:border-primary/50 hover:bg-muted/60'"
            @click="pick(e)"
          >
            {{ e }}
          </button>
        </div>
      </TabsContent>

      <TabsContent value="url" class="mt-3 flex flex-col gap-2">
        <Input
          :model-value="isUrl ? modelValue : ''"
          placeholder="https://favicon.im/example.com"
          @update:model-value="(v) => setUrl(String(v))"
        />
        <p class="text-xs text-muted-foreground leading-relaxed">
          {{ t('settings.iconUrlHint') }}
        </p>
      </TabsContent>
    </Tabs>
  </div>
</template>
