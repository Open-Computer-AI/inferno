<script setup lang="ts">
/**
 * System logs.
 *
 * The largest surface on the ops page and the one with the most legacy in it:
 * a `.input` global class on eleven fields, four `.btn` variants, two native
 * window.confirm() calls, and a header row that broke three ground rules in
 * one class list. Those are mechanical. Five things are not.
 *
 * THE DETAIL CELL WAS A WALL
 * Every row read `http request completed status=200 latency_ms=4 method=GET
 * path=/api/v1/auth/me ip=172.18.0.1 proto=HTTP/1.1 req=ad6bcde2-80b4-...`,
 * joined into one string and set with `break-all`, which splits a UUID
 * mid-character. Twenty rows of that are twenty identical-looking rows: the
 * message is the same on all of them and the parts that actually differ are
 * buried mid-string. The data was already parsed into fields by the formatter
 * before being re-flattened into text -- so it is now rendered as fields.
 * Message and access facts on one line, correlation ids on a quieter second
 * line, errors on a third. Nothing is dropped; it is the same content, keyed.
 *
 * ZERO IS NOT AN ALARM
 * "Dropped 0" was permanently amber and "Failed 0" permanently red, on a sink
 * that had never dropped or failed anything. A counter at zero is the good
 * outcome. Both are neutral until they are non-zero, and the queue pill takes
 * the same ok/warning/critical thresholds as the resource meters.
 *
 * ONLY WARN AND ERROR GET A BADGE
 * `info` was a blue pill -- a colour that does not exist in this palette --
 * on 7,724 of 7,739 rows. A badge that appears on everything marks nothing.
 * info and debug are plain muted text now, so an error row is the only thing
 * with a chip in that column and it is visible from across the table.
 *
 * THE RUNTIME CONFIG COLLAPSES
 * It is a settings form for verbosity, sampling and retention, and it sat
 * between the header and the filters costing ~150px on a page you open to
 * read log lines. It is a disclosure now, closed by default, with its live
 * state -- level, sampling on/off, retention -- shown in the summary row so
 * the setting is still readable without opening it.
 *
 * TIME MATCHES THE REST OF THE PAGE
 * `toLocaleString()` gave "8/12/2026, 9:24:59 PM" directly below an alerts
 * table showing "08-12 20:26:50". Two clocks on one page is a defect, so this
 * uses the same ops formatter as everything else here.
 */
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { opsAPI, type OpsRuntimeLogConfig, type OpsSystemLog, type OpsSystemLogSinkHealth } from '@/api/admin/ops'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Input from '@/components/common/Input.vue'
import Checkbox from '@/components/common/Checkbox.vue'
import Button from '@/components/common/Button.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '../utils/opsFormatters'

const appStore = useAppStore()
const { t } = useI18n()

// 与 DataTable 一致：< 768px 切换为卡片视图，避免宽表在移动端被截断。
const isDesktopViewport = useMediaQuery('(min-width: 768px)')

const props = withDefaults(defineProps<{
  platformFilter?: string
  refreshToken?: number
}>(), {
  platformFilter: '',
  refreshToken: 0
})

const loading = ref(false)
const logs = ref<OpsSystemLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const health = ref<OpsSystemLogSinkHealth>({
  queue_depth: 0,
  queue_capacity: 0,
  dropped_count: 0,
  write_failed_count: 0,
  written_count: 0,
  avg_write_delay_ms: 0
})

const configOpen = ref(false)
const showResetConfirm = ref(false)
const showCleanupConfirm = ref(false)

/**
 * Text drafts for the three numeric settings.
 *
 * Coercing on every keystroke fights the user: clearing the field to retype it
 * yields "", which snaps straight back to the fallback before the next digit
 * lands. The draft holds whatever is typed and commits on change, which fires
 * on blur or Enter.
 */
const drafts = reactive({
  sampling_initial: '100',
  sampling_thereafter: '100',
  retention_days: '30'
})

/** Integer-only, so a stray "12abc" or an empty field cannot reach the API. */
const toCount = (value: string | number, fallback: number) => {
  const n = Number.parseInt(String(value), 10)
  return Number.isFinite(n) && n > 0 ? n : fallback
}

type DraftKey = keyof typeof drafts

const commitDraft = (key: DraftKey, fallback: number) => {
  const n = toCount(drafts[key], fallback)
  runtimeConfig[key] = n
  drafts[key] = String(n)
}

const runtimeLoading = ref(false)
const runtimeSaving = ref(false)
const runtimeConfig = reactive<OpsRuntimeLogConfig>({
  level: 'info',
  enable_sampling: false,
  sampling_initial: 100,
  sampling_thereafter: 100,
  caller: true,
  stacktrace_level: 'error',
  retention_days: 30
})

