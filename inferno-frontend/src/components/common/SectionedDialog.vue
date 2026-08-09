<template>
  <BaseDialog
    :show="show"
    :title="title"
    :width="width"
    :close-on-escape="closeOnEscape"
    :close-on-click-outside="closeOnClickOutside"
    :show-close-button="showCloseButton"
    @close="emit('close')"
  >
    <!-- `data-rail` mirrors Button/Select's data-attribute-for-a-decision house
         style: whether the rail shows is one decision (is the current section a
         blocking question still unanswered), not a class per combination.
         Negative margin cancels BaseDialog's 16px `.dlg-body` padding: the rail
         and the divider between it and the form need to bleed to the panel's
         edges, per the prototype mock, and BaseDialog owns that padding so this
         is the only way to reach it without editing the shell. -->
    <div class="secd" :data-rail="showRail ? '' : undefined">
      <nav v-if="showRail" class="secd__rail" :aria-label="t('common.sectionedDialog.railLabel')">
        <ol class="secd__list" role="list">
          <li v-for="(section, index) in sections" :key="section.id">
            <button
              type="button"
              class="secd__item"
              :aria-current="section.id === activeId ? 'step' : undefined"
              :aria-disabled="isGated(index) || undefined"
              :data-active="section.id === activeId || undefined"
              :title="isGated(index) ? t('common.sectionedDialog.sectionGated') : undefined"
              @click="selectSection(section, index)"
            >
              <i
                :class="['hgi-stroke', markerIcon(section)]"
                class="secd__marker"
                :style="{ color: markerColor(section, section.id === activeId) }"
                aria-hidden="true"
              />
              <span class="secd__label">{{ section.label }}</span>
            </button>
          </li>
        </ol>
        <p v-if="$slots['rail-hint']" class="secd__hint"><slot name="rail-hint" /></p>
      </nav>

      <!-- One region, labelled by whichever section is active. Not a
           role="tabpanel": switching sections here also changes what is
           required and what gates what, which is a step in a process rather
           than an alternate view of identical content, so the ARIA tabs
           pattern (with its mandatory arrow-key contract) is the wrong fit.
           A named landmark plus aria-current on the rail item is enough to
           announce where you are. -->
      <div class="secd__panel" :aria-label="activeSection?.label">
        <slot :name="`section-${activeId}`" :section="activeSection" />
      </div>
    </div>

    <template #footer>
      <!-- Full width so it can put the step count on the left and the caller's
           own actions on the right, overriding dlg-footer's default
           justify-flex-end -- the same move ConfirmDialog's .cf-actions makes. -->
      <div class="secd__foot">
        <span v-if="showStepCount && sections.length" class="secd__step">
          {{ t('common.sectionedDialog.stepOf', { current: activeIndex + 1, total: sections.length }) }}
        </span>
        <span v-else class="secd__step" />
        <div class="secd__actions">
          <slot
            name="actions"
            :active-id="activeId"
            :active-index="activeIndex"
            :section-count="sections.length"
            :is-first-section="isFirstSection"
            :is-last-section="isLastSection"
            :go-to-previous="goToPrevious"
            :go-to-next="goToNext"
          />
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
/**
 * SectionedDialog — part 05, section 03 "Sectioned form". Archetype C, 7
 * modals (CreateAccountModal, EditAccountModal, MonitorFormDialog,
 * UserCreateModal, UserEditModal, BulkEditAccountModal, BulkEditUserModal).
 *
 * This is the primitive the prototype specifies, not a migration of any one
 * of those seven. CreateAccountModal (6,338 lines) and EditAccountModal
 * (4,799 lines) are explicitly out of scope: part 05's own "calls" section
 * names splitting them "a project rather than a pass" that needs a
 * credential component per platform behind a common interface first. This
 * component only builds the shell that project will eventually sit inside.
 *
 * Migration note from the prototype (spec.sectioned):
 *   "720px panel, 172px rail on the left, form column on the right, 52px
 *    footer with a step count. The rail lists only sections the current
 *    answers make relevant."
 * and from the section body:
 *   "The platform choice moves to the top and is the only thing on screen
 *    until it is answered, because everything else depends on it."
 *
 * That second line is the gating behaviour, generalised: this component knows
 * nothing about platforms. A section carries `blocking: true` when later
 * sections depend on its answer; until that section's `state` is 'complete',
 * every section after it is unreachable, and while the *active* section is
 * itself an unanswered blocking one, the rail does not render at all -- the
 * question really is the only thing on screen, not a list of mostly-disabled
 * steps beside it. Once any blocking section is answered, the rail returns
 * and later sections render as real, focusable, aria-disabled rail items
 * rather than being hidden -- ground rule from part 02: a gated control stays
 * in the accessibility tree so it can explain itself, it just cannot act.
 *
 * Composes BaseDialog for the shell (scrim, teleport, focus, escape, body
 * scroll lock) and reuses its `footer` slot; this component only owns the
 * rail, the section switcher and the fixed footer layout (step count left,
 * actions right). It never reimplements the dialog itself.
 *
 * Each of the seven consumers supplies its own fields through a dynamically
 * named slot, `section-<id>`, one per entry in `sections`. Only the active
 * section's slot is invoked, so a seven-platform form never mounts six
 * credential shapes it isn't showing -- the actual fix for the 6,338 line
 * file, once someone does the splitting project this shell is built for.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from './BaseDialog.vue'

const { t } = useI18n()

export type SectionedDialogSectionState = 'pending' | 'complete' | 'invalid'

export interface SectionedDialogSection {
  id: string
  label: string
  /** Hugeicons stroke name, e.g. "hgi-user-01". Shown only while the section
   *  has no state yet -- ground rule 5: a section's identity is category and
   *  may not carry colour, so this glyph always renders in
   *  --muted-foreground or, when the section is active, the same --brand the
   *  active row's text takes. It never encodes anything on its own. */
  icon?: string
  /** Rail marker. Unset reads as 'pending'. 'complete' and 'invalid' replace
   *  the marker with a checkmark or an alert glyph and own their colour --
   *  that is state, which is allowed the accent. */
  state?: SectionedDialogSectionState
  /** When true, every section after this one is unreachable until this
   *  section's state is 'complete'. This is the whole gate: a plain boolean
   *  on whichever section the rest of the form depends on, not a concept
   *  this component has any other opinion about. */
  blocking?: boolean
}

