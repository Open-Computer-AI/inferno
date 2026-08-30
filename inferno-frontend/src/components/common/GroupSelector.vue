<template>
  <!-- No trigger/popover: the prototype renders this permanently open, unlike
       Select. Root stays a single element so aria-labelledby and data-tour,
       passed as fallthrough attrs by BulkEditAccountModal/EditAccountModal/
       CreateAccountModal, keep landing on the one visible box. -->
  <div class="grpsel">
    <div class="grpsel__header">
      <i v-if="isSearchable" class="hgi-stroke hgi-search-01" aria-hidden="true" />
      <input
        v-if="isSearchable"
        v-model="searchText"
        type="text"
        class="grpsel__search"
        :placeholder="t('common.searchGroupsPlaceholder')"
        :aria-label="t('common.searchGroupsPlaceholder')"
      />
      <span v-else class="grpsel__title">{{ t('admin.users.groups') }}</span>
      <span class="grpsel__count">{{ t('common.selectedCount', { count: modelValue.length }) }}</span>
    </div>

    <div class="grpsel__list" role="group" :aria-label="t('admin.users.groups')">
      <div
        v-for="group in filteredGroups"
        :key="group.id"
        class="grpsel__row"
        :data-selected="modelValue.includes(group.id) || undefined"
        :title="t('admin.groups.rateAndAccounts', { rate: group.rate_multiplier, count: group.account_count || 0 })"
        @click="onRowClick(group.id, $event)"
      >
        <Checkbox
          :model-value="modelValue.includes(group.id)"
          :aria-label="group.name"
          @update:model-value="handleChange(group.id, $event)"
        />

        <span class="grpsel__meta">
          <span class="grpsel__name">{{ group.name }}</span>
          <!-- Platform is category, not state: no colour, plain muted text.
               Replaces GroupBadge's platform-tinted pill, which spent colour
               on a thing that never changes for a given group. -->
          <span class="grpsel__sub">{{ platformLabel(group.platform) }} · {{ group.rate_multiplier }}x</span>
        </span>

        <span class="grpsel__cap">
          <span class="grpsel__bar">
            <span
              class="grpsel__bar-fill"
              :style="{ width: capacityPct(group) + '%', background: capacityColor(group) }"
            />
          </span>
          <span class="grpsel__cap-label">{{ capacityLabel(group) }}</span>
        </span>
      </div>

      <div v-if="filteredGroups.length === 0" class="grpsel__empty">
        {{ t('common.noGroupsAvailable') }}
      </div>
    </div>

    <div v-if="footerText" class="grpsel__footer">{{ footerText }}</div>
  </div>
</template>

<script setup lang="ts">
/**
 * GroupSelector — part 02, section 14.
 *
 * Migration note from the prototype:
 *   "Capacity is not shown today, so an operator picks groups blind. The bar
 *    is the one real addition. The platform filter footer replaces silently
 *    hiding the ineligible groups."
 *
 * The props table marks every prop "unchanged" except `groups`, annotated
 * "now also reads capacity off the group". AdminGroup (src/types/index.ts)
 * has no dedicated capacity/max field, and types/index.ts is out of scope for
 * this file set, so the bar below approximates capacity from fields
 * GroupsView.vue already surfaces for the same purpose: account_count and
 * rate_limited_account_count. Fill = the rate-limited share of the group's
 * accounts (how much of it is currently constrained); past 80% it crosses
 * into the attention colour, mirroring "over 80 percent" in spec.tokens. This
 * is an honest approximation, not a hard ceiling like the prototype's mockup
 * data implies — flagging it here rather than silently inventing a field.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { platformLabel } from '@/utils/platformColors'
import type { AdminGroup, GroupPlatform } from '@/types'
import Checkbox from '@/components/common/Checkbox.vue'

const { t } = useI18n()

interface Props {
  modelValue: number[]
  groups: AdminGroup[]
  platform?: GroupPlatform // Optional platform filter
  mixedScheduling?: boolean // For antigravity accounts: allow anthropic/gemini groups
  searchable?: boolean | 'auto'
}

const props = withDefaults(defineProps<Props>(), {
  searchable: 'auto'
})
const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

const searchText = ref('')

const isSearchable = computed(() => {
  if (props.searchable === 'auto') return props.groups.length > 5
  return props.searchable
})

// Platform-only filter, no search: shared by filteredGroups and the footer's
// "some groups are hidden" check, so the hidden count is real rather than
// re-derived from a different filter path.
const platformFilteredGroups = computed(() => {
  let result: AdminGroup[] = props.groups
  if (props.platform) {
    if (props.platform === 'antigravity' && props.mixedScheduling) {
      result = result.filter(
        (g) => g.platform === 'antigravity' || g.platform === 'anthropic' || g.platform === 'gemini' || g.platform === 'composite'
      )
    } else {
      result = result.filter((g) => g.platform === props.platform || g.platform === 'composite')
    }
  }
  return result
})

const filteredGroups = computed(() => {
  let result = platformFilteredGroups.value
  if (isSearchable.value && searchText.value) {
    const q = searchText.value.toLowerCase()
    result = result.filter(
      (g) => g.name.toLowerCase().includes(q) || g.description?.toLowerCase().includes(q)
    )
  }
  return result
})

const hiddenByPlatform = computed(() => props.groups.length - platformFilteredGroups.value.length)

// Replaces silently hiding ineligible groups (migration note): explain why
// the list is short instead of leaving the operator to guess.
const footerText = computed(() => {
  if (!props.platform || hiddenByPlatform.value <= 0) return null
  if (props.platform === 'antigravity' && !props.mixedScheduling) {
    return t('common.groupSelectorMixedSchedulingHint')
  }
  return t('common.groupSelectorPlatformHint')
})

const capacityRatio = (group: AdminGroup): number => {
  const total = group.account_count || 0
  if (total <= 0) return 0
  const limited = group.rate_limited_account_count || 0
  return Math.min(1, limited / total)
}

const capacityPct = (group: AdminGroup): number => Math.round(capacityRatio(group) * 100)

// Colour is state (how constrained the group is right now), never category.
const capacityColor = (group: AdminGroup): string =>
  capacityRatio(group) > 0.8 ? 'var(--s2a-attn)' : 'var(--brand)'

const capacityLabel = (group: AdminGroup): string => {
  const total = group.account_count || 0
  if (total <= 0) return t('common.groupCapacityEmpty')
  const limited = group.rate_limited_account_count || 0
  if (limited > 0) return t('common.groupCapacityRateLimited', { limited, total })
  return t('common.groupCapacityTotal', { total })
}

// The row advertises a full-width target (cursor + hover background) and
// upstream got that for free by making the row itself a <label>. Ours cannot:
// Checkbox already renders its own <label>, and nesting labels is invalid. So
// the row forwards the click, skipping events that came from the checkbox
// itself -- those already toggle and would otherwise toggle twice.
const onRowClick = (groupId: number, event: MouseEvent) => {
  if ((event.target as HTMLElement | null)?.closest('.chk')) return
  handleChange(groupId, !props.modelValue.includes(groupId))
}

const handleChange = (groupId: number, checked: boolean) => {
  const newValue = checked
    ? [...props.modelValue, groupId]
    : props.modelValue.filter((id) => id !== groupId)
  emit('update:modelValue', newValue)
}
</script>

<style scoped>
.grpsel {
  width: 100%;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--card);
  overflow: hidden;
}

.grpsel__header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 11px;
  color: var(--muted-foreground);
}
.grpsel__header i {
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph */
  flex-shrink: 0;
}

