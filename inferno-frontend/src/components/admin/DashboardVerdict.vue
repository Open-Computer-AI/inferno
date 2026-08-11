<template>
  <p class="verdict" :data-tone="tone">
    <i v-if="tone === 'attention'" class="verdict__icon hgi-stroke hgi-alert-circle" aria-hidden="true" />
    <span>{{ sentence }}</span>
  </p>
</template>

<script setup lang="ts">
/**
 * The one sentence at the top of a dashboard that answers the only question the
 * fold exists for: is anything wrong right now.
 *
 * Part 14 calls this out as a product decision rather than a design one -- what
 * counts as "needing attention" is a judgement, and the sentence is only as good
 * as the rule behind it. The rule we agreed:
 *
 *   COUNTS      auth expired or invalid, account disabled by error, channel down
 *   DOES NOT    cooldown (self-healing), rate limited (transient)
 *
 * What the dashboard payload actually exposes today is `error_accounts` -- the
 * count of accounts the gateway has taken out of service. That maps to
 * "disabled by error" exactly, and it does NOT include cooldown or rate-limited
 * accounts, so the rule holds with the data we have. When the payload gains a
 * per-reason breakdown, this is where it goes; nothing else has to change.
 *
 * Derived entirely from stats the page already loads, so it costs no endpoint.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  totalAccounts: number
  normalAccounts: number
  errorAccounts: number
}>()

const { t } = useI18n()

const tone = computed(() => (props.errorAccounts > 0 ? 'attention' : 'calm'))

const sentence = computed(() => {
  if (props.errorAccounts > 0) {
    return t('admin.dashboard.verdictAttention', {
      count: props.errorAccounts,
      serving: props.normalAccounts,
      total: props.totalAccounts
    })
  }
  // Nothing wrong still says what it is basing that on. "Everything is fine"
  // with no number behind it is a claim; with the count it is a reading.
  if (props.totalAccounts === 0) return t('admin.dashboard.verdictEmpty')
  return t('admin.dashboard.verdictHealthy', { serving: props.normalAccounts })
})
</script>

<style scoped>
/* Set in the serif at the display size: this is the page's headline finding,
   not a status chip. It reads before the numbers because it IS the answer, and
   the strip below is the evidence. */
.verdict {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: 0 0 16px;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-xl);
  font-weight: 400;
  line-height: 1.4;
}

/* Colour encodes state, and "nothing is wrong" is not a state worth colouring
   (ground rule 5). Only the attention case takes ink. */
.verdict[data-tone='attention'] {
  color: var(--s2a-attn);
}

.verdict__icon {
  /* Glyphs are fixed chrome and must not scale with --font-scale. */
  font-size: 16px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  line-height: 1;
}
</style>