const filters = reactive({
  time_range: '1h' as '5m' | '30m' | '1h' | '6h' | '24h' | '7d' | '30d',
  start_time: '',
  end_time: '',
  host: '',
  level: '',
  component: '',
  request_id: '',
  client_request_id: '',
  user_id: '',
  api_key_id: '',
  account_id: '',
  platform: '',
  model: '',
  q: ''
})

const runtimeLevelOptions = [
  { value: 'debug', label: 'debug' },
  { value: 'info', label: 'info' },
  { value: 'warn', label: 'warn' },
  { value: 'error', label: 'error' }
]

const stacktraceLevelOptions = [
  { value: 'none', label: 'none' },
  { value: 'error', label: 'error' },
  { value: 'fatal', label: 'fatal' }
]

const timeRangeOptions = [
  { value: '5m', label: '5m' },
  { value: '30m', label: '30m' },
  { value: '1h', label: '1h' },
  { value: '6h', label: '6h' },
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' }
]

/* Names its own dimension rather than "All", so the control reads as a filter
   for levels even when nothing is chosen. */
const filterLevelOptions = computed(() => [
  { value: '', label: t('admin.ops.systemLogs.anyLevel') },
  { value: 'debug', label: 'debug' },
  { value: 'info', label: 'info' },
  { value: 'warn', label: 'warn' },
  { value: 'error', label: 'error' }
])

type LevelTone = 'error' | 'warn' | null

/**
 * Only the levels worth stopping for get a chip.
 *
 * `info` was a blue pill on 7,724 of 7,739 rows. A badge on every row is not a
 * badge, and blue is not in this palette. info and debug fall back to plain
 * text so an error is the only chip in the column.
 */
const levelTone = (level: string): LevelTone => {
  const v = String(level || '').toLowerCase()
  if (v === 'error' || v === 'fatal' || v === 'panic') return 'error'
  if (v === 'warn' || v === 'warning') return 'warn'
  return null
}

type HealthTone = 'warning' | 'critical' | undefined

/* Same thresholds as OpsResourceMeters, so a queue at 80% reads the same here
   as it does up the page. Undefined rather than 'ok' when healthy: every pill
   in this row signals the same way, by having no tone until it has news. */
const queueTone = computed<HealthTone>(() => {
  const cap = health.value.queue_capacity
  if (!cap) return undefined
  const pct = (health.value.queue_depth / cap) * 100
  if (pct >= 90) return 'critical'
  if (pct >= 70) return 'warning'
  return undefined
})

const getExtraString = (extra: Record<string, any> | undefined, key: string) => {
  if (!extra) return ''
  const v = extra[key]
  if (v == null) return ''
  if (typeof v === 'string') return v.trim()
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  return ''
}

interface LogField {
  key: string
  value: string
  tone?: 'bad'
}

interface LogDetail {
  message: string
  /** What happened: status, latency, route. Read left to right. */
  facts: LogField[]
  /** Correlation ids. Present to be grepped, not read, so they sit quieter. */
  refs: LogField[]
  error: string
}

/**
 * Same fields the old formatter collected, kept as fields instead of being
 * joined back into one string.
 *
 * Only 5xx tones the status: a 401 on /auth/me is the client being told no,
 * which is the gateway working. Painting every 4xx would put amber on a large
 * share of ordinary rows and teach people to ignore it.
 */
const logDetail = (row: OpsSystemLog): LogDetail => {
  const extra = row.extra || {}
  const facts: LogField[] = []
  const refs: LogField[] = []

  const statusCode = getExtraString(extra, 'status_code')
  if (statusCode) {
    facts.push({ key: 'status', value: statusCode, tone: /^5\d\d$/.test(statusCode) ? 'bad' : undefined })
  }
  const latencyMs = getExtraString(extra, 'latency_ms')
  if (latencyMs) facts.push({ key: 'latency_ms', value: latencyMs })
  const method = getExtraString(extra, 'method')
  if (method) facts.push({ key: 'method', value: method })
  const path = getExtraString(extra, 'path')
  if (path) facts.push({ key: 'path', value: path })
  const clientIP = getExtraString(extra, 'client_ip')
  if (clientIP) facts.push({ key: 'ip', value: clientIP })
  const protocol = getExtraString(extra, 'protocol')
  if (protocol) facts.push({ key: 'proto', value: protocol })

  if (row.request_id) refs.push({ key: 'req', value: row.request_id })
  if (row.client_request_id) refs.push({ key: 'client_req', value: row.client_request_id })
  if (row.user_id != null) refs.push({ key: 'user', value: String(row.user_id) })
  if (row.api_key_id != null) refs.push({ key: 'key', value: String(row.api_key_id) })
  if (row.account_id != null) refs.push({ key: 'acc', value: String(row.account_id) })
  if (row.platform) refs.push({ key: 'platform', value: row.platform })
  if (row.model) refs.push({ key: 'model', value: row.model })

  const errors = getExtraString(extra, 'errors')
  const err = getExtraString(extra, 'err') || getExtraString(extra, 'error')
  const errorText = [errors, err].filter(Boolean).join('  ')

  return { message: String(row.message || '').trim(), facts, refs, error: errorText }
}

