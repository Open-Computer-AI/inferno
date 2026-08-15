<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-[var(--brand-line-strong)]"></div>
      </div>

      <template v-else>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.title') }}</h1>
            <p class="mt-1 text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.description') }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <Button type="button" :loading="statusLoading" :disabled="statusLoading" @click="loadStatus(false)">
              <Icon v-if="!statusLoading" name="refresh" size="sm" />
              {{ t('admin.riskControl.refreshStatus') }}
            </Button>
            <Button type="button" variant="solid" @click="openSettings">
              <Icon name="cog" size="sm" />
              {{ t('admin.riskControl.openSettings') }}
            </Button>
          </div>
        </div>

        <div class="grid grid-cols-2 items-start gap-4 lg:grid-cols-4">
          <div
            v-for="item in overviewItems"
            :key="item.key"
            class="min-w-0"
          >
            <StatTile
              :label="item.label"
              :value="item.value"
              :context="item.context"
              :context-tone="item.contextTone"
              :icon="item.icon"
              :tone="item.tone"
            />
          </div>
        </div>

        <div
          v-if="showPreBlockRuntimeCard"
          data-test="pre-block-runtime-cards"
          class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,520px)_minmax(0,1fr)]"
        >
          <div data-test="pre-block-sync-card" class="risk-panel">
            <div class="flex flex-col gap-4 border-b border-[var(--brand-line)] px-6 py-4 dark:border-[var(--border-subtle)] lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.preBlockSyncStatus') }}</h2>
                <p class="mt-1 text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.preBlockSyncHint') }}</p>
              </div>
              <span class="inline-flex w-fit items-center whitespace-nowrap rounded-full bg-[var(--brand-tint)] px-2.5 py-1 text-xs font-[var(--fw-medium)] text-[var(--muted-foreground)] dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">
                {{ modeLabel(status?.mode ?? configForm.mode) }}
              </span>
            </div>

            <div class="p-6">
              <div data-test="pre-block-metric-grid" class="grid grid-cols-2 gap-3 md:grid-cols-3">
                <div
                  v-for="item in preBlockMetricItems"
                  :key="item.key"
                  class="rounded-lg p-4"
                  :class="item.class"
                >
                  <p class="text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ item.label }}</p>
                  <p class="mt-2 truncate text-2xl font-[var(--fw-medium)] leading-8" :class="item.valueClass">{{ item.value }}</p>
                  <p v-if="item.meta" class="mt-1 truncate text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ item.meta }}</p>
                </div>
              </div>
            </div>
          </div>

          <div data-test="pre-block-api-key-load-card" class="risk-panel">
            <div class="flex flex-col gap-4 border-b border-[var(--brand-line)] px-6 py-4 dark:border-[var(--border-subtle)] lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.preBlockAPIKeyLoad') }}</h2>
                <p class="mt-1 text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                  {{ t('admin.riskControl.preBlockAPIKeyLoadHint') }}
                </p>
              </div>
              <span class="inline-flex max-w-[280px] items-center justify-end rounded-[var(--r-lg)] bg-[var(--brand-tint)] px-3 py-1.5 text-right text-xs font-[var(--fw-medium)] leading-4 text-[var(--muted-foreground)] dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">
                {{ preBlockAPIKeyLoadSummaryText }}
              </span>
            </div>

            <div class="p-6">
              <div
                v-if="preBlockAPIKeyLoads.length > 0"
                data-test="pre-block-api-key-load-list"
                class="max-h-[280px] space-y-3 overflow-y-auto pr-1"
              >
                <div
                  v-for="item in preBlockAPIKeyLoads"
                  :key="item.key_hash || item.index"
                  class="rounded-lg bg-[var(--brand-tint)] p-3 dark:bg-[var(--card)]"
                >
                  <div class="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                    <div class="min-w-0">
                      <div class="flex min-w-0 items-center gap-2">
                        <span class="font-mono text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">#{{ item.index + 1 }}</span>
                        <span class="truncate font-mono text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">{{ item.masked || '-' }}</span>
                        <span class="h-2 w-2 flex-shrink-0 rounded-full" :class="apiKeyStatusDotClass(item.status)"></span>
                      </div>
                      <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                        {{ t('admin.riskControl.preBlockAPIKeyTotals', { total: formatNumber(item.total), success: formatNumber(item.success), errors: formatNumber(item.errors) }) }}
                      </p>
                    </div>
                    <div class="grid grid-cols-4 gap-2 text-right text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)] sm:min-w-[280px]">
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyActiveShort') }}</p>
                        <p class="mt-1 text-sm font-[var(--fw-medium)] text-[var(--brand)] dark:text-[var(--brand)]">{{ formatNumber(item.active) }}</p>
                      </div>
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyTotalShort') }}</p>
                        <p class="mt-1 text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ formatNumber(item.total) }}</p>
                      </div>
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyAvgShort') }}</p>
                        <p class="mt-1 text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ formatNumber(item.avg_latency_ms) }} ms</p>
                      </div>
                      <div>
                        <p>{{ t('admin.riskControl.preBlockKeyLastShort') }}</p>
                        <p class="mt-1 text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ formatNumber(item.last_latency_ms) }} ms</p>
                      </div>
                    </div>
                  </div>
                  <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-white dark:bg-[var(--card)]">
                    <div class="h-full rounded-full bg-[var(--brand-tint)]" :style="{ width: preBlockAPIKeyLoadWidth(item.total) }"></div>
                  </div>
                </div>
              </div>
              <p v-else class="rounded-lg bg-[var(--brand-tint)] p-4 text-sm text-[var(--muted-foreground)] dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">
                {{ t('admin.riskControl.preBlockAPIKeyLoadEmpty') }}
              </p>
            </div>
          </div>
        </div>

        <div v-if="showWorkerRuntimeCard" class="risk-panel">
          <div class="flex flex-col gap-4 border-b border-[var(--brand-line)] px-6 py-4 dark:border-[var(--border-subtle)] lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.workerStatus') }}</h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.workerStatusHint') }}</p>
            </div>
            <div class="flex flex-wrap items-center gap-2 text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
              <span>{{ t('admin.riskControl.autoRefresh') }}</span>
              <span v-if="status?.last_cleanup_at">
                {{ t('admin.riskControl.lastCleanup', { time: formatDateTime(status.last_cleanup_at) }) }}
              </span>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-6 p-6 xl:grid-cols-[minmax(0,360px)_1fr]">
            <div class="space-y-4">
              <div class="rounded-lg border border-[var(--brand-line)] p-4 dark:border-[var(--border-subtle)]">
                <div class="flex items-center justify-between gap-3">
                  <div>
                    <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.queueUsage') }}</p>
                    <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                      {{ formatNumber(status?.queue_length ?? 0) }} / {{ formatNumber(status?.queue_size ?? configForm.queue_size) }}
                    </p>
                  </div>
                  <span class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ queueUsagePercent }}</span>
                </div>
                <div class="mt-4 h-2 overflow-hidden rounded-full bg-[var(--brand-tint)] dark:bg-[var(--card)]">
                  <div class="h-full rounded-full bg-[var(--brand)] transition-[background-color,color,opacity,transform] duration-300" :style="queueUsageStyle"></div>
                </div>
              </div>

              <div class="grid grid-cols-2 gap-3">
                <div class="rounded-lg bg-[var(--brand-tint)] p-4 dark:bg-[var(--card)]">
                  <p class="text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.activeWorkers') }}</p>
                  <p class="mt-2 text-2xl font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ status?.active_workers ?? 0 }}</p>
                </div>
                <div class="rounded-lg bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] p-4 dark:bg-[color-mix(in_oklch,var(--success)_14%,var(--card))]">
                  <p class="text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.idleWorkers') }}</p>
                  <p class="mt-2 text-2xl font-[var(--fw-medium)] text-[var(--success)] dark:text-[var(--success)]">{{ status?.idle_workers ?? configForm.worker_count }}</p>
                </div>
                <div class="rounded-lg bg-[var(--brand-tint)] p-4 dark:bg-[var(--card)]">
                  <p class="text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.processed') }}</p>
                  <p class="mt-2 text-2xl font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ formatNumber(status?.processed ?? 0) }}</p>
                </div>
                <div class="rounded-lg bg-[var(--brand-tint)] p-4 dark:bg-[var(--card)]">
                  <p class="text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.droppedErrors') }}</p>
                  <p class="mt-2 text-2xl font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ formatNumber((status?.dropped ?? 0) + (status?.errors ?? 0)) }}</p>
                </div>
              </div>
            </div>

            <div>
              <div class="mb-3 flex items-center justify-between gap-3">
                <div>
                  <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.workerPool') }}</p>
                  <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                    {{ t('admin.riskControl.workerPoolMeta', { active: status?.active_workers ?? 0, idle: status?.idle_workers ?? configForm.worker_count, total: status?.worker_count ?? configForm.worker_count }) }}
                  </p>
                </div>
                <span class="inline-flex items-center rounded-full bg-[var(--brand-tint)] px-2.5 py-1 text-xs font-[var(--fw-medium)] text-[var(--muted-foreground)] dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">
                  {{ modeLabel(status?.mode ?? configForm.mode) }}
                </span>
              </div>
              <div class="grid grid-cols-2 gap-2 sm:grid-cols-4 md:grid-cols-6 xl:grid-cols-8 2xl:grid-cols-10">
                <div
                  v-for="worker in workerSlots"
                  :key="worker.id"
                  class="flex h-12 items-center justify-between rounded-lg border px-3 transition-colors"
                  :class="workerSlotClass(worker.state)"
                  :title="worker.label"
                >
                  <span class="text-sm font-[var(--fw-medium)]">#{{ worker.id }}</span>
                  <span class="h-2.5 w-2.5 rounded-full" :class="workerDotClass(worker.state)"></span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="risk-panel">
          <div class="flex flex-col gap-4 border-b border-[var(--brand-line)] px-6 py-4 dark:border-[var(--border-subtle)]">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.records') }}</h2>
                <p class="mt-1 text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.recordsHint') }}</p>
              </div>
              <Button type="button" :loading="logsLoading" :disabled="logsLoading" @click="loadLogs">
                <Icon v-if="!logsLoading" name="refresh" size="sm" />
                {{ t('admin.riskControl.refresh') }}
              </Button>
            </div>

            <div class="flex flex-col gap-2 rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] px-3 py-2 dark:border-[var(--border-subtle)] dark:bg-[var(--card)] sm:flex-row sm:items-center sm:justify-between">
              <div class="flex min-w-0 items-center gap-2 text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">
                <Icon name="filter" size="sm" class="flex-shrink-0 text-[var(--muted-foreground)]" />
                <span class="font-[var(--fw-medium)]">{{ t('admin.riskControl.modelFilter') }}</span>
                <span class="truncate text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ modelFilterSummary }}</span>
              </div>
              <div v-if="modelFilterPreviewModels.length > 0" class="flex flex-wrap gap-1.5">
                <span
                  v-for="model in modelFilterPreviewModels"
                  :key="model"
                  class="inline-flex max-w-[180px] items-center truncate rounded-md bg-white px-2 py-1 font-mono text-xs text-[var(--muted-foreground)] shadow-sm dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]"
                >
                  {{ model }}
                </span>
                <span v-if="hiddenModelFilterModelCount > 0" class="inline-flex rounded-md bg-white px-2 py-1 text-xs text-[var(--muted-foreground)] shadow-sm dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">
                  +{{ hiddenModelFilterModelCount }}
                </span>
              </div>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-6">
              <Select v-model="filters.result" :options="resultOptions" @change="reloadLogsFromFirstPage" />
              <Select v-model="filters.group_id" :options="groupFilterOptions" @change="reloadLogsFromFirstPage" />
              <Select v-model="filters.endpoint" :options="endpointOptions" @change="reloadLogsFromFirstPage" />
              <input v-model.trim="filters.search" type="search" class="field-control" :placeholder="t('admin.riskControl.filters.search')" @keyup.enter="reloadLogsFromFirstPage" />
              <input v-model="filters.from" type="datetime-local" class="field-control" :title="t('admin.riskControl.filters.from')" @change="reloadLogsFromFirstPage" />
              <input v-model="filters.to" type="datetime-local" class="field-control" :title="t('admin.riskControl.filters.to')" @change="reloadLogsFromFirstPage" />
            </div>
          </div>

          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-[var(--border-subtle)] dark:divide-[var(--border-subtle)]">
              <thead class="bg-[var(--brand-tint)] dark:bg-[var(--card)]">
                <tr>
                  <th class="px-5 py-3 text-left text-xs font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.time') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.group') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.user') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.apiKey') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.endpoint') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.result') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.highest') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.actionMeta') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.latency') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-[var(--fw-medium)] tracking-wider text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.input') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-[var(--border-subtle)] bg-white dark:divide-[var(--border-subtle)] dark:bg-[var(--card)]">
                <tr v-if="logsLoading">
                  <td colspan="10" class="px-5 py-12 text-center text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('common.loading') }}</td>
                </tr>
                <tr v-else-if="logs.length === 0">
                  <td colspan="10" class="px-5 py-12 text-center text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.emptyLogs') }}</td>
                </tr>
                <template v-else>
                  <tr v-for="row in logs" :key="row.id" class="hover:bg-[var(--brand-tint)] dark:hover:bg-[var(--card)]">
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">{{ formatDateTime(row.created_at) }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">{{ row.group_name || '-' }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">
                      <div>{{ row.user_email || '-' }}</div>
                      <div v-if="row.user_id" class="text-xs text-[var(--muted-foreground)]">UID {{ row.user_id }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">{{ row.api_key_name || '-' }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">
                      <div>{{ row.endpoint || '-' }}</div>
                      <div class="text-xs text-[var(--muted-foreground)]">{{ row.provider || '-' }} / {{ row.model || '-' }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4">
                      <span class="inline-flex rounded-md px-2 py-1 text-xs font-[var(--fw-medium)]" :class="resultBadgeClass(row)">
                        {{ resultLabel(row) }}
                      </span>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">
                      <div>{{ row.highest_category || '-' }}</div>
                      <div class="text-xs text-[var(--muted-foreground)]">{{ percent(row.highest_score) }}</div>
                      <div v-if="row.matched_keyword" class="mt-0.5 text-xs font-[var(--fw-medium)] text-[var(--destructive)] dark:text-[var(--destructive)]" :title="t('admin.riskControl.matchedKeyword') + ': ' + row.matched_keyword">
                        {{ t('admin.riskControl.matchedKeyword') }}: {{ row.matched_keyword }}
                      </div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">
                      <div>{{ violationCountText(row) }}</div>
                      <div class="text-xs text-[var(--muted-foreground)]">
                        {{ row.email_sent ? t('admin.riskControl.emailSent') : t('admin.riskControl.emailNotSent') }}
                        <span v-if="row.auto_banned"> / {{ t('admin.riskControl.autoBanned') }}</span>
                      </div>
                      <Button
                        v-if="canUnbanRow(row)"
                        variant="success"
                        size="xs"
                        icon="hgi-checkmark-circle-02"
                        class="mt-2"
                        :loading="unbanningUserID === row.user_id"
                        :disabled="unbanningUserID === row.user_id"
                        @click="unbanUser(row)"
                      >
                        {{ unbanningUserID === row.user_id ? t('common.processing') : t('admin.riskControl.unbanUser') }}
                      </Button>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">
                      <div>{{ latencyText(row.upstream_latency_ms) }}</div>
                      <div v-if="row.queue_delay_ms !== null && row.queue_delay_ms !== undefined" class="text-xs text-[var(--muted-foreground)]">
                        {{ t('admin.riskControl.queueDelay', { ms: row.queue_delay_ms }) }}
                      </div>
                    </td>
                    <td class="w-[320px] max-w-sm px-5 py-4 text-sm text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">
                      <button
                        type="button"
                        class="group flex w-full min-w-0 items-center gap-2 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-[var(--brand-tint)] dark:hover:bg-[var(--card)]"
                        :title="inputSummaryText(row)"
                        @click="openInputDetail(row)"
                      >
                        <span class="min-w-0 flex-1 truncate">{{ inputSummaryText(row) }}</span>
                        <Icon name="eye" size="xs" class="flex-shrink-0 text-[var(--muted-foreground)] transition-colors group-hover:text-[var(--brand)] dark:text-[var(--muted-foreground)]" />
                      </button>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>

          <Pagination
            v-if="pagination.total > 0"
            :page="pagination.page"
            :total="pagination.total"
            :page-size="pagination.page_size"
            @update:page="onPageChange"
            @update:pageSize="onPageSizeChange"
          />
        </div>
      </template>

      <BaseDialog :show="settingsOpen" :title="t('admin.riskControl.settingsTitle')" width="extra-wide" @close="settingsOpen = false">
        <div class="space-y-6">
          <div class="flex gap-2 overflow-x-auto border-b border-[var(--brand-line)] pb-3 dark:border-[var(--border-subtle)]">
            <Segmented
              :model-value="activeSettingsTab"
              :items="settingsTabItems"
              variant="tabs"
              aria-label="Risk control settings"
              @update:model-value="activeSettingsTab = $event as SettingsTab"
            />
          </div>

          <div v-if="activeSettingsTab === 'basic'" class="space-y-5">
            <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div class="flex items-center justify-between rounded-lg border border-[var(--brand-line)] p-4 dark:border-[var(--border-subtle)]">
                <div>
                  <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.enabled') }}</p>
                  <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.enabledHint') }}</p>
                </div>
                <Toggle v-model="configForm.enabled" />
              </div>
              <div>
                <label class="field-label">{{ t('admin.riskControl.mode') }}</label>
                <Select v-model="configForm.mode" :options="modeOptions" />
                <p class="mt-2 text-xs leading-5 text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ modeDescription(configForm.mode) }}</p>
              </div>
              <div>
                <label class="field-label">{{ t('admin.riskControl.baseUrl') }}</label>
                <input v-model.trim="configForm.base_url" type="url" class="field-control" placeholder="https://api.openai.com" />
              </div>
              <div>
                <label class="field-label">{{ t('admin.riskControl.model') }}</label>
                <input v-model.trim="configForm.model" type="text" class="field-control" placeholder="omni-moderation-latest" />
              </div>
              <div>
                <label class="field-label">{{ t('admin.riskControl.timeoutMs') }}</label>
                <input v-model.number="configForm.timeout_ms" type="number" min="500" max="30000" class="field-control" />
              </div>
              <div>
                <label class="field-label">{{ t('admin.riskControl.retryCount') }}</label>
                <input v-model.number="configForm.retry_count" type="number" min="0" max="5" class="field-control" />
              </div>
              <div>
                <label class="field-label">{{ t('admin.riskControl.sampleRate') }}</label>
                <div class="relative">
                  <input v-model.number="configForm.sample_rate" type="number" min="0" max="100" step="1" class="field-control pr-8" />
                  <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]">%</span>
                </div>
              </div>
              <div>
                <label class="field-label">{{ t('admin.riskControl.proxy') }}</label>
                <ProxySelector v-model="configForm.proxy_id" :proxies="proxies" />
                <p class="mt-2 text-xs leading-5 text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.proxyHint') }}</p>
              </div>
            </div>

            <div class="overflow-hidden rounded-xl border border-[var(--brand-line)] bg-white shadow-sm dark:border-[var(--border-subtle)] dark:bg-[var(--card)]">
              <div class="flex flex-col gap-4 border-b border-[var(--brand-line)] bg-[var(--brand-tint)] px-4 py-4 dark:border-[var(--border-subtle)] dark:bg-[var(--card)] lg:flex-row lg:items-center lg:justify-between">
                <div class="flex items-start gap-3">
                  <span class="mt-0.5 flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-[var(--brand-tint)] text-[var(--brand)] dark:bg-[var(--brand)] dark:text-[var(--brand)]">
                    <Icon name="key" size="md" />
                  </span>
                  <div>
                    <label class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.apiKeys') }}</label>
                    <p class="mt-1 max-w-3xl text-xs leading-5 text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                      {{ t('admin.riskControl.apiKeysHint', { count: configForm.api_key_count }) }}
                    </p>
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <Button
                    type="button"
                    :disabled="apiKeyTesting || inputApiKeyCount === 0 || configForm.clear_api_key"
                    :loading="apiKeyTesting"
                    @click="testApiKeys(true)"
                  >
                    <Icon name="beaker" size="sm" :class="apiKeyTesting ? 'animate-pulse' : ''" />
                    {{ apiKeyTesting ? t('admin.riskControl.testingApiKeys') : t('admin.riskControl.testInputApiKeys') }}
                  </Button>
                  <Button
                    type="button"
                    :disabled="apiKeyTesting || effectiveStoredApiKeyCount === 0 || pendingDeletedApiKeyCount > 0 || configForm.clear_api_key || configForm.api_keys_mode === 'replace'"
                    :loading="apiKeyTesting"
                    @click="testApiKeys(false)"
                  >
                    <Icon name="shield" size="sm" />
                    {{ storedApiKeyTestButtonText }}
                  </Button>
                  <Button
                    v-if="configForm.api_key_configured"
                    type="button"
                    @click="toggleClearApiKey"
                  >
                    <Icon :name="configForm.clear_api_key ? 'x' : 'trash'" size="sm" />
                    {{ configForm.clear_api_key ? t('admin.riskControl.keepApiKey') : t('admin.riskControl.clearApiKey') }}
                  </Button>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-4 p-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,440px)]">
                <div class="space-y-3">
                  <div class="flex flex-col gap-2 rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-2 dark:border-[var(--border-subtle)] dark:bg-[var(--card)] sm:flex-row sm:items-center sm:justify-between">
                    <div class="text-xs leading-5 text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                      <span class="font-[var(--fw-medium)] text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.apiKeysWriteMode') }}</span>
                      <span class="ml-2">{{ apiKeysModeHint }}</span>
                    </div>
                    <Segmented
                      :model-value="configForm.api_keys_mode"
                      :items="apiKeysModeItems"
                      aria-label="API key write mode"
                      @update:model-value="setAPIKeysMode($event as 'append' | 'replace')"
                    />
                  </div>
                  <textarea
                    v-model="configForm.api_keys_text"
                    class="field-control min-h-44 resize-y font-mono text-sm"
                    :placeholder="apiKeysPlaceholder"
                    autocomplete="new-password"
                    :disabled="configForm.clear_api_key"
                  ></textarea>
                  <div class="flex flex-wrap items-center gap-2 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                    <span class="inline-flex rounded-md bg-[var(--brand-tint)] px-2 py-1 dark:bg-[var(--card)]">
                      {{ t('admin.riskControl.inputApiKeyCount', { count: inputApiKeyCount }) }}
                    </span>
                    <span v-if="configForm.api_key_configured" class="inline-flex rounded-md bg-[var(--brand-tint)] px-2 py-1 dark:bg-[var(--card)]">
                      {{ t('admin.riskControl.storedApiKeyCount', { count: configForm.api_key_count }) }}
                    </span>
                    <span v-if="configForm.clear_api_key" class="inline-flex rounded-md bg-[var(--destructive-soft)] px-2 py-1 text-[var(--destructive)] dark:bg-[var(--destructive-soft)] dark:text-[var(--destructive)]">
                      {{ t('admin.riskControl.apiKeyWillClear') }}
                    </span>
                    <span v-else-if="pendingDeletedApiKeyCount > 0" class="inline-flex rounded-md bg-[var(--brand-tint)] px-2 py-1 text-[var(--brand)] dark:bg-[var(--brand-tint)] dark:text-[var(--brand)]">
                      {{ t('admin.riskControl.apiKeyPendingDeleteCount', { count: pendingDeletedApiKeyCount }) }}
                    </span>
                    <span v-if="configForm.api_keys_mode === 'replace'" class="inline-flex rounded-md bg-[var(--brand-tint)] px-2 py-1 text-[var(--brand)] dark:bg-[var(--brand-tint)] dark:text-[var(--brand)]">
                      {{ t('admin.riskControl.apiKeysReplaceWarning') }}
                    </span>
                  </div>

                  <div class="rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-3 dark:border-[var(--border-subtle)] dark:bg-[var(--card)]" @paste="handleModerationImagePaste">
                    <div class="mb-3 flex items-center justify-between gap-3">
                      <div>
                        <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.auditTestInput') }}</p>
                        <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.auditTestInputHint') }}</p>
                      </div>
                      <Button
                        v-if="moderationTestPrompt || moderationTestImages.length > 0 || moderationTestResult"
                        variant="ghost"
                        size="xs"
                        icon="hgi-cancel-01"
                        @click="clearModerationTestInput"
                      >
                        {{ t('admin.riskControl.clearAuditTest') }}
                      </Button>
                    </div>
                    <textarea
                      v-model="moderationTestPrompt"
                      class="field-control min-h-24 resize-y text-sm"
                      :placeholder="t('admin.riskControl.auditTestPromptPlaceholder')"
                    ></textarea>
                    <div
                      class="mt-3 rounded-lg border border-dashed border-[var(--brand-line)] bg-white p-3 dark:border-[var(--border-subtle)] dark:bg-[var(--card)]"
                      @dragover.prevent
                      @drop.prevent="handleModerationImageDrop"
                    >
                      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                        <div class="flex items-start gap-2">
                          <Icon name="upload" size="md" class="mt-0.5 text-[var(--muted-foreground)]" />
                          <div>
                            <p class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.auditTestImages') }}</p>
                            <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.auditTestImagesHint') }}</p>
                          </div>
                        </div>
                        <label class="btn2 cursor-pointer" data-variant="secondary" data-size="sm">
                          <Icon name="plus" size="sm" />
                          {{ t('admin.riskControl.addAuditTestImage') }}
                          <input type="file" accept="image/*" multiple class="sr-only" @change="handleModerationImageUpload" />
                        </label>
                      </div>
                      <div v-if="moderationTestImages.length > 0" class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                        <div
                          v-for="(image, index) in moderationTestImages"
                          :key="image.slice(0, 64) + index"
                          class="group relative aspect-square overflow-hidden rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] dark:border-[var(--border-subtle)] dark:bg-[var(--card)]"
                        >
                          <img :src="image" alt="" class="h-full w-full object-cover" />
                          <IconButton
                            icon="hgi-cancel-01"
                            :label="t('common.remove', 'Remove image')"
                            tone="danger"
                            size="xs"
                            type="button"
                            class="absolute right-1.5 top-1.5 opacity-0 transition-opacity group-hover:opacity-100"
                            @click="removeModerationTestImage(index)"
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <div class="rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-3 dark:border-[var(--border-subtle)] dark:bg-[var(--card)]">
                  <div class="mb-3 flex items-start justify-between gap-3">
                    <div class="min-w-0">
                      <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.apiKeyHealth') }}</p>
                      <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.apiKeyFreezeRule') }}</p>
                    </div>
                    <span class="inline-flex shrink-0 items-center whitespace-nowrap rounded-full bg-white px-2 py-0.5 text-[11px] font-[var(--fw-medium)] leading-5 text-[var(--muted-foreground)] shadow-sm dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">
                      {{ t('admin.riskControl.apiKeyRows', { count: apiKeyRows.length }) }}
                    </span>
                  </div>

                  <div v-if="apiKeyRows.length === 0" class="flex min-h-32 flex-col items-center justify-center rounded-lg border border-dashed border-[var(--brand-line)] bg-white px-4 py-6 text-center dark:border-[var(--border-subtle)] dark:bg-[var(--card)]">
                    <Icon name="infoCircle" size="lg" class="text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]" />
                    <p class="mt-2 text-sm font-[var(--fw-medium)] text-[var(--body-copy)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.apiKeyHealthEmpty') }}</p>
                    <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.apiKeyHealthEmptyHint') }}</p>
                  </div>
                  <div v-else class="space-y-2">
                    <div class="space-y-2" :class="apiKeyRowsExpanded ? 'max-h-72 overflow-y-auto pr-1' : ''">
                      <div
                        v-for="(row, index) in visibleApiKeyRows"
                        :key="apiKeyRowKey(row, index)"
                        class="rounded-lg border bg-white p-2.5 shadow-sm dark:bg-[var(--card)]"
                        :class="isStoredApiKeyPendingDelete(row) ? 'border-[var(--brand-line)] opacity-70 dark:border-[var(--brand-line)]' : 'border-[var(--brand-line)] dark:border-[var(--border-subtle)]'"
                      >
                        <div class="flex items-start justify-between gap-2">
                          <div class="min-w-0">
                            <div class="flex min-w-0 flex-wrap items-center gap-2">
                              <span class="truncate font-mono text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ row.masked || '-' }}</span>
                              <span
                                class="inline-flex rounded-md px-1.5 py-0.5 text-[11px] font-[var(--fw-medium)]"
                                :class="row.configured ? 'bg-[var(--brand-tint)] text-[var(--brand)] dark:bg-[var(--brand)] dark:text-[var(--brand)]' : 'bg-[var(--brand-tint)] text-[var(--brand)] dark:bg-[var(--brand-tint)] dark:text-[var(--brand)]'"
                              >
                                {{ isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.apiKeyPendingDelete') : row.configured ? t('admin.riskControl.apiKeyConfigured') : t('admin.riskControl.apiKeyTemporary') }}
                              </span>
                            </div>
                            <p class="mt-1 text-xs leading-5 text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ apiKeyStatusMeta(row) }}</p>
                          </div>
                          <div class="flex flex-shrink-0 items-center gap-1.5">
                            <span class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-[var(--fw-medium)]" :class="apiKeyStatusBadgeClass(row.status)">
                              <span class="h-1.5 w-1.5 rounded-full" :class="apiKeyStatusDotClass(row.status)"></span>
                              {{ apiKeyStatusLabel(row.status) }}
                            </span>
                            <IconButton
                              v-if="row.configured && !configForm.clear_api_key"
                              :icon="isStoredApiKeyPendingDelete(row) ? 'hgi-arrow-reload-horizontal' : 'hgi-delete-01'"
                              :label="isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.undoDeleteApiKey') : t('admin.riskControl.deleteApiKey')"
                              size="xs"
                              @click="toggleDeleteStoredApiKey(row)"
                            />
                          </div>
                        </div>
                        <p v-if="row.last_error" class="mt-1.5 rounded-md bg-[var(--brand-tint)] px-2 py-1.5 text-xs leading-5 text-[var(--brand)] dark:bg-[var(--brand-tint)] dark:text-[var(--brand)]">
                          {{ row.last_error }}
                        </p>
                      </div>
                    </div>

                    <div v-if="canToggleApiKeyRows" class="flex items-center justify-between gap-3 rounded-lg border border-dashed border-[var(--brand-line)] bg-white px-3 py-2 text-xs text-[var(--muted-foreground)] dark:border-[var(--border-subtle)] dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">
                      <span class="min-w-0 truncate">
                        {{ apiKeyRowsExpanded ? t('admin.riskControl.apiKeyRowsExpanded', { count: apiKeyRows.length }) : t('admin.riskControl.apiKeyRowsCollapsed', { count: hiddenApiKeyRowCount }) }}
                      </span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="xs"
                        :icon="apiKeyRowsExpanded ? 'hgi-arrow-up-01' : 'hgi-arrow-down-01'"
                        @click="apiKeyRowsExpanded = !apiKeyRowsExpanded"
                      >
                        {{ apiKeyRowsExpanded ? t('admin.riskControl.collapseApiKeyRows') : t('admin.riskControl.expandApiKeyRows') }}
                      </Button>
                    </div>
                  </div>

                  <div v-if="moderationTestResult" class="mt-4 rounded-lg border border-[var(--brand-line)] bg-white p-3 dark:border-[var(--border-subtle)] dark:bg-[var(--card)]">
                    <div class="flex items-start justify-between gap-3">
                      <div>
                        <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.auditTestResult') }}</p>
                        <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                          {{ t('admin.riskControl.auditTestHighest', { category: moderationTestResult.highest_category || '-', score: percent(moderationTestResult.highest_score) }) }}
                        </p>
                      </div>
                      <span class="inline-flex rounded-full px-2 py-1 text-xs font-[var(--fw-medium)]" :class="moderationTestResult.flagged ? 'bg-[var(--destructive-soft)] text-[var(--destructive)] dark:bg-[var(--destructive-soft)] dark:text-[var(--destructive)]' : 'bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] text-[var(--success)] dark:bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] dark:text-[var(--success)]'">
                        {{ moderationTestResult.flagged ? t('admin.riskControl.auditTestFlagged') : t('admin.riskControl.auditTestPassed') }}
                      </span>
                    </div>
                    <div class="mt-3">
                      <div class="mb-2 flex items-center justify-between text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                        <span>{{ t('admin.riskControl.auditTestComposite') }}</span>
                        <span class="font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ percent(moderationTestResult.composite_score) }}</span>
                      </div>
                      <div class="h-2 overflow-hidden rounded-full bg-[var(--brand-tint)] dark:bg-[var(--card)]">
                        <div class="h-full rounded-full" :class="moderationTestResult.flagged ? 'bg-[var(--destructive-soft)]' : 'bg-[color-mix(in_oklch,var(--success)_14%,var(--card))]'" :style="{ width: percentWidth(moderationTestResult.composite_score) }"></div>
                      </div>
                    </div>
                    <div class="mt-3 max-h-52 space-y-2 overflow-y-auto pr-1">
                      <div v-for="score in moderationScoreRows" :key="score.category">
                        <div class="mb-1 flex items-center justify-between gap-3 text-xs">
                          <span class="truncate text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ score.category }}</span>
                          <span class="font-mono text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ percent(score.score) }} / {{ percent(score.threshold) }}</span>
                        </div>
                        <div class="h-1.5 overflow-hidden rounded-full bg-[var(--brand-tint)] dark:bg-[var(--card)]">
                          <div class="h-full rounded-full" :class="score.hit ? 'bg-[var(--destructive-soft)]' : 'bg-[var(--brand)]'" :style="{ width: percentWidth(score.score) }"></div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'scope'" class="space-y-5">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h3 class="text-base font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.groupScope') }}</h3>
                <p class="mt-1 text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.groupScopeHint') }}</p>
              </div>
              <div class="inline-flex rounded-lg bg-[var(--brand-tint)] p-1 dark:bg-[var(--card)]">
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-[var(--fw-medium)] transition-colors"
                  :class="configForm.all_groups ? 'bg-white text-[var(--foreground)] shadow-sm dark:bg-[var(--card)] dark:text-[var(--foreground)]' : 'text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]'"
                  @click="configForm.all_groups = true"
                >
                  {{ t('admin.riskControl.allGroups') }}
                </button>
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-[var(--fw-medium)] transition-colors"
                  :class="!configForm.all_groups ? 'bg-white text-[var(--foreground)] shadow-sm dark:bg-[var(--card)] dark:text-[var(--foreground)]' : 'text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]'"
                  @click="configForm.all_groups = false"
                >
                  {{ t('admin.riskControl.selectedGroups') }}
                </button>
              </div>
            </div>

            <div v-if="!configForm.all_groups" class="space-y-4">
              <div class="relative">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]" />
                <input v-model.trim="groupSearch" type="search" class="field-control pl-9" :placeholder="t('admin.riskControl.searchGroups')" />
              </div>
              <div class="grid max-h-[420px] grid-cols-1 gap-3 overflow-y-auto pr-1 md:grid-cols-2 xl:grid-cols-3">
                <button
                  v-for="group in filteredGroups"
                  :key="group.id"
                  type="button"
                  class="flex min-h-20 items-center justify-between rounded-lg border p-4 text-left transition-colors"
                  :class="isGroupSelected(group.id) ? 'border-[var(--brand-line)] bg-[var(--brand-tint)] dark:border-[var(--brand-line-strong)] dark:bg-[var(--brand)]' : 'border-[var(--brand-line)] hover:bg-[var(--brand-tint)] dark:border-[var(--border-subtle)] dark:hover:bg-[var(--card)]'"
                  @click="toggleGroup(group.id)"
                >
                  <span class="min-w-0">
                    <span class="block truncate text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ group.name }}</span>
                    <span class="mt-1 inline-flex rounded-md bg-[var(--brand-tint)] px-2 py-0.5 text-xs text-[var(--muted-foreground)] dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">{{ group.platform }}</span>
                  </span>
                  <span
                    class="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full border"
                    :class="isGroupSelected(group.id) ? 'border-[var(--brand-line-strong)] bg-[var(--brand)] text-white' : 'border-[var(--brand-line)] text-transparent dark:border-[var(--border-subtle)]'"
                  >
                    <Icon name="check" size="xs" :stroke-width="2" />
                  </span>
                </button>
                <p v-if="filteredGroups.length === 0" class="text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.noGroups') }}</p>
              </div>
            </div>

            <div class="space-y-4 rounded-lg border border-[var(--brand-line)] p-4 dark:border-[var(--border-subtle)]">
              <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <h3 class="text-base font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.modelFilter') }}</h3>
                  <p class="mt-1 text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.modelFilterHint') }}</p>
                </div>
                <span class="inline-flex w-fit rounded-md bg-[var(--brand-tint)] px-2.5 py-1 text-xs font-[var(--fw-medium)] text-[var(--muted-foreground)] dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">
                  {{ modelFilterSummary }}
                </span>
              </div>

              <div class="grid grid-cols-1 gap-2 md:grid-cols-3">
                <button
                  v-for="option in modelFilterOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-lg border p-3 text-left transition-colors"
                  :class="configForm.model_filter_type === option.value
                    ? 'border-[var(--brand-line)] bg-[var(--brand-tint)] text-[var(--brand)] shadow-sm dark:border-[var(--brand-line-strong)] dark:bg-[var(--brand)] dark:text-[var(--brand)]'
                    : 'border-[var(--brand-line)] hover:bg-[var(--brand-tint)] dark:border-[var(--border-subtle)] dark:hover:bg-[var(--card)]'"
                  @click="setModelFilterType(option.value)"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-sm font-[var(--fw-medium)]">{{ option.label }}</span>
                    <span
                      class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
                      :class="configForm.model_filter_type === option.value
                        ? 'border-[var(--brand-line-strong)] bg-[var(--brand)] text-white'
                        : 'border-[var(--brand-line)] text-transparent dark:border-[var(--border-subtle)]'"
                    >
                      <Icon name="check" size="xs" :stroke-width="2" />
                    </span>
                  </div>
                  <p class="mt-1 text-xs leading-5 text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ option.description }}</p>
                </button>
              </div>

              <div v-if="configForm.model_filter_type !== 'all'" class="space-y-2">
                <label class="field-label">{{ t('admin.riskControl.modelFilterModels') }}</label>
                <ModelWhitelistSelector v-model="configForm.model_filter_models" />
                <p class="text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                  {{ t('admin.riskControl.modelFilterModelCount', { count: modelFilterModelCount }) }}
                </p>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'runtime'" class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="field-label">{{ t('admin.riskControl.workerCount') }}</label>
              <input v-model.number="configForm.worker_count" type="number" min="1" max="32" class="field-control" />
            </div>
            <div>
              <label class="field-label">{{ t('admin.riskControl.queueSize') }}</label>
              <input v-model.number="configForm.queue_size" type="number" min="100" max="100000" class="field-control" />
            </div>
            <div class="flex items-center justify-between rounded-lg border border-[var(--brand-line)] p-4 dark:border-[var(--border-subtle)] lg:col-span-2">
              <div>
                <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.recordNonHits') }}</p>
                <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.recordNonHitsHint') }}</p>
              </div>
              <Toggle v-model="configForm.record_non_hits" />
            </div>
            <div class="space-y-4 rounded-lg border border-[var(--brand-line)] p-4 dark:border-[var(--border-subtle)] lg:col-span-2">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.preHashCheck') }}</p>
                  <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.preHashCheckHint') }}</p>
                </div>
                <Toggle v-model="configForm.pre_hash_check_enabled" />
              </div>
              <div class="rounded-lg bg-[var(--brand-tint)] p-3 dark:bg-[var(--card)]">
                <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
                  <div>
                    <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">
                      {{ t('admin.riskControl.flaggedHashCount', { count: formatNumber(status?.flagged_hash_count ?? 0) }) }}
                    </p>
                    <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.flaggedHashHint') }}</p>
                  </div>
                  <Button
                    type="button"
                    class="text-[var(--destructive)]"
                    :disabled="hashActionLoading || (status?.flagged_hash_count ?? 0) === 0"
                    :loading="hashActionLoading"
                    @click="clearFlaggedHashes"
                  >
                    <Icon name="trash" size="sm" :class="hashActionLoading ? 'animate-pulse' : ''" />
                    {{ t('admin.riskControl.clearFlaggedHashes') }}
                  </Button>
                </div>
                <div class="mt-3 flex flex-col gap-2 sm:flex-row">
                  <input
                    v-model.trim="flaggedHashInput"
                    type="text"
                    class="field-control font-mono text-sm"
                    :placeholder="t('admin.riskControl.flaggedHashPlaceholder')"
                  />
                  <Button
                    type="button"
                    :disabled="hashActionLoading || !isFlaggedHashInputValid"
                    :loading="hashActionLoading"
                    @click="deleteFlaggedHash"
                  >
                    <Icon name="trash" size="sm" />
                    {{ t('admin.riskControl.deleteFlaggedHash') }}
                  </Button>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'response'" class="space-y-5">
            <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div>
                <label class="field-label">{{ t('admin.riskControl.blockStatus') }}</label>
                <input v-model.number="configForm.block_status" type="number" min="400" max="599" class="field-control" />
              </div>
              <div>
                <label class="field-label">{{ t('admin.riskControl.blockMessage') }}</label>
                <input v-model.trim="configForm.block_message" type="text" class="field-control" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-[var(--brand-line)] p-4 dark:border-[var(--border-subtle)]">
                <div>
                  <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.emailOnHit') }}</p>
                  <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.emailOnHitHint') }}</p>
                </div>
                <Toggle v-model="configForm.email_on_hit" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-[var(--brand-line)] p-4 dark:border-[var(--border-subtle)]">
                <div>
                  <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.autoBan') }}</p>
                  <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.autoBanHint') }}</p>
                </div>
                <Toggle v-model="configForm.auto_ban_enabled" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-[var(--brand-line)] p-4 dark:border-[var(--border-subtle)] lg:col-span-2">
                <div>
                  <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.cyberPolicyExcludeBan') }}</p>
                  <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.cyberPolicyExcludeBanHint') }}</p>
                </div>
                <Toggle v-model="configForm.cyber_policy_exclude_from_ban_count" />
              </div>
              <div>
                <label class="field-label">{{ t('admin.riskControl.banThreshold') }}</label>
                <input v-model.number="configForm.ban_threshold" type="number" min="1" max="1000" class="field-control" />
              </div>
              <div>
                <label class="field-label">{{ t('admin.riskControl.violationWindowHours') }}</label>
                <input v-model.number="configForm.violation_window_hours" type="number" min="1" max="8760" class="field-control" />
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'riskThresholds'" class="space-y-5">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h3 class="text-base font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.riskThresholds') }}</h3>
                <p class="mt-1 text-sm text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.riskThresholdsHint') }}</p>
              </div>
              <Button
                type="button"
                @click="resetRiskThresholds"
              >
                <Icon name="refresh" size="sm" />
                {{ t('admin.riskControl.riskThresholdReset') }}
              </Button>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
              <div
                v-for="row in riskThresholdRows"
                :key="row.category"
                class="rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-4 dark:border-[var(--border-subtle)] dark:bg-[var(--card)]"
              >
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <label class="block truncate text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]" :for="`risk-threshold-${row.category}`">
                      {{ row.category }}
                    </label>
                    <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                      {{ t('admin.riskControl.riskThresholdDefault', { value: formatThresholdPercent(row.defaultValue) }) }}
                    </p>
                  </div>
                  <span class="inline-flex shrink-0 rounded-md bg-white px-2 py-1 font-mono text-xs font-[var(--fw-medium)] text-[var(--muted-foreground)] shadow-sm dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">
                    {{ formatThresholdPercent(row.value) }}
                  </span>
                </div>
                <div class="mt-3">
                  <label class="sr-only" :for="`risk-threshold-${row.category}`">
                    {{ t('admin.riskControl.riskThresholdPercent') }}
                  </label>
                  <div class="relative">
                    <input
                      :id="`risk-threshold-${row.category}`"
                      v-model.number="configForm.thresholds[row.category]"
                      :data-test="`risk-threshold-${row.category}`"
                      type="number"
                      min="0"
                      max="100"
                      step="0.1"
                      class="field-control pr-8 font-mono"
                    />
                    <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]">%</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'keywords'" class="space-y-5">
            <div
              class="flex items-start gap-3 rounded-lg border p-4"
              :class="keywordNotice.toneClass"
            >
              <Icon
                :name="keywordNotice.icon"
                size="md"
                :class="keywordNotice.iconClass"
              />
              <div class="text-sm leading-6">
                <p class="font-[var(--fw-medium)]" :class="keywordNotice.titleClass">{{ keywordNotice.title }}</p>
                <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ keywordNotice.description }}</p>
              </div>
            </div>

            <div class="space-y-2">
              <label class="field-label">{{ t('admin.riskControl.keywordBlockingMode') }}</label>
              <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
                <button
                  v-for="option in keywordBlockingModeOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-lg border p-3 text-left transition-colors"
                  :class="configForm.keyword_blocking_mode === option.value
                    ? 'border-[var(--brand-line)] bg-[var(--brand-tint)] text-[var(--brand)] shadow-sm dark:border-[var(--brand-line-strong)] dark:bg-[var(--brand)] dark:text-[var(--brand)]'
                    : 'border-[var(--brand-line)] hover:bg-[var(--brand-tint)] dark:border-[var(--border-subtle)] dark:hover:bg-[var(--card)]'"
                  @click="configForm.keyword_blocking_mode = option.value"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-sm font-[var(--fw-medium)]">{{ option.label }}</span>
                    <span
                      class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
                      :class="configForm.keyword_blocking_mode === option.value
                        ? 'border-[var(--brand-line-strong)] bg-[var(--brand)] text-white'
                        : 'border-[var(--brand-line)] text-transparent dark:border-[var(--border-subtle)]'"
                    >
                      <Icon name="check" size="xs" :stroke-width="2" />
                    </span>
                  </div>
                  <p class="mt-1 text-xs leading-5 text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ option.description }}</p>
                </button>
              </div>
            </div>

            <div>
              <div class="mb-2 flex items-center justify-between">
                <label class="field-label mb-0">{{ t('admin.riskControl.blockedKeywords') }}</label>
                <span class="inline-flex rounded-md bg-[var(--brand-tint)] px-2 py-1 text-xs text-[var(--muted-foreground)] dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]">
                  {{ t('admin.riskControl.blockedKeywordCount', { count: blockedKeywordCount }) }}
                </span>
              </div>
              <textarea
                v-model="configForm.blocked_keywords_text"
                class="field-control min-h-52 resize-y font-mono text-sm"
                :placeholder="t('admin.riskControl.blockedKeywordsPlaceholder')"
                :disabled="configForm.keyword_blocking_mode === 'api_only'"
              ></textarea>
              <p class="mt-2 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                {{ t('admin.riskControl.blockedKeywordsLimit', { max: blockedKeywordMax }) }}
              </p>
            </div>
          </div>

          <div v-else class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="field-label">{{ t('admin.riskControl.hitRetentionDays') }}</label>
              <input v-model.number="configForm.hit_retention_days" type="number" min="1" max="3650" class="field-control" />
            </div>
            <div>
              <label class="field-label">{{ t('admin.riskControl.nonHitRetentionDays') }}</label>
              <input v-model.number="configForm.non_hit_retention_days" type="number" min="1" max="3" class="field-control" />
            </div>
            <div class="rounded-lg border border-[var(--brand-line)] p-4 text-sm text-[var(--muted-foreground)] dark:border-[var(--border-subtle)] dark:text-[var(--muted-foreground)] lg:col-span-2">
              <div class="flex flex-wrap items-center gap-3">
                <Icon name="database" size="md" class="text-[var(--muted-foreground)]" />
                <span>{{ t('admin.riskControl.cleanupStats', { hit: status?.last_cleanup_deleted_hit ?? 0, nonHit: status?.last_cleanup_deleted_non_hit ?? 0 }) }}</span>
              </div>
            </div>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end gap-2">
            <Button type="button" @click="settingsOpen = false">{{ t('common.cancel') }}</Button>
            <Button type="button" variant="solid" :loading="saving" :disabled="saving" @click="saveConfig">
              {{ t('admin.riskControl.saveConfig') }}
            </Button>
          </div>
        </template>
      </BaseDialog>

      <BaseDialog
        :show="inputDetailRow !== null"
        :title="t('admin.riskControl.inputDetailTitle')"
        width="wide"
        @close="closeInputDetail"
      >
        <div v-if="inputDetailRow" class="space-y-5">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div class="rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-4 dark:border-[var(--border-subtle)] dark:bg-[var(--card)]">
              <p class="text-xs font-[var(--fw-medium)] text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.time') }}</p>
              <p class="mt-1 truncate text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ formatDateTime(inputDetailRow.created_at) }}</p>
            </div>
            <div class="rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-4 dark:border-[var(--border-subtle)] dark:bg-[var(--card)]">
              <p class="text-xs font-[var(--fw-medium)] text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.user') }}</p>
              <p class="mt-1 truncate text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ inputDetailRow.user_email || '-' }}</p>
            </div>
            <div class="rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-4 dark:border-[var(--border-subtle)] dark:bg-[var(--card)]">
              <p class="text-xs font-[var(--fw-medium)] text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.result') }}</p>
              <span class="mt-1 inline-flex rounded-md px-2 py-1 text-xs font-[var(--fw-medium)]" :class="resultBadgeClass(inputDetailRow)">
                {{ resultLabel(inputDetailRow) }}
              </span>
            </div>
            <div class="rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-4 dark:border-[var(--border-subtle)] dark:bg-[var(--card)]">
              <p class="text-xs font-[var(--fw-medium)] text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">{{ t('admin.riskControl.table.highest') }}</p>
              <p class="mt-1 truncate text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">
                {{ inputDetailRow.highest_category || '-' }} / {{ percent(inputDetailRow.highest_score) }}
              </p>
            </div>
            <div v-if="inputDetailRow.matched_keyword" class="rounded-lg border border-[var(--destructive)] bg-[var(--destructive-soft)] p-4 dark:border-[var(--destructive)] dark:bg-[var(--destructive-soft)]">
              <p class="text-xs font-[var(--fw-medium)] text-[var(--destructive)] dark:text-[var(--destructive)]">{{ t('admin.riskControl.matchedKeyword') }}</p>
              <p class="mt-1 truncate text-sm font-[var(--fw-medium)] text-[var(--destructive)] dark:text-[var(--destructive)]" :title="inputDetailRow.matched_keyword">{{ inputDetailRow.matched_keyword }}</p>
            </div>
          </div>

          <div class="rounded-xl border border-[var(--brand-line)] bg-white p-4 shadow-sm dark:border-[var(--border-subtle)] dark:bg-[var(--card)]">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-[var(--foreground)]">{{ t('admin.riskControl.inputDetailContent') }}</p>
                <p class="mt-1 text-xs text-[var(--muted-foreground)] dark:text-[var(--muted-foreground)]">
                  {{ inputDetailRow.endpoint || '-' }} · {{ inputDetailRow.provider || '-' }} / {{ inputDetailRow.model || '-' }}
                </p>
              </div>
              <span v-if="inputDetailRow.group_name" class="inline-flex rounded-md bg-[var(--brand-tint)] px-2.5 py-1 text-xs font-[var(--fw-medium)] text-[var(--brand)] dark:bg-[var(--brand-tint)] dark:text-[var(--brand)]">
                {{ inputDetailRow.group_name }}
              </span>
            </div>
            <pre class="mt-4 max-h-[420px] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-[var(--scrim)] p-4 text-sm leading-6 text-[var(--muted-foreground)] shadow-inner dark:bg-[var(--scrim)]">{{ inputDetailText }}</pre>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end">
            <Button type="button" @click="closeInputDetail">{{ t('common.close') }}</Button>
          </div>
        </template>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDialogService } from '@/composables/useDialogService'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import Segmented from '@/components/common/Segmented.vue'
