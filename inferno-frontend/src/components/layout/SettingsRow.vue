<template>
  <div class="sr">
    <div class="sr__lead">
      <label v-if="labelFor" class="sr__label" :for="labelFor">{{ label }}</label>
      <span v-else class="sr__label">{{ label }}</span>
      <p v-if="hint || $slots.hint" class="sr__hint">
        <slot name="hint">{{ hint }}</slot>
      </p>
    </div>
    <div class="sr__control">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * One setting. Label and optional hint on the left, the control on the right.
 *
 * `labelFor` renders a real `<label for>` so clicking the text focuses the
 * control. It is opt-in because a row whose control is a group of radios or a
 * button has nothing single to point at, and a `for` aimed at nothing is worse
 * than no label element at all.
 */
defineProps<{
  label: string
  hint?: string
  /** id of the control this row labels. Omit for rows with no single input. */
  labelFor?: string
}>()
</script>

<style scoped>
.sr {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 12px 14px;
}

/* Hairlines between rows, drawn by the row so the group needs no :deep(). */
.sr + .sr {
  border-top: 1px solid var(--border-subtle);
}

.sr__lead {
  min-width: 0;
}

.sr__label {
  display: block;
  color: var(--foreground);
  font-size: var(--fs-md);
}

.sr__hint {
  margin: 2px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  line-height: 1.45;
}

.sr__control {
  display: flex;
  flex: none;
  align-items: center;
  gap: 8px;
}

/* A control that has to be wide (a select, a text field) gets the room; one
   that does not (a toggle) stays its own size. */
@media (max-width: 600px) {
  .sr {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
  }

  .sr__control {
    justify-content: flex-start;
  }
}
</style>
