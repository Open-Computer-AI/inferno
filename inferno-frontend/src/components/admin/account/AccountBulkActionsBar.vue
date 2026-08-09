<template>
  <div class="bulkbar" :data-active="hasSelection || undefined">
    <div class="bulkbar__info">
      <!-- Decorative status swatch, not a control: the prototype draws it as a
           plain span, not an input, because nothing here is toggleable by
           clicking the swatch itself. Dash for "some of the page", tick for
           "the whole filtered result set" -- the same two glyphs Checkbox.vue
           uses for indeterminate vs checked, so the vocabulary matches even
           though this one is inert. -->
      <span v-if="hasSelection" class="bulkbar__badge" :data-mode="allResultsSelected ? 'all' : 'partial'" aria-hidden="true">
        <i :class="['hgi-stroke', allResultsSelected ? 'hgi-tick-01' : 'hgi-remove-01']" />
      </span>

      <!-- The whole point of this bar: which of the three states is true has
           to read as a sentence, because "all results" is destructive and a
           tristate box only shows a mark, never which of the two large
           numbers it refers to. -->
      <span class="bulkbar__sentence">
        <template v-if="allResultsSelected">{{ t('admin.accounts.bulkActions.allInFilterSelected', { count: selectedCount }) }}</template>
        <template v-else-if="hasSelection">{{ t('admin.accounts.bulkActions.selected', { count: selectedCount }) }}</template>
        <template v-else>{{ t('admin.accounts.bulkEdit.title') }}</template>
      </span>

      <button v-if="hasSelection" type="button" class="bulkbar__link" @click="emit('select-page')">
        {{ allResultsSelected ? t('admin.accounts.bulkActions.selectPageOnly') : t('admin.accounts.bulkActions.selectCurrentPage') }}
      </button>

      <span v-if="hasSelection && canSelectAllResults" class="bulkbar__dot" aria-hidden="true">&middot;</span>
      <button
        v-if="canSelectAllResults"
        type="button"
        class="bulkbar__link"
        :disabled="selectingAll"
        :aria-busy="selectingAll || undefined"
        @click="emit('select-all-results')"
      >
        {{ selectingAll ? t('admin.accounts.bulkActions.selectingAll') : t('admin.accounts.bulkActions.selectAllResultsLink', { count: totalResults }) }}
      </button>
    </div>

    <div class="bulkbar__actions">
      <template v-if="hasSelection">
        <Button variant="secondary" size="xs" icon="hgi-edit-02" @click="emit('edit-selected')">
          {{ t('admin.accounts.bulkActions.edit') }}
        </Button>
        <Button variant="secondary" size="xs" icon="hgi-refresh-01" @click="emit('reset-status')">
          {{ t('admin.accounts.bulkActions.resetStatus') }}
        </Button>
        <Button variant="secondary" size="xs" icon="hgi-arrow-reload-horizontal" @click="emit('refresh-token')">
          {{ t('admin.accounts.bulkActions.refreshToken') }}
        </Button>
        <Button variant="secondary" size="xs" icon="hgi-money-exchange-01" @click="emit('probe-upstream-billing')">
          {{ t('admin.accounts.bulkActions.probeUpstreamBilling') }}
        </Button>
        <Button variant="success" size="xs" icon="hgi-play" @click="emit('toggle-schedulable', true)">
          {{ t('admin.accounts.bulkActions.enableScheduling') }}
        </Button>
        <Button variant="warning" size="xs" icon="hgi-pause" @click="emit('toggle-schedulable', false)">
          {{ t('admin.accounts.bulkActions.disableScheduling') }}
        </Button>
        <!-- Outlined, never filled: a solid red bar here would read as the
             primary action in a row that otherwise carries none. Routes
             through ConfirmDialog rather than emitting straight through, so
             the scope (page vs all results) gets restated at the one moment
             it is irreversible. -->
        <Button variant="danger" size="xs" icon="hgi-delete-02" @click="requestDelete">
          {{ t('admin.accounts.bulkActions.delete') }}
        </Button>
      </template>

      <!-- Always present, selection or not: bulk-editing everything that
           matches the current filter does not require checking a single row
           first. The one solid button in the row, because it is the only
           action that is always this bar's primary purpose. -->
      <Button variant="solid" size="xs" icon="hgi-edit-table" @click="emit('edit-filtered')">
        {{ t('admin.accounts.bulkEdit.submit') }}
      </Button>

      <IconButton
        v-if="hasSelection"
        icon="hgi-cancel-01"
        :label="t('admin.accounts.bulkActions.clear')"
        size="xs"
        @click="emit('clear')"
      />
    </div>
  </div>

  <ConfirmDialog
    :show="showDeleteConfirm"
    :title="t('admin.accounts.bulkActions.confirmDeleteTitle')"
    :message="deleteConfirmMessage"
    :confirm-text="t('admin.accounts.bulkActions.delete')"
    :cancel-text="t('common.cancel')"
    danger
    @confirm="confirmDelete"
    @cancel="cancelDelete"
  />
