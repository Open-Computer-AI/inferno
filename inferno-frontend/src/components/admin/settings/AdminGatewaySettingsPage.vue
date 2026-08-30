<template>
        <!-- Tab: Gateway -->
        <div class="space-y-6">
          <!-- Overload Cooldown (529) Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.overloadCooldown.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.overloadCooldown.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div
                v-if="overloadCooldownLoading"
                class="flex items-center gap-2 text-[var(--muted-foreground)]"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-[var(--brand-line)]"
                ></div>
                {{ t("common.loading") }}
              </div>

              <template v-else>
                <div class="flex items-center justify-between">
                  <div>
                    <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                      t("admin.settings.overloadCooldown.enabled")
                    }}</label>
                    <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.overloadCooldown.enabledHint") }}
                    </p>
                  </div>
                  <Toggle v-model="overloadCooldownForm.enabled" />
                </div>

                <div
                  v-if="overloadCooldownForm.enabled"
                  class="space-y-4 border-t border-[var(--border-subtle)] pt-4                ">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.overloadCooldown.cooldownMinutes") }}
                    </label>
                    <input
                      v-model.number="overloadCooldownForm.cooldown_minutes"
                      type="number"
                      min="1"
                      max="120"
                      class="field-control w-32"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        t("admin.settings.overloadCooldown.cooldownMinutesHint")
                      }}
                    </p>
                  </div>
                </div>

                <div
                  class="flex justify-end border-t border-[var(--border-subtle)] pt-4                ">
                  <AppButton
                    type="button"
                    @click="saveOverloadCooldownSettings"
                    :disabled="overloadCooldownSaving"
                    variant="solid"
                    :loading="overloadCooldownSaving"
                  >
                    {{
                      overloadCooldownSaving
                        ? t("common.saving")
                        : t("common.save")
                    }}
                  </AppButton>
                </div>
              </template>
            </div>
          </div>

          <!-- Rate Limit Cooldown (429) Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.rateLimit429Cooldown.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.rateLimit429Cooldown.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div
                v-if="rateLimit429CooldownLoading"
                class="flex items-center gap-2 text-[var(--muted-foreground)]"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-[var(--brand-line)]"
                ></div>
                {{ t("common.loading") }}
              </div>

              <template v-else>
                <div class="flex items-center justify-between">
                  <div>
                    <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                      t("admin.settings.rateLimit429Cooldown.enabled")
                    }}</label>
                    <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.rateLimit429Cooldown.enabledHint") }}
                    </p>
                  </div>
                  <Toggle v-model="rateLimit429CooldownForm.enabled" />
                </div>

                <div
                  v-if="rateLimit429CooldownForm.enabled"
                  class="space-y-4 border-t border-[var(--border-subtle)] pt-4                ">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{
                        t(
                          "admin.settings.rateLimit429Cooldown.cooldownSeconds",
                        )
                      }}
                    </label>
                    <input
                      v-model.number="rateLimit429CooldownForm.cooldown_seconds"
                      type="number"
                      min="1"
                      max="7200"
                      class="field-control w-32"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        t(
                          "admin.settings.rateLimit429Cooldown.cooldownSecondsHint",
                        )
                      }}
                    </p>
                  </div>
                </div>

                <div
                  class="flex justify-end border-t border-[var(--border-subtle)] pt-4                ">
                  <AppButton
                    type="button"
                    @click="saveRateLimit429CooldownSettings"
                    :disabled="rateLimit429CooldownSaving"
                    variant="solid"
                    :loading="rateLimit429CooldownSaving"
                  >
                    <svg
                      v-if="rateLimit429CooldownSaving"
                      class="mr-1 h-4 w-4 animate-spin"
                      fill="none"
                      viewBox="0 0 24 24"
                    >
                      <circle
                        class="opacity-25"
                        cx="12"
                        cy="12"
                        r="10"
                        stroke="currentColor"
                        stroke-width="4"
                      ></circle>
                      <path
                        class="opacity-75"
                        fill="currentColor"
                        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                      ></path>
                    </svg>
                    {{
                      rateLimit429CooldownSaving
                        ? t("common.saving")
                        : t("common.save")
                    }}
                  </AppButton>
                </div>
              </template>
            </div>
          </div>

          <!-- Stream Timeout Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.streamTimeout.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.streamTimeout.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Loading State -->
              <div
                v-if="streamTimeoutLoading"
                class="flex items-center gap-2 text-[var(--muted-foreground)]"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-[var(--brand-line)]"
                ></div>
                {{ t("common.loading") }}
              </div>

              <template v-else>
                <!-- Enable Stream Timeout -->
                <div class="flex items-center justify-between">
                  <div>
                    <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                      t("admin.settings.streamTimeout.enabled")
                    }}</label>
                    <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.streamTimeout.enabledHint") }}
                    </p>
                  </div>
                  <Toggle v-model="streamTimeoutForm.enabled" />
                </div>

                <!-- Settings - Only show when enabled -->
                <div
                  v-if="streamTimeoutForm.enabled"
                  class="space-y-4 border-t border-[var(--border-subtle)] pt-4                ">
                  <!-- Action -->
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.streamTimeout.action") }}
                    </label>
                    <BaseSelect
                      v-model="streamTimeoutForm.action"
                      :options="[
                        {
                          value: 'temp_unsched',
                          label: t('admin.settings.streamTimeout.actionTempUnsched'),
                        },
                        {
                          value: 'error',
                          label: t('admin.settings.streamTimeout.actionError'),
                        },
                        {
                          value: 'none',
                          label: t('admin.settings.streamTimeout.actionNone'),
                        },
                      ]"
                      aria-label="Stream timeout action"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.streamTimeout.actionHint") }}
                    </p>
                  </div>

                  <!-- Temp Unsched Minutes (only show when action is temp_unsched) -->
                  <div v-if="streamTimeoutForm.action === 'temp_unsched'">
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.streamTimeout.tempUnschedMinutes") }}
                    </label>
                    <input
                      v-model.number="streamTimeoutForm.temp_unsched_minutes"
                      type="number"
                      min="1"
                      max="60"
                      class="field-control w-32"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        t("admin.settings.streamTimeout.tempUnschedMinutesHint")
                      }}
                    </p>
                  </div>

                  <!-- Threshold Count -->
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.streamTimeout.thresholdCount") }}
                    </label>
                    <input
                      v-model.number="streamTimeoutForm.threshold_count"
                      type="number"
                      min="1"
                      max="10"
                      class="field-control w-32"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.streamTimeout.thresholdCountHint") }}
                    </p>
                  </div>

                  <!-- Threshold Window Minutes -->
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{
                        t("admin.settings.streamTimeout.thresholdWindowMinutes")
                      }}
                    </label>
                    <input
                      v-model.number="
                        streamTimeoutForm.threshold_window_minutes
                      "
                      type="number"
                      min="1"
                      max="60"
                      class="field-control w-32"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        t(
                          "admin.settings.streamTimeout.thresholdWindowMinutesHint",
                        )
                      }}
                    </p>
                  </div>
                </div>

                <!-- Save Button -->
                <div
                  class="flex justify-end border-t border-[var(--border-subtle)] pt-4                ">
                  <AppButton
                    type="button"
                    @click="saveStreamTimeoutSettings"
                    :disabled="streamTimeoutSaving"
                    variant="solid"
                    :loading="streamTimeoutSaving"
                  >
                    <svg
                      v-if="streamTimeoutSaving"
                      class="mr-1 h-4 w-4 animate-spin"
                      fill="none"
                      viewBox="0 0 24 24"
                    >
                      <circle
                        class="opacity-25"
                        cx="12"
                        cy="12"
                        r="10"
                        stroke="currentColor"
                        stroke-width="4"
                      ></circle>
                      <path
                        class="opacity-75"
                        fill="currentColor"
                        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                      ></path>
                    </svg>
                    {{
                      streamTimeoutSaving
                        ? t("common.saving")
                        : t("common.save")
                    }}
                  </AppButton>
                </div>
              </template>
            </div>
          </div>

          <!-- Request Rectifier Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.rectifier.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.rectifier.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Loading State -->
              <div
                v-if="rectifierLoading"
                class="flex items-center gap-2 text-[var(--muted-foreground)]"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-[var(--brand-line)]"
                ></div>
                {{ t("common.loading") }}
              </div>

              <template v-else>
                <!-- Master Toggle -->
                <div class="flex items-center justify-between">
                  <div>
                    <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                      t("admin.settings.rectifier.enabled")
                    }}</label>
                    <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.rectifier.enabledHint") }}
                    </p>
                  </div>
                  <Toggle v-model="rectifierForm.enabled" />
                </div>

                <!-- Sub-toggles (only show when master is enabled) -->
                <div
                  v-if="rectifierForm.enabled"
                  class="space-y-4 border-t border-[var(--border-subtle)] pt-4                ">
                  <!-- Thinking Signature Rectifier -->
                  <div class="flex items-center justify-between">
                    <div>
                      <label
                        class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >{{
                          t("admin.settings.rectifier.thinkingSignature")
                        }}</label
                      >
                      <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{
                          t("admin.settings.rectifier.thinkingSignatureHint")
                        }}
                      </p>
                    </div>
                    <Toggle
                      v-model="rectifierForm.thinking_signature_enabled"
                    />
                  </div>

                  <!-- Thinking Budget Rectifier -->
                  <div class="flex items-center justify-between">
                    <div>
                      <label
                        class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >{{
                          t("admin.settings.rectifier.thinkingBudget")
                        }}</label
                      >
                      <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.rectifier.thinkingBudgetHint") }}
                      </p>
                    </div>
                    <Toggle v-model="rectifierForm.thinking_budget_enabled" />
                  </div>

                  <!-- API Key Signature Rectifier -->
                  <div class="flex items-center justify-between">
                    <div>
                      <label
                        class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >{{
                          t("admin.settings.rectifier.apikeySignature")
                        }}</label
                      >
                      <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.rectifier.apikeySignatureHint") }}
                      </p>
                    </div>
                    <Toggle v-model="rectifierForm.apikey_signature_enabled" />
                  </div>

                  <!-- Custom Patterns (only when apikey_signature_enabled) -->
                  <div
                    v-if="rectifierForm.apikey_signature_enabled"
                    class="ml-4 space-y-3 border-l-2 border-[var(--border-subtle)] pl-4                  ">
                    <div>
                      <label
                        class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >{{
                          t("admin.settings.rectifier.apikeyPatterns")
                        }}</label
                      >
                      <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.rectifier.apikeyPatternsHint") }}
                      </p>
                    </div>
                    <div
                      v-for="(
                        _, index
                      ) in rectifierForm.apikey_signature_patterns"
                      :key="index"
                      class="flex items-center gap-2"
                    >
                      <input
                        v-model="rectifierForm.apikey_signature_patterns[index]"
                        type="text"
                        class="field-control input-sm flex-1"
                        :placeholder="
                          t('admin.settings.rectifier.apikeyPatternPlaceholder')
                        "
                      />
                      <AppButton
                        variant="ghost"
                        size="xs"
                        aria-label="Remove API key signature pattern"
                        type="button"
                        @click="
                          rectifierForm.apikey_signature_patterns.splice(
                            index,
                            1,
                          )
                        "
                      >
                        <svg
                          class="h-4 w-4"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M6 18L18 6M6 6l12 12"
                          />
                        </svg>
                      </AppButton>
                    </div>
                    <AppButton
                      variant="ghost"
                      size="xs"
                      type="button"
                      @click="rectifierForm.apikey_signature_patterns.push('')"
                    >
                      + {{ t("admin.settings.rectifier.addPattern") }}
                    </AppButton>
                  </div>
                </div>

                <!-- Save Button -->
                <div
                  class="flex justify-end border-t border-[var(--border-subtle)] pt-4                ">
                  <AppButton
                    type="button"
                    @click="saveRectifierSettings"
                    :disabled="rectifierSaving"
                    variant="solid"
                    :loading="rectifierSaving"
                  >
                    <svg
                      v-if="rectifierSaving"
                      class="mr-1 h-4 w-4 animate-spin"
                      fill="none"
                      viewBox="0 0 24 24"
                    >
                      <circle
                        class="opacity-25"
                        cx="12"
                        cy="12"
                        r="10"
                        stroke="currentColor"
                        stroke-width="4"
                      ></circle>
                      <path
                        class="opacity-75"
                        fill="currentColor"
                        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                      ></path>
                    </svg>
                    {{
                      rectifierSaving ? t("common.saving") : t("common.save")
                    }}
                  </AppButton>
                </div>
              </template>
            </div>
          </div>
          <!-- Beta Policy Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.betaPolicy.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.betaPolicy.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Loading State -->
              <div
                v-if="betaPolicyLoading"
                class="flex items-center gap-2 text-[var(--muted-foreground)]"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-[var(--brand-line)]"
                ></div>
                {{ t("common.loading") }}
              </div>

              <template v-else>
                <!-- Rule Cards -->
                <div
                  v-for="rule in betaPolicyForm.rules"
                  :key="rule.beta_token"
                  class="rounded-[var(--r-md)] border border-[var(--border-subtle)] p-4                ">
                  <div class="mb-3 flex items-center gap-2">
                    <span
                      class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] "
                    >
                      {{ getBetaDisplayName(rule.beta_token) }}
                    </span>
                    <span
                      class="rounded bg-[var(--surface-subtle)] px-2 py-0.5 text-xs text-[var(--muted-foreground)]  text-[var(--muted-foreground)]"
                    >
                      {{ rule.beta_token }}
                    </span>
                  </div>

                  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                    <!-- Action -->
                    <div>
                      <label
                        class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                      >
                        {{ t("admin.settings.betaPolicy.action") }}
                      </label>
                      <BaseSelect
                        :modelValue="rule.action"
                        @update:modelValue="rule.action = $event as any"
                        :options="betaPolicyActionOptions"
                      />
                    </div>

                    <!-- Scope -->
                    <div>
                      <label
                        class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                      >
                        {{ t("admin.settings.betaPolicy.scope") }}
                      </label>
                      <BaseSelect
                        :modelValue="rule.scope"
                        @update:modelValue="rule.scope = $event as any"
                        :options="betaPolicyScopeOptions"
                      />
                    </div>
                  </div>

                  <!-- Error Message (only when action=block) -->
                  <div v-if="rule.action === 'block'" class="mt-3">
                    <label
                      class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                    >
                      {{ t("admin.settings.betaPolicy.errorMessage") }}
                    </label>
                    <input
                      v-model="rule.error_message"
                      type="text"
                      class="field-control"
                      :placeholder="
                        t('admin.settings.betaPolicy.errorMessagePlaceholder')
                      "
                    />
                    <p class="mt-1 text-xs text-[var(--muted-foreground)] ">
                      {{ t("admin.settings.betaPolicy.errorMessageHint") }}
                    </p>
                  </div>

                  <!-- Quick Presets (only for tokens with presets) -->
                  <div v-if="betaPresets[rule.beta_token]?.length" class="mt-3">
                    <label
                      class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                    >
                      {{ t("admin.settings.betaPolicy.quickPresets") }}
                    </label>
                    <div class="flex flex-wrap gap-2">
                      <AppButton
                        v-for="preset in betaPresets[rule.beta_token]"
                        :key="preset.label"
                        variant="ghost"
                        size="xs"
                        type="button"
                        @click="applyBetaPreset(rule, preset)"
                        :title="preset.description"
                      >
                        {{ preset.label }}
                      </AppButton>
                    </div>
                  </div>

                  <!-- Model Whitelist -->
                  <div class="mt-3">
                    <label
                      class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                    >
                      {{ t("admin.settings.betaPolicy.modelWhitelist") }}
                    </label>
                    <p class="mb-2 text-xs text-[var(--muted-foreground)] ">
                      {{ t("admin.settings.betaPolicy.modelWhitelistHint") }}
                    </p>
                    <!-- Existing patterns -->
                    <div
                      v-for="(_, index) in rule.model_whitelist || []"
                      :key="index"
                      class="mb-1.5 flex items-center gap-2"
                    >
                      <input
                        v-model="rule.model_whitelist![index]"
                        type="text"
                        class="field-control input-sm flex-1"
                        :placeholder="
                          t('admin.settings.betaPolicy.modelPatternPlaceholder')
                        "
                      />
                      <AppButton
                        variant="ghost"
                        size="xs"
                        aria-label="Remove model whitelist pattern"
                        type="button"
                        @click="rule.model_whitelist!.splice(index, 1)"
                      >
                        <svg
                          class="h-4 w-4"
                          fill="none"
                          viewBox="0 0 24 24"
                          stroke="currentColor"
                          stroke-width="2"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M6 18L18 6M6 6l12 12"
                          />
                        </svg>
                      </AppButton>
                    </div>
                    <!-- Add pattern button -->
                    <AppButton
                      variant="ghost"
                      size="xs"
                      type="button"
                      @click="
                        if (!rule.model_whitelist) rule.model_whitelist = [];
                        rule.model_whitelist.push('');
                      "
                    >
                      <svg
                        class="h-3.5 w-3.5"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                        stroke-width="2"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          d="M12 4v16m8-8H4"
                        />
                      </svg>
                      {{ t("admin.settings.betaPolicy.addModelPattern") }}
                    </AppButton>
                    <!-- Common pattern chips -->
                    <div class="flex flex-wrap items-center gap-1.5">
                      <span class="text-xs text-[var(--muted-foreground)] "
                        >{{
                          t("admin.settings.betaPolicy.commonPatterns")
                        }}:</span
                      >
                      <AppButton
                        v-for="pattern in commonModelPatterns"
                        :key="pattern"
                        variant="ghost"
                        size="xs"
                        type="button"
                        @click="addQuickPattern(rule, pattern)"
                      >
                        {{ pattern }}
                      </AppButton>
                    </div>
                  </div>

                  <!-- Other Models Action (only when target models are non-empty) -->
                  <div
                    v-if="hasOpenAIFastPolicyTargetModels(rule)"
                    class="mt-3"
                  >
                    <label
                      class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                    >
                      {{ t("admin.settings.betaPolicy.fallbackAction") }}
                    </label>
                    <BaseSelect
                      :modelValue="rule.fallback_action || 'pass'"
                      @update:modelValue="rule.fallback_action = $event as any"
                      :options="betaPolicyActionOptions"
                    />
                    <p class="mt-1 text-xs text-[var(--muted-foreground)] ">
                      {{ t("admin.settings.betaPolicy.fallbackActionHint") }}
                    </p>
                    <!-- Fallback Error Message (only when fallback_action=block) -->
                    <div v-if="rule.fallback_action === 'block'" class="mt-2">
                      <input
                        v-model="rule.fallback_error_message"
                        type="text"
                        class="field-control"
                        :placeholder="
                          t(
                            'admin.settings.betaPolicy.fallbackErrorMessagePlaceholder',
                          )
                        "
                      />
                      <p class="mt-1 text-xs text-[var(--muted-foreground)] ">
                        {{ t("admin.settings.betaPolicy.errorMessageHint") }}
                      </p>
                    </div>
                  </div>
                </div>

                <!-- Save Button -->
                <div
                  class="flex justify-end border-t border-[var(--border-subtle)] pt-4                ">
                  <AppButton
                    type="button"
                    @click="saveBetaPolicySettings"
                    :disabled="betaPolicySaving"
                    variant="solid"
                    :loading="betaPolicySaving"
                  >
                    <svg
                      v-if="betaPolicySaving"
                      class="mr-1 h-4 w-4 animate-spin"
                      fill="none"
                      viewBox="0 0 24 24"
                    >
                      <circle
                        class="opacity-25"
                        cx="12"
                        cy="12"
                        r="10"
                        stroke="currentColor"
                        stroke-width="4"
                      ></circle>
                      <path
                        class="opacity-75"
                        fill="currentColor"
                        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                      ></path>
                    </svg>
                    {{
                      betaPolicySaving ? t("common.saving") : t("common.save")
                    }}
                  </AppButton>
                </div>
              </template>
            </div>
          </div>
          <!-- OpenAI Fast/Flex Policy Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.openaiFastPolicy.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.openaiFastPolicy.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Empty state -->
              <div
                v-if="openaiFastPolicyForm.rules.length === 0"
                class="rounded-[var(--r-md)] border border-dashed border-[var(--border-subtle)] p-6 text-center text-sm text-[var(--muted-foreground)]  text-[var(--muted-foreground)]"
              >
                {{ t("admin.settings.openaiFastPolicy.empty") }}
              </div>

              <!-- Rule Cards -->
              <div
                v-for="(rule, ruleIndex) in openaiFastPolicyForm.rules"
                :key="ruleIndex"
                class="rounded-[var(--r-md)] border border-[var(--border-subtle)] p-4              ">
                <div class="mb-3 flex items-center justify-between">
                  <span
                    class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] "
                  >
                    {{
                      t("admin.settings.openaiFastPolicy.ruleHeader", {
                        index: ruleIndex + 1,
                      })
                    }}
                  </span>
                  <AppButton
                    variant="ghost"
                    size="xs"
                    aria-label="Remove OpenAI fast policy rule"
                    type="button"
                    @click="removeOpenAIFastPolicyRule(ruleIndex)"
                    :title="t('admin.settings.openaiFastPolicy.removeRule')"
                  >
                    <svg
                      class="h-4 w-4"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      stroke-width="2"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M6 18L18 6M6 6l12 12"
                      />
                    </svg>
                  </AppButton>
                </div>

                <!-- A one-line reading of the rule, so an operator does not have
                     to infer it from three separate controls. -->
                <div
                  class="mb-4 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[var(--muted-foreground)]"
                  :data-testid="`openai-fast-policy-summary-${ruleIndex}`"
                >
                  <span class="font-[var(--fw-medium)] text-[var(--body-copy)]">
                    {{
                      t(
                        hasOpenAIFastPolicyTargetModels(rule)
                          ? 'admin.settings.openaiFastPolicy.summaryTargetModels'
                          : 'admin.settings.openaiFastPolicy.summaryAllModels'
                      )
                    }}
                  </span>
                  <span aria-hidden="true">-&gt;</span>
                  <span
                    class="inline-flex items-center rounded-[var(--r-sm)] bg-[var(--brand-tint)] px-2 py-0.5 font-[var(--fw-medium)] text-[var(--brand)]"
                  >
                    {{ openaiFastPolicyActionSummary(rule.action) }}
                  </span>
                  <template v-if="hasOpenAIFastPolicyTargetModels(rule)">
                    <span aria-hidden="true">·</span>
                    <span>{{ t('admin.settings.openaiFastPolicy.summaryOtherModels') }}</span>
                    <span
                      class="inline-flex items-center rounded-[var(--r-sm)] bg-[var(--muted)] px-2 py-0.5 font-[var(--fw-medium)] text-[var(--muted-foreground)]"
                    >
                      {{ openaiFastPolicyActionSummary(rule.fallback_action || 'pass') }}
                    </span>
                  </template>
                </div>

                <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
                  <!-- Service Tier -->
                  <div>
                    <label
                      class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                    >
                      {{ t("admin.settings.openaiFastPolicy.serviceTier") }}
                    </label>
                    <BaseSelect
                      :modelValue="rule.service_tier"
                      @update:modelValue="
                        rule.service_tier = $event as
                          | 'all'
                          | 'priority'
                          | 'flex'
                      "
                      :options="openaiFastPolicyTierOptions"
                    />
                  </div>

                  <!-- Action -->
                  <div>
                    <label
                      class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                    >
                      {{ t("admin.settings.openaiFastPolicy.action") }}
                    </label>
                    <BaseSelect
                      :modelValue="rule.action"
                      @update:modelValue="
                        rule.action = $event as
                          | 'pass'
                          | 'filter'
                          | 'block'
                          | 'force_priority'
                      "
                      :options="openaiFastPolicyActionOptions"
                    />
                  </div>

                  <!-- Scope -->
                  <div>
                    <label
                      class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                    >
                      {{ t("admin.settings.openaiFastPolicy.scope") }}
                    </label>
                    <BaseSelect
                      :modelValue="rule.scope"
                      @update:modelValue="
                        rule.scope = $event as
                          | 'all'
                          | 'oauth'
                          | 'apikey'
                          | 'bedrock'
                      "
                      :options="openaiFastPolicyScopeOptions"
                    />
                  </div>
                </div>

                <!-- User Scope -->
                <div class="mt-3">
                  <label
                    class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                  >
                    {{ t("admin.settings.openaiFastPolicy.userIds") }}
                  </label>
                  <p class="mb-2 text-xs text-[var(--muted-foreground)] ">
                    {{ t("admin.settings.openaiFastPolicy.userIdsHint") }}
                  </p>
                  <OpenAIFastPolicyUserSelector
                    :model-value="rule.user_ids || []"
                    @update:model-value="rule.user_ids = $event"
                  />
                </div>

                <!-- Error Message (only when action=block) -->
                <div v-if="rule.action === 'block'" class="mt-3">
                  <label
                    class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                  >
                    {{ t("admin.settings.openaiFastPolicy.errorMessage") }}
                  </label>
                  <input
                    v-model="rule.error_message"
                    type="text"
                    class="field-control"
                    :placeholder="
                      t(
                        'admin.settings.openaiFastPolicy.errorMessagePlaceholder',
                      )
                    "
                  />
                  <p class="mt-1 text-xs text-[var(--muted-foreground)] ">
                    {{ t("admin.settings.openaiFastPolicy.errorMessageHint") }}
                  </p>
                </div>

                <!-- Target Models -->
                <div
                  class="mt-3"
                  role="group"
                  :aria-labelledby="`openai-fast-policy-models-label-${ruleIndex}`"
                  :aria-describedby="`openai-fast-policy-models-hint-${ruleIndex}`"
                >
                  <label
                    :id="`openai-fast-policy-models-label-${ruleIndex}`"
                    class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                  >
                    {{ t("admin.settings.openaiFastPolicy.modelWhitelist") }}
                  </label>
                  <p
                    :id="`openai-fast-policy-models-hint-${ruleIndex}`"
                    class="mb-2 text-xs text-[var(--muted-foreground)]"
                  >
                    {{
                      t("admin.settings.openaiFastPolicy.modelWhitelistHint")
                    }}
                  </p>
                  <div
                    v-for="(_, patternIdx) in rule.model_whitelist || []"
                    :key="patternIdx"
                    class="mb-1.5 flex items-center gap-2"
                  >
                    <input
                      v-model="rule.model_whitelist![patternIdx]"
                      type="text"
                      class="field-control input-sm flex-1"
                      :placeholder="
                        t(
                          'admin.settings.openaiFastPolicy.modelPatternPlaceholder',
                        )
                      "
                    />
                    <AppButton
                      variant="ghost"
                      size="xs"
                      aria-label="Remove OpenAI model whitelist pattern"
                      type="button"
                      @click="
                        removeOpenAIFastPolicyModelPattern(rule, patternIdx)
                      "
                    >
                      <svg
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                        stroke-width="2"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          d="M6 18L18 6M6 6l12 12"
                        />
                      </svg>
                    </AppButton>
                  </div>
                  <AppButton
                    variant="ghost"
                    size="xs"
                    type="button"
                    @click="addOpenAIFastPolicyModelPattern(rule)"
                  >
                    <svg
                      class="h-3.5 w-3.5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      stroke-width="2"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M12 4v16m8-8H4"
                      />
                    </svg>
                    {{ t("admin.settings.openaiFastPolicy.addModelPattern") }}
                  </AppButton>
                </div>

                <!-- Fallback Action (only when model_whitelist is non-empty) -->
                <div
                  v-if="
                    rule.model_whitelist && rule.model_whitelist.length > 0
                  "
                  class="mt-3"
                >
                  <label
                    class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]"
                  >
                    {{ t("admin.settings.openaiFastPolicy.fallbackAction") }}
                  </label>
                  <BaseSelect
                    :modelValue="rule.fallback_action || 'pass'"
                    @update:modelValue="
                      rule.fallback_action = $event as
                        | 'pass'
                        | 'filter'
                        | 'block'
                        | 'force_priority'
                    "
                    :options="openaiFastPolicyActionOptions"
                  />
                  <p class="mt-1 text-xs text-[var(--muted-foreground)] ">
                    {{
                      t("admin.settings.openaiFastPolicy.fallbackActionHint")
                    }}
                  </p>
                  <div v-if="rule.fallback_action === 'block'" class="mt-2">
                    <input
                      v-model="rule.fallback_error_message"
                      type="text"
                      class="field-control"
                      :placeholder="
                        t(
                          'admin.settings.openaiFastPolicy.fallbackErrorMessagePlaceholder',
                        )
                      "
                    />
                  </div>
                </div>
              </div>

              <!-- Add Rule Button -->
              <div>
                <AppButton
                  variant="secondary"
                  size="sm"
                  type="button"
                  @click="addOpenAIFastPolicyRule"
                >
                  <svg
                    class="h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 4v16m8-8H4"
                    />
                  </svg>
                  {{ t("admin.settings.openaiFastPolicy.addRule") }}
                </AppButton>
                <p class="mt-2 text-xs text-[var(--muted-foreground)] ">
                  {{ t("admin.settings.openaiFastPolicy.saveHint") }}
                </p>
              </div>
            </div>
          </div>
        </div>
        <!-- /Tab: Gateway -->

        <!-- Tab: Gateway — Claude Code, Scheduling -->
        <div class="space-y-6">
          <!-- Claude Code Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.claudeCode.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.claudeCode.description") }}
              </p>
            </div>
            <div class="p-6">
              <div>
                <label
                  class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                >
                  {{ t("admin.settings.claudeCode.minVersion") }}
                </label>
                <input
                  v-model="form.min_claude_code_version"
                  type="text"
                  class="field-control max-w-xs font-mono text-sm"
                  :placeholder="
                    t('admin.settings.claudeCode.minVersionPlaceholder')
                  "
                />
                <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t("admin.settings.claudeCode.minVersionHint") }}
                </p>
              </div>
              <div class="mt-4">
                <label
                  class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                >
                  {{ t("admin.settings.claudeCode.maxVersion") }}
                </label>
                <input
                  v-model="form.max_claude_code_version"
                  type="text"
                  class="field-control max-w-xs font-mono text-sm"
                  :placeholder="
                    t('admin.settings.claudeCode.maxVersionPlaceholder')
                  "
                />
                <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t("admin.settings.claudeCode.maxVersionHint") }}
                </p>
              </div>
            </div>
          </div>

          <!-- Codex Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.gatewayForwarding.codexHardeningTitle") }}
              </h2>
            </div>
            <div class="p-6 space-y-4">
                <div>
                  <h3 class="text-base font-[var(--fw-medium)] text-[var(--foreground)] ">
                    {{ t("admin.settings.gatewayForwarding.codexClientRestrictionTitle") }}
                  </h3>
                  <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.gatewayForwarding.codexHardeningDesc") }}
                  </p>
                </div>
                <div class="grid gap-4 sm:grid-cols-2">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.gatewayForwarding.minCodexVersion") }}
                    </label>
                    <input
                      v-model="form.min_codex_version"
                      type="text"
                      class="field-control w-full font-mono text-sm"
                      :placeholder="
                        t(
                          'admin.settings.gatewayForwarding.minCodexVersionPlaceholder',
                        )
                      "
                    />
                  </div>
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.gatewayForwarding.maxCodexVersion") }}
                    </label>
                    <input
                      v-model="form.max_codex_version"
                      type="text"
                      class="field-control w-full font-mono text-sm"
                      :placeholder="
                        t(
                          'admin.settings.gatewayForwarding.maxCodexVersionPlaceholder',
                        )
                      "
                    />
                  </div>
                </div>
                <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t("admin.settings.gatewayForwarding.codexVersionHint") }}
                </p>

                <div>
                  <label class="block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                    {{ t("admin.settings.gatewayForwarding.codexFingerprintSignals") }}
                  </label>
                  <p class="mb-2 mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.gatewayForwarding.codexFingerprintSignalsDesc") }}
                  </p>
                  <div
                    v-for="(row, i) in codexFingerprintRows"
                    :key="`codex-fp-${i}`"
                    class="mb-2 flex items-center gap-2"
                  >
                    <BaseSelect
                      v-model="row.type"
                      :options="[
                        {
                          value: 'header_exact',
                          label: t('admin.settings.gatewayForwarding.codexFpTypeHeaderExact'),
                        },
                        {
                          value: 'header_prefix',
                          label: t('admin.settings.gatewayForwarding.codexFpTypeHeaderPrefix'),
                        },
                        {
                          value: 'body_path',
                          label: t('admin.settings.gatewayForwarding.codexFpTypeBodyPath'),
                        },
                      ]"
                      aria-label="Codex fingerprint signal type"
                    />
                    <input
                      v-model="row.match"
                      type="text"
                      class="field-control flex-1 font-mono text-sm"
                      :placeholder="t('admin.settings.gatewayForwarding.codexFpMatchPlaceholder')"
                    />
                    <label class="flex shrink-0 items-center gap-1 text-xs text-[var(--body-copy)] text-[var(--muted-foreground)]">
                      <Checkbox v-model="row.required" />
                      {{ t("admin.settings.gatewayForwarding.codexFpRequired") }}
                    </label>
                    <AppButton
                      variant="danger"
                      size="sm"
                      type="button"
                      @click="removeCodexFingerprintRow(i)"
                    >
                      {{ t("admin.settings.gatewayForwarding.codexRemoveRow") }}
                    </AppButton>
                  </div>
                  <AppButton type="button" variant="secondary" size="sm" @click="addCodexFingerprintRow">
                    {{ t("admin.settings.gatewayForwarding.codexAddRow") }}
                  </AppButton>
                  <p
                    v-if="codexFingerprintNoRequired"
                    class="mt-2 text-xs text-[var(--s2a-attn)] "
                  >
                    {{ t("admin.settings.gatewayForwarding.codexFingerprintNoRequiredWarn") }}
                  </p>
                </div>

                <div class="flex items-center justify-between">
                  <div class="pr-4">
                    <label
                      class="block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{
                        t("admin.settings.gatewayForwarding.codexAllowAppServer")
                      }}
                    </label>
                    <p class="mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        t(
                          "admin.settings.gatewayForwarding.codexAllowAppServerDesc",
                        )
                      }}
                    </p>
                  </div>
                  <Toggle
                    v-model="form.codex_cli_only_allow_app_server_clients"
                  />
                </div>

                <div>
                  <label
                    class="block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.gatewayForwarding.codexBlacklist") }}
                  </label>
                  <p class="mb-2 mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.gatewayForwarding.codexBlacklistDesc") }}
                  </p>
                  <div
                    v-for="(row, i) in codexBlacklistRows"
                    :key="`codex-bl-${i}`"
                    class="mb-2 flex gap-2"
                  >
                    <input
                      v-model="row.originator"
                      type="text"
                      class="field-control w-1/3 font-mono text-sm"
                      :placeholder="
                        t(
                          'admin.settings.gatewayForwarding.codexOriginatorPlaceholder',
                        )
                      "
                    />
                    <input
                      v-model="row.uaContains"
                      type="text"
                      class="field-control flex-1 font-mono text-sm"
                      :placeholder="
                        t(
                          'admin.settings.gatewayForwarding.codexUaContainsPlaceholder',
                        )
                      "
                    />
                    <AppButton
                      variant="danger"
                      size="sm"
                      type="button"
                      @click="removeCodexBlacklistRow(i)"
                    >
                      {{ t("admin.settings.gatewayForwarding.codexRemoveRow") }}
                    </AppButton>
                  </div>
                  <AppButton
                    variant="secondary"
                    size="sm"
                    type="button"
                    @click="addCodexBlacklistRow"
                  >
                    {{ t("admin.settings.gatewayForwarding.codexAddRow") }}
                  </AppButton>
                </div>

                <div>
                  <label
                    class="block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.gatewayForwarding.codexWhitelist") }}
                  </label>
                  <p class="mb-2 mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.gatewayForwarding.codexWhitelistDesc") }}
                  </p>
                  <div
                    v-for="(row, i) in codexWhitelistRows"
                    :key="`codex-wl-${i}`"
                    class="mb-2 flex gap-2"
                  >
                    <input
                      v-model="row.originator"
                      type="text"
                      class="field-control w-1/3 font-mono text-sm"
                      :placeholder="
                        t(
                          'admin.settings.gatewayForwarding.codexOriginatorPlaceholder',
                        )
                      "
                    />
                    <input
                      v-model="row.uaContains"
                      type="text"
                      class="field-control flex-1 font-mono text-sm"
                      :placeholder="
                        t(
                          'admin.settings.gatewayForwarding.codexUaContainsPlaceholder',
                        )
                      "
                    />
                    <label
                      class="flex shrink-0 items-center gap-1 text-xs text-[var(--body-copy)] text-[var(--muted-foreground)]"
                      :title="
                        t(
                          'admin.settings.gatewayForwarding.codexWhitelistSkipFingerprintTooltip',
                        )
                      "
                    >
                      <Checkbox v-model="row.skipEngineFingerprint" />
                      {{
                        t(
                          'admin.settings.gatewayForwarding.codexWhitelistSkipFingerprint',
                        )
                      }}
                    </label>
                    <AppButton
                      variant="danger"
                      size="sm"
                      type="button"
                      @click="removeCodexWhitelistRow(i)"
                    >
                      {{ t("admin.settings.gatewayForwarding.codexRemoveRow") }}
                    </AppButton>
                  </div>
                  <AppButton
                    variant="secondary"
                    size="sm"
                    type="button"
                    @click="addCodexWhitelistRow"
                  >
                    {{ t("admin.settings.gatewayForwarding.codexAddRow") }}
                  </AppButton>
                </div>
            </div>
          </div>

          <!-- Upstream Billing Probe Settings -->
          <div class="settings-surface" data-testid="upstream-billing-probe-settings">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.upstreamBillingProbe.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.upstreamBillingProbe.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div
                v-if="upstreamBillingProbeLoading"
                class="flex items-center gap-2 text-[var(--muted-foreground)]"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-[var(--brand-line)]"
                ></div>
                {{ t("common.loading") }}
              </div>

              <template v-else>
                <div class="flex items-center justify-between gap-4">
                  <div>
                    <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                      {{ t("admin.settings.upstreamBillingProbe.enabled") }}
                    </label>
                    <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.upstreamBillingProbe.enabledHint") }}
                    </p>
                  </div>
                  <Toggle
                    v-model="upstreamBillingProbeForm.enabled"
                    :aria-label="t('admin.settings.upstreamBillingProbe.enabled')"
                    data-testid="upstream-billing-probe-enabled"
                  />
                </div>

                <div
                  v-if="upstreamBillingProbeForm.enabled"
                  class="border-t border-[var(--border-subtle)] pt-4                ">
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    for="upstream-billing-probe-interval"
                  >
                    {{ t("admin.settings.upstreamBillingProbe.intervalMinutes") }}
                  </label>
                  <input
                    id="upstream-billing-probe-interval"
                    v-model.number="upstreamBillingProbeForm.interval_minutes"
                    type="number"
                    min="5"
                    max="1440"
                    class="field-control w-32"
                    data-testid="upstream-billing-probe-interval"
                    @keydown.enter.prevent="saveUpstreamBillingProbeSettings"
                  />
                  <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.upstreamBillingProbe.intervalHint") }}
                  </p>
                </div>

                <div
                  class="flex justify-end border-t border-[var(--border-subtle)] pt-4                ">
                  <AppButton
                    type="button"
                    variant="solid"
                    :loading="upstreamBillingProbeSaving"
                    :disabled="upstreamBillingProbeSaving"
                    data-testid="upstream-billing-probe-save"
                    @click="saveUpstreamBillingProbeSettings"
                  >
                    {{
                      upstreamBillingProbeSaving
                        ? t("common.saving")
                        : t("common.save")
                    }}
                  </AppButton>
                </div>
              </template>
            </div>
          </div>

          <!-- Ollama Cloud Usage Settings -->
          <div class="settings-surface" data-testid="ollama-cloud-usage-global-settings">
            <div class="border-b border-[var(--border-subtle)] px-6 py-4              ">
