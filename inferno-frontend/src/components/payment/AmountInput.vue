<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'

/**
 * AmountInput — part 02, section 12.
 *
 * Migration note from the prototype:
 *   "The min and max already filter the presets. Add the same validation to
 *    the custom field, with the message before submit rather than a toast
 *    after it."
 *
 * The prop, emit surface is byte-for-byte what it was at
 * `payment/AmountInput.vue` — the spec marks all four props "unchanged" (min
 * and max already existed and already filtered presets). This is a
 * presentation rewrite: modelValue stays a plain number, exactly as both the
 * old component and the prototype's own `effective` value do, so no call
 * site's contract changes.
 *
 * Two things the prototype adds that the old component only had half of:
 *
 *   out-of-range copy   the old component filtered the preset chips by min
 *                        and max but only ever showed range hints inside the
 *                        placeholder. The prototype states the same min/max
 *                        rule as a persistent status line under the field
 *                        (`amt.hint`), in --destructive when the typed amount
 *                        is out of range, so the message is visible before
 *                        submit rather than surfacing as a toast after it.
 *   chip selection       the prototype's chip only lights up when it was
 *                        actually clicked (`!s.custom && s.amount === a`), not
 *                        whenever the typed custom amount happens to match a
 *                        preset's value. Preserved below: a chip is
 *                        `data-selected` only while the custom field is
 *                        empty, mirroring "selecting a preset clears the
 *                        custom field and the other way round."
 */
interface Props {
  modelValue: number | null
  amounts?: number[]
  min?: number
  max?: number
}

const props = withDefaults(defineProps<Props>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: number | null): void
}>()

const { t } = useI18n()

const customText = ref('')

// 0 = no limit, same convention the old component's filteredAmounts used.
const filteredAmounts = computed(() =>
  props.amounts.filter((a) => (props.min <= 0 || a >= props.min) && (props.max <= 0 || a <= props.max))
)

const placeholderText = computed(() => {
  if (props.min > 0 && props.max > 0) return `${props.min} - ${props.max}`
  if (props.min > 0) return `>= ${props.min}`
  if (props.max > 0) return `<= ${props.max}`
  return t('payment.enterAmount')
})

// Typed text is kept as a string and only parsed at the boundary, so a
// half-typed "12." or a trailing zero never gets silently reformatted out
// from under the person typing it.
const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/
const customNum = computed(() => parseFloat(customText.value))
const hasCustomNum = computed(() => !isNaN(customNum.value) && customNum.value > 0)
const overMax = computed(() => props.max > 0 && hasCustomNum.value && customNum.value > props.max)
const underMin = computed(() => props.min > 0 && hasCustomNum.value && customNum.value < props.min)
const outOfRange = computed(() => overMax.value || underMin.value)

const rangeHint = computed(() => {
  if (overMax.value) return `The maximum amount is $${props.max.toLocaleString()}.`
  if (underMin.value) return `The minimum amount is $${props.min.toLocaleString()}.`
  if (props.min > 0 && props.max > 0) return `Between $${props.min.toLocaleString()} and $${props.max.toLocaleString()}.`
  if (props.min > 0) return `At least $${props.min.toLocaleString()}.`
  if (props.max > 0) return `At most $${props.max.toLocaleString()}.`
  return ''
})

function isSelected(amount: number) {
  return customText.value === '' && props.modelValue === amount
}

function pickPreset(amount: number) {
  customText.value = ''
  emit('update:modelValue', amount)
}

function onInput(event: Event) {
  const val = (event.target as HTMLInputElement).value
  if (!AMOUNT_PATTERN.test(val)) return
  customText.value = val
  if (val === '') {
    emit('update:modelValue', null)
    return
  }
  const num = parseFloat(val)
  emit('update:modelValue', !isNaN(num) && num > 0 ? num : null)
}

