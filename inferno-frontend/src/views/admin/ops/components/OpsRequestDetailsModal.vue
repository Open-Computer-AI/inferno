<script setup lang="ts">
/**
 * The requests behind a chart click, as a paged table.
 *
 * COLOUR WAS ON THE DEFAULT STATE, AND ON THE WRONG COLUMN
 * `kindBadgeClass` returned a green pill for anything that was not an error,
 * so a table that is mostly successful requests rendered a column of green.
 * Colour spent on the common case is not a signal, it is texture, and it stops
 * the rare case from standing out -- the same defect the system log had when
 * `info` painted a blue pill on 7,724 of its 7,739 rows.
 *
 * Worse, the tone was on the column that needed it least. `kind` and
 * `status_code` say nearly the same thing, but `kind` got the colour while the
 * status code -- where 503 and 429 and 400 genuinely differ -- was plain grey
 * text. The tone moved to the status code, which follows the same convention
 * as the error detail modal and the system log: >=500 destructive, >=400
 * attention, otherwise nothing.
 *
 * The kind column stays. It is not redundant in every case (a network failure
 * is `error` with no status code at all), and dropping a column to tidy a
 * layout is not this pass's call to make. It just no longer shouts on success.
 *
 * PLATFORM WAS SHOUTING
 * `(row.platform || 'unknown').toUpperCase()` rendered ANTHROPIC, OPENAI. An
 * all-caps transform is both harder to read and against ground rule 1.
 */
