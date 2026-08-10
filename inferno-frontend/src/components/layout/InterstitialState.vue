<template>
  <div class="int" :data-tone="tone">
    <span v-if="tone === 'waiting'" class="int__spinner" aria-hidden="true" />
    <i v-else class="int__icon hgi-stroke" :class="icon" aria-hidden="true" />

    <h1 class="int__title">{{ title }}</h1>
    <p v-if="body" class="int__body">{{ body }}</p>

    <!-- A wait with no recovery path is a dead end. Part 14 is explicit that an
         interstitial owes "a real recovery path when it fails" -- so the action
         slot is the point of the component, not decoration on it. -->
    <div v-if="$slots.action || $slots.alt" class="int__actions">
      <slot name="action" />
      <slot name="alt" />
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * Archetype F. Nine interstitial routes collapse to this one component: the
 * OAuth callbacks, email verify, and the payment redirects all show the same
 * three things -- a wait, a statement of what is being waited for, and a way
 * out when it fails.
 *
 * The three tones are the spec's own states:
 *
 *   waiting      "Signing you in" -- a spinner, no icon, no action. Neutral.
 *   attention    "That link has already been used" -- recoverable, and the
 *                recovery is the action.
 *   failed       "Google could not verify you" -- names the provider's own
 *                error and says what was NOT changed, because the first thing
 *                a person wants to know is whether they broke something.
 *
 * Tone drives colour and nothing else drives colour: ground rule 5 reserves it
 * for state, and "waiting" is not a state worth colouring.
 */
withDefaults(
  defineProps<{
    tone?: 'waiting' | 'attention' | 'failed'
    title: string
    body?: string
    /** Hugeicons class. Ignored while waiting, which shows the spinner. */
    icon?: string
  }>(),
  { tone: 'waiting', icon: 'hgi-alert-circle' }
)
</script>

<style scoped>
.int {
  display: flex;
  max-width: 380px;
  margin: 0 auto;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 24px 20px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--card);
  text-align: center;
}

.int[data-tone='attention'] {
  border-color: color-mix(in oklch, var(--s2a-attn) 26%, transparent);
  background: var(--s2a-attn-soft);
}

.int[data-tone='failed'] {
  border-color: color-mix(in oklch, var(--destructive) 24%, transparent);
  background: var(--destructive-soft);
}

.int__icon {
  /* Glyphs are fixed chrome and must not scale with --font-scale. */
  font-size: 20px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
}

.int[data-tone='attention'] .int__icon {
  color: var(--s2a-attn);
}

.int[data-tone='failed'] .int__icon {
  color: var(--destructive);
}

.int__spinner {
  display: inline-block;
  height: 18px;
  width: 18px;
  border: 2px solid var(--muted-foreground);
  border-top-color: transparent;
  border-radius: 50%;
  opacity: 0.7;
  /* Shared keyframe; it carries its own reduced-motion guard. */
  animation: s2a-spin 0.7s linear infinite;
}

.int__title {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
  line-height: 1.35;
}

.int__body {
  margin: 0;
  color: var(--body-copy);
  font-size: var(--fs-md);
  line-height: 1.5;
}

.int__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
  margin-top: 4px;
}
</style>