const toRFC3339 = (value: string) => {
  if (!value) return undefined
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return undefined
  return d.toISOString()
}

const buildQuery = () => {
  const query: Record<string, any> = {
    page: page.value,
    page_size: pageSize.value,
    time_range: filters.time_range
  }

  if (filters.time_range === '30d') {
    query.time_range = '30d'
  }
  if (filters.start_time) query.start_time = toRFC3339(filters.start_time)
  if (filters.end_time) query.end_time = toRFC3339(filters.end_time)
  if (filters.host.trim()) query.host = filters.host.trim()
  if (filters.level.trim()) query.level = filters.level.trim()
  if (filters.component.trim()) query.component = filters.component.trim()
  if (filters.request_id.trim()) query.request_id = filters.request_id.trim()
  if (filters.client_request_id.trim()) query.client_request_id = filters.client_request_id.trim()
  if (filters.user_id.trim()) {
    const v = Number.parseInt(filters.user_id.trim(), 10)
    if (Number.isFinite(v) && v > 0) query.user_id = v
  }
  if (filters.api_key_id.trim()) {
    const v = Number.parseInt(filters.api_key_id.trim(), 10)
    if (Number.isFinite(v) && v > 0) query.api_key_id = v
  }
  if (filters.account_id.trim()) {
    const v = Number.parseInt(filters.account_id.trim(), 10)
    if (Number.isFinite(v) && v > 0) query.account_id = v
  }
  if (filters.platform.trim()) query.platform = filters.platform.trim()
  if (filters.model.trim()) query.model = filters.model.trim()
  if (filters.q.trim()) query.q = filters.q.trim()
  return query
}