import StatTile from '@/components/common/StatTile.vue'
import Toggle from '@/components/common/Toggle.vue'
import Pagination from '@/components/common/Pagination.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import { adminAPI } from '@/api/admin'
import type {
  ContentModerationAPIKeyLoad,
  ContentModerationAPIKeyStatus,
  ContentModerationConfig,
  ContentModerationLog,
  ContentModerationModelFilter,
  ContentModerationModelFilterType,
  ContentModerationRuntimeStatus,
  ContentModerationTestAuditResult,
  KeywordBlockingMode,
  ModerationMode,
  UpdateContentModerationConfig,
} from '@/api/admin/riskControl'
import type { AdminGroup, Proxy, SelectOption } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDateTimeValue } from '@/utils/format'

type SettingsTab = 'basic' | 'scope' | 'runtime' | 'response' | 'riskThresholds' | 'retention' | 'keywords'
type WorkerSlotState = 'active' | 'idle' | 'disabled'
type APIKeysWriteMode = 'append' | 'replace'
type OverviewIcon = 'shield-01' | 'key-01' | 'user-group' | 'file-02'
type OverviewTone = 'brand' | 'info' | 'success' | 'accent'
type OverviewContextTone = 'muted' | 'good' | 'attention'
type OverviewItem = {
  key: string
  label: string
  value: string
  context: string
  contextTone: OverviewContextTone
  icon: OverviewIcon
  tone: OverviewTone
}
type ModerationScoreRow = {
  category: string
  score: number
  threshold: number
  hit: boolean
}
type RiskThresholdRow = {
  category: string
  value: number
  defaultValue: number
}

