<template>
        <!-- Tab: Security — Registration, Turnstile, LinuxDo -->
        <div class="space-y-6">
          <!-- Registration Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.registration.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.registration.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Enable Registration -->
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.registration.enableRegistration")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      t("admin.settings.registration.enableRegistrationHint")
                    }}
                  </p>
                </div>
                <Toggle v-model="form.registration_enabled" />
              </div>

              <!-- Email Verification -->
              <div
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.registration.emailVerification")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.registration.emailVerificationHint") }}
                  </p>
                </div>
                <Toggle v-model="form.email_verify_enabled" />
              </div>

              <!-- Email Suffix Whitelist -->
              <div class="border-t border-[var(--border-subtle)] pt-4                ">
<label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                  t("admin.settings.registration.emailSuffixWhitelist")
                }}</label>
                <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{
                    t("admin.settings.registration.emailSuffixWhitelistHint")
                  }}
                </p>
                <div
                  class="mt-3 rounded-[var(--r-md)] border border-[var(--border-subtle)] bg-[var(--card)] p-2                 ">
                  <div class="flex flex-wrap items-center gap-2">
                    <span
                      v-for="suffix in registrationEmailSuffixWhitelistTags"
                      :key="suffix"
                      class="inline-flex items-center gap-1 rounded bg-[var(--surface-subtle)] px-2 py-1 text-xs font-mono text-[var(--body-copy)]  text-[var(--body-copy)]"
                    >
                      <span>{{ suffix }}</span>
                      <AppButton
                        variant="ghost"
                        size="xs"
                        aria-label="Remove email suffix"
                        type="button"
                        @click="
                          removeRegistrationEmailSuffixWhitelistTag(suffix)
                        "
                      >
                        <Icon
                          name="x"
                          size="xs"
                          class="h-3.5 w-3.5"
                          :stroke-width="2"
                        />
                      </AppButton>
                    </span>

                    <div
                      class="flex min-w-[220px] flex-1 items-center gap-1 rounded border border-transparent px-2 py-1 focus-within:border-[var(--brand-line)] "
                    >
                      <input
                        v-model="registrationEmailSuffixWhitelistDraft"
                        type="text"
                        class="w-full bg-transparent text-sm font-mono text-[var(--foreground)] outline-none placeholder:text-[var(--muted-foreground)]  "
                        :placeholder="
                          t(
                            'admin.settings.registration.emailSuffixWhitelistPlaceholder',
                          )
                        "
                        @input="
                          handleRegistrationEmailSuffixWhitelistDraftInput
                        "
                        @keydown="
                          handleRegistrationEmailSuffixWhitelistDraftKeydown
                        "
                        @blur="commitRegistrationEmailSuffixWhitelistDraft"
                        @paste="handleRegistrationEmailSuffixWhitelistPaste"
                      />
                    </div>
                  </div>
                </div>
                <p class="mt-2 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{
                    t(
                      "admin.settings.registration.emailSuffixWhitelistInputHint",
                    )
                  }}
                </p>
              </div>

              <!-- Email Domain Quota -->
              <div
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.registration.emailDomainQuota")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.registration.emailDomainQuotaHint") }}
                  </p>
                </div>
                <Toggle
                  v-model="form.registration_email_domain_quota_enabled"
                />
              </div>

              <!-- Promo Code -->
              <div
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.registration.promoCode")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.registration.promoCodeHint") }}
                  </p>
                </div>
                <Toggle v-model="form.promo_code_enabled" />
              </div>

              <!-- Invitation Code -->
              <div
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.registration.invitationCode")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.registration.invitationCodeHint") }}
                  </p>
                </div>
                <Toggle v-model="form.invitation_code_enabled" />
              </div>
              <!-- Password Reset - Only show when email verification is enabled -->
              <div
                v-if="form.email_verify_enabled"
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.registration.passwordReset")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.registration.passwordResetHint") }}
                  </p>
                </div>
                <Toggle v-model="form.password_reset_enabled" />
              </div>
              <!-- Frontend URL - Only show when password reset is enabled -->
              <div
                v-if="form.email_verify_enabled && form.password_reset_enabled"
                class="border-t border-[var(--border-subtle)] pt-4              ">
                <label
                  class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                >
                  {{ t("admin.settings.registration.frontendUrl") }}
                </label>
                <input
                  v-model="form.frontend_url"
                  type="url"
                  class="field-control"
                  :placeholder="
                    t('admin.settings.registration.frontendUrlPlaceholder')
                  "
                />
                <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t("admin.settings.registration.frontendUrlHint") }}
                </p>
              </div>

              <!-- TOTP 2FA -->
              <div
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.registration.totp")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.registration.totpHint") }}
                  </p>
                  <!-- Warning when encryption key not configured -->
                  <p
                    v-if="!form.totp_encryption_key_configured"
                    class="mt-2 text-sm text-[var(--s2a-attn)] "
                  >
                    {{ t("admin.settings.registration.totpKeyNotConfigured") }}
                  </p>
                </div>
                <Toggle
                  v-model="form.totp_enabled"
                  :disabled="!form.totp_encryption_key_configured"
                />
              </div>

              <!-- Passkey sign-in -->
              <div
                class="border-t border-[var(--border-subtle)] pt-4"
                data-testid="passkey-settings"
              >
                <div class="flex items-start justify-between gap-4">
                  <div>
                    <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                      t("admin.settings.security.passkey")
                    }}</label>
                    <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.security.passkeyHint") }}
                    </p>
                  </div>
                  <Toggle
                    v-model="form.passkey_enabled"
                    data-testid="passkey-toggle"
                    :disabled="!form.passkey_configured"
                  />
                </div>
                <div
                  class="mt-3 rounded-[var(--r-md)] border px-3 py-2 text-sm"
                  :class="
                    form.passkey_configured
                      ? 'border-[var(--success)] bg-[var(--brand-tint)] text-[var(--success)]   '
                      : 'border-[var(--s2a-attn)] bg-[var(--s2a-attn-soft)] text-[var(--s2a-attn)]   '
                  "
                  data-testid="passkey-config-status"
                >
                  <p class="font-[var(--fw-medium)]">
                    {{
                      form.passkey_configured
                        ? t("admin.settings.security.passkeyConfigured")
                        : t("admin.settings.security.passkeyNotConfigured")
                    }}
                  </p>
                  <p class="mt-1 break-all">
                    {{ t("admin.settings.security.passkeyRPID") }}:
                    {{
                      form.passkey_rp_id ||
                      t("admin.settings.security.passkeyValueNotConfigured")
                    }}
                  </p>
                  <p class="mt-1 break-all">
                    {{ t("admin.settings.security.passkeyOrigins") }}:
                    {{
                      form.passkey_rp_origins.length > 0
                        ? form.passkey_rp_origins.join(", ")
                        : t(
                            "admin.settings.security.passkeyValueNotConfigured",
                          )
                    }}
                  </p>
                  <p v-if="!form.passkey_configured" class="mt-2">
                    {{ t("admin.settings.security.passkeyDeploymentHint") }}
                  </p>
                </div>
              </div>

              <!-- 敏感操作 step-up 2FA -->
              <div
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.security.stepUp")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.security.stepUpHint") }}
                  </p>
                </div>
                <Toggle v-model="form.step_up_enabled" />
              </div>

              <!-- 会话 IP/UA 绑定 -->
              <div
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.security.sessionBinding")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.security.sessionBindingHint") }}
                  </p>
                </div>
                <Toggle v-model="form.session_binding_enabled" />
              </div>

              <!-- 审计日志保留天数 -->
              <div
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.security.auditRetention")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.security.auditRetentionHint") }}
                  </p>
                </div>
                <input
                  v-model.number="form.audit_log_retention_days"
                  type="number"
                  min="0"
                  class="field-control w-28 text-right"
                />
              </div>
            </div>
          </div>

          <!-- API Key IP ACL Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.apiKeyAcl.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.apiKeyAcl.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                    {{ t("admin.settings.apiKeyAcl.trustForwardedIp") }}
                  </label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.apiKeyAcl.trustForwardedIpHint") }}
                  </p>
                </div>
                <Toggle v-model="form.api_key_acl_trust_forwarded_ip" />
              </div>

              <div
                v-if="form.api_key_acl_trust_forwarded_ip"
                class="border-t border-[var(--border-subtle)] pt-4              ">
                <label
                  for="forwarded-client-ip-headers"
                  class="font-[var(--fw-medium)] text-[var(--foreground)] "
                >
                  {{ t("admin.settings.apiKeyAcl.forwardedClientIpHeaders") }}
                </label>
                <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t("admin.settings.apiKeyAcl.forwardedClientIpHeadersHint") }}
                </p>
                <div
                  class="mt-3 rounded-[var(--r-md)] border border-[var(--border-subtle)] bg-[var(--card)] p-2                 ">
                  <div class="flex flex-wrap items-center gap-2">
                    <span
                      v-for="header in form.forwarded_client_ip_headers"
                      :key="header"
                      data-testid="forwarded-client-ip-header-tag"
                      class="inline-flex items-center gap-1 rounded bg-[var(--surface-subtle)] px-2 py-1 text-xs font-mono text-[var(--body-copy)]  text-[var(--body-copy)]"
                    >
                      <span>{{ header }}</span>
                      <AppButton
                        variant="ghost"
                        size="xs"
                        type="button"
                        :aria-label="t('admin.settings.apiKeyAcl.removeForwardedClientIpHeader', { header })"
                        @click="removeForwardedClientIpHeader(header)"
                      >
                        <Icon
                          name="x"
                          size="xs"
                          class="h-3.5 w-3.5"
                          :stroke-width="2"
                        />
                      </AppButton>
                    </span>
                    <div
                      class="flex min-w-[220px] flex-1 items-center gap-1 rounded border border-transparent px-2 py-1 focus-within:border-[var(--brand-line)] "
                    >
                      <input
                        id="forwarded-client-ip-headers"
                        v-model="forwardedClientIpHeaderDraft"
                        data-testid="forwarded-client-ip-headers-input"
                        type="text"
                        class="w-full bg-transparent text-sm font-mono text-[var(--foreground)] outline-none placeholder:text-[var(--muted-foreground)]  "
                        :placeholder="t('admin.settings.apiKeyAcl.forwardedClientIpHeadersPlaceholder')"
                        @keydown="handleForwardedClientIpHeaderKeydown"
                        @blur="commitForwardedClientIpHeaderDraft"
                        @paste="handleForwardedClientIpHeaderPaste"
                      />
                    </div>
                  </div>
                </div>
                <p class="mt-2 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t("admin.settings.apiKeyAcl.forwardedClientIpHeadersRiskHint") }}
                </p>
              </div>
            </div>
          </div>

          <!-- Panel API Rate Limit Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <div class="flex items-center gap-2">
                <Icon
                  name="shield"
                  size="md"
                  class="text-[var(--brand)]"
                />
                <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                  {{ t("admin.settings.panelRateLimit.title") }}
                </h2>
              </div>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.panelRateLimit.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div
                v-if="panelRateLimitLoading"
                class="flex items-center gap-2 text-[var(--muted-foreground)]"
              >
                <div
                  class="h-4 w-4 animate-spin rounded-full border-b-2 border-[var(--brand-line)]"
                ></div>
                {{ t("common.loading") }}
              </div>

              <template v-else>
                <!-- 计数维度说明：按账号计数，反代部署无误伤 -->
                <div
                  class="rounded-[var(--r-md)] border border-sky-200 bg-sky-50 p-4                 ">
                  <div class="flex items-start">
                    <Icon
                      name="infoCircle"
                      size="md"
                      class="mt-0.5 flex-shrink-0 text-[var(--brand)]"
                    />
                    <p class="ml-3 text-sm text-[var(--brand)] ">
                      {{ t("admin.settings.panelRateLimit.proxySafeNote") }}
                    </p>
                  </div>
                </div>

                <div class="flex items-center justify-between">
                  <div>
                    <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                      t("admin.settings.panelRateLimit.enabled")
                    }}</label>
                    <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.panelRateLimit.enabledHint") }}
                    </p>
                  </div>
                  <Toggle v-model="panelRateLimitForm.enabled" />
                </div>

                <div
                  v-if="panelRateLimitForm.enabled"
                  class="space-y-5 border-t border-[var(--border-subtle)] pt-4                ">
                  <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
                    <div>
                      <label
                        class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                      >
                        {{ t("admin.settings.panelRateLimit.userRpm") }}
                      </label>
                      <div class="flex items-center gap-2">
                        <input
                          v-model.number="panelRateLimitForm.user_rpm"
                          data-testid="panel-rate-limit-user-rpm"
                          type="number"
                          min="0"
                          max="100000"
                          class="field-control w-32"
                        />
                        <span class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{ t("admin.settings.panelRateLimit.perMinute") }}
                        </span>
                      </div>
                      <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.panelRateLimit.userRpmHint") }}
                      </p>
                    </div>

                    <div>
                      <label
                        class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                      >
                        {{ t("admin.settings.panelRateLimit.heavyRpm") }}
                      </label>
                      <div class="flex items-center gap-2">
                        <input
                          v-model.number="panelRateLimitForm.heavy_rpm"
                          type="number"
                          min="0"
                          max="100000"
                          class="field-control w-32"
                        />
                        <span class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{ t("admin.settings.panelRateLimit.perMinute") }}
                        </span>
                      </div>
                      <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.panelRateLimit.heavyRpmHint") }}
                      </p>
                    </div>

                    <div>
                      <label
                        class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                      >
                        {{ t("admin.settings.panelRateLimit.publicIpRpm") }}
                      </label>
                      <div class="flex items-center gap-2">
                        <input
                          v-model.number="panelRateLimitForm.public_ip_rpm"
                          type="number"
                          min="0"
                          max="100000"
                          class="field-control w-32"
                        />
                        <span class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{ t("admin.settings.panelRateLimit.perMinute") }}
                        </span>
                      </div>
                      <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.panelRateLimit.publicIpRpmHint") }}
                      </p>
                    </div>
                  </div>

                  <div
                    class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4                  ">
                    <div>
                      <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                        t("admin.settings.panelRateLimit.exemptAdmin")
                      }}</label>
                      <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.panelRateLimit.exemptAdminHint") }}
                      </p>
                    </div>
                    <Toggle v-model="panelRateLimitForm.exempt_admin" />
                  </div>
                </div>

                <div
                  class="flex justify-end border-t border-[var(--border-subtle)] pt-4                ">
                  <AppButton
                    variant="solid"
                    size="sm"
                    type="button"
                    data-testid="panel-rate-limit-save"
                    @click="savePanelRateLimitSettings"
                    :disabled="panelRateLimitSaving"
                    :loading="panelRateLimitSaving"
                  >
                    {{
                      panelRateLimitSaving
                        ? t("common.saving")
                        : t("common.save")
                    }}
                  </AppButton>
                </div>
              </template>
            </div>
          </div>

          <!-- 人机验证 Settings -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.captcha.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.captcha.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <!-- Enable Captcha -->
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.captcha.enable")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.captcha.enableHint") }}
                  </p>
                </div>
                <Toggle
                  v-model="captchaMasterEnabled"
                  data-testid="captcha-enabled-toggle"
                />
              </div>

              <!-- Provider fields - Only show when enabled -->
              <div
                v-if="captchaMasterEnabled"
                class="border-t border-[var(--border-subtle)] pt-4              ">
                <!-- Provider Selector -->
                <div class="mb-6">
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.captcha.provider") }}
                  </label>
                  <Segmented
                    :model-value="captchaProviderSelection"
                    :items="[
                      {
                        value: 'turnstile',
                        label: t('admin.settings.captcha.providerTurnstile'),
                      },
                      {
                        value: 'tencent',
                        label: t('admin.settings.captcha.providerTencent'),
                      },
                      {
                        value: 'aliyun',
                        label: t('admin.settings.captcha.providerAliyun'),
                      },
                    ]"
                    aria-label="Captcha provider"
                    @update:model-value="selectCaptchaProvider"
                  />
                </div>

                <!-- Cloudflare Turnstile fields -->
                <div
                  v-if="captchaProviderSelection === 'turnstile'"
                  class="grid grid-cols-1 gap-6"
                >
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.turnstile.siteKey") }}
                    </label>
                    <input
                      v-model="form.turnstile_site_key"
                      type="text"
                      class="field-control font-mono text-sm"
                      placeholder="0x4AAAAAAA..."
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.turnstile.siteKeyHint") }}
                      <a
                        href="https://dash.cloudflare.com/"
                        target="_blank"
                        class="text-[var(--brand)] hover:text-[var(--brand)]"
                        >{{
                          t("admin.settings.turnstile.cloudflareDashboard")
                        }}</a
                      >
                    </p>
                  </div>
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.turnstile.secretKey") }}
                    </label>
                    <input
                      v-model="form.turnstile_secret_key"
                      type="password"
                      class="field-control font-mono text-sm"
                      placeholder="0x4AAAAAAA..."
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        form.turnstile_secret_key_configured
                          ? t(
                              "admin.settings.turnstile.secretKeyConfiguredHint",
                            )
                          : t("admin.settings.turnstile.secretKeyHint")
                      }}
                    </p>
                  </div>
                </div>

                <!-- Tencent Captcha fields -->
                <div v-else-if="captchaProviderSelection === 'tencent'">
                  <div class="mb-6 max-w-sm">
                    <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                      {{ t("admin.settings.tencentCaptcha.region") }}
                    </label>
                    <Segmented
                      v-model="form.tencent_captcha_region"
                      :items="[
                        { value: 'cn', label: t('admin.settings.tencentCaptcha.regionCn') },
                        { value: 'intl', label: t('admin.settings.tencentCaptcha.regionIntl') },
                      ]"
                      aria-label="Tencent captcha region"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.tencentCaptcha.regionHint") }}
                    </p>
                  </div>
                  <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
                    <div class="md:col-span-2">
                      <h3 class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] ">
                        {{ t("admin.settings.tencentCaptcha.appCredentialsTitle") }}
                      </h3>
                      <p class="mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.tencentCaptcha.appCredentialsHint") }}
                      </p>
                    </div>
                    <div>
                      <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                        {{ t("admin.settings.tencentCaptcha.appId") }}
                      </label>
                      <input
                        v-model="form.tencent_captcha_app_id"
                        type="text"
                        inputmode="numeric"
                        class="field-control font-mono text-sm"
                        placeholder="123456789"
                      />
                    </div>
                    <div>
                      <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                        {{ t("admin.settings.tencentCaptcha.appSecretKey") }}
                      </label>
                      <input
                        v-model="form.tencent_captcha_app_secret_key"
                        type="password"
                        autocomplete="new-password"
                        class="field-control font-mono text-sm"
                        :placeholder="t('admin.settings.tencentCaptcha.keepExisting')"
                      />
                      <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ form.tencent_captcha_app_secret_key_configured ? t("admin.settings.tencentCaptcha.configured") : t("admin.settings.tencentCaptcha.required") }}
                      </p>
                    </div>
                    <div class="border-t border-[var(--border-subtle)] pt-5 md:col-span-2                      ">