const fetchLogs = async () => {
  loading.value = true
  try {
    const res = await opsAPI.listSystemLogs(buildQuery())
    logs.value = res.items || []
    total.value = res.total || 0
  } catch (err: any) {
    console.error('[OpsSystemLogTable] Failed to fetch logs', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.systemLogs.loadFailed'))
  } finally {
    loading.value = false
  }
}

const fetchHealth = async () => {
  try {
    health.value = await opsAPI.getSystemLogSinkHealth()
  } catch {
    // 忽略健康数据读取失败，不影响主流程。
  }
}

const loadRuntimeConfig = async () => {
  runtimeLoading.value = true
  try {
    applyRuntimeConfig(await opsAPI.getRuntimeLogConfig())
  } catch (err: any) {
    console.error('[OpsSystemLogTable] Failed to load runtime log config', err)
  } finally {
    runtimeLoading.value = false
  }
}

const applyRuntimeConfig = (saved: OpsRuntimeLogConfig) => {
  runtimeConfig.level = saved.level
  runtimeConfig.enable_sampling = saved.enable_sampling
  runtimeConfig.sampling_initial = saved.sampling_initial
  runtimeConfig.sampling_thereafter = saved.sampling_thereafter
  runtimeConfig.caller = saved.caller
  runtimeConfig.stacktrace_level = saved.stacktrace_level
  runtimeConfig.retention_days = saved.retention_days
  drafts.sampling_initial = String(saved.sampling_initial)
  drafts.sampling_thereafter = String(saved.sampling_thereafter)
  drafts.retention_days = String(saved.retention_days)
}

const saveRuntimeConfig = async () => {
  runtimeSaving.value = true
  try {
    applyRuntimeConfig(await opsAPI.updateRuntimeLogConfig({ ...runtimeConfig }))
    appStore.showSuccess(t('admin.ops.systemLogs.runtimeConfigActive'))
  } catch (err: any) {
    console.error('[OpsSystemLogTable] Failed to save runtime log config', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.systemLogs.runtimeConfigSaveFailed'))
  } finally {
    runtimeSaving.value = false
  }
}

/* Both destructive actions used window.confirm, which cannot be styled, cannot
   be keyboard-trapped with the rest of the app and looks like a browser fault
   rather than a decision the product is asking for. */
const resetRuntimeConfig = async () => {
  showResetConfirm.value = false
  runtimeSaving.value = true
  try {
    applyRuntimeConfig(await opsAPI.resetRuntimeLogConfig())
    appStore.showSuccess(t('admin.ops.systemLogs.runtimeConfigReset'))
    await fetchHealth()
  } catch (err: any) {
    console.error('[OpsSystemLogTable] Failed to reset runtime log config', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.systemLogs.runtimeConfigResetFailed'))
  } finally {
    runtimeSaving.value = false
  }
}

const cleanupCurrentFilter = async () => {
  showCleanupConfirm.value = false
  try {
    const payload = {
      start_time: toRFC3339(filters.start_time),
      end_time: toRFC3339(filters.end_time),
      host: filters.host.trim() || undefined,
      level: filters.level.trim() || undefined,
      component: filters.component.trim() || undefined,
      request_id: filters.request_id.trim() || undefined,
      client_request_id: filters.client_request_id.trim() || undefined,
      user_id: filters.user_id.trim() ? Number.parseInt(filters.user_id.trim(), 10) : undefined,
      api_key_id: filters.api_key_id.trim() ? Number.parseInt(filters.api_key_id.trim(), 10) : undefined,
      account_id: filters.account_id.trim() ? Number.parseInt(filters.account_id.trim(), 10) : undefined,
      platform: filters.platform.trim() || undefined,
      model: filters.model.trim() || undefined,
      q: filters.q.trim() || undefined
    }
    const res = await opsAPI.cleanupSystemLogs(payload)
    appStore.showSuccess(t('admin.ops.systemLogs.cleanupSuccess', { count: res.deleted || 0 }))
    page.value = 1
    await Promise.all([fetchLogs(), fetchHealth()])
  } catch (err: any) {
    console.error('[OpsSystemLogTable] Failed to cleanup logs', err)
    appStore.showError(
      extractApiErrorMessage(err, t('admin.ops.systemLogs.cleanupFailed'), {
        OPS_SYSTEM_LOG_CLEANUP_FILTER_REQUIRED: t('admin.ops.systemLogs.cleanupFilterRequired')
      })
    )
  }
}

const resetFilters = () => {
  filters.time_range = '1h'
  filters.start_time = ''
  filters.end_time = ''
  filters.host = ''
  filters.level = ''
  filters.component = ''
  filters.request_id = ''
  filters.client_request_id = ''
  filters.user_id = ''
  filters.api_key_id = ''
  filters.account_id = ''
  filters.platform = props.platformFilter || ''
  filters.model = ''
  filters.q = ''
  page.value = 1
  fetchLogs()
}

watch(() => props.platformFilter, (v) => {
  if (v && !filters.platform) {
    filters.platform = v
    page.value = 1
    fetchLogs()
  }
})

watch(() => props.refreshToken, () => {
  fetchLogs()
  fetchHealth()
})

const onPageChange = (next: number) => {
  page.value = next
  fetchLogs()
}

const onPageSizeChange = (next: number) => {
  pageSize.value = next
  page.value = 1
  fetchLogs()
}

const applyFilters = () => {
  page.value = 1
  fetchLogs()
}

const hasData = computed(() => logs.value.length > 0)

/**
 * Row view models, built once per fetch.
 *
 * The template needs the parsed detail in four places per row (message, facts,
 * refs, error) and again in the card view. Calling the parser inline would run
 * it eight times a row on every re-render, for twenty rows.
 */
const rows = computed(() =>
  logs.value.map((row) => ({
    row,
    tone: levelTone(row.level),
    detail: logDetail(row)
  }))
)

onMounted(async () => {
  if (props.platformFilter) {
    filters.platform = props.platformFilter
  }
  await Promise.all([fetchLogs(), fetchHealth(), loadRuntimeConfig()])
})
</script>

<template>
  <section class="syslog">
    <div class="syslog__head">
      <div>
        <h3 class="syslog__title">
          <i class="hgi-stroke hgi-inbox syslog__title-icon" aria-hidden="true" />
          {{ t('admin.ops.systemLogs.title') }}
        </h3>
        <p class="syslog__subtitle">{{ t('admin.ops.systemLogs.description') }}</p>
      </div>

      <!-- A counter at zero is the good outcome, so it stays neutral. Colour
           arrives when the number does. -->
      <div class="syslog__health">
        <span class="syslog__pill" :data-tone="queueTone">
          <span class="syslog__pill-key">{{ t('admin.ops.systemLogs.queue') }}</span>
          <span class="syslog__pill-value">{{ health.queue_depth }}/{{ health.queue_capacity }}</span>
        </span>
        <span class="syslog__pill">
          <span class="syslog__pill-key">{{ t('admin.ops.systemLogs.written') }}</span>
          <span class="syslog__pill-value">{{ health.written_count }}</span>
        </span>
        <span class="syslog__pill" :data-tone="health.dropped_count > 0 ? 'warning' : undefined">
          <span class="syslog__pill-key">{{ t('admin.ops.systemLogs.dropped') }}</span>
          <span class="syslog__pill-value">{{ health.dropped_count }}</span>
        </span>
        <span class="syslog__pill" :data-tone="health.write_failed_count > 0 ? 'critical' : undefined">
          <span class="syslog__pill-key">{{ t('admin.ops.systemLogs.failed') }}</span>
          <span class="syslog__pill-value">{{ health.write_failed_count }}</span>
        </span>
      </div>
    </div>

    <!-- Settings, not reading. Closed by default; the summary keeps the live
         values visible so the setting can be checked without opening it. -->
    <div class="syslog__config" :data-open="configOpen || undefined">
      <button
        type="button"
        class="syslog__config-toggle"
        :aria-expanded="configOpen"
        @click="configOpen = !configOpen"
      >
        <i class="hgi-stroke hgi-arrow-down-01 syslog__chev" aria-hidden="true" />
        <span class="syslog__config-title">{{ t('admin.ops.systemLogs.runtimeConfig') }}</span>
        <span class="syslog__config-summary">
          <span class="syslog__tag">{{ runtimeConfig.level }}</span>
          <span class="syslog__tag">
            {{ runtimeConfig.enable_sampling ? t('admin.ops.systemLogs.samplingOn') : t('admin.ops.systemLogs.samplingOff') }}
          </span>
          <span class="syslog__tag">
            {{ t('admin.ops.systemLogs.retentionSummary', { days: runtimeConfig.retention_days }) }}
          </span>
        </span>
        <LoadingSpinner v-if="runtimeLoading" size="sm" />
      </button>

      <div v-if="configOpen" class="syslog__config-body">
        <p class="syslog__config-hint">{{ t('admin.ops.systemLogs.runtimeConfigHint') }}</p>

        <div class="syslog__grid syslog__grid--config">
          <label class="syslog__field">
            <span class="syslog__field-label">{{ t('admin.ops.systemLogs.level') }}</span>
            <Select v-model="runtimeConfig.level" :options="runtimeLevelOptions" />
          </label>
          <label class="syslog__field">
            <span class="syslog__field-label">{{ t('admin.ops.systemLogs.stacktraceThreshold') }}</span>
            <Select v-model="runtimeConfig.stacktrace_level" :options="stacktraceLevelOptions" />
          </label>
          <Input
            v-model="drafts.sampling_initial"
            type="number"
            :label="t('admin.ops.systemLogs.samplingInitial')"
            @change="commitDraft('sampling_initial', 1)"
          />
          <Input
            v-model="drafts.sampling_thereafter"
            type="number"
            :label="t('admin.ops.systemLogs.samplingThereafter')"
            @change="commitDraft('sampling_thereafter', 1)"
          />
          <Input
            v-model="drafts.retention_days"
            type="number"
            :label="t('admin.ops.systemLogs.retentionDays')"
            @change="commitDraft('retention_days', 30)"
          />
        </div>

        <div class="syslog__config-foot">
          <div class="syslog__toggles">
            <label class="syslog__toggle">
              <Checkbox v-model="runtimeConfig.caller" />
              {{ t('admin.ops.systemLogs.caller') }}
            </label>
            <label class="syslog__toggle">
              <Checkbox v-model="runtimeConfig.enable_sampling" />
              {{ t('admin.ops.systemLogs.sampling') }}
            </label>
          </div>
          <div class="syslog__config-actions">
            <Button variant="solid" :disabled="runtimeSaving" :loading="runtimeSaving" @click="saveRuntimeConfig">
              {{ runtimeSaving ? t('common.saving') : t('admin.ops.systemLogs.saveAndApply') }}
            </Button>
            <Button variant="secondary" :disabled="runtimeSaving" @click="showResetConfirm = true">
              {{ t('admin.ops.systemLogs.resetDefaults') }}
            </Button>
          </div>
        </div>

        <p v-if="health.last_error" class="syslog__write-error">
          {{ t('admin.ops.systemLogs.latestWriteError') }} {{ health.last_error }}
        </p>
      </div>
    </div>

    <div class="syslog__grid syslog__grid--filters">
      <label class="syslog__field">
        <span class="syslog__field-label">{{ t('admin.ops.systemLogs.timeRange') }}</span>
        <Select v-model="filters.time_range" :options="timeRangeOptions" />
      </label>
      <Input v-model="filters.start_time" type="datetime-local" :label="t('admin.ops.systemLogs.startTime')" />
      <Input v-model="filters.end_time" type="datetime-local" :label="t('admin.ops.systemLogs.endTime')" />
      <label class="syslog__field">
        <span class="syslog__field-label">{{ t('admin.ops.systemLogs.level') }}</span>
        <Select v-model="filters.level" :options="filterLevelOptions" />
      </label>
      <Input
        v-model="filters.component"
        :label="t('admin.ops.systemLogs.component')"
        :placeholder="t('admin.ops.systemLogs.componentPlaceholder')"
      />
      <Input v-model="filters.host" :label="t('admin.ops.systemLogs.host')" />
      <Input v-model="filters.request_id" label="request_id" @enter="applyFilters" />
      <Input v-model="filters.client_request_id" label="client_request_id" @enter="applyFilters" />
      <Input v-model="filters.user_id" label="user_id" @enter="applyFilters" />
      <Input v-model="filters.api_key_id" :label="t('admin.ops.systemLogs.keyId')" @enter="applyFilters" />
      <Input v-model="filters.account_id" label="account_id" @enter="applyFilters" />
      <Input v-model="filters.platform" :label="t('admin.ops.systemLogs.platform')" @enter="applyFilters" />
      <Input v-model="filters.model" :label="t('admin.ops.systemLogs.model')" @enter="applyFilters" />
      <Input
        v-model="filters.q"
        :label="t('admin.ops.systemLogs.keyword')"
        :placeholder="t('admin.ops.systemLogs.keywordPlaceholder')"
        @enter="applyFilters"
      />
    </div>

    <div class="syslog__actions">
      <Button variant="solid" icon="hgi-search-01" @click="applyFilters">
        {{ t('admin.ops.systemLogs.search') }}
      </Button>
      <Button variant="secondary" @click="resetFilters">{{ t('common.reset') }}</Button>
      <Button variant="secondary" icon="hgi-refresh" @click="fetchHealth">
        {{ t('admin.ops.systemLogs.refreshHealth') }}
      </Button>
      <!-- Destructive, and pushed away from Search so a mis-aimed click on the
           common action cannot land on the irreversible one. -->
      <Button variant="danger" class="syslog__danger" @click="showCleanupConfirm = true">
        {{ t('admin.ops.systemLogs.cleanCurrentFilters') }}
      </Button>
    </div>

    <div class="syslog__frame">
      <div v-if="loading" class="syslog__state">
        <LoadingSpinner size="md" />
      </div>

      <div v-else-if="!hasData" class="syslog__state">
        <EmptyState
          icon="hgi-inbox"
          :title="t('admin.ops.systemLogs.empty')"
          :description="t('admin.ops.systemLogs.emptyHint')"
        />
      </div>

      <!-- Card view, under 768px -->
      <div v-else-if="!isDesktopViewport" class="syslog__cards">
        <div v-for="item in rows" :key="item.row.id" class="syslog__card">
          <div class="syslog__card-top">
            <span v-if="item.tone" class="syslog__level" :data-tone="item.tone">{{ item.row.level }}</span>
            <span v-else class="syslog__level-plain">{{ item.row.level }}</span>
            <span v-if="item.row.host" class="syslog__host" :title="item.row.host">{{ item.row.host }}</span>
            <span class="syslog__time syslog__time--push">{{ formatDateTime(item.row.created_at) }}</span>
          </div>
          <div class="syslog__detail">
            <div class="syslog__line">
              <span v-if="item.detail.message" class="syslog__message">{{ item.detail.message }}</span>
              <span v-for="f in item.detail.facts" :key="f.key" class="syslog__fact" :data-tone="f.tone">
                <span class="syslog__fact-key">{{ f.key }}</span>
                <span class="syslog__fact-value">{{ f.value }}</span>
              </span>
            </div>
            <div v-if="item.detail.refs.length" class="syslog__refs">
              <span v-for="r in item.detail.refs" :key="r.key" class="syslog__ref">
                <span class="syslog__ref-key">{{ r.key }}</span>
                <span class="syslog__ref-value">{{ r.value }}</span>
              </span>
            </div>
            <p v-if="item.detail.error" class="syslog__error">{{ item.detail.error }}</p>
          </div>
        </div>
      </div>

      <!-- Table view, 768px and up -->
      <div v-else class="syslog__scroll">
        <table class="syslog__table">
          <thead class="syslog__thead">
            <tr>
              <th class="syslog__th syslog__th--time">{{ t('admin.ops.systemLogs.time') }}</th>
              <th class="syslog__th syslog__th--host">{{ t('admin.ops.systemLogs.host') }}</th>
              <th class="syslog__th syslog__th--level">{{ t('admin.ops.systemLogs.level') }}</th>
              <th class="syslog__th">{{ t('admin.ops.systemLogs.logDetails') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in rows" :key="item.row.id" class="syslog__row">
              <td class="syslog__td syslog__td--time">{{ formatDateTime(item.row.created_at) }}</td>
              <td class="syslog__td">
                <span class="syslog__host" :title="item.row.host || '-'">{{ item.row.host || '-' }}</span>
              </td>
              <td class="syslog__td">
                <span v-if="item.tone" class="syslog__level" :data-tone="item.tone">{{ item.row.level }}</span>
                <span v-else class="syslog__level-plain">{{ item.row.level }}</span>
              </td>
              <td class="syslog__td">
                <div class="syslog__detail">
                  <div class="syslog__line">
                    <span v-if="item.detail.message" class="syslog__message">{{ item.detail.message }}</span>
                    <span v-for="f in item.detail.facts" :key="f.key" class="syslog__fact" :data-tone="f.tone">
                      <span class="syslog__fact-key">{{ f.key }}</span>
                      <span class="syslog__fact-value">{{ f.value }}</span>
                    </span>
                  </div>
                  <div v-if="item.detail.refs.length" class="syslog__refs">
                    <span v-for="r in item.detail.refs" :key="r.key" class="syslog__ref">
                      <span class="syslog__ref-key">{{ r.key }}</span>
                      <span class="syslog__ref-value">{{ r.value }}</span>
                    </span>
                  </div>
                  <p v-if="item.detail.error" class="syslog__error">{{ item.detail.error }}</p>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <Pagination
        :total="total"
        :page="page"
        :page-size="pageSize"
        @update:page="onPageChange"
        @update:page-size="onPageSizeChange"
      />
    </div>

    <ConfirmDialog
      :show="showResetConfirm"
      :title="t('admin.ops.systemLogs.resetConfigTitle')"
      :message="t('admin.ops.systemLogs.resetRuntimeConfigConfirm')"
      :confirm-text="t('admin.ops.systemLogs.resetDefaults')"
      @confirm="resetRuntimeConfig"
      @cancel="showResetConfirm = false"
    />

    <ConfirmDialog
      :show="showCleanupConfirm"
      :title="t('admin.ops.systemLogs.cleanupTitle')"
      :message="t('admin.ops.systemLogs.cleanupConfirm')"
      :confirm-text="t('admin.ops.systemLogs.cleanCurrentFilters')"
      danger
      @confirm="cleanupCurrentFilter"
      @cancel="showCleanupConfirm = false"
    />
  </section>
</template>

<style scoped>
.syslog {
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.syslog__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.syslog__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.syslog__title-icon {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.syslog__subtitle {
  margin: 3px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* --- sink health ------------------------------------------------------- */

.syslog__health {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.syslog__pill {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  padding: 3px 9px;
  border-radius: var(--r-sm);
  background: var(--surface-subtle);
  font-size: var(--fs-xs);
}

.syslog__pill-key {
  color: var(--muted-foreground);
}

.syslog__pill-value {
  color: var(--foreground);
  font-variant-numeric: tabular-nums;
}

/* Colour arrives with the number. Dropped and Failed were permanently amber
   and red at zero, which is an alarm for the good outcome. */
.syslog__pill[data-tone='warning'] {
  background: var(--s2a-attn-soft);
}
.syslog__pill[data-tone='warning'] .syslog__pill-key,
.syslog__pill[data-tone='warning'] .syslog__pill-value {
  color: var(--s2a-attn);
}

.syslog__pill[data-tone='critical'] {
  background: color-mix(in oklch, var(--destructive) 14%, var(--card));
}
.syslog__pill[data-tone='critical'] .syslog__pill-key,
.syslog__pill[data-tone='critical'] .syslog__pill-value {
  color: var(--destructive);
}

/* --- runtime config disclosure ----------------------------------------- */

.syslog__config {
  margin-bottom: 12px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--surface-subtle);
}

.syslog__config-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 9px 12px;
  border: 0;
  border-radius: var(--r-md);
  background: none;
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  text-align: left;
  cursor: pointer;
  transition: background var(--motion-hover);
}

.syslog__config-toggle:hover {
  background: var(--sidebar-accent);
}

.syslog__chev {
  flex: none;
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
  /* Rotation only; the border never moves (ground rule 6). */
  transition: transform var(--t-med) var(--ease-out);
  transform: rotate(-90deg);
}

.syslog__config[data-open] .syslog__chev {
  transform: rotate(0deg);
}

.syslog__config-title {
  flex: none;
}

/* The live values, readable without opening the panel. */
.syslog__config-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  margin-left: auto;
}

.syslog__tag {
  padding: 1px 7px;
  border-radius: var(--r-pill);
  background: var(--card);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  white-space: nowrap;
}

.syslog__config-body {
  padding: 0 12px 12px;
}

.syslog__config-hint {
  margin: 0 0 10px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.syslog__config-foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: 10px;
}

.syslog__toggles {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 16px;
}

.syslog__toggle {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--body-copy);
  font-size: var(--fs-md);
  cursor: pointer;
}

.syslog__config-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.syslog__write-error {
  margin: 10px 0 0;
  padding: 8px 10px;
  border-radius: var(--r-sm);
  background: color-mix(in oklch, var(--destructive) 10%, var(--card));
  color: var(--destructive);
  font-size: var(--fs-sm);
}

/* --- filters ----------------------------------------------------------- */

.syslog__grid {
  display: grid;
  gap: 10px;
}

.syslog__grid--config {
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
}

.syslog__grid--filters {
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
  margin-bottom: 12px;
}

/* Matches Input's own label so a Select in the grid lines up with the fields
   either side of it. */
.syslog__field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.syslog__field-label {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.syslog__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.syslog__danger {
  margin-left: auto;
}

/* --- table ------------------------------------------------------------- */

.syslog__frame {
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
}

.syslog__state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 0;
}

.syslog__scroll {
  overflow-x: auto;
  scrollbar-width: thin;
  scrollbar-color: var(--border) transparent;
}

.syslog__scroll::-webkit-scrollbar {
  height: 6px;
}

.syslog__scroll::-webkit-scrollbar-track {
  background: transparent;
}

.syslog__scroll::-webkit-scrollbar-thumb {
  background-color: var(--border);
  border-radius: 3px;
}

.syslog__table {
  width: 100%;
  table-layout: fixed;
  border-collapse: collapse;
}

.syslog__thead {
  background: var(--card);
}

/* Sentence case, --fs-xs, weight 400. Was `text-[11px] font-semibold`. */
.syslog__th {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
  font-weight: 400;
  text-align: left;
  white-space: nowrap;
}

.syslog__th--time {
  width: 118px;
}

.syslog__th--host {
  width: 130px;
}

.syslog__th--level {
  width: 76px;
}

.syslog__row + .syslog__row .syslog__td {
  border-top: 1px solid var(--border-subtle);
}

.syslog__td {
  padding: 9px 12px;
  color: var(--body-copy);
  font-size: var(--fs-sm);
  vertical-align: top;
}

.syslog__td--time {
  color: var(--muted-foreground);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.syslog__host {
  display: block;
  overflow: hidden;
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* --- level ------------------------------------------------------------- */

.syslog__level {
  display: inline-block;
  padding: 1px 8px;
  border-radius: var(--r-pill);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
}

.syslog__level[data-tone='error'] {
  background: color-mix(in oklch, var(--destructive) 15%, var(--card));
  color: var(--destructive);
}

.syslog__level[data-tone='warn'] {
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
}

/* info and debug: no chip. A badge on 7,724 of 7,739 rows marks nothing. */
.syslog__level-plain {
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

/* --- log detail -------------------------------------------------------- */

.syslog__detail {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

/* Message and access facts share one wrapping line: the message is short and
   repetitive, so it reads as the leading token rather than its own row. */
.syslog__line {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 6px;
}

.syslog__message {
  color: var(--foreground);
}

.syslog__fact {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  padding: 1px 6px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  font-size: var(--fs-2xs);
  max-width: 100%;
}

.syslog__fact-key {
  flex: none;
  color: var(--muted-foreground);
}

/* `anywhere`, not `break-all`. break-all splits every long run at the box edge
   including short ones; anywhere only breaks when there is no other option, so
   a path wraps at its limit instead of mid-token everywhere. */
.syslog__fact-value {
  color: var(--foreground);
  font-family: var(--font-mono);
  overflow-wrap: anywhere;
}

.syslog__fact[data-tone='bad'] {
  background: color-mix(in oklch, var(--destructive) 12%, var(--card));
}

.syslog__fact[data-tone='bad'] .syslog__fact-key,
.syslog__fact[data-tone='bad'] .syslog__fact-value {
  color: var(--destructive);
}

/* Correlation ids are for grepping, not reading. No chip, no fill, one step
   quieter than the facts above them. */
.syslog__refs {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 10px;
  font-size: var(--fs-2xs);
}

.syslog__ref {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  max-width: 100%;
}

.syslog__ref-key {
  flex: none;
  color: var(--muted-foreground);
}

.syslog__ref-value {
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  overflow-wrap: anywhere;
}

.syslog__error {
  margin: 2px 0 0;
  color: var(--destructive);
  font-size: var(--fs-xs);
  overflow-wrap: anywhere;
}

/* --- card view (under 768px) ------------------------------------------- */

.syslog__cards {
  display: flex;
  flex-direction: column;
}

.syslog__card {
  padding: 12px;
}

.syslog__card + .syslog__card {
  border-top: 1px solid var(--border-subtle);
}

.syslog__card-top {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.syslog__card .syslog__host {
  display: inline-block;
  max-width: 140px;
}

.syslog__time {
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
}

.syslog__time--push {
  margin-left: auto;
}
</style>
