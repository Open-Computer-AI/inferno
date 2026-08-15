<template>
        <!-- Tab: Users -->
        <div class="space-y-6">
          <!-- Default Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.defaults.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.defaults.description") }}
              </p>
            </div>
            <div class="space-y-6 p-6">
              <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div>
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.defaults.defaultBalance") }}
                  </label>
                  <input
                    v-model.number="form.default_balance"
                    type="number"
                    step="0.01"
                    min="0"
                    class="field-control"
                    placeholder="0.00"
                  />
                  <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.defaults.defaultBalanceHint") }}
                  </p>
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.defaults.defaultConcurrency") }}
                  </label>
                  <input
                    v-model.number="form.default_concurrency"
                    type="number"
                    min="1"
                    class="field-control"
                    placeholder="1"
                  />
                  <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.defaults.defaultConcurrencyHint") }}
                  </p>
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.defaults.defaultUserRpmLimit") }}
                  </label>
                  <input
                    v-model.number="form.default_user_rpm_limit"
                    type="number"
                    min="0"
                    step="1"
                    class="field-control"
                    placeholder="0"
                  />
                  <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.defaults.defaultUserRpmLimitHint") }}
                  </p>
                </div>
              </div>

              <div class="border-t border-[var(--border-subtle)] pt-4                ">
<div class="mb-3 flex items-center justify-between">
                  <div>
                    <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                      {{ t("admin.settings.defaults.defaultSubscriptions") }}
                    </label>
                    <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        t("admin.settings.defaults.defaultSubscriptionsHint")
                      }}
                    </p>
                  </div>
                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    @click="addDefaultSubscription"
                    :disabled="subscriptionGroups.length === 0"
                  >
                    {{ t("admin.settings.defaults.addDefaultSubscription") }}
                  </Button>
                </div>

                <div
                  v-if="form.default_subscriptions.length === 0"
                  class="rounded border border-dashed border-[var(--border-subtle)] px-4 py-3 text-sm text-[var(--muted-foreground)]  text-[var(--muted-foreground)]"
                >
                  {{ t("admin.settings.defaults.defaultSubscriptionsEmpty") }}
                </div>

                <div v-else class="space-y-3">
                  <div
                    v-for="(item, index) in form.default_subscriptions"
                    :key="`default-sub-${index}`"
                    class="grid grid-cols-1 gap-3 rounded border border-[var(--border-subtle)] p-3 md:grid-cols-[1fr_160px_auto]                  ">
                    <div>
                      <label
                        class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                      >
                        {{ t("admin.settings.defaults.subscriptionGroup") }}
                      </label>
                      <BaseSelect
                        v-model="item.group_id"
                        class="default-sub-group-select"
                        :options="defaultSubscriptionGroupOptions"
                        :placeholder="
                          t('admin.settings.defaults.subscriptionGroup')
                        "
                      >
                        <template #selected="{ option }">
                          <GroupBadge
                            v-if="option"
                            :name="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).label
                            "
                            :platform="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).platform
                            "
                            :subscription-type="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).subscriptionType
                            "
                            :rate-multiplier="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).rate
                            "
                          />
                          <span v-else class="text-[var(--muted-foreground)]">
                            {{ t("admin.settings.defaults.subscriptionGroup") }}
                          </span>
                        </template>
                        <template #option="{ option, selected }">
                          <GroupOptionItem
                            :name="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).label
                            "
                            :platform="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).platform
                            "
                            :subscription-type="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).subscriptionType
                            "
                            :rate-multiplier="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).rate
                            "
                            :description="
                              (
                                option as unknown as DefaultSubscriptionGroupOption
                              ).description
                            "
                            :selected="selected"
                          />
                        </template>
                      </BaseSelect>
                    </div>
                    <div>
                      <label
                        class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                      >
                        {{
                          t("admin.settings.defaults.subscriptionValidityDays")
                        }}
                      </label>
                      <input
                        v-model.number="item.validity_days"
                        type="number"
                        min="1"
                        max="36500"
                        class="field-control h-[42px]"
                      />
                    </div>
                    <div class="flex items-end">
                      <Button
                        type="button"
                        variant="danger"
                        size="sm"
                        class="w-full"
                        @click="removeDefaultSubscription(index)"
                      >
                        {{ t("common.delete") }}
                      </Button>
                    </div>
                  </div>
                </div>
              </div>

              <!-- ★ 新增：系统全局默认平台限额矩阵 -->
              <div class="border-t border-[var(--border-subtle)] pt-4                ">
