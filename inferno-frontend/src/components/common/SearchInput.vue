<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import IconButton from './IconButton.vue'

/**
 * SearchInput — part 02, section 10.
 *
 * Migration note from the prototype:
 *   "The source has no clear button, no pending indicator and no result
 *    count, so a slow query looks like a broken one. Add the clear button,
 *    the pending spinner and the result count. All three are presentation,
 *    so debounceMs and the two emits stay as they are."
 *
 * The prop and emit surface is unchanged except for one addition:
 *
 *   resultText  optional, new. The field only debounces and emits `search`;
 *               it never runs the query itself, so it cannot count matches
 *               on its own — the caller supplies the finished sentence
 *               ("5 of 96 accounts"). Left unset, the status line stays
 *               reserved and empty, exactly like every call site today.
 *
 * The pending spinner needed no new prop: it is on for exactly the debounce
 * window, from the keystroke to the delayed `search` emit, which is state
 * the field already owns and can derive itself.
 */
interface Props {
  modelValue: string
  placeholder?: string
  debounceMs?: number
  resultText?: string
}

const props = withDefaults(defineProps<Props>(), {
  placeholder: 'Search...',
  debounceMs: 300
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'search', value: string): void
}>()

const inputRef = ref<HTMLInputElement | null>(null)
const hasValue = computed(() => props.modelValue.length > 0)

// True from the keystroke to the delayed `search` emit. A hand-rolled timer
// rather than vueuse's useDebounceFn because clear() below needs a real
// cancel, not just a fresh call that races the in-flight one.
let timer: ReturnType<typeof setTimeout> | undefined
const pending = ref(false)

const scheduleSearch = (value: string) => {
  pending.value = true
  clearTimeout(timer)
  timer = setTimeout(() => {
    pending.value = false
    emit('search', value)
  }, props.debounceMs)
}

const onInput = (event: Event) => {
  const value = (event.target as HTMLInputElement).value
  emit('update:modelValue', value)
  scheduleSearch(value)
}

// Clearing is not a debounced edit, it is a direct reset: cancel whatever
// was pending and search immediately, so a stale in-flight query can never
// land after the field has already gone empty.
const clear = () => {
  clearTimeout(timer)
  pending.value = false
  emit('update:modelValue', '')
  emit('search', '')
  inputRef.value?.focus()
}

onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div class="srch">
    <div class="srch__wrap">
      <i class="hgi-stroke hgi-search-01 srch__icon" aria-hidden="true" />

      <input
        ref="inputRef"
        type="text"
        class="srch__input"
        :value="modelValue"
        :placeholder="placeholder"
        :data-has-clear="hasValue ? '' : undefined"
        @input="onInput"
      />

      <!-- Real button, not a bare glyph: needs an accessible name and a hit
           target of its own, not just a clickable icon. -->
      <IconButton
        v-if="hasValue"
        class="srch__clear"
        icon="hgi-cancel-01"
        label="Clear search"
        size="sm"
        @click="clear"
      />
    </div>

    <!-- Reserved at 17px whether or not it has content, so a result landing
         never shifts anything below the field. -->
    <div class="srch__status">
      <template v-if="pending">
        <span class="srch__spinner s2a-spinner" aria-hidden="true" />
        <span>Searching</span>
      </template>
      <template v-else-if="hasValue && resultText">
        <span>{{ resultText }}</span>
      </template>
    </div>
  </div>
</template>

<style scoped>
.srch {
  width: 100%;
}

.srch__wrap {
  position: relative;
  display: flex;
  align-items: center;
}

/* Icon glyph: fixed chrome, sized in px, must not scale with --font-scale. */
.srch__icon {
  position: absolute;
  left: 10px;
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
  pointer-events: none;
}

.srch__input {
  width: 100%;
  /* 32px, the toolbar height, exactly as the specimen draws it.
   *
   * Not Input.vue's 36px. Those two look like the same control and are not: a
   * form field sits in a column of fields at 36px, a search box sits in a
   * toolbar beside 32px buttons. Matching the field system here would put the
   * search box a row out of alignment with everything next to it. */
  height: var(--s2a-h-sm);
  padding: 0 11px 0 31px;
  border: 1px solid var(--input);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  /* Background only, never border-color (ground rule 6). */
  transition: background var(--motion-hover);
}

.srch__input::placeholder {
  color: var(--muted-foreground);
}

/* Room for the clear button once there is a value to clear. */
.srch__input[data-has-clear] {
  padding-right: 32px;
}

.srch__input:focus {
  outline: none;
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.srch__clear {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
}

.srch__status {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 17px;
  margin-top: 7px;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}

.srch__spinner {
  display: inline-block;
  width: 11px;
  height: 11px;
  border: 1.5px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: s2a-spin 0.7s linear infinite;
}
</style>
