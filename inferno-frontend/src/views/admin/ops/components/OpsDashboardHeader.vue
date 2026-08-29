<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'
import Input from '@/components/common/Input.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import OpsResourceMeters from './OpsResourceMeters.vue'
import OpsTrafficSplit from './OpsTrafficSplit.vue'
import OpsStatCard from './OpsStatCard.vue'
import OpsHealthRing from './OpsHealthRing.vue'
import OpsRealtimeTraffic from './OpsRealtimeTraffic.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api'
import { opsAPI, type OpsDashboardOverview, type OpsMetricThresholds, type OpsRealtimeTrafficSummary } from '@/api/admin/ops'
import type { OpsRequestDetailsPreset } from './OpsRequestDetailsModal.vue'
import { useAdminSettingsStore } from '@/stores'
import { formatNumber } from '@/utils/format'
import { formatDateTime } from '../utils/opsFormatters'

type RealtimeWindow = '1min' | '5min' | '30min' | '1h'

interface Props {
  overview?: OpsDashboardOverview | null
  platform: string
  groupId: number | null
  timeRange: string
  queryMode: string
  loading: boolean
  lastUpdated: Date | null
  thresholds?: OpsMetricThresholds | null // 阈值配置
  autoRefreshEnabled?: boolean
  autoRefreshCountdown?: number
  fullscreen?: boolean
  customStartTime?: string | null
  customEndTime?: string | null
}

