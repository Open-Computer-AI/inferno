<script setup lang="ts">
/**
 * The alert rules list and its editor.
 *
 * IT WAS A CARD INSIDE A DIALOG, WITH THE TITLE PRINTED TWICE
 * OpsDashboard renders this inside `<BaseDialog :title="t(...alertRules.title)">`
 * and the component then drew its own `rounded-3xl bg-white shadow-sm ring-1`
 * card containing an `<h3>` bound to that same key. Two surfaces and two copies
 * of one heading. The card chrome and the duplicate `<h3>` are gone; the
 * description stays, as the dialog's lead.
 *
 * THE LIST SHOWED MACHINE NAMES THAT ALREADY HAD HUMAN LABELS
 * The table rendered `row.metric_type` raw -- `group_available_ratio > 10` --
 * while `metricDefinitions` directly above it carries a translated `label` and
 * a `unit` for all fourteen types, used only by the editor's dropdown. The list
 * reads "Group available ratio > 10%" now, off data the component already had.
 * An unrecognised metric_type still falls back to the raw string, so a rule
 * created against a newer backend cannot render blank.
 */
import { computed, onMounted, ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Input from '@/components/common/Input.vue'
import NumberField from '@/components/common/NumberField.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI } from '@/api'
import { opsAPI } from '@/api/admin/ops'
import type { AlertRule, MetricType, Operator } from '../types'
import type { OpsSeverity } from '@/api/admin/ops'
import { formatDateTime } from '../utils/opsFormatters'

const { t } = useI18n()
const appStore = useAppStore()

// 与 DataTable 一致：< 768px 切换为卡片视图，避免宽表在移动端被截断。
const isDesktopViewport = useMediaQuery('(min-width: 768px)')

const loading = ref(false)
const rules = ref<AlertRule[]>([])

async function load() {
  loading.value = true
  try {
    rules.value = await opsAPI.listAlertRules()
  } catch (err: any) {
    console.error('[OpsAlertRulesCard] Failed to load rules', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertRules.loadFailed'))
    rules.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
  loadGroups()
})

const sortedRules = computed(() => {
  return [...rules.value].sort((a, b) => (b.id || 0) - (a.id || 0))
})

const showEditor = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const draft = ref<AlertRule | null>(null)

type MetricGroup = 'system' | 'group' | 'account'

interface MetricDefinition {
  type: MetricType
  group: MetricGroup
  label: string
  description: string
  recommendedOperator: Operator
  recommendedThreshold: number
  unit?: string
}

const groupMetricTypes = new Set<MetricType>([
  'group_available_accounts',
  'group_available_ratio',
  'group_rate_limit_ratio'
])

function parsePositiveInt(value: unknown): number | null {
  if (value == null) return null
  if (typeof value === 'boolean') return null
  const n = typeof value === 'number' ? value : Number.parseInt(String(value), 10)
  return Number.isFinite(n) && n > 0 ? n : null
}

const groupOptionsBase = ref<SelectOption[]>([])

async function loadGroups() {
  try {
    const list = await adminAPI.groups.getAll()
    groupOptionsBase.value = list.map((g) => ({ value: g.id, label: g.name }))
  } catch (err) {
    console.error('[OpsAlertRulesCard] Failed to load groups', err)
    groupOptionsBase.value = []
  }
}

const isGroupMetricSelected = computed(() => {
  const metricType = draft.value?.metric_type
  return metricType ? groupMetricTypes.has(metricType) : false
})

const draftGroupId = computed<number | null>({
  get() {
    return parsePositiveInt(draft.value?.filters?.group_id)
  },
  set(value) {
    if (!draft.value) return
    if (value == null) {
      if (!draft.value.filters) return
      delete draft.value.filters.group_id
      if (Object.keys(draft.value.filters).length === 0) {
        delete draft.value.filters
      }
      return
    }
    if (!draft.value.filters) draft.value.filters = {}
    draft.value.filters.group_id = value
  }
})

const groupOptions = computed<SelectOption[]>(() => {
  if (isGroupMetricSelected.value) return groupOptionsBase.value
  return [{ value: null, label: t('admin.ops.alertRules.form.allGroups') }, ...groupOptionsBase.value]
})

const metricDefinitions = computed(() => {
  return [
    // System-level metrics
    {
      type: 'success_rate',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.successRate'),
      description: t('admin.ops.alertRules.metricDescriptions.successRate'),
      recommendedOperator: '<',
      recommendedThreshold: 99,
      unit: '%'
    },
    {
      type: 'error_rate',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.errorRate'),
      description: t('admin.ops.alertRules.metricDescriptions.errorRate'),
      recommendedOperator: '>',
      recommendedThreshold: 1,
      unit: '%'
    },
    {
      type: 'upstream_error_rate',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.upstreamErrorRate'),
      description: t('admin.ops.alertRules.metricDescriptions.upstreamErrorRate'),
      recommendedOperator: '>',
      recommendedThreshold: 1,
      unit: '%'
    },
    {
      type: 'cpu_usage_percent',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.cpu'),
      description: t('admin.ops.alertRules.metricDescriptions.cpu'),
      recommendedOperator: '>',
      recommendedThreshold: 80,
      unit: '%'
    },
    {
      type: 'memory_usage_percent',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.memory'),
      description: t('admin.ops.alertRules.metricDescriptions.memory'),
      recommendedOperator: '>',
      recommendedThreshold: 80,
      unit: '%'
    },
    {
      type: 'concurrency_queue_depth',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.queueDepth'),
      description: t('admin.ops.alertRules.metricDescriptions.queueDepth'),
      recommendedOperator: '>',
      recommendedThreshold: 10
    },

    // Group-level metrics (requires group_id filter)
    {
      type: 'group_available_accounts',
      group: 'group',
      label: t('admin.ops.alertRules.metrics.groupAvailableAccounts'),
      description: t('admin.ops.alertRules.metricDescriptions.groupAvailableAccounts'),
      recommendedOperator: '<',
      recommendedThreshold: 1
    },
    {
      type: 'group_available_ratio',
      group: 'group',
      label: t('admin.ops.alertRules.metrics.groupAvailableRatio'),
      description: t('admin.ops.alertRules.metricDescriptions.groupAvailableRatio'),
      recommendedOperator: '<',
      recommendedThreshold: 50,
      unit: '%'
    },
    {
      type: 'group_rate_limit_ratio',
      group: 'group',
      label: t('admin.ops.alertRules.metrics.groupRateLimitRatio'),
      description: t('admin.ops.alertRules.metricDescriptions.groupRateLimitRatio'),
      recommendedOperator: '>',
      recommendedThreshold: 10,
      unit: '%'
    },

    // Account-level metrics
    {
      type: 'account_rate_limited_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountRateLimitedCount'),
      description: t('admin.ops.alertRules.metricDescriptions.accountRateLimitedCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    },
    {
      type: 'account_error_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountErrorCount'),
      description: t('admin.ops.alertRules.metricDescriptions.accountErrorCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    },
    {
      type: 'account_error_ratio',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountErrorRatio'),
      description: t('admin.ops.alertRules.metricDescriptions.accountErrorRatio'),
      recommendedOperator: '>',
      recommendedThreshold: 5,
      unit: '%'
    },
    {
      type: 'account_temp_unscheduled_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountTempUnscheduledCount'),
      description: t('admin.ops.alertRules.metricDescriptions.accountTempUnscheduledCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    },
    {
      type: 'overload_account_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.overloadAccountCount'),
      description: t('admin.ops.alertRules.metricDescriptions.overloadAccountCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    }
  ] satisfies MetricDefinition[]
})

const selectedMetricDefinition = computed(() => {
  const metricType = draft.value?.metric_type
  if (!metricType) return null
  return metricDefinitions.value.find((m) => m.type === metricType) ?? null
})

const metricOptions = computed(() => {
  const buildGroup = (group: MetricGroup): SelectOption[] => {
    const items = metricDefinitions.value.filter((m) => m.group === group)
    if (items.length === 0) return []
    const headerValue = `__group__${group}`
    return [
      {
        value: headerValue,
        label: t(`admin.ops.alertRules.metricGroups.${group}`),
        disabled: true,
        kind: 'group'
      },
      ...items.map((m) => ({ value: m.type, label: m.label }))
    ]
  }

  return [...buildGroup('system'), ...buildGroup('group'), ...buildGroup('account')]
})

const operatorOptions = computed(() => {
  const ops: Operator[] = ['>', '>=', '<', '<=', '==', '!=']
  return ops.map((o) => ({ value: o, label: o }))
})

const severityOptions = computed(() => {
  const sev: OpsSeverity[] = ['P0', 'P1', 'P2', 'P3']
  return sev.map((s) => ({ value: s, label: s }))
})

const windowOptions = computed(() => {
  const windows = [1, 5, 60]
  return windows.map((m) => ({ value: m, label: `${m}m` }))
})

/* The editor's dropdown had these labels all along; the list did not. Falls
   back to the raw type so a rule from a newer backend still renders. */
function metricLabel(type: MetricType): string {
  return metricDefinitions.value.find((m) => m.type === type)?.label ?? String(type)
}

function metricUnit(type: MetricType): string {
  return metricDefinitions.value.find((m) => m.type === type)?.unit ?? ''
}

function newRuleDraft(): AlertRule {
  return {
    name: '',
    description: '',
    enabled: true,
    metric_type: 'error_rate',
    operator: '>',
    threshold: 1,
    window_minutes: 1,
    sustained_minutes: 2,
    severity: 'P1',
    cooldown_minutes: 10,
    notify_email: true
  }
}

function openCreate() {
  editingId.value = null
  draft.value = newRuleDraft()
  showEditor.value = true
}

function openEdit(rule: AlertRule) {
  editingId.value = rule.id ?? null
  draft.value = JSON.parse(JSON.stringify(rule))
  showEditor.value = true
}

const editorValidation = computed(() => {
  const errors: string[] = []
  const r = draft.value
  if (!r) return { valid: true, errors }
  if (!r.name || !r.name.trim()) errors.push(t('admin.ops.alertRules.validation.nameRequired'))
  if (!r.metric_type) errors.push(t('admin.ops.alertRules.validation.metricRequired'))
  if (groupMetricTypes.has(r.metric_type) && !parsePositiveInt(r.filters?.group_id)) {
    errors.push(t('admin.ops.alertRules.validation.groupIdRequired'))
  }
  if (!r.operator) errors.push(t('admin.ops.alertRules.validation.operatorRequired'))
  if (!(typeof r.threshold === 'number' && Number.isFinite(r.threshold)))
    errors.push(t('admin.ops.alertRules.validation.thresholdRequired'))
  if (!(typeof r.window_minutes === 'number' && Number.isFinite(r.window_minutes) && [1, 5, 60].includes(r.window_minutes))) {
    errors.push(t('admin.ops.alertRules.validation.windowRange'))
  }
  if (!(typeof r.sustained_minutes === 'number' && Number.isFinite(r.sustained_minutes) && r.sustained_minutes >= 1 && r.sustained_minutes <= 1440)) {
    errors.push(t('admin.ops.alertRules.validation.sustainedRange'))
  }
  if (!(typeof r.cooldown_minutes === 'number' && Number.isFinite(r.cooldown_minutes) && r.cooldown_minutes >= 0 && r.cooldown_minutes <= 1440)) {
    errors.push(t('admin.ops.alertRules.validation.cooldownRange'))
  }
  return { valid: errors.length === 0, errors }
})

async function save() {
  if (!draft.value) return
  if (!editorValidation.value.valid) {
    appStore.showError(editorValidation.value.errors[0] || t('admin.ops.alertRules.validation.invalid'))
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await opsAPI.updateAlertRule(editingId.value, draft.value)
    } else {
      await opsAPI.createAlertRule(draft.value)
    }
    showEditor.value = false
    draft.value = null
    editingId.value = null
    await load()
    appStore.showSuccess(t('admin.ops.alertRules.saveSuccess'))
  } catch (err: any) {
    console.error('[OpsAlertRulesCard] Failed to save rule', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertRules.saveFailed'))
  } finally {
    saving.value = false
  }
}

const showDeleteConfirm = ref(false)
const pendingDelete = ref<AlertRule | null>(null)

function requestDelete(rule: AlertRule) {
  pendingDelete.value = rule
  showDeleteConfirm.value = true
}

async function confirmDelete() {
  if (!pendingDelete.value?.id) return
  try {
    await opsAPI.deleteAlertRule(pendingDelete.value.id)
    showDeleteConfirm.value = false
    pendingDelete.value = null
    await load()
    appStore.showSuccess(t('admin.ops.alertRules.deleteSuccess'))
  } catch (err: any) {
    console.error('[OpsAlertRulesCard] Failed to delete rule', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertRules.deleteFailed'))
  }
}

function cancelDelete() {
  showDeleteConfirm.value = false
  pendingDelete.value = null
}
</script>

<template>
  <div class="arules">
    <div class="arules__bar">
      <p class="arules__lead">{{ t('admin.ops.alertRules.description') }}</p>

      <div class="arules__actions">
        <button class="btn btn-sm btn-primary" :disabled="loading" @click="openCreate">
          {{ t('admin.ops.alertRules.create') }}
        </button>
        <button type="button" class="arules__refresh" :disabled="loading" @click="load">
          {{ t('common.refresh') }}
        </button>
      </div>
    </div>

    <p v-if="loading" class="arules__state">{{ t('admin.ops.alertRules.loading') }}</p>

    <p v-else-if="sortedRules.length === 0" class="arules__empty">
      {{ t('admin.ops.alertRules.empty') }}
    </p>

    <div v-else class="arules__frame">
      <div class="arules__scroll">
        <!-- Card view below 768px: a wide table truncates badly there. -->
        <div v-if="!isDesktopViewport" class="arules__cards">
          <article v-for="row in sortedRules" :key="row.id" class="arules__card" :data-off="row.enabled ? undefined : true">
            <div class="arules__card-top">
              <div class="arules__name-cell">
                <p class="arules__name">{{ row.name }}</p>
                <p v-if="row.description" class="arules__desc">{{ row.description }}</p>
              </div>
              <span class="arules__sev">{{ row.severity }}</span>
            </div>

            <p class="arules__cond">
              <span class="arules__metric">{{ metricLabel(row.metric_type) }}</span>
              <span class="arules__op">{{ row.operator }}</span>
              <span class="arules__threshold">{{ row.threshold }}{{ metricUnit(row.metric_type) }}</span>
            </p>

            <div class="arules__card-foot">
              <span class="arules__state-chip" :data-off="row.enabled ? undefined : true">
                {{ row.enabled ? t('common.enabled') : t('common.disabled') }}
              </span>
              <div class="arules__row-actions">
                <button class="btn btn-sm btn-secondary" @click="openEdit(row)">{{ t('common.edit') }}</button>
                <button class="btn btn-sm btn-danger" @click="requestDelete(row)">{{ t('common.delete') }}</button>
              </div>
            </div>

            <p v-if="row.updated_at" class="arules__stamp">{{ formatDateTime(row.updated_at) }}</p>
          </article>
        </div>

        <table v-else class="arules__table">
          <thead>
            <tr>
              <th scope="col">{{ t('admin.ops.alertRules.table.name') }}</th>
              <th scope="col">{{ t('admin.ops.alertRules.table.metric') }}</th>
              <th scope="col">{{ t('admin.ops.alertRules.table.severity') }}</th>
              <th scope="col">{{ t('admin.ops.alertRules.table.enabled') }}</th>
              <th scope="col" class="arules__th--right">{{ t('admin.ops.alertRules.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in sortedRules" :key="row.id" :data-off="row.enabled ? undefined : true">
              <td>
                <p class="arules__name">{{ row.name }}</p>
                <p v-if="row.description" class="arules__desc">{{ row.description }}</p>
                <p v-if="row.updated_at" class="arules__stamp">{{ formatDateTime(row.updated_at) }}</p>
              </td>
              <td class="arules__td--nowrap">
                <span class="arules__metric">{{ metricLabel(row.metric_type) }}</span>
                <span class="arules__op">{{ row.operator }}</span>
                <span class="arules__threshold">{{ row.threshold }}{{ metricUnit(row.metric_type) }}</span>
              </td>
              <td class="arules__td--nowrap">
                <span class="arules__sev">{{ row.severity }}</span>
              </td>
              <td class="arules__td--nowrap">
                <span class="arules__state-chip" :data-off="row.enabled ? undefined : true">
                  {{ row.enabled ? t('common.enabled') : t('common.disabled') }}
                </span>
              </td>
              <td class="arules__td--nowrap arules__td--right">
                <div class="arules__row-actions">
                  <button class="btn btn-sm btn-secondary" @click="openEdit(row)">{{ t('common.edit') }}</button>
                  <button class="btn btn-sm btn-danger" @click="requestDelete(row)">{{ t('common.delete') }}</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <BaseDialog
      :show="showEditor"
      :title="editingId ? t('admin.ops.alertRules.editTitle') : t('admin.ops.alertRules.createTitle')"
      width="wide"
      @close="showEditor = false"
    >
      <div v-if="draft" class="arules__form">
        <div v-if="!editorValidation.valid" class="arules__invalid">
          <p class="arules__invalid-title">{{ t('admin.ops.alertRules.validation.title') }}</p>
          <ul class="arules__invalid-list">
            <li v-for="e in editorValidation.errors" :key="e">{{ e }}</li>
          </ul>
        </div>

        <div class="arules__grid">
          <div class="arules__span2">
            <Input v-model="draft.name" :label="t('admin.ops.alertRules.form.name')" type="text" />
          </div>

          <div class="arules__span2">
            <Input v-model="draft.description" :label="t('admin.ops.alertRules.form.description')" type="text" />
          </div>

          <div>
            <label class="arules__label">{{ t('admin.ops.alertRules.form.metric') }}</label>
            <Select v-model="draft.metric_type" :options="metricOptions" />
            <template v-if="selectedMetricDefinition">
              <p class="arules__hint">{{ selectedMetricDefinition.description }}</p>
              <p class="arules__hint">
                {{
                  t('admin.ops.alertRules.hints.recommended', {
                    operator: selectedMetricDefinition.recommendedOperator,
                    threshold: selectedMetricDefinition.recommendedThreshold,
                    unit: selectedMetricDefinition.unit || ''
                  })
                }}
              </p>
            </template>
          </div>

          <div>
            <label class="arules__label">{{ t('admin.ops.alertRules.form.operator') }}</label>
            <Select v-model="draft.operator" :options="operatorOptions" />
          </div>

          <div class="arules__span2">
            <label class="arules__label">
              {{ t('admin.ops.alertRules.form.groupId') }}
              <span v-if="isGroupMetricSelected" class="arules__required" aria-hidden="true">*</span>
            </label>
            <Select
              v-model="draftGroupId"
              :options="groupOptions"
              searchable
              :placeholder="t('admin.ops.alertRules.form.groupPlaceholder')"
              :error="isGroupMetricSelected && !draftGroupId"
            />
            <p class="arules__hint">
              {{ isGroupMetricSelected ? t('admin.ops.alertRules.hints.groupRequired') : t('admin.ops.alertRules.hints.groupOptional') }}
            </p>
          </div>

          <NumberField v-model="draft.threshold" :label="t('admin.ops.alertRules.form.threshold')" />

          <div>
            <label class="arules__label">{{ t('admin.ops.alertRules.form.severity') }}</label>
            <Select v-model="draft.severity" :options="severityOptions" />
          </div>

          <div>
            <label class="arules__label">{{ t('admin.ops.alertRules.form.window') }}</label>
            <Select v-model="draft.window_minutes" :options="windowOptions" />
          </div>

          <NumberField
            v-model="draft.sustained_minutes"
            :label="t('admin.ops.alertRules.form.sustained')"
            :min="1"
            :max="1440"
            :fallback="1"
            integer
          />

          <NumberField
            v-model="draft.cooldown_minutes"
            :label="t('admin.ops.alertRules.form.cooldown')"
            :min="0"
            :max="1440"
            :fallback="0"
            integer
          />

          <div class="arules__switch arules__span2">
            <span class="arules__switch-label">{{ t('admin.ops.alertRules.form.enabled') }}</span>
            <Toggle v-model="draft.enabled" />
          </div>

          <div class="arules__switch arules__span2">
            <span class="arules__switch-label">{{ t('admin.ops.alertRules.form.notifyEmail') }}</span>
            <Toggle v-model="draft.notify_email" />
          </div>
        </div>
      </div>

      <template #footer>
        <div class="arules__footer">
          <button class="btn btn-secondary" :disabled="saving" @click="showEditor = false">
            {{ t('common.cancel') }}
          </button>
          <button class="btn btn-primary" :disabled="saving" @click="save">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.ops.alertRules.deleteConfirmTitle')"
      :message="t('admin.ops.alertRules.deleteConfirmMessage')"
      :confirmText="t('common.delete')"
      :cancelText="t('common.cancel')"
      @confirm="confirmDelete"
      @cancel="cancelDelete"
    />
  </div>
</template>

<style scoped>
/* No card. This renders inside a BaseDialog that already supplies the surface
   and the heading; the old `rounded-3xl bg-white shadow-sm ring-1` wrapper put
   a second one inside the first. */
.arules {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.arules__bar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.arules__lead {
  margin: 0;
  max-width: 62ch;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.arules__actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.arules__refresh {
  padding: 4px 10px;
  border: none;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}

.arules__refresh:hover:not(:disabled) {
  background: var(--surface-hover);
  color: var(--foreground);
}

.arules__refresh:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.arules__refresh:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* --- states ------------------------------------------------------------- */

.arules__state {
  margin: 0;
  padding: 40px 0;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  text-align: center;
}

.arules__empty {
  margin: 0;
  padding: 32px 24px;
  border: var(--border-width) dashed var(--border-subtle);
  border-radius: var(--r-md);
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  text-align: center;
}

/* --- frame -------------------------------------------------------------- */

.arules__frame {
  min-height: 0;
  overflow: hidden;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-md);
}

.arules__scroll {
  max-height: 520px;
  overflow-y: auto;
}

/* --- table -------------------------------------------------------------- */

.arules__table {
  width: 100%;
  border-collapse: collapse;
}

.arules__table thead th {
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

.arules__th--right {
  text-align: right;
}

.arules__table tbody td {
  padding: 10px 14px;
  border-top: var(--border-width) solid var(--border-subtle);
  color: var(--body-copy);
  font-size: var(--fs-sm);
  vertical-align: top;
}

.arules__table tbody tr:first-child td {
  border-top: none;
}

.arules__table tbody tr:hover td {
  background: var(--surface-hover);
}

/* A rule that is switched off is not doing anything, and should not read as
   loud as one that is armed. */
.arules__table tbody tr[data-off] td {
  opacity: 0.55;
}

.arules__td--nowrap {
  white-space: nowrap;
  vertical-align: middle;
}

.arules__td--right {
  text-align: right;
}

/* --- cells -------------------------------------------------------------- */

.arules__name {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}

.arules__desc {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  margin: 2px 0 0;
  overflow: hidden;
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

.arules__stamp {
  margin: 3px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

.arules__cond {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px;
  margin: 0;
}

.arules__metric {
  color: var(--body-copy);
}

.arules__op {
  margin: 0 4px;
  color: var(--muted-foreground);
  font-family: var(--font-mono);
}

.arules__threshold {
  color: var(--foreground);
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}

/* Severity here is what a rule WOULD raise, not an alert that is firing, so it
   stays neutral. The alert events card is where the P0..P3 ramp earns colour,
   and only on rows that are still open. */
.arules__sev {
  display: inline-block;
  padding: 2px 7px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
}

.arules__state-chip {
  display: inline-block;
  padding: 2px 8px;
  border-radius: var(--r-pill);
  background: var(--surface-subtle);
  color: var(--body-copy);
  font-size: var(--fs-2xs);
  white-space: nowrap;
}

.arules__state-chip[data-off] {
  color: var(--muted-foreground);
}

.arules__row-actions {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

/* --- card view (below 768px) -------------------------------------------- */

.arules__cards {
  display: flex;
  flex-direction: column;
}

.arules__card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 14px;
  border-top: var(--border-width) solid var(--border-subtle);
}

.arules__card:first-child {
  border-top: none;
}

.arules__card[data-off] {
  opacity: 0.55;
}

.arules__card-top {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}

.arules__name-cell {
  min-width: 0;
}

.arules__card-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

/* --- editor ------------------------------------------------------------- */

.arules__form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

/* Attention, not destructive: nothing has failed, the form is not finished. */
.arules__invalid {
  padding: 10px 12px;
  border-radius: var(--r-md);
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
  font-size: var(--fs-sm);
}

.arules__invalid-title {
  margin: 0;
  font-weight: var(--fw-medium);
}

.arules__invalid-list {
  margin: 4px 0 0;
  padding-left: 18px;
}

.arules__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 14px;
  align-items: start;
}

.arules__span2 {
  grid-column: 1 / -1;
}

.arules__label {
  display: block;
  margin-bottom: 5px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.arules__required {
  margin-left: 2px;
  color: var(--destructive);
}

.arules__hint {
  margin: 6px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.arules__switch {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  border-radius: var(--r-md);
  background: var(--surface-subtle);
}

.arules__switch-label {
  color: var(--foreground);
  font-size: var(--fs-md);
}

.arules__footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}
</style>