<h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.ollamaCloudUsage.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.ollamaCloudUsage.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div v-if="ollamaCloudUsageLoading" class="flex items-center gap-2 text-[var(--muted-foreground)]">
                <div class="h-4 w-4 animate-spin rounded-full border-b-2 border-[var(--brand-line)]"></div>
                {{ t("common.loading") }}
              </div>
              <template v-else>
                <div class="flex items-center justify-between gap-4">
                  <div>
                    <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                      {{ t("admin.settings.ollamaCloudUsage.enabled") }}
                    </label>
                    <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.ollamaCloudUsage.enabledHint") }}
                    </p>
                  </div>
                  <Toggle
                    v-model="ollamaCloudUsageForm.enabled"
                    :aria-label="t('admin.settings.ollamaCloudUsage.enabled')"
                    data-testid="ollama-cloud-usage-global-enabled"
                  />
                </div>
                <div v-if="ollamaCloudUsageForm.enabled" class="space-y-4 border-t border-[var(--border-subtle)] pt-4                  ">
<div>
                    <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]" for="ollama-cloud-usage-debounce">
                      {{ t("admin.settings.ollamaCloudUsage.debounceMinutes") }}
                    </label>
                    <input
                      id="ollama-cloud-usage-debounce"
                      v-model.number="ollamaCloudUsageForm.debounce_minutes"
                      type="number"
                      min="1"
                      max="60"
                      class="field-control w-32"
                      data-testid="ollama-cloud-usage-global-debounce"
                      @keydown.enter.prevent="saveOllamaCloudUsageSettings"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.ollamaCloudUsage.debounceHint") }}
                    </p>
                  </div>
                  <div>
                    <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]" for="ollama-cloud-usage-interval">
                      {{ t("admin.settings.ollamaCloudUsage.intervalMinutes") }}
                    </label>
                    <input
                      id="ollama-cloud-usage-interval"
                      v-model.number="ollamaCloudUsageForm.interval_minutes"
                      type="number"
                      min="15"
                      max="1440"
                      class="field-control w-32"
                      data-testid="ollama-cloud-usage-global-interval"
                      @keydown.enter.prevent="saveOllamaCloudUsageSettings"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.ollamaCloudUsage.intervalHint") }}
                    </p>
                  </div>
                </div>
                <div class="flex justify-end border-t border-[var(--border-subtle)] pt-4                  ">
