<script setup lang="ts">
/**
 * A number input that does not fight you while you type.
 *
 * There are 23 raw `<input type="number" v-model.number>` controls across the
 * ops screens alone, and every one of them has the same two defects.
 *
 * IT COERCED ON EVERY KEYSTROKE
 * Clearing a field to retype it yields "", which `v-model.number` turns into
 * NaN or an empty string in the model, and any parent that guards against that
 * snaps the value back before the next digit lands. The draft here holds the
 * raw text and only commits on `change`, which fires on blur or Enter -- so
 * the field is empty while you are mid-edit, and valid the moment you leave.
 *
 * min AND max WERE DECORATIVE
 * A browser enforces them on the spinner arrows and on form validation. It
 * does NOT stop you typing 5000 into a field declared `max="100"`, and the old
 * code sent that straight to the API. Commit clamps, so the bound value is
 * always inside the declared range.
 *
 * `fallback` is what an unparseable or empty commit resolves to -- the previous
 * value would be friendlier still, but these are settings forms where a blank
 * field must not silently become 0.
 */
import { ref, watch } from 'vue'
import Input from './Input.vue'

const props = withDefaults(
  defineProps<{
    modelValue: number | null | undefined
    label?: string
    hint?: string
    error?: string
    placeholder?: string
    disabled?: boolean
    min?: number
    max?: number
    step?: number
    /** Used when the field is committed empty or unparseable. */
    fallback?: number
    /** Whole numbers only; a count of seconds should not accept 1.5. */
    integer?: boolean
  }>(),
  { fallback: 0, integer: false }
)

const emit = defineEmits<{ (e: 'update:modelValue', value: number): void }>()

const draft = ref(props.modelValue == null ? '' : String(props.modelValue))

/* Only re-sync when the committed value genuinely differs from what the draft
   already represents. Without that guard, committing "007" would rewrite the
   draft to "7" mid-edit on the next parent render. */
watch(
  () => props.modelValue,
  (v) => {
    const next = v == null ? '' : String(v)
    if (Number(draft.value) !== Number(next) || draft.value === '') draft.value = next
  }
)

function commit() {
  const raw = props.integer ? Number.parseInt(draft.value, 10) : Number.parseFloat(draft.value)
  let value = Number.isFinite(raw) ? raw : props.fallback
  if (props.min != null) value = Math.max(props.min, value)
  if (props.max != null) value = Math.min(props.max, value)
  draft.value = String(value)
  emit('update:modelValue', value)
}
</script>

<template>
  <Input
    v-model="draft"
    type="number"
    :label="label"
    :hint="hint"
    :error="error"
    :placeholder="placeholder"
    :disabled="disabled"
    :min="min"
    :max="max"
    :step="step"
    @change="commit"
  />
</template>