<div class="mb-3">
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                    {{ t("admin.settings.defaults.defaultPlatformQuotas") }}
                  </label>
                  <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.defaults.defaultPlatformQuotasHint") }}
                  </p>
                  <p class="mt-0.5 text-xs text-[var(--s2a-attn)] ">
                    {{ t("admin.settings.defaults.platformQuotaNotice") }}
                  </p>
                </div>
                <div class="overflow-x-auto">
                  <table class="min-w-full text-sm">
                    <thead>
                      <tr class="text-left text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        <th class="pb-2 pr-4 font-[var(--fw-medium)]">{{ t("admin.settings.platformQuota.platform") }}</th>
                        <th class="pb-2 pr-4 font-[var(--fw-medium)]">{{ t("admin.settings.platformQuota.daily") }}</th>
                        <th class="pb-2 pr-4 font-[var(--fw-medium)]">{{ t("admin.settings.platformQuota.weekly") }}</th>
                        <th class="pb-2 font-[var(--fw-medium)]">{{ t("admin.settings.platformQuota.monthly") }}</th>
                      </tr>
                    </thead>
                    <tbody class="space-y-2">
                      <tr v-for="p in (['anthropic', 'openai', 'gemini', 'antigravity', 'grok'] as const)" :key="p" class="align-top">
                        <td class="pr-4 py-1">
                          <span class="font-mono text-xs text-[var(--body-copy)] text-[var(--body-copy)]">{{ p }}</span>
                        </td>
                        <td class="pr-4 py-1">
                          <input
                            v-model.number="form.default_platform_quotas[p]!.daily"
                            type="number"
                            step="0.01"
                            min="0"
                            class="field-control h-8 w-28 text-sm"
                            :placeholder="t('admin.settings.platformQuota.placeholder')"
                          />
                        </td>
                        <td class="pr-4 py-1">
                          <input
                            v-model.number="form.default_platform_quotas[p]!.weekly"
                            type="number"
                            step="0.01"
                            min="0"
                            class="field-control h-8 w-28 text-sm"
                            :placeholder="t('admin.settings.platformQuota.placeholder')"
                          />
                        </td>
                        <td class="py-1">
                          <input
                            v-model.number="form.default_platform_quotas[p]!.monthly"
                            type="number"
                            step="0.01"
                            min="0"
                            class="field-control h-8 w-28 text-sm"
                            :placeholder="t('admin.settings.platformQuota.placeholder')"
                          />
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
              <!-- /全局平台限额矩阵 -->
            </div>
          </div>

          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.authSourceDefaults.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.authSourceDefaults.description") }}
              </p>
            </div>
            <div class="space-y-6 p-6">
              <div
                class="flex items-center justify-between rounded border border-[var(--border-subtle)] px-4 py-3              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                    {{ t("admin.settings.authSourceDefaults.requireEmailLabel") }}
                  </label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.authSourceDefaults.requireEmailHint") }}
                  </p>
                </div>
                <Toggle v-model="form.force_email_on_third_party_signup" />
              </div>

              <div class="space-y-4">
                <div
                  v-for="authSource in authSourceDefaultsMeta"
                  :key="authSource.source"
                  class="rounded-[var(--r-lg)] border border-[var(--border-subtle)] p-4                ">
                  <div class="flex items-center justify-between gap-4">
                    <div>
                      <div class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                        {{ authSource.title }}
                      </div>
                      <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ authSource.description }}
                      </p>
                    </div>
                    <Toggle
                      v-model="
                        authSourceDefaults[authSource.source].grant_on_signup
                      "
                      :data-testid="`auth-source-${authSource.source}-enabled`"
                    />
                  </div>

                  <div
                    v-if="authSourceDefaults[authSource.source].grant_on_signup"
                    :data-testid="`auth-source-${authSource.source}-panel`"
                    class="mt-4 space-y-4 border-t border-[var(--border-subtle)] pt-4                  ">
                    <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.authSourceDefaults.enabledHint") }}
                    </p>

                    <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
                      <div>
                        <label
                          class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >
                          {{ t("admin.settings.defaults.defaultBalance") }}
                        </label>
                        <input
                          v-model.number="
                            authSourceDefaults[authSource.source].balance
                          "
                          type="number"
                          step="0.01"
                          min="0"
                          class="field-control"
                          placeholder="0.00"
                        />
                      </div>
                      <div>
                        <label
                          class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >
                          {{ t("admin.settings.defaults.defaultConcurrency") }}
                        </label>
                        <input
                          v-model.number="
                            authSourceDefaults[authSource.source].concurrency
                          "
                          type="number"
                          min="1"
                          class="field-control"
                          placeholder="5"
                        />
                      </div>
                    </div>

                    <div
                      class="flex items-center justify-between rounded border border-[var(--border-subtle)] px-4 py-3                    ">
                      <div>
                        <label
                          class="font-[var(--fw-medium)] text-[var(--foreground)] "
                        >
                          {{ t("admin.settings.authSourceDefaults.grantOnFirstBindLabel") }}
                        </label>
                        <p
                          class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]"
                        >
                          {{ t("admin.settings.authSourceDefaults.grantOnFirstBindHint") }}
                        </p>
                      </div>
                      <Toggle
                        v-model="
                          authSourceDefaults[authSource.source]
                            .grant_on_first_bind
                        "
                      />
                    </div>

                    <div class="mb-3 flex items-center justify-between">
                      <div>
                        <label
                          class="font-[var(--fw-medium)] text-[var(--foreground)] "
                        >
                          {{ t("admin.settings.authSourceDefaults.defaultSubscriptionsLabel") }}
                        </label>
                        <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{ t("admin.settings.authSourceDefaults.defaultSubscriptionsHint") }}
                        </p>
                      </div>
                      <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        @click="
                          addAuthSourceDefaultSubscription(authSource.source)
                        "
                        :disabled="subscriptionGroups.length === 0"
                      >
                        {{
                          t("admin.settings.defaults.addDefaultSubscription")
                        }}
                      </Button>
                    </div>

                    <div
                      v-if="
                        authSourceDefaults[authSource.source].subscriptions
                          .length === 0
                      "
                      class="rounded border border-dashed border-[var(--border-subtle)] px-4 py-3 text-sm text-[var(--muted-foreground)]  text-[var(--muted-foreground)]"
                    >
                      {{ t("admin.settings.authSourceDefaults.noSourceSubscriptions") }}
                    </div>

                    <div v-else class="space-y-3">
                      <div
                        v-for="(item, index) in authSourceDefaults[
                          authSource.source
                        ].subscriptions"
                        :key="`${authSource.source}-sub-${index}`"
                        class="grid grid-cols-1 gap-3 rounded border border-[var(--border-subtle)] p-3 md:grid-cols-[1fr_160px_auto]                      ">
                        <div>
                          <label
                            class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                          >
                            {{ t("admin.settings.defaults.subscriptionGroup") }}
                          </label>
                          <BaseSelect
                            v-model="item.group_id"
                            class="default-sub-group-select"
                            :options="defaultSubscriptionGroupOptions"
                            :placeholder="
                              t('admin.settings.defaults.subscriptionGroup')
                            "
                          >
                            <template #selected="{ option }">
                              <GroupBadge
                                v-if="option"
                                :name="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).label
                                "
                                :platform="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).platform
                                "
                                :subscription-type="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).subscriptionType
                                "
                                :rate-multiplier="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).rate
                                "
                              />
                              <span v-else class="text-[var(--muted-foreground)]">
                                {{
                                  t("admin.settings.defaults.subscriptionGroup")
                                }}
                              </span>
                            </template>
                            <template #option="{ option, selected }">
                              <GroupOptionItem
                                :name="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).label
                                "
                                :platform="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).platform
                                "
                                :subscription-type="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).subscriptionType
                                "
                                :rate-multiplier="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).rate
                                "
                                :description="
                                  (
                                    option as unknown as DefaultSubscriptionGroupOption
                                  ).description
                                "
                                :selected="selected"
                              />
                            </template>
                          </BaseSelect>
                        </div>
                        <div>
                          <label
                            class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                          >
                            {{
                              t(
                                "admin.settings.defaults.subscriptionValidityDays",
                              )
                            }}
                          </label>
                          <input
                            v-model.number="item.validity_days"
                            type="number"
                            min="1"
                            max="36500"
                            class="field-control h-[42px]"
                          />
                        </div>
                        <div class="flex items-end">
                          <Button
                            type="button"
                            variant="danger"
                            size="sm"
                            class="w-full"
                            @click="
                              removeAuthSourceDefaultSubscription(
                                authSource.source,
                                index,
                              )
                            "
                          >
                            {{ t("common.delete") }}
                          </Button>
                        </div>
                      </div>
                    </div>

                    <!-- ★ 新增：auth source 平台限额覆盖区块 -->
                    <div class="border-t border-[var(--border-subtle)] pt-4                      ">