<AppButton
                    type="button"
                    variant="solid"
                    :loading="ollamaCloudUsageSaving"
                    :disabled="ollamaCloudUsageSaving"
                    data-testid="ollama-cloud-usage-global-save"
                    @click="saveOllamaCloudUsageSettings"
                  >
                    {{ ollamaCloudUsageSaving ? t("common.saving") : t("common.save") }}
                  </AppButton>
                </div>
              </template>
            </div>
          </div>

          <!-- Gateway Scheduling Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.scheduling.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.scheduling.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.scheduling.allowUngroupedKey") }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.scheduling.allowUngroupedKeyHint") }}
                  </p>
                </div>
                <Toggle v-model="form.allow_ungrouped_key_scheduling" />
              </div>

              <div class="border-t border-[var(--border-subtle)] pt-4                ">
<div class="mb-3">
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                    {{
                      t(
                        "admin.settings.scheduling.accountSchedulingThresholdsTitle",
                      )
                    }}
                  </label>
                  <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t(
                        "admin.settings.scheduling.accountSchedulingThresholdsDescription",
                      )
                    }}
                  </p>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t(
                        "admin.settings.scheduling.accountSchedulingThresholdsGlobalHint",
                      )
                    }}
                  </p>
                  <p class="mt-0.5 text-xs text-[var(--s2a-attn)] ">
                    {{
                      t(
                        "admin.settings.scheduling.accountSchedulingThresholdsDisabledHint",
                      )
                    }}
                  </p>
                </div>
                <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  <div
                    v-for="platform in schedulingThresholdPlatforms"
                    :key="platform"
                    class="rounded-[var(--r-md)] border border-[var(--border-subtle)] p-4                  ">
                    <div class="flex items-start justify-between gap-3">
                      <div>
                        <label
                          class="font-mono text-sm font-[var(--fw-medium)] text-[var(--foreground)] "
                        >
                          {{ platform }}
                        </label>
                        <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{
                            t(
                              "admin.settings.scheduling.accountSchedulingThresholdsRangeHint",
                            )
                          }}
                        </p>
                      </div>
                      <span
                        class="rounded bg-[var(--surface-subtle)] px-2 py-0.5 text-[11px] font-[var(--fw-medium)] text-[var(--body-copy)]  text-[var(--body-copy)]"
                      >
                        %
                      </span>
                    </div>
                    <input
                      v-model.number="form.account_scheduling_thresholds[platform]"
                      type="number"
                      min="1"
                      max="100"
                      step="1"
                      class="field-control mt-3"
                      :data-testid="`account-scheduling-threshold-${platform}`"
                      placeholder="100"
                    />
                  </div>
                </div>
              </div>

              <div
                v-if="!form.openai_advanced_scheduler_enabled"
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-5              ">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.openaiExperimentalScheduler.lowRatePriorityTitle") }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t("admin.settings.openaiExperimentalScheduler.lowRatePriorityDescription")
                    }}
                  </p>
                </div>
                <Toggle
                  v-model="form.openai_low_upstream_rate_priority_enabled"
                  data-testid="openai-low-rate-priority-toggle"
                />
              </div>

              <div
                v-if="!form.openai_advanced_scheduler_enabled && form.openai_low_upstream_rate_priority_enabled"
                class="flex flex-col items-stretch gap-3 border-t border-[var(--border-subtle)] pt-5 sm:flex-row sm:items-start sm:justify-between sm:gap-6              ">
                <div class="min-w-0">
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    for="openai-oauth-scheduling-rate-multiplier"
                  >
                    {{ t("admin.settings.openaiExperimentalScheduler.oauthRateTitle") }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.openaiExperimentalScheduler.oauthRatePriorityDescription") }}
                  </p>
                </div>
                <div class="relative w-full shrink-0 sm:w-32">
                  <input
                    id="openai-oauth-scheduling-rate-multiplier"
                    v-model.number="form.openai_oauth_scheduling_rate_multiplier"
                    class="field-control pr-8"
                    data-testid="openai-oauth-scheduling-rate-multiplier"
                    min="0"
                    required
                    step="0.01"
                    type="number"
                  />
                  <span
                    class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-[var(--muted-foreground)]"
                  >x</span>
                </div>
              </div>

              <div class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-5                ">
<div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.openaiExperimentalScheduler.title") }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t("admin.settings.openaiExperimentalScheduler.description")
                    }}
                  </p>
                </div>
                <Toggle
                  v-model="form.openai_advanced_scheduler_enabled"
                  data-testid="openai-advanced-scheduler-toggle"
                />
              </div>

              <div
                v-if="form.openai_advanced_scheduler_enabled"
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-5              ">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.openaiExperimentalScheduler.stickyWeightedTitle") }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t("admin.settings.openaiExperimentalScheduler.stickyWeightedDescription")
                    }}
                  </p>
                </div>
                <Toggle v-model="form.openai_advanced_scheduler_sticky_weighted_enabled" />
              </div>

              <div
                v-if="form.openai_advanced_scheduler_enabled"
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-5              ">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.openaiExperimentalScheduler.subscriptionPriorityTitle") }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t("admin.settings.openaiExperimentalScheduler.subscriptionPriorityDescription")
                    }}
                  </p>
                </div>
                <Toggle v-model="form.openai_advanced_scheduler_subscription_priority_enabled" />
              </div>

              <div
                v-if="form.openai_advanced_scheduler_enabled"
                class="flex flex-col items-stretch gap-3 border-t border-[var(--border-subtle)] pt-5 sm:flex-row sm:items-start sm:justify-between sm:gap-6              ">
                <div class="min-w-0">
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    for="openai-oauth-scheduling-rate-multiplier"
                  >
                    {{ t("admin.settings.openaiExperimentalScheduler.oauthRateTitle") }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.openaiExperimentalScheduler.oauthRateWeightedDescription") }}
                  </p>
                </div>
                <div class="relative w-full shrink-0 sm:w-32">
                  <input
                    id="openai-oauth-scheduling-rate-multiplier"
                    v-model.number="form.openai_oauth_scheduling_rate_multiplier"
                    class="field-control pr-8"
                    data-testid="openai-oauth-scheduling-rate-multiplier"
                    min="0"
                    required
                    step="0.01"
                    type="number"
                  />
                  <span
                    class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-[var(--muted-foreground)]"
                  >x</span>
                </div>
              </div>

              <div
                v-if="form.openai_advanced_scheduler_enabled"
                class="border-t border-[var(--border-subtle)] pt-5              ">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.openaiExperimentalScheduler.weightsTitle") }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t("admin.settings.openaiExperimentalScheduler.weightsDescription")
                    }}
                  </p>
                </div>

                <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-5">
                  <label
                    v-for="field in openAIAdvancedSchedulerWeightFields"
                    :key="field.key"
                    class="block"
                  >
                    <span class="text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]">
                      {{ field.label }}
                    </span>
                    <input
                      v-model="form[field.key]"
                      class="field-control mt-1"
                      inputmode="decimal"
                      :placeholder="field.placeholder"
                      type="text"
                    />
                  </label>
                </div>
              </div>
            </div>
          </div>

          <!-- Gateway Forwarding Behavior -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.gatewayForwarding.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.gatewayForwarding.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="grid gap-5 border-b border-[var(--border-subtle)] pb-5  md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
                <div>
                  <label
                    for="grok-default-text-model"
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.gatewayForwarding.grokDefaultTextModel") }}
                  </label>
                  <input
                    id="grok-default-text-model"
                    v-model.trim="form.grok_default_text_model"
                    type="text"
                    class="field-control mt-2 w-full"
                    list="grok-default-text-model-options"
                    data-testid="grok-default-text-model"
                    placeholder="grok-4.5"
                  />
                  <datalist id="grok-default-text-model-options">
                    <option value="grok-4.5" />
                    <option value="grok-4.1-fast" />
                    <option value="grok-4" />
                  </datalist>
                  <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.gatewayForwarding.grokDefaultTextModelHint") }}
                  </p>
                </div>
                <div class="flex items-center justify-between gap-5 md:min-w-72">
                  <div>
                    <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                      {{ t("admin.settings.gatewayForwarding.grokCrossClientMap") }}
                    </label>
                    <p class="mt-0.5 max-w-sm text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.gatewayForwarding.grokCrossClientMapHint") }}
                    </p>
                  </div>
                  <Toggle
                    v-model="form.grok_cross_client_model_map_enabled"
                    data-testid="grok-cross-client-model-map-toggle"
                  />
                </div>
                </div>
                <div class="md:col-span-2">
                  <label
                    for="grok-default-base-url-mode"
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.gatewayForwarding.grokDefaultBaseURLMode") }}
                  </label>
                  <BaseSelect
                    id="grok-default-base-url-mode"
                    v-model="form.grok_default_base_url_mode"
                    data-testid="grok-default-base-url-mode"
                    :options="[
                      {
                        value: 'cli',
                        label: t('admin.settings.gatewayForwarding.grokBaseURLModeCLI'),
                      },
                      {
                        value: 'api',
                        label: t('admin.settings.gatewayForwarding.grokBaseURLModeAPI'),
                      },
                      {
                        value: 'us-east-1',
                        label: t('admin.settings.gatewayForwarding.grokBaseURLModeUSEast1'),
                      },
                      {
                        value: 'us-west-2',
                        label: t('admin.settings.gatewayForwarding.grokBaseURLModeUSWest2'),
                      },
                      {
                        value: 'eu-west-1',
                        label: t('admin.settings.gatewayForwarding.grokBaseURLModeEUWest1'),
                      },
                    ]"
                    aria-label="Grok default base URL mode"
                  />
                  <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.gatewayForwarding.grokDefaultBaseURLModeHint") }}
                  </p>
                </div>

              <!-- Fingerprint Unification -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{
                      t(
                        "admin.settings.gatewayForwarding.fingerprintUnification",
                      )
                    }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t(
                        "admin.settings.gatewayForwarding.fingerprintUnificationHint",
                      )
                    }}
                  </p>
                </div>
                <Toggle v-model="form.enable_fingerprint_unification" />
              </div>

              <!-- Metadata Passthrough -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{
                      t("admin.settings.gatewayForwarding.metadataPassthrough")
                    }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t(
                        "admin.settings.gatewayForwarding.metadataPassthroughHint",
                      )
                    }}
                  </p>
                </div>
                <Toggle v-model="form.enable_metadata_passthrough" />
              </div>

              <!-- CCH Signing -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.gatewayForwarding.cchSigning") }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.gatewayForwarding.cchSigningHint") }}
                  </p>
                </div>
                <Toggle v-model="form.enable_cch_signing" />
              </div>

              <!-- Claude OAuth System Prompt Injection -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{
                      t(
                        "admin.settings.gatewayForwarding.claudeOAuthSystemPromptInjection",
                      )
                    }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t(
                        "admin.settings.gatewayForwarding.claudeOAuthSystemPromptInjectionHint",
                      )
                    }}
                  </p>
                </div>
                <Toggle
                  v-model="form.enable_claude_oauth_system_prompt_injection"
                />
              </div>

              <div>
                <label
                  class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                >
                  {{
                    t(
                      "admin.settings.gatewayForwarding.claudeOAuthSystemPromptBlocks",
                    )
                  }}
                </label>
                <div class="space-y-3">
                  <div
                    v-for="(block, index) in claudeOAuthSystemPromptBlocks"
                    :key="block.id"
                    class="rounded-[var(--r-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-4                   ">
                    <div
                      :class="[
                        'flex flex-wrap items-center justify-between gap-3',
                        block.expanded && 'mb-3',
                      ]"
                    >
                      <div class="min-w-0">
                        <div
                          class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] "
                        >
                          {{
                            t(
                              "admin.settings.gatewayForwarding.systemBlockTitle",
                              { index: index + 1 },
                            )
                          }}
                        </div>
                        <div
                          class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]"
                        >
                          {{ getClaudeOAuthPresetLabel(block.preset) }}
                        </div>
                      </div>
                      <div class="flex items-center gap-2">
                        <AppButton
                          variant="secondary"
                          size="sm"
                          type="button"
                          class="px-2"
                          :title="
                            block.expanded
                              ? t(
                                  'admin.settings.gatewayForwarding.systemBlockHide',
                                )
                              : t(
                                  'admin.settings.gatewayForwarding.systemBlockShow',
                                )
                          "
                          :aria-label="
                            block.expanded
                              ? t(
                                  'admin.settings.gatewayForwarding.systemBlockHide',
                                )
                              : t(
                                  'admin.settings.gatewayForwarding.systemBlockShow',
                                )
                          "
                          @click="toggleClaudeOAuthSystemPromptBlock(index)"
                        >
                          <Icon
                            :name="block.expanded ? 'eyeOff' : 'eye'"
                            size="xs"
                          />
                        </AppButton>
                        <AppButton
                          variant="secondary"
                          size="sm"
                          class="px-2"
                          type="button"
                          :disabled="index === 0"
                          @click="moveClaudeOAuthSystemPromptBlock(index, -1)"
                        >
                          <Icon name="arrowUp" size="xs" />
                        </AppButton>
                        <AppButton
                          variant="secondary"
                          size="sm"
                          class="px-2"
                          type="button"
                          :disabled="
                            index === claudeOAuthSystemPromptBlocks.length - 1
                          "
                          @click="moveClaudeOAuthSystemPromptBlock(index, 1)"
                        >
                          <Icon name="arrowDown" size="xs" />
                        </AppButton>
                        <Toggle v-model="block.enabled" />
                        <AppButton
                          variant="danger"
                          size="sm"
                          class="px-2"
                          type="button"
                          aria-label="Remove system prompt block"
                          @click="removeClaudeOAuthSystemPromptBlock(index)"
                        >
                          <Icon name="trash" size="xs" />
                        </AppButton>
                      </div>
                    </div>

                    <div v-show="block.expanded">
                      <div class="grid gap-3 md:grid-cols-2">
                        <div>
                          <label
                            class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                          >
                            {{
                              t(
                                "admin.settings.gatewayForwarding.systemBlockPreset",
                              )
                            }}
                          </label>
                          <BaseSelect
                            v-model="block.preset"
                            :options="claudeOAuthSystemPromptPresetOptions"
                            @change="
                              (value) =>
                                applyClaudeOAuthSystemPromptPreset(index, value)
                            "
                          />
                        </div>
                        <div>
                          <label
                            class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                          >
                            {{
                              t(
                                "admin.settings.gatewayForwarding.systemBlockType",
                              )
                            }}
                          </label>
                          <BaseSelect
                            v-model="block.type"
                            :options="claudeOAuthSystemPromptBlockTypeOptions"
                          />
                        </div>
                      </div>

                      <div class="mt-3">
                        <label
                          class="mb-1 block text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >
                          {{ t("admin.settings.gatewayForwarding.systemBlockText") }}
                        </label>
                        <textarea
                          v-model="block.text"
                          rows="6"
                          class="field-control w-full resize-y font-mono text-xs leading-5"
                          @input="markClaudeOAuthSystemPromptBlockCustom(block)"
                        />
                      </div>

                      <div
                        class="mt-3 grid gap-3 md:grid-cols-[minmax(0,1fr)_160px]"
                      >
                        <div class="flex items-center justify-between gap-4">
                          <div>
                            <label
                              class="text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                            >
                              {{
                                t(
                                  "admin.settings.gatewayForwarding.systemBlockCacheControl",
                                )
                              }}
                            </label>
                          </div>
                          <Toggle v-model="block.cacheControlEnabled" />
                        </div>
                        <div v-if="block.cacheControlEnabled">
                          <BaseSelect
                            v-model="block.cacheControlTTL"
                            :options="claudeOAuthSystemPromptCacheTTLOptions"
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <div class="mt-3 flex flex-wrap gap-2">
                  <AppButton
                    variant="secondary"
                    size="sm"
                    type="button"
                    @click="addClaudeOAuthSystemPromptBlock"
                  >
                    <Icon name="plus" size="xs" />
                    {{ t("admin.settings.gatewayForwarding.addSystemBlock") }}
                  </AppButton>
                  <AppButton
                    variant="secondary"
                    size="sm"
                    type="button"
                    @click="resetClaudeOAuthSystemPromptBlocks"
                  >
                    <Icon name="refresh" size="xs" />
                    {{
                      t("admin.settings.gatewayForwarding.resetSystemBlocks")
                    }}
                  </AppButton>
                </div>
                <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{
                    t(
                      "admin.settings.gatewayForwarding.claudeOAuthSystemPromptBlocksHint",
                    )
                  }}
                </p>
              </div>

              <!-- Anthropic Cache TTL 1h Injection -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{
                      t(
                        "admin.settings.gatewayForwarding.anthropicCacheTTL1hInjection",
                      )
                    }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t(
                        "admin.settings.gatewayForwarding.anthropicCacheTTL1hInjectionHint",
                      )
                    }}
                  </p>
                </div>
                <Toggle
                  v-model="form.enable_anthropic_cache_ttl_1h_injection"
                />
              </div>

              <!-- messages cache_control 改写 -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{
                      t(
                        "admin.settings.gatewayForwarding.rewriteMessageCacheControl",
                      )
                    }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t(
                        "admin.settings.gatewayForwarding.rewriteMessageCacheControlHint",
                      )
                    }}
                  </p>
                </div>
                <Toggle v-model="form.rewrite_message_cache_control" />
              </div>

              <!-- 客户端 dateline 归一化（仅 Anthropic OAuth/SetupToken） -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{
                      t(
                        "admin.settings.gatewayForwarding.clientDatelineNormalization",
                      )
                    }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t(
                        "admin.settings.gatewayForwarding.clientDatelineNormalizationHint",
                      )
                    }}
                  </p>
                </div>
                <Toggle
                  v-model="form.enable_client_dateline_normalization"
                />
              </div>

              <!-- Antigravity UA 版本 -->
              <div>
                <label
                  class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                >
                  {{
                    t(
                      "admin.settings.gatewayForwarding.antigravityUserAgentVersion",
                    )
                  }}
                </label>
                <input
                  v-model="form.antigravity_user_agent_version"
                  type="text"
                  class="field-control max-w-xs font-mono text-sm"
                  :placeholder="
                    t(
                      'admin.settings.gatewayForwarding.antigravityUserAgentVersionPlaceholder',
                    )
                  "
                />
                <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{
                    t(
                      "admin.settings.gatewayForwarding.antigravityUserAgentVersionHint",
                    )
                  }}
                </p>
              </div>

              <!-- OpenAI Codex UA -->
              <div>
                <label
                  class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                >
                  {{
                    t(
                      "admin.settings.gatewayForwarding.openaiCodexUserAgent",
                    )
                  }}
                </label>
                <input
                  v-model="form.openai_codex_user_agent"
                  type="text"
                  class="field-control w-full font-mono text-sm"
                  :placeholder="
                    t(
                      'admin.settings.gatewayForwarding.openaiCodexUserAgentPlaceholder',
                    )
                  "
                />
                <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{
                    t(
                      "admin.settings.gatewayForwarding.openaiCodexUserAgentHint",
                    )
                  }}
                </p>
              </div>

              <!-- Codex 客户端版本号 -->
              <div>
                <label
                  class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                >
                  {{
                    t(
                      "admin.settings.gatewayForwarding.openaiCodexClientVersion",
                    )
                  }}
                </label>
                <input
                  v-model="form.openai_codex_client_version"
                  type="text"
                  class="field-control w-full font-mono text-sm"
                  :placeholder="
                    t(
                      'admin.settings.gatewayForwarding.openaiCodexClientVersionPlaceholder',
                    )
                  "
                />
                <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{
                    t(
                      "admin.settings.gatewayForwarding.openaiCodexClientVersionHint",
                    )
                  }}
                </p>
              </div>

              <!-- Codex 版本号自动同步 -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{
                      t(
                        "admin.settings.gatewayForwarding.openaiCodexVersionAutoSync",
                      )
                    }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t(
                        "admin.settings.gatewayForwarding.openaiCodexVersionAutoSyncHint",
                      )
                    }}
                  </p>
                  <p
                    v-if="codexSyncedVersionLabel"
                    class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]"
                  >
                    {{ codexSyncedVersionLabel }}
                  </p>
                </div>
                <Toggle v-model="form.openai_codex_version_auto_sync_enabled" />
              </div>

            </div>
          </div>

          <!-- Web Search Emulation -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.webSearchEmulation.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.webSearchEmulation.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Global Toggle -->
              <div class="flex items-center justify-between">
                <div>
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.webSearchEmulation.enabled") }}
                  </label>
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.webSearchEmulation.enabledHint") }}
                  </p>
                </div>
                <Toggle v-model="webSearchConfig.enabled" />
              </div>

              <!-- Providers -->
              <div v-if="webSearchConfig.enabled" class="space-y-4">
                <div class="flex items-center justify-between">
                  <label
                    class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.webSearchEmulation.providers") }}
                  </label>
                  <AppButton
                    variant="secondary"
                    size="sm"
                    type="button"
                    @click="addWebSearchProvider"
                  >
                    {{ t("admin.settings.webSearchEmulation.addProvider") }}
                  </AppButton>
                </div>

                <div
                  v-if="webSearchConfig.providers.length === 0"
                  class="rounded-[var(--r-md)] border border-dashed border-[var(--border-subtle)] p-4 text-center text-sm text-[var(--muted-foreground)]                ">
                  {{ t("admin.settings.webSearchEmulation.noProviders") }}
                </div>

                <div
                  v-for="(provider, pIdx) in webSearchConfig.providers"
                  :key="pIdx"
                  class="rounded-[var(--r-md)] border border-[var(--border-subtle)]                ">
                  <!-- Collapsible header -->
                  <div
                    class="flex cursor-pointer items-center justify-between px-4 py-3"
                    @click="toggleProviderExpand(pIdx)"
                  >
                    <div class="flex items-center gap-3">
                      <svg
                        class="h-4 w-4 text-[var(--muted-foreground)] transition-transform"
                        :class="{ 'rotate-90': expandedProviders[pIdx] }"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M9 5l7 7-7 7"
                        />
                      </svg>
                      <BaseSelect
                        v-model="provider.type"
                        :options="[
                          { value: 'brave', label: 'Brave Search' },
                          { value: 'tavily', label: 'Tavily' },
                        ]"
                        class="w-36"
                        @click.stop
                      />
                      <!-- Quota summary (always visible) -->
                      <span class="text-xs text-[var(--muted-foreground)]">
                        {{ provider.quota_used ?? 0 }} /
                        {{
                          provider.quota_limit != null &&
                          provider.quota_limit > 0
                            ? provider.quota_limit
                            : "∞"
                        }}
                      </span>
                      <span
                        v-if="
                          !expandedProviders[pIdx] &&
                          provider.api_key_configured
                        "
                        class="text-xs text-[var(--success)]"
                      >
                        {{
                          t(
                            "admin.settings.webSearchEmulation.apiKeyConfigured",
                          )
                        }}
                      </span>
                    </div>
                    <AppButton
                      variant="danger"
                      size="xs"
                      type="button"
                      @click.stop="removeWebSearchProvider(pIdx)"
                    >
                      {{
                        t("admin.settings.webSearchEmulation.removeProvider")
                      }}
                    </AppButton>
                  </div>

                  <!-- Expanded content -->
                  <div
                    v-if="expandedProviders[pIdx]"
                    class="space-y-3 border-t border-[var(--border-subtle)] px-4 pb-4 pt-3                  ">
                    <!-- API Key with inline show/copy -->
                    <div>
                      <label class="text-xs text-[var(--muted-foreground)]">{{
                        t("admin.settings.webSearchEmulation.apiKey")
                      }}</label>
                      <div class="relative">
                        <input
                          v-model="provider.api_key"
                          :type="apiKeyVisible[pIdx] ? 'text' : 'password'"
                          class="field-control w-full text-sm"
                          :class="
                            provider.api_key || provider.api_key_configured
                              ? 'pr-16'
                              : ''
                          "
                          :placeholder="
                            provider.api_key_configured
                              ? '••••••••'
                              : t(
                                  'admin.settings.webSearchEmulation.apiKeyPlaceholder',
                                )
                          "
                        />
                        <div
                          v-if="provider.api_key || provider.api_key_configured"
                          class="absolute inset-y-0 right-0 flex items-center pr-1.5"
                        >
                          <AppButton
                            variant="ghost"
                            size="xs"
                            aria-label="Toggle API key visibility"
                            type="button"
                            :title="
                              apiKeyVisible[pIdx]
                                ? t(
                                    'admin.settings.webSearchEmulation.hideApiKey',
                                  )
                                : t(
                                    'admin.settings.webSearchEmulation.showApiKey',
                                  )
                            "
                            @click="apiKeyVisible[pIdx] = !apiKeyVisible[pIdx]"
                          >
                            <svg
                              v-if="!apiKeyVisible[pIdx]"
                              class="h-4 w-4"
                              fill="none"
                              viewBox="0 0 24 24"
                              stroke="currentColor"
                            >
                              <path
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                stroke-width="2"
                                d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                              />
                              <path
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                stroke-width="2"
                                d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"
                              />
                            </svg>
                            <svg
                              v-else
                              class="h-4 w-4"
                              fill="none"
                              viewBox="0 0 24 24"
                              stroke="currentColor"
                            >
                              <path
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                stroke-width="2"
                                d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21"
                              />
                            </svg>
                          </AppButton>
                          <AppButton
                            variant="ghost"
                            size="xs"
                            aria-label="Copy API key"
                            type="button"
                            :class="{
                              'opacity-30 cursor-not-allowed':
                                !provider.api_key,
                            }"
                            :title="
                              t('admin.settings.webSearchEmulation.copyApiKey')
                            "
                            :disabled="!provider.api_key"
                            @click="copyApiKey(pIdx)"
                          >
                            <svg
                              class="h-4 w-4"
                              fill="none"
                              viewBox="0 0 24 24"
                              stroke="currentColor"
                            >
                              <path
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                stroke-width="2"
                                d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                              />
                            </svg>
                          </AppButton>
                        </div>
                      </div>
                    </div>

                    <!-- Quota + Subscription in compact row -->
                    <div class="grid grid-cols-2 gap-3">
                      <div>
                        <label class="text-xs text-[var(--muted-foreground)]">{{
                          t("admin.settings.webSearchEmulation.quotaLimit")
                        }}</label>
                        <input
                          v-model="provider.quota_limit"
                          type="number"
                          min="1"
                          class="field-control text-sm"
                          :placeholder="'∞'"
                        />
                        <p class="mt-0.5 text-xs text-[var(--muted-foreground)]">
                          {{
                            t(
                              "admin.settings.webSearchEmulation.quotaLimitHint",
                            )
                          }}
                        </p>
                      </div>
                      <div>
                        <label class="text-xs text-[var(--muted-foreground)]">{{
                          t("admin.settings.webSearchEmulation.subscribedAt")
                        }}</label>
                        <input
                          :value="formatSubscribedAt(provider.subscribed_at)"
                          type="date"
                          class="field-control text-sm"
                          @input="
                            provider.subscribed_at = parseSubscribedAt(
                              ($event.target as HTMLInputElement).value,
                            )
                          "
                        />
                        <p class="mt-0.5 text-xs text-[var(--muted-foreground)]">
                          {{
                            t(
                              "admin.settings.webSearchEmulation.subscribedAtHint",
                            )
                          }}
                        </p>
                      </div>
                    </div>

                    <!-- Usage display -->
                    <div class="flex items-center gap-2">
                      <span class="text-xs text-[var(--muted-foreground)]"
                        >{{
                          t("admin.settings.webSearchEmulation.quotaUsage")
                        }}:</span
                      >
                      <div
                        v-if="
                          provider.quota_limit != null &&
                          provider.quota_limit > 0
                        "
                        class="flex-1 rounded-full bg-[var(--surface-subtle)]"
                        style="height: 6px"
                      >
                        <div
                          class="h-full rounded-full transition-[background-color,color,opacity,transform]"
                          :class="
                            quotaPercentage(provider) > 90
                              ? 'bg-[var(--destructive)]'
                              : quotaPercentage(provider) > 70
                                ? 'bg-yellow-500'
                                : 'bg-[var(--success)]'
                          "
                          :style="{
                            width:
                              Math.min(quotaPercentage(provider), 100) + '%',
                          }"
                        />
                      </div>
                      <div v-else class="flex-1" />
                      <span class="text-xs text-[var(--muted-foreground)]"
                        >{{ provider.quota_used ?? 0 }} /
                        {{
                          provider.quota_limit != null &&
                          provider.quota_limit > 0
                            ? provider.quota_limit
                            : "∞"
                        }}</span
                      >
                      <AppButton
                        variant="ghost"
                        size="xs"
                        v-if="(provider.quota_used ?? 0) > 0"
                        type="button"
                        @click="resetWebSearchUsage(pIdx)"
                      >
                        {{ t("admin.settings.webSearchEmulation.resetUsage") }}
                      </AppButton>
                    </div>

                    <!-- Proxy + Test on same row -->
                    <div class="flex items-end gap-3">
                      <div class="flex-1">
                        <label class="text-xs text-[var(--muted-foreground)]">{{
                          t("admin.settings.webSearchEmulation.proxy")
                        }}</label>
                        <ProxySelector
                          v-model="provider.proxy_id"
                          :proxies="webSearchProxies"
                        />
                      </div>
                      <AppButton
                        variant="secondary"
                        size="sm"
                        type="button"
                        class="whitespace-nowrap"
                        @click="openTestDialog()"
                      >
                        {{ t("admin.settings.webSearchEmulation.test") }}
                      </AppButton>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Web Search Test Dialog -->
          <BaseDialog
            v-if="wsTestDialogOpen"
            :show="wsTestDialogOpen"
            :title="t('admin.settings.webSearchEmulation.testResultTitle')"
            width="wide"
            @close="wsTestDialogOpen = false"
          >
              <div class="flex items-center gap-2">
                <input
                  v-model="wsTestQuery"
                  type="text"
                  class="field-control flex-1 text-sm"
                  :placeholder="
                    t('admin.settings.webSearchEmulation.testDefaultQuery')
                  "
                  @keyup.enter="testWebSearchProvider()"
                />
                <AppButton
                  variant="solid"
                  size="sm"
                  type="button"
                  :disabled="wsTestLoading"
                  :loading="wsTestLoading"
                  @click="testWebSearchProvider()"
                >
                  {{
                    wsTestLoading
                      ? t("admin.settings.webSearchEmulation.testing")
                      : t("admin.settings.webSearchEmulation.test")
                  }}
                </AppButton>
              </div>
              <!-- Test results -->
              <div
                v-if="wsTestResult"
                class="mt-4 max-h-80 overflow-y-auto rounded-[var(--r-md)] bg-[var(--surface-subtle)] p-4              ">
                <p
                  class="mb-2 text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                >
                  {{
                    t("admin.settings.webSearchEmulation.testResultProvider")
                  }}: {{ wsTestResult.provider }}
                </p>
                <div
                  v-if="wsTestResult.results.length === 0"
                  class="text-sm text-[var(--muted-foreground)]"
                >
                  {{ t("admin.settings.webSearchEmulation.testNoResults") }}
                </div>
                <div
                  v-for="(r, rIdx) in wsTestResult.results"
                  :key="rIdx"
                  class="mt-2 border-t border-[var(--border-subtle)] pt-2 first:mt-0 first:border-0 first:pt-0                ">
                  <a
                    :href="r.url"
                    target="_blank"
                    class="text-sm font-[var(--fw-medium)] text-[var(--brand)] hover:underline "
                    >{{ r.title }}</a
                  >
                  <p class="mt-0.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ r.snippet }}
                  </p>
                </div>
              </div>
              <template #footer>
                <AppButton
                  variant="secondary"
                  size="sm"
                  type="button"
                  @click="wsTestDialogOpen = false"
                >
                  {{ t("common.close") }}
                </AppButton>
              </template>
          </BaseDialog>

        <!-- Usage Records Settings -->
        <div class="settings-surface">
          <div class="border-b border-[var(--border-subtle)] px-6 py-4            ">
<h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
              {{ t('admin.settings.usageRecords.title') }}
            </h2>
            <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
              {{ t('admin.settings.usageRecords.description') }}
            </p>
          </div>
          <div class="space-y-4 p-6">
            <!-- User error requests visibility -->
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                  {{ t('admin.settings.user_error_view.label') }}
                </label>
                <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t('admin.settings.user_error_view.description') }}
                </p>
              </div>
              <Toggle v-model="form.allow_user_view_error_requests" />
            </div>
          </div>
        </div>
        </div>
        <!-- /Tab: Gateway — Claude Code, Scheduling -->
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import AppButton from '@/components/common/Button.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Checkbox from '@/components/common/Checkbox.vue'
import OpenAIFastPolicyUserSelector from '@/views/admin/settings/OpenAIFastPolicyUserSelector.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import BaseSelect from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useSettingsPageBindings } from './settingsPageBindings'

export default defineComponent({
  name: 'AdminGatewaySettingsPage',
  components: {
    BaseSelect,
    BaseDialog,
    AppButton,
    Checkbox,
    Icon,
    OpenAIFastPolicyUserSelector,
    ProxySelector,
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
