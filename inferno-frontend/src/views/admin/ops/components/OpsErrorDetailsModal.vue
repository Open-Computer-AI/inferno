<script setup lang="ts">
/**
 * The filtered error list, which opens the single-error detail modal.
 *
 * ITS STYLE BLOCK WAS NOT SCOPED
 * `.compact-select .select-trigger { @apply ... }` was injected globally the
 * moment this component loaded, so it restyled any `.compact-select` on any
 * route -- a modal reaching outside itself. Nothing else in src/ uses that
 * class, so the reach was unused, but it was still there. Scoped now, with
 * :deep() to cross into Select, which says the same thing without the reach.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import OpsErrorLogTable from './OpsErrorLogTable.vue'
import { opsAPI, type OpsErrorLog } from '@/api/admin/ops'
import { buildOpsErrorTimeParams } from '../utils/opsErrorParams'

interface Props {
  show: boolean
  timeRange: string
  customStartTime?: string | null
  customEndTime?: string | null
  platform?: string
  groupId?: number | null
  errorType: 'request' | 'upstream'
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'openErrorDetail', errorId: number): void
}>()

const { t } = useI18n()


const loading = ref(false)
const rows = ref<OpsErrorLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const q = ref('')
const statusCode = ref<number | 'other' | null>(null)
const phase = ref<string>('')
const errorOwner = ref<string>('')
const viewMode = ref<'errors' | 'excluded' | 'all'>('errors')


const modalTitle = computed(() => {
  return props.errorType === 'upstream' ? t('admin.ops.errorDetails.upstreamErrors') : t('admin.ops.errorDetails.requestErrors')
})

const statusCodeSelectOptions = computed(() => {
  const codes = [400, 401, 403, 404, 409, 422, 429, 500, 502, 503, 504, 529]
  return [
    { value: null, label: t('common.all') },
    ...codes.map((c) => ({ value: c, label: String(c) })),
    { value: 'other', label: t('admin.ops.errorDetails.statusCodeOther') || 'Other' }
  ]
})

const ownerSelectOptions = computed(() => {
  return [
    { value: '', label: t('common.all') },
    { value: 'provider', label: t('admin.ops.errorDetails.owner.provider') || 'provider' },
    { value: 'client', label: t('admin.ops.errorDetails.owner.client') || 'client' },
    { value: 'platform', label: t('admin.ops.errorDetails.owner.platform') || 'platform' }
  ]
})


const viewModeSelectOptions = computed(() => {
  return [
    { value: 'errors', label: t('admin.ops.errorDetails.viewErrors') || 'errors' },
    { value: 'excluded', label: t('admin.ops.errorDetails.viewExcluded') || 'excluded' },
    { value: 'all', label: t('common.all') }
  ]
})

const phaseSelectOptions = computed(() => {
  const options = [
    { value: '', label: t('common.all') },
    { value: 'request', label: t('admin.ops.errorDetails.phase.request') || 'request' },
    { value: 'auth', label: t('admin.ops.errorDetails.phase.auth') || 'auth' },
    { value: 'account_auth', label: t('admin.ops.errorDetails.phase.account_auth') || 'account_auth' },
    { value: 'routing', label: t('admin.ops.errorDetails.phase.routing') || 'routing' },
    { value: 'upstream', label: t('admin.ops.errorDetails.phase.upstream') || 'upstream' },
    { value: 'network', label: t('admin.ops.errorDetails.phase.network') || 'network' },
    { value: 'internal', label: t('admin.ops.errorDetails.phase.internal') || 'internal' }
  ]
  return options
})

function close() {
  emit('update:show', false)
}

const sortBy = ref('created_at')
const sortOrder = ref<'asc' | 'desc'>('desc')

function onSort(nextSortBy: string, nextSortOrder: 'asc' | 'desc') {
  sortBy.value = nextSortBy
  sortOrder.value = nextSortOrder
  page.value = 1
  void fetchErrorLogs()
}

async function fetchErrorLogs() {
  if (!props.show) return

  loading.value = true
  try {
    const params: Record<string, any> = {
      page: page.value,
      page_size: pageSize.value,
      view: viewMode.value,
      sort_by: sortBy.value,
      sort_order: sortOrder.value
    }
    Object.assign(params, buildOpsErrorTimeParams(props.timeRange, props.customStartTime, props.customEndTime))

    const platform = String(props.platform || '').trim()
    if (platform) params.platform = platform
    if (typeof props.groupId === 'number' && props.groupId > 0) params.group_id = props.groupId

    if (q.value.trim()) params.q = q.value.trim()
    if (statusCode.value === 'other') params.status_codes_other = '1'
    else if (typeof statusCode.value === 'number') params.status_codes = String(statusCode.value)

    const phaseVal = String(phase.value || '').trim()
    if (phaseVal) params.phase = phaseVal

    const ownerVal = String(errorOwner.value || '').trim()
    if (ownerVal) params.error_owner = ownerVal


    const res = props.errorType === 'upstream'
      ? await opsAPI.listUpstreamErrors(params)
      : await opsAPI.listRequestErrors(params)
    rows.value = res.items || []
    total.value = res.total || 0
  } catch (err) {
    console.error('[OpsErrorDetailsModal] Failed to fetch error logs', err)
    rows.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

  function resetFilters() {
    q.value = ''
    statusCode.value = null
    phase.value = props.errorType === 'upstream' ? 'upstream' : ''
    errorOwner.value = ''
    viewMode.value = 'errors'
    page.value = 1
    fetchErrorLogs()
  }


watch(
  () => props.show,
  (open) => {
    if (!open) return
    page.value = 1
    pageSize.value = 10
    resetFilters()
  }
)

watch(
  () => [props.timeRange, props.customStartTime, props.customEndTime, props.platform, props.groupId] as const,
  () => {
    if (!props.show) return
    page.value = 1
    fetchErrorLogs()
  }
)

watch(
  () => [page.value, pageSize.value] as const,
  () => {
    if (!props.show) return
    fetchErrorLogs()
  }
)

let searchTimeout: number | null = null
watch(
  () => q.value,
  () => {
    if (!props.show) return
    if (searchTimeout) window.clearTimeout(searchTimeout)
    searchTimeout = window.setTimeout(() => {
      page.value = 1
      fetchErrorLogs()
    }, 350)
  }
)

watch(
  () => [statusCode.value, phase.value, errorOwner.value, viewMode.value] as const,
  () => {
    if (!props.show) return
    page.value = 1
    fetchErrorLogs()
  }
)
</script>

<template>
  <BaseDialog :show="show" :title="modalTitle" width="full" @close="close">
    <div class="errdet">
      <!-- Filters -->
      <div class="errdet__filters">
        <div class="errdet__search">
          <Icon name="search" size="xs" :stroke-width="2" class="errdet__search-icon" aria-hidden="true" />
          <input
            v-model="q"
            type="search"
            class="errdet__search-input"
            :placeholder="t('admin.ops.errorDetails.searchPlaceholder')"
          />
        </div>

        <div class="errdet__select">
          <Select :model-value="statusCode" :options="statusCodeSelectOptions" @update:model-value="statusCode = $event as any" />
        </div>

        <div class="errdet__select">
          <Select :model-value="phase" :options="phaseSelectOptions" @update:model-value="phase = String($event ?? '')" />
        </div>

        <div class="errdet__select">
          <Select :model-value="errorOwner" :options="ownerSelectOptions" @update:model-value="errorOwner = String($event ?? '')" />
        </div>

        <div class="errdet__select">
          <Select :model-value="viewMode" :options="viewModeSelectOptions" @update:model-value="viewMode = $event as any" />
        </div>

        <button type="button" class="errdet__reset" @click="resetFilters">
          {{ t('common.reset') }}
        </button>
      </div>

      <!-- Body -->
      <div class="errdet__body">
        <p class="errdet__count">{{ t('admin.ops.errorDetails.total') }} {{ total }}</p>

        <OpsErrorLogTable
          class="errdet__table"
          :rows="rows"
          :total="total"
          :loading="loading"
          :page="page"
          :page-size="pageSize"
          @openErrorDetail="emit('openErrorDetail', $event)"
          @sort="onSort"
          @update:page="page = $event"
          @update:pageSize="pageSize = $event"
        />
      </div>
    </div>
  </BaseDialog>
</template>

<style scoped>
.errdet {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}

.errdet__filters {
  display: flex;
  flex: none;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
  padding-bottom: 14px;
  border-bottom: var(--border-width) solid var(--border-subtle);
}

/* --- search ------------------------------------------------------------- */