watch(
  () => props.modelValue,
  (v) => {
    if (v === null) {
      customText.value = ''
      return
    }
    // A value that matches a preset reads as that preset being picked, not as
    // typed custom text, same as pickPreset() clearing the field on click.
    if (props.amounts.includes(v)) {
      customText.value = ''
    } else if (String(v) !== customText.value) {
      customText.value = String(v)
    }
  },
  { immediate: true }
)
</script>

<template>
  <div class="amt">
    <div class="amt__chips">
      <button
        v-for="a in filteredAmounts"
        :key="a"
        type="button"
        class="amt__chip"
        :data-selected="isSelected(a) ? '' : undefined"
        @click="pickPreset(a)"
      >
        {{ '$' + a.toLocaleString() }}
      </button>
    </div>

    <label class="amt__field">
      <span class="amt__label">Or a custom amount</span>
      <span class="amt__field-wrap">
        <span class="amt__prefix" aria-hidden="true">$</span>
        <input
          type="text"
          inputmode="decimal"
          class="amt__input"
          :value="customText"
          :placeholder="placeholderText"
          :data-invalid="outOfRange ? '' : undefined"
          :aria-invalid="outOfRange ? true : undefined"
          @input="onInput"
        />
      </span>
      <p v-if="rangeHint" class="amt__msg" :class="{ 'amt__msg--error': outOfRange }">{{ rangeHint }}</p>
    </label>
  </div>
</template>

<style scoped>
.amt {
  width: 100%;
}

.amt__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}

/* 62px is the widest four-figure preset ("$5,000") plus its padding, not a
   token: it is this control's own measurement, not a scale value. */
.amt__chip {
  min-width: 62px;
  height: var(--s2a-h-sm); /* 32px, the prototype's explicit chip height */
  padding: 0 12px;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--body-copy);
  font-family: inherit;
  font-size: var(--fs-md);
  cursor: pointer;
  /* Background only, never border-color. */
  transition: background var(--motion-hover);
}

/* Hover is neutral chrome (ground rule 5: colour encodes state, never
   category); a selected chip keeps its brand-tint state colour instead of
   being repainted grey on hover. */
.amt__chip:hover:not([data-selected]) {
  background: var(--sidebar-accent);
}

.amt__chip[data-selected] {
  border-color: var(--brand-line-strong);
  background: var(--brand-tint);
  color: var(--warm-strong);
}

.amt__field {
  display: block;
  margin-top: 14px;
}

.amt__label {
  display: block;
  margin-bottom: 6px;
  font-size: var(--fs-md);
  color: var(--foreground);
}

.amt__field-wrap {
  position: relative;
  display: block;
}

/* The affix is chrome, not accent: --muted-foreground, same as any prefix
   glyph in Input.vue. It is real currency text, not an icon glyph, so it
   takes a --fs-* token rather than the icon-glyph exception in ground rule 4. */
.amt__prefix {
  position: absolute;
  inset-block: 0;
  inset-inline-start: 0;
  display: flex;
  align-items: center;
  padding-inline-start: 13px;
  color: var(--muted-foreground);
  font-size: var(--fs-lg);
  pointer-events: none;
}

.amt__input {
  width: 100%;
  height: var(--s2a-h-lg); /* 40px: the checkout's primary field, not the 36px default */
  padding: 0 13px 0 30px;
  border: 1px solid var(--input);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-lg);
  transition: background var(--motion-hover);
}

.amt__input::placeholder {
  color: var(--muted-foreground);
}

/* The token composite: 1px ring plus a 3px warm halo, same as Input.vue. */
.amt__input:focus {
  outline: none;
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.amt__input[data-invalid] {
  border-color: var(--destructive);
}
.amt__input[data-invalid]:focus {
  box-shadow: 0 0 0 3px color-mix(in oklch, var(--destructive) 18%, transparent);
}

.amt__msg {
  margin: 6px 0 0;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
  line-height: 1.45;
}

.amt__msg--error {
  color: var(--destructive);
}
</style>
