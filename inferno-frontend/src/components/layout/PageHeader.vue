<template>
  <header class="ph">
    <div class="ph__lead">
      <h1 class="ph__title">{{ resolvedTitle }}</h1>
      <p v-if="resolvedSummary || $slots.summary" class="ph__summary">
        <slot name="summary">{{ resolvedSummary }}</slot>
      </p>
    </div>
    <div v-if="$slots.actions" class="ph__actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<script setup lang="ts">
/**
 * Title, one line of summary, and the page's own actions. Part 14 gives every
 * archetype the same head: "Title and one line summary" -- and for table pages
 * it is explicit that "nothing else is allowed above the table".
 *
 * The summary is ONE line by design. It is the only place on a screen that can
 * say what the screen is for, and a paragraph there pushes the first row of
 * real content below the fold on a laptop.
 *
 * Actions sit beside the title, not above the table, so the filter bar stays a
 * filter bar rather than becoming a second toolbar.
 */
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  /** Omit to take the route's own title, the same source as the tab title. */
  title?: string
  /** One line. Use the slot instead when it needs a link or a count. */
  summary?: string
}>()

/**
 * Falls back to route meta, exactly as TablePageLayout does.
 *
 * Every route already carries `title` + `titleKey`, and 30 carry a
 * `descriptionKey`. Reading them here means a page heading cannot drift from
 * the tab title, and a view opting into this layout needs no prop at all.
 */
const route = useRoute()
const { t, te } = useI18n()

const resolvedTitle = computed(() => {
  if (props.title) return props.title
  const key = route.meta?.titleKey
  if (typeof key === 'string' && te(key)) return t(key)
  const fallback = route.meta?.title
  return typeof fallback === 'string' ? fallback : ''
})

const resolvedSummary = computed(() => {
  if (props.summary) return props.summary
  const key = route.meta?.descriptionKey
  return typeof key === 'string' && te(key) ? t(key) : ''
})
</script>

<style scoped>
.ph {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.ph__lead {
  min-width: 0;
}

.ph__title {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.2;
}

.ph__summary {
  /* 4px, not 8: the summary belongs to the title, and a bigger gap makes it
     read as the first line of page content instead. */
  margin: 4px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  line-height: 1.45;
}

.ph__actions {
  display: flex;
  flex: none;
  align-items: center;
  gap: 8px;
}
</style>