<h3 class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] ">
                        {{ t("admin.settings.tencentCaptcha.cloudCredentialsTitle") }}
                      </h3>
                      <p class="mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.tencentCaptcha.cloudCredentialsHint") }}
                      </p>
                    </div>
                    <div>
                      <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                        {{ t("admin.settings.tencentCaptcha.cloudSecretId") }}
                      </label>
                      <input
                        v-model="form.tencent_captcha_cloud_secret_id"
                        type="password"
                        autocomplete="new-password"
                        class="field-control font-mono text-sm"
                        :placeholder="t('admin.settings.tencentCaptcha.keepExisting')"
                      />
                      <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ form.tencent_captcha_cloud_secret_id_configured ? t("admin.settings.tencentCaptcha.configured") : t("admin.settings.tencentCaptcha.required") }}
                      </p>
                    </div>
                    <div>
                      <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                        {{ t("admin.settings.tencentCaptcha.cloudSecretKey") }}
                      </label>
                      <input
                        v-model="form.tencent_captcha_cloud_secret_key"
                        type="password"
                        autocomplete="new-password"
                        class="field-control font-mono text-sm"
                        :placeholder="t('admin.settings.tencentCaptcha.keepExisting')"
                      />
                      <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ form.tencent_captcha_cloud_secret_key_configured ? t("admin.settings.tencentCaptcha.configured") : t("admin.settings.tencentCaptcha.required") }}
                      </p>
                    </div>
                  </div>
                  <p class="mt-5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.tencentCaptcha.camPermissionHint") }}
                  </p>
                  <p class="mt-2 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.tencentCaptcha.aidEncryptedHint") }}
                  </p>
                  <div class="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-sm">
                    <a
                      :href="tencentCaptchaLinks.console"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="text-[var(--brand)] hover:text-[var(--brand)]"
                    >
                      {{ t("admin.settings.tencentCaptcha.openCaptchaConsole") }}
                    </a>
                    <a
                      :href="tencentCaptchaLinks.cloudKeys"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="text-[var(--brand)] hover:text-[var(--brand)]"
                    >
                      {{ t("admin.settings.tencentCaptcha.createCloudKeys") }}
                    </a>
                    <a
                      :href="tencentCaptchaLinks.webDocs"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="text-[var(--brand)] hover:text-[var(--brand)]"
                    >
                      {{ t("admin.settings.tencentCaptcha.openWebDocs") }}
                    </a>
                  </div>
                </div>

                <!-- Aliyun Captcha 2.0 fields -->
                <div v-else class="grid grid-cols-1 gap-6">
                  <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
                    <div>
                      <label
                        class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                      >
                        {{ t("admin.settings.aliyunCaptcha.region") }}
                      </label>
                      <Segmented
                        v-model="form.aliyun_captcha_region"
                        :items="[
                          { value: 'cn', label: t('admin.settings.aliyunCaptcha.regionCn') },
                          { value: 'sgp', label: t('admin.settings.aliyunCaptcha.regionSgp') },
                        ]"
                        aria-label="Aliyun captcha region"
                      />
                      <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.aliyunCaptcha.regionHint") }}
                      </p>
                    </div>
                    <div>
                      <label
                        class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                      >
                        {{ t("admin.settings.aliyunCaptcha.prefix") }}
                      </label>
                      <input
                        v-model="form.aliyun_captcha_prefix"
                        type="text"
                        class="field-control font-mono text-sm"
                        placeholder="14xxxxx"
                      />
                      <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.aliyunCaptcha.prefixHint") }}
                      </p>
                    </div>
                  </div>
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.aliyunCaptcha.sceneId") }}
                    </label>
                    <input
                      v-model="form.aliyun_captcha_scene_id"
                      type="text"
                      class="field-control font-mono text-sm"
                      placeholder="1cxxxxxx"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.aliyunCaptcha.sceneIdHint") }}
                    </p>
                  </div>
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.aliyunCaptcha.accessKeyId") }}
                    </label>
                    <input
                      v-model="form.aliyun_captcha_access_key_id"
                      type="text"
                      class="field-control font-mono text-sm"
                      placeholder="LTAI..."
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.aliyunCaptcha.accessKeyIdHint") }}
                    </p>
                  </div>
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.aliyunCaptcha.accessKeySecret") }}
                    </label>
                    <input
                      v-model="form.aliyun_captcha_access_key_secret"
                      type="password"
                      autocomplete="new-password"
                      class="field-control font-mono text-sm"
                      placeholder="••••••••"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        form.aliyun_captcha_access_key_secret_configured
                          ? t(
                              "admin.settings.aliyunCaptcha.accessKeySecretConfiguredHint",
                            )
                          : t("admin.settings.aliyunCaptcha.accessKeySecretHint")
                      }}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- LinuxDo Connect OAuth 登录 -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.linuxdo.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.linuxdo.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.linuxdo.enable")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.linuxdo.enableHint") }}
                  </p>
                </div>
                <Toggle v-model="form.linuxdo_connect_enabled" />
              </div>

              <div
                v-if="form.linuxdo_connect_enabled"
                class="border-t border-[var(--border-subtle)] pt-4              ">
                <div class="grid grid-cols-1 gap-6">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.linuxdo.clientId") }}
                    </label>
                    <input
                      v-model="form.linuxdo_connect_client_id"
                      type="text"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.linuxdo.clientIdPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.linuxdo.clientIdHint") }}
                    </p>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.linuxdo.clientSecret") }}
                    </label>
                    <input
                      v-model="form.linuxdo_connect_client_secret"
                      type="password"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        form.linuxdo_connect_client_secret_configured
                          ? t(
                              'admin.settings.linuxdo.clientSecretConfiguredPlaceholder',
                            )
                          : t('admin.settings.linuxdo.clientSecretPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        form.linuxdo_connect_client_secret_configured
                          ? t(
                              "admin.settings.linuxdo.clientSecretConfiguredHint",
                            )
                          : t("admin.settings.linuxdo.clientSecretHint")
                      }}
                    </p>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.linuxdo.redirectUrl") }}
                    </label>
                    <input
                      v-model="form.linuxdo_connect_redirect_url"
                      type="url"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.linuxdo.redirectUrlPlaceholder')
                      "
                    />
                    <div
                      class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3"
                    >
                      <AppButton
                        variant="secondary"
                        size="sm"
                        type="button"
                        @click="setAndCopyLinuxdoRedirectUrl"
                      >
                        {{ t("admin.settings.linuxdo.quickSetCopy") }}
                      </AppButton>
                      <code
                        v-if="linuxdoRedirectUrlSuggestion"
                        class="select-all break-all rounded bg-[var(--surface-subtle)] px-2 py-1 font-mono text-xs text-[var(--body-copy)]  text-[var(--body-copy)]"
                      >
                        {{ linuxdoRedirectUrlSuggestion }}
                      </code>
                    </div>
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.linuxdo.redirectUrlHint") }}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- GitHub / Google 邮箱快捷登录 -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ localText("邮箱快捷登录", "Email OAuth Sign-in") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{
                  localText(
                    "开启 GitHub 或 Google 邮箱授权登录后，系统会读取已验证邮箱，存在则直接登录，不存在则自动注册。",
                    "After GitHub or Google email OAuth is enabled, the system reads a verified email, signs in matching users, and auto-registers missing users.",
                  )
                }}
              </p>
            </div>
            <div class="space-y-6 p-6">
              <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
                <div class="rounded-[var(--r-md)] border border-[var(--border-subtle)] p-4                  ">