interface Emits {
  (e: 'update:platform', value: string): void
  (e: 'update:group', value: number | null): void
  (e: 'update:timeRange', value: string): void
  (e: 'update:queryMode', value: string): void
  (e: 'update:customTimeRange', startTime: string, endTime: string): void
  (e: 'refresh'): void
  (e: 'openRequestDetails', preset?: OpsRequestDetailsPreset): void
  (e: 'openErrorDetails', kind: 'request' | 'upstream'): void
  (e: 'openSettings'): void
  (e: 'openAlertRules'): void
  (e: 'enterFullscreen'): void
  (e: 'exitFullscreen'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const adminSettingsStore = useAdminSettingsStore()

const realtimeWindow = ref<RealtimeWindow>('1min')
const showDiagnosis = ref(false)

/**
 * The refresh timestamp used to be hardcoded to zh-CN.
 *
 * `toLocaleString('zh-CN', ...)` with the slashes swapped for hyphens: an
 * English user got a Chinese-locale date, every time, on the most-read line of
 * the page. It now uses the ops formatter that the alert and log tables use,
 * so all three clocks on this screen agree.
 */
const lastUpdatedLabel = computed(() =>
  props.lastUpdated ? formatDateTime(props.lastUpdated.toISOString()) : t('common.unknown')
)

/* Three severities, three glyphs. The old markup drew each as a hand-inlined
   20x20 SVG with a Tailwind colour, including a blue "info" that this palette
   does not contain. */
function diagnosisIcon(type: 'critical' | 'warning' | 'info'): string {
  if (type === 'critical') return 'hgi-cancel-circle'
  if (type === 'warning') return 'hgi-alert-02'
  return 'hgi-information-circle'
}


const overview = computed(() => props.overview ?? null)
const systemMetrics = computed(() => overview.value?.system_metrics ?? null)

const REALTIME_WINDOW_MINUTES: Record<RealtimeWindow, number> = {
  '1min': 1,
  '5min': 5,
  '30min': 30,
  '1h': 60
}

const TOOLBAR_RANGE_MINUTES: Record<string, number> = {
  '5m': 5,
  '30m': 30,
  '1h': 60,
  '6h': 6 * 60,
  '24h': 24 * 60
}

const availableRealtimeWindows = computed(() => {
  const toolbarMinutes = TOOLBAR_RANGE_MINUTES[props.timeRange] ?? 60
  return (['1min', '5min', '30min', '1h'] as const).filter((w) => REALTIME_WINDOW_MINUTES[w] <= toolbarMinutes)
})

watch(
  () => props.timeRange,
  () => {
    // The realtime window must be inside the toolbar window; reset to keep UX predictable.
    realtimeWindow.value = '1min'
    // Keep realtime traffic consistent with toolbar changes even when the window is already 1min.
    loadRealtimeTrafficSummary()
  }
)

// --- Filters ---

const showCustomTimeRangeDialog = ref(false)
const customStartTimeInput = ref('')
const customEndTimeInput = ref('')
/* A range that ends before it starts silently returned no data. */
const rangeError = computed(() => {
  if (!customStartTimeInput.value || !customEndTimeInput.value) return ''
  const start = new Date(customStartTimeInput.value).getTime()
  const end = new Date(customEndTimeInput.value).getTime()
  if (Number.isNaN(start) || Number.isNaN(end)) return ''
  return end > start ? '' : t('admin.ops.customTimeRange.endBeforeStart')
})


function formatCustomTimeRangeLabel(startTime: string, endTime: string): string {
  const start = new Date(startTime)
  const end = new Date(endTime)
  const formatDate = (d: Date) => {
    const month = String(d.getMonth() + 1).padStart(2, '0')
    const day = String(d.getDate()).padStart(2, '0')
    const hour = String(d.getHours()).padStart(2, '0')
    const minute = String(d.getMinutes()).padStart(2, '0')
    return `${month}-${day} ${hour}:${minute}`
  }
  return `${formatDate(start)} ~ ${formatDate(end)}`
}

const groups = ref<Array<{ id: number; name: string; platform: string }>>([])

const platformOptions = computed(() => [
  { value: '', label: t('common.all') },
  ...CONCRETE_PLATFORM_OPTIONS
])

const timeRangeOptions = computed(() => [
  { value: '5m', label: t('admin.ops.timeRange.5m') },
  { value: '30m', label: t('admin.ops.timeRange.30m') },
  { value: '1h', label: t('admin.ops.timeRange.1h') },
  { value: '6h', label: t('admin.ops.timeRange.6h') },
  { value: '24h', label: t('admin.ops.timeRange.24h') },
  {
    value: 'custom',
    label: props.timeRange === 'custom' && props.customStartTime && props.customEndTime
      ? `${t('admin.ops.timeRange.custom')} (${formatCustomTimeRangeLabel(props.customStartTime, props.customEndTime)})`
      : t('admin.ops.timeRange.custom')
  }
])

const queryModeOptions = computed(() => [
  { value: 'auto', label: t('admin.ops.queryMode.auto') },
  { value: 'raw', label: t('admin.ops.queryMode.raw') },
  { value: 'preagg', label: t('admin.ops.queryMode.preagg') }
])

const groupOptions = computed(() => {
  const filtered = props.platform ? groups.value.filter((g) => g.platform === props.platform) : groups.value
  return [{ value: null, label: t('common.all') }, ...filtered.map((g) => ({ value: g.id, label: g.name }))]
})

watch(
  () => props.platform,
  (newPlatform) => {
    if (!newPlatform) return
    const currentGroup = groups.value.find((g) => g.id === props.groupId)
    if (currentGroup && currentGroup.platform !== newPlatform) {
      emit('update:group', null)
    }
  }
)

onMounted(async () => {
  try {
    const list = await adminAPI.groups.getAll()
    groups.value = list.map((g) => ({ id: g.id, name: g.name, platform: g.platform }))
  } catch (e) {
    console.error('[OpsDashboardHeader] Failed to load groups', e)
    groups.value = []
  }
})

function handlePlatformChange(val: string | number | boolean | null) {
  emit('update:platform', String(val || ''))
}

function handleGroupChange(val: string | number | boolean | null) {
  if (val === null || val === '' || typeof val === 'boolean') {
    emit('update:group', null)
    return
  }
  const id = typeof val === 'number' ? val : Number.parseInt(String(val), 10)
  emit('update:group', Number.isFinite(id) && id > 0 ? id : null)
}

function handleTimeRangeChange(val: string | number | boolean | null) {
  const newValue = String(val || '1h')
  if (newValue === 'custom') {
    // 初始化为最近1小时
    const now = new Date()
    const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000)
    customStartTimeInput.value = oneHourAgo.toISOString().slice(0, 16)
    customEndTimeInput.value = now.toISOString().slice(0, 16)
    showCustomTimeRangeDialog.value = true
  } else {
    emit('update:timeRange', newValue)
  }
}

function handleCustomTimeRangeConfirm() {
  if (!customStartTimeInput.value || !customEndTimeInput.value) return
  const startTime = new Date(customStartTimeInput.value).toISOString()
  const endTime = new Date(customEndTimeInput.value).toISOString()
  // Emit custom time range first so the parent can build correct API params
  // when it reacts to timeRange switching to "custom".
  emit('update:customTimeRange', startTime, endTime)
  emit('update:timeRange', 'custom')
  showCustomTimeRangeDialog.value = false
}

function handleCustomTimeRangeCancel() {
  showCustomTimeRangeDialog.value = false
  // 如果当前不是 custom，不需要做任何事
  // 如果当前是 custom，保持不变
}

function handleQueryModeChange(val: string | number | boolean | null) {
  emit('update:queryMode', String(val || 'auto'))
}

function openDetails(preset?: OpsRequestDetailsPreset) {
  emit('openRequestDetails', preset)
}

function openErrorDetails(kind: 'request' | 'upstream') {
  emit('openErrorDetails', kind)
}

// --- Threshold checking helpers ---
type ThresholdLevel = 'normal' | 'warning' | 'critical'


function getTTFTThresholdLevel(ttftMs: number | null): ThresholdLevel {
  if (ttftMs == null) return 'normal'
  const threshold = props.thresholds?.ttft_p99_ms_max
  if (threshold == null) return 'normal'
  if (ttftMs >= threshold) return 'critical'
  if (ttftMs >= threshold * 0.8) return 'warning'
  return 'normal'
}


function getUpstreamErrorRateThresholdLevel(upstreamErrorRatePercent: number | null): ThresholdLevel {
  if (upstreamErrorRatePercent == null) return 'normal'
  const threshold = props.thresholds?.upstream_error_rate_percent_max
  if (threshold == null) return 'normal'
  if (upstreamErrorRatePercent >= threshold) return 'critical'
  if (upstreamErrorRatePercent >= threshold * 0.8) return 'warning'
  return 'normal'
}


// --- Realtime / Overview labels ---

const totalRequestsLabel = computed(() => formatNumber(overview.value?.request_count_total ?? 0))
const totalTokensLabel = computed(() => formatNumber(overview.value?.token_consumed ?? 0))

const realtimeTrafficSummary = ref<OpsRealtimeTrafficSummary | null>(null)
const realtimeTrafficLoading = ref(false)

function makeZeroRealtimeTrafficSummary(): OpsRealtimeTrafficSummary {
  const now = new Date().toISOString()
  return {
    window: realtimeWindow.value,
    start_time: now,
    end_time: now,
    platform: props.platform,
    group_id: props.groupId,
    qps: { current: 0, peak: 0, avg: 0 },
    tps: { current: 0, peak: 0, avg: 0 }
  }
}

async function loadRealtimeTrafficSummary() {
  if (realtimeTrafficLoading.value) return
  if (!adminSettingsStore.opsRealtimeMonitoringEnabled) {
    realtimeTrafficSummary.value = makeZeroRealtimeTrafficSummary()
    return
  }
  realtimeTrafficLoading.value = true
  try {
    const res = await opsAPI.getRealtimeTrafficSummary(realtimeWindow.value, props.platform, props.groupId)
    if (res && res.enabled === false) {
      adminSettingsStore.setOpsRealtimeMonitoringEnabledLocal(false)
    }
    realtimeTrafficSummary.value = res?.summary ?? null
  } catch (err) {
    console.error('[OpsDashboardHeader] Failed to load realtime traffic summary', err)
    realtimeTrafficSummary.value = null
  } finally {
    realtimeTrafficLoading.value = false
  }
}

watch(
  () => [realtimeWindow.value, props.platform, props.groupId] as const,
  () => {
    loadRealtimeTrafficSummary()
  },
  { immediate: true }
)

watch(
  () => adminSettingsStore.opsRealtimeMonitoringEnabled,
  (enabled) => {
    if (!enabled) {
      // Keep UI stable when realtime monitoring is turned off.
      realtimeTrafficSummary.value = makeZeroRealtimeTrafficSummary()
    } else {
      loadRealtimeTrafficSummary()
    }
  },
  { immediate: true }
)

// Realtime traffic refresh follows the parent (OpsDashboard) refresh cadence.
watch(
  () => [props.autoRefreshEnabled, props.autoRefreshCountdown, props.loading] as const,
  ([enabled, countdown, loading]) => {
    if (!enabled) return
    if (loading) return
    // Treat countdown reset (or reaching 0) as a refresh boundary.
    if (countdown === 0) {
      loadRealtimeTrafficSummary()
    }
  }
)

// no-op: parent controls refresh cadence

const displayRealTimeQps = computed(() => {
  const v = realtimeTrafficSummary.value?.qps?.current
  return typeof v === 'number' && Number.isFinite(v) ? v : 0
})

const displayRealTimeTps = computed(() => {
  const v = realtimeTrafficSummary.value?.tps?.current
  return typeof v === 'number' && Number.isFinite(v) ? v : 0
})

const realtimeQpsPeakLabel = computed(() => {
  const v = realtimeTrafficSummary.value?.qps?.peak
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(1) : '-'
})
const realtimeTpsPeakLabel = computed(() => {
  const v = realtimeTrafficSummary.value?.tps?.peak
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(1) : '-'
})
const realtimeQpsAvgLabel = computed(() => {
  const v = realtimeTrafficSummary.value?.qps?.avg
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(1) : '-'
})
const realtimeTpsAvgLabel = computed(() => {
  const v = realtimeTrafficSummary.value?.tps?.avg
  return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(1) : '-'
})

const qpsAvgLabel = computed(() => {
  const v = overview.value?.qps?.avg
  if (typeof v !== 'number') return '-'
  return v.toFixed(1)
})

const tpsAvgLabel = computed(() => {
  const v = overview.value?.tps?.avg
  if (typeof v !== 'number') return '-'
  return v.toFixed(1)
})



const upstreamErrorRatePercent = computed(() => {
  const v = overview.value?.upstream_error_rate
  if (typeof v !== 'number') return null
  return v * 100
})

const durationP99Ms = computed(() => overview.value?.duration?.p99_ms ?? null)
const durationP95Ms = computed(() => overview.value?.duration?.p95_ms ?? null)
const durationP90Ms = computed(() => overview.value?.duration?.p90_ms ?? null)
const durationP50Ms = computed(() => overview.value?.duration?.p50_ms ?? null)
const durationAvgMs = computed(() => overview.value?.duration?.avg_ms ?? null)
const durationMaxMs = computed(() => overview.value?.duration?.max_ms ?? null)

const ttftP99Ms = computed(() => overview.value?.ttft?.p99_ms ?? null)
const ttftP95Ms = computed(() => overview.value?.ttft?.p95_ms ?? null)
const ttftP90Ms = computed(() => overview.value?.ttft?.p90_ms ?? null)
const ttftP50Ms = computed(() => overview.value?.ttft?.p50_ms ?? null)
const ttftAvgMs = computed(() => overview.value?.ttft?.avg_ms ?? null)
const ttftMaxMs = computed(() => overview.value?.ttft?.max_ms ?? null)

// --- Health Score & Diagnosis (primary) ---

const isSystemIdle = computed(() => {
  const ov = overview.value
  if (!ov) return true
  const qps = ov.qps?.current
  const errorRate = ov.error_rate ?? 0
  return (qps ?? 0) === 0 && errorRate === 0
})

const healthScoreValue = computed<number | null>(() => {
  const v = overview.value?.health_score
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})




interface DiagnosisItem {
  type: 'critical' | 'warning' | 'info'
  message: string
  impact: string
  action?: string
}

const diagnosisReport = computed<DiagnosisItem[]>(() => {
  const ov = overview.value
  if (!ov) return []

  const report: DiagnosisItem[] = []

  if (isSystemIdle.value) {
    report.push({
      type: 'info',
      message: t('admin.ops.diagnosis.idle'),
      impact: t('admin.ops.diagnosis.idleImpact')
    })
    return report
  }

  // Resource diagnostics (highest priority)
  const sm = ov.system_metrics
  if (sm) {
    if (sm.db_ok === false) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.dbDown'),
        impact: t('admin.ops.diagnosis.dbDownImpact'),
        action: t('admin.ops.diagnosis.dbDownAction')
      })
    }
    if (sm.redis_ok === false) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.redisDown'),
        impact: t('admin.ops.diagnosis.redisDownImpact'),
        action: t('admin.ops.diagnosis.redisDownAction')
      })
    }

    const cpuPct = sm.cpu_usage_percent ?? 0
    if (cpuPct > 90) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.cpuCritical', { usage: cpuPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.cpuCriticalImpact'),
        action: t('admin.ops.diagnosis.cpuCriticalAction')
      })
    } else if (cpuPct > 80) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.cpuHigh', { usage: cpuPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.cpuHighImpact'),
        action: t('admin.ops.diagnosis.cpuHighAction')
      })
    }

    const memPct = sm.memory_usage_percent ?? 0
    if (memPct > 90) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.memoryCritical', { usage: memPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.memoryCriticalImpact'),
        action: t('admin.ops.diagnosis.memoryCriticalAction')
      })
    } else if (memPct > 85) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.memoryHigh', { usage: memPct.toFixed(1) }),
        impact: t('admin.ops.diagnosis.memoryHighImpact'),
        action: t('admin.ops.diagnosis.memoryHighAction')
      })
    }
  }

  const ttftP99 = ov.ttft?.p99_ms ?? 0
  if (ttftP99 > 500) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.ttftHigh', { ttft: ttftP99.toFixed(0) }),
      impact: t('admin.ops.diagnosis.ttftHighImpact'),
      action: t('admin.ops.diagnosis.ttftHighAction')
    })
  }

  // Error rate diagnostics (adjusted thresholds)
  const upstreamRatePct = (ov.upstream_error_rate ?? 0) * 100
  if (upstreamRatePct > 5) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.upstreamCritical', { rate: upstreamRatePct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.upstreamCriticalImpact'),
      action: t('admin.ops.diagnosis.upstreamCriticalAction')
    })
  } else if (upstreamRatePct > 2) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.upstreamHigh', { rate: upstreamRatePct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.upstreamHighImpact'),
      action: t('admin.ops.diagnosis.upstreamHighAction')
    })
  }

  const errorPct = (ov.error_rate ?? 0) * 100
  if (errorPct > 3) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.errorHigh', { rate: errorPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.errorHighImpact'),
      action: t('admin.ops.diagnosis.errorHighAction')
    })
  } else if (errorPct > 0.5) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.errorElevated', { rate: errorPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.errorElevatedImpact'),
      action: t('admin.ops.diagnosis.errorElevatedAction')
    })
  }

  // SLA diagnostics
  const slaPct = (ov.sla ?? 0) * 100
  if (slaPct < 90) {
    report.push({
      type: 'critical',
      message: t('admin.ops.diagnosis.slaCritical', { sla: slaPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.slaCriticalImpact'),
      action: t('admin.ops.diagnosis.slaCriticalAction')
    })
  } else if (slaPct < 98) {
    report.push({
      type: 'warning',
      message: t('admin.ops.diagnosis.slaLow', { sla: slaPct.toFixed(2) }),
      impact: t('admin.ops.diagnosis.slaLowImpact'),
      action: t('admin.ops.diagnosis.slaLowAction')
    })
  }

  // Health score diagnostics (lowest priority)
  if (healthScoreValue.value != null) {
    if (healthScoreValue.value < 60) {
      report.push({
        type: 'critical',
        message: t('admin.ops.diagnosis.healthCritical', { score: healthScoreValue.value }),
        impact: t('admin.ops.diagnosis.healthCriticalImpact'),
        action: t('admin.ops.diagnosis.healthCriticalAction')
      })
    } else if (healthScoreValue.value < 90) {
      report.push({
        type: 'warning',
        message: t('admin.ops.diagnosis.healthLow', { score: healthScoreValue.value }),
        impact: t('admin.ops.diagnosis.healthLowImpact'),
        action: t('admin.ops.diagnosis.healthLowAction')
      })
    }
  }

  if (report.length === 0) {
    report.push({
      type: 'info',
      message: t('admin.ops.diagnosis.healthy'),
      impact: t('admin.ops.diagnosis.healthyImpact')
    })
  }

  return report
})

