<script setup lang="ts">
/**
 * EmptyState — part 03, section 13.
 *
 * Migration note from the prototype:
 *   "The component is fine. The copy is not: the default title is
 *    common.noData, which is wrong for a filtered list, a failure and a
 *    permission wall. Four presets, each naming the cause and the way out."
 *
 * icon, title, description, actionText, actionTo are unchanged. Two
 * deliberate departures from the spec's literal prop table, both because 21
 * call sites already depend on the current runtime behaviour:
 *
 *   title default   the spec says remove the common.noData fallback "so a
 *                    caller has to say". Three call sites (UserErrorRequestsTable,
 *                    OpsErrorLogTable, UsageTable) pass only `message` and rely
 *                    on the default title to render at all; removing it would
 *                    blank a live heading. The fallback stays.
 *
 *   message prop     was declared but never read by the old template, so
 *                    those same three call sites were silently dropping their
 *                    text. "Duplicate of description, keep one" is honoured by
 *                    making description win when both are given, and message
 *                    fill in when it is not, rather than deleting the prop.
 *
 * actionIcon's default does flip from true to false ("only a create action
 * needs a glyph"): no call site passes action-icon explicitly, so nothing
 * observes the change.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Component } from 'vue'

const { t } = useI18n()

interface Props {
  icon?: Component | string
  title?: string
  description?: string
  actionText?: string
  actionTo?: string | object
  actionIcon?: boolean
  message?: string
}

const props = withDefaults(defineProps<Props>(), {
  description: '',
  actionIcon: false
})

const displayTitle = computed(() => props.title || t('common.noData'))
const displayDescription = computed(() => props.description || props.message || '')
const isStringIcon = computed(() => typeof props.icon === 'string')

defineEmits(['action'])
</script>

<template>
  <div class="empty2">
    <div class="empty2__icon">
      <slot name="icon">
        <i v-if="isStringIcon" class="hgi-stroke" :class="icon" aria-hidden="true" />
        <component :is="icon" v-else-if="icon" aria-hidden="true" />
        <i v-else class="hgi-stroke hgi-inbox" aria-hidden="true" />
      </slot>
    </div>

    <h3 class="empty2__title">{{ displayTitle }}</h3>

    <p v-if="displayDescription" class="empty2__desc">{{ displayDescription }}</p>

    <div v-if="actionText || $slots.action" class="empty2__action">
      <slot name="action">
        <component
          :is="actionTo ? 'RouterLink' : 'button'"
          v-if="actionText"
          :to="actionTo"
          type="button"
          class="empty2__btn"
          @click="!actionTo && $emit('action')"
        >
          <i v-if="actionIcon" class="hgi-stroke hgi-add-01" aria-hidden="true" />
          {{ actionText }}
        </component>
      </slot>
    </div>
  </div>
</template>

<style scoped>
/* Centred, 28px vertical padding, inside whatever container the content
 * would have filled: this replaces the source's 80px icon tile with a bare
 * glyph, since a card that holds nothing does not also need a card inside it. */
.empty2 {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 28px 22px;
  text-align: center;
}

.empty2__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 2px;
  font-size: 24px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
}

/* A Component icon (the other half of the `Component | string` prop) is
 * usually an inline SVG sized by its own viewBox, not this font-size. */
.empty2__icon :deep(svg) {
  width: 24px;
  height: 24px;
}

.empty2__title {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-xl);
  font-weight: 400;
}

.empty2__desc {
  margin: 0;
  max-width: 38ch;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  line-height: 1.55;
}

.empty2__action {
  margin-top: 4px;
}

/* Solid, not outlined: every real call site today is a "create the first X"
 * action (assign a subscription, create a group, a key...), the same case the
 * spec's own filled example shows. An outlined secondary weight is a
 * different, quieter call the source never made. */
.empty2__btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: var(--s2a-h-sm);
  padding: 0 14px;
  border: 0;
  border-radius: var(--r-md);
  background: var(--primary);
  color: var(--primary-foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
  text-decoration: none;
  cursor: pointer;
  transition: background var(--motion-hover);
}
.empty2__btn:hover {
  background: color-mix(in oklch, var(--primary) 92%, var(--foreground));
}
.empty2__btn i {
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph */
}
.empty2__btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
</style>
