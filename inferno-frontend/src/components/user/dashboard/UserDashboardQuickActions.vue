<template>
  <section class="dashboard-panel">
    <header class="dashboard-panel__header">
      <h2 class="dashboard-panel__title">{{ t('dashboard.quickActions') }}</h2>
    </header>
    <div class="dashboard-action-list">
      <button @click="router.push('/keys')" class="dashboard-action">
        <div class="dashboard-action__icon">
          <Icon name="key" size="md" />
        </div>
        <div class="dashboard-action__copy">
          <p>{{ t('dashboard.createApiKey') }}</p>
          <span>{{ t('dashboard.generateNewKey') }}</span>
        </div>
        <Icon name="chevronRight" size="sm" class="dashboard-action__chevron" />
      </button>

      <button @click="router.push('/usage')" class="dashboard-action">
        <div class="dashboard-action__icon">
          <Icon name="chart" size="md" />
        </div>
        <div class="dashboard-action__copy">
          <p>{{ t('dashboard.viewUsage') }}</p>
          <span>{{ t('dashboard.checkDetailedLogs') }}</span>
        </div>
        <Icon name="chevronRight" size="sm" class="dashboard-action__chevron" />
      </button>

      <button v-if="canUseBatchImage" @click="router.push('/batch-image')" class="dashboard-action">
        <div class="dashboard-action__icon">
          <Icon name="sparkles" size="md" />
        </div>
        <div class="dashboard-action__copy">
          <p>{{ t('dashboard.batchImageAgent') }}</p>
          <span>{{ t('dashboard.batchImageAgentDesc') }}</span>
        </div>
        <Icon name="chevronRight" size="sm" class="dashboard-action__chevron" />
      </button>

      <button @click="router.push('/redeem')" class="dashboard-action">
        <div class="dashboard-action__icon">
          <Icon name="gift" size="md" />
        </div>
        <div class="dashboard-action__copy">
          <p>{{ t('dashboard.redeemCode') }}</p>
          <span>{{ t('dashboard.addBalanceWithCode') }}</span>
        </div>
        <Icon name="chevronRight" size="sm" class="dashboard-action__chevron" />
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'
const router = useRouter()
const { t } = useI18n()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()

onMounted(() => {
  void refreshBatchImageAccess()
})
</script>

<style scoped>
.dashboard-action-list{display:grid;gap:8px}
.dashboard-action{display:flex;align-items:center;gap:12px;width:100%;padding:12px;border:0;border-radius:var(--r-md);background:var(--surface-subtle);color:var(--foreground);text-align:left;cursor:pointer;transition:background var(--motion-hover)}
.dashboard-action:hover{background:var(--sidebar-accent)}
.dashboard-action__icon{display:grid;place-items:center;flex:none;width:32px;height:32px;border-radius:var(--r-sm);background:var(--muted);color:var(--muted-foreground)}
.dashboard-action__copy{min-width:0;flex:1}
.dashboard-action__copy p{margin:0;overflow:hidden;font-size:var(--fs-sm);font-weight:var(--fw-medium);text-overflow:ellipsis;white-space:nowrap}
.dashboard-action__copy span{display:block;margin-top:3px;overflow:hidden;color:var(--muted-foreground);font-size:var(--fs-xs);text-overflow:ellipsis;white-space:nowrap}
.dashboard-action__chevron{flex:none;color:var(--muted-foreground)}
</style>
