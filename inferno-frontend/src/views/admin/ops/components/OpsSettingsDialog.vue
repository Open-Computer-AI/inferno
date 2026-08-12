<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { opsAPI } from '@/api/admin/ops'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Input from '@/components/common/Input.vue'
import NumberField from '@/components/common/NumberField.vue'
import Button from '@/components/common/Button.vue'
import type { OpsAlertRuntimeSettings, EmailNotificationConfig, AlertSeverity, OpsAdvancedSettings, OpsMetricThresholds } from '../types'

const { t } = useI18n()
const appStore = useAppStore()

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  close: []
  saved: []
}>()

const loading = ref(false)
const saving = ref(false)

// 运行时设置
const runtimeSettings = ref<OpsAlertRuntimeSettings | null>(null)
// 邮件通知配置
const emailConfig = ref<EmailNotificationConfig | null>(null)
// 高级设置
const advancedSettings = ref<OpsAdvancedSettings | null>(null)
// 指标阈值配置
const metricThresholds = ref<OpsMetricThresholds>({
  sla_percent_min: 99.5,
  ttft_p99_ms_max: 500,
  request_error_rate_percent_max: 5,
  upstream_error_rate_percent_max: 5
})

// 加载所有配置
async function loadAllSettings() {
  loading.value = true
  try {
    const [runtime, email, advanced, thresholds] = await Promise.all([
      opsAPI.getAlertRuntimeSettings(),
      opsAPI.getEmailNotificationConfig(),
      opsAPI.getAdvancedSettings(),
      opsAPI.getMetricThresholds()
    ])
    runtimeSettings.value = runtime
    emailConfig.value = email
    advancedSettings.value = advanced
    // 兼容旧 payload：后端未返回该字段时补默认值，保证表单可绑定
    if (advancedSettings.value && !advancedSettings.value.openai_account_quota_auto_pause) {
      advancedSettings.value.openai_account_quota_auto_pause = { default_threshold_5h: 0, default_threshold_7d: 0 }
    }
    // 如果后端返回了阈值，使用后端的值；否则保持默认值
    if (thresholds && Object.keys(thresholds).length > 0) {
        metricThresholds.value = {
          sla_percent_min: thresholds.sla_percent_min ?? 99.5,
          ttft_p99_ms_max: thresholds.ttft_p99_ms_max ?? 500,
          request_error_rate_percent_max: thresholds.request_error_rate_percent_max ?? 5,
          upstream_error_rate_percent_max: thresholds.upstream_error_rate_percent_max ?? 5
        }
    }
  } catch (err: any) {
    console.error('[OpsSettingsDialog] Failed to load settings', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.settings.loadFailed'))
  } finally {
    loading.value = false
  }
}

// 监听弹窗打开
watch(() => props.show, (show) => {
  if (show) {
    loadAllSettings()
  }
})

// 邮件输入
const alertRecipientInput = ref('')
const reportRecipientInput = ref('')

// 严重级别选项
const severityOptions: Array<{ value: AlertSeverity | ''; label: string }> = [
  { value: '', label: t('admin.ops.email.minSeverityAll') },
  { value: 'critical', label: t('common.critical') },
  { value: 'warning', label: t('common.warning') },
  { value: 'info', label: t('common.info') }
]

// 验证邮箱
function isValidEmailAddress(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)
}

// 添加收件人
function addRecipient(target: 'alert' | 'report') {
  if (!emailConfig.value) return
  const raw = (target === 'alert' ? alertRecipientInput.value : reportRecipientInput.value).trim()
  if (!raw) return

  if (!isValidEmailAddress(raw)) {
    appStore.showError(t('common.invalidEmail'))
    return
  }

  const normalized = raw.toLowerCase()
  const list = target === 'alert' ? emailConfig.value.alert.recipients : emailConfig.value.report.recipients
  if (!list.includes(normalized)) {
    list.push(normalized)
  }
  if (target === 'alert') alertRecipientInput.value = ''
  else reportRecipientInput.value = ''
}

// 移除收件人
function removeRecipient(target: 'alert' | 'report', email: string) {
  if (!emailConfig.value) return
  const list = target === 'alert' ? emailConfig.value.alert.recipients : emailConfig.value.report.recipients
  const idx = list.indexOf(email)
  if (idx >= 0) list.splice(idx, 1)
}

