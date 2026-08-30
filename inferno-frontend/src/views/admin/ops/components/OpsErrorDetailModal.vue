<script setup lang="ts">
/**
 * One failed request, in full.
 *
 * TWELVE CARDS BECOME A DEFINITION LIST
 * Every field here -- request id, platform, group, model, both endpoints --
 * used to be its own `rounded-xl bg-gray-50 p-4` tile in a four-column grid.
 * Card weight is a signal: it says "this is a distinct thing that deserves its
 * own surface". Spent on all twelve key/value pairs at once it signals nothing
 * and costs about 48px of chrome per fact. These are metadata, so they are a
 * `<dl>` now, which says the same thing in roughly a third of the height.
 *
 * THE MESSAGE WAS THE LEAST PROMINENT THING ON THE SCREEN
 * `detail.message` -- the actual error text, the reason anyone opens this
 * modal -- sat in a quarter-width tile under `truncate`, so the full text was
 * reachable only as a native `title` tooltip. `platform` got equal billing.
 * Message and status code are the header now; everything else is reference
 * material below them.
 *
 * EMPTY FIELDS KEEP THEIR ROW
 * A blank where a correlation id should be is itself a finding: it means the
 * request never got one and cannot be traced. So a missing value renders as
 * "not recorded" rather than dropping the row. (It also cannot render as an
 * em dash any more -- ground rule 2.)
 *
 * 429 WAS PURPLE
 * The status chip ramped red / purple / amber. Purple carries no meaning in
 * this palette. It follows the same convention as the system log now: 5xx is
 * destructive because the provider or we broke, any other 4xx including 429 is
 * attention because it is expected traffic being refused, and anything else is
 * neutral.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useAppStore } from '@/stores'
import { opsAPI, type OpsErrorDetail } from '@/api/admin/ops'
import { formatDateTime } from '@/utils/format'
import { resolveUpstreamPayload } from '../utils/errorDetailResponse'

interface Props {
  show: boolean
  errorId: number | null
  errorType?: 'request' | 'upstream'
  backToList?: boolean
}

interface Emits {
  (e: 'update:show', value: boolean): void
  (e: 'back'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()

function goBack() {
  emit('update:show', false)
  emit('back')
}
const appStore = useAppStore()

const loading = ref(false)
const detail = ref<OpsErrorDetail | null>(null)

const showUpstreamList = computed(() => props.errorType === 'request')

const requestId = computed(() => detail.value?.request_id || detail.value?.client_request_id || '')

type DiagnosticPayloadKey = 'client' | 'upstream_message' | 'upstream_detail' | 'upstream_events'

/* An error's real cause is whichever of these the upstream actually filled in.
   Preferring the first meaningful one stops the modal leading with a generic
   gateway message when a specific upstream reason exists. */
const rootCauseMessage = computed(() => {
  const current = detail.value
  if (!current) return ''
  for (const candidate of [current.upstream_error_message, current.upstream_error_detail, current.message, current.error_body]) {
    const value = meaningfulPayload(candidate)
    if (value) return value
  }
  return ''
})

/* Every payload worth showing, de-duplicated: upstreams often repeat the same
   string in two fields, and rendering it twice reads as two separate faults. */
const diagnosticPayloadSections = computed(() => {
  const current = detail.value
  if (!current) return []
  const candidates: Array<{ key: DiagnosticPayloadKey; value: string }> = [
    { key: 'client', value: meaningfulPayload(current.error_body) },
    { key: 'upstream_message', value: meaningfulPayload(current.upstream_error_message) },
    { key: 'upstream_detail', value: meaningfulPayload(current.upstream_error_detail) },
    { key: 'upstream_events', value: meaningfulPayload(current.upstream_errors) }
  ]
  return candidates.filter((section, index, all) => {
    return section.value && all.findIndex(candidate => candidate.value === section.value) === index
  })
})

function meaningfulPayload(candidate: unknown): string {
  const value = String(candidate || '').trim()
  if (!value || value === '[]' || value === '{}' || value.toLowerCase() === 'null') return ''
  return value
}