.grpsel__search {
  flex: 1;
  min-width: 0;
  border: 0;
  background: transparent;
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  outline: none;
}
.grpsel__search::placeholder {
  color: var(--muted-foreground);
}

.grpsel__title {
  flex: 1;
  min-width: 0;
  color: var(--foreground);
  font-size: var(--fs-md);
}

.grpsel__count {
  flex-shrink: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.grpsel__list {
  max-height: 260px;
  overflow-y: auto;
}

/* Only the row carries border-top, not the header too: the two together
   would double the hairline between the header and the first row. */
.grpsel__row {
  position: relative;
  display: grid;
  grid-template-columns: 15px minmax(0, 1fr) 96px;
  align-items: center;
  gap: 10px;
  min-height: 38px;
  padding: 7px 11px;
  border-top: 1px solid var(--border-subtle);
  cursor: pointer;
  transition: background var(--motion-hover);
}

/* Hover and selected share one token, same as Select's option rows: colour
   is spent on state, not doubled between "pointing at" and "is the value". */
.grpsel__row:hover,
.grpsel__row[data-selected] {
  background: var(--sidebar-accent);
}

.grpsel__input {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

/* 15 square, --r-xs, checked fills --brand: matches Checkbox.vue's chrome. */
.grpsel__box {
  display: grid;
  place-items: center;
  width: 15px;
  height: 15px;
  border: 1px solid var(--border);
  border-radius: var(--r-xs);
  background: var(--card);
  color: var(--on-solid);
  transition: background var(--t-fast) var(--ease-out);
}
.grpsel__box[data-checked] {
  border-color: var(--brand);
  background: var(--brand);
}
.grpsel__tick {
  font-size: 10px; /* june-lint-disable ground-rule-4: icon glyph */
}

.grpsel__input:focus-visible + .grpsel__box {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.grpsel__meta {
  min-width: 0;
}
.grpsel__name {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--fs-md);
  color: var(--foreground);
}
.grpsel__sub {
  display: block;
  font-size: var(--fs-2xs);
  color: var(--muted-foreground);
}

.grpsel__cap {
  display: block;
}
.grpsel__bar {
  display: block;
  height: 4px;
  border-radius: 2px;
  background: var(--muted);
  overflow: hidden;
}
.grpsel__bar-fill {
  display: block;
  height: 100%;
  /* Background only; the fill's width is layout, not a colour transition. */
  transition: background var(--motion-hover);
}
.grpsel__cap-label {
  display: block;
  margin-top: 3px;
  font-size: var(--fs-2xs);
  color: var(--muted-foreground);
}

.grpsel__empty {
  padding: 12px 8px;
  text-align: center;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.grpsel__footer {
  padding: 8px 11px;
  border-top: 1px solid var(--border-subtle);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}
</style>