import { computed, ref, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores'
import { opsAPI, type OpsRequestDetailsParams, type OpsRequestDetail } from '@/api/admin/ops'
import { parseTimeRangeMinutes, formatDateTime } from '../utils/opsFormatters'

export interface OpsRequestDetailsPreset {
  title: string
  kind?: OpsRequestDetailsParams['kind']
  sort?: OpsRequestDetailsParams['sort']
  min_duration_ms?: number
  max_duration_ms?: number
}

interface Props {
  modelValue: boolean
  timeRange: string
  preset: OpsRequestDetailsPreset
  platform?: string
  groupId?: number | null
  resumeState?: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'openErrorDetail', errorId: number): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

// 与 DataTable 一致：< 768px 切换为卡片视图，避免宽表在移动端被截断。
const isDesktopViewport = useMediaQuery('(min-width: 768px)')

const loading = ref(false)
const items = ref<OpsRequestDetail[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const close = () => emit('update:modelValue', false)

/* Pluralised: a 60 minute window read "Window: 1 hours". Chinese carries a
   single form, which vue-i18n returns unchanged for any count. */
const rangeLabel = computed(() => {
  const minutes = parseTimeRangeMinutes(props.timeRange)
  if (minutes >= 60) {
    const hours = Math.round(minutes / 60)
    return t('admin.ops.requestDetails.rangeHours', hours, { named: { n: hours } })
  }
  return t('admin.ops.requestDetails.rangeMinutes', minutes, { named: { n: minutes } })
})

function buildTimeParams(): Pick<OpsRequestDetailsParams, 'start_time' | 'end_time'> {
  const minutes = parseTimeRangeMinutes(props.timeRange)
  const endTime = new Date()
  const startTime = new Date(endTime.getTime() - minutes * 60 * 1000)
  return {
    start_time: startTime.toISOString(),
    end_time: endTime.toISOString()
  }
}

const fetchData = async () => {
  if (!props.modelValue) return
  loading.value = true
  try {
    const params: OpsRequestDetailsParams = {
      ...buildTimeParams(),
      page: page.value,
      page_size: pageSize.value,
      kind: props.preset.kind ?? 'all',
      sort: props.preset.sort ?? 'created_at_desc'
    }

    const platform = (props.platform || '').trim()
    if (platform) params.platform = platform
    if (typeof props.groupId === 'number' && props.groupId > 0) params.group_id = props.groupId

    if (typeof props.preset.min_duration_ms === 'number') params.min_duration_ms = props.preset.min_duration_ms
    if (typeof props.preset.max_duration_ms === 'number') params.max_duration_ms = props.preset.max_duration_ms

    const res = await opsAPI.listRequestDetails(params)
    items.value = res.items || []
    total.value = res.total || 0
  } catch (e: any) {
    console.error('[OpsRequestDetailsModal] Failed to fetch request details', e)
    appStore.showError(e?.message || t('admin.ops.requestDetails.failedToLoad'))
    items.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      if (props.resumeState) return
      page.value = 1
      pageSize.value = 10
      fetchData()
    }
  }
)

watch(
  () => [
    props.timeRange,
    props.platform,
    props.groupId,
    props.preset.kind,
    props.preset.sort,
    props.preset.min_duration_ms,
    props.preset.max_duration_ms
  ],
  () => {
    if (!props.modelValue) return
    page.value = 1
    fetchData()
  }
)

function handlePageChange(next: number) {
  page.value = next
  fetchData()
}

function handlePageSizeChange(next: number) {
  pageSize.value = next
  page.value = 1
  fetchData()
}

async function handleCopyRequestId(requestId: string) {
  const ok = await copyToClipboard(requestId, t('admin.ops.requestDetails.requestIdCopied'))
  if (ok) return
  // `useClipboard` already shows toast on failure; this keeps UX consistent with older ops modal.
  appStore.showWarning(t('admin.ops.requestDetails.copyFailed'))
}

function openErrorDetail(errorId: number | null | undefined) {
  if (!errorId) return
  emit('openErrorDetail', errorId)
}

/* A real identifier beats the array index: paging replaces the whole array, and
   keying on position makes Vue patch row 1's DOM into row 1 of the next page. */
function rowKey(row: OpsRequestDetail, idx: number) {
  return row.request_id || (row.error_id != null ? `e${row.error_id}` : `i${idx}`)
}

/* Same ladder as OpsErrorDetailModal and the system log's level badge. A 4xx is
   the caller being refused, which is worth seeing but is not an alarm. */
function statusTone(code: number | null | undefined) {
  const n = code ?? 0
  if (n >= 500) return 'error'
  if (n >= 400) return 'warn'
  return undefined
}

function kindLabel(kind: string) {
  return kind === 'error'
    ? t('admin.ops.requestDetails.kind.error')
    : t('admin.ops.requestDetails.kind.success')
}

function durationLabel(ms: number | null | undefined) {
  return typeof ms === 'number' ? `${ms} ms` : '-'
}
</script>

<template>
  <BaseDialog :show="modelValue" :title="props.preset.title || t('admin.ops.requestDetails.title')" width="full" @close="close">
    <template #default>
      <div class="reqd">
        <div class="reqd__bar">
          <p class="reqd__range">{{ t('admin.ops.requestDetails.rangeLabel', { range: rangeLabel }) }}</p>
          <button type="button" class="btn btn-secondary btn-sm" @click="fetchData">
            {{ t('common.refresh') }}
          </button>
        </div>

        <!-- Loading -->
        <div v-if="loading" class="reqd__state">
          <LoadingSpinner size="md" />
          <p class="reqd__state-text">{{ t('common.loading') }}</p>
        </div>

        <!-- Table -->
        <div v-else class="reqd__body">
          <div v-if="items.length === 0" class="reqd__empty">
            <p class="reqd__empty-title">{{ t('admin.ops.requestDetails.empty') }}</p>
            <p class="reqd__empty-hint">{{ t('admin.ops.requestDetails.emptyHint') }}</p>
          </div>

          <div v-else class="reqd__frame">
            <div class="reqd__scroll">
              <!-- Card view below 768px: a wide table truncates badly there. -->
              <div v-if="!isDesktopViewport" class="reqd__cards">
                <article v-for="(row, idx) in items" :key="rowKey(row, idx)" class="reqd__card">
                  <div class="reqd__card-top">
                    <span v-if="row.kind === 'error'" class="reqd__kind" data-tone="error">{{ kindLabel(row.kind) }}</span>
                    <span v-else class="reqd__kind">{{ kindLabel(row.kind) }}</span>
                    <span class="reqd__platform">{{ row.platform || 'unknown' }}</span>
                    <span class="reqd__time">{{ formatDateTime(row.created_at) }}</span>
                  </div>

                  <p class="reqd__model">{{ row.model || '-' }}</p>

                  <div class="reqd__facts">
                    <span class="reqd__fact">{{ durationLabel(row.duration_ms) }}</span>
                    <span v-if="row.status_code != null" class="reqd__status" :data-tone="statusTone(row.status_code)">
                      {{ row.status_code }}
                    </span>
                    <span v-else class="reqd__absent">-</span>
                  </div>

                  <div v-if="row.request_id" class="reqd__rid">
                    <span class="reqd__rid-value" :title="row.request_id">{{ row.request_id }}</span>
                    <button type="button" class="reqd__copy" @click="handleCopyRequestId(row.request_id)">
                      {{ t('admin.ops.requestDetails.copy') }}
                    </button>
                  </div>

                  <button
                    v-if="row.kind === 'error' && row.error_id"
                    type="button"
                    class="reqd__view reqd__view--block"
                    @click="openErrorDetail(row.error_id)"
                  >
                    {{ t('admin.ops.requestDetails.viewError') }}
                  </button>
                </article>
              </div>

              <table v-else class="reqd__table">
                <thead>
                  <tr>
                    <th scope="col">{{ t('admin.ops.requestDetails.table.time') }}</th>
                    <th scope="col">{{ t('admin.ops.requestDetails.table.kind') }}</th>
                    <th scope="col">{{ t('admin.ops.requestDetails.table.platform') }}</th>
                    <th scope="col">{{ t('admin.ops.requestDetails.table.model') }}</th>
                    <th scope="col">{{ t('admin.ops.requestDetails.table.duration') }}</th>
                    <th scope="col">{{ t('admin.ops.requestDetails.table.status') }}</th>
                    <th scope="col">{{ t('admin.ops.requestDetails.table.requestId') }}</th>
                    <th scope="col" class="reqd__th--right">{{ t('admin.ops.requestDetails.table.actions') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(row, idx) in items" :key="rowKey(row, idx)">
                    <td class="reqd__td--nowrap">{{ formatDateTime(row.created_at) }}</td>
                    <td class="reqd__td--nowrap">
                      <span v-if="row.kind === 'error'" class="reqd__kind" data-tone="error">{{ kindLabel(row.kind) }}</span>
                      <span v-else class="reqd__kind">{{ kindLabel(row.kind) }}</span>
                    </td>
                    <td class="reqd__td--nowrap">
                      <span class="reqd__platform">{{ row.platform || 'unknown' }}</span>
                    </td>
                    <td class="reqd__td--model" :title="row.model || ''">{{ row.model || '-' }}</td>
                    <td class="reqd__td--nowrap reqd__td--num">{{ durationLabel(row.duration_ms) }}</td>
                    <td class="reqd__td--nowrap">
                      <!-- A chip implies a recorded value. A null status gets plain
                           text instead, or every success row shows an empty pill. -->
                      <span v-if="row.status_code != null" class="reqd__status" :data-tone="statusTone(row.status_code)">
                        {{ row.status_code }}
                      </span>
                      <span v-else class="reqd__absent">-</span>
                    </td>
                    <td>
                      <div v-if="row.request_id" class="reqd__rid">
                        <span class="reqd__rid-value" :title="row.request_id">{{ row.request_id }}</span>
                        <button type="button" class="reqd__copy" @click="handleCopyRequestId(row.request_id)">
                          {{ t('admin.ops.requestDetails.copy') }}
                        </button>
                      </div>
                      <span v-else class="reqd__absent">-</span>
                    </td>
                    <td class="reqd__td--nowrap reqd__td--right">
                      <button
                        v-if="row.kind === 'error' && row.error_id"
                        type="button"
                        class="reqd__view"
                        @click="openErrorDetail(row.error_id)"
                      >
                        {{ t('admin.ops.requestDetails.viewError') }}
                      </button>
                      <span v-else class="reqd__absent">-</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>

            <Pagination
              :total="total"
              :page="page"
              :page-size="pageSize"
              @update:page="handlePageChange"
              @update:pageSize="handlePageSizeChange"
            />
          </div>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<style scoped>
.reqd {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}

.reqd__bar {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.reqd__range {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* --- states ------------------------------------------------------------- */

.reqd__state {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 56px 0;
}

.reqd__state-text {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}

.reqd__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}

.reqd__empty {
  padding: 40px 24px;
  border: var(--border-width) dashed var(--border-subtle);
  border-radius: var(--r-md);
  text-align: center;
}

.reqd__empty-title {
  margin: 0;
  color: var(--body-copy);
  font-size: var(--fs-md);
}

.reqd__empty-hint {
  margin: 4px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* --- frame -------------------------------------------------------------- */

.reqd__frame {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-md);
}

.reqd__scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

/* --- table -------------------------------------------------------------- */

.reqd__table {
  width: 100%;
  border-collapse: collapse;
}

.reqd__table thead th {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: 9px 14px;
  border-bottom: var(--border-width) solid var(--border-subtle);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  text-align: left;
  white-space: nowrap;
}

.reqd__th--right {
  text-align: right;
}

.reqd__table tbody td {
  padding: 9px 14px;
  border-top: var(--border-width) solid var(--border-subtle);
  color: var(--body-copy);
  font-size: var(--fs-sm);
  vertical-align: middle;
}

.reqd__table tbody tr:first-child td {
  border-top: none;
}

.reqd__table tbody tr:hover td {
  background: var(--surface-hover);
}

.reqd__td--nowrap {
  white-space: nowrap;
}

.reqd__td--right {
  text-align: right;
}

.reqd__td--num {
  font-variant-numeric: tabular-nums;
}

.reqd__td--model {
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* --- cells -------------------------------------------------------------- */

/* Untoned by default. Success is the common case here and does not earn a
   colour; only `data-tone` paints one. */
.reqd__kind {
  display: inline-block;
  padding: 2px 8px;
  border-radius: var(--r-pill);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  white-space: nowrap;
}

.reqd__kind[data-tone='error'] {
  background: color-mix(in oklch, var(--destructive) 15%, var(--card));
  color: var(--destructive);
}

.reqd__platform {
  color: var(--body-copy);
  font-size: var(--fs-sm);
}

.reqd__status {
  display: inline-block;
  padding: 2px 7px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
}

.reqd__status[data-tone='error'] {
  background: color-mix(in oklch, var(--destructive) 15%, var(--card));
  color: var(--destructive);
}

.reqd__status[data-tone='warn'] {
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
}

.reqd__rid {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.reqd__rid-value {
  flex: 1;
  min-width: 0;
  max-width: 220px;
  overflow: hidden;
  color: var(--body-copy);
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.reqd__copy {
  flex: none;
  padding: 3px 8px;
  border: none;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}

.reqd__copy:hover {
  background: var(--surface-hover);
  color: var(--foreground);
}

.reqd__copy:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

/* A quiet control, not a filled red button on every error row. The status
   chip beside it already carries the severity. */
.reqd__view {
  padding: 3px 9px;
  border: none;
  border-radius: var(--r-xs);
  background: transparent;
  color: var(--destructive);
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: background var(--motion-hover);
}

.reqd__view:hover {
  background: color-mix(in oklch, var(--destructive) 10%, var(--card));
}

.reqd__view:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.reqd__view--block {
  width: 100%;
  margin-top: 4px;
  padding: 6px 9px;
  background: color-mix(in oklch, var(--destructive) 8%, var(--card));
}

.reqd__absent {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* --- card view (below 768px) -------------------------------------------- */

.reqd__cards {
  display: flex;
  flex-direction: column;
}

.reqd__card {
  display: flex;
  flex-direction: column;
  gap: 7px;
  padding: 14px;
  border-top: var(--border-width) solid var(--border-subtle);
}

.reqd__card:first-child {
  border-top: none;
}

.reqd__card-top {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.reqd__time {
  margin-left: auto;
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

.reqd__model {
  margin: 0;
  color: var(--body-copy);
  font-size: var(--fs-sm);
  overflow-wrap: anywhere;
}

.reqd__facts {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 12px;
}

.reqd__fact {
  color: var(--body-copy);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
</style>
