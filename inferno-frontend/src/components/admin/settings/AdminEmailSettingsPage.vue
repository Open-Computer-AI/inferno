<template>
<div class="settings-email-page">
          <!-- Email disabled hint - show when email_verify_enabled is off -->
          <div v-if="!form.email_verify_enabled" class="settings-surface">
            <div class="p-6">
              <div class="flex items-start gap-3">
                <Icon
                  name="mail"
                  size="md"
                  class="mt-0.5 flex-shrink-0 text-[var(--muted-foreground)] "
                />
                <div>
                  <h3 class="font-[var(--fw-medium)] text-[var(--foreground)] ">
                    {{ t("admin.settings.emailTabDisabledTitle") }}
                  </h3>
                  <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.emailTabDisabledHint") }}
                  </p>
                </div>
              </div>
            </div>
          </div>

          <!-- SMTP Settings - Only show when email verification is enabled -->
          <div v-if="form.email_verify_enabled" class="settings-surface">
            <div
              class="flex items-center justify-between border-b border-[var(--border-subtle)] px-6 py-4            ">
              <div>
                <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                  {{ t("admin.settings.smtp.title") }}
                </h2>
                <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t("admin.settings.smtp.description") }}
                </p>
              </div>
              <Button
                type="button"
                @click="testSmtpConnection"
                :disabled="testingSmtp || loadFailed"
                variant="secondary"
                size="sm"
                :loading="testingSmtp"
              >
                {{
                  testingSmtp
                    ? t("admin.settings.smtp.testing")
                    : t("admin.settings.smtp.testConnection")
                }}
              </Button>
            </div>
            <div class="space-y-6 p-6">
              <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div>
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.smtp.host") }}
                  </label>
                  <input
                    v-model="form.smtp_host"
                    type="text"
                    class="field-control"
                    :placeholder="t('admin.settings.smtp.hostPlaceholder')"
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.smtp.port") }}
                  </label>
                  <input
                    v-model.number="form.smtp_port"
                    type="number"
                    min="1"
                    max="65535"
                    class="field-control"
                    :placeholder="t('admin.settings.smtp.portPlaceholder')"
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.smtp.username") }}
                  </label>
                  <input
                    v-model="form.smtp_username"
                    type="text"
                    class="field-control"
                    :placeholder="t('admin.settings.smtp.usernamePlaceholder')"
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.smtp.password") }}
                  </label>
                  <input
                    v-model="form.smtp_password"
                    type="password"
                    class="field-control"
                    autocomplete="new-password"
                    autocapitalize="off"
                    spellcheck="false"
                    @keydown="smtpPasswordManuallyEdited = true"
                    @paste="smtpPasswordManuallyEdited = true"
                    :placeholder="
                      form.smtp_password_configured
                        ? t('admin.settings.smtp.passwordConfiguredPlaceholder')
                        : t('admin.settings.smtp.passwordPlaceholder')
                    "
                  />
                  <p class="mt-1.5 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{
                      form.smtp_password_configured
                        ? t("admin.settings.smtp.passwordConfiguredHint")
                        : t("admin.settings.smtp.passwordHint")
                    }}
                  </p>
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.smtp.fromEmail") }}
                  </label>
                  <input
                    v-model="form.smtp_from_email"
                    type="email"
                    class="field-control"
                    :placeholder="t('admin.settings.smtp.fromEmailPlaceholder')"
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.smtp.fromName") }}
                  </label>
                  <input
                    v-model="form.smtp_from_name"
                    type="text"
                    class="field-control"
                    :placeholder="t('admin.settings.smtp.fromNamePlaceholder')"
                  />
                </div>
              </div>

              <!-- Use TLS Toggle -->
              <div
                class="flex items-center justify-between border-t border-[var(--border-subtle)] pt-4              ">
                <div>
                  <label class="font-[var(--fw-medium)] text-[var(--foreground)] ">{{
                    t("admin.settings.smtp.useTls")
                  }}</label>
                  <p class="text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.smtp.useTlsHint") }}
                  </p>
                </div>
                <Toggle v-model="form.smtp_use_tls" />
              </div>
            </div>
          </div>

          <!-- Send Test Email - Only show when email verification is enabled -->
          <div v-if="form.email_verify_enabled" class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h2 class="text-lg font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.testEmail.title") }}
              </h2>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.testEmail.description") }}
              </p>
            </div>
            <div class="p-6">
              <div class="flex items-end gap-4">
                <div class="flex-1">
                  <label
                    class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.testEmail.recipientEmail") }}
                  </label>
                  <input
                    v-model="testEmailAddress"
                    type="email"
                    class="field-control"
                    :placeholder="
                      t('admin.settings.testEmail.recipientEmailPlaceholder')
                    "
                  />
                </div>
                <Button
                  type="button"
                  @click="sendTestEmail"
                  :disabled="
                    sendingTestEmail || !testEmailAddress || loadFailed
                  "
                  variant="secondary"
                  :loading="sendingTestEmail"
                >
                  {{
                    sendingTestEmail
                      ? t("admin.settings.testEmail.sending")
                      : t("admin.settings.testEmail.sendTestEmail")
                  }}
                </Button>
              </div>
            </div>
          </div>

          <!-- 订阅到期提醒 -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h3 class="text-base font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.subscriptionExpiryNotify.title") }}
              </h3>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.subscriptionExpiryNotify.description") }}
              </p>
            </div>
            <div class="px-6 py-6">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <label
                    class="mb-0 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >
                    {{ t("admin.settings.subscriptionExpiryNotify.enabled") }}
                  </label>
                  <p class="mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                    {{ t("admin.settings.subscriptionExpiryNotify.enabledHint") }}
                  </p>
                </div>
                <Toggle v-model="form.subscription_expiry_notify_enabled" />
              </div>
            </div>
          </div>

          <EmailTemplateEditor />

          <!-- Balance Low Notification -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h3 class="text-base font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.balanceNotify.title") }}
              </h3>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.balanceNotify.description") }}
              </p>
            </div>
            <div class="px-6 py-6 space-y-4">
              <div class="flex items-center justify-between">
                <label
                  class="mb-0 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >{{ t("admin.settings.balanceNotify.enabled") }}</label
                >
                <Toggle v-model="form.balance_low_notify_enabled" />
              </div>
              <div v-if="form.balance_low_notify_enabled">
                <label
                  class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >{{ t("admin.settings.balanceNotify.threshold") }}</label
                >
                <div class="relative">
                  <span
                    class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]"
                    >$</span
                  >
                  <input
                    v-model.number="form.balance_low_notify_threshold"
                    type="number"
                    min="0"
                    step="0.01"
                    class="field-control pl-7"
                  />
                </div>
                <p class="mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t("admin.settings.balanceNotify.thresholdHint") }}
                </p>
              </div>
              <div>
                <label
                  class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >{{ t("admin.settings.balanceNotify.rechargeUrl") }}</label
                >
                <input
                  v-model="form.balance_low_notify_recharge_url"
                  type="url"
                  class="field-control"
                  :placeholder="currentOrigin"
                />
                <p class="mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t("admin.settings.balanceNotify.rechargeUrlHint") }}
                </p>
              </div>
            </div>
          </div>

          <!-- Account Quota Notification -->
          <div class="settings-surface">
            <div
              class="border-b border-[var(--border-subtle)] px-6 py-4            ">
              <h3 class="text-base font-[var(--fw-medium)] text-[var(--foreground)] ">
                {{ t("admin.settings.quotaNotify.title") }}
              </h3>
              <p class="mt-1 text-sm text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                {{ t("admin.settings.quotaNotify.description") }}
              </p>
            </div>
            <div class="px-6 py-6 space-y-4">
              <div class="flex items-center justify-between">
                <label
                  class="mb-0 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >{{ t("admin.settings.quotaNotify.enabled") }}</label
                >
                <Toggle v-model="form.account_quota_notify_enabled" />
              </div>
              <div v-if="form.account_quota_notify_enabled">
                <label
                  class="mb-2 block text-sm font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--body-copy)]"
                  >{{ t("admin.settings.quotaNotify.emails") }}</label
                >
                <div class="space-y-2">
                  <div
                    v-for="(entry, index) in form.account_quota_notify_emails ||
                    []"
                    :key="index"
                    class="flex items-center gap-2"
                  >
                    <Toggle
                      :model-value="!entry.disabled"
                      @update:model-value="entry.disabled = !$event"
                    />
                    <input
                      v-model="entry.email"
                      type="email"
                      class="field-control flex-1"
                      :placeholder="
                        t('admin.settings.quotaNotify.emailPlaceholder')
                      "
                    />
                    <Button
                      @click="form.account_quota_notify_emails.splice(index, 1)"
                      variant="ghost"
                      size="xs"
                      type="button"
                      :aria-label="t('common.delete')"
                    >
                      <Icon name="x" size="xs" class="h-4 w-4" />
                    </Button>
                  </div>
                  <Button
                    @click="addQuotaNotifyEmail"
                    variant="secondary"
                    size="sm"
                    type="button"
                  >
                    + {{ t("admin.settings.quotaNotify.addEmail") }}
                  </Button>
                </div>
                <p class="mt-1 text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">
                  {{ t("admin.settings.quotaNotify.emailsHint") }}
                </p>
              </div>
            </div>
          </div>
        </div>
        <!-- /Tab: Email -->
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import Button from '@/components/common/Button.vue'
import Toggle from '@/components/common/Toggle.vue'
import EmailTemplateEditor from '@/views/admin/settings/EmailTemplateEditor.vue'
import { useSettingsPageBindings } from './settingsPageBindings'

export default defineComponent({
  name: 'AdminEmailSettingsPage',
  components: {
    EmailTemplateEditor,
    Button,
    Icon,
    Toggle,
  },
  setup() {
    return useSettingsPageBindings()
  },
})
</script>

<style scoped>
.settings-email-page {
  display: grid;
  gap: 20px;
}

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

  :deep(.flex.items-end) {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
