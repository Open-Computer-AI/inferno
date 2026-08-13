<script setup lang="ts">
/**
 * Per-model OpenAI throughput.
 *
 * SIX NUMERIC COLUMNS, ALL LEFT ALIGNED WITH PROPORTIONAL FIGURES
 * The point of this table is comparing magnitudes down a column, and that only
 * works when the digits line up: right alignment plus tabular-nums. Without
 * both, "1,234" and "987" start at the same x and the eye cannot rank them.
 *
 * TWO MILLISECOND COLUMNS, FORMATTED TWO WAYS
 * `avg_duration_ms` went through formatInt and `avg_first_token_ms` through
 * formatRate, so adjacent columns of the same unit read "245" and "245.00".
 * Two decimals on a millisecond measure is false precision. Both are integers
 * now; formatRate is kept for tokens/sec, where the fraction is real.
 */
import { computed, ref, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { opsAPI, type OpsOpenAITokenStatsResponse, type OpsOpenAITokenStatsTimeRange } from '@/api/admin/ops'
import { formatNumber } from '@/utils/format'

interface Props {
  platformFilter?: string
  groupIdFilter?: number | null
  refreshToken: number
}

type ViewMode = 'topn' | 'pagination'

const props = withDefaults(defineProps<Props>(), {
  platformFilter: '',
  groupIdFilter: null
})

const { t } = useI18n()

// 与 DataTable 一致：< 768px 切换为卡片视图，避免宽表在移动端被截断。
const isDesktopViewport = useMediaQuery('(min-width: 768px)')

const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsOpenAITokenStatsResponse | null>(null)

const timeRange = ref<OpsOpenAITokenStatsTimeRange>('30d')
const viewMode = ref<ViewMode>('topn')
const topN = ref<number>(20)
const page = ref<number>(1)
const pageSize = ref<number>(20)

const items = computed(() => response.value?.items ?? [])
const total = computed(() => response.value?.total ?? 0)
const totalPages = computed(() => {
  if (viewMode.value !== 'pagination') return 1
  const size = pageSize.value > 0 ? pageSize.value : 20
  return Math.max(1, Math.ceil(total.value / size))
})

const timeRangeOptions = computed(() => [
  { value: '30m', label: t('admin.ops.timeRange.30m') },
  { value: '1h', label: t('admin.ops.timeRange.1h') },
  { value: '1d', label: t('admin.ops.timeRange.1d') },
  { value: '15d', label: t('admin.ops.timeRange.15d') },
  { value: '30d', label: t('admin.ops.timeRange.30d') }
])

const viewModeOptions = computed(() => [
  { value: 'topn', label: t('admin.ops.openaiTokenStats.viewModeTopN') },
  { value: 'pagination', label: t('admin.ops.openaiTokenStats.viewModePagination') }
])

const topNOptions = computed(() => [
  { value: 10, label: 'Top 10' },
  { value: 20, label: 'Top 20' },
  { value: 50, label: 'Top 50' },
  { value: 100, label: 'Top 100' }
])

const pageSizeOptions = computed(() => [
  { value: 10, label: '10' },
  { value: 20, label: '20' },
  { value: 50, label: '50' },
  { value: 100, label: '100' }
])

/* Two decimals, for a rate where the fraction carries information. */
function formatRate(v?: number | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return v.toFixed(2)
}

function formatInt(v?: number | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return formatNumber(Math.round(v))
}

function buildParams() {
  const params: Record<string, any> = {
    time_range: timeRange.value,
    platform: props.platformFilter || undefined,
    group_id: typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0 ? props.groupIdFilter : undefined
  }

  if (viewMode.value === 'topn') {
    params.top_n = topN.value
  } else {
    params.page = page.value
    params.page_size = pageSize.value
  }
  return params
}

async function loadData() {
  loading.value = true
  errorMessage.value = ''
  try {
    response.value = await opsAPI.getOpenAITokenStats(buildParams())
    // 防御：若 total 变化导致当前页超出最大页，则回退到末页并重新拉取一次。
    if (viewMode.value === 'pagination' && page.value > totalPages.value) {
      page.value = totalPages.value
      response.value = await opsAPI.getOpenAITokenStats(buildParams())
    }
  } catch (err: any) {
    console.error('[OpsOpenAITokenStatsCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.openaiTokenStats.failedToLoad')
  } finally {
    loading.value = false
  }
}

watch(
  () => ({
    timeRange: timeRange.value,
    viewMode: viewMode.value,
    topN: topN.value,
    page: page.value,
    pageSize: pageSize.value,
    platform: props.platformFilter,
    groupId: props.groupIdFilter,
    refreshToken: props.refreshToken
  }),
  (next, prev) => {
    // 避免“筛选变化 -> 重置页码 -> 触发两次请求”：
    // 先只重置页码，等待下一次 watch（仅 page 变化）再发起请求。
    const filtersChanged = !prev ||
      next.timeRange !== prev.timeRange ||
      next.viewMode !== prev.viewMode ||
      next.pageSize !== prev.pageSize ||
      next.platform !== prev.platform ||
      next.groupId !== prev.groupId

    if (next.viewMode === 'pagination' && filtersChanged && next.page !== 1) {
      page.value = 1
      return
    }

    void loadData()
  },
  { immediate: true }
)

function onPrevPage() {
  if (viewMode.value !== 'pagination') return
  if (page.value > 1) page.value -= 1
}

function onNextPage() {
  if (viewMode.value !== 'pagination') return
  if (page.value < totalPages.value) page.value += 1
}
</script>

<template>
  <section class="oaits">
    <div class="oaits__bar">
      <h3 class="oaits__title">{{ t('admin.ops.openaiTokenStats.title') }}</h3>
      <div class="oaits__controls">
        <div class="oaits__control oaits__control--wide">
          <Select v-model="timeRange" :options="timeRangeOptions" />
        </div>
        <div class="oaits__control oaits__control--wide">
          <Select v-model="viewMode" :options="viewModeOptions" />
        </div>
        <div v-if="viewMode === 'topn'" class="oaits__control">
          <Select v-model="topN" :options="topNOptions" />
        </div>
        <template v-else>
          <div class="oaits__control oaits__control--narrow">
            <Select v-model="pageSize" :options="pageSizeOptions" />
          </div>
          <button class="btn btn-secondary btn-sm" :disabled="loading || page <= 1" @click="onPrevPage">
            {{ t('admin.ops.openaiTokenStats.prevPage') }}
          </button>
          <button class="btn btn-secondary btn-sm" :disabled="loading || page >= totalPages" @click="onNextPage">
            {{ t('admin.ops.openaiTokenStats.nextPage') }}
          </button>
          <span class="oaits__page-info">
            {{ t('admin.ops.openaiTokenStats.pageInfo', { page, total: totalPages }) }}
          </span>
        </template>
      </div>
    </div>

    <p v-if="errorMessage" class="oaits__error">{{ errorMessage }}</p>

    <p v-if="loading" class="oaits__state">{{ t('admin.ops.loadingText') }}</p>

    <EmptyState
      v-else-if="items.length === 0"
      :title="t('common.noData')"
      :description="t('admin.ops.openaiTokenStats.empty')"
    />

    <div v-else class="oaits__body">
      <div class="oaits__frame">
        <div class="oaits__scroll">
          <!-- Card view below 768px: a seven column table truncates badly there. -->
          <div v-if="!isDesktopViewport" class="oaits__cards">
            <article v-for="row in items" :key="row.model" class="oaits__card">
              <p class="oaits__model">{{ row.model }}</p>
              <dl class="oaits__facts">
                <div class="oaits__fact">
                  <dt>{{ t('admin.ops.openaiTokenStats.table.requestCount') }}</dt>
                  <dd>{{ formatInt(row.request_count) }}</dd>
                </div>
                <div class="oaits__fact">
                  <dt>{{ t('admin.ops.openaiTokenStats.table.avgTokensPerSec') }}</dt>
                  <dd>{{ formatRate(row.avg_tokens_per_sec) }}</dd>
                </div>
                <div class="oaits__fact">
                  <dt>{{ t('admin.ops.openaiTokenStats.table.avgFirstTokenMs') }}</dt>
                  <dd>{{ formatInt(row.avg_first_token_ms) }}</dd>
                </div>
                <div class="oaits__fact">
                  <dt>{{ t('admin.ops.openaiTokenStats.table.totalOutputTokens') }}</dt>
                  <dd>{{ formatInt(row.total_output_tokens) }}</dd>
                </div>
                <div class="oaits__fact">
                  <dt>{{ t('admin.ops.openaiTokenStats.table.avgDurationMs') }}</dt>
                  <dd>{{ formatInt(row.avg_duration_ms) }}</dd>
                </div>
                <div class="oaits__fact">
                  <dt>{{ t('admin.ops.openaiTokenStats.table.requestsWithFirstToken') }}</dt>
                  <dd>{{ formatInt(row.requests_with_first_token) }}</dd>
                </div>
              </dl>
            </article>
          </div>

          <table v-else class="oaits__table">
            <thead>
              <tr>
                <th scope="col">{{ t('admin.ops.openaiTokenStats.table.model') }}</th>
                <th scope="col" class="oaits__th--num">{{ t('admin.ops.openaiTokenStats.table.requestCount') }}</th>
                <th scope="col" class="oaits__th--num">{{ t('admin.ops.openaiTokenStats.table.avgTokensPerSec') }}</th>
                <th scope="col" class="oaits__th--num">{{ t('admin.ops.openaiTokenStats.table.avgFirstTokenMs') }}</th>
                <th scope="col" class="oaits__th--num">{{ t('admin.ops.openaiTokenStats.table.totalOutputTokens') }}</th>
                <th scope="col" class="oaits__th--num">{{ t('admin.ops.openaiTokenStats.table.avgDurationMs') }}</th>
                <th scope="col" class="oaits__th--num">{{ t('admin.ops.openaiTokenStats.table.requestsWithFirstToken') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in items" :key="row.model">
                <td class="oaits__td--model">{{ row.model }}</td>
                <td class="oaits__td--num">{{ formatInt(row.request_count) }}</td>
                <td class="oaits__td--num">{{ formatRate(row.avg_tokens_per_sec) }}</td>
                <td class="oaits__td--num">{{ formatInt(row.avg_first_token_ms) }}</td>
                <td class="oaits__td--num">{{ formatInt(row.total_output_tokens) }}</td>
                <td class="oaits__td--num">{{ formatInt(row.avg_duration_ms) }}</td>
                <td class="oaits__td--num">{{ formatInt(row.requests_with_first_token) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <p v-if="viewMode === 'topn'" class="oaits__total">
        {{ t('admin.ops.openaiTokenStats.totalModels', { total }) }}
      </p>
    </div>
  </section>
</template>

<style scoped>
.oaits {
  padding: 16px;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.oaits__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.oaits__title {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.oaits__controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.oaits__control {
  width: 112px;
}

.oaits__control--wide {
  width: 144px;
}

.oaits__control--narrow {
  width: 96px;
}

.oaits__page-info {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}

/* --- states ------------------------------------------------------------- */

/* Destructive: the fetch actually failed, which is the case this tone is for. */
.oaits__error {
  margin: 0 0 14px;
  padding: 8px 12px;
  border-radius: var(--r-md);
  background: color-mix(in oklch, var(--destructive) 12%, var(--card));
  color: var(--destructive);
  font-size: var(--fs-sm);
}

.oaits__state {
  margin: 0;
  padding: 32px 0;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  text-align: center;
}

.oaits__body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.oaits__frame {
  overflow: hidden;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-md);
}

.oaits__scroll {
  max-height: 420px;
  overflow: auto;
}

.oaits__total {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* --- table -------------------------------------------------------------- */

.oaits__table {
  width: 100%;
  border-collapse: collapse;
}

.oaits__table thead th {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: 8px 12px;
  border-bottom: var(--border-width) solid var(--border-subtle);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  text-align: left;
  white-space: nowrap;
}

/* Qualified with the element selector deliberately: the base rule above is
   `.oaits__table thead th`, which is (0,1,2), so a bare `.oaits__th--num` at
   (0,1,0) loses and the numeric headers stay left aligned over right aligned
   data. Right aligned so the label sits over its own digits. */
.oaits__table thead th.oaits__th--num {
  text-align: right;
}

.oaits__table tbody td {
  padding: 8px 12px;
  border-top: var(--border-width) solid var(--border-subtle);
  color: var(--body-copy);
  font-size: var(--fs-sm);
}

.oaits__table tbody tr:first-child td {
  border-top: none;
}

.oaits__table tbody tr:hover td {
  background: var(--surface-hover);
}

.oaits__td--model {
  color: var(--foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
  overflow-wrap: anywhere;
}

/* tabular-nums is the other half of the alignment fix: right alignment only
   helps if every digit is the same width. */
.oaits__td--num {
  color: var(--foreground);
  font-variant-numeric: tabular-nums;
  text-align: right;
  white-space: nowrap;
}

/* --- card view (below 768px) -------------------------------------------- */

.oaits__cards {
  display: flex;
  flex-direction: column;
}

.oaits__card {
  padding: 12px;
  border-top: var(--border-width) solid var(--border-subtle);
}

.oaits__card:first-child {
  border-top: none;
}

.oaits__model {
  margin: 0 0 8px;
  color: var(--foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
  overflow-wrap: anywhere;
}

.oaits__facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 3px 14px;
  margin: 0;
}

.oaits__fact {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}

.oaits__fact dt {
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

.oaits__fact dd {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-xs);
  font-variant-numeric: tabular-nums;
}
</style>