function diagnosticPayloadLabel(key: DiagnosticPayloadKey): string {
  return t(`admin.ops.errorDetail.payloads.${key}`)
}

const title = computed(() => {
  if (!props.errorId) return t('admin.ops.errorDetail.title')
  return t('admin.ops.errorDetail.titleWithId', { id: String(props.errorId) })
})

const emptyText = computed(() => t('admin.ops.errorDetail.noErrorSelected'))

function isUpstreamError(d: OpsErrorDetail | null): boolean {
  if (!d) return false
  const phase = String(d.phase || '').toLowerCase()
  const owner = String(d.error_owner || '').toLowerCase()
  return phase === 'upstream' && owner === 'provider'
}

function formatRequestTypeLabel(type: number | null | undefined): string {
  switch (type) {
    case 1: return t('admin.ops.errorDetail.requestTypeSync')
    case 2: return t('admin.ops.errorDetail.requestTypeStream')
    case 3: return t('admin.ops.errorDetail.requestTypeWs')
    default: return t('admin.ops.errorDetail.requestTypeUnknown')
  }
}

function hasModelMapping(d: OpsErrorDetail | null): boolean {
  if (!d) return false
  const requested = String(d.requested_model || '').trim()
  const upstream = String(d.upstream_model || '').trim()
  return !!requested && !!upstream && requested !== upstream
}

function displayModel(d: OpsErrorDetail | null): string {
  if (!d) return ''
  const upstream = String(d.upstream_model || '').trim()
  if (upstream) return upstream
  const requested = String(d.requested_model || '').trim()
  if (requested) return requested
  return String(d.model || '').trim()
}

/**
 * `from`/`to` is set only for the model row, which is the one field that is a
 * mapping rather than a value -- "you asked for X, we sent Y". Everything else
 * is a plain string.
 */
type MetaField = {
  key: string
  label: string
  value: string
  mono?: boolean
  from?: string
  to?: string
}

const metaFields = computed<MetaField[]>(() => {
  const d = detail.value
  if (!d) return []

  const upstream = isUpstreamError(d)

  const fields: MetaField[] = [
    { key: 'requestId', label: t('admin.ops.errorDetail.requestId'), value: requestId.value, mono: true },
    { key: 'time', label: t('admin.ops.errorDetail.time'), value: formatDateTime(d.created_at) },
    {
      key: 'principal',
      label: upstream ? t('admin.ops.errorDetail.account') : t('admin.ops.errorDetail.user'),
      value: upstream
        ? d.account_name || (d.account_id != null ? String(d.account_id) : '')
        : d.user_email || (d.user_id != null ? String(d.user_id) : '')
    },
    { key: 'platform', label: t('admin.ops.errorDetail.platform'), value: d.platform || '' },
    {
      key: 'group',
      label: t('admin.ops.errorDetail.group'),
      value: d.group_name || (d.group_id != null ? String(d.group_id) : '')
    },
    hasModelMapping(d)
      ? {
          key: 'model',
          label: t('admin.ops.errorDetail.model'),
          value: '',
          mono: true,
          from: String(d.requested_model || '').trim(),
          to: String(d.upstream_model || '').trim()
        }
      : { key: 'model', label: t('admin.ops.errorDetail.model'), value: displayModel(d), mono: true },
    {
      key: 'inbound',
      label: t('admin.ops.errorDetail.inboundEndpoint'),
      value: d.inbound_endpoint || '',
      mono: true
    },
    {
      key: 'upstreamEndpoint',
      label: t('admin.ops.errorDetail.upstreamEndpoint'),
      value: d.upstream_endpoint || '',
      mono: true
    },
    {
      key: 'requestType',
      label: t('admin.ops.errorDetail.requestType'),
      value: formatRequestTypeLabel(d.request_type)
    }
  ]

  /* Only present when the request carried a key at all, so an absent row here
     is not "not recorded", it is "not applicable". Stays conditional. */
  if (d.api_key_prefix) {
    fields.push({
      key: 'apiKeyPrefix',
      label: t('admin.ops.errorDetail.apiKeyPrefix'),
      value: d.api_key_prefix,
      mono: true
    })
  }

  return fields
})