<div class="mb-3">
                        <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                          {{ t("admin.settings.authSourceDefaults.platformQuotasOverride") }}
                        </label>
                        <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{ t("admin.settings.authSourceDefaults.platformQuotasOverrideHint") }}
                        </p>
                      </div>
                      <div class="overflow-x-auto">
                        <table class="min-w-full text-sm">
                          <thead>
                            <tr class="text-left text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                              <th class="pb-2 pr-4 font-[var(--fw-medium)]">{{ t("admin.settings.platformQuota.platform") }}</th>
                              <th class="pb-2 pr-4 font-[var(--fw-medium)]">{{ t("admin.settings.platformQuota.daily") }}</th>
                              <th class="pb-2 pr-4 font-[var(--fw-medium)]">{{ t("admin.settings.platformQuota.weekly") }}</th>
                              <th class="pb-2 font-[var(--fw-medium)]">{{ t("admin.settings.platformQuota.monthly") }}</th>
                            </tr>
                          </thead>
                          <tbody>
                            <tr v-for="p in (['anthropic', 'openai', 'gemini', 'antigravity', 'grok'] as const)" :key="`${authSource.source}-pq-${p}`" class="align-top">
                              <td class="pr-4 py-1">
                                <span class="font-mono text-xs text-[var(--body-copy)] text-[var(--body-copy)]">{{ p }}</span>
                              </td>
                              <td class="pr-4 py-1">
                                <input
                                  v-model.number="authSourceDefaults[authSource.source].platform_quotas[p]!.daily"
                                  type="number"
                                  step="0.01"
                                  min="0"
                                  class="field-control h-8 w-28 text-sm"
                                  :placeholder="t('admin.settings.platformQuota.placeholder')"
                                />
                              </td>
                              <td class="pr-4 py-1">
                                <input
                                  v-model.number="authSourceDefaults[authSource.source].platform_quotas[p]!.weekly"
                                  type="number"
                                  step="0.01"
                                  min="0"
                                  class="field-control h-8 w-28 text-sm"
                                  :placeholder="t('admin.settings.platformQuota.placeholder')"
                                />
                              </td>
                              <td class="py-1">
                                <input
                                  v-model.number="authSourceDefaults[authSource.source].platform_quotas[p]!.monthly"
                                  type="number"
                                  step="0.01"
                                  min="0"
                                  class="field-control h-8 w-28 text-sm"
                                  :placeholder="t('admin.settings.platformQuota.placeholder')"
                                />
                              </td>
                            </tr>
                          </tbody>
                        </table>
                      </div>
                    </div>
                    <!-- /auth source 平台限额覆盖区块 -->
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <!-- /Tab: Users -->
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import BaseSelect from '@/components/common/Select.vue'
import Button from '@/components/common/Button.vue'
import Toggle from '@/components/common/Toggle.vue'
import type { GroupPlatform, SubscriptionType } from '@/types'
import { useSettingsPageBindings } from './settingsPageBindings'

interface DefaultSubscriptionGroupOption {
  label: string
  platform: GroupPlatform
  subscriptionType: SubscriptionType
  rate: number
  description: string | null
}

export default defineComponent({
  name: 'AdminUserSettingsPage',
  components: { BaseSelect, Button, GroupBadge, GroupOptionItem, Toggle },
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