</template>

<script setup lang="ts">
/**
 * Bulk actions bar -- part 04, section 04 of the table library, 4 props.
 *
 * Migration note from the prototype:
 *   "Today it floats over the content. Docking it means the table never
 *    moves under the pointer when a selection starts, which is the actual
 *    complaint about floating bars."
 * Docking itself is a layout decision made where this component is slotted
 * (AccountsView.vue, out of scope here); this file supplies the bar.
 *
 * The prototype's headline: "Appears on selection and carries the two facts
 * that matter: how many are selected, and whether that is the page or the
 * whole result set. The select all across results path is real and
 * destructive, so it is a sentence, not a checkbox tristate the operator has
 * to interpret." That sentence is `bulkbar__sentence` below, with three
 * branches (all results / page selection / nothing yet) rather than a single
 * count plus a checked/indeterminate box.
 *
 * All 4 props, all 10 emits and no slots change: this is a presentation-only
 * rewrite of an existing call site (AccountsView.vue), not a new API.
 *
 * Verb count: the library's generic demo caps visible verbs at three plus an
 * overflow, with delete always in the overflow. This concrete instance has
 * seven distinct emits its one caller already depends on (edit, reset
 * status, refresh token, probe upstream billing, enable/disable scheduling,
 * delete) and the frozen prop table adds no `actions` config to collapse
 * them through. Building a bespoke overflow menu here would either drop a
 * caller's action or duplicate AccountActionMenu.vue, which another agent
 * owns concurrently. So every verb stays a direct button; delete is ordered
 * last and is the only one outlined in destructive red, which is the part of
 * the rule that is load-bearing (colour encodes state, never category).
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const { t } = useI18n()

const props = defineProps<{
  selectedIds: number[]
  totalResults: number
  selectingAll: boolean
  allResultsSelected: boolean
}>()

const emit = defineEmits<{
  delete: []
  'edit-selected': []
  'edit-filtered': []
  clear: []
  'select-page': []
  'select-all-results': []
  'toggle-schedulable': [enable: boolean]
  'reset-status': []
  'refresh-token': []
  'probe-upstream-billing': []
}>()

const selectedCount = computed(() => props.selectedIds.length)
const hasSelection = computed(() => selectedCount.value > 0)
const canSelectAllResults = computed(() => !props.allResultsSelected && props.totalResults > selectedCount.value)

const showDeleteConfirm = ref(false)
const requestDelete = () => {
  showDeleteConfirm.value = true
}
const confirmDelete = () => {
  showDeleteConfirm.value = false
  emit('delete')
}
const cancelDelete = () => {
  showDeleteConfirm.value = false
}

// Restates the scope, not just the count: "the selected accounts" (bounded,
// whatever the operator checked) and "all accounts matching your filter"
// (unbounded, grows with the result set) are different amounts of damage.
const deleteConfirmMessage = computed(() =>
  props.allResultsSelected
    ? t('admin.accounts.bulkActions.confirmDeleteAllInFilter', { count: selectedCount.value })
    : t('admin.accounts.bulkActions.confirmDelete', { count: selectedCount.value })
)
</script>

<style scoped>
/* 40px (--s2a-h-lg), docked above the table. Brand tint only while a
   selection is live -- ground rule 5, colour is spent on the state that
   exists, not sprayed on an idle toolbar. */
.bulkbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 10px;
  min-height: var(--s2a-h-lg);
  margin-bottom: 12px;
  padding: 5px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--card);
}

.bulkbar[data-active] {
  border-color: var(--brand-line);
  background: var(--brand-tint);
}

.bulkbar__info {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 10px;
  min-width: 0;
}

.bulkbar__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-left: auto;
}

/* 13px swatch: the prototype's own measurement, kept literal because it is
   deliberately smaller than Checkbox.vue's 15px control -- this one is not a
   control. Radius rounds to --r-xs, the nearest token to the prototype's 3px. */
.bulkbar__badge {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 13px;
  height: 13px;
  border-radius: var(--r-xs);
  background: var(--brand);
  color: var(--on-solid);
}
.bulkbar__badge i {
  font-size: 8px; /* june-lint-disable ground-rule-4: icon glyph */
}

.bulkbar__sentence {
  font-size: var(--fs-md);
  color: var(--foreground);
  white-space: nowrap;
}

.bulkbar__dot {
  color: var(--muted-foreground);
}

/* Plain text links, not Button.vue: the prototype draws these as bare
   anchors with no box. Hover still follows the house rule of background
   only, never a colour-only or underline hover. */
.bulkbar__link {
  border: 0;
  border-radius: var(--r-sm);
  margin: -3px -6px;
  padding: 3px 6px;
  background: transparent;
  color: var(--body-copy);
  font-family: inherit;
  font-size: var(--fs-sm);
  white-space: nowrap;
  cursor: pointer;
  transition: background var(--motion-hover);
}
.bulkbar__link:hover:not(:disabled) {
  background: var(--sidebar-accent);
}
.bulkbar__link:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
.bulkbar__link:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
</style>
