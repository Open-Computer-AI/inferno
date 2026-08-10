<template>
  <section class="sg">
    <h2 v-if="title" class="sg__title">{{ title }}</h2>
    <p v-if="description" class="sg__desc">{{ description }}</p>
    <div class="sg__rows">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
/**
 * A settings group: part 14's archetype C, "Groups of rows, one border per
 * group, no card per setting."
 *
 * The "no card per setting" half is the load-bearing one. A card around every
 * toggle turns a settings page into a wall of boxes where nothing is grouped
 * and nothing is scannable; the border belongs to the GROUP, and rows inside
 * are separated by hairlines.
 *
 * Rows are `SettingsRow`. The group only draws the frame and the heading.
 */
defineProps<{
  title?: string
  /** One line under the heading. Anything longer belongs in the row's hint. */
  description?: string
}>()
</script>

<style scoped>
.sg + .sg {
  margin-top: 28px;
}

.sg__title {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
  line-height: 1.35;
}

.sg__desc {
  margin: 4px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  line-height: 1.45;
}

.sg__rows {
  margin-top: 10px;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--card);
  overflow: hidden;
}
</style>