// --- System health (secondary) ---

function formatTimeShort(ts?: string | null): string {
  if (!ts) return '-'
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleTimeString()
}

























const jobHeartbeats = computed(() => overview.value?.job_heartbeats ?? [])

const jobsStatus = computed<'ok' | 'warn' | 'unknown'>(() => {
  const list = jobHeartbeats.value
  if (!list.length) return 'unknown'
  for (const hb of list) {
    if (!hb) continue
    if (hb.last_error_at && (!hb.last_success_at || hb.last_error_at > hb.last_success_at)) return 'warn'
  }
  return 'ok'
})

const jobsWarnCount = computed(() => {
  let warn = 0
  for (const hb of jobHeartbeats.value) {
    if (!hb) continue
    if (hb.last_error_at && (!hb.last_success_at || hb.last_error_at > hb.last_success_at)) warn++
  }
  return warn
})

const jobsStatusLabel = computed(() => {
  switch (jobsStatus.value) {
    case 'ok':
      return t('admin.ops.ok')
    case 'warn':
      return t('common.warning')
    default:
      return t('admin.ops.noData')
  }
})


const showJobsDetails = ref(false)

function openJobsDetails() {
  showJobsDetails.value = true
}

function handleToolbarRefresh() {
  loadRealtimeTrafficSummary()
  emit('refresh')
}
/* The old cards mapped a threshold level to a Tailwind colour class. The card
   now takes a tone name and owns the ink, so the mapping is level -> tone. */
