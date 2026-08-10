<template>
  <header class="ph">
    <div class="ph__lead">
      <h1 class="ph__title">{{ title }}</h1>
      <p v-if="summary || $slots.summary" class="ph__summary">
        <slot name="summary">{{ summary }}</slot>
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
defineProps<{
  title: string
  /** One line. Use the slot instead when it needs a link or a count. */
  summary?: string
}>()
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