const maxModerationTestImages = 1
const maxModerationTestImageSize = 8 * 1024 * 1024
const maxVisibleApiKeyRows: number = 3
const blockedKeywordMax = 10000
const riskThresholdDefaults: Record<string, number> = {
  harassment: 98,
  'harassment/threatening': 90,
  hate: 65,
  'hate/threatening': 65,
  illicit: 95,
  'illicit/violent': 95,
  'self-harm': 65,
  'self-harm/intent': 85,
  'self-harm/instructions': 65,
  sexual: 65,
  'sexual/minors': 65,
  violence: 95,
  'violence/graphic': 95,
}
const riskThresholdCategories = Object.keys(riskThresholdDefaults)

const { t } = useI18n()
const { confirm } = useDialogService()
const appStore = useAppStore()
const defaultBlockMessage = () => t('admin.riskControl.defaultBlockMessage')

const loading = ref(true)
const saving = ref(false)
const logsLoading = ref(false)
const statusLoading = ref(false)
const apiKeyTesting = ref(false)
const hashActionLoading = ref(false)
const unbanningUserID = ref<number | null>(null)
const settingsOpen = ref(false)
const activeSettingsTab = ref<SettingsTab>('basic')
const groupSearch = ref('')
const flaggedHashInput = ref('')
const groups = ref<AdminGroup[]>([])
const proxies = ref<Proxy[]>([])
const logs = ref<ContentModerationLog[]>([])
const status = ref<ContentModerationRuntimeStatus | null>(null)
const testedApiKeyStatuses = ref<ContentModerationAPIKeyStatus[]>([])
const pendingDeleteApiKeyHashes = ref<string[]>([])
const apiKeyRowsExpanded = ref<boolean>(false)
const moderationTestPrompt = ref('')
const moderationTestImages = ref<string[]>([])
const moderationTestResult = ref<ContentModerationTestAuditResult | null>(null)
const inputDetailRow = ref<ContentModerationLog | null>(null)
let statusTimer: number | null = null