// OpenAI 账号配额自动暂停：后端按 0~1 分数存储，UI 按百分比(0~100)展示
const quotaAutoPause5hPercent = computed<number | null>({
  get() {
    const v = advancedSettings.value?.openai_account_quota_auto_pause?.default_threshold_5h
    return v && v > 0 ? Math.round(v * 1000) / 10 : null
  },
  set(val) {
    if (!advancedSettings.value?.openai_account_quota_auto_pause) return
    advancedSettings.value.openai_account_quota_auto_pause.default_threshold_5h = val != null && val > 0 ? val / 100 : 0
  }
})
const quotaAutoPause7dPercent = computed<number | null>({
  get() {
    const v = advancedSettings.value?.openai_account_quota_auto_pause?.default_threshold_7d
    return v && v > 0 ? Math.round(v * 1000) / 10 : null
  },
  set(val) {
    if (!advancedSettings.value?.openai_account_quota_auto_pause) return
    advancedSettings.value.openai_account_quota_auto_pause.default_threshold_7d = val != null && val > 0 ? val / 100 : 0
  }
})

// 验证
const validation = computed(() => {
  const errors: string[] = []

  // 验证运行时设置
  if (runtimeSettings.value) {
    const evalSeconds = runtimeSettings.value.evaluation_interval_seconds
    if (!Number.isFinite(evalSeconds) || evalSeconds < 1 || evalSeconds > 86400) {
      errors.push(t('admin.ops.runtime.validation.evalIntervalRange'))
    }
  }

  // 邮件配置: 启用但无收件人时不阻断保存, 保存时会自动禁用

  // 验证高级设置
  if (advancedSettings.value) {
    const { error_log_retention_days, minute_metrics_retention_days, hourly_metrics_retention_days } = advancedSettings.value.data_retention
    if (error_log_retention_days < 0 || error_log_retention_days > 365) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }
    if (minute_metrics_retention_days < 0 || minute_metrics_retention_days > 365) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }
    if (hourly_metrics_retention_days < 0 || hourly_metrics_retention_days > 365) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }

    const { default_threshold_5h, default_threshold_7d } = advancedSettings.value.openai_account_quota_auto_pause
    if (default_threshold_5h < 0 || default_threshold_5h > 1 || default_threshold_7d < 0 || default_threshold_7d > 1) {
      errors.push(t('admin.ops.settings.validation.openaiQuotaAutoPauseRange'))
    }
  }

  // 验证指标阈值
  if (metricThresholds.value.sla_percent_min != null && (metricThresholds.value.sla_percent_min < 0 || metricThresholds.value.sla_percent_min > 100)) {
    errors.push(t('admin.ops.settings.validation.slaMinPercentRange'))
  }
  if (metricThresholds.value.ttft_p99_ms_max != null && metricThresholds.value.ttft_p99_ms_max < 0) {
    errors.push(t('admin.ops.settings.validation.ttftP99MaxRange'))
  }
  if (metricThresholds.value.request_error_rate_percent_max != null && (metricThresholds.value.request_error_rate_percent_max < 0 || metricThresholds.value.request_error_rate_percent_max > 100)) {
    errors.push(t('admin.ops.settings.validation.requestErrorRateMaxRange'))
  }
  if (metricThresholds.value.upstream_error_rate_percent_max != null && (metricThresholds.value.upstream_error_rate_percent_max < 0 || metricThresholds.value.upstream_error_rate_percent_max > 100)) {
    errors.push(t('admin.ops.settings.validation.upstreamErrorRateMaxRange'))
  }

  return { valid: errors.length === 0, errors }
})