type CardTone = 'none' | 'ok' | 'warning' | 'critical'
const toCardTone = (level: ThresholdLevel): CardTone =>
  level === 'critical' ? 'critical' : level === 'warning' ? 'warning' : 'none'

const ttftTone = computed<CardTone>(() => toCardTone(getTTFTThresholdLevel(ttftP99Ms.value)))
const upstreamTone = computed<CardTone>(() =>
  toCardTone(getUpstreamErrorRateThresholdLevel(upstreamErrorRatePercent.value))
)

</script>

<template>
  <div class="opshead" :data-fullscreen="props.fullscreen || undefined">
    <!-- Top Toolbar -->
    <div class="opshead__bar">
      <div class="opshead__ident">
        <h1 class="opshead__title">
          <i class="hgi-stroke hgi-analytics-01 opshead__title-icon" aria-hidden="true" />
          {{ t('admin.ops.title') }}
        </h1>

        <div v-if="!props.fullscreen" class="opshead__status">
          <span
            class="opshead__state"
            :data-loading="props.loading || undefined"
            :title="props.loading ? t('admin.ops.loadingText') : t('admin.ops.ready')"
          >
            <span class="opshead__state-dot" aria-hidden="true" />
            {{ props.loading ? t('admin.ops.loadingText') : t('admin.ops.ready') }}
          </span>

          <span class="opshead__sep" aria-hidden="true" />
          <span>{{ t('common.refresh') }}: {{ lastUpdatedLabel }}</span>

          <template v-if="props.autoRefreshEnabled && props.autoRefreshCountdown !== undefined">
            <span class="opshead__sep" aria-hidden="true" />
            <span>{{ t('admin.ops.autoRefreshRemaining', { seconds: props.autoRefreshCountdown }) }}</span>
          </template>
        </div>
      </div>

      <div class="opshead__tools">
        <template v-if="!props.fullscreen">
          <Select
            :model-value="platform"
            :options="platformOptions"
            class="opshead__filter"
            @update:model-value="handlePlatformChange"
          />

          <Select
            :model-value="groupId"
            :options="groupOptions"
            class="opshead__filter opshead__filter--wide"
            @update:model-value="handleGroupChange"
          />

          <span class="opshead__divider" aria-hidden="true" />

          <Select
            :model-value="timeRange"
            :options="timeRangeOptions"
            class="opshead__filter opshead__filter--wide"
            @update:model-value="handleTimeRangeChange"
          />
        </template>

        <Select
          v-if="false"
          :model-value="queryMode"
          :options="queryModeOptions"
          class="opshead__filter opshead__filter--wide"
          @update:model-value="handleQueryModeChange"
        />

        <template v-if="!props.fullscreen">
          <IconButton
            icon="hgi-refresh"
            :class="{ 'opshead__refresh--busy s2a-spinner': loading }"
            :label="t('common.refresh')"
            size="sm"
            :disabled="loading"
            @click="handleToolbarRefresh"
          />

          <span class="opshead__divider" aria-hidden="true" />

          <!-- Was bg-blue-100/text-blue-700, a colour this system does not
               have. Alert rules is a normal secondary action. -->
          <Button
            variant="secondary"
            size="sm"
            icon="hgi-alert-02"
            :title="t('admin.ops.alertRules.title')"
            @click="emit('openAlertRules')"
          >
            {{ t('admin.ops.alertRules.manage') }}
          </Button>

          <Button
            variant="secondary"
            size="sm"
            icon="hgi-settings-01"
            :title="t('admin.ops.settings.title')"
            @click="emit('openSettings')"
          >
            {{ t('common.settings') }}
          </Button>

          <IconButton
            icon="hgi-arrow-expand"
            :label="t('admin.ops.fullscreen.enter')"
            size="sm"
            @click="emit('enterFullscreen')"
          />
        </template>
      </div>
    </div>

    <div v-if="overview" class="opshead__grid">
      <!-- Left: Health + Realtime, then Jobs -->
      <div class="opshead__left">
      <div class="opshead__now">
        <div class="opshead__now-inner">
          <!-- 1) Health Score -->
          <div class="opshead__health">
            <OpsHealthRing
              :score="healthScoreValue"
              :idle="isSystemIdle"
              :fullscreen="props.fullscreen"
            />

            <!--
              The diagnosis was hover-only on a div with no affordance: nothing
              said the score could be opened, and a hover target does not exist
              on touch at all. It is a disclosure button now, which is
              focusable, announces its state, and works on a phone.
            -->
            <button
              v-if="!props.fullscreen && diagnosisReport.length"
              type="button"
              class="opshead__diag-toggle"
              :aria-expanded="showDiagnosis"
              @click="showDiagnosis = !showDiagnosis"
            >
              <i class="hgi-stroke hgi-idea-01 opshead__diag-icon" aria-hidden="true" />
              {{ t('admin.ops.diagnosis.title') }}
              <span class="opshead__diag-count">{{ diagnosisReport.length }}</span>
            </button>
          </div>

          <!-- 2) Realtime Traffic -->
          <OpsRealtimeTraffic
            :window="realtimeWindow"
            :windows="availableRealtimeWindows"
            :qps="displayRealTimeQps"
            :tps="displayRealTimeTps"
            :qps-peak="realtimeQpsPeakLabel"
            :tps-peak="realtimeTpsPeakLabel"
            :qps-avg="realtimeQpsAvgLabel"
            :tps-avg="realtimeTpsAvgLabel"
            :fullscreen="props.fullscreen"
            @update:window="realtimeWindow = $event as RealtimeWindow"
          />
        </div>

        <!--
          In flow rather than floating. The old popover was absolutely
          positioned with a shadow-xl and z-50, escaping its card to overlay
          whatever was to its right; opening it in place pushes the layout
          instead of covering it, and needs no elevation to be readable.
        -->
        <div v-if="showDiagnosis && !props.fullscreen" class="opshead__diag">
          <ul class="opshead__diag-list">
            <li v-for="(item, idx) in diagnosisReport" :key="idx" class="opshead__diag-item">
              <i
                class="hgi-stroke opshead__diag-mark"
                :class="diagnosisIcon(item.type)"
                :data-tone="item.type"
                aria-hidden="true"
              />
              <div class="opshead__diag-body">
                <p class="opshead__diag-msg">{{ item.message }}</p>
                <p class="opshead__diag-impact">{{ item.impact }}</p>
                <p v-if="item.action" class="opshead__diag-action">{{ item.action }}</p>
              </div>
            </li>
          </ul>
          <p class="opshead__diag-foot">{{ t('admin.ops.diagnosis.footer') }}</p>
        </div>
      </div>

          <div class="ops-jobs">
            <div class="ops-jobs__head">
              <span class="ops-jobs__label">
                {{ t('admin.ops.jobs') }}
                <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.jobs')" />
              </span>
              <button v-if="!props.fullscreen" class="ops-jobs__action" type="button" @click="openJobsDetails">
                {{ t('admin.ops.requestDetails.details') }}
              </button>
            </div>
            <div class="ops-jobs__body">
              <p class="ops-jobs__value">{{ jobsStatusLabel }}</p>
              <p v-if="!props.fullscreen" class="ops-jobs__meta">
                {{ t('common.total') }} {{ jobHeartbeats.length }} · {{ t('common.warning') }} {{ jobsWarnCount }}
              </p>
            </div>
          </div>
      </div>

      <!-- Right: 6 cards (3 cols x 2 rows) -->
      <div class="opshead__tiles">
        <!-- Card 1: Requests -->
        <div style="order: 1;">
          <OpsStatCard
            :title="t('admin.ops.requestsTitle')"
            :tooltip="props.fullscreen ? '' : t('admin.ops.tooltips.totalRequests')"
            :rows="[
              { label: t('admin.ops.requests'), value: totalRequestsLabel },
              { label: t('admin.ops.tokens'), value: totalTokensLabel },
              { label: t('admin.ops.avgQps'), value: qpsAvgLabel },
              { label: t('admin.ops.avgTps'), value: tpsAvgLabel }
            ]"
            :show-details="!props.fullscreen"
            :details-label="t('admin.ops.requestDetails.details')"
            @open-details="openDetails({ title: t('admin.ops.requestDetails.title') })"
          />
        </div>

        <!--
          Was two cards: "SLA 88.050%" and "Request errors 11.95%", shown as if
          unrelated, with business-limited in small print under one of them.
          They are three parts of one total -- and the first two do NOT sum to
          100% whenever the third is non-zero, because business limits are
          excluded from the SLA denominator. One split bar shows that instead
          of leaving it as arithmetic for the reader.
        -->
        <div class="ops-traffic-cell" style="order: 2;">
          <OpsTrafficSplit
            :success-count="overview?.success_count ?? 0"
            :error-count-sla="overview?.error_count_sla ?? 0"
            :business-limited-count="overview?.business_limited_count ?? 0"
            :sla="overview?.sla ?? null"
            :fullscreen="props.fullscreen"
            @open-details="openErrorDetails('request')"
          />
        </div>

        <!-- Card 4: Request Duration -->
        <div style="order: 4;">
          <OpsStatCard
            :title="t('admin.ops.latencyDuration')"
            :tooltip="props.fullscreen ? '' : t('admin.ops.tooltips.latency')"
            :value="durationP99Ms == null ? '-' : String(durationP99Ms)"
            unit="ms (P99)"
            :rows="[
              { label: 'P95', value: durationP95Ms == null ? '-' : `${durationP95Ms} ms` },
              { label: 'P90', value: durationP90Ms == null ? '-' : `${durationP90Ms} ms` },
              { label: 'P50', value: durationP50Ms == null ? '-' : `${durationP50Ms} ms` },
              { label: 'Avg', value: durationAvgMs == null ? '-' : `${durationAvgMs} ms` },
              { label: 'Max', value: durationMaxMs == null ? '-' : `${durationMaxMs} ms` }
            ]"
            :show-details="!props.fullscreen"
            :details-label="t('admin.ops.requestDetails.details')"
            @open-details="openDetails({ title: t('admin.ops.latencyDuration'), sort: 'duration_desc' })"
          />
        </div>

        <!-- Card 5: TTFT -->
        <div style="order: 5;">
          <OpsStatCard
            :title="t('admin.ops.ttftLabel')"
            :tooltip="props.fullscreen ? '' : t('admin.ops.tooltips.ttft')"
            :value="ttftP99Ms == null ? '-' : String(ttftP99Ms)"
            unit="ms (P99)"
            :tone="ttftTone"
            :rows="[
              { label: 'P95', value: ttftP95Ms == null ? '-' : `${ttftP95Ms} ms` },
              { label: 'P90', value: ttftP90Ms == null ? '-' : `${ttftP90Ms} ms` },
              { label: 'P50', value: ttftP50Ms == null ? '-' : `${ttftP50Ms} ms` },
              { label: 'Avg', value: ttftAvgMs == null ? '-' : `${ttftAvgMs} ms` },
              { label: 'Max', value: ttftMaxMs == null ? '-' : `${ttftMaxMs} ms` }
            ]"
            :show-details="!props.fullscreen"
            :details-label="t('admin.ops.requestDetails.details')"
            @open-details="openDetails({ title: t('admin.ops.ttftLabel'), sort: 'duration_desc' })"
          />
        </div>


        <!-- Card 6: Upstream Errors -->
        <div style="order: 6;">
          <OpsStatCard
            :title="t('admin.ops.upstreamErrors')"
            :tooltip="props.fullscreen ? '' : t('admin.ops.tooltips.upstreamErrors')"
            :value="upstreamErrorRatePercent == null ? '-' : `${upstreamErrorRatePercent.toFixed(2)}%`"
            :tone="upstreamTone"
            :rows="[
              { label: t('admin.ops.errorCountExcl429529'), value: formatNumber(overview.upstream_error_count_excl_429_529 ?? 0) },
              { label: '429/529', value: formatNumber((overview.upstream_429_count ?? 0) + (overview.upstream_529_count ?? 0)) }
            ]"
            :show-details="!props.fullscreen"
            :details-label="t('admin.ops.requestDetails.details')"
            @open-details="openErrorDetails('upstream')"
          />
        </div>
      </div>
    </div>

    <!--
      Resource usage, full width.
      Five of the six cards here were flat text -- "CPU 0.5%", "DB ok" -- each
      already knowing its ceiling and its thresholds. OpsResourceMeters gives
      them the track that was missing.

      Jobs used to sit beside them in a 2/1 grid, where it was three lines of
      text stranded next to a five-row meter list. It has moved up beside the
      health ring: jobs are heartbeats, which answer "is the system healthy
      right now", the same question the ring and the realtime figures answer.
      Resource usage answers a different one -- what it is consuming -- and is
      the only thing on this row now.

      The rule that used to separate this block from the tiles above went with
      the outer card: these are peer blocks on the page background now, and the
      gap between them is the separation.
    -->
    <OpsResourceMeters v-if="overview" :metrics="systemMetrics" />

    <BaseDialog :show="showJobsDetails" :title="t('admin.ops.jobs')" width="wide" @close="showJobsDetails = false">
      <p v-if="!jobHeartbeats.length" class="opshead__empty">
        {{ t('admin.ops.noData') }}
      </p>
      <div v-else class="opshead__jobs-list">
        <!-- A job that last errored is the only one worth finding in this
             list, so it is the only one that carries any colour. -->
        <div
          v-for="hb in jobHeartbeats"
          :key="hb.job_name"
          class="opshead__job"
          :data-failed="hb.last_error ? '' : undefined"
        >
          <div class="opshead__job-head">
            <span class="opshead__job-name">{{ hb.job_name }}</span>
            <span class="opshead__job-when">
              <span v-if="hb.last_duration_ms != null" class="opshead__mono">{{ hb.last_duration_ms }}ms</span>
              <span>{{ formatTimeShort(hb.updated_at) }}</span>
            </span>
          </div>

          <dl class="opshead__job-facts">
            <div class="opshead__job-fact">
              <dt>{{ t('admin.ops.lastSuccess') }}</dt>
              <dd class="opshead__mono">{{ formatTimeShort(hb.last_success_at) }}</dd>
            </div>
            <div class="opshead__job-fact">
              <dt>{{ t('admin.ops.lastError') }}</dt>
              <dd class="opshead__mono">{{ formatTimeShort(hb.last_error_at) }}</dd>
            </div>
            <!-- The result string is the point of the row, so it gets the
                 full width and wraps rather than being cut at 180px. -->
            <div class="opshead__job-fact opshead__job-fact--wide">
              <dt>{{ t('admin.ops.result') }}</dt>
              <dd class="opshead__mono">{{ hb.last_result || '-' }}</dd>
            </div>
          </dl>

          <p v-if="hb.last_error" class="opshead__job-error">{{ hb.last_error }}</p>
        </div>
      </div>
    </BaseDialog>

    <!-- Custom Time Range Dialog -->
    <BaseDialog :show="showCustomTimeRangeDialog" :title="t('admin.ops.timeRange.custom')" width="narrow" @close="handleCustomTimeRangeCancel">
      <div class="opshead__range">
        <Input
          v-model="customStartTimeInput"
          type="datetime-local"
          :label="t('admin.ops.customTimeRange.startTime')"
        />
        <Input
          v-model="customEndTimeInput"
          type="datetime-local"
          :label="t('admin.ops.customTimeRange.endTime')"
          :error="rangeError"
        />
      </div>

      <template #footer>
        <div class="opshead__range-actions">
          <Button variant="ghost" size="md" @click="handleCustomTimeRangeCancel">
            {{ t('common.cancel') }}
          </Button>
          <Button variant="solid" size="md" :disabled="!!rangeError || !customStartTimeInput || !customEndTimeInput" @click="handleCustomTimeRangeConfirm">
            {{ t('common.confirm') }}
          </Button>
        </div>
      </template>
    </BaseDialog>
  </div>