const configForm = reactive({
  enabled: false,
  mode: 'pre_block' as ModerationMode,
  base_url: 'https://api.openai.com',
  model: 'omni-moderation-latest',
  proxy_id: null as number | null,
  api_keys_text: '',
  api_key_configured: false,
  api_key_masked: '',
  api_key_count: 0,
  api_key_masks: [] as string[],
  api_key_statuses: [] as ContentModerationAPIKeyStatus[],
  api_keys_mode: 'append' as APIKeysWriteMode,
  clear_api_key: false,
  timeout_ms: 3000,
  retry_count: 2,
  sample_rate: 100,
  all_groups: true,
  group_ids: [] as number[],
  record_non_hits: false,
  worker_count: 4,
  queue_size: 32768,
  block_status: 403,
  block_message: defaultBlockMessage(),
  email_on_hit: true,
  auto_ban_enabled: true,
  cyber_policy_exclude_from_ban_count: false,
  ban_threshold: 10,
  violation_window_hours: 720,
  hit_retention_days: 180,
  non_hit_retention_days: 3,
  pre_hash_check_enabled: false,
  thresholds: { ...riskThresholdDefaults } as Record<string, number>,
  blocked_keywords_text: '',
  keyword_blocking_mode: 'keyword_and_api' as KeywordBlockingMode,
  model_filter_type: 'all' as ContentModerationModelFilterType,
  model_filter_models: [] as string[],
})

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 1,
})