<div class="flex items-start justify-between gap-4">
                    <div>
                      <h3 class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                        GitHub
                      </h3>
                      <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{
                          localText(
                            "GitHub OAuth App 需要 read:user user:email 权限，回调地址填写下方后端地址。",
                            "GitHub OAuth App needs read:user user:email scopes. Use the backend callback URL below.",
                          )
                        }}
                      </p>
                    </div>
                    <Toggle v-model="form.github_oauth_enabled" />
                  </div>

                  <div v-if="form.github_oauth_enabled" class="mt-4 space-y-4">
                    <div class="rounded-[var(--r-md)] bg-[var(--surface-subtle)] px-3 py-2 text-xs text-[var(--body-copy)]  text-[var(--body-copy)]">
                      <template v-if="isZhLocale">
                        开通引导：GitHub Settings → Developer settings →
                        <a
                          data-testid="github-oauth-apps-guide-link"
                          href="https://github.com/settings/developers"
                          target="_blank"
                          rel="noopener noreferrer"
                          class="font-[var(--fw-medium)] text-[var(--brand)] hover:underline "
                        >OAuth Apps</a>
                        → New OAuth App；Homepage URL 填站点域名，Authorization callback URL 填下面的后端回调地址。
                      </template>
                      <template v-else>
                        Setup guide: GitHub Settings → Developer settings →
                        <a
                          data-testid="github-oauth-apps-guide-link"
                          href="https://github.com/settings/developers"
                          target="_blank"
                          rel="noopener noreferrer"
                          class="font-[var(--fw-medium)] text-[var(--brand)] hover:underline "
                        >OAuth Apps</a>
                        → New OAuth App. Use your site origin as Homepage URL and the backend callback URL below as Authorization callback URL.
                      </template>
                    </div>

                    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
                      <div>
                        <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">Client ID</label>
                        <input
                          v-model="form.github_oauth_client_id"
                          type="text"
                          class="field-control font-mono text-sm"
                          placeholder="GitHub OAuth Client ID"
                        />
                      </div>
                      <div>
                        <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">Client Secret</label>
                        <input
                          v-model="form.github_oauth_client_secret"
                          type="password"
                          class="field-control font-mono text-sm"
                          :placeholder="
                            form.github_oauth_client_secret_configured
                              ? localText('密钥已配置，留空以保留当前值。', 'Secret configured. Leave empty to keep the current value.')
                              : 'GitHub OAuth Client Secret'
                          "
                        />
                      </div>
                    </div>

                    <div>
                      <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                        {{ localText("后端回调地址", "Backend Callback URL") }}
                      </label>
                      <input
                        v-model="form.github_oauth_redirect_url"
                        type="url"
                        class="field-control font-mono text-sm"
                        placeholder="https://your-domain.com/api/v1/auth/oauth/github/callback"
                      />
                      <div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                        <AppButton
                          variant="secondary"
                          size="sm"
                          type="button"
                          @click="setAndCopyEmailOAuthRedirectUrl('github')"
                        >
                          {{ localText("生成并复制", "Generate and copy") }}
                        </AppButton>
                        <code
                          v-if="githubOAuthRedirectUrlSuggestion"
                          class="select-all break-all rounded bg-[var(--surface-subtle)] px-2 py-1 font-mono text-xs text-[var(--body-copy)]  text-[var(--body-copy)]"
                        >
                          {{ githubOAuthRedirectUrlSuggestion }}
                        </code>
                      </div>
                    </div>

                    <div>
                      <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                        {{ localText("前端回跳地址", "Frontend Callback URL") }}
                      </label>
                      <input
                        v-model="form.github_oauth_frontend_redirect_url"
                        type="text"
                        class="field-control font-mono text-sm"
                        placeholder="/auth/oauth/callback"
                      />
                    </div>
                  </div>
                </div>

                <div class="rounded-[var(--r-md)] border border-[var(--border-subtle)] p-4                  ">