const correlatedUpstream = ref<OpsErrorDetail[]>([])
const correlatedUpstreamLoading = ref(false)

const correlatedUpstreamErrors = computed<OpsErrorDetail[]>(() => correlatedUpstream.value)

const expandedUpstreamDetailIds = ref(new Set<number>())

function getUpstreamResponsePreview(ev: OpsErrorDetail): string {
  const upstreamPayload = resolveUpstreamPayload(ev)
  if (upstreamPayload) return upstreamPayload
  return String(ev.error_body || '').trim()
}

function toggleUpstreamDetail(id: number) {
  const next = new Set(expandedUpstreamDetailIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedUpstreamDetailIds.value = next
}

async function fetchCorrelatedUpstreamErrors(requestErrorId: number) {
  correlatedUpstreamLoading.value = true
  try {
    const res = await opsAPI.listRequestErrorUpstreamErrors(
      requestErrorId,
      { page: 1, page_size: 100, view: 'all' },
      { include_detail: true }
    )
    correlatedUpstream.value = res.items || []
  } catch (err) {
    console.error('[OpsErrorDetailModal] Failed to load correlated upstream errors', err)
    correlatedUpstream.value = []
  } finally {
    correlatedUpstreamLoading.value = false
  }
}

function close() {
  emit('update:show', false)
}

function prettyJSON(raw?: string): string {
  if (!raw) return 'N/A'
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

async function fetchDetail(id: number) {
  loading.value = true
  try {
    const kind = props.errorType || (detail.value?.phase === 'upstream' ? 'upstream' : 'request')
    const d = kind === 'upstream' ? await opsAPI.getUpstreamErrorDetail(id) : await opsAPI.getRequestErrorDetail(id)
    detail.value = d
  } catch (err: any) {
    detail.value = null
    appStore.showError(err?.message || t('admin.ops.failedToLoadErrorDetail'))
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.show, props.errorId] as const,
  ([show, id]) => {
    if (!show) {
      detail.value = null
      return
    }
    if (typeof id === 'number' && id > 0) {
      expandedUpstreamDetailIds.value = new Set()
      fetchDetail(id)
      if (props.errorType === 'request') {
        fetchCorrelatedUpstreamErrors(id)
      } else {
        correlatedUpstream.value = []
      }
    }
  },
  { immediate: true }
)

/* Same convention as the system log's level badge: destructive only when the
   thing that failed was the provider or us. A 4xx is the caller being told no,
   which is worth seeing but is not an alarm. */
const statusTone = computed(() => {
  const code = detail.value?.status_code ?? 0
  if (code >= 500) return 'error'
  if (code >= 400) return 'warn'
  return undefined
})

function upstreamStatusTone(code: number | null | undefined) {
  const n = code ?? 0
  if (n >= 500) return 'error'
  if (n >= 400) return 'warn'
  return undefined
}
</script>

<template>
  <BaseDialog :show="show" :title="title" width="full" :close-on-click-outside="true" @close="close">
    <div v-if="loading" class="errd__state">
      <LoadingSpinner size="md" />
      <p class="errd__state-text">{{ t('admin.ops.errorDetail.loading') }}</p>
    </div>

    <div v-else-if="!detail" class="errd__state errd__state--empty">
      {{ emptyText }}
    </div>

    <div v-else class="errd">
      <!-- The two things you opened this modal for. -->
      <div class="errd__head">
        <span class="errd__status" :data-tone="statusTone">{{ detail.status_code }}</span>
        <span
          v-if="detail.upstream_status_code != null"
          class="errd__status"
          :data-tone="upstreamStatusTone(detail.upstream_status_code)"
          :title="t('admin.ops.errorDetail.upstreamStatus')"
        >{{ detail.upstream_status_code }}</span>
        <p class="errd__message" :data-empty="rootCauseMessage ? undefined : true">
          {{ rootCauseMessage || t('common.notRecorded') }}
        </p>
      </div>

      <dl class="errd__meta">
        <div v-for="f in metaFields" :key="f.key" class="errd__field">
          <dt class="errd__label">{{ f.label }}</dt>
          <dd class="errd__value" :class="{ 'errd__value--mono': f.mono }">
            <template v-if="f.from">
              <span>{{ f.from }}</span>
              <span class="errd__arrow" aria-hidden="true">&rarr;</span>
              <span>{{ f.to }}</span>
            </template>
            <template v-else-if="f.value">{{ f.value }}</template>
            <span v-else class="errd__absent">{{ t('common.notRecorded') }}</span>
          </dd>
        </div>
      </dl>

      <!-- Every distinct payload the request produced, client and upstream. -->
      <section class="errd__section">
        <h3 class="errd__section-title">{{ t('admin.ops.errorDetail.diagnosticPayloads') }}</h3>
        <p v-if="!diagnosticPayloadSections.length" class="errd__empty">{{ t('common.noData') }}</p>
        <template v-else>
          <div v-for="section in diagnosticPayloadSections" :key="section.key" class="errd__payload">
            <h4 class="errd__payload-title">{{ diagnosticPayloadLabel(section.key) }}</h4>
            <pre class="errd__code"><code>{{ prettyJSON(section.value) }}</code></pre>
          </div>
        </template>
      </section>

      <!-- Upstream errors list (only for request errors) -->
      <section v-if="showUpstreamList" class="errd__section">
        <div class="errd__section-head">
          <h3 class="errd__section-title">{{ t('admin.ops.errorDetails.upstreamErrors') }}</h3>
          <span v-if="correlatedUpstreamLoading" class="errd__section-note">{{ t('common.loading') }}</span>
        </div>

        <p v-if="!correlatedUpstreamLoading && !correlatedUpstreamErrors.length" class="errd__section-note">
          {{ t('common.noData') }}
        </p>

        <div v-else class="errd__events">
          <article v-for="(ev, idx) in correlatedUpstreamErrors" :key="ev.id" class="errd__event">
            <div class="errd__event-head">
              <div class="errd__event-id">
                <span class="errd__event-index">#{{ idx + 1 }}</span>
                <span v-if="ev.type" class="errd__event-type">{{ ev.type }}</span>
              </div>
              <div class="errd__event-actions">
                <span class="errd__event-status" :data-tone="upstreamStatusTone(ev.status_code)">
                  <template v-if="ev.status_code != null">{{ ev.status_code }}</template>
                  <template v-else>{{ t('common.notRecorded') }}</template>
                </span>
                <button
                  type="button"
                  class="errd__toggle"
                  :disabled="!getUpstreamResponsePreview(ev)"
                  :title="getUpstreamResponsePreview(ev) ? '' : t('common.noData')"
                  :aria-expanded="expandedUpstreamDetailIds.has(ev.id)"
                  @click="toggleUpstreamDetail(ev.id)"
                >
                  <Icon
                    :name="expandedUpstreamDetailIds.has(ev.id) ? 'chevronDown' : 'chevronRight'"
                    size="xs"
                    :stroke-width="2"
                  />
                  <span>
                    {{
                      expandedUpstreamDetailIds.has(ev.id)
                        ? t('admin.ops.errorDetail.responsePreview.collapse')
                        : t('admin.ops.errorDetail.responsePreview.expand')
                    }}
                  </span>
                </button>
              </div>
            </div>

            <p v-if="ev.message" class="errd__event-message">{{ ev.message }}</p>

            <dl class="errd__event-meta">
              <div class="errd__field">
                <dt class="errd__label">{{ t('admin.ops.errorDetail.upstreamEvent.requestId') }}</dt>
                <dd class="errd__value errd__value--mono">
                  <template v-if="ev.request_id || ev.client_request_id">
                    {{ ev.request_id || ev.client_request_id }}
                  </template>
                  <span v-else class="errd__absent">{{ t('common.notRecorded') }}</span>
                </dd>
              </div>
            </dl>

            <pre
              v-if="expandedUpstreamDetailIds.has(ev.id)"
              class="errd__code errd__code--nested"
            ><code>{{ prettyJSON(getUpstreamResponsePreview(ev)) }}</code></pre>
          </article>
        </div>
      </section>
    </div>

    <!-- Opened from the error list: closing should hand the operator back to it
         with their filters intact, not drop them on the dashboard. -->
    <template v-if="backToList" #footer>
      <button
        type="button"
        class="btn btn-secondary"
        data-testid="error-detail-back-to-list"
        @click="goBack"
      >
        {{ t('admin.ops.errorDetail.backToList') }}
      </button>
    </template>
  </BaseDialog>
</template>

<style scoped>
.errd {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

/* --- states ------------------------------------------------------------- */

.errd__state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 56px 0;
}

.errd__state-text,
.errd__state--empty {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}

/* --- head: status + message --------------------------------------------- */

.errd__head {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.errd__status {
  flex: none;
  padding: 3px 9px;
  border-radius: var(--r-sm);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
  line-height: 1.5;
}

.errd__status[data-tone='error'] {
  background: color-mix(in oklch, var(--destructive) 15%, var(--card));
  color: var(--destructive);
}

.errd__status[data-tone='warn'] {
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
}

/* No truncation. An error message that does not fit is still the most useful
   text on the screen, and `anywhere` keeps an unbroken stack trace from
   pushing the dialog sideways. */
.errd__message {
  margin: 0;
  color: var(--foreground);
  /* --fs-xl against the meta list's --fs-sm. --fs-lg was only 14px to their
     12px, which is not enough separation to read as the headline it is. */
  font-size: var(--fs-xl);
  line-height: 1.35;
  overflow-wrap: anywhere;
}

.errd__message[data-empty] {
  color: var(--muted-foreground);
}

/* --- metadata ----------------------------------------------------------- */

.errd__meta {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 0 28px;
  margin: 0;
  padding-top: 14px;
  border-top: var(--border-width) solid var(--border-subtle);
}

.errd__field {
  display: grid;
  grid-template-columns: 108px minmax(0, 1fr);
  align-items: baseline;
  gap: 12px;
  padding: 4px 0;
}

.errd__label {
  min-width: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  overflow-wrap: anywhere;
}

.errd__value {
  min-width: 0;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-sm);
  overflow-wrap: anywhere;
}

.errd__value--mono {
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
}

.errd__arrow {
  margin: 0 5px;
  color: var(--muted-foreground);
}

.errd__absent {
  color: var(--muted-foreground);
  font-family: var(--font-sans);
  font-size: var(--fs-sm);
}

/* --- sections ----------------------------------------------------------- */

.errd__section {
  padding-top: 14px;
  border-top: var(--border-width) solid var(--border-subtle);
}

.errd__section-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.errd__section-title {
  margin: 0 0 10px;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
}

.errd__section-head .errd__section-title {
  margin-bottom: 10px;
}

.errd__section-note {
  margin: 0 0 10px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.errd__code {
  max-height: 460px;
  margin: 0;
  overflow: auto;
  padding: 12px 14px;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--surface-subtle);
  color: var(--body-copy);
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
  line-height: 1.55;
}

.errd__code--nested {
  max-height: 240px;
  margin-top: 10px;
  background: var(--card);
}

/* --- correlated upstream events ----------------------------------------- */

.errd__events {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.errd__event {
  padding: 12px 14px;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-md);
}

.errd__event-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.errd__event-id {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.errd__event-index {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}

.errd__event-type {
  padding: 2px 7px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  color: var(--body-copy);
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
}

.errd__event-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.errd__event-status {
  padding: 2px 7px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
}

.errd__event-status[data-tone='error'] {
  background: color-mix(in oklch, var(--destructive) 15%, var(--card));
  color: var(--destructive);
}

.errd__event-status[data-tone='warn'] {
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
}

.errd__toggle {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 7px;
  border: none;
  border-radius: var(--r-xs);
  background: transparent;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}

.errd__toggle:hover:not(:disabled) {
  background: var(--surface-hover);
  color: var(--foreground);
}

.errd__toggle:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.errd__toggle:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.errd__event-message {
  margin: 10px 0 0;
  color: var(--foreground);
  font-size: var(--fs-sm);
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.errd__event-meta {
  margin: 6px 0 0;
}
</style>