const filters = reactive({
  result: '',
  group_id: 0,
  endpoint: '',
  search: '',
  from: '',
  to: '',
})

const settingsTabs = computed<Array<{ id: SettingsTab; label: string }>>(() => [
  { id: 'basic', label: t('admin.riskControl.tabs.basic') },
  { id: 'scope', label: t('admin.riskControl.tabs.scope') },
  { id: 'runtime', label: t('admin.riskControl.tabs.runtime') },
  { id: 'response', label: t('admin.riskControl.tabs.response') },
  { id: 'riskThresholds', label: t('admin.riskControl.tabs.riskThresholds') },
  { id: 'keywords', label: t('admin.riskControl.tabs.keywords') },
  { id: 'retention', label: t('admin.riskControl.tabs.retention') },
])
const settingsTabItems = computed(() => settingsTabs.value.map((tab) => ({
  value: tab.id,
  label: tab.label,
})))

const modeOptions = computed<SelectOption[]>(() => [
  { value: 'pre_block', label: t('admin.riskControl.modePreBlock') },
  { value: 'observe', label: t('admin.riskControl.modeObserve') },
  { value: 'off', label: t('admin.riskControl.modeOff') },
])

const keywordBlockingModeOptions = computed<Array<{ value: KeywordBlockingMode; label: string; description: string }>>(() => [
  {
    value: 'keyword_and_api',
    label: t('admin.riskControl.keywordModeKeywordAndApi'),
    description: t('admin.riskControl.keywordModeKeywordAndApiDesc'),
  },
  {
    value: 'keyword_only',
    label: t('admin.riskControl.keywordModeKeywordOnly'),
    description: t('admin.riskControl.keywordModeKeywordOnlyDesc'),
  },
  {
    value: 'api_only',
    label: t('admin.riskControl.keywordModeApiOnly'),
    description: t('admin.riskControl.keywordModeApiOnlyDesc'),
  },
])

const modelFilterOptions = computed<Array<{ value: ContentModerationModelFilterType; label: string; description: string }>>(() => [
  {
    value: 'all',
    label: t('admin.riskControl.modelFilterAll'),
    description: t('admin.riskControl.modelFilterAllDesc'),
  },
  {
    value: 'include',
    label: t('admin.riskControl.modelFilterInclude'),
    description: t('admin.riskControl.modelFilterIncludeDesc'),
  },
  {
    value: 'exclude',
    label: t('admin.riskControl.modelFilterExclude'),
    description: t('admin.riskControl.modelFilterExcludeDesc'),
  },
])

type KeywordNoticeView = {
  title: string
  description: string
  icon: 'infoCircle' | 'exclamationTriangle'
  toneClass: string
  iconClass: string
  titleClass: string
}

const keywordNoticeTones = {
  info: {
    icon: 'infoCircle' as const,
    toneClass: 'border-[var(--brand-line)] bg-[var(--brand-tint)] dark:border-[var(--brand-line-strong)] dark:bg-[var(--brand)]',
    iconClass: 'mt-0.5 flex-shrink-0 text-[var(--brand)] dark:text-[var(--brand)]',
    titleClass: 'text-[var(--brand)] dark:text-[var(--brand)]',
  },
  warning: {
    icon: 'exclamationTriangle' as const,
    toneClass: 'border-[var(--brand-line)] bg-[var(--brand-tint)] dark:border-[var(--brand-line)] dark:bg-[var(--brand-tint)]',
    iconClass: 'mt-0.5 flex-shrink-0 text-[var(--brand)] dark:text-[var(--brand)]',
    titleClass: 'text-[var(--brand)] dark:text-[var(--brand)]',
  },
}

const keywordNotice = computed<KeywordNoticeView>(() => {
  const strategy = configForm.keyword_blocking_mode
  if (strategy === 'api_only') {
    return {
      ...keywordNoticeTones.info,
      title: t('admin.riskControl.keywordModeApiOnlyNotice'),
      description: t('admin.riskControl.keywordModeApiOnlyDesc'),
    }
  }
  if (configForm.mode !== 'pre_block') {
    return {
      ...keywordNoticeTones.warning,
      title: t('admin.riskControl.blockedKeywordsModeWarning', { mode: modeLabel(configForm.mode) }),
      description: t('admin.riskControl.blockedKeywordsDescription'),
    }
  }
  if (strategy === 'keyword_only') {
    return {
      ...keywordNoticeTones.info,
      title: t('admin.riskControl.keywordModeKeywordOnlyNotice'),
      description: t('admin.riskControl.keywordModeKeywordOnlyDesc'),
    }
  }
  return {
    ...keywordNoticeTones.info,
    title: t('admin.riskControl.blockedKeywordsPreBlockHint'),
    description: t('admin.riskControl.blockedKeywordsDescription'),
  }
})

const resultOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.riskControl.result.all') },
  { value: 'hit', label: t('admin.riskControl.result.hit') },
  { value: 'blocked', label: t('admin.riskControl.result.blocked') },
  { value: 'pass', label: t('admin.riskControl.result.pass') },
  { value: 'error', label: t('admin.riskControl.result.error') },
])

const endpointOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.riskControl.filters.allEndpoints') },
  { value: '/v1/messages', label: '/v1/messages' },
  { value: '/v1/responses', label: '/v1/responses' },
  { value: '/v1/chat/completions', label: '/v1/chat/completions' },
  { value: '/v1beta/models', label: '/v1beta/models' },
  { value: '/v1/images/generations', label: '/v1/images/generations' },
  { value: '/v1/images/edits', label: '/v1/images/edits' },
])

const groupFilterOptions = computed<SelectOption[]>(() => [
  { value: 0, label: t('admin.riskControl.filters.allGroups') },
  ...groups.value.map((group) => ({
    value: group.id,
    label: `${group.name} (${group.platform})`,
  })),
])

const selectedGroupCount = computed(() => String(configForm.group_ids.length))

const modelFilterModelCount = computed(() => configForm.model_filter_models.length)

const modelFilterSummary = computed(() => {
  if (configForm.model_filter_type === 'include') {
    return t('admin.riskControl.modelFilterIncludeSummary', { count: modelFilterModelCount.value })
  }
  if (configForm.model_filter_type === 'exclude') {
    return t('admin.riskControl.modelFilterExcludeSummary', { count: modelFilterModelCount.value })
  }
  return t('admin.riskControl.modelFilterAllSummary')
})

const modelFilterPreviewModels = computed(() => configForm.model_filter_models.slice(0, 6))

const hiddenModelFilterModelCount = computed(() => Math.max(0, configForm.model_filter_models.length - modelFilterPreviewModels.value.length))

const filteredGroups = computed(() => {
  const keyword = groupSearch.value.trim().toLowerCase()
  if (!keyword) return groups.value
  return groups.value.filter((group) => {
    return group.name.toLowerCase().includes(keyword) || String(group.platform).toLowerCase().includes(keyword)
  })
})

const inputApiKeyCount = computed(() => parseApiKeys(configForm.api_keys_text).length)

const blockedKeywordList = computed(() => parseBlockedKeywords(configForm.blocked_keywords_text))

const blockedKeywordCount = computed(() => blockedKeywordList.value.length)

const pendingDeletedApiKeyCount = computed(() => pendingDeleteApiKeyHashes.value.length)

const effectiveStoredApiKeyCount = computed(() => Math.max(0, configForm.api_key_count - pendingDeletedApiKeyCount.value))

const apiKeysPlaceholder = computed(() => (
  configForm.api_keys_mode === 'replace'
    ? t('admin.riskControl.apiKeysPlaceholderReplace')
    : t('admin.riskControl.apiKeysPlaceholder')
))

const apiKeysModeHint = computed(() => (
  configForm.api_keys_mode === 'replace'
    ? t('admin.riskControl.apiKeysModeReplaceHint')
    : t('admin.riskControl.apiKeysModeAppendHint')
))
const apiKeysModeItems = computed(() => [
  { value: 'append' as const, label: t('admin.riskControl.apiKeysModeAppend'), disabled: configForm.clear_api_key },
  { value: 'replace' as const, label: t('admin.riskControl.apiKeysModeReplace'), disabled: configForm.clear_api_key },
])

const hasModerationAuditInput = computed(() => {
  return moderationTestPrompt.value.trim() !== '' || moderationTestImages.value.length > 0
})

const isFlaggedHashInputValid = computed(() => /^[a-fA-F0-9]{64}$/.test(flaggedHashInput.value.trim()))

const storedApiKeyTestButtonText = computed(() => {
  if (apiKeyTesting.value) return t('admin.riskControl.testingApiKeys')
  if (hasModerationAuditInput.value) return t('admin.riskControl.testContentWithStoredApiKey')
  return t('admin.riskControl.testStoredApiKeys')
})

const savedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
  const rows = status.value?.api_key_statuses?.length
    ? status.value.api_key_statuses
    : configForm.api_key_statuses
  return Array.isArray(rows) ? rows : []
})

const apiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => [
  ...savedApiKeyRows.value,
  ...testedApiKeyStatuses.value,
])

const visibleApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
  if (apiKeyRowsExpanded.value) return apiKeyRows.value
  return apiKeyRows.value.slice(0, maxVisibleApiKeyRows)
})

const hiddenApiKeyRowCount = computed<number>(() => Math.max(0, apiKeyRows.value.length - visibleApiKeyRows.value.length))

const canToggleApiKeyRows = computed<boolean>(() => apiKeyRows.value.length > maxVisibleApiKeyRows)

const activeSavedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => (
  savedApiKeyRows.value.filter((row) => !isStoredApiKeyPendingDelete(row))
))

const apiKeyHealthBadges = computed<Array<{ status: ContentModerationAPIKeyStatus['status']; count: number }>>(() => {
  const counts: Record<ContentModerationAPIKeyStatus['status'], number> = {
    ok: 0,
    error: 0,
    frozen: 0,
    unknown: 0,
  }
  for (const row of activeSavedApiKeyRows.value) {
    counts[row.status] = (counts[row.status] ?? 0) + 1
  }
  if (activeSavedApiKeyRows.value.length === 0 && effectiveStoredApiKeyCount.value > 0) {
    counts.unknown = effectiveStoredApiKeyCount.value
  }
  return (['ok', 'frozen', 'error', 'unknown'] as Array<ContentModerationAPIKeyStatus['status']>)
    .map((item) => ({ status: item, count: counts[item] }))
    .filter((item) => item.count > 0)
})

const apiKeyHealthSummary = computed(() => {
  if (!configForm.api_key_configured) return ''
  if (apiKeyHealthBadges.value.length === 0) return t('admin.riskControl.apiKeyStatusUnknown')
  return apiKeyHealthBadges.value
    .map((badge) => `${apiKeyStatusLabel(badge.status)} ${badge.count}`)
    .join(' · ')
})

const overviewItems = computed<OverviewItem[]>(() => [
  {
    key: 'status',
    label: t('admin.riskControl.overview.status'),
    value: configForm.enabled ? t('admin.riskControl.overview.enabled') : t('admin.riskControl.overview.disabled'),
    context: `${modeLabel(configForm.mode)} · ${runtimeBadgeText.value}`,
    contextTone: configForm.enabled && status.value?.risk_control_enabled ? 'good' : 'attention',
    icon: 'shield-01',
    tone: 'brand',
  },
  {
    key: 'api-key',
    label: t('admin.riskControl.overview.apiKey'),
    value: configForm.api_key_configured ? t('admin.riskControl.apiKeyCount', { count: configForm.api_key_count }) : t('admin.riskControl.notConfigured'),
    context: configForm.api_key_configured ? apiKeyHealthSummary.value || configForm.model || '-' : configForm.model || '-',
    contextTone: 'muted',
    icon: 'key-01',
    tone: 'info',
  },
  {
    key: 'scope',
    label: t('admin.riskControl.overview.groupScope'),
    value: configForm.all_groups ? t('admin.riskControl.allGroups') : selectedGroupCount.value,
    context: modelFilterSummary.value,
    contextTone: 'muted',
    icon: 'user-group',
    tone: 'accent',
  },
  {
    key: 'logs',
    label: t('admin.riskControl.overview.logs'),
    value: formatNumber(pagination.total),
    context: t('admin.riskControl.overview.currentFilter'),
    contextTone: 'muted',
    icon: 'file-02',
    tone: 'brand',
  },
])

const moderationScoreRows = computed<ModerationScoreRow[]>(() => {
  const result = moderationTestResult.value
  if (!result) return []
  return Object.entries(result.category_scores || {})
    .map(([category, score]) => {
      const threshold = result.thresholds?.[category] ?? 1
      return {
        category,
        score,
        threshold,
        hit: score >= threshold,
      }
    })
    .sort((a, b) => b.score - a.score)
})

const riskThresholdRows = computed<RiskThresholdRow[]>(() => (
  riskThresholdCategories.map((category) => ({
    category,
    value: configForm.thresholds[category] ?? riskThresholdDefaults[category],
    defaultValue: riskThresholdDefaults[category],
  }))
))

const inputDetailText = computed(() => {
  if (!inputDetailRow.value) return '-'
  return inputDetailRow.value.input_excerpt || inputDetailRow.value.error || '-'
})

const queueUsagePercent = computed(() => `${Math.min(100, Math.max(0, status.value?.queue_usage_percent ?? 0)).toFixed(1)}%`)

const queueUsageStyle = computed(() => ({
  width: queueUsagePercent.value,
}))

const runtimeMode = computed<ModerationMode>(() => status.value?.mode ?? configForm.mode)

const showPreBlockRuntimeCard = computed(() => runtimeMode.value === 'pre_block')

const showWorkerRuntimeCard = computed(() => runtimeMode.value === 'observe')

const preBlockMetricItems = computed(() => [
  {
    key: 'active',
    label: t('admin.riskControl.preBlockActive'),
    value: formatNumber(status.value?.pre_block_active ?? 0),
    meta: t('admin.riskControl.preBlockActiveHint'),
    class: 'bg-[var(--brand-tint)] dark:bg-[var(--brand-tint)]',
    valueClass: 'text-[var(--brand)] dark:text-[var(--brand)]',
  },
  {
    key: 'checked',
    label: t('admin.riskControl.preBlockChecked'),
    value: formatNumber(status.value?.pre_block_checked ?? 0),
    meta: t('admin.riskControl.preBlockCheckedHint'),
    class: 'bg-[var(--brand-tint)] dark:bg-[var(--card)]',
    valueClass: 'text-[var(--foreground)] dark:text-[var(--foreground)]',
  },
  {
    key: 'allowed',
    label: t('admin.riskControl.preBlockAllowed'),
    value: formatNumber(status.value?.pre_block_allowed ?? 0),
    meta: t('admin.riskControl.preBlockAllowedHint'),
    class: 'bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] dark:bg-[color-mix(in_oklch,var(--success)_14%,var(--card))]',
    valueClass: 'text-[var(--success)] dark:text-[var(--success)]',
  },
  {
    key: 'blocked',
    label: t('admin.riskControl.preBlockBlocked'),
    value: formatNumber(status.value?.pre_block_blocked ?? 0),
    meta: t('admin.riskControl.preBlockBlockedHint'),
    class: 'bg-[var(--destructive-soft)] dark:bg-[var(--destructive-soft)]',
    valueClass: 'text-[var(--destructive)] dark:text-[var(--destructive)]',
  },
  {
    key: 'errors',
    label: t('admin.riskControl.preBlockErrors'),
    value: formatNumber(status.value?.pre_block_errors ?? 0),
    meta: t('admin.riskControl.preBlockErrorsHint'),
    class: 'bg-[var(--brand-tint)] dark:bg-[var(--brand-tint)]',
    valueClass: 'text-[var(--brand)] dark:text-[var(--brand)]',
  },
  {
    key: 'latency',
    label: t('admin.riskControl.preBlockAvgLatency'),
    value: `${formatNumber(status.value?.pre_block_avg_latency_ms ?? 0)} ms`,
    meta: t('admin.riskControl.preBlockAvgLatencyHint'),
    class: 'bg-[var(--brand-tint)] dark:bg-[var(--brand-tint)]',
    valueClass: 'text-[var(--brand)] dark:text-[var(--brand)]',
  },
])

const preBlockAPIKeyLoads = computed<ContentModerationAPIKeyLoad[]>(() => (
  [...(status.value?.pre_block_api_key_loads ?? [])].sort((a, b) => a.index - b.index)
))

const preBlockAPIKeyMaxTotal = computed(() => Math.max(1, ...preBlockAPIKeyLoads.value.map((item) => item.total || 0)))

const preBlockAPIKeyLoadSummaryText = computed(() => t('admin.riskControl.preBlockAPIKeyLoadSummary', {
  active: formatNumber(status.value?.pre_block_api_key_active ?? 0),
  available: formatNumber(status.value?.pre_block_api_key_available_count ?? 0),
  total: formatNumber(status.value?.pre_block_api_key_total_calls ?? 0),
  workerActive: formatNumber(status.value?.active_workers ?? 0),
  workerTotal: formatNumber(status.value?.worker_count ?? configForm.worker_count),
}))

function preBlockAPIKeyLoadWidth(total: number): string {
  return `${Math.min(100, Math.max(0, (total / preBlockAPIKeyMaxTotal.value) * 100)).toFixed(1)}%`
}

