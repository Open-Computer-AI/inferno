<script setup lang="ts">
/**
 * AccountGroupsCell — table cells part 08, kind E ("Set").
 *
 * Migration note from the prototype (restCells, "AccountGroupsCell"):
 *   "Has a maxDisplay prop already, which is the right idea. Default drops
 *    from an unbounded list to two."
 * Kind E anatomy (part 08, section 01):
 *   "The first one or two members as chips, then a count that expands in
 *    place. Sixty model names never become sixty chips."
 *
 * Two changes from the pre-June version: `maxDisplay` defaults to 2 instead
 * of 4 (the row must stay one line inside a 36px cell; the previous 4-chip
 * default relied on `max-h-14` wrapping to two lines, which stretches the
 * row), and the popover, badge row and "+N" trigger move off Tailwind
 * utilities onto June tokens. GroupBadge was already composed here, not
 * reimplemented, and stays that way.
 *
 * Props are otherwise unchanged. AccountsView.vue's call site pins
 * `:max-display="4"` explicitly, which overrides this default and is out of
 * scope for this file — see the report for this part.
 */
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupBadge from '@/components/common/GroupBadge.vue'
import IconButton from '@/components/common/IconButton.vue'
import type { Group } from '@/types'

interface Props {
  groups: Group[] | null | undefined
  maxDisplay?: number
}

const props = withDefaults(defineProps<Props>(), {
  maxDisplay: 2
})

const { t } = useI18n()

const moreButtonRef = ref<HTMLElement | null>(null)
const showPopover = ref(false)

// Displayed chips (leaves one slot for the +N trigger when the list overflows).
const displayGroups = computed(() => {
  if (!props.groups) return []
  if (props.groups.length <= props.maxDisplay) return props.groups
  return props.groups.slice(0, props.maxDisplay - 1)
})

const hiddenCount = computed(() => {
  if (!props.groups) return 0
  if (props.groups.length <= props.maxDisplay) return 0
  return props.groups.length - (props.maxDisplay - 1)
})

// Popover position: anchored under the trigger, flipped above or shifted left
// when there isn't room. The panel itself is fixed-positioned (Teleported to
// body) so it is never clipped by the table's horizontal scroll container.
const popoverStyle = computed(() => {
  if (!moreButtonRef.value) return {}
  const rect = moreButtonRef.value.getBoundingClientRect()
  const viewportHeight = window.innerHeight
  const viewportWidth = window.innerWidth

  let top = rect.bottom + 8
  let left = rect.left

  if (top + 280 > viewportHeight) {
    top = Math.max(8, rect.top - 280)
  }
  if (left + 384 > viewportWidth) {
    left = Math.max(8, viewportWidth - 392)
  }

  return { top: `${top}px`, left: `${left}px` }
})

const handleKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape') showPopover.value = false
}

onMounted(() => window.addEventListener('keydown', handleKeydown))
onUnmounted(() => window.removeEventListener('keydown', handleKeydown))
</script>

<template>
  <div v-if="groups && groups.length > 0" class="gcell">
    <div class="gcell__row">
      <GroupBadge
        v-for="group in displayGroups"
        :key="group.id"
        :name="group.name"
        :platform="group.platform"
        :subscription-type="group.subscription_type"
        :rate-multiplier="group.rate_multiplier"
        :show-rate="false"
        class="gcell__badge"
      />
      <button
        v-if="hiddenCount > 0"
        ref="moreButtonRef"
        type="button"
        class="gcell__more"
        :aria-label="t('common.expand')"
        @click.stop="showPopover = !showPopover"
      >
        +{{ hiddenCount }}
      </button>
    </div>

    <Teleport to="body">
      <div v-if="showPopover" class="gcell__popover" :style="popoverStyle">
        <div class="gcell__popover-head">
          <span class="gcell__popover-count">{{ t('admin.accounts.groupCountTotal', { count: groups.length }) }}</span>
          <IconButton icon="hgi-cancel-01" :label="t('common.close')" size="xs" @click="showPopover = false" />
        </div>
        <div class="gcell__popover-list">
          <GroupBadge
            v-for="group in groups"
            :key="group.id"
            :name="group.name"
            :platform="group.platform"
            :subscription-type="group.subscription_type"
            :rate-multiplier="group.rate_multiplier"
            :show-rate="false"
          />
        </div>
      </div>
      <div v-if="showPopover" class="gcell__scrim" @click="showPopover = false" />
    </Teleport>
  </div>
  <span v-else class="gcell__empty">-</span>
</template>

<style scoped>
.gcell {
  min-width: 0;
  max-width: 100%;
}

/* Kind E: chips on one line, never two. The row must not grow the 36px
   table row, so overflow is hidden rather than wrapped. */
.gcell__row {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  overflow: hidden;
}

.gcell__badge {
  flex-shrink: 0;
  max-width: 96px;
}

.gcell__more {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-pill);
  background: var(--card);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  cursor: pointer;
  /* Background only, never border-color. */
  transition: background var(--motion-hover), color var(--motion-hover);
}
.gcell__more:hover {
  background: var(--sidebar-accent);
  color: var(--foreground);
}

.gcell__empty {
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}

/* --- popover: expands the set "in place" without touching row height ---- */
.gcell__popover {
  position: fixed;
  z-index: 50;
  min-width: 192px;
  max-width: 384px;
  padding: 12px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--popover);
  box-shadow: var(--shadow-md);
}

.gcell__popover-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}

.gcell__popover-count {
  font-size: var(--fs-xs);
  color: var(--muted-foreground);
}

.gcell__popover-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  max-height: 256px;
  overflow-y: auto;
}

.gcell__scrim {
  position: fixed;
  inset: 0;
  z-index: 40;
}
</style>