</template>

<style scoped>
/* The split takes the width of the two cards it replaces, because it is
   showing what both of them used to. */
.ops-traffic-cell {
  grid-column: span 2;
  min-width: 0;
}

@media (max-width: 900px) {
  .ops-traffic-cell {
    grid-column: span 1;
  }
}

.ops-jobs {
  display: flex;
  flex: none;
  flex-direction: column;
  padding: 14px 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.ops-jobs__body {
  display: flex;
  flex-direction: column;
}

.ops-jobs__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.ops-jobs__label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.ops-jobs__action {
  border: 0;
  background: none;
  color: var(--brand);
  font-size: var(--fs-sm);
  cursor: pointer;
}

.ops-jobs__action:hover {
  text-decoration: underline;
}

.ops-jobs__value {
  margin: 6px 0 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.1;
}

.ops-jobs__meta {
  margin: 2px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}

/* --- shell ------------------------------------------------------------- */

/*
 * NOT A CARD.
 *
 * This was `rounded-3xl bg-white shadow-sm ring-1 ring-gray-900/5` -- a
 * shadow (ground rule 9) at a 24px radius, wrapping a grey tray that wrapped
 * six more cards. Three levels of surface for what is really a masthead and a
 * grid of tiles. Every other block on this page is a peer card sitting on the
 * page background; the header is now the same, so the top of the screen stops
 * looking like a different product.
 */
.opshead {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.opshead[data-fullscreen] {
  gap: 20px;
}

.opshead__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.opshead__ident {
  min-width: 0;
}

/* Was text-xl font-black, which is a weight this family does not ship. */
.opshead__title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-xl);
  font-weight: var(--fw-medium);
}