const workerSlots = computed(() => {
  const total = Math.max(0, status.value?.worker_count ?? configForm.worker_count)
  const active = Math.max(0, status.value?.active_workers ?? 0)
  const enabled = Boolean(status.value?.risk_control_enabled && status.value?.enabled && status.value?.mode !== 'off')
  return Array.from({ length: total }, (_, index) => ({
    id: index + 1,
    state: (!enabled ? 'disabled' : index < active ? 'active' : 'idle') as WorkerSlotState,
    label: !enabled
      ? t('admin.riskControl.workerDisabled')
      : index < active
        ? t('admin.riskControl.workerActive')
        : t('admin.riskControl.workerIdle'),
  }))
})

const runtimeBadgeText = computed(() => {
  if (!status.value?.risk_control_enabled) return t('admin.riskControl.riskSwitchOff')
  if (!configForm.enabled || configForm.mode === 'off') return t('admin.riskControl.overview.disabled')
  return t('admin.riskControl.overview.enabled')
})

function applyConfig(config: ContentModerationConfig) {
  configForm.enabled = config.enabled
  configForm.mode = config.mode
  configForm.base_url = config.base_url || 'https://api.openai.com'
  configForm.model = config.model || 'omni-moderation-latest'
  configForm.proxy_id = config.proxy_id || null
  configForm.api_keys_text = ''
  configForm.api_key_configured = config.api_key_configured
  configForm.api_key_masked = config.api_key_masked || ''
  configForm.api_key_count = config.api_key_count || 0
  configForm.api_key_masks = Array.isArray(config.api_key_masks) ? [...config.api_key_masks] : []
  configForm.api_key_statuses = Array.isArray(config.api_key_statuses) ? [...config.api_key_statuses] : []
  configForm.api_keys_mode = 'append'
  configForm.clear_api_key = false
  pendingDeleteApiKeyHashes.value = []
  testedApiKeyStatuses.value = []
  apiKeyRowsExpanded.value = false
  configForm.timeout_ms = config.timeout_ms || 3000
  configForm.retry_count = config.retry_count ?? 2
  configForm.sample_rate = config.sample_rate ?? 100
  configForm.all_groups = config.all_groups
  configForm.group_ids = Array.isArray(config.group_ids) ? [...config.group_ids] : []
  configForm.record_non_hits = config.record_non_hits
  configForm.worker_count = config.worker_count || 4
  configForm.queue_size = config.queue_size || 32768
  configForm.block_status = config.block_status || 403
  configForm.block_message = config.block_message || defaultBlockMessage()
  configForm.email_on_hit = config.email_on_hit ?? true
  configForm.auto_ban_enabled = config.auto_ban_enabled ?? true
  configForm.cyber_policy_exclude_from_ban_count = config.cyber_policy_exclude_from_ban_count ?? false
  configForm.ban_threshold = config.ban_threshold || 10
  configForm.violation_window_hours = config.violation_window_hours || 720
  configForm.hit_retention_days = config.hit_retention_days || 180
  configForm.non_hit_retention_days = Math.min(Math.max(config.non_hit_retention_days || 3, 1), 3)
  configForm.pre_hash_check_enabled = config.pre_hash_check_enabled ?? false
  configForm.thresholds = riskThresholdsFromConfig(config.thresholds)
  configForm.blocked_keywords_text = Array.isArray(config.blocked_keywords) ? config.blocked_keywords.join('\n') : ''
  configForm.keyword_blocking_mode = normalizeKeywordBlockingMode(config.keyword_blocking_mode)
  const modelFilter = normalizeModelFilter(config.model_filter)
  configForm.model_filter_type = modelFilter.type
  configForm.model_filter_models = modelFilter.models
}

