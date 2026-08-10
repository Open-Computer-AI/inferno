<template>
  <article class="msg">
    <h1 v-if="title" class="msg__title">{{ title }}</h1>
    <p v-if="lead" class="msg__lead">{{ lead }}</p>
    <div class="msg__body">
      <slot />
    </div>
    <div v-if="$slots.actions" class="msg__actions">
      <slot name="actions" />
    </div>
  </article>
</template>

<script setup lang="ts">
/**
 * Archetype G: "A single readable column at --content-max with no chrome of its
 * own." Not found, legal documents, custom pages, guides -- seven routes whose
 * only job is to be read.
 *
 * "No chrome of its own" is the rule that matters: no card, no border, no
 * panel. The page background IS the page. A message page that draws a card
 * around its own text is pretending to be a dashboard.
 */
defineProps<{
  title?: string
  /** The one-sentence answer, before the body. */
  lead?: string
}>()
</script>

<style scoped>
.msg {
  /* The reading measure from 01-TOKENS. A legal document set to the full width
     of a 2560px monitor is unreadable regardless of how correct the type is. */
  max-width: var(--content-max);
  margin: 0 auto;
  padding: 8px 0 48px;
}

.msg__title {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.2;
}

.msg__lead {
  margin: 8px 0 0;
  color: var(--body-copy);
  font-size: var(--fs-lg);
  line-height: 1.55;
}

.msg__body {
  margin-top: 20px;
  color: var(--body-copy);
  font-size: var(--fs-md);
  line-height: 1.6;
}

.msg__actions {
  display: flex;
  gap: 8px;
  margin-top: 24px;
}
</style>