<div class="flex items-start justify-between gap-4">
                    <div>
                      <h3 class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                        Google
                      </h3>
                      <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{
                          localText(
                            "Google OAuth 客户端需要 openid email profile 范围，并在凭据里登记后端回调地址。",
                            "Google OAuth client needs openid email profile scopes and the backend callback URL registered in credentials.",
                          )
                        }}
                      </p>
                    </div>
                    <Toggle v-model="form.google_oauth_enabled" />
                  </div>

                  <div v-if="form.google_oauth_enabled" class="mt-4 space-y-4">
                    <div class="rounded-[var(--r-md)] bg-[var(--surface-subtle)] px-3 py-2 text-xs text-[var(--body-copy)]  text-[var(--body-copy)]">
                      {{
                        localText(
                          "开通引导：Google Cloud Console → APIs & Services → OAuth consent screen 完成同意屏幕；Credentials → Create Credentials → OAuth client ID，类型选择 Web application，并把下面地址加入 Authorized redirect URIs。",
                          "Setup guide: Google Cloud Console → APIs & Services → OAuth consent screen, then Credentials → Create Credentials → OAuth client ID, choose Web application, and add the URL below to Authorized redirect URIs.",
                        )
                      }}
                    </div>

                    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
                      <div>
                        <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">Client ID</label>
                        <input
                          v-model="form.google_oauth_client_id"
                          type="text"
                          class="field-control font-mono text-sm"
                          placeholder="Google OAuth Client ID"
                        />
                      </div>
                      <div>
                        <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">Client Secret</label>
                        <input
                          v-model="form.google_oauth_client_secret"
                          type="password"
                          class="field-control font-mono text-sm"
                          :placeholder="
                            form.google_oauth_client_secret_configured
                              ? localText('密钥已配置，留空以保留当前值。', 'Secret configured. Leave empty to keep the current value.')
                              : 'Google OAuth Client Secret'
                          "
                        />
                      </div>
                    </div>

                    <div>
                      <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                        {{ localText("后端回调地址", "Backend Callback URL") }}
                      </label>
                      <input
                        v-model="form.google_oauth_redirect_url"
                        type="url"
                        class="field-control font-mono text-sm"
                        placeholder="https://your-domain.com/api/v1/auth/oauth/google/callback"
                      />
                      <div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                        <AppButton
                          variant="secondary"
                          size="sm"
                          type="button"
                          @click="setAndCopyEmailOAuthRedirectUrl('google')"
                        >
                          {{ localText("生成并复制", "Generate and copy") }}
                        </AppButton>
                        <code
                          v-if="googleOAuthRedirectUrlSuggestion"
                          class="select-all break-all rounded bg-[var(--surface-subtle)] px-2 py-1 font-mono text-xs text-[var(--body-copy)]  text-[var(--body-copy)]"
                        >
                          {{ googleOAuthRedirectUrlSuggestion }}
                        </code>
                      </div>
                    </div>

                    <div>
                      <label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                        {{ localText("前端回跳地址", "Frontend Callback URL") }}
                      </label>
                      <input
                        v-model="form.google_oauth_frontend_redirect_url"
                        type="text"
                        class="field-control font-mono text-sm"
                        placeholder="/auth/oauth/callback"
                      />
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- WeChat Connect OAuth 登录 -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.wechatConnect.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.wechatConnect.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.wechatConnect.enabledLabel")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.wechatConnect.enabledHint") }}
                  </p>
                </div>
                <Toggle
                  v-model="form.wechat_connect_enabled"
                  data-testid="wechat-connect-enabled"
                />
              </div>

              <div
                v-if="form.wechat_connect_enabled"
                class="space-y-6 border-t border-[var(--border-subtle)] pt-4              ">
                <div class="space-y-4">
                  <div
                    class="rounded-[var(--r-md)] border border-[var(--border-subtle)] p-4                  ">
                    <div class="flex items-start justify-between gap-4">
                      <div>
                        <h3 class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                          {{ localText("PC 应用", "PC App") }}
                        </h3>
                        <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{
                            localText(
                              "桌面浏览器通过微信开放平台扫码登录。可与公众号或移动应用同时存在。",
                              "Desktop browsers sign in through WeChat Open Platform QR login. This can coexist with Official Account or Mobile App.",
                            )
                          }}
                        </p>
                      </div>
                      <Toggle
                        :model-value="form.wechat_connect_open_enabled"
                        data-testid="wechat-connect-open-enabled"
                        @update:model-value="handleWeChatOpenEnabledChange"
                      />
                    </div>
                    <div
                      v-if="form.wechat_connect_open_enabled"
                      class="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-2"
                    >
                      <div>
                        <label
                          class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >
                          {{ localText("PC AppID", "PC App ID") }}
                        </label>
                        <input
                          v-model="form.wechat_connect_open_app_id"
                          data-testid="wechat-connect-open-app-id"
                          type="text"
                          class="field-control font-mono text-sm"
                          :placeholder="
                            localText(
                              '微信开放平台 PC 应用 AppID',
                              'WeChat Open Platform PC App ID',
                            )
                          "
                        />
                      </div>
                      <div>
                        <label
                          class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >
                          {{ localText("PC AppSecret", "PC App Secret") }}
                        </label>
                        <input
                          v-model="form.wechat_connect_open_app_secret"
                          data-testid="wechat-connect-open-app-secret"
                          type="password"
                          class="field-control font-mono text-sm"
                          :placeholder="
                            form.wechat_connect_open_app_secret_configured
                              ? localText(
                                  '密钥已配置，留空以保留当前值。',
                                  'Secret configured. Leave empty to keep the current value.',
                                )
                              : localText(
                                  '微信开放平台 PC 应用 AppSecret',
                                  'WeChat Open Platform PC App Secret',
                                )
                          "
                        />
                      </div>
                    </div>
                  </div>

                  <div
                    class="rounded-[var(--r-md)] border border-[var(--border-subtle)] p-4                  ">
                    <div class="flex items-start justify-between gap-4">
                      <div>
                        <h3 class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                          {{ localText("公众号", "Official Account") }}
                        </h3>
                        <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{
                            localText(
                              "仅在微信内浏览器可用；非微信环境下会显示不可用。",
                              "Only available inside the WeChat browser. It is shown as unavailable outside WeChat.",
                            )
                          }}
                        </p>
                      </div>
                      <Toggle
                        :model-value="form.wechat_connect_mp_enabled"
                        data-testid="wechat-connect-mp-enabled"
                        @update:model-value="handleWeChatMPEnabledChange"
                      />
                    </div>
                    <div
                      v-if="form.wechat_connect_mp_enabled"
                      class="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-2"
                    >
                      <div>
                        <label
                          class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >
                          {{ localText("公众号 AppID", "Official Account App ID") }}
                        </label>
                        <input
                          v-model="form.wechat_connect_mp_app_id"
                          data-testid="wechat-connect-mp-app-id"
                          type="text"
                          class="field-control font-mono text-sm"
                          :placeholder="
                            localText(
                              '公众号 AppID',
                              'Official Account App ID',
                            )
                          "
                        />
                      </div>
                      <div>
                        <label
                          class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >
                          {{
                            localText(
                              "公众号 AppSecret",
                              "Official Account App Secret",
                            )
                          }}
                        </label>
                        <input
                          v-model="form.wechat_connect_mp_app_secret"
                          data-testid="wechat-connect-mp-app-secret"
                          type="password"
                          class="field-control font-mono text-sm"
                          :placeholder="
                            form.wechat_connect_mp_app_secret_configured
                              ? localText(
                                  '密钥已配置，留空以保留当前值。',
                                  'Secret configured. Leave empty to keep the current value.',
                                )
                              : localText(
                                  '公众号 AppSecret',
                                  'Official Account App Secret',
                                )
                          "
                        />
                      </div>
                    </div>
                  </div>

                  <div
                    class="rounded-[var(--r-md)] border border-[var(--border-subtle)] p-4                  ">
                    <div class="flex items-start justify-between gap-4">
                      <div>
                        <h3 class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                          {{ localText("移动应用", "Mobile App") }}
                        </h3>
                        <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{
                            localText(
                              "原生移动端通过微信 SDK 唤起授权，网页端不会直接发起该流程。",
                              "Native mobile clients start authorization through the WeChat SDK. The web UI does not launch this flow directly.",
                            )
                          }}
                        </p>
                      </div>
                      <Toggle
                        :model-value="form.wechat_connect_mobile_enabled"
                        data-testid="wechat-connect-mobile-enabled"
                        @update:model-value="handleWeChatMobileEnabledChange"
                      />
                    </div>
                    <div
                      v-if="form.wechat_connect_mobile_enabled"
                      class="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-2"
                    >
                      <div>
                        <label
                          class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >
                          {{ localText("移动应用 AppID", "Mobile App ID") }}
                        </label>
                        <input
                          v-model="form.wechat_connect_mobile_app_id"
                          data-testid="wechat-connect-mobile-app-id"
                          type="text"
                          class="field-control font-mono text-sm"
                          :placeholder="
                            localText(
                              '移动应用 AppID',
                              'Mobile App ID',
                            )
                          "
                        />
                      </div>
                      <div>
                        <label
                          class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                        >
                          {{ localText("移动应用 AppSecret", "Mobile App Secret") }}
                        </label>
                        <input
                          v-model="form.wechat_connect_mobile_app_secret"
                          data-testid="wechat-connect-mobile-app-secret"
                          type="password"
                          class="field-control font-mono text-sm"
                          :placeholder="
                            form.wechat_connect_mobile_app_secret_configured
                              ? localText(
                                  '密钥已配置，留空以保留当前值。',
                                  'Secret configured. Leave empty to keep the current value.',
                                )
                              : localText(
                                  '移动应用 AppSecret',
                                  'Mobile App Secret',
                                )
                          "
                        />
                      </div>
                    </div>
                  </div>
                </div>

                <div
                  v-if="
                    form.wechat_connect_open_enabled &&
                    (form.wechat_connect_mp_enabled ||
                      form.wechat_connect_mobile_enabled)
                  "
                  class="rounded-[var(--r-md)] border border-[var(--s2a-attn)] bg-[var(--s2a-attn-soft)] px-4 py-3 text-sm text-[var(--s2a-attn)]   "
                >
                  {{
                    localText(
                      "如果同时启用 PC 应用和公众号/移动应用，这些应用需要挂在同一个微信开放平台主体下，否则 UnionID 无法稳定归并账号。",
                      "When PC App is enabled together with Official Account or Mobile App, they should belong to the same WeChat Open Platform account so UnionID can merge identities reliably.",
                    )
                  }}
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{
                        localText(
                          "浏览器回调地址",
                          "Browser Redirect URL",
                        )
                      }}
                    </label>
                    <input
                      data-testid="wechat-connect-redirect-url"
                      v-model="form.wechat_connect_redirect_url"
                      type="url"
                      class="field-control font-mono text-sm"
                      :placeholder="t('admin.settings.wechatConnect.redirectUrlPlaceholder')"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        localText(
                          "用于 PC 应用和公众号的网页回调。移动应用走原生 SDK 时不直接使用这个浏览器回调。",
                          "Used by PC App and Official Account browser callbacks. Native mobile SDK flows do not start from this browser callback directly.",
                        )
                      }}
                    </p>
                    <div
                      class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3"
                    >
                      <AppButton
                        variant="secondary"
                        size="sm"
                        type="button"
                        @click="setAndCopyWeChatRedirectUrl"
                      >
                        {{ t("admin.settings.wechatConnect.generateAndCopy") }}
                      </AppButton>
                      <code
                        v-if="wechatRedirectUrlSuggestion"
                        class="select-all break-all rounded bg-[var(--surface-subtle)] px-2 py-1 font-mono text-xs text-[var(--body-copy)]  text-[var(--body-copy)]"
                      >
                        {{ wechatRedirectUrlSuggestion }}
                      </code>
                    </div>
                  </div>
                </div>

                <div>
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.wechatConnect.frontendRedirectUrlLabel") }}
                  </label>
                  <input
                    data-testid="wechat-connect-frontend-redirect-url"
                    v-model="form.wechat_connect_frontend_redirect_url"
                    type="text"
                    class="field-control font-mono text-sm"
                    :placeholder="t('admin.settings.wechatConnect.frontendRedirectUrlPlaceholder')"
                  />
                  <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.wechatConnect.frontendRedirectUrlHint") }}
                  </p>
                </div>
              </div>
            </div>
          </div>

          <!-- DingTalk Connect OAuth 登录 -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.dingtalk.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.dingtalk.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.dingtalk.enable")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.dingtalk.enableHint") }}
                  </p>
                </div>
                <Toggle v-model="form.dingtalk_connect_enabled" />
              </div>

              <div
                v-if="form.dingtalk_connect_enabled"
                class="border-t border-[var(--border-subtle)] pt-4              ">
                <div class="grid grid-cols-1 gap-6">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.dingtalk.clientId") }}
                    </label>
                    <input
                      v-model="form.dingtalk_connect_client_id"
                      type="text"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.dingtalk.clientIdPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.dingtalk.clientIdHint") }}
                    </p>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.dingtalk.clientSecret") }}
                    </label>
                    <input
                      v-model="form.dingtalk_connect_client_secret"
                      type="password"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        form.dingtalk_connect_client_secret_configured
                          ? t(
                              'admin.settings.dingtalk.clientSecretConfiguredPlaceholder',
                            )
                          : t('admin.settings.dingtalk.clientSecretPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        form.dingtalk_connect_client_secret_configured
                          ? t(
                              "admin.settings.dingtalk.clientSecretConfiguredHint",
                            )
                          : t("admin.settings.dingtalk.clientSecretHint")
                      }}
                    </p>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.dingtalk.redirectUrl") }}
                    </label>
                    <input
                      v-model="form.dingtalk_connect_redirect_url"
                      type="url"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.dingtalk.redirectUrlPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.dingtalk.redirectUrlHint") }}
                    </p>
                  </div>

                  <!-- Corp Restriction Policy -->
                  <div class="border-t border-[var(--border-subtle)] pt-4                    ">