async function loadAll() {
  loading.value = true
  try {
    const [config, groupItems, runtimeStatus, proxyItems] = await Promise.all([
      adminAPI.riskControl.getConfig(),
      adminAPI.groups.getAll(),
      adminAPI.riskControl.getStatus(),
      // 代理列表加载失败不阻塞风控页面（仅影响下拉可选项）
      adminAPI.proxies.getAll().catch(() => [] as Proxy[]),
    ])
    applyConfig(config)
    groups.value = groupItems
    status.value = runtimeStatus
    proxies.value = proxyItems
    if (Array.isArray(runtimeStatus.api_key_statuses)) {
      configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
      prunePendingDeleteAPIKeyHashes()
    }
    await loadLogs()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function loadStatus(silent = true) {
  statusLoading.value = true
  try {
    const runtimeStatus = await adminAPI.riskControl.getStatus()
    status.value = runtimeStatus
    if (Array.isArray(runtimeStatus.api_key_statuses)) {
      configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
      prunePendingDeleteAPIKeyHashes()
    }
  } catch (err: unknown) {
    if (!silent) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.statusFailed')))
    }
  } finally {
    statusLoading.value = false
  }
}

async function saveConfig() {
  saving.value = true
  try {
    const modelFilterPayload = buildModelFilterPayload()
    if (modelFilterPayload.type !== 'all' && modelFilterPayload.models.length === 0) {
      appStore.showError(t('admin.riskControl.modelFilterModelsRequired'))
      return
    }
    const payload: UpdateContentModerationConfig = {
      enabled: configForm.enabled,
      mode: configForm.mode,
      base_url: configForm.base_url,
      model: configForm.model,
      // 后端语义：0 清除代理（直连），>0 指定代理
      proxy_id: configForm.proxy_id ?? 0,
      timeout_ms: Number(configForm.timeout_ms) || 3000,
      retry_count: Number(configForm.retry_count) || 0,
      sample_rate: Number(configForm.sample_rate) || 0,
      all_groups: configForm.all_groups,
      group_ids: configForm.all_groups ? [] : [...configForm.group_ids],
      record_non_hits: configForm.record_non_hits,
      clear_api_key: configForm.clear_api_key,
      worker_count: Number(configForm.worker_count) || 4,
      queue_size: Number(configForm.queue_size) || 32768,
      block_status: Number(configForm.block_status) || 403,
      block_message: configForm.block_message || defaultBlockMessage(),
      email_on_hit: configForm.email_on_hit,
      auto_ban_enabled: configForm.auto_ban_enabled,
      cyber_policy_exclude_from_ban_count: configForm.cyber_policy_exclude_from_ban_count,
      ban_threshold: Number(configForm.ban_threshold) || 10,
      violation_window_hours: Number(configForm.violation_window_hours) || 720,
      hit_retention_days: Number(configForm.hit_retention_days) || 180,
      non_hit_retention_days: Math.min(Math.max(Number(configForm.non_hit_retention_days) || 3, 1), 3),
      pre_hash_check_enabled: configForm.pre_hash_check_enabled,
      thresholds: buildRiskThresholdPayload(),
      blocked_keywords: blockedKeywordList.value,
      keyword_blocking_mode: configForm.keyword_blocking_mode,
      model_filter: modelFilterPayload,
    }
    const keys = parseApiKeys(configForm.api_keys_text)
    if (!payload.clear_api_key && configForm.api_keys_mode === 'replace' && keys.length === 0) {
      appStore.showError(t('admin.riskControl.apiKeysReplaceNoInput'))
      return
    }
    if (keys.length > 0) {
      payload.api_keys = keys
      payload.api_keys_mode = configForm.api_keys_mode
      payload.clear_api_key = false
    }
    if (!payload.clear_api_key && configForm.api_keys_mode !== 'replace' && pendingDeleteApiKeyHashes.value.length > 0) {
      payload.delete_api_key_hashes = [...pendingDeleteApiKeyHashes.value]
    }

    const updated = await adminAPI.riskControl.updateConfig(payload)
    applyConfig(updated)
    settingsOpen.value = false
    appStore.showSuccess(t('admin.riskControl.saved'))
    await Promise.all([loadStatus(true), loadLogs()])
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function loadLogs() {
  logsLoading.value = true
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.page_size,
      result: filters.result || undefined,
      group_id: filters.group_id || undefined,
      endpoint: filters.endpoint || undefined,
      search: filters.search || undefined,
      from: normalizeDateTimeLocal(filters.from),
      to: normalizeDateTimeLocal(filters.to),
    }
    const result = await adminAPI.riskControl.listLogs(params)
    logs.value = result.items
    pagination.total = result.total
    pagination.page = result.page
    pagination.page_size = result.page_size
    pagination.pages = result.pages
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.logsFailed')))
  } finally {
    logsLoading.value = false
  }
}

function canUnbanRow(row: ContentModerationLog): boolean {
  return Boolean(row.auto_banned && row.user_id && row.user_status === 'disabled')
}

function inputSummaryText(row: ContentModerationLog): string {
  return row.input_excerpt || row.error || '-'
}

function openInputDetail(row: ContentModerationLog) {
  inputDetailRow.value = row
}

function closeInputDetail() {
  inputDetailRow.value = null
}

async function unbanUser(row: ContentModerationLog) {
  if (!row.user_id || unbanningUserID.value !== null) return
  unbanningUserID.value = row.user_id
  try {
    const result = await adminAPI.riskControl.unbanUser(row.user_id)
    logs.value = logs.value.map((item) => {
      if (item.user_id !== row.user_id) return item
      return { ...item, user_status: result.status }
    })
    appStore.showSuccess(t('admin.riskControl.unbanSuccess'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.unbanFailed')))
  } finally {
    unbanningUserID.value = null
  }
}

async function deleteFlaggedHash() {
  if (!isFlaggedHashInputValid.value || hashActionLoading.value) return
  hashActionLoading.value = true
  try {
    const result = await adminAPI.riskControl.deleteFlaggedHash(flaggedHashInput.value)
    flaggedHashInput.value = ''
    await loadStatus(true)
    appStore.showSuccess(result.deleted ? t('admin.riskControl.flaggedHashDeleted') : t('admin.riskControl.flaggedHashNotFound'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashDeleteFailed')))
  } finally {
    hashActionLoading.value = false
  }
}

async function clearFlaggedHashes() {
  if (hashActionLoading.value) return
  const confirmed = await confirm({
    message: t('admin.riskControl.clearFlaggedHashesConfirm'),
    danger: true,
  })
  if (!confirmed) return
  hashActionLoading.value = true
  try {
    const result = await adminAPI.riskControl.clearFlaggedHashes()
    await loadStatus(true)
    appStore.showSuccess(t('admin.riskControl.flaggedHashesCleared', { count: result.deleted }))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashesClearFailed')))
  } finally {
    hashActionLoading.value = false
  }
}

function openSettings() {
  activeSettingsTab.value = 'basic'
  settingsOpen.value = true
}

function reloadLogsFromFirstPage() {
  pagination.page = 1
  void loadLogs()
}

function onPageChange(page: number) {
  pagination.page = page
  void loadLogs()
}

function onPageSizeChange(pageSize: number) {
  pagination.page = 1
  pagination.page_size = pageSize
  void loadLogs()
}

function toggleClearApiKey() {
  configForm.clear_api_key = !configForm.clear_api_key
  if (configForm.clear_api_key) {
    configForm.api_keys_text = ''
    configForm.api_keys_mode = 'append'
    testedApiKeyStatuses.value = []
    pendingDeleteApiKeyHashes.value = []
  }
}

function setAPIKeysMode(mode: APIKeysWriteMode) {
  configForm.api_keys_mode = mode
  if (mode === 'replace') {
    pendingDeleteApiKeyHashes.value = []
  }
}

function setModelFilterType(type: ContentModerationModelFilterType) {
  configForm.model_filter_type = type
  if (type === 'all') {
    configForm.model_filter_models = []
  }
}

async function testApiKeys(useInputKeys: boolean) {
  const keys = useInputKeys ? parseApiKeys(configForm.api_keys_text) : []
  if (useInputKeys && keys.length === 0) {
    appStore.showError(t('admin.riskControl.apiKeyTestNoInput'))
    return
  }
  apiKeyTesting.value = true
  try {
    const result = await adminAPI.riskControl.testAPIKeys({
      api_keys: keys,
      base_url: configForm.base_url,
      model: configForm.model,
      timeout_ms: Number(configForm.timeout_ms) || 3000,
      // 与保存语义一致：0 强制直连，>0 指定代理，确保测试与实际审计走同一条链路
      proxy_id: configForm.proxy_id ?? 0,
      prompt: moderationTestPrompt.value,
      images: moderationTestImages.value,
    })
    moderationTestResult.value = result.audit_result ?? null
    if (useInputKeys) {
      testedApiKeyStatuses.value = result.items.map((item) => ({ ...item, configured: false }))
    } else {
      mergeConfiguredAPIKeyStatuses(result.items)
      testedApiKeyStatuses.value = []
      await loadStatus(true)
    }
    appStore.showSuccess(t('admin.riskControl.apiKeyTestDone', { count: result.items.length }))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.apiKeyTestFailed')))
  } finally {
    apiKeyTesting.value = false
  }
}

function mergeConfiguredAPIKeyStatuses(items: ContentModerationAPIKeyStatus[]) {
  if (!hasModerationAuditInput.value || configForm.api_key_statuses.length === 0) {
    configForm.api_key_statuses = items
    return
  }
  const updates = new Map(items.map((item) => [item.key_hash, item]))
  configForm.api_key_statuses = configForm.api_key_statuses.map((item) => updates.get(item.key_hash) ?? item)
}

function toggleDeleteStoredApiKey(row: ContentModerationAPIKeyStatus) {
  if (!row.configured || !row.key_hash) return
  const index = pendingDeleteApiKeyHashes.value.indexOf(row.key_hash)
  if (index >= 0) {
    pendingDeleteApiKeyHashes.value.splice(index, 1)
    return
  }
  pendingDeleteApiKeyHashes.value.push(row.key_hash)
}

function isStoredApiKeyPendingDelete(row: ContentModerationAPIKeyStatus): boolean {
  return row.configured && row.key_hash !== '' && pendingDeleteApiKeyHashes.value.includes(row.key_hash)
}

function prunePendingDeleteAPIKeyHashes() {
  const currentHashes = new Set(savedApiKeyRows.value.map((row) => row.key_hash).filter(Boolean))
  pendingDeleteApiKeyHashes.value = pendingDeleteApiKeyHashes.value.filter((hash) => currentHashes.has(hash))
}

function clearModerationTestInput() {
  moderationTestPrompt.value = ''
  moderationTestImages.value = []
  moderationTestResult.value = null
}

function removeModerationTestImage(index: number) {
  moderationTestImages.value.splice(index, 1)
}

async function handleModerationImageUpload(event: Event) {
  const input = event.target as HTMLInputElement
  await addModerationTestFiles(input.files)
  input.value = ''
}

async function handleModerationImageDrop(event: DragEvent) {
  await addModerationTestFiles(event.dataTransfer?.files ?? null)
}

async function handleModerationImagePaste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.files ?? []).filter((file) => file.type.startsWith('image/'))
  if (files.length === 0) return
  event.preventDefault()
  await addModerationTestFiles(files)
}

async function addModerationTestFiles(files: FileList | File[] | null) {
  if (!files) return
  const items = Array.from(files).filter((file) => file.type.startsWith('image/'))
  for (const file of items) {
    if (moderationTestImages.value.length >= maxModerationTestImages) {
      appStore.showError(t('admin.riskControl.auditTestImageLimit', { count: maxModerationTestImages }))
      return
    }
    if (file.size > maxModerationTestImageSize) {
      appStore.showError(t('admin.riskControl.auditTestImageTooLarge'))
      continue
    }
    try {
      moderationTestImages.value.push(await fileToDataURL(file))
    } catch {
      appStore.showError(t('admin.riskControl.auditTestImageReadFailed'))
    }
  }
}

function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

function toggleGroup(groupID: number) {
  const index = configForm.group_ids.indexOf(groupID)
  if (index >= 0) {
    configForm.group_ids.splice(index, 1)
  } else {
    configForm.group_ids.push(groupID)
  }
}

function isGroupSelected(groupID: number): boolean {
  return configForm.group_ids.includes(groupID)
}

function modeLabel(mode: ModerationMode): string {
  const found = modeOptions.value.find((option) => option.value === mode)
  return found?.label ?? mode
}

function modeDescription(mode: ModerationMode): string {
  const descriptions: Record<ModerationMode, string> = {
    pre_block: t('admin.riskControl.modePreBlockDesc'),
    observe: t('admin.riskControl.modeObserveDesc'),
    off: t('admin.riskControl.modeOffDesc'),
  }
  return descriptions[mode] ?? ''
}

function resultLabel(row: ContentModerationLog): string {
  if (row.action === 'cyber_policy') return t('admin.riskControl.action.cyberPolicy')
  if (row.action === 'keyword_block') return t('admin.riskControl.action.keywordBlock')
  if (row.action === 'block') return t('admin.riskControl.action.block')
  if (row.action === 'error' || row.error) return t('admin.riskControl.action.error')
  if (row.flagged) return t('admin.riskControl.result.hit')
  return t('admin.riskControl.result.pass')
}

function resultBadgeClass(row: ContentModerationLog): string {
  if (row.action === 'block' || row.action === 'keyword_block' || row.action === 'cyber_policy') return 'bg-[var(--destructive-soft)] text-[var(--destructive)] dark:bg-[var(--destructive-soft)] dark:text-[var(--destructive)]'
  if (row.action === 'error' || row.error) return 'bg-[var(--brand-tint)] text-[var(--brand)] dark:bg-[var(--brand-tint)] dark:text-[var(--brand)]'
  if (row.flagged) return 'bg-[var(--brand-tint)] text-[var(--brand)] dark:bg-[var(--brand-tint)] dark:text-[var(--brand)]'
  return 'bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] text-[var(--success)] dark:bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] dark:text-[var(--success)]'
}

function workerSlotClass(state: WorkerSlotState): string {
  if (state === 'active') {
    return 'border-[var(--brand-line)] bg-[var(--brand-tint)] text-[var(--brand)] dark:border-[var(--brand-line)] dark:bg-[var(--brand-tint)] dark:text-[var(--brand)]'
  }
  if (state === 'idle') {
    return 'border-[var(--success)] bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] text-[var(--success)] dark:border-[var(--success)] dark:bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] dark:text-[var(--success)]'
  }
  return 'border-[var(--brand-line)] bg-white text-[var(--muted-foreground)] dark:border-[var(--border-subtle)] dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]'
}

function workerDotClass(state: WorkerSlotState): string {
  if (state === 'active') return 'bg-[var(--brand-tint)]'
  if (state === 'idle') return 'bg-[color-mix(in_oklch,var(--success)_14%,var(--card))]'
  return 'bg-[var(--brand-tint)] dark:bg-[var(--surface-subtle)]'
}

function percent(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${(value * 100).toFixed(1)}%`
}

function percentWidth(value: number): string {
  if (!Number.isFinite(value)) return '0%'
  return `${Math.min(100, Math.max(0, value * 100)).toFixed(1)}%`
}

function latencyText(value: number | null): string {
  if (value === null || value === undefined) return '-'
  return `${value} ms`
}

function apiKeyRowKey(row: ContentModerationAPIKeyStatus, index: number): string {
  return `${row.configured ? 'saved' : 'test'}-${row.key_hash || index}`
}

function apiKeyStatusLabel(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const labels: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: t('admin.riskControl.apiKeyStatusOk'),
    error: t('admin.riskControl.apiKeyStatusError'),
    frozen: t('admin.riskControl.apiKeyStatusFrozen'),
    unknown: t('admin.riskControl.apiKeyStatusUnknown'),
  }
  return labels[statusValue] ?? labels.unknown
}

function apiKeyStatusBadgeClass(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const classes: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: 'bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] text-[var(--success)] dark:bg-[color-mix(in_oklch,var(--success)_14%,var(--card))] dark:text-[var(--success)]',
    error: 'bg-[var(--brand-tint)] text-[var(--brand)] dark:bg-[var(--brand-tint)] dark:text-[var(--brand)]',
    frozen: 'bg-[var(--destructive-soft)] text-[var(--destructive)] dark:bg-[var(--destructive-soft)] dark:text-[var(--destructive)]',
    unknown: 'bg-[var(--brand-tint)] text-[var(--muted-foreground)] dark:bg-[var(--card)] dark:text-[var(--muted-foreground)]',
  }
  return classes[statusValue] ?? classes.unknown
}

function apiKeyStatusDotClass(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const classes: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: 'bg-[color-mix(in_oklch,var(--success)_14%,var(--card))]',
    error: 'bg-[var(--brand-tint)]',
    frozen: 'bg-[var(--destructive-soft)]',
    unknown: 'bg-[var(--brand-tint)]',
  }
  return classes[statusValue] ?? classes.unknown
}

function apiKeyStatusMeta(row: ContentModerationAPIKeyStatus): string {
  const parts: string[] = []
  parts.push(t('admin.riskControl.apiKeyFailureCount', { count: row.failure_count || 0 }))
  if (row.last_latency_ms > 0) {
    parts.push(t('admin.riskControl.apiKeyLatency', { ms: row.last_latency_ms }))
  }
  if (row.last_http_status > 0) {
    parts.push(t('admin.riskControl.apiKeyHTTPStatus', { status: row.last_http_status }))
  }
  if (row.frozen_until) {
    parts.push(t('admin.riskControl.apiKeyFrozenUntil', { time: formatDateTime(row.frozen_until) }))
  } else if (row.last_checked_at) {
    parts.push(t('admin.riskControl.apiKeyLastChecked', { time: formatDateTime(row.last_checked_at) }))
  } else {
    parts.push(t('admin.riskControl.apiKeyNotTested'))
  }
  return parts.join(' / ')
}

function parseApiKeys(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter((item, index, arr) => item && arr.indexOf(item) === index)
}

function normalizeKeywordBlockingMode(value: unknown): KeywordBlockingMode {
  if (value === 'keyword_only' || value === 'api_only' || value === 'keyword_and_api') {
    return value
  }
  return 'keyword_and_api'
}

function normalizeModelFilter(value: unknown): ContentModerationModelFilter {
  if (!value || typeof value !== 'object') {
    return { type: 'all', models: [] }
  }
  const raw = value as Partial<ContentModerationModelFilter>
  const type = normalizeModelFilterType(raw.type)
  const models = type === 'all' ? [] : normalizeModelNames(raw.models)
  return { type, models }
}

function normalizeModelFilterType(value: unknown): ContentModerationModelFilterType {
  if (value === 'include' || value === 'exclude' || value === 'all') {
    return value
  }
  return 'all'
}

function normalizeModelNames(models: unknown): string[] {
  if (!Array.isArray(models)) return []
  const seen = new Set<string>()
  const out: string[] = []
  for (const item of models) {
    const model = String(item ?? '').trim()
    if (!model) continue
    const key = model.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push(model)
  }
  return out
}

function buildModelFilterPayload(): ContentModerationModelFilter {
  const type = normalizeModelFilterType(configForm.model_filter_type)
  if (type === 'all') {
    return { type: 'all', models: [] }
  }
  return {
    type,
    models: normalizeModelNames(configForm.model_filter_models),
  }
}

function riskThresholdsFromConfig(thresholds: Record<string, number> | null | undefined): Record<string, number> {
  const out: Record<string, number> = { ...riskThresholdDefaults }
  for (const category of riskThresholdCategories) {
    const value = thresholds?.[category]
    if (Number.isFinite(value)) {
      out[category] = clampPercent(Number(value) * 100)
    }
  }
  return out
}

function buildRiskThresholdPayload(): Record<string, number> {
  const payload: Record<string, number> = {}
  for (const category of riskThresholdCategories) {
    payload[category] = Number((clampPercent(configForm.thresholds[category]) / 100).toFixed(4))
  }
  return payload
}

function resetRiskThresholds() {
  configForm.thresholds = { ...riskThresholdDefaults }
}

function clampPercent(value: unknown): number {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) {
    return 0
  }
  return Math.min(100, Math.max(0, numeric))
}

function formatThresholdPercent(value: number): string {
  return `${clampPercent(value).toFixed(1)}%`
}

function parseBlockedKeywords(value: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const line of value.split(/\r?\n/)) {
    const kw = line.trim()
    if (!kw) continue
    const key = kw.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push(kw)
  }
  return out
}

function violationCountText(row: ContentModerationLog): string {
  if (!row.flagged) return '-'
  if (row.violation_count === 0) return t('admin.riskControl.violationNotCounted')
  return t('admin.riskControl.violationCount', { count: row.violation_count || 1 })
}

function normalizeDateTimeLocal(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

function formatDateTime(value: string): string {
  return formatDateTimeValue(value) || '-'
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value)
}

onMounted(() => {
  void loadAll()
  statusTimer = window.setInterval(() => {
    void loadStatus(true)
  }, 15000)
})

onUnmounted(() => {
  if (statusTimer !== null) {
    window.clearInterval(statusTimer)
    statusTimer = null
  }
})
</script>

<style scoped>
.risk-panel {
  min-width: 0;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-xl);
  background: var(--card);
}
</style>
