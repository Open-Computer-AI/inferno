<template>
  <section class="dashboard-panel">
    <header class="dashboard-panel__header">
      <h2 class="dashboard-panel__title">{{ t('dashboard.recentUsage') }}</h2>
      <span class="dashboard-panel__badge">{{ t('dashboard.last7Days') }}</span>
    </header>
    <div>
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner size="lg" />
      </div>
      <div v-else-if="data.length === 0" class="py-8">
        <EmptyState :title="t('dashboard.noUsageRecords')" :description="t('dashboard.startUsingApi')" />
      </div>
      <div v-else class="dashboard-list">
        <div v-for="log in data" :key="log.id" class="dashboard-list__row">
          <div class="dashboard-list__identity">
            <div class="dashboard-list__icon">
              <Icon name="beaker" size="md" />
            </div>
            <div class="dashboard-list__copy">
              <p class="dashboard-list__title">{{ log.model }}</p>
              <p class="dashboard-list__meta">{{ formatDateTime(log.created_at) }}</p>
            </div>
          </div>
          <div class="dashboard-list__value">
            <p class="dashboard-list__costline">
              <span class="dashboard-list__cost" :title="t('dashboard.actual')">${{ formatCost(log.actual_cost) }}</span>
              <span class="dashboard-list__standard" :title="t('dashboard.standard')"> / ${{ formatCost(log.total_cost) }}</span>
            </p>
            <p class="dashboard-list__meta">{{ (log.input_tokens + log.output_tokens).toLocaleString() }} tokens</p>
          </div>
        </div>

        <router-link to="/usage" class="dashboard-list__link">
          {{ t('dashboard.viewAllUsage') }}
          <Icon name="arrowRight" size="sm" />
        </router-link>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { UsageLog } from '@/types'

defineProps<{
  data: UsageLog[]
  loading: boolean
}>()
const { t } = useI18n()
const formatCost = (c: number) => c.toFixed(4)
</script>

<style scoped>
.dashboard-panel__badge{display:inline-flex;align-items:center;min-height:var(--control-xs);padding:0 8px;border-radius:var(--r-pill);background:var(--muted);color:var(--muted-foreground);font-size:var(--fs-xs);white-space:nowrap}
.dashboard-list{display:grid;gap:6px}
.dashboard-list__row{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:5px 10px;border-radius:var(--r-md);background:var(--surface-subtle);transition:background var(--motion-hover)}
.dashboard-list__row:hover{background:var(--sidebar-accent)}
.dashboard-list__identity{display:flex;align-items:center;gap:8px;min-width:0}
.dashboard-list__icon{display:grid;place-items:center;flex:none;width:24px;height:24px;border-radius:var(--r-sm);background:var(--muted);color:var(--muted-foreground)}
.dashboard-list__copy{display:flex;align-items:baseline;gap:8px;min-width:0}
.dashboard-list__title{margin:0;overflow:hidden;color:var(--foreground);font-size:var(--fs-sm);font-weight:var(--fw-medium);text-overflow:ellipsis;white-space:nowrap}
.dashboard-list__meta{margin:0;color:var(--muted-foreground);font-size:var(--fs-xs);white-space:nowrap}
.dashboard-list__value{display:flex;align-items:baseline;gap:8px;text-align:right;white-space:nowrap}
.dashboard-list__value p{margin:0}
.dashboard-list__costline{display:inline-flex;align-items:baseline}
.dashboard-list__cost{color:var(--success);font-size:var(--fs-sm);font-weight:var(--fw-medium)}
.dashboard-list__standard{color:var(--muted-foreground);font-size:var(--fs-sm)}
.dashboard-list__link{display:flex;align-items:center;justify-content:center;gap:8px;padding:6px;color:var(--brand);font-size:var(--fs-sm);font-weight:var(--fw-medium);text-decoration:none}
.dashboard-list__link:hover{color:var(--warm-strong)}
@media (max-width:640px){.dashboard-list__row{gap:8px;padding-inline:8px}.dashboard-list__copy{gap:4px}.dashboard-list__copy .dashboard-list__meta{display:none}.dashboard-list__value{gap:4px}.dashboard-list__standard{display:none}}
</style>