// 保存所有配置
async function saveAllSettings() {
  if (!validation.value.valid) {
    appStore.showError(validation.value.errors[0])
    return
  }

  saving.value = true
  try {
    // 无收件人时自动禁用邮件通知
    if (emailConfig.value) {
      if (emailConfig.value.alert.enabled && emailConfig.value.alert.recipients.length === 0) {
        emailConfig.value.alert.enabled = false
      }
      if (emailConfig.value.report.enabled && emailConfig.value.report.recipients.length === 0) {
        emailConfig.value.report.enabled = false
      }
    }
    await Promise.all([
      runtimeSettings.value ? opsAPI.updateAlertRuntimeSettings(runtimeSettings.value) : Promise.resolve(),
      emailConfig.value ? opsAPI.updateEmailNotificationConfig(emailConfig.value) : Promise.resolve(),
      advancedSettings.value ? opsAPI.updateAdvancedSettings(advancedSettings.value) : Promise.resolve(),
      opsAPI.updateMetricThresholds(metricThresholds.value)
    ])
    appStore.showSuccess(t('admin.ops.settings.saveSuccess'))
    emit('saved')
    emit('close')
  } catch (err: any) {
    console.error('[OpsSettingsDialog] Failed to save settings', err)
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || t('admin.ops.settings.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <BaseDialog :show="show" :title="t('admin.ops.settings.title')" width="extra-wide" @close="emit('close')">
    <div v-if="loading" class="opsset__loading">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="runtimeSettings && emailConfig && advancedSettings" class="opsset__body">
      <!-- 验证错误 -->
      <div v-if="!validation.valid" class="opsset__invalid">
        <p class="opsset__invalid-title">{{ t('admin.ops.settings.validation.title') }}</p>
        <ul class="opsset__invalid-list">
          <li v-for="msg in validation.errors" :key="msg">{{ msg }}</li>
        </ul>
      </div>

      <!-- 数据采集频率 -->
      <div class="opsset__section">
        <h4 class="opsset__title">{{ t('admin.ops.settings.dataCollection') }}</h4>
        <div>
          <NumberField
              :label="t('admin.ops.settings.evaluationInterval')"
              v-model="runtimeSettings.evaluation_interval_seconds"
              :min="1"
              :max="86400"
              :hint="t('admin.ops.settings.evaluationIntervalHint')"
            />
        </div>
      </div>

      <!-- 预警配置 -->
      <div class="opsset__section">
        <h4 class="opsset__title">{{ t('admin.ops.settings.alertConfig') }}</h4>

        <div class="opsset__stack">
          <div class="opsset__row">
            <div>
              <label class="opsset__row-label">{{ t('admin.ops.settings.enableAlert') }}</label>
            </div>
            <Toggle v-model="emailConfig.alert.enabled" />
          </div>

          <div v-if="emailConfig.alert.enabled">
            <label class="opsset__label">{{ t('admin.ops.settings.alertRecipients') }}</label>
            <div class="opsset__inline">
              <Input
                v-model="alertRecipientInput"
                type="email"
                class="opsset__inline-field"
                :placeholder="t('admin.ops.settings.emailPlaceholder')"
                @enter="addRecipient('alert')"
              />
              <Button variant="secondary" size="md" @click="addRecipient('alert')">{{ t('common.add') }}</Button>
            </div>
            <div class="opsset__chips">
              <span
                v-for="email in emailConfig.alert.recipients"
                :key="email"
                class="opsset__chip"
              >
                {{ email }}
                <button type="button" class="opsset__chip-remove" :aria-label="t('common.delete')" @click="removeRecipient('alert', email)">×</button>
              </span>
            </div>
            <p class="opsset__hint">
              {{ t('admin.ops.settings.recipientsHint') }}
            </p>
          </div>

          <div v-if="emailConfig.alert.enabled">
            <label class="opsset__label">{{ t('admin.ops.settings.minSeverity') }}</label>
            <Select v-model="emailConfig.alert.min_severity" :options="severityOptions" />
          </div>
        </div>
      </div>

      <!-- 评估报告配置 -->
      <div class="opsset__section">
        <h4 class="opsset__title">{{ t('admin.ops.settings.reportConfig') }}</h4>

        <div class="opsset__stack">
          <div class="opsset__row">
            <div>
              <label class="opsset__row-label">{{ t('admin.ops.settings.enableReport') }}</label>
            </div>
            <Toggle v-model="emailConfig.report.enabled" />
          </div>

          <div v-if="emailConfig.report.enabled">
            <label class="opsset__label">{{ t('admin.ops.settings.reportRecipients') }}</label>
            <div class="opsset__inline">
              <Input
                v-model="reportRecipientInput"
                type="email"
                class="opsset__inline-field"
                :placeholder="t('admin.ops.settings.emailPlaceholder')"
                @enter="addRecipient('report')"
              />
              <Button variant="secondary" size="md" @click="addRecipient('report')">{{ t('common.add') }}</Button>
            </div>
            <div class="opsset__chips">
              <span
                v-for="email in emailConfig.report.recipients"
                :key="email"
                class="opsset__chip"
              >
                {{ email }}
                <button type="button" class="opsset__chip-remove" :aria-label="t('common.delete')" @click="removeRecipient('report', email)">×</button>
              </span>
            </div>
            <p class="opsset__hint">
              {{ t('admin.ops.settings.recipientsHint') }}
            </p>
          </div>

          <div v-if="emailConfig.report.enabled" class="opsset__grid">
            <div class="opsset__row">
              <label class="opsset__row-label">{{ t('admin.ops.settings.dailySummary') }}</label>
              <Toggle v-model="emailConfig.report.daily_summary_enabled" />
            </div>
            <div v-if="emailConfig.report.daily_summary_enabled">
              <Input v-model="emailConfig.report.daily_summary_schedule" placeholder="0 9 * * *" />
            </div>
            <div class="opsset__row">
              <label class="opsset__row-label">{{ t('admin.ops.settings.weeklySummary') }}</label>
              <Toggle v-model="emailConfig.report.weekly_summary_enabled" />
            </div>
            <div v-if="emailConfig.report.weekly_summary_enabled">
              <Input v-model="emailConfig.report.weekly_summary_schedule" placeholder="0 9 * * 1" />
            </div>
          </div>
        </div>
      </div>

      <!-- 指标阈值配置 -->
      <div class="opsset__section">
        <h4 class="opsset__title">{{ t('admin.ops.settings.metricThresholds') }}</h4>
        <p class="opsset__lead">{{ t('admin.ops.settings.metricThresholdsHint') }}</p>

        <div class="opsset__stack">
          <div>
            <NumberField
              :label="t('admin.ops.settings.slaMinPercent')"
              v-model="metricThresholds.sla_percent_min"
              :min="0"
              :max="100"
              :step="0.1"
              :hint="t('admin.ops.settings.slaMinPercentHint')"
            />
          </div>


          <div>
            <NumberField
              :label="t('admin.ops.settings.ttftP99MaxMs')"
              v-model="metricThresholds.ttft_p99_ms_max"
              :min="0"
              :step="50"
              :hint="t('admin.ops.settings.ttftP99MaxMsHint')"
            />
          </div>

          <div>
            <NumberField
              :label="t('admin.ops.settings.requestErrorRateMaxPercent')"
              v-model="metricThresholds.request_error_rate_percent_max"
              :min="0"
              :max="100"
              :step="0.1"
              :hint="t('admin.ops.settings.requestErrorRateMaxPercentHint')"
            />
          </div>

          <div>
            <NumberField
              :label="t('admin.ops.settings.upstreamErrorRateMaxPercent')"
              v-model="metricThresholds.upstream_error_rate_percent_max"
              :min="0"
              :max="100"
              :step="0.1"
              :hint="t('admin.ops.settings.upstreamErrorRateMaxPercentHint')"
            />
          </div>
        </div>
      </div>

      <!-- 高级设置 -->
      <details class="opsset__advanced">
        <summary class="opsset__advanced-summary">
          {{ t('admin.ops.settings.advancedSettings') }}
        </summary>
        <div class="space-y-4 px-4 pb-4">
          <!-- 数据保留策略 -->
          <div class="space-y-3">
            <h5 class="opsset__subtitle">{{ t('admin.ops.settings.dataRetention') }}</h5>

            <div class="opsset__row">
              <label class="opsset__row-label">{{ t('admin.ops.settings.enableCleanup') }}</label>
              <Toggle v-model="advancedSettings.data_retention.cleanup_enabled" />
            </div>

            <div v-if="advancedSettings.data_retention.cleanup_enabled">
              <Input
                v-model="advancedSettings.data_retention.cleanup_schedule"
                :label="t('admin.ops.settings.cleanupSchedule')"
                :hint="t('admin.ops.settings.cleanupScheduleHint')"
                placeholder="0 2 * * *"
              />
            </div>

            <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
              <div>
                <NumberField
              :label="t('admin.ops.settings.errorLogRetentionDays')"
              v-model="advancedSettings.data_retention.error_log_retention_days"
              :min="0"
              :max="365"
            /></div>
              <div>
                <NumberField
              :label="t('admin.ops.settings.minuteMetricsRetentionDays')"
              v-model="advancedSettings.data_retention.minute_metrics_retention_days"
              :min="0"
              :max="365"
            /></div>
              <div>
                <NumberField
              :label="t('admin.ops.settings.hourlyMetricsRetentionDays')"
              v-model="advancedSettings.data_retention.hourly_metrics_retention_days"
              :min="0"
              :max="365"
            /></div>
            </div>
            <p class="opsset__hint">{{ t('admin.ops.settings.retentionDaysHint') }}</p>
          </div>

          <!-- 预聚合任务 -->
          <div class="space-y-3">
            <h5 class="opsset__subtitle">{{ t('admin.ops.settings.aggregation') }}</h5>

            <div class="opsset__row">
              <div>
                <label class="opsset__row-label">{{ t('admin.ops.settings.enableAggregation') }}</label>
                <p class="opsset__hint">{{ t('admin.ops.settings.aggregationHint') }}</p>
              </div>
              <Toggle v-model="advancedSettings.aggregation.aggregation_enabled" />
            </div>
          </div>

          <!-- OpenAI 账号配额自动暂停（全局默认阈值） -->
          <div class="space-y-3">
            <h5 class="opsset__subtitle">{{ t('admin.ops.settings.openaiQuotaAutoPause') }}</h5>
            <p class="opsset__hint">{{ t('admin.ops.settings.openaiQuotaAutoPauseHint') }}</p>

            <div class="opsset__grid">
              <div>
                <NumberField
                  v-model="quotaAutoPause5hPercent"
                  :label="t('admin.ops.settings.openaiQuotaAutoPauseDefault5h')"
                  :min="0"
                  :max="100"
                  :step="0.1"
                  data-testid="ops-quota-auto-pause-5h"
                />
              </div>
              <div>
                <NumberField
                  v-model="quotaAutoPause7dPercent"
                  :label="t('admin.ops.settings.openaiQuotaAutoPauseDefault7d')"
                  :min="0"
                  :max="100"
                  :step="0.1"
                  data-testid="ops-quota-auto-pause-7d"
                />
              </div>
            </div>
            <p class="opsset__hint">{{ t('admin.ops.settings.openaiQuotaAutoPauseThresholdHint') }}</p>
          </div>

          <!-- Error Filtering -->
          <div class="space-y-3">
            <h5 class="opsset__subtitle">{{ t('admin.ops.settings.errorFiltering') }}</h5>

            <div class="opsset__row">
              <div>
                <label class="opsset__row-label">{{ t('admin.ops.settings.ignoreCountTokensErrors') }}</label>
                <p class="opsset__hint">
                  {{ t('admin.ops.settings.ignoreCountTokensErrorsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_count_tokens_errors" />
            </div>

            <div class="opsset__row">
              <div>
                <label class="opsset__row-label">{{ t('admin.ops.settings.ignoreContextCanceled') }}</label>
                <p class="opsset__hint">
                  {{ t('admin.ops.settings.ignoreContextCanceledHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_context_canceled" />
            </div>

            <div class="opsset__row">
              <div>
                <label class="opsset__row-label">{{ t('admin.ops.settings.ignoreNoAvailableAccounts') }}</label>
                <p class="opsset__hint">
                  {{ t('admin.ops.settings.ignoreNoAvailableAccountsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_no_available_accounts" />
            </div>

            <div class="opsset__row">
              <div>
                <label class="opsset__row-label">{{ t('admin.ops.settings.ignoreInsufficientBalanceErrors') }}</label>
                <p class="opsset__hint">
                  {{ t('admin.ops.settings.ignoreInsufficientBalanceErrorsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_insufficient_balance_errors" />
            </div>
          </div>

          <!-- Auto Refresh -->
          <div class="space-y-3">
            <h5 class="opsset__subtitle">{{ t('admin.ops.settings.autoRefresh') }}</h5>

            <div class="opsset__row">
              <div>
                <label class="opsset__row-label">{{ t('admin.ops.settings.enableAutoRefresh') }}</label>
                <p class="opsset__hint">
                  {{ t('admin.ops.settings.enableAutoRefreshHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.auto_refresh_enabled" />
            </div>

            <div v-if="advancedSettings.auto_refresh_enabled">
              <label class="opsset__label">{{ t('admin.ops.settings.refreshInterval') }}</label>
              <Select
                v-model="advancedSettings.auto_refresh_interval_seconds"
                :options="[
                  { value: 15, label: t('admin.ops.settings.refreshInterval15s') },
                  { value: 30, label: t('admin.ops.settings.refreshInterval30s') },
                  { value: 60, label: t('admin.ops.settings.refreshInterval60s') }
                ]"
              />
            </div>
          </div>

          <!-- Dashboard Cards -->
          <div class="space-y-3">
            <h5 class="opsset__subtitle">{{ t('admin.ops.settings.dashboardCards') }}</h5>

            <div class="opsset__row">
              <div>
                <label class="opsset__row-label">{{ t('admin.ops.settings.displayAlertEvents') }}</label>
                <p class="opsset__hint">
                  {{ t('admin.ops.settings.displayAlertEventsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.display_alert_events" />
            </div>

            <div class="opsset__row">
              <div>
                <label class="opsset__row-label">{{ t('admin.ops.settings.displayOpenAITokenStats') }}</label>
                <p class="opsset__hint">
                  {{ t('admin.ops.settings.displayOpenAITokenStatsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.display_openai_token_stats" />
            </div>
          </div>
        </div>
      </details>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving || !validation.valid" @click="saveAllSettings">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<style scoped>
.opsset__body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.opsset__loading {
  padding: 40px 0;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  text-align: center;
}

/* --- section ----------------------------------------------------------- */

/* --r-md, not the old rounded-2xl. A panel nested inside a dialog takes a
   smaller radius than the dialog around it (concentric radii: the inner
   corner is always tighter than the outer, or the gap between them pinches). */
.opsset__section {
  padding: 14px 16px;
  border-radius: var(--r-md);
  background: var(--surface-subtle);
}

.opsset__title {
  margin: 0 0 10px;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
}

.opsset__subtitle {
  margin: 0;
  color: var(--body-copy);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}

.opsset__lead {
  margin: -4px 0 12px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.opsset__hint {
  margin: 6px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.opsset__label {
  display: block;
  margin-bottom: 5px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.opsset__stack {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.opsset__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
  align-items: end;
}

/* --- a setting and its switch ------------------------------------------ */

.opsset__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.opsset__row-label {
  color: var(--foreground);
  font-size: var(--fs-md);
}

/* --- recipient entry ---------------------------------------------------- */

.opsset__inline {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.opsset__inline-field {
  flex: 1;
  min-width: 0;
}

.opsset__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

/* Was bg-blue-100/text-blue-700. An address someone typed is not a state, and
   blue is not in this palette. */
.opsset__chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 6px 2px 10px;
  border-radius: var(--r-pill);
  background: var(--card);
  color: var(--body-copy);
  font-size: var(--fs-sm);
}

.opsset__chip-remove {
  display: inline-grid;
  place-items: center;
  width: 16px;
  height: 16px;
  border: 0;
  border-radius: 50%;
  background: none;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  line-height: 1;
  cursor: pointer;
  transition: background var(--motion-hover);
}

.opsset__chip-remove:hover {
  background: var(--muted);
  color: var(--destructive);
}

/* --- validation --------------------------------------------------------- */

/* Attention, not destructive: the form is not broken, it is telling you a
   value will not be accepted yet. */
.opsset__invalid {
  padding: 10px 12px;
  border-radius: var(--r-md);
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
  font-size: var(--fs-sm);
}

.opsset__invalid-title {
  margin: 0;
  font-weight: var(--fw-medium);
}

.opsset__invalid-list {
  margin: 4px 0 0;
  padding-left: 18px;
  list-style: disc;
}

.opsset__invalid-list li + li {
  margin-top: 2px;
}

/* --- advanced disclosure ------------------------------------------------ */

.opsset__advanced {
  border-radius: var(--r-md);
  background: var(--surface-subtle);
}

.opsset__advanced-summary {
  padding: 12px 16px;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
  cursor: pointer;
}

.opsset__advanced-summary::marker {
  color: var(--muted-foreground);
}
</style>
