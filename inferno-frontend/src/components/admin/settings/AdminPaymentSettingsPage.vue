<template>
        <!-- Tab: Payment -->
        <div class="space-y-6">
          <!-- Payment System Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.payment.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.payment.description") }}
                <a
                  :href="paymentGuideHref"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="ml-2 inline-flex items-center text-[var(--brand)] hover:text-[var(--brand)]  "
                >
                  <svg
                    class="mr-0.5 h-3.5 w-3.5"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                    />
                  </svg>
                  {{ t("admin.settings.payment.configGuide") }}
                </a>
              </p>
            </div>
            <div class="space-y-4 p-6">
              <!-- Enable toggle -->
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.payment.enabled")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.payment.enabledHint") }}
                  </p>
                </div>
                <Toggle v-model="form.payment_enabled" />
              </div>
              <template v-if="form.payment_enabled">
                <!-- Row 1: Product name -->
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.productNamePrefix")
                    }}</label
                    ><input
                      v-model="form.payment_product_name_prefix"
                      type="text"
                      class="field-control"
                      placeholder="Sub2API"
                    />
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.productNameSuffix")
                    }}</label
                    ><input
                      v-model="form.payment_product_name_suffix"
                      type="text"
                      class="field-control"
                      placeholder="CNY"
                    />
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.preview")
                    }}</label>
                    <div
                      class="rounded-[var(--r-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] px-3 py-2 text-sm text-[var(--body-copy)]   text-[var(--body-copy)]"
                    >
                      {{
                        (form.payment_product_name_prefix || "Sub2API") +
                        " 100 " +
                        (form.payment_product_name_suffix || "CNY")
                      }}
                    </div>
                  </div>
                </div>
                <!-- Row 2: Balance toggle + amounts -->
                <div class="grid grid-cols-2 gap-3 sm:grid-cols-5">
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.minAmount")
                    }}</label
                    ><input
                      :value="form.payment_min_amount || ''"
                      @input="
                        form.payment_min_amount =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="field-control"
                      :placeholder="t('admin.settings.payment.noLimit')"
                    />
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.maxAmount")
                    }}</label
                    ><input
                      :value="form.payment_max_amount || ''"
                      @input="
                        form.payment_max_amount =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="field-control"
                      :placeholder="t('admin.settings.payment.noLimit')"
                    />
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.dailyLimit")
                    }}</label
                    ><input
                      :value="form.payment_daily_limit || ''"
                      @input="
                        form.payment_daily_limit =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="field-control"
                      :placeholder="t('admin.settings.payment.noLimit')"
                    />
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.balanceRechargeMultiplier")
                    }}</label>
                    <input
                      :value="form.payment_balance_recharge_multiplier || ''"
                      @input="
                        form.payment_balance_recharge_multiplier =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 1
                      "
                      type="number"
                      step="0.01"
                      min="0.01"
                      class="field-control"
                    />
                    <p class="mt-0.5 text-xs text-[var(--muted-foreground)]">
                      {{
                        t(
                          "admin.settings.payment.balanceRechargeMultiplierHint",
                        )
                      }}
                    </p>
                    <p
                      class="mt-1 text-xs font-[var(--fw-medium)] text-[var(--brand)] "
                    >
                      {{
                        t("admin.settings.payment.balanceRechargePreview", {
                          usd: (
                            Number(form.payment_balance_recharge_multiplier) ||
                            1
                          ).toFixed(2),
                        })
                      }}
                    </p>
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.subscriptionUsdToCnyRate")
                    }}</label>
                    <input
                      :value="form.payment_subscription_usd_to_cny_rate || ''"
                      @input="
                        form.payment_subscription_usd_to_cny_rate =
                          parseFloat(
                            ($event.target as HTMLInputElement).value,
                          ) || 0
                      "
                      type="number"
                      step="0.01"
                      min="0"
                      class="field-control"
                      :placeholder="
                        t(
                          'admin.settings.payment.subscriptionUsdToCnyRateDisabled',
                        )
                      "
                    />
                    <p class="mt-0.5 text-xs text-[var(--muted-foreground)]">
                      {{
                        t("admin.settings.payment.subscriptionUsdToCnyRateHint")
                      }}
                    </p>
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.rechargeFeeRate")
                    }}</label>
                    <div class="relative">
                      <input
                        :value="form.payment_recharge_fee_rate ?? ''"
                        @input="
                          form.payment_recharge_fee_rate = Math.min(
                            100,
                            Math.max(
                              0,
                              Math.round(
                                parseFloat(
                                  ($event.target as HTMLInputElement).value ||
                                    '0',
                                ) * 100,
                              ) / 100,
                            ),
                          )
                        "
                        type="number"
                        step="0.01"
                        min="0"
                        max="100"
                        class="field-control pr-8"
                      />
                      <span
                        class="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-3 text-[var(--muted-foreground)]"
                        >%</span
                      >
                    </div>
                    <p class="mt-0.5 text-xs text-[var(--muted-foreground)]">
                      {{ t("admin.settings.payment.rechargeFeeRateHint") }}
                    </p>
                    <p
                      v-if="(Number(form.payment_recharge_fee_rate) || 0) > 0"
                      class="mt-1 text-xs font-[var(--fw-medium)] text-[var(--brand)] "
                    >
                      {{
                        t("admin.settings.payment.rechargeFeePreview", {
                          fee: (
                            Number(form.payment_recharge_fee_rate) || 0
                          ).toFixed(2),
                        })
                      }}
                    </p>
                  </div>
                  <div>
                    <label class="field-label"
                      >{{ t("admin.settings.payment.orderTimeout") }}
                      <span class="text-[var(--destructive)]">*</span></label
                    ><input
                      v-model.number="form.payment_order_timeout_minutes"
                      type="number"
                      min="1"
                      class="field-control"
                      required
                    />
                    <p class="mt-0.5 text-xs text-[var(--muted-foreground)]">
                      {{ t("admin.settings.payment.orderTimeoutHint") }}
                    </p>
                  </div>
                </div>
                <!-- Row 3: Pending orders + load balance + cancel rate limit (all in one row) -->
                <div class="flex flex-wrap items-end gap-4">
                  <div class="w-28">
                    <label class="field-label">{{
                      t("admin.settings.payment.maxPendingOrders")
                    }}</label
                    ><input
                      v-model.number="form.payment_max_pending_orders"
                      type="number"
                      min="1"
                      class="field-control"
                    />
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.loadBalanceStrategy")
                    }}</label>
                    <BaseSelect
                      v-model="form.payment_load_balance_strategy"
                      :options="loadBalanceOptions"
                      class="w-40"
                    />
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.cancelRateLimit")
                    }}</label>
                    <div class="flex items-center gap-2">
                      <Toggle v-model="form.payment_cancel_rate_limit_enabled" />
                      <BaseSelect
                        v-model="form.payment_cancel_rate_limit_window_mode"
                        :options="cancelRateLimitModeOptions"
                        class="w-24"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <span
                        :class="[
                          'text-sm whitespace-nowrap',
                          form.payment_cancel_rate_limit_enabled
                            ? 'text-[var(--body-copy)] text-[var(--body-copy)]'
                            : 'text-[var(--muted-foreground)] ',
                        ]"
                        >{{
                          t("admin.settings.payment.cancelRateLimitEvery")
                        }}</span
                      >
                      <input
                        v-model.number="form.payment_cancel_rate_limit_window"
                        type="number"
                        min="1"
                        required
                        class="field-control w-14 text-center"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <BaseSelect
                        v-model="form.payment_cancel_rate_limit_unit"
                        :options="cancelRateLimitUnitOptions"
                        class="w-28"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <span
                        :class="[
                          'text-sm whitespace-nowrap',
                          form.payment_cancel_rate_limit_enabled
                            ? 'text-[var(--body-copy)] text-[var(--body-copy)]'
                            : 'text-[var(--muted-foreground)] ',
                        ]"
                        >{{
                          t("admin.settings.payment.cancelRateLimitAllowMax")
                        }}</span
                      >
                      <input
                        v-model.number="form.payment_cancel_rate_limit_max"
                        type="number"
                        min="1"
                        required
                        class="field-control w-14 text-center"
                        :disabled="!form.payment_cancel_rate_limit_enabled"
                      />
                      <span
                        :class="[
                          'text-sm whitespace-nowrap',
                          form.payment_cancel_rate_limit_enabled
                            ? 'text-[var(--body-copy)] text-[var(--body-copy)]'
                            : 'text-[var(--muted-foreground)] ',
                        ]"
                        >{{
                          t("admin.settings.payment.cancelRateLimitTimes")
                        }}</span
                      >
                    </div>
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.alipayForceQRCode")
                    }}</label>
                    <div class="flex items-center gap-2">
                      <Toggle v-model="form.payment_alipay_force_qrcode" />
                      <span class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{
                        t("admin.settings.payment.alipayForceQRCodeHint")
                      }}</span>
                    </div>
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.alipayMobilePrecreateDeepLink")
                    }}</label>
                    <div class="flex items-center gap-2">
                      <Toggle v-model="form.payment_alipay_mobile_precreate_deep_link" />
                      <span class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{
                        t("admin.settings.payment.alipayMobilePrecreateDeepLinkHint")
                      }}</span>
                    </div>
                  </div>
                </div>
                <!-- Row 4: Enabled payment types (provider badges like sub2apipay) -->
                <div>
                  <label class="field-label">{{
                    t("admin.settings.payment.enabledPaymentTypes")
                  }}</label>
                  <div class="mt-1.5 flex flex-wrap gap-2">
                    <Button
                      v-for="pt in allPaymentTypes"
                      :key="pt.value"
                      type="button"
                      @click="togglePaymentType(pt.value)"
                      :variant="isPaymentTypeEnabled(pt.value) ? 'solid' : 'secondary'"
                      size="sm"
                      :aria-pressed="isPaymentTypeEnabled(pt.value)"
                    >
                      {{ pt.label }}
                    </Button>
                  </div>
                  <p class="mt-2 text-xs text-[var(--muted-foreground)] ">
                    {{ t("admin.settings.payment.enabledPaymentTypesHint") }}
                    <a
                      :href="paymentMethodsHref"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="ml-1 text-[var(--brand)] hover:text-[var(--brand)]  "
                    >
                      {{ t("admin.settings.payment.findProvider") }}
                      <svg
                        class="mb-0.5 ml-0.5 inline h-3 w-3"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                        />
                      </svg>
                    </a>
                  </p>
                </div>
                <!-- Row 5: Help image + text -->
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.helpImage")
                    }}</label>
                    <ImageUpload
                      v-model="form.payment_help_image_url"
                      :upload-label="t('admin.settings.site.uploadImage')"
                      :remove-label="t('admin.settings.site.remove')"
                      :placeholder="
                        t('admin.settings.payment.helpImagePlaceholder')
                      "
                    />
                  </div>
                  <div>
                    <label class="field-label">{{
                      t("admin.settings.payment.helpText")
                    }}</label>
                    <textarea
                      v-model="form.payment_help_text"
                      rows="3"
                      class="field-control"
                      :placeholder="
                        t('admin.settings.payment.helpTextPlaceholder')
                      "
                    ></textarea>
                  </div>
                </div>
              </template>
            </div>
          </div>

          <!-- Provider Management -->
          <PaymentProviderList
            v-if="form.payment_enabled"
            :providers="providers"
            :loading="providersLoading"
            :can-create="hasAnyPaymentTypeEnabled"
            :enabled-payment-types="form.payment_enabled_types"
            :all-payment-types="allPaymentTypes"
            :redirect-label="t('admin.settings.payment.easypayRedirect')"
            @refresh="loadProviders"
            @create="openCreateProvider"
            @edit="openEditProvider"
            @delete="confirmDeleteProvider"
            @toggle-field="handleToggleField"
            @toggle-type="handleToggleType"
            @reorder="handleReorderProviders"
          />
        </div>
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import Button from '@/components/common/Button.vue'
import Icon from '@/components/icons/Icon.vue'
import ImageUpload from '@/components/common/ImageUpload.vue'
import PaymentProviderList from '@/components/payment/PaymentProviderList.vue'
import BaseSelect from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useSettingsPageBindings } from './settingsPageBindings'

export default defineComponent({
  name: 'AdminPaymentSettingsPage',
  components: {
    BaseSelect,
    Button,
    Icon,
    ImageUpload,
    PaymentProviderList,
    Toggle,
  },
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
