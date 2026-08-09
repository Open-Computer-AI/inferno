<script setup lang="ts">
import { computed, ref } from 'vue'

/**
 * Text area — part 02, section 04.
 *
 * "Same as Input, minus the prefix and suffix slots, plus a rows prop.
 *  Vertical resize only. The machine data variant adds a line gutter and
 *  switches to --font-mono at 1.75 leading."
 *
 * Migration: "One new variant, mono, for the four bulk import modals. Nothing
 * else changes." So the entire existing API is preserved and `variant` is
 * additive with a default that reproduces today's behaviour.
 *
 * Resize is vertical only. Horizontal resize lets a user drag a field outside
 * its own form and there is never a reason to.
 */
interface Props {
  modelValue: string | null | undefined
  label?: string
  placeholder?: string
  disabled?: boolean
  required?: boolean
  readonly?: boolean
  error?: string
  hint?: string
  id?: string
  rows?: number | string
  /** 'mono' is for pasted machine data: keys, ids, one record per line. */
  variant?: 'text' | 'mono'
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  required: false,
  readonly: false,
  rows: 3,
  variant: 'text'
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', value: string): void
  (e: 'blur', event: FocusEvent): void
  (e: 'focus', event: FocusEvent): void
}>()

const textAreaRef = ref<HTMLTextAreaElement | null>(null)
const placeholderText = computed(() => props.placeholder || '')

const onInput = (event: Event) => {
  emit('update:modelValue', (event.target as HTMLTextAreaElement).value)
}

defineExpose({
  focus: () => textAreaRef.value?.focus(),
  select: () => textAreaRef.value?.select()
})
</script>

<template>
  <div class="fld">
    <label v-if="label" :for="id" class="fld__label">
      {{ label }}
      <span v-if="required" class="fld__req" aria-hidden="true">*</span>
    </label>

    <textarea
      :id="id"
      ref="textAreaRef"
      class="fld__area"
      :data-variant="variant"
      :rows="rows"
      :value="modelValue ?? ''"
      :disabled="disabled"
      :required="required"
      :readonly="readonly"
      :placeholder="placeholderText"
      :data-invalid="error ? '' : undefined"
      :aria-invalid="error ? true : undefined"
      @input="onInput"
      @change="$emit('change', ($event.target as HTMLTextAreaElement).value)"
      @blur="$emit('blur', $event)"
      @focus="$emit('focus', $event)"
    />

    <p v-if="error" class="fld__msg fld__msg--error">{{ error }}</p>
    <p v-else-if="hint" class="fld__msg">{{ hint }}</p>
  </div>
</template>

<style scoped>
.fld {
  width: 100%;
}

.fld__label {
  display: block;
  margin-bottom: 6px;
  font-size: var(--fs-md);
  color: var(--foreground);
}

.fld__req {
  color: var(--destructive);
}

.fld__area {
  display: block;
  width: 100%;
  padding: 8px 10px;
  border: 1px solid var(--input);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  line-height: 1.5;
  /* Vertical only: a horizontally resizable field can be dragged out of its
     own form, and nothing is gained by allowing it. */
  resize: vertical;
  transition: background var(--motion-hover);
}

.fld__area::placeholder {
  color: var(--muted-foreground);
}

/* Machine data. Mono at 1.75 leading so pasted records stay scannable line by
   line, and --fs-sm because these are identifiers, not prose. */
.fld__area[data-variant='mono'] {
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
  line-height: 1.75;
}

.fld__area:focus {
  outline: none;
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.fld__area[data-invalid] {
  border-color: var(--destructive);
}
.fld__area[data-invalid]:focus {
  box-shadow: 0 0 0 3px color-mix(in oklch, var(--destructive) 18%, transparent);
}

.fld__area:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.fld__area:read-only:not(:disabled) {
  background: var(--surface-subtle);
  border-color: var(--border-subtle);
}

.fld__msg {
  margin: 6px 0 0;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
  line-height: 1.45;
}

.fld__msg--error {
  color: var(--destructive);
}
</style>
