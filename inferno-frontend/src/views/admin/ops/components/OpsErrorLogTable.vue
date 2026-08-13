<template>
  <div class="errlog">
    <div class="errlog__frame" :class="flat ? '' : 'surface-card'">
      <IpGeoBatchToolbar :ips="rows.map((r) => r.client_ip)" @failed="emit('ipGeoBatchFailed')" />

      <DataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        clickable-rows
        server-side-sort
        default-sort-key="created_at"
        default-sort-order="desc"
        @sort="onSort"
        @rowClick="(row) => emit('openErrorDetail', row.id)"
      >
        <template #cell-created_at="{ row }">
          <span class="errlog__muted" :title="row.request_id || row.client_request_id">
            {{ formatDateTime(row.created_at) }}
          </span>
        </template>

        <!-- Type is a classification, not a severity. It used to ramp through six
             hues (red / amber / blue / orange / purple / grey) while the Category
             column right beside it -- the same kind of information -- was plain
             text. Both are neutral now; status carries the tone. -->
        <template #cell-type="{ row }">
          <span class="errlog__chip">{{ getTypeBadge(row).label }}</span>
        </template>

        <template #cell-endpoint="{ row }">
          <div class="errlog__endpoints">
            <p class="errlog__endpoint">
              <span class="errlog__endpoint-key">{{ t('usage.inbound') }}</span>
              <span class="errlog__endpoint-value">{{ row.inbound_endpoint?.trim() || '-' }}</span>
            </p>
            <p v-if="row.upstream_endpoint" class="errlog__endpoint">
              <span class="errlog__endpoint-key">{{ t('usage.upstream') }}</span>
              <span class="errlog__endpoint-value">{{ row.upstream_endpoint?.trim() || '-' }}</span>
            </p>
          </div>
        </template>

        <template #cell-platform="{ row }">
          <span class="errlog__text">{{ row.platform || '-' }}</span>
        </template>

        <template #cell-model="{ row }">
          <div v-if="hasModelMapping(row)" class="errlog__map">
            <span class="errlog__map-from">{{ row.requested_model }}</span>
            <span class="errlog__map-to">
              <span class="errlog__map-arrow" aria-hidden="true">&rarr;</span>{{ row.upstream_model }}
            </span>
          </div>
          <span v-else-if="displayModel(row)" class="errlog__text">{{ displayModel(row) }}</span>
          <span v-else class="errlog__absent">-</span>
        </template>

        <!-- A group name is an identity, not a state. It was an indigo pill. -->
        <template #cell-group="{ row }">
          <span
            v-if="row.group_id"
            class="errlog__chip"
            :title="t('admin.ops.errorLog.id') + ' ' + row.group_id"
          >{{ row.group_name || '#' + row.group_id }}</span>
          <span v-else class="errlog__absent">-</span>
        </template>

        <template #cell-user="{ row }">
          <div v-if="row.user_id" class="errlog__user">
            <button
              v-if="userClickable && row.user_email"
              type="button"
              class="errlog__link"
              :title="t('admin.usage.clickToViewBalance')"
              @click.stop="emit('userClick', row.user_id, row.user_email)"
            >{{ row.user_email }}</button>
            <span v-else class="errlog__text">{{ row.user_email || '-' }}</span>
            <span class="errlog__muted">#{{ row.user_id }}</span>
          </div>
          <span v-else class="errlog__absent">-</span>
        </template>

        <template #cell-api_key="{ row }">
          <div v-if="row.api_key_id || row.api_key_name" class="errlog__user">
            <span class="errlog__text">{{ row.api_key_name || '#' + row.api_key_id }}</span>
            <!-- A deleted key IS a state, and one that explains why a request
                 failed, so this keeps a tone where the identity chips lost theirs. -->
            <span v-if="row.api_key_deleted" class="errlog__chip" data-tone="warn">
              {{ t('admin.ops.errorLog.keyDeletedBadge') }}
            </span>
          </div>
          <span v-else class="errlog__absent">-</span>
        </template>

        <template #cell-account="{ row }">
          <span
            v-if="row.account_id"
            class="errlog__text"
            :title="t('admin.ops.errorLog.accountId') + ' ' + row.account_id"
          >{{ row.account_name || '#' + row.account_id }}</span>
          <span v-else class="errlog__absent">-</span>
        </template>

        <template #cell-category="{ row }">
          <span class="errlog__text">
            {{ t('usage.errors.categories.' + mapErrorCategory(row.phase, row.type)) }}
          </span>
        </template>

        <template #cell-status="{ row }">
          <div class="errlog__status-cell">
            <span class="errlog__status" :data-tone="statusTone(row.status_code)">{{ row.status_code }}</span>
            <!-- Neutral. These are historical rows, not open alerts; the alert
                 events card is where the P0..P3 ramp earns its colour. -->
            <span v-if="row.severity" class="errlog__sev">{{ row.severity }}</span>
            <span v-if="row.request_type != null && row.request_type > 0" class="errlog__chip">
              {{ formatRequestType(row.request_type) }}
            </span>
          </div>
        </template>

        <template #cell-message="{ row }">
          <span v-if="row.message" class="errlog__message" :title="row.message">
            {{ formatSmartMessage(row.message) || '-' }}
          </span>
          <span v-else class="errlog__absent">-</span>
        </template>

        <template #cell-user_agent="{ row }">
          <span v-if="row.user_agent" class="errlog__ua" :title="row.user_agent">{{ row.user_agent }}</span>
          <span v-else class="errlog__absent">-</span>
        </template>

        <template #cell-client_ip="{ row }">
          <div @click.stop>
            <div v-if="row.client_ip">
              <span class="errlog__ip">{{ row.client_ip }}</span>
              <IpGeoCell :ip="row.client_ip" />
            </div>
            <span v-else class="errlog__absent">-</span>
          </div>
        </template>

        <template #cell-actions="{ row }">
          <button
            type="button"
            class="errlog__action"
            :title="t('admin.ops.errorLog.details')"
            :aria-label="t('admin.ops.errorLog.details')"
            @click.stop="emit('openErrorDetail', row.id)"
          >
            <Icon name="document" size="sm" :stroke-width="2" />
          </button>
        </template>

        <template #empty><EmptyState :message="t('admin.ops.errorLog.noErrors')" /></template>
      </DataTable>
    </div>

    <div class="errlog__foot">
      <Pagination
        v-if="total > 0"
        :total="total"
        :page="page"
        :page-size="pageSize"
        @update:page="emit('update:page', $event)"
        @update:pageSize="emit('update:pageSize', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import IpGeoCell from '@/components/common/IpGeoCell.vue'
