<template>
  <div class="strip" :style="{ '--strip-cols': String(columns) }">
    <slot />
  </div>
</template>

<script setup lang="ts">
/**
 * The number strip from part 03, as part 14's fold specifies it: "One border,
 * dividers, no icon tiles, no shadows."
 *
 * That sentence is the whole component. The cells are `StatCard`s, which
 * already draw no border and no shadow of their own -- this supplies the single
 * border around the group and the hairlines between, so four numbers read as
 * one instrument rather than four floating cards.
 *
 * Four is the specified count above the fold ("the four number strip"). The
 * prop exists because the dashboards below the fold use eight, not because a
 * page should pick a number freely.
 */
withDefaults(
  defineProps<{
    columns?: number
  }>(),
  { columns: 4 }
)
</script>

<style scoped>
.strip {
  display: grid;
  grid-template-columns: repeat(var(--strip-cols), minmax(0, 1fr));
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--card);
  overflow: hidden;
}

/* Dividers are borders on the cells, not gaps in a grid: a gap would show the
   page through the strip and break the "one instrument" read. */
/* `:deep()` takes one selector, so `> :deep(*) + :deep(*)` does not compile to
   an adjacent-sibling rule -- it produced no divider at all. `:not(:first-child)`
   inside a single :deep() does. */
.strip > :deep(*:not(:first-child)) {
  border-left: 1px solid var(--border-subtle);
}

/* Below this a four-across strip puts each number in about 90px, which
   truncates the value -- the one thing the strip exists to show. Two across
   keeps them legible; the row divider replaces the column one. */
@media (max-width: 720px) {
  .strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .strip > :deep(*:not(:first-child)) {
    border-left: 0;
  }

  .strip > :deep(*:nth-child(even)) {
    border-left: 1px solid var(--border-subtle);
  }

  .strip > :deep(*:nth-child(n + 3)) {
    border-top: 1px solid var(--border-subtle);
  }
}
</style>
