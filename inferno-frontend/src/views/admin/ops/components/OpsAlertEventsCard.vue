<script setup lang="ts">
/**
 * Alert events: what fired, how bad, and whether it is still happening.
 *
 * The conversion is mostly mechanical -- card chrome, table chrome, filter row,
 * scoped CSS instead of ~68 Tailwind utilities. Five decisions are not.
 *
 * COLOUR MARKS WHAT IS STILL OPEN, NOT WHAT RANK IT WAS
 * Before, a severity badge was painted from its severity alone, so a P0 that
 * resolved four days ago shouted exactly as loudly as a P0 firing right now.
 * On a page whose entire job is "is anything wrong", that is the wrong ink.
 * Severity is a CATEGORY (a property of the rule); whether it is firing is the
 * STATE. Ground rule 5 says colour encodes state. So a firing row keeps the
 * severity ramp -- destructive for P0, attention for P1, muted for P2, ghost
 * for P3 -- and every closed row drops to neutral chrome while keeping its
 * "P0" text. The red in this table is then exactly the set of open problems.
 *
 * ONE SOLID CHIP PER ROW
 * Severity and status were two filled pills side by side, both coloured, both
 * competing. Severity keeps the pill; status becomes a dot plus a label. Only
 * `firing` tints its label -- resolved gets a green dot but muted text, the
 * same asymmetry StatTile uses, so "fine" registers without demanding a look.
 *
 * DURATION DROPS ITS PREFIX
 * It read "Firing 34m" / "Resolved 38m", repeating in every row a word the
 * status cell two columns left already says. The prefix existed because the
 * number means different things (elapsed-so-far vs time-to-resolve) -- but the
 * status column supplies exactly that qualifier, and it is now adjacent. The
 * full phrase survives as the cell's title attribute, so nothing is lost for
 * anyone reading it out of context.
 *
 * EMAIL DELIVERY IS NOT A HEALTH STATE
 * "Sent" was a green tick on every row it applied to. Whether a notification
 * went out is a fact about plumbing, not a condition of the system, and 24
 * green ticks in a column say nothing at all. Both states are neutral now; the
 * glyph carries the difference.
 *
 * DIMENSIONS BECOME CHIPS
 * `platform=anthropic group_id=1 region=us-east` was one run of key=value soup
 * in a 11px grey. Same data, one chip each, key muted and value in mono, so
 * the eye can find `group_id` without reading the whole string.
 */
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Button from '@/components/common/Button.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { opsAPI, type AlertEventsQuery } from '@/api/admin/ops'
import type { AlertEvent } from '../types'
import { formatDateTime } from '../utils/opsFormatters'

const { t } = useI18n()
const appStore = useAppStore()

// 与 DataTable 一致：< 768px 切换为卡片视图，避免宽表在移动端被截断。
const isDesktopViewport = useMediaQuery('(min-width: 768px)')

const PAGE_SIZE = 10

const loading = ref(false)
const loadingMore = ref(false)
const events = ref<AlertEvent[]>([])
const hasMore = ref(true)
const scrollEl = ref<HTMLElement | null>(null)

// Detail modal
const showDetail = ref(false)
const selected = ref<AlertEvent | null>(null)
const detailLoading = ref(false)
const detailActionLoading = ref(false)
const historyLoading = ref(false)
const history = ref<AlertEvent[]>([])
const historyRange = ref('7d')
const historyRangeOptions = computed(() => [
  { value: '7d', label: t('admin.ops.timeRange.7d') },
  { value: '30d', label: t('admin.ops.timeRange.30d') }
])

const silenceDuration = ref('1h')
const silenceDurationOptions = computed(() => [
  { value: '1h', label: t('admin.ops.timeRange.1h') },
  { value: '24h', label: t('admin.ops.timeRange.24h') },
  { value: '7d', label: t('admin.ops.timeRange.7d') }
])

// Filters
const timeRange = ref('24h')
const timeRangeOptions = computed(() => [
  { value: '5m', label: t('admin.ops.timeRange.5m') },
  { value: '30m', label: t('admin.ops.timeRange.30m') },
  { value: '1h', label: t('admin.ops.timeRange.1h') },
  { value: '6h', label: t('admin.ops.timeRange.6h') },
  { value: '24h', label: t('admin.ops.timeRange.24h') },
  { value: '7d', label: t('admin.ops.timeRange.7d') },
  { value: '30d', label: t('admin.ops.timeRange.30d') }
])

/* The unset option names its own dimension rather than saying "All". Three
   adjacent selects all reading "All" is three controls that cannot be told
   apart without opening them. */
const severity = ref<string>('')
const severityOptions = computed(() => [
  { value: '', label: t('admin.ops.alertEvents.filters.anySeverity') },
  { value: 'P0', label: 'P0' },
  { value: 'P1', label: 'P1' },
  { value: 'P2', label: 'P2' },
  { value: 'P3', label: 'P3' }
])

const status = ref<string>('')
const statusOptions = computed(() => [
  { value: '', label: t('admin.ops.alertEvents.filters.anyStatus') },
  { value: 'firing', label: t('admin.ops.alertEvents.status.firing') },
  { value: 'resolved', label: t('admin.ops.alertEvents.status.resolved') },
  { value: 'manual_resolved', label: t('admin.ops.alertEvents.status.manualResolved') }
])