import IpGeoBatchToolbar from '@/components/common/IpGeoBatchToolbar.vue'
import Icon from '@/components/icons/Icon.vue'
import type { OpsErrorLog } from '@/api/admin/ops'
import type { Column } from '@/components/common/types'
import { formatDateTime } from '../utils/opsFormatters'
import { mapErrorCategory } from '@/utils/errorCategory'
import { mapErrorSortKey } from '@/utils/errorBadges'

const { t } = useI18n()

// 列序对齐管理端用量明细:身份(用户→Key→账号)→ 请求形态(平台→模型→端点→分组→类型)
// → 结果(状态→消息)→ 时间→UA→IP→操作
const allColumns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.ops.errorLog.user') },
  { key: 'api_key', label: t('admin.ops.errorLog.apiKey') },
  { key: 'account', label: t('admin.ops.errorLog.account') },
  { key: 'platform', label: t('admin.ops.errorLog.platform') },
  { key: 'model', label: t('admin.ops.errorLog.model'), sortable: true },
  { key: 'endpoint', label: t('admin.ops.errorLog.endpoint') },
  { key: 'group', label: t('admin.ops.errorLog.group') },
  { key: 'type', label: t('admin.ops.errorLog.type') },
  { key: 'category', label: t('usage.errors.category') },
  { key: 'status', label: t('admin.ops.errorLog.status'), sortable: true },
  { key: 'message', label: t('admin.ops.errorLog.message') },
  { key: 'created_at', label: t('admin.ops.errorLog.time'), sortable: true },
  { key: 'user_agent', label: t('usage.userAgent') },
  { key: 'client_ip', label: t('admin.ops.errorLog.ip') },
  { key: 'actions', label: t('admin.ops.errorLog.action') },
])