<label class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]">
                      {{ t("admin.settings.dingtalk.corpPolicy.label") }}
                    </label>
                    <p class="mb-3 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.dingtalk.corpPolicy.hint") }}
                    </p>
                    <div class="space-y-2">
                      <Radio
                        v-model="form.dingtalk_connect_corp_restriction_policy"
                        value="none"
                        name="dingtalk-corp-policy"
                        :label="t('admin.settings.dingtalk.corpPolicy.none')"
                      />
                      <Radio
                        v-model="form.dingtalk_connect_corp_restriction_policy"
                        value="internal_only"
                        name="dingtalk-corp-policy"
                        :label="t('admin.settings.dingtalk.corpPolicy.internalOnly')"
                      />
                    </div>
                  </div>

                  <!-- bypass_registration toggle（仅 internal_only 模式下可见可用） -->
                  <div
                    v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
                    class="flex items-center justify-between pt-4 border-t border-[var(--border-subtle)]                  ">
                    <div>
                      <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                        t("admin.settings.dingtalk.bypassRegistration")
                      }}</label>
                      <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                        {{ t("admin.settings.dingtalk.bypassRegistrationHint") }}
                      </p>
                    </div>
                    <Toggle v-model="form.dingtalk_connect_bypass_registration" />
                  </div>

                  <!-- 身份同步开关（仅 internal_only 模式下可见） -->
                  <div
                    v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
                    class="pt-4 border-t border-[var(--border-subtle)]  space-y-2"
                  >
                    <div class="flex items-center justify-between">
                      <div>
                        <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                          t("admin.settings.dingtalk.syncDisplayName")
                        }}</label>
                        <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{ t("admin.settings.dingtalk.syncDisplayNameHint") }}
                        </p>
                      </div>
                      <Toggle v-model="form.dingtalk_connect_sync_display_name" />
                    </div>
                    <div v-if="form.dingtalk_connect_sync_display_name" class="space-y-2">
                      <div class="flex items-center gap-2">
                        <label class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)] whitespace-nowrap min-w-[5rem]">
                          {{ t("admin.settings.dingtalk.syncDisplayNameTarget") }}
                        </label>
                        <input
                          v-model="form.dingtalk_connect_sync_display_name_attr_key"
                          type="text"
                          placeholder="dingtalk_name"
                          class="field-control text-sm flex-1 max-w-xs"
                        />
                      </div>
                      <div class="flex items-center gap-2">
                        <label class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)] whitespace-nowrap min-w-[5rem]">
                          {{ t("admin.settings.dingtalk.syncAttrDisplayName") }}
                        </label>
                        <input
                          v-model="form.dingtalk_connect_sync_display_name_attr_name"
                          type="text"
                          :placeholder="localText('钉钉姓名', 'DingTalk Name')"
                          class="field-control text-sm flex-1 max-w-xs"
                        />
                      </div>
                    </div>
                    <p v-if="form.dingtalk_connect_sync_display_name" class="text-xs text-[var(--muted-foreground)] ">
                      {{ t("admin.settings.dingtalk.syncDisplayNameTargetHint") }}
                    </p>
                  </div>
                  <div
                    v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
                    class="pt-4 border-t border-[var(--border-subtle)]  space-y-2"
                  >
                    <div class="flex items-center justify-between">
                      <div>
                        <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                          t("admin.settings.dingtalk.syncCorpEmail")
                        }}</label>
                        <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{ t("admin.settings.dingtalk.syncCorpEmailHint") }}
                        </p>
                        <p class="text-xs text-[var(--s2a-attn)]  mt-1">
                          {{ t("admin.settings.dingtalk.syncCorpEmailPermissionHint") }}
                        </p>
                      </div>
                      <Toggle v-model="form.dingtalk_connect_sync_corp_email" />
                    </div>
                    <div v-if="form.dingtalk_connect_sync_corp_email" class="space-y-2">
                      <div class="flex items-center gap-2">
                        <label class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)] whitespace-nowrap min-w-[5rem]">
                          {{ t("admin.settings.dingtalk.syncCorpEmailTarget") }}
                        </label>
                        <input
                          v-model="form.dingtalk_connect_sync_corp_email_attr_key"
                          type="text"
                          placeholder="dingtalk_email"
                          class="field-control text-sm flex-1 max-w-xs"
                        />
                      </div>
                      <div class="flex items-center gap-2">
                        <label class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)] whitespace-nowrap min-w-[5rem]">
                          {{ t("admin.settings.dingtalk.syncAttrDisplayName") }}
                        </label>
                        <input
                          v-model="form.dingtalk_connect_sync_corp_email_attr_name"
                          type="text"
                          :placeholder="localText('钉钉企业邮箱', 'DingTalk Corporate Email')"
                          class="field-control text-sm flex-1 max-w-xs"
                        />
                      </div>
                    </div>
                    <p v-if="form.dingtalk_connect_sync_corp_email" class="text-xs text-[var(--muted-foreground)] ">
                      {{ t("admin.settings.dingtalk.syncCorpEmailTargetHint") }}
                    </p>
                  </div>
                  <div
                    v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
                    class="pt-4 border-t border-[var(--border-subtle)]  space-y-2"
                  >
                    <div class="flex items-center justify-between">
                      <div>
                        <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                          t("admin.settings.dingtalk.syncDept")
                        }}</label>
                        <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                          {{ t("admin.settings.dingtalk.syncDeptHint") }}
                        </p>
                        <p class="text-xs text-[var(--s2a-attn)]  mt-1">
                          {{ t("admin.settings.dingtalk.syncDeptPermissionHint") }}
                        </p>
                      </div>
                      <Toggle v-model="form.dingtalk_connect_sync_dept" />
                    </div>
                    <div v-if="form.dingtalk_connect_sync_dept" class="space-y-2">
                      <div class="flex items-center gap-2">
                        <label class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)] whitespace-nowrap min-w-[5rem]">
                          {{ t("admin.settings.dingtalk.syncDeptTarget") }}
                        </label>
                        <input
                          v-model="form.dingtalk_connect_sync_dept_attr_key"
                          type="text"
                          placeholder="dingtalk_department"
                          class="field-control text-sm flex-1 max-w-xs"
                        />
                      </div>
                      <div class="flex items-center gap-2">
                        <label class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)] whitespace-nowrap min-w-[5rem]">
                          {{ t("admin.settings.dingtalk.syncAttrDisplayName") }}
                        </label>
                        <input
                          v-model="form.dingtalk_connect_sync_dept_attr_name"
                          type="text"
                          :placeholder="localText('钉钉部门', 'DingTalk Department')"
                          class="field-control text-sm flex-1 max-w-xs"
                        />
                      </div>
                    </div>
                    <p v-if="form.dingtalk_connect_sync_dept" class="text-xs text-[var(--muted-foreground)] ">
                      {{ t("admin.settings.dingtalk.syncDeptTargetHint") }}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Generic OIDC OAuth 登录 -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.oidc.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.oidc.description") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.oidc.enable")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.oidc.enableHint") }}
                  </p>
                </div>
                <Toggle v-model="form.oidc_connect_enabled" />
              </div>

              <div
                v-if="form.oidc_connect_enabled"
                class="space-y-6 border-t border-[var(--border-subtle)] pt-4              ">
                <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.providerName") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_provider_name"
                      type="text"
                      class="field-control"
                      :placeholder="
                        t('admin.settings.oidc.providerNamePlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.clientId") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_client_id"
                      type="text"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.clientIdPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.clientSecret") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_client_secret"
                      type="password"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        form.oidc_connect_client_secret_configured
                          ? t(
                              'admin.settings.oidc.clientSecretConfiguredPlaceholder',
                            )
                          : t('admin.settings.oidc.clientSecretPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{
                        form.oidc_connect_client_secret_configured
                          ? t("admin.settings.oidc.clientSecretConfiguredHint")
                          : t("admin.settings.oidc.clientSecretHint")
                      }}
                    </p>
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.issuerUrl") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_issuer_url"
                      type="url"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.issuerUrlPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.discoveryUrl") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_discovery_url"
                      type="url"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.discoveryUrlPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.authorizeUrl") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_authorize_url"
                      type="url"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.authorizeUrlPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.tokenUrl") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_token_url"
                      type="url"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.tokenUrlPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.userinfoUrl") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_userinfo_url"
                      type="url"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.userinfoUrlPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.jwksUrl") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_jwks_url"
                      type="url"
                      class="field-control font-mono text-sm"
                      :placeholder="t('admin.settings.oidc.jwksUrlPlaceholder')"
                    />
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.scopes") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_scopes"
                      type="text"
                      class="field-control font-mono text-sm"
                      :placeholder="t('admin.settings.oidc.scopesPlaceholder')"
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.oidc.scopesHint") }}
                    </p>
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.redirectUrl") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_redirect_url"
                      type="url"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.redirectUrlPlaceholder')
                      "
                    />
                    <div
                      class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3"
                    >
                      <AppButton
                        variant="secondary"
                        size="sm"
                        type="button"
                        @click="setAndCopyOIDCRedirectUrl"
                      >
                        {{ t("admin.settings.oidc.quickSetCopy") }}
                      </AppButton>
                      <code
                        v-if="oidcRedirectUrlSuggestion"
                        class="select-all break-all rounded bg-[var(--surface-subtle)] px-2 py-1 font-mono text-xs text-[var(--body-copy)]  text-[var(--body-copy)]"
                      >
                        {{ oidcRedirectUrlSuggestion }}
                      </code>
                    </div>
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.oidc.redirectUrlHint") }}
                    </p>
                  </div>

                  <div class="lg:col-span-2">
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.frontendRedirectUrl") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_frontend_redirect_url"
                      type="text"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.frontendRedirectUrlPlaceholder')
                      "
                    />
                    <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                      {{ t("admin.settings.oidc.frontendRedirectUrlHint") }}
                    </p>
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.tokenAuthMethod") }}
                    </label>
                    <BaseSelect
                      v-model="form.oidc_connect_token_auth_method"
                      :options="[
                        { value: 'client_secret_post', label: 'client_secret_post' },
                        { value: 'client_secret_basic', label: 'client_secret_basic' },
                        { value: 'none', label: 'none' },
                      ]"
                      aria-label="OIDC token authentication method"
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.clockSkewSeconds") }}
                    </label>
                    <input
                      v-model.number="form.oidc_connect_clock_skew_seconds"
                      type="number"
                      min="0"
                      max="600"
                      class="field-control"
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.allowedSigningAlgs") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_allowed_signing_algs"
                      type="text"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.allowedSigningAlgsPlaceholder')
                      "
                    />
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                  <div
                    class="flex items-center justify-between rounded border border-[var(--border-subtle)] px-4 py-3                  ">
                    <div>
                      <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                        {{ t("admin.settings.oidc.usePkce") }}
                      </label>
                    </div>
                    <Toggle
                      v-model="form.oidc_connect_use_pkce"
                      data-testid="oidc-connect-use-pkce"
                    />
                  </div>

                  <div
                    class="flex items-center justify-between rounded border border-[var(--border-subtle)] px-4 py-3                  ">
                    <div>
                      <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                        {{ t("admin.settings.oidc.validateIdToken") }}
                      </label>
                    </div>
                    <Toggle
                      v-model="form.oidc_connect_validate_id_token"
                      data-testid="oidc-connect-validate-id-token"
                    />
                  </div>

                  <div
                    class="flex items-center justify-between rounded border border-[var(--border-subtle)] px-4 py-3                  ">
                    <div>
                      <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                        {{ t("admin.settings.oidc.requireEmailVerified") }}
                      </label>
                    </div>
                    <Toggle
                      v-model="form.oidc_connect_require_email_verified"
                    />
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.userinfoEmailPath") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_userinfo_email_path"
                      type="text"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.userinfoEmailPathPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.userinfoIdPath") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_userinfo_id_path"
                      type="text"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.userinfoIdPathPlaceholder')
                      "
                    />
                  </div>

                  <div>
                    <label
                      class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                    >
                      {{ t("admin.settings.oidc.userinfoUsernamePath") }}
                    </label>
                    <input
                      v-model="form.oidc_connect_userinfo_username_path"
                      type="text"
                      class="field-control font-mono text-sm"
                      :placeholder="
                        t('admin.settings.oidc.userinfoUsernamePathPlaceholder')
                      "
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <!-- /Tab: Security — Registration, Turnstile, LinuxDo, OIDC -->
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import AppButton from '@/components/common/Button.vue'
import BaseSelect from '@/components/common/Select.vue'
import Segmented from '@/components/common/Segmented.vue'
import Toggle from '@/components/common/Toggle.vue'
import Radio from '@/components/common/Radio.vue'
import { useSettingsPageBindings } from './settingsPageBindings'

export default defineComponent({
  name: 'AdminSecuritySettingsPage',
  components: { AppButton, BaseSelect, Icon, Radio, Segmented, Toggle },
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