const emailSent = ref<string>('')
const emailSentOptions = computed(() => [
  { value: '', label: t('admin.ops.alertEvents.filters.anyEmail') },
  { value: 'true', label: t('admin.ops.alertEvents.table.emailSent') },
  { value: 'false', label: t('admin.ops.alertEvents.table.emailIgnored') }
])

function buildQuery(overrides: Partial<AlertEventsQuery> = {}): AlertEventsQuery {
  const q: AlertEventsQuery = {
    limit: PAGE_SIZE,
    time_range: timeRange.value
  }
  if (severity.value) q.severity = severity.value
  if (status.value) q.status = status.value
  if (emailSent.value === 'true') q.email_sent = true
  if (emailSent.value === 'false') q.email_sent = false
  return { ...q, ...overrides }
}

async function loadFirstPage() {
  loading.value = true
  try {
    const data = await opsAPI.listAlertEvents(buildQuery())
    events.value = data
    hasMore.value = data.length === PAGE_SIZE
  } catch (err: any) {
    console.error('[OpsAlertEventsCard] Failed to load alert events', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertEvents.loadFailed'))
    events.value = []
    hasMore.value = false
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (loadingMore.value || loading.value) return
  if (!hasMore.value) return
  const last = events.value[events.value.length - 1]
  if (!last) return

  loadingMore.value = true
  try {
    const data = await opsAPI.listAlertEvents(
      buildQuery({ before_fired_at: last.fired_at || last.created_at, before_id: last.id })
    )
    if (!data.length) {
      hasMore.value = false
      return
    }
    events.value = [...events.value, ...data]
    if (data.length < PAGE_SIZE) hasMore.value = false
  } catch (err: any) {
    console.error('[OpsAlertEventsCard] Failed to load more alert events', err)
    hasMore.value = false
  } finally {
    loadingMore.value = false
  }
}

function onScroll(e: Event) {
  const el = e.target as HTMLElement | null
  if (!el) return
  const nearBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 120
  if (nearBottom) loadMore()
}

/**
 * Keep loading until the list actually overflows its box.
 *
 * Infinite scroll has a dead end: if one page does not fill the container
 * there is nothing to scroll, so the scroll handler never fires and the rest
 * of the data is unreachable. Measured here at exactly that boundary -- ten
 * rows came to 588px inside a 600px box, so the eleventh event could not be
 * reached by any means. It is not hypothetical and it is not row-height
 * specific: rows without a description are shorter again.
 *
 * The guard caps this at five extra pages so a container that reports zero
 * height (hidden tab, collapsed parent) cannot spin the loop.
 */
async function refresh() {
  hasMore.value = true
  await loadFirstPage()
  await fillViewport()
}

async function fillViewport() {
  await nextTick()
  let guard = 0
  while (hasMore.value && guard < 5) {
    const el = scrollEl.value
    if (!el || el.clientHeight === 0) return
    if (el.scrollHeight > el.clientHeight) return
    guard++
    await loadMore()
    await nextTick()
  }
}

function getDimensionString(event: AlertEvent | null | undefined, key: string): string {
  const v = event?.dimensions?.[key]
  if (v == null) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  return ''
}

function formatDurationMs(ms: number): string {
  const safe = Math.max(0, Math.floor(ms))
  const sec = Math.floor(safe / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr}h`
  const day = Math.floor(hr / 24)
  return `${day}d`
}

/** The elapsed figure alone. What it measures is supplied by the status cell. */
function formatDurationValue(event: AlertEvent): string {
  const firedAt = new Date(event.fired_at || event.created_at)
  if (Number.isNaN(firedAt.getTime())) return '-'

  const resolvedAtStr = event.resolved_at || null
  if (resolvedAtStr) {
    const resolvedAt = new Date(resolvedAtStr)
    if (!Number.isNaN(resolvedAt.getTime())) {
      return formatDurationMs(resolvedAt.getTime() - firedAt.getTime())
    }
  }
  return formatDurationMs(Date.now() - firedAt.getTime())
}

/** Status word plus the figure, for the cell's title -- the phrase still has to
 *  stand alone for a screen reader or a hover, away from the status column. */
function formatDurationLabel(event: AlertEvent): string {
  const value = formatDurationValue(event)
  if (value === '-') return '-'
  return `${formatStatusLabel(event.status)} ${value}`
}

/** One chip per dimension rather than one run of key=value text. */
function dimensionChips(event: AlertEvent): { key: string; value: string }[] {
  const out: { key: string; value: string }[] = []
  const platform = getDimensionString(event, 'platform')
  if (platform) out.push({ key: 'platform', value: platform })
  const groupId = event.dimensions?.group_id
  if (groupId != null && groupId !== '') out.push({ key: 'group_id', value: String(groupId) })
  const region = getDimensionString(event, 'region')
  if (region) out.push({ key: 'region', value: region })
  return out
}

/** Two decimals at most, and none when the value is whole: a latency threshold
 *  of 8000 should not render as "8000.00". */
function formatMetricValue(v: number | undefined | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return String(Math.round(v * 100) / 100)
}

function formatMetricPair(event: AlertEvent | null): string {
  if (!event) return '-'
  if (typeof event.metric_value !== 'number' || typeof event.threshold_value !== 'number') return '-'
  return `${formatMetricValue(event.metric_value)} / ${formatMetricValue(event.threshold_value)}`
}

function closeDetail() {
  showDetail.value = false
  selected.value = null
  history.value = []
}

async function openDetail(row: AlertEvent) {
  showDetail.value = true
  selected.value = row
  detailLoading.value = true
  historyLoading.value = true

  try {
    const detail = await opsAPI.getAlertEvent(row.id)
    selected.value = detail
  } catch (err: any) {
    console.error('[OpsAlertEventsCard] Failed to load alert detail', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertEvents.detail.loadFailed'))
  } finally {
    detailLoading.value = false
  }

  await loadHistory()
}

async function loadHistory() {
  const ev = selected.value
  if (!ev) {
    history.value = []
    historyLoading.value = false
    return
  }

  historyLoading.value = true
  try {
    const platform = getDimensionString(ev, 'platform')
    const groupIdRaw = ev.dimensions?.group_id
    const groupId = typeof groupIdRaw === 'number' ? groupIdRaw : undefined

    const items = await opsAPI.listAlertEvents({
      limit: 20,
      time_range: historyRange.value,
      platform: platform || undefined,
      group_id: groupId,
      status: ''
    })

    // Best-effort: narrow to same rule_id + dimensions
    history.value = items.filter((it) => {
      if (it.rule_id !== ev.rule_id) return false
      const p1 = getDimensionString(it, 'platform')
      const p2 = getDimensionString(ev, 'platform')
      if ((p1 || '') !== (p2 || '')) return false
      const g1 = it.dimensions?.group_id
      const g2 = ev.dimensions?.group_id
      return (g1 ?? null) === (g2 ?? null)
    })
  } catch (err: any) {
    console.error('[OpsAlertEventsCard] Failed to load alert history', err)
    history.value = []
  } finally {
    historyLoading.value = false
  }
}

function durationToUntilRFC3339(duration: string): string {
  const now = Date.now()
  if (duration === '1h') return new Date(now + 60 * 60 * 1000).toISOString()
  if (duration === '24h') return new Date(now + 24 * 60 * 60 * 1000).toISOString()
  if (duration === '7d') return new Date(now + 7 * 24 * 60 * 60 * 1000).toISOString()
  return new Date(now + 60 * 60 * 1000).toISOString()
}

async function silenceAlert() {
  const ev = selected.value
  if (!ev) return
  if (detailActionLoading.value) return
  detailActionLoading.value = true
  try {
    const platform = getDimensionString(ev, 'platform')
    const groupIdRaw = ev.dimensions?.group_id
    const groupId = typeof groupIdRaw === 'number' ? groupIdRaw : null
    const region = getDimensionString(ev, 'region') || null

    await opsAPI.createAlertSilence({
      rule_id: ev.rule_id,
      platform: platform || '',
      group_id: groupId ?? undefined,
      region: region ?? undefined,
      until: durationToUntilRFC3339(silenceDuration.value),
      reason: `silence from UI (${silenceDuration.value})`
    })

    appStore.showSuccess(t('admin.ops.alertEvents.detail.silenceSuccess'))
  } catch (err: any) {
    console.error('[OpsAlertEventsCard] Failed to silence alert', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertEvents.detail.silenceFailed'))
  } finally {
    detailActionLoading.value = false
  }
}

async function manualResolve() {
  if (!selected.value) return
  if (detailActionLoading.value) return
  detailActionLoading.value = true
  try {
    await opsAPI.updateAlertEventStatus(selected.value.id, 'manual_resolved')
    appStore.showSuccess(t('admin.ops.alertEvents.detail.manualResolvedSuccess'))

    // Refresh detail + first page to reflect new status
    const detail = await opsAPI.getAlertEvent(selected.value.id)
    selected.value = detail
    await loadFirstPage()
    await loadHistory()
  } catch (err: any) {
    console.error('[OpsAlertEventsCard] Failed to resolve alert', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertEvents.detail.manualResolvedFailed'))
  } finally {
    detailActionLoading.value = false
  }
}

onMounted(async () => {
  await loadFirstPage()
  await fillViewport()
})

watch([timeRange, severity, status, emailSent], async () => {
  events.value = []
  hasMore.value = true
  await loadFirstPage()
  await fillViewport()
})

watch(historyRange, () => {
  if (showDetail.value) loadHistory()
})

type SeverityTone = 'p0' | 'p1' | 'p2' | 'p3'
type StatusTone = 'firing' | 'resolved' | 'manual' | 'unknown'

/* P0..P3 is an ordinal, so it gets an ordinal ramp rather than four unrelated
   hues. The legacy `critical`/`warning`/`info` aliases are kept because the
   backend emits both spellings depending on which rule wrote the event. */
function severityTone(severity: string | undefined): SeverityTone {
  const s = String(severity || '').trim().toLowerCase()
  if (s === 'p0' || s === 'critical') return 'p0'
  if (s === 'p1' || s === 'warning') return 'p1'
  if (s === 'p2' || s === 'info') return 'p2'
  return 'p3'
}

/** Open = still firing. This, not severity, is what earns colour. */
function isOpen(status: string | undefined): boolean {
  return String(status || '').trim().toLowerCase() === 'firing'
}

function statusTone(status: string | undefined): StatusTone {
  const s = String(status || '').trim().toLowerCase()
  if (s === 'firing') return 'firing'
  if (s === 'resolved') return 'resolved'
  if (s === 'manual_resolved') return 'manual'
  return 'unknown'
}

/* The fallback used to be `s.toUpperCase()`, which shouted an unrecognised
   status back at the reader (ground rule 1). An unknown value is shown as it
   arrived. */
function formatStatusLabel(status: string | undefined): string {
  const s = String(status || '').trim().toLowerCase()
  if (!s) return '-'
  if (s === 'firing') return t('admin.ops.alertEvents.status.firing')
  if (s === 'resolved') return t('admin.ops.alertEvents.status.resolved')
  if (s === 'manual_resolved') return t('admin.ops.alertEvents.status.manualResolved')
  return s
}

const empty = computed(() => events.value.length === 0 && !loading.value)

/* NO "N FIRING" BADGE NEXT TO THE TITLE.
 * It is the obvious thing to add and it would be a lie. The list pages ten at
 * a time ordered by fired_at, and an alert that is still firing can easily be
 * older than the tenth row -- so a count taken from `events` is a count of
 * what happens to be loaded, presented as a total. There is no count endpoint
 * to back the real number. A true "open alerts" figure belongs in the header
 * stat row with a query behind it, not inferred from a page of rows. */
</script>

<template>
  <div class="alerts">
    <div class="alerts__head">
      <div class="alerts__heading">
        <h3 class="alerts__title">
          <i class="hgi-stroke hgi-alert-02 alerts__title-icon" aria-hidden="true" />
          {{ t('admin.ops.alertEvents.title') }}
        </h3>
        <p class="alerts__subtitle">{{ t('admin.ops.alertEvents.description') }}</p>
      </div>

      <div class="alerts__filters">
        <Select
          :model-value="timeRange"
          :options="timeRangeOptions"
          class="alerts__filter alerts__filter--wide"
          @change="timeRange = String($event || '24h')"
        />
        <Select
          :model-value="severity"
          :options="severityOptions"
          class="alerts__filter"
          @change="severity = String($event || '')"
        />
        <Select
          :model-value="status"
          :options="statusOptions"
          class="alerts__filter alerts__filter--status"
          @change="status = String($event || '')"
        />
        <Select
          :model-value="emailSent"
          :options="emailSentOptions"
          class="alerts__filter"
          @change="emailSent = String($event || '')"
        />
        <button
          type="button"
          class="alerts__refresh"
          :disabled="loading"
          :title="t('common.refresh')"
          @click="refresh"
        >
          <i
            class="hgi-stroke hgi-refresh alerts__refresh-icon"
            :class="loading ? 's2a-spinner alerts__refresh-icon--busy' : ''"
            aria-hidden="true"
          />
          {{ t('common.refresh') }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="alerts__state">
      <LoadingSpinner size="md" />
    </div>

    <div v-else-if="empty" class="alerts__state">
      <EmptyState
        icon="hgi-alert-02"
        :title="t('admin.ops.alertEvents.empty')"
        :description="t('admin.ops.alertEvents.emptyHint')"
      />
    </div>

    <div v-else class="alerts__frame">
      <div ref="scrollEl" class="alerts__scroll" @scroll="onScroll">
        <!-- Card view, under 768px -->
        <div v-if="!isDesktopViewport" class="alerts__cards">
          <div
            v-for="row in events"
            :key="row.id"
            class="alerts__card"
            :data-open="isOpen(row.status) || undefined"
            @click="openDetail(row)"
          >
            <div class="alerts__card-top">
              <span class="alerts__sev" :data-tone="severityTone(row.severity)" :data-open="isOpen(row.status) || undefined">
                {{ row.severity || '-' }}
              </span>
              <span class="alerts__status" :data-tone="statusTone(row.status)">
                <span class="alerts__status-dot" aria-hidden="true" />
                {{ formatStatusLabel(row.status) }}
              </span>
              <span class="alerts__time alerts__time--push">
                {{ formatDateTime(row.fired_at || row.created_at) }}
              </span>
            </div>

            <p class="alerts__card-title">{{ row.title || '-' }}</p>
            <p v-if="row.description" class="alerts__desc">{{ row.description }}</p>

            <div class="alerts__card-foot">
              <span class="alerts__rule">#{{ row.rule_id }}</span>
              <span class="alerts__dot" aria-hidden="true" />
              <span
                class="alerts__duration"
                :title="formatDurationLabel(row)"
              >{{ formatDurationValue(row) }}</span>
              <span class="alerts__email alerts__email--push">
                <i
                  class="hgi-stroke alerts__email-icon"
                  :class="row.email_sent ? 'hgi-tick-02' : 'hgi-remove-circle'"
                  aria-hidden="true"
                />
                {{ row.email_sent ? t('admin.ops.alertEvents.table.emailSent') : t('admin.ops.alertEvents.table.emailIgnored') }}
              </span>
            </div>

            <div v-if="dimensionChips(row).length" class="alerts__dims">
              <span v-for="d in dimensionChips(row)" :key="d.key" class="alerts__dim">
                <span class="alerts__dim-key">{{ d.key }}</span>
                <span class="alerts__dim-value">{{ d.value }}</span>
              </span>
            </div>
          </div>
        </div>

        <!-- Table view, 768px and up -->
        <table v-else class="alerts__table">
          <thead class="alerts__thead">
            <tr>
              <th class="alerts__th">{{ t('admin.ops.alertEvents.table.time') }}</th>
              <th class="alerts__th">{{ t('admin.ops.alertEvents.table.severity') }}</th>
              <th class="alerts__th">{{ t('admin.ops.alertEvents.table.platform') }}</th>
              <th class="alerts__th">{{ t('admin.ops.alertEvents.table.ruleId') }}</th>
              <th class="alerts__th alerts__th--title">{{ t('admin.ops.alertEvents.table.title') }}</th>
              <th class="alerts__th">{{ t('admin.ops.alertEvents.table.duration') }}</th>
              <th class="alerts__th">{{ t('admin.ops.alertEvents.table.dimensions') }}</th>
              <th class="alerts__th alerts__th--end">{{ t('admin.ops.alertEvents.table.email') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="row in events"
              :key="row.id"
              class="alerts__row"
              :data-open="isOpen(row.status) || undefined"
              :title="row.title || ''"
              @click="openDetail(row)"
            >
              <td class="alerts__td alerts__td--time">
                {{ formatDateTime(row.fired_at || row.created_at) }}
              </td>
              <td class="alerts__td">
                <div class="alerts__badges">
                  <span class="alerts__sev" :data-tone="severityTone(row.severity)" :data-open="isOpen(row.status) || undefined">
                    {{ row.severity || '-' }}
                  </span>
                  <span class="alerts__status" :data-tone="statusTone(row.status)">
                    <span class="alerts__status-dot" aria-hidden="true" />
                    {{ formatStatusLabel(row.status) }}
                  </span>
                </div>
              </td>
              <td class="alerts__td alerts__td--muted">
                {{ getDimensionString(row, 'platform') || '-' }}
              </td>
              <td class="alerts__td">
                <span class="alerts__rule">#{{ row.rule_id }}</span>
              </td>
              <td class="alerts__td alerts__td--title">
                <p class="alerts__row-title">{{ row.title || '-' }}</p>
                <p v-if="row.description" class="alerts__desc">{{ row.description }}</p>
              </td>
              <td class="alerts__td" :title="formatDurationLabel(row)">
                <span class="alerts__duration">{{ formatDurationValue(row) }}</span>
              </td>
              <td class="alerts__td">
                <div v-if="dimensionChips(row).length" class="alerts__dims">
                  <span v-for="d in dimensionChips(row)" :key="d.key" class="alerts__dim">
                    <span class="alerts__dim-key">{{ d.key }}</span>
                    <span class="alerts__dim-value">{{ d.value }}</span>
                  </span>
                </div>
                <span v-else class="alerts__td-empty">-</span>
              </td>
              <td class="alerts__td alerts__td--end">
                <span
                  class="alerts__email"
                  :title="row.email_sent ? t('admin.ops.alertEvents.table.emailSent') : t('admin.ops.alertEvents.table.emailIgnored')"
                >
                  <i
                    class="hgi-stroke alerts__email-icon"
                    :class="row.email_sent ? 'hgi-tick-02' : 'hgi-remove-circle'"
                    aria-hidden="true"
                  />
                  {{ row.email_sent ? t('admin.ops.alertEvents.table.emailSent') : t('admin.ops.alertEvents.table.emailIgnored') }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>

        <div v-if="loadingMore" class="alerts__more">
          <LoadingSpinner size="sm" />
        </div>
        <div v-else-if="!hasMore && events.length > 0" class="alerts__more">
          {{ t('admin.ops.alertEvents.endOfList') }}
        </div>
      </div>
    </div>

    <BaseDialog
      :show="showDetail"
      :title="t('admin.ops.alertEvents.detail.title')"
      width="wide"
      :close-on-click-outside="true"
      @close="closeDetail"
    >
      <div v-if="detailLoading" class="alerts__dialog-state">
        <LoadingSpinner size="md" />
      </div>

      <div v-else-if="!selected" class="alerts__dialog-state">
        {{ t('admin.ops.alertEvents.detail.empty') }}
      </div>

      <div v-else class="alerts__detail">
        <div class="alerts__summary">
          <div class="alerts__summary-main">
            <div class="alerts__badges">
              <span
                class="alerts__sev"
                :data-tone="severityTone(selected.severity)"
                :data-open="isOpen(selected.status) || undefined"
              >
                {{ selected.severity || '-' }}
              </span>
              <span class="alerts__status" :data-tone="statusTone(selected.status)">
                <span class="alerts__status-dot" aria-hidden="true" />
                {{ formatStatusLabel(selected.status) }}
              </span>
              <span class="alerts__time" :title="formatDurationLabel(selected)">{{ formatDurationValue(selected) }}</span>
            </div>
            <p class="alerts__detail-title">{{ selected.title || '-' }}</p>
            <p v-if="selected.description" class="alerts__detail-desc">{{ selected.description }}</p>
          </div>

          <div class="alerts__actions">
            <div class="alerts__silence">
              <span class="alerts__silence-label">{{ t('admin.ops.alertEvents.detail.silence') }}</span>
              <Select
                :model-value="silenceDuration"
                :options="silenceDurationOptions"
                class="alerts__filter"
                @change="silenceDuration = String($event || '1h')"
              />
              <Button
                variant="secondary"
                size="sm"
                icon="hgi-cancel-01"
                :disabled="detailActionLoading"
                @click="silenceAlert"
              >
                {{ t('common.apply') }}
              </Button>
            </div>

            <Button
              variant="secondary"
              size="sm"
              icon="hgi-tick-02"
              :disabled="detailActionLoading"
              @click="manualResolve"
            >
              {{ t('admin.ops.alertEvents.detail.manualResolve') }}
            </Button>
          </div>
        </div>

        <dl class="alerts__facts">
          <div class="alerts__fact">
            <dt class="alerts__fact-label">{{ t('admin.ops.alertEvents.detail.firedAt') }}</dt>
            <dd class="alerts__fact-value">{{ formatDateTime(selected.fired_at || selected.created_at) }}</dd>
          </div>
          <div class="alerts__fact">
            <dt class="alerts__fact-label">{{ t('admin.ops.alertEvents.detail.resolvedAt') }}</dt>
            <dd class="alerts__fact-value">{{ selected.resolved_at ? formatDateTime(selected.resolved_at) : '-' }}</dd>
          </div>
          <div class="alerts__fact">
            <dt class="alerts__fact-label">{{ t('admin.ops.alertEvents.detail.measured') }}</dt>
            <dd class="alerts__fact-value alerts__fact-value--mono">{{ formatMetricPair(selected) }}</dd>
          </div>
          <div class="alerts__fact">
            <dt class="alerts__fact-label">{{ t('admin.ops.alertEvents.detail.ruleId') }}</dt>
            <dd class="alerts__fact-value">
              <div class="alerts__rule-row">
                <span class="alerts__rule">#{{ selected.rule_id }}</span>
                <a
                  class="alerts__link"
                  :href="`/admin/ops?open_alert_rules=1&alert_rule_id=${selected.rule_id}`"
                >
                  <i class="hgi-stroke hgi-link-01 alerts__link-icon" aria-hidden="true" />
                  {{ t('admin.ops.alertEvents.detail.viewRule') }}
                </a>
                <a
                  class="alerts__link"
                  :href="`/admin/ops?platform=${encodeURIComponent(getDimensionString(selected, 'platform') || '')}&group_id=${selected.dimensions?.group_id || ''}&error_type=request&open_error_details=1`"
                >
                  <i class="hgi-stroke hgi-link-01 alerts__link-icon" aria-hidden="true" />
                  {{ t('admin.ops.alertEvents.detail.viewLogs') }}
                </a>
              </div>
            </dd>
          </div>
          <div class="alerts__fact alerts__fact--wide">
            <dt class="alerts__fact-label">{{ t('admin.ops.alertEvents.detail.dimensions') }}</dt>
            <dd class="alerts__fact-value">
              <div v-if="dimensionChips(selected).length" class="alerts__dims">
                <span v-for="d in dimensionChips(selected)" :key="d.key" class="alerts__dim">
                  <span class="alerts__dim-key">{{ d.key }}</span>
                  <span class="alerts__dim-value">{{ d.value }}</span>
                </span>
              </div>
              <span v-else class="alerts__td-empty">-</span>
            </dd>
          </div>
        </dl>

        <div class="alerts__history">
          <div class="alerts__history-head">
            <div>
              <p class="alerts__history-title">{{ t('admin.ops.alertEvents.detail.historyTitle') }}</p>
              <p class="alerts__history-hint">{{ t('admin.ops.alertEvents.detail.historyHint') }}</p>
            </div>
            <Select
              :model-value="historyRange"
              :options="historyRangeOptions"
              class="alerts__filter alerts__filter--wide"
              @change="historyRange = String($event || '7d')"
            />
          </div>

          <div v-if="historyLoading" class="alerts__history-state">
            <LoadingSpinner size="sm" />
          </div>
          <div v-else-if="history.length === 0" class="alerts__history-state">
            {{ t('admin.ops.alertEvents.detail.historyEmpty') }}
          </div>
          <div v-else class="alerts__history-frame">
            <table class="alerts__table alerts__table--compact">
              <thead class="alerts__thead">
                <tr>
                  <th class="alerts__th">{{ t('admin.ops.alertEvents.table.time') }}</th>
                  <th class="alerts__th">{{ t('admin.ops.alertEvents.table.status') }}</th>
                  <th class="alerts__th alerts__th--end">{{ t('admin.ops.alertEvents.table.metric') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="it in history" :key="it.id" class="alerts__row" :data-open="isOpen(it.status) || undefined">
                  <td class="alerts__td alerts__td--time">{{ formatDateTime(it.fired_at || it.created_at) }}</td>
                  <td class="alerts__td">
                    <span class="alerts__status" :data-tone="statusTone(it.status)">
                      <span class="alerts__status-dot" aria-hidden="true" />
                      {{ formatStatusLabel(it.status) }}
                    </span>
                  </td>
                  <td class="alerts__td alerts__td--end alerts__td--mono">
                    {{ formatMetricPair(it) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </BaseDialog>
  </div>
</template>

<style scoped>
/* Card chrome matches every other ops panel: hairline, --r-lg, no shadow
   (ground rule 9). The old one was rounded-3xl with a shadow and a ring. */
.alerts {
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.alerts__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.alerts__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.alerts__title-icon {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.alerts__subtitle {
  margin: 3px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.alerts__filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

/* Widths land on the child Select's root, which carries this component's scope
   id as well as its own. Each is sized to its LONGEST option, not its current
   one, so choosing a value never reflows the toolbar. */
.alerts__filter {
  width: 118px;
}

.alerts__filter--wide {
  width: 132px;
}

/* "Manually resolved" is the long one here. */
.alerts__filter--status {
  width: 152px;
}

.alerts__refresh {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: var(--s2a-h-sm);
  padding: 0 10px;
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  background: var(--card);
  color: var(--muted-foreground);
  font-family: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: background var(--motion-hover);
}

.alerts__refresh:hover:not(:disabled) {
  background: var(--sidebar-accent);
}

.alerts__refresh:disabled {
  opacity: 0.6;
  cursor: default;
}

.alerts__refresh-icon {
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  line-height: 1;
}

/* The `s2a-spinner` class on the same element is what buys the reduced-motion
   guard in inferno.css; this only supplies the animation. */
.alerts__refresh-icon--busy {
  animation: s2a-spin 0.7s linear infinite;
}

.alerts__state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 0;
}

.alerts__frame {
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
}

.alerts__scroll {
  max-height: 600px;
  overflow: auto;
  scrollbar-width: thin;
  scrollbar-color: var(--border) transparent;
}

.alerts__scroll::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

.alerts__scroll::-webkit-scrollbar-track {
  background: transparent;
}

.alerts__scroll::-webkit-scrollbar-thumb {
  background-color: var(--border);
  border-radius: 3px;
}

.alerts__scroll::-webkit-scrollbar-thumb:hover {
  background-color: var(--muted-foreground);
}

/* --- table ------------------------------------------------------------ */

.alerts__table {
  width: 100%;
  min-width: max-content;
  border-collapse: collapse;
}

.alerts__thead {
  position: sticky;
  top: 0;
  z-index: 2;
  background: var(--card);
}

/* Sentence case at --fs-xs, weight 400. The old header was
   `text-[11px] font-bold uppercase tracking-wider`, which broke three ground
   rules in one class list. */
.alerts__th {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
  font-weight: 400;
  text-align: left;
  white-space: nowrap;
}

.alerts__th--end {
  text-align: right;
}

.alerts__th--title {
  min-width: 260px;
}

.alerts__row {
  background: var(--card);
  cursor: pointer;
  /* Background only, never border-color (ground rule 6). */
  transition: background var(--motion-hover);
}

.alerts__row:hover {
  background: var(--sidebar-accent);
}

.alerts__row + .alerts__row .alerts__td {
  border-top: 1px solid var(--border-subtle);
}

.alerts__td {
  padding: 9px 12px;
  color: var(--body-copy);
  font-size: var(--fs-sm);
  vertical-align: middle;
  white-space: nowrap;
}

.alerts__td--muted {
  color: var(--muted-foreground);
}

.alerts__td--end {
  text-align: right;
}

.alerts__td--time {
  color: var(--muted-foreground);
  font-variant-numeric: tabular-nums;
}

.alerts__td--mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}

.alerts__td--title {
  max-width: 380px;
  white-space: normal;
}

.alerts__td-empty {
  color: var(--muted-foreground);
}

.alerts__row-title {
  margin: 0;
  max-width: 360px;
  overflow: hidden;
  color: var(--foreground);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* One line, not two. The table's job is to be scanned; the full text is one
   click away in the detail dialog, and a two-line clamp put roughly six rows
   in a 600px viewport. */
.alerts__desc {
  margin: 2px 0 0;
  max-width: 360px;
  overflow: hidden;
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.alerts__duration {
  font-variant-numeric: tabular-nums;
}

/* --- severity + status ------------------------------------------------ */

.alerts__badges {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

/* A closed row has NO chip at all -- bare muted text, keeping the pill's
   padding so the column still aligns. The chip's PRESENCE is what says "still
   open" and its fill says how bad; measured against the live page, an open P2
   fill and a closed fill sat 0.02 apart in lightness, which is not a
   distinction anyone can see. Presence is unmissable. */
.alerts__sev {
  flex: none;
  padding: 1px 8px;
  border-radius: var(--r-pill);
  background: transparent;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
  font-variant-numeric: tabular-nums;
}

.alerts__sev[data-open][data-tone='p0'] {
  background: color-mix(in oklch, var(--destructive) 15%, var(--card));
  color: var(--destructive);
}

.alerts__sev[data-open][data-tone='p1'] {
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
}

.alerts__sev[data-open][data-tone='p2'] {
  background: var(--muted);
  color: var(--body-copy);
}

/* P3 open gets the faintest fill there is. The ramp stays monotonic in chroma
   -- red, amber, then two neutrals -- because chroma is what the eye picks out
   of a table that is otherwise grey, not luminance contrast. */
.alerts__sev[data-open][data-tone='p3'] {
  background: var(--s2a-ghost);
  color: var(--muted-foreground);
}

.alerts__status {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 5px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  white-space: nowrap;
}

.alerts__status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--muted-foreground);
}

/* Asymmetric on purpose: firing tints the whole label, resolved tints only the
   dot. "Fine" should register without asking for a look. */
.alerts__status[data-tone='firing'] {
  color: var(--destructive);
}

.alerts__status[data-tone='firing'] .alerts__status-dot {
  background: var(--destructive);
}

.alerts__status[data-tone='resolved'] .alerts__status-dot {
  background: var(--success);
}

/* Manually resolved keeps a neutral dot rather than the success green: a human
   closed the ticket, which is not the same claim as the condition clearing. */
.alerts__status[data-tone='manual'] .alerts__status-dot {
  background: var(--muted-foreground);
}

/* --- rule id, dimensions, email --------------------------------------- */

.alerts__rule {
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
  font-variant-numeric: tabular-nums;
}

.alerts__dims {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}

.alerts__dim {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  padding: 1px 6px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  font-size: var(--fs-2xs);
  white-space: nowrap;
}

.alerts__dim-key {
  color: var(--muted-foreground);
}

.alerts__dim-value {
  color: var(--foreground);
  font-family: var(--font-mono);
}

/* Both delivery states are neutral. Whether an email went out is plumbing, not
   a condition of the system, and a green tick on every row says nothing. */
.alerts__email {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 5px;
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
  white-space: nowrap;
}

.alerts__email-icon {
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  line-height: 1;
}

.alerts__more {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 10px 0;
  border-top: 1px solid var(--border-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

/* --- card view (under 768px) ------------------------------------------ */

.alerts__cards {
  display: flex;
  flex-direction: column;
}

.alerts__card {
  padding: 12px;
  background: var(--card);
  cursor: pointer;
  transition: background var(--motion-hover);
}

.alerts__card:hover {
  background: var(--sidebar-accent);
}

.alerts__card + .alerts__card {
  border-top: 1px solid var(--border-subtle);
}

.alerts__card-top {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.alerts__time {
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
}

.alerts__time--push,
.alerts__email--push {
  margin-left: auto;
}

.alerts__card-title {
  margin: 8px 0 0;
  color: var(--foreground);
  font-size: var(--fs-md);
}

.alerts__card-foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

.alerts__dot {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: var(--border);
}

.alerts__cards .alerts__dims {
  margin-top: 8px;
}

.alerts__cards .alerts__desc {
  max-width: none;
  white-space: normal;
}

/* --- detail dialog ---------------------------------------------------- */

.alerts__dialog-state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 0;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}

.alerts__detail {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.alerts__summary {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 14px;
  border-radius: var(--r-md);
  background: var(--surface-subtle);
}

.alerts__summary-main {
  min-width: 0;
  flex: 1 1 320px;
}

.alerts__detail-title {
  margin: 8px 0 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.alerts__detail-desc {
  margin: 4px 0 0;
  color: var(--body-copy);
  font-size: var(--fs-md);
  white-space: pre-wrap;
}

.alerts__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.alerts__silence {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 6px 5px 10px;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--card);
}

.alerts__silence-label {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  white-space: nowrap;
}

/* Two fixed columns, not auto-fit. auto-fit gave three at this dialog width,
   which left the fourth panel sitting alone beside two columns of nothing. Two
   divides the five panels evenly: 2 + 2 + one full-width row. */
.alerts__facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin: 0;
}

@media (max-width: 560px) {
  .alerts__facts {
    grid-template-columns: minmax(0, 1fr);
  }
}

.alerts__fact {
  min-width: 0;
  padding: 10px 12px;
  border-radius: var(--r-md);
  background: var(--surface-subtle);
}

.alerts__fact--wide {
  grid-column: 1 / -1;
}

/* Sentence case at --fs-xs. These were `text-xs font-bold uppercase
   tracking-wider text-gray-400`. */
.alerts__fact-label {
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

.alerts__fact-value {
  margin: 4px 0 0;
  color: var(--foreground);
  font-size: var(--fs-md);
}

.alerts__fact-value--mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}

.alerts__rule-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.alerts__link {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 8px;
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  background: var(--card);
  color: var(--body-copy);
  font-size: var(--fs-sm);
  text-decoration: none;
  transition: background var(--motion-hover);
}

.alerts__link:hover {
  background: var(--sidebar-accent);
}

.alerts__link-icon {
  font-size: 12px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.alerts__history {
  padding: 14px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
}

.alerts__history-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 10px;
}

.alerts__history-title {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
}

.alerts__history-hint {
  margin: 2px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.alerts__history-state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.alerts__history-frame {
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-sm);
}

.alerts__table--compact .alerts__th {
  padding: 6px 10px;
}

.alerts__table--compact .alerts__td {
  padding: 6px 10px;
}

.alerts__table--compact .alerts__row {
  cursor: default;
}
</style>