.opshead__title-icon {
  font-size: 19px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.opshead__status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.opshead__state {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

/* Brand when live, neutral while fetching. Was bg-green-500, the only green
   on a page that otherwise reads "good" as brand. */
.opshead__state-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--brand);
}

.opshead__state[data-loading] .opshead__state-dot {
  background: var(--muted-foreground);
}

/* A dot, not a "·" glyph: the separator was a text character sitting on the
   baseline at the same colour and size as the words either side of it. */
.opshead__sep {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: var(--border);
}

.opshead__tools {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.opshead__filter {
  width: 140px;
}

.opshead__filter--wide {
  width: 160px;
}

.opshead__divider {
  width: 1px;
  height: 16px;
  background: var(--border);
}

@media (max-width: 640px) {
  .opshead__divider {
    display: none;
  }

  .opshead__filter,
  .opshead__filter--wide {
    width: 100%;
  }
}

.opshead__refresh--busy {
  animation: s2a-spin 0.7s linear infinite;
}

/* --- top grid ---------------------------------------------------------- */

/*
 * 4/8 rather than 5/7. The left column held a 100px dial and a small realtime
 * block in 42% of the width, vertically centred inside a box as tall as six
 * stat cards -- roughly 200px of dead space, measured. Narrowing it gives the
 * width to the tiles that actually have content in them.
 */
.opshead__grid {
  display: grid;
  grid-template-columns: minmax(0, 4fr) minmax(0, 8fr);
  gap: 12px;
}

@media (max-width: 1100px) {
  .opshead__grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

/*
 * Three columns of tiles, with the traffic split taking two of them.
 *
 * This was `lg:col-span-7 lg:grid-cols-3` -- a column span left over from the
 * 12-column parent. Against the 2-column grid above it, `span 7` makes the
 * browser generate five implicit columns and the whole row falls apart: the
 * tiles drop out of the right cell and stack at full width under the health
 * card. Sized by its own grid now, not by a leftover span.
 */
.opshead__tiles {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  align-content: center;
  gap: 12px;
}

/* Each tile is wrapped in a positioning div carrying `order`. Without this the
   wrapper stretched to the row but the card inside kept its content height, so
   a short tile like Requests left a hairline-bounded gap under itself while
   its neighbour ran the full row. */
.opshead__tiles > div {
  display: flex;
  min-width: 0;
}

.opshead__tiles > div > * {
  flex: 1;
  min-width: 0;
}

@media (max-width: 900px) {
  .opshead__tiles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 560px) {
  .opshead__tiles {
    grid-template-columns: minmax(0, 1fr);
  }
}

/* Health + realtime, then jobs. Stacking the two lets this column reach the
   height the six tiles set, instead of one card being padded out to match. */
.opshead__left {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

/*
 * A container, because the thing that decides this card's layout is the CARD's
 * width, not the viewport's.
 *
 * Safe to rely on: this palette is built on `color-mix(in oklch, ...)`, which
 * needs Chrome 111 / Safari 16.2. `@container` shipped before both, so any
 * browser that can render the design system at all supports it.
 */
.opshead__now {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
  container-type: inline-size;
  container-name: opsnow;
}

/*
 * `auto minmax(0, 1fr)` was the bug behind the overflowing window selector.
 *
 * An `auto` track takes its max-content width and keeps it; `minmax(0, 1fr)`
 * lets the other track collapse all the way to zero. So as the card narrowed,
 * the health column held its full 139px while the realtime column was squeezed
 * to 73px -- and the segmented control inside it, which cannot render below
 * 199px, painted straight out through the card border. The `minmax(0, 1fr)`
 * was written to PREVENT overflow and is what guaranteed it.
 *
 * The floor is the measured intrinsic width of that control. Below the
 * breakpoint the two stack, and the realtime block gets the full card width.
 *
 * 420px is derived, not guessed. An inline-size container query measures the
 * CONTENT box, so the padding is already excluded: 139 health + 19 for the
 * divider and its padding + 18 gap + 199 control = 375 needed, and 420 leaves
 * headroom for a longer label. Measured across card widths 240px to 820px,
 * the control never overflows and the switch lands between 430 and 460.
 */
.opshead__now-inner {
  display: grid;
  grid-template-columns: auto minmax(200px, 1fr);
  align-items: stretch;
  gap: 18px;
  flex: 1;
}

@container opsnow (max-width: 420px) {
  .opshead__now-inner {
    grid-template-columns: minmax(0, 1fr);
  }
}

.opshead__health {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: space-evenly;
  gap: 10px;
  padding-right: 18px;
  border-right: 1px solid var(--border-subtle);
}

/* The rule turns from a vertical divider into a horizontal one when the two
   stack, so it always separates them along the axis they are split on. */
@container opsnow (max-width: 420px) {
  .opshead__health {
    padding-right: 0;
    padding-bottom: 14px;
    border-right: 0;
    border-bottom: 1px solid var(--border-subtle);
  }
}

/* --- diagnosis --------------------------------------------------------- */

.opshead__diag-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 9px;
  border: 1px solid var(--border);
  border-radius: var(--r-pill);
  background: var(--card);
  color: var(--muted-foreground);
  font-family: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: background var(--motion-hover);
}

.opshead__diag-toggle:hover {
  background: var(--sidebar-accent);
}

.opshead__diag-icon {
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  line-height: 1;
}

.opshead__diag-count {
  color: var(--body-copy);
  font-variant-numeric: tabular-nums;
}

.opshead__diag {
  padding: 12px 14px;
  border-radius: var(--r-md);
  background: var(--surface-subtle);
}

.opshead__diag-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.opshead__diag-item {
  display: flex;
  gap: 9px;
}

.opshead__diag-mark {
  flex: none;
  margin-top: 1px;
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1.2;
}

.opshead__diag-mark[data-tone='critical'] {
  color: var(--destructive);
}

.opshead__diag-mark[data-tone='warning'] {
  color: var(--s2a-attn);
}

.opshead__diag-body {
  min-width: 0;
}

.opshead__diag-msg {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-md);
}

.opshead__diag-impact {
  margin: 2px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* The suggested action was text-blue-600. It is advice, not a link. */
.opshead__diag-action {
  margin: 3px 0 0;
  color: var(--body-copy);
  font-size: var(--fs-sm);
}

.opshead__diag-foot {
  margin: 10px 0 0;
  padding-top: 8px;
  border-top: 1px solid var(--border-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

/* --- jobs dialog ------------------------------------------------------- */

.opshead__empty {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}

.opshead__jobs-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.opshead__job {
  padding: 12px 14px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--card);
}

.opshead__job-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
}

.opshead__job-name {
  min-width: 0;
  overflow: hidden;
  color: var(--foreground);
  font-size: var(--fs-md);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.opshead__job-when {
  display: inline-flex;
  flex: none;
  align-items: baseline;
  gap: 10px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.opshead__job-facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 4px 14px;
  margin: 8px 0 0;
}

.opshead__job-fact {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.opshead__job-fact dd {
  margin: 0;
  min-width: 0;
  overflow: hidden;
  color: var(--body-copy);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.opshead__job-fact--wide {
  grid-column: 1 / -1;
  align-items: baseline;
}

.opshead__job-fact--wide dd {
  overflow: visible;
  overflow-wrap: anywhere;
  white-space: normal;
}

.opshead__mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}

.opshead__job-error {
  margin: 10px 0 0;
  padding: 8px 10px;
  border-radius: var(--r-sm);
  background: color-mix(in oklch, var(--destructive) 10%, var(--card));
  color: var(--destructive);
  font-size: var(--fs-sm);
  overflow-wrap: anywhere;
}

/* --- custom range dialog ----------------------------------------------- */

.opshead__range {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.opshead__range-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