interface Props {
  show: boolean
  title: string
  sections: SectionedDialogSection[]
  /** Controlled active section id (v-model:active-id). */
  activeId: string
  width?: 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  /** The "Step N of total" label. Off for consumers whose sections are not a
   *  linear count, e.g. a settings-style dialog with no wizard order. */
  showStepCount?: boolean
}

interface Emits {
  (e: 'update:activeId', id: string): void
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  width: 'wide',
  closeOnEscape: true,
  closeOnClickOutside: false,
  showCloseButton: true,
  showStepCount: true
})

const emit = defineEmits<Emits>()

const activeIndex = computed(() => props.sections.findIndex((s) => s.id === props.activeId))
const activeSection = computed<SectionedDialogSection | null>(() => props.sections[activeIndex.value] ?? null)

/** A section at `index` is gated when some earlier section is blocking and
 *  still not complete. Stacked gates (more than one blocking section) just
 *  compound: nothing after the first unanswered one is reachable. */
function isGated(index: number): boolean {
  for (let i = 0; i < index; i++) {
    const s = props.sections[i]
    if (s.blocking && s.state !== 'complete') return true
  }
  return false
}

/** The one case the rail itself disappears: the active section is the
 *  question everything else depends on, and it has no answer yet. That is
 *  the prototype's "only thing on screen", made generic. */
const showRail = computed(() => !(activeSection.value?.blocking && activeSection.value.state !== 'complete'))

function markerIcon(section: SectionedDialogSection): string {
  if (section.state === 'complete') return 'hgi-checkmark-circle-01'
  if (section.state === 'invalid') return 'hgi-alert-circle'
  return section.icon || 'hgi-circle'
}

function markerColor(section: SectionedDialogSection, isActive: boolean): string {
  if (section.state === 'complete') return 'var(--brand)'
  if (section.state === 'invalid') return 'var(--destructive)'
  return isActive ? 'var(--brand)' : 'var(--muted-foreground)'
}

function selectSection(section: SectionedDialogSection, index: number) {
  if (isGated(index)) return
  emit('update:activeId', section.id)
}

const isFirstSection = computed(() => activeIndex.value <= 0)
const isLastSection = computed(() => activeIndex.value === props.sections.length - 1)

function goToPrevious() {
  const idx = activeIndex.value
  if (idx > 0) emit('update:activeId', props.sections[idx - 1].id)
}

function goToNext() {
  const idx = activeIndex.value
  const nextIndex = idx + 1
  if (nextIndex < props.sections.length && !isGated(nextIndex)) {
    emit('update:activeId', props.sections[nextIndex].id)
  }
}
</script>

<style scoped>
.secd {
  display: grid;
  grid-template-columns: 172px minmax(0, 1fr);
  /* Cancels .dlg-body's 16px padding so the rail's fill and its divider
     reach the panel edges; each half restores its own padding below. */
  margin: -16px;
}
.secd[data-rail] {
  min-height: calc(100% + 32px);
}
.secd:not([data-rail]) {
  grid-template-columns: minmax(0, 1fr);
}

.secd__rail {
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 12px 10px;
  border-right: 1px solid var(--border-subtle);
  background: var(--surface-subtle);
}

.secd__list {
  display: grid;
  gap: 1px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.secd__item {
  display: grid;
  grid-template-columns: 16px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 28px;
  padding: 0 8px;
  border: 0;
  border-radius: var(--r-md);
  background: transparent;
  color: var(--body-copy);
  font-family: inherit;
  font-size: var(--fs-md);
  text-align: left;
  cursor: pointer;
  /* Background only, never border-color (ground rule 6). */
  transition: background var(--motion-hover);
}

/* Active carries the accent -- state, not category (ground rule 5). */
.secd__item[aria-current='step'] {
  background: var(--brand-tint);
  color: var(--warm-strong);
}

.secd__item:not([aria-current='step']):not([aria-disabled]):hover {
  background: var(--sidebar-accent);
}

.secd__item:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

/* aria-disabled, not the disabled attribute: part 02's rule is that a gated
   control stays reachable so it can explain itself (here, via `title`)
   rather than dropping silently out of the tab order. Mirrors Button.vue. */
.secd__item[aria-disabled='true'] {
  opacity: 0.55;
  cursor: not-allowed;
}

.secd__marker {
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
}

.secd__label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.secd__hint {
  margin: 8px 0 0;
  padding: 0 8px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  line-height: 1.5;
}

.secd__panel {
  min-width: 0;
  padding: 16px;
}

.secd__foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}

.secd__step {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.secd__actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
