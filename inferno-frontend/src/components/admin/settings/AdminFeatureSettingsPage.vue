<template>
	        <!-- Tab: Features (功能开关) -->
        <div class="space-y-6">

        <div class="settings-surface">
          <div class="border-b border-[var(--border-subtle)] px-6 py-4            ">
<h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
              {{ t('admin.settings.features.channelMonitor.title') }}
            </h2>
            <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
              {{ t('admin.settings.features.channelMonitor.description') }}
            </p>
            <p class="mt-1.5 text-xs">
              <router-link
                to="/admin/channels/monitor"
                class="inline-flex items-center gap-1 text-[var(--brand)] hover:underline "
              >
                {{ t('admin.settings.features.channelMonitor.configureLink') }}
                <span aria-hidden="true">→</span>
              </router-link>
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                  {{ t('admin.settings.features.channelMonitor.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.channelMonitor.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.channel_monitor_enabled" />
            </div>

            <div v-if="form.channel_monitor_enabled" class="space-y-5">
              <div>
                <label class="field-label">
                  {{ t('admin.settings.features.channelMonitor.mode') }}
                </label>
                <Segmented
                  v-model="form.channel_monitor_mode"
                  class="mt-1.5 w-full max-w-md"
                  :items="[
                    {
                      value: 'v2',
                      label: t('admin.settings.features.channelMonitor.modeV2'),
                    },
                    {
                      value: 'v1',
                      label: t('admin.settings.features.channelMonitor.modeV1'),
                    },
                  ]"
                  aria-label="Channel monitor mode"
                />
                <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{
                    form.channel_monitor_mode === 'v1'
                      ? t('admin.settings.features.channelMonitor.modeV1Hint')
                      : t('admin.settings.features.channelMonitor.modeV2Hint')
                  }}
                </p>
                <p class="mt-1 text-xs text-[var(--muted-foreground)] ">
                  {{ t('admin.settings.features.channelMonitor.modeHint') }}
                </p>
              </div>

              <div v-if="form.channel_monitor_mode === 'v1'">
                <label class="field-label">
                  {{ t('admin.settings.features.channelMonitor.defaultInterval') }}
                  <span class="text-[var(--destructive)]">*</span>
                </label>
                <input
                  v-model.number="form.channel_monitor_default_interval_seconds"
                  type="number"
                  min="15"
                  max="3600"
                  class="field-control"
                />
                <p class="mt-1 text-xs text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.channelMonitor.defaultIntervalHint') }}
                </p>
              </div>

              <div v-if="form.channel_monitor_mode === 'v2'" class="flex items-start justify-between gap-4">
                <div class="min-w-0">
                  <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] ">
                    {{ t('admin.settings.features.channelMonitor.hideThroughput') }}
                  </p>
                  <p class="mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t('admin.settings.features.channelMonitor.hideThroughputHint') }}
                  </p>
                </div>
                <Toggle v-model="form.channel_monitor_hide_throughput" />
              </div>

              <div v-if="form.channel_monitor_mode === 'v1'" class="flex items-start justify-between gap-4">
                <div class="min-w-0">
                  <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)]">
                    {{ t('admin.settings.features.channelMonitor.showQuota') }}
                  </p>
                  <p class="mt-1 text-xs text-[var(--muted-foreground)]">
                    {{ t('admin.settings.features.channelMonitor.showQuotaHint') }}
                  </p>
                </div>
                <Toggle v-model="form.channel_monitor_show_quota" />
              </div>
            </div>
          </div>
        </div>

        <div class="settings-surface">
          <div class="border-b border-[var(--border-subtle)] px-6 py-4">
            <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)]">
              {{ t('admin.settings.features.pluginManagement.title') }}
            </h2>
            <p class="mt-1 text-sm text-[var(--muted-foreground)]">
              {{ t('admin.settings.features.pluginManagement.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between gap-4">
              <div>
                <label class="text-sm font-[var(--fw-medium)] text-[var(--foreground)]">
                  {{ t('admin.settings.features.pluginManagement.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.pluginManagement.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.plugin_management_enabled" />
            </div>
          </div>
        </div>

        <div class="settings-surface">
          <div class="border-b border-[var(--border-subtle)] px-6 py-4            ">
<h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
              {{ t('admin.settings.features.availableChannels.title') }}
            </h2>
            <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
              {{ t('admin.settings.features.availableChannels.description') }}
            </p>
            <p class="mt-1.5 text-xs">
              <router-link
                to="/admin/channels/pricing"
                class="inline-flex items-center gap-1 text-[var(--brand)] hover:underline "
              >
                {{ t('admin.settings.features.availableChannels.configureLink') }}
                <span aria-hidden="true">→</span>
              </router-link>
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                  {{ t('admin.settings.features.availableChannels.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.availableChannels.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.available_channels_enabled" />
            </div>
          </div>
        </div>

        <div class="settings-surface">
          <div class="border-b border-[var(--border-subtle)] px-6 py-4            ">
<h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
              {{ t('admin.settings.features.modelPlaza.title') }}
            </h2>
            <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
              {{ t('admin.settings.features.modelPlaza.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                  {{ t('admin.settings.features.modelPlaza.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.modelPlaza.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.model_plaza_enabled" />
            </div>

            <div v-if="form.model_plaza_enabled" class="flex items-center justify-between">
              <div>
                <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                  {{ t('admin.settings.features.modelPlaza.requireAuth') }}
                </label>
                <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.modelPlaza.requireAuthHint') }}
                </p>
              </div>
              <Toggle v-model="form.model_plaza_require_auth" />
            </div>

            <div v-if="form.model_plaza_enabled">
              <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                {{ t('admin.settings.features.modelPlaza.priceDescription') }}
              </label>
              <p class="mb-2 mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t('admin.settings.features.modelPlaza.priceDescriptionHint') }}
              </p>
              <textarea
                v-model="form.model_plaza_description"
                rows="6"
                class="field-control font-mono text-sm"
              ></textarea>
            </div>
          </div>
        </div>

        <div class="settings-surface">
          <div class="border-b border-[var(--border-subtle)] px-6 py-4            ">
<h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
              {{ t('admin.settings.features.riskControl.title') }}
            </h2>
            <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
              {{ t('admin.settings.features.riskControl.description') }}
            </p>
            <p class="mt-1.5 text-xs">
              <router-link
                to="/admin/risk-control"
                class="inline-flex items-center gap-1 text-[var(--brand)] hover:underline "
              >
                {{ t('admin.settings.features.riskControl.configureLink') }}
                <span aria-hidden="true">→</span>
              </router-link>
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                  {{ t('admin.settings.features.riskControl.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.riskControl.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.risk_control_enabled" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                  {{ t('admin.settings.features.riskControl.cyberSessionBlock') }}
                </label>
                <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.riskControl.cyberSessionBlockHint') }}
                </p>
              </div>
              <Toggle v-model="form.cyber_session_block_enabled" />
            </div>

            <div v-if="form.cyber_session_block_enabled">
              <label class="field-label">
                {{ t('admin.settings.features.riskControl.cyberSessionBlockTTL') }}
                <span class="text-[var(--destructive)]">*</span>
              </label>
              <input
                v-model.number="form.cyber_session_block_ttl_seconds"
                type="number"
                min="1"
                class="field-control"
              />
            </div>
          </div>
        </div>

        <!-- Affiliate (邀请返利) feature card -->
        <div class="settings-surface">
          <div class="border-b border-[var(--border-subtle)] px-6 py-4            ">
<h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
              {{ t('admin.settings.features.affiliate.title') }}
            </h2>
            <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
              {{ t('admin.settings.features.affiliate.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                  {{ t('admin.settings.features.affiliate.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.affiliate.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.affiliate_enabled" />
            </div>

            <div v-if="form.affiliate_enabled" class="space-y-6">
              <div class="flex items-center justify-between">
                <div>
                  <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                    {{ t('admin.settings.features.affiliate.adminRechargeRebate') }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t('admin.settings.features.affiliate.adminRechargeRebateHint') }}
                  </p>
                </div>
                <Toggle v-model="form.affiliate_admin_recharge_enabled" />
              </div>

              <div>
                <label class="field-label">
                  {{ t('admin.settings.features.affiliate.rebateRate') }}
                </label>
                <div class="relative">
                  <input
                    v-model.number="form.affiliate_rebate_rate"
                    type="number"
                    step="0.01"
                    min="0"
                    max="100"
                    class="field-control pr-8"
                    placeholder="20"
                  />
                  <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]">%</span>
                </div>
                <p class="mt-1 text-xs text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.affiliate.rebateRateHint') }}
                </p>
              </div>

              <div>
                <label class="field-label">
                  {{ t('admin.settings.features.affiliate.freezeHours') }}
                </label>
                <input
                  v-model.number="form.affiliate_rebate_freeze_hours"
                  type="number"
                  step="1"
                  min="0"
                  max="720"
                  class="field-control"
                />
                <p class="mt-1 text-xs text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.affiliate.freezeHoursDesc') }}
                </p>
              </div>

              <div>
                <label class="field-label">
                  {{ t('admin.settings.features.affiliate.durationDays') }}
                </label>
                <input
                  v-model.number="form.affiliate_rebate_duration_days"
                  type="number"
                  step="1"
                  min="0"
                  max="3650"
                  class="field-control"
                />
                <p class="mt-1 text-xs text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.affiliate.durationDaysDesc') }}
                </p>
              </div>

              <div>
                <label class="field-label">
                  {{ t('admin.settings.features.affiliate.perInviteeCap') }}
                </label>
                <input
                  v-model.number="form.affiliate_rebate_per_invitee_cap"
                  type="number"
                  step="0.01"
                  min="0"
                  class="field-control"
                />
                <p class="mt-1 text-xs text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.affiliate.perInviteeCapDesc') }}
                </p>
              </div>

              <!-- 专属用户管理 -->
              <div class="border-t border-[var(--border-subtle)] pt-6                ">
<div class="mb-3 flex items-center justify-between">
                  <div>
                    <h3 class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] ">
                      {{ t('admin.settings.features.affiliate.customUsers.title') }}
                    </h3>
                    <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t('admin.settings.features.affiliate.customUsers.description') }}
                    </p>
                  </div>
                  <AppButton
                    variant="solid"
                    size="sm"
                    type="button"
                    @click="openAffiliateModal(null)"
                  >
                    + {{ t('admin.settings.features.affiliate.customUsers.addButton') }}
                  </AppButton>
                </div>

                <div class="mb-3 flex items-center gap-2">
                  <input
                    v-model="affiliateState.search"
                    type="text"
                    class="field-control flex-1"
                    :placeholder="t('admin.settings.features.affiliate.customUsers.searchPlaceholder')"
                    @input="onAffiliateSearchInput"
                  />
                  <AppButton
                    variant="secondary"
                    size="sm"
                    v-if="affiliateState.selected.length > 0"
                    type="button"
                    @click="openAffiliateBatchModal"
                  >
                    {{ t('admin.settings.features.affiliate.customUsers.batchButton', { count: affiliateState.selected.length }) }}
                  </AppButton>
                </div>

                <div class="overflow-x-auto rounded-[var(--r-md)] border border-[var(--border-subtle)]                  ">
<table class="min-w-full divide-y divide-[var(--border-subtle)] ">
                    <thead class="bg-[var(--surface-subtle)]                      ">
<tr>
                        <th class="px-3 py-2 text-left">
                          <Checkbox
                            :model-value="affiliateState.entries.length > 0 && affiliateState.selected.length === affiliateState.entries.length"
                            @update:model-value="toggleAffiliateSelectAll"
                          />
                        </th>
                        <th class="px-3 py-2 text-left text-xs font-[var(--fw-medium)] normal-case text-[var(--muted-foreground)]">{{ t('admin.settings.features.affiliate.customUsers.col.email') }}</th>
                        <th class="px-3 py-2 text-left text-xs font-[var(--fw-medium)] normal-case text-[var(--muted-foreground)]">{{ t('admin.settings.features.affiliate.customUsers.col.username') }}</th>
                        <th class="px-3 py-2 text-left text-xs font-[var(--fw-medium)] normal-case text-[var(--muted-foreground)]">{{ t('admin.settings.features.affiliate.customUsers.col.code') }}</th>
                        <th class="px-3 py-2 text-left text-xs font-[var(--fw-medium)] normal-case text-[var(--muted-foreground)]">{{ t('admin.settings.features.affiliate.customUsers.col.rate') }}</th>
                        <th class="px-3 py-2 text-left text-xs font-[var(--fw-medium)] normal-case text-[var(--muted-foreground)]">{{ t('admin.settings.features.affiliate.customUsers.col.actions') }}</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-[var(--border-subtle)] bg-[var(--card)]                       ">
<tr v-if="affiliateState.loading">
                        <td colspan="6" class="px-3 py-6 text-center text-sm text-[var(--muted-foreground)]">
                          {{ t('common.loading') }}
                        </td>
                      </tr>
                      <tr v-else-if="affiliateState.entries.length === 0">
                        <td colspan="6" class="px-3 py-6 text-center text-sm text-[var(--muted-foreground)]">
                          {{ t('admin.settings.features.affiliate.customUsers.empty') }}
                        </td>
                      </tr>
                      <tr v-for="entry in affiliateState.entries" :key="entry.user_id">
                        <td class="px-3 py-2">
                          <Checkbox
                            :model-value="affiliateState.selected.includes(entry.user_id)"
                            @update:model-value="toggleAffiliateSelect(entry.user_id)"
                          />
                        </td>
                        <td class="px-3 py-2 text-sm text-[var(--foreground)] ">{{ entry.email }}</td>
                        <td class="px-3 py-2 text-sm text-[var(--body-copy)] text-[var(--body-copy)]">{{ entry.username }}</td>
                        <td class="px-3 py-2 text-sm font-mono">
                          {{ entry.aff_code }}
                          <span
                            v-if="entry.aff_code_custom"
                            class="ml-1 inline-block rounded bg-[var(--brand-tint)] px-1.5 py-0.5 text-[10px] font-[var(--fw-medium)] text-[var(--brand)]  "
                          >{{ t('admin.settings.features.affiliate.customUsers.customBadge') }}</span>
                        </td>
                        <td class="px-3 py-2 text-sm">
                          <span v-if="entry.aff_rebate_rate_percent != null">{{ entry.aff_rebate_rate_percent }}%</span>
                          <span v-else class="text-[var(--muted-foreground)]">{{ t('admin.settings.features.affiliate.customUsers.useGlobal') }}</span>
                        </td>
                        <td class="px-3 py-2 text-sm">
                          <div class="flex items-center gap-2">
                            <AppButton variant="ghost" size="xs" type="button" @click="openAffiliateModal(entry)">
                              {{ t('common.edit') }}
                            </AppButton>
                            <AppButton
                              variant="danger"
                              size="xs"
                              type="button"
                              @click="askResetAffiliateUser(entry)"
                            >
                              {{ t('common.delete') }}
                            </AppButton>
                          </div>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>

                <div v-if="affiliateState.total > affiliateState.pageSize" class="mt-3 flex items-center justify-between text-sm">
                  <span class="text-[var(--muted-foreground)]">
                    {{ t('admin.settings.features.affiliate.customUsers.totalLabel', { total: affiliateState.total }) }}
                  </span>
                  <div class="flex items-center gap-2">
                    <AppButton
                      variant="secondary"
                      size="sm"
                      type="button"
                      :disabled="affiliateState.page <= 1"
                      @click="changeAffiliatePage(affiliateState.page - 1)"
                    >
                      {{ t('pagination.previous') }}
                    </AppButton>
                    <span class="text-[var(--muted-foreground)]">{{ affiliateState.page }} / {{ Math.max(1, Math.ceil(affiliateState.total / affiliateState.pageSize)) }}</span>
                    <AppButton
                      variant="secondary"
                      size="sm"
                      type="button"
                      :disabled="affiliateState.page >= Math.ceil(affiliateState.total / affiliateState.pageSize)"
                      @click="changeAffiliatePage(affiliateState.page + 1)"
                    >
                      {{ t('pagination.next') }}
                    </AppButton>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Affiliate add/edit modal -->
        <BaseDialog
          v-if="affiliateModal.open"
          :show="affiliateModal.open"
          :title="affiliateModal.mode === 'add' ? t('admin.settings.features.affiliate.modal.addTitle') : t('admin.settings.features.affiliate.modal.editTitle')"
          width="normal"
          @close="closeAffiliateModal"
        >
            <div class="space-y-4">
              <div v-if="affiliateModal.mode === 'add'">
                <label class="field-label">{{ t('admin.settings.features.affiliate.modal.userLabel') }}</label>
                <!-- Chip showing the picked user; clicking it re-opens the search -->
                <div
                  v-if="affiliateModal.selectedUser"
                  class="flex items-center justify-between rounded-[var(--r-md)] border border-[var(--brand-line)] bg-[var(--brand-tint)] px-3 py-2                 ">
                  <div class="text-sm">
                    <span class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{ affiliateModal.selectedUser.email }}</span>
                    <span class="ml-1 text-xs text-[var(--muted-foreground)]">({{ affiliateModal.selectedUser.username }})</span>
                  </div>
                  <AppButton
                    variant="ghost"
                    size="xs"
                    type="button"
                    :title="t('admin.settings.features.affiliate.modal.changeUser')"
                    aria-label="Change selected user"
                    @click="clearSelectedAffiliateUser"
                  >
                    ×
                  </AppButton>
                </div>
                <!-- Search input + result dropdown — hidden once a selection is made -->
                <template v-else>
                  <input
                    v-model="affiliateModal.userQuery"
                    type="text"
                    class="field-control"
                    :placeholder="t('admin.settings.features.affiliate.modal.userPlaceholder')"
                    @input="onAffiliateUserSearchInput"
                  />
                  <div
                    v-if="affiliateModal.userResults.length > 0"
                    class="mt-1 max-h-40 overflow-y-auto rounded border border-[var(--border-subtle)]                  ">
                    <AppButton
                      v-for="u in affiliateModal.userResults"
                      :key="u.id"
                      variant="ghost"
                      size="sm"
                      type="button"
                      class="w-full justify-start"
                      @click="selectAffiliateUser(u)"
                    >
                      {{ u.email }} <span class="text-xs text-[var(--muted-foreground)]">({{ u.username }})</span>
                    </AppButton>
                  </div>
                </template>
              </div>
              <div v-else>
                <label class="field-label">{{ t('admin.settings.features.affiliate.modal.userLabel') }}</label>
                <input
                  type="text"
                  class="field-control"
                  :value="affiliateModal.editingEntry ? affiliateModal.editingEntry.email : ''"
                  disabled
                />
              </div>

              <div>
                <label class="field-label">{{ t('admin.settings.features.affiliate.modal.codeLabel') }}</label>
                <input
                  v-model="affiliateModal.code"
                  type="text"
                  class="field-control font-mono"
                  :placeholder="t('admin.settings.features.affiliate.modal.codePlaceholder')"
                  maxlength="32"
                />
                <p class="mt-1 text-xs text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.affiliate.modal.codeHint') }}
                </p>
              </div>

              <div>
                <label class="field-label">{{ t('admin.settings.features.affiliate.modal.rateLabel') }}</label>
                <div class="relative">
                  <input
                    v-model="affiliateModal.rate"
                    type="number"
                    step="0.01"
                    min="0"
                    max="100"
                    class="field-control pr-8"
                    :placeholder="t('admin.settings.features.affiliate.modal.ratePlaceholder')"
                  />
                  <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]">%</span>
                </div>
                <p class="mt-1 text-xs text-[var(--muted-foreground)]">
                  {{ t('admin.settings.features.affiliate.modal.rateHint') }}
                </p>
              </div>
            </div>

            <template #footer>
            <div class="flex items-center justify-between gap-3">
              <p
                v-if="!affiliateModalCanSubmit"
                class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]"
              >
                {{ t('admin.settings.features.affiliate.modal.errorEmpty') }}
              </p>
              <span v-else></span>
              <div class="flex gap-2">
                <AppButton variant="secondary" type="button" @click="closeAffiliateModal">
                  {{ t('common.cancel') }}
                </AppButton>
                <AppButton
                  variant="solid"
                  type="button"
                  :disabled="affiliateModal.saving || !affiliateModalCanSubmit"
                  :loading="affiliateModal.saving"
                  @click="submitAffiliateModal"
                >
                  {{ affiliateModal.saving ? t('common.saving') : t('common.save') }}
                </AppButton>
              </div>
            </div>
            </template>
        </BaseDialog>

        <!-- Affiliate batch rate modal -->
        <BaseDialog
          v-if="affiliateBatchModal.open"
          :show="affiliateBatchModal.open"
          :title="t('admin.settings.features.affiliate.batchModal.title', { count: affiliateState.selected.length })"
          width="normal"
          @close="affiliateBatchModal.open = false"
        >
            <p class="mb-4 text-sm text-[var(--muted-foreground)]">
              {{ t('admin.settings.features.affiliate.batchModal.hint') }}
            </p>
            <div class="relative">
              <input
                v-model="affiliateBatchModal.rate"
                type="number"
                step="0.01"
                min="0"
                max="100"
                class="field-control pr-8"
                :placeholder="t('admin.settings.features.affiliate.batchModal.placeholder')"
              />
              <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]">%</span>
            </div>
            <p class="mt-2 text-xs text-[var(--muted-foreground)]">
              {{ t('admin.settings.features.affiliate.batchModal.clearHint') }}
            </p>
            <template #footer>
            <div class="flex justify-end gap-2">
              <AppButton variant="secondary" type="button" @click="affiliateBatchModal.open = false">
                {{ t('common.cancel') }}
              </AppButton>
              <AppButton
                variant="solid"
                type="button"
                :disabled="affiliateBatchModal.saving"
                :loading="affiliateBatchModal.saving"
                @click="submitAffiliateBatchModal"
              >
                {{ affiliateBatchModal.saving ? t('common.saving') : t('common.save') }}
              </AppButton>
            </div>
            </template>
        </BaseDialog>

        </div><!-- /Tab: Features -->
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AppButton from '@/components/common/Button.vue'
import Checkbox from '@/components/common/Checkbox.vue'
import Segmented from '@/components/common/Segmented.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useSettingsPageBindings } from './settingsPageBindings'

export default defineComponent({
  name: 'AdminFeatureSettingsPage',
  components: { AppButton, BaseDialog, Checkbox, Segmented, Toggle },
  setup() {
    return useSettingsPageBindings()
  },
})
</script>

<style scoped>
:deep(.settings-surface) {
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
  box-shadow: none;
  overflow: hidden;
}

:deep(.settings-surface > .border-b) {
  border-bottom-color: var(--border-subtle) !important;
  background: transparent;
}

:deep(.settings-surface [class~="border-[var(--border-subtle)]"]) {
  border-color: var(--border-subtle) !important;
}

:deep(.settings-surface [class~="bg-[var(--surface-subtle)]"]) {
  background: var(--surface-subtle) !important;
}

:deep(.field-control) {
  border-color: var(--input);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--foreground);
}

:deep(.field-control:focus) {
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

@media (max-width: 700px) {
  :deep(.settings-surface) {
    border-radius: var(--r-md);
  }
}
</style>