.errdet__search {
  position: relative;
  flex: 1 1 240px;
  min-width: 200px;
}

.errdet__search-icon {
  position: absolute;
  top: 50%;
  left: 10px;
  transform: translateY(-50%);
  color: var(--muted-foreground);
  pointer-events: none;
}

.errdet__search-input {
  width: 100%;
  padding: 6px 12px 6px 30px;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-sm);
  background: var(--surface-subtle);
  color: var(--foreground);
  font-size: var(--fs-sm);
  transition: background var(--motion-hover), box-shadow var(--motion-hover);
}

.errdet__search-input::placeholder {
  color: var(--muted-foreground);
}

/* Focus was focus:border-blue-500 with a blue ring; the focus ring token is
   the same one every other control in the product uses. */
.errdet__search-input:focus {
  outline: none;
  background: var(--card);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.errdet__select {
  flex: 0 1 140px;
  min-width: 110px;
}

/* Was a GLOBAL rule using @apply. :deep() reaches into Select from a scoped
   block, so the height match stays local to this modal. */
.errdet__select :deep(.select-trigger) {
  padding: 6px 12px;
  border-radius: var(--r-sm);
  font-size: var(--fs-sm);
}

.errdet__reset {
  margin-left: auto;
  padding: 6px 12px;
  border: none;
  border-radius: var(--r-sm);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}

.errdet__reset:hover {
  background: var(--surface-hover);
  color: var(--foreground);
}

.errdet__reset:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

/* --- body --------------------------------------------------------------- */

.errdet__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}

.errdet__count {
  flex: none;
  margin: 0 0 8px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}

.errdet__table {
  flex: 1;
  min-height: 0;
}
</style>