// 传入 visibleColumnKeys 时按其过滤(列设置);未传则全量(Ops 弹窗等使用方)
const columns = computed<Column[]>(() =>
  props.visibleColumnKeys
    ? allColumns.value.filter((c) => props.visibleColumnKeys!.includes(c.key))
    : allColumns.value
)

function isUpstreamRow(log: OpsErrorLog): boolean {
  const phase = String(log.phase || '').toLowerCase()
  const owner = String(log.error_owner || '').toLowerCase()
  return phase === 'upstream' && owner === 'provider'
}

function hasModelMapping(log: OpsErrorLog): boolean {
  const requested = String(log.requested_model || '').trim()
  const upstream = String(log.upstream_model || '').trim()
  return !!requested && !!upstream && requested !== upstream
}

function displayModel(log: OpsErrorLog): string {
  const upstream = String(log.upstream_model || '').trim()
  if (upstream) return upstream
  const requested = String(log.requested_model || '').trim()
  if (requested) return requested
  return String(log.model || '').trim()
}

function formatRequestType(type: number | null | undefined): string {
  switch (type) {
    case 1: return t('admin.ops.errorLog.requestTypeSync')
    case 2: return t('admin.ops.errorLog.requestTypeStream')
    case 3: return t('admin.ops.errorLog.requestTypeWs')
    default: return ''
  }
}

/*
 * Label only. This used to return a `className` too, ramping through six hues
 * -- red for upstream, amber for request, blue for auth, orange for account
 * auth, purple for routing, grey for internal -- which is a categorical
 * rainbow, and it sat next to a Category column carrying the same class of
 * information as plain text. Colour on this table now means severity and
 * nothing else.
 */
function getTypeBadge(log: OpsErrorLog): { label: string } {
  const phase = String(log.phase || '').toLowerCase()
  const owner = String(log.error_owner || '').toLowerCase()

  if (isUpstreamRow(log)) return { label: t('admin.ops.errorLog.typeUpstream') }
  if (phase === 'request' && owner === 'client') return { label: t('admin.ops.errorLog.typeRequest') }
  if (phase === 'auth' && owner === 'client') return { label: t('admin.ops.errorLog.typeAuth') }
  if (phase === 'account_auth') return { label: t('admin.ops.errorLog.typeAccountAuth') }
  if (phase === 'routing' && owner === 'platform') return { label: t('admin.ops.errorLog.typeRouting') }
  if (phase === 'internal' && owner === 'platform') return { label: t('admin.ops.errorLog.typeInternal') }

  return { label: phase || owner || t('common.unknown') }
}

interface Props {
  rows: OpsErrorLog[]
  total: number
  loading: boolean
  page: number
  pageSize: number
  /** 用户邮箱可点击(emit userClick),仅在有弹窗承接的使用方开启 */
  userClickable?: boolean
  /** 列设置:仅显示这些 key 的列;不传则全量 */
  visibleColumnKeys?: string[]
  /** 嵌入统一卡片内使用：去掉自身卡片外观 */
  flat?: boolean
}

