<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { fetchOne, getEntry } from '@/utils/ipGeoLookup'
import IconButton from '@/components/common/IconButton.vue'

/**
 * IpGeoCell — part 03, section 10 ("IP geo cell and version badge").
 *
 * Migration note from the prototype:
 *   "The source uses a dashed underlined teal link for idle and a dotted
 *    underlined grey one for success, two different affordances for the
 *    same target. One affordance, four states."
 *
 * One affordance means one underline style (dotted) for every state that is
 * a link to more detail — idle (look it up) and success (open the full
 * lookup) — coloured by state rather than switching decoration style. Error
 * gets the same dotted-link affordance in --destructive, since a failed
 * lookup is also one click from a retry. Loading and private are not links,
 * so neither is underlined. The IP address itself is rendered by the table
 * cell that hosts this component, in --font-mono, since it is a technical
 * identifier; this component only ever renders the resolved place name,
 * which is prose and stays --font-sans.
 */
const props = defineProps<{ ip: string }>()
const { t } = useI18n()

const entry = computed(() => getEntry(props.ip))

const tooltipText = computed(() => {
  const detail = entry.value.detail
  if (!detail) return ''
  const lines = [
    detail.organization ? `${t('usage.ipGeo.detailOrg')}: ${detail.organization}` : '',
    detail.timezone ? `${t('usage.ipGeo.detailTimezone')}: ${detail.timezone}` : '',
    detail.accuracy != null ? `${t('usage.ipGeo.detailAccuracy')}: ${detail.accuracy}km` : '',
    detail.latitude && detail.longitude
      ? `${t('usage.ipGeo.detailCoordinates')}: ${detail.latitude}, ${detail.longitude}`
      : '',
  ].filter(Boolean)
  return lines.join('\n')
})

const handleFetch = () => {
  void fetchOne(props.ip)
}

const handleRefresh = () => {
  void fetchOne(props.ip, true)
}

const handleOpenDetail = () => {
  window.open(
    `https://www.iplocation.net/ip-lookup?query=${encodeURIComponent(props.ip)}`,
    '_blank',
    'noopener,noreferrer'
  )
}
</script>

<template>
  <div class="geo">
    <button v-if="entry.status === 'idle'" type="button" class="geo__link" data-tone="idle" @click="handleFetch">
      <i class="hgi-stroke hgi-location-01 geo__icon" aria-hidden="true" />
      <span class="geo__text">{{ t('usage.ipGeo.fetch') }}</span>
    </button>

    <div v-else-if="entry.status === 'loading'" class="geo__row" data-tone="loading">
      <span class="geo__spinner s2a-spinner" aria-hidden="true" />
      <span class="geo__text">{{ t('usage.ipGeo.fetching') }}</span>
    </div>

    <div v-else-if="entry.status === 'success'" class="geo__row geo__row--success">
      <button
        type="button"
        class="geo__link geo__link--clip"
        data-tone="success"
        :title="tooltipText"
        @click="handleOpenDetail"
      >
        <i class="hgi-stroke hgi-location-01 geo__icon" aria-hidden="true" />
        <span class="geo__text">{{ entry.label }}</span>
      </button>
      <IconButton
        class="geo__refresh"
        icon="hgi-refresh"
        :label="t('usage.ipGeo.refreshTitle')"
        size="xs"
        @click="handleRefresh"
      />
    </div>

    <button
      v-else-if="entry.status === 'error'"
      type="button"
      class="geo__link"
      data-tone="error"
      @click="handleFetch"
    >
      <i class="hgi-stroke hgi-alert-circle geo__icon" aria-hidden="true" />
      <span class="geo__text">{{ t('usage.ipGeo.failed') }}</span>
    </button>

    <div v-else class="geo__row" data-tone="private">
      <i class="hgi-stroke hgi-remove-circle geo__icon" aria-hidden="true" />
      <span class="geo__text">{{ t('usage.ipGeo.private') }}</span>
    </div>
  </div>
</template>

<style scoped>
.geo {
  margin-top: 2px;
  min-width: 0;
}

/* Non-interactive rows: loading and private. Neither is a link, so neither
   is underlined. */
.geo__row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
.geo__row--success {
  justify-content: space-between;
}

/* The one affordance: dotted underline, coloured by state. Idle and success
   both use it because both are one click from more detail; error too,
   because a failed lookup is one click from a retry. */
.geo__link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
  font: inherit;
  font-size: var(--fs-sm);
  text-decoration: underline;
  text-decoration-style: dotted;
  text-underline-offset: 2px;
  cursor: pointer;
  /* Colour only, never border-color. */
  transition: color var(--motion-hover);
}
.geo__link--clip {
  flex: 1 1 auto;
}
.geo__link--clip .geo__text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.geo__link[data-tone='idle'] {
  color: var(--warm-strong);
}
.geo__link[data-tone='idle']:hover {
  color: var(--foreground);
}
.geo__link[data-tone='success'] {
  color: var(--body-copy);
}
.geo__link[data-tone='success']:hover {
  color: var(--foreground);
}
.geo__link[data-tone='error'] {
  color: var(--destructive);
}
.geo__link[data-tone='error']:hover {
  color: color-mix(in oklch, var(--destructive) 85%, var(--foreground));
}

.geo__icon {
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
  flex-shrink: 0;
}

.geo__spinner {
  display: inline-block;
  width: 11px;
  height: 11px;
  border: 1.5px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: s2a-spin 0.7s linear infinite;
}
</style>
