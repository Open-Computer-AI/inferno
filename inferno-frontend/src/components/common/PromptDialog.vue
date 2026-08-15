<template>
  <BaseDialog :show="show" :title="title" width="narrow" @close="handleCancel">
    <div class="prompt-body">
      <p v-if="message" class="prompt-message">{{ message }}</p>
      <Input
        ref="inputRef"
        v-model="value"
        :label="title"
        :placeholder="placeholder"
        autocomplete="off"
        @enter="handleConfirm"
      />
    </div>

    <template #footer>
      <div class="prompt-actions">
        <Button variant="ghost" size="sm" @click="handleCancel">
          {{ cancelText }}
        </Button>
        <Button :variant="danger ? 'danger' : 'solid'" size="sm" @click="handleConfirm">
          {{ confirmText }}
        </Button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import BaseDialog from './BaseDialog.vue'
import Button from './Button.vue'
import Input from './Input.vue'

const props = withDefaults(defineProps<{
  show: boolean
  title: string
  message?: string
  defaultValue?: string
  placeholder?: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
}>(), {
  message: '',
  defaultValue: '',
  placeholder: '',
  confirmText: 'Confirm',
  cancelText: 'Cancel',
  danger: false,
})

const emit = defineEmits<{
  confirm: [value: string]
  cancel: []
}>()

const value = ref(props.defaultValue)
const inputRef = ref<InstanceType<typeof Input> | null>(null)

watch(
  () => [props.show, props.defaultValue] as const,
  async ([show, defaultValue]) => {
    if (!show) return
    value.value = defaultValue
    await nextTick()
    inputRef.value?.focus()
  },
  { immediate: true },
)

function handleConfirm() {
  emit('confirm', value.value)
}

function handleCancel() {
  emit('cancel')
}
</script>

<style scoped>
.prompt-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.prompt-message {
  margin: 0;
  color: var(--body-copy);
  font-size: var(--fs-md);
  line-height: 1.55;
  white-space: pre-line;
}

.prompt-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  width: 100%;
}
</style>
