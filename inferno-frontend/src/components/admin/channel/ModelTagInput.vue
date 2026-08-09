<script setup lang="ts">
/**
 * ModelTagInput — part 02, section 13.
 *
 * Migration note from the prototype:
 *   "Replace getPlatformTagClass, which fills the whole tag with a platform
 *    colour, with a 5px dot from the series ramp. Add the count. The paste,
 *    enter, tab and backspace behaviour is already correct."
 *
 * The dot is a deliberate, product-owner-approved exception to June ground
 * rule 5 ("colour encodes state, never category"): an earlier build of this
 * file dropped the dot for exactly that reason, and the product owner has
 * since overruled that call. The colour here is series-ramp identity — which
 * platform this channel's models belong to — not UI chrome tint, so it sits
 * outside what rule 5 is protecting against, the same way a legend swatch or
 * a platform icon would. Do not delete this dot citing rule 5 again without
 * checking with the product owner first.
 *
 * The prototype's fixture colours the dot with `var(--sr-1)`, but no `--sr-*`
 * token exists anywhere in this codebase's token set (checked
 * `design-system/tokens/colors.css` and `design-system/inferno.css`) — it is
 * a placeholder in the mock data, not a real token. The actual, already
 * centralized platform colour source is `platformAccentColor()` in
 * `src/utils/platformColors.ts`: "single raw color per platform; consumers
 * derive washes/tints from it", the same source `GroupBadge.vue`,
 * `PlatformTypeBadge.vue` and `SupportedModelChip.vue` use for platform
 * identity. Reused here rather than inventing a hex value. `platform` was
 * already in the prop list and is now live again, driving the dot.
 *
 * Tags are model ids: technical identifiers, so the tag body and the draft
 * input both set --font-mono (01-TOKENS). The label above and the hint below
 * stay --font-sans, because they are prose, not identifiers.
 *
 * A tag is a badge, not a row: --r-xs, not the --r-md the field wrapper uses.
 *
 * The field wrapper borrows Input.vue's geometry (--r-md, 1px --input,
 * --card fill, the 1px + 3px focus composite) but it wraps and grows with
 * its content, so height is a min-height, not the fixed 36px .fld__input
 * uses. Focus lands on the draft input, not the wrapper, so the composite is
 * keyed off :focus-within.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { platformAccentColor } from '@/utils/platformColors'

const { t } = useI18n()

const props = defineProps<{
  models: string[]
  placeholder?: string
  platform?: string
}>()

const emit = defineEmits<{
  'update:models': [models: string[]]
}>()

const inputValue = ref('')
const inputRef = ref<HTMLInputElement>()

// Series-ramp colour for the per-tag dot, keyed off the field's platform.
// See the file header comment for why this dot exists at all.
const dotColor = computed(() => platformAccentColor(props.platform ?? ''))

// The count the migration note asks the redesign to add.
const tagCountLabel = computed(() =>
  t('admin.channels.form.modelTagCount', { count: props.models.length })
)

function addModel() {
  const val = inputValue.value.trim()
  if (!val) return
  if (!props.models.includes(val)) {
    emit('update:models', [...props.models, val])
  }
  inputValue.value = ''
}

function removeModel(idx: number) {
  const newModels = [...props.models]
  newModels.splice(idx, 1)
  emit('update:models', newModels)
}

function handleBackspace() {
  if (inputValue.value === '' && props.models.length > 0) {
    removeModel(props.models.length - 1)
  }
}

function handlePaste(e: ClipboardEvent) {
  e.preventDefault()
  const text = e.clipboardData?.getData('text') || ''
  const items = text.split(/[,\n;]+/).map(s => s.trim()).filter(Boolean)
  if (items.length === 0) return
  const unique = [...new Set([...props.models, ...items])]
  emit('update:models', unique)
  inputValue.value = ''
}
</script>

<template>
  <div class="mti">
    <div class="mti__wrap">
      <span v-for="(model, idx) in models" :key="idx" class="mti__tag">
        <span class="mti__tag-dot" :style="{ background: dotColor }" aria-hidden="true" />
        <span class="mti__tag-name">{{ model }}</span>
        <button
          type="button"
          class="mti__remove"
          :aria-label="`Remove ${model}`"
          :title="`Remove ${model}`"
          @click="removeModel(idx)"
        >
          <i class="hgi-stroke hgi-cancel-01 mti__remove-icon" aria-hidden="true" />
        </button>
      </span>
      <input
        ref="inputRef"
        v-model="inputValue"
        type="text"
        class="mti__input"
        :placeholder="models.length === 0 ? placeholder : ''"
        @keydown.enter.prevent="addModel"
        @keydown.tab.prevent="addModel"
        @keydown.delete="handleBackspace"
        @paste="handlePaste"
        @blur="addModel"
      />
    </div>
    <p class="mti__foot">
      <span>{{ t('admin.channels.form.modelInputHint', 'Press Enter to add, supports paste for batch import.') }}</span>
      <span class="mti__count">{{ tagCountLabel }}</span>
    </p>
  </div>
</template>

<style scoped>
.mti {
  width: 100%;
}

/* Input.vue's field geometry, grown: min-height instead of a fixed height,
   because tags wrap onto more than one row as the list grows. */
.mti__wrap {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  min-height: var(--s2a-h-md);
  padding: 5px 6px;
  border: 1px solid var(--input);
  border-radius: var(--r-md);
  background: var(--card);
  /* Background only, never border-color. */
  transition: background var(--motion-hover);
}

/* The draft input is what actually takes focus, so the composite is keyed
   off :focus-within on the wrapper rather than :focus on any one child. */
.mti__wrap:focus-within {
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

/* Badge tier: --r-xs, not the --r-md the wrapper around it uses. */
.mti__tag {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 24px;
  padding: 0 5px 0 8px;
  border-radius: var(--r-xs);
  background: var(--muted);
  color: var(--foreground);
}

/* Platform identity dot, 5px, series-ramp colour. Product-owner-approved
   exception to ground rule 5 ("colour encodes state, never category") — see
   the file header comment before removing this. */
.mti__tag-dot {
  flex: none;
  width: 5px;
  height: 5px;
  border-radius: 50%;
}

/* Model ids are technical identifiers: --font-mono per 01-TOKENS. */
.mti__tag-name {
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
}

.mti__remove {
  display: grid;
  place-items: center;
  width: 16px;
  height: 16px;
  border: 0;
  border-radius: var(--r-xs);
  background: transparent;
  color: var(--muted-foreground);
  cursor: pointer;
  transition: background var(--motion-hover);
}
.mti__remove:hover {
  background: var(--sidebar-accent);
  color: var(--foreground);
}

.mti__remove-icon {
  font-size: 10px; /* june-lint-disable ground-rule-4: icon glyph */
}

.mti__input {
  flex: 1;
  min-width: 130px;
  height: 24px;
  border: 0;
  background: transparent;
  color: var(--foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
  outline: none;
}
.mti__input::placeholder {
  /* Placeholder is prose ("Applies to every model in the channel", "Add a
     model id"), not an identifier, so it reads in the body face rather than
     inheriting the input's mono. */
  font-family: var(--font-sans);
  color: var(--muted-foreground);
}

.mti__foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 6px 0 0;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
  line-height: 1.45;
}

.mti__count {
  flex: none;
  white-space: nowrap;
}
</style>