interface Emits {
  (e: 'openErrorDetail', id: number): void
  (e: 'update:page', value: number): void
  (e: 'update:pageSize', value: number): void
  (e: 'ipGeoBatchFailed'): void
  (e: 'sort', sortBy: string, sortOrder: 'asc' | 'desc'): void
  (e: 'userClick', userId: number, email?: string): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

function onSort(key: string, order: 'asc' | 'desc') {
  emit('sort', mapErrorSortKey(key), order)
}

/* Local rather than the shared `statusCodeBadgeClass`, which is still on the
   old bg-X-100 palette and still used by the unconverted UserErrorRequestsTable.
   Same ladder as OpsErrorDetailModal and OpsRequestDetailsModal: >=500 is
   destructive because the provider or we broke, any other 4xx is attention
   because it is expected traffic being refused. */
function statusTone(code: number | null | undefined) {
  const n = code ?? 0
  if (n >= 500) return 'error'
  if (n >= 400) return 'warn'
  return undefined
}

function formatSmartMessage(msg: string): string {
  if (!msg) return ''

  if (msg.startsWith('{') || msg.startsWith('[')) {
    try {
      const obj = JSON.parse(msg)
      if (obj?.error?.message) return String(obj.error.message)
      if (obj?.message) return String(obj.message)
      if (obj?.detail) return String(obj.detail)
      if (typeof obj === 'object') return JSON.stringify(obj).substring(0, 150)
    } catch {
      // ignore parse error
    }
  }

  if (msg.includes('context deadline exceeded')) return t('admin.ops.errorLog.commonErrors.contextDeadlineExceeded')
  if (msg.includes('connection refused')) return t('admin.ops.errorLog.commonErrors.connectionRefused')
  if (msg.toLowerCase().includes('rate limit')) return t('admin.ops.errorLog.commonErrors.rateLimit')

  return msg.length > 200 ? msg.substring(0, 200) + '...' : msg
}
</script>

<style scoped>
/* DataTable owns cell padding, borders and the header row. Everything here
   styles cell CONTENT only, which is why there is no table geometry below. */
.errlog {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}

.errlog__frame {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.errlog__foot {
  flex: none;
}

/* --- shared cell text --------------------------------------------------- */

.errlog__text {
  color: var(--foreground);
  font-size: var(--fs-sm);
}

.errlog__muted {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.errlog__absent {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* --- neutral chip: type, group, request type ---------------------------- */

.errlog__chip {
  display: inline-block;
  padding: 2px 7px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  color: var(--body-copy);
  font-size: var(--fs-2xs);
  white-space: nowrap;
}

.errlog__chip[data-tone='warn'] {
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
}

/* --- status ------------------------------------------------------------- */

.errlog__status-cell {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.errlog__status {
  display: inline-block;
  padding: 2px 7px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
}

.errlog__status[data-tone='error'] {
  background: color-mix(in oklch, var(--destructive) 15%, var(--card));
  color: var(--destructive);
}

.errlog__status[data-tone='warn'] {
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
}

.errlog__sev {
  display: inline-block;
  padding: 2px 6px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
}

/* --- endpoints ---------------------------------------------------------- */

.errlog__endpoints {
  display: flex;
  flex-direction: column;
  gap: 3px;
  max-width: 320px;
}

.errlog__endpoint {
  margin: 0;
  font-size: var(--fs-xs);
  overflow-wrap: anywhere;
}

.errlog__endpoint-key {
  margin-right: 5px;
  color: var(--muted-foreground);
}

.errlog__endpoint-value {
  color: var(--body-copy);
  font-family: var(--font-mono);
}

/* --- model mapping ------------------------------------------------------ */

.errlog__map {
  display: flex;
  flex-direction: column;
  gap: 1px;
  font-size: var(--fs-xs);
}

.errlog__map-from {
  color: var(--foreground);
  overflow-wrap: anywhere;
}

.errlog__map-to {
  color: var(--muted-foreground);
  overflow-wrap: anywhere;
}

.errlog__map-arrow {
  margin-right: 4px;
}

/* --- user / key --------------------------------------------------------- */

.errlog__user {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 5px;
  font-size: var(--fs-sm);
}

/* A link that reads as a link without borrowing a colour the palette does not
   have. It was text-primary-600 with a dashed underline. */
.errlog__link {
  padding: 0;
  border: none;
  background: none;
  color: var(--foreground);
  font-size: var(--fs-sm);
  text-decoration: underline;
  text-decoration-color: var(--border-subtle);
  text-underline-offset: 2px;
  cursor: pointer;
  transition: text-decoration-color var(--motion-hover);
}

.errlog__link:hover {
  text-decoration-color: var(--muted-foreground);
}

.errlog__link:focus-visible {
  outline: none;
  border-radius: var(--r-xs);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

/* --- message / ua / ip -------------------------------------------------- */

.errlog__message {
  display: block;
  max-width: 280px;
  overflow: hidden;
  color: var(--body-copy);
  font-size: var(--fs-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.errlog__ua {
  display: block;
  max-width: 320px;
  overflow: hidden;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.errlog__ip {
  color: var(--body-copy);
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
}

/* --- row action --------------------------------------------------------- */

.errlog__action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px;
  border: none;
  border-radius: var(--r-xs);
  background: transparent;
  color: var(--muted-foreground);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}

.errlog__action:hover {
  background: var(--surface-hover);
  color: var(--foreground);
}

.errlog__action:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
</style>
