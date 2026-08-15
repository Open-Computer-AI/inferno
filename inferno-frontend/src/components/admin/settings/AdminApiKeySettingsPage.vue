<template>
  <section class="settings-page-section settings-page-section--security">
    <div class="settings-page-section__surface">
      <header class="settings-page-section__header">
        <h2>{{ t("admin.settings.adminApiKey.title") }}</h2>
        <p>{{ t("admin.settings.adminApiKey.description") }}</p>
      </header>

      <div class="settings-page-section__body">
        <div class="settings-page-section__notice settings-page-section__notice--attention">
          <Icon name="exclamationTriangle" size="md" aria-hidden="true" />
          <p>{{ t("admin.settings.adminApiKey.securityWarning") }}</p>
        </div>

        <div v-if="adminApiKeyLoading" class="settings-page-section__loading">
          <span class="settings-page-section__spinner" aria-hidden="true" />
          {{ t("common.loading") }}
        </div>

        <div v-else-if="!adminApiKeyExists" class="settings-page-section__action-row">
          <span class="settings-page-section__muted">
            {{ t("admin.settings.adminApiKey.notConfigured") }}
          </span>
          <Button
            type="button"
            variant="solid"
            size="sm"
            :disabled="adminApiKeyOperating"
            :loading="adminApiKeyOperating"
            @click="createAdminApiKey"
          >
            {{
              adminApiKeyOperating
                ? t("admin.settings.adminApiKey.creating")
                : t("admin.settings.adminApiKey.create")
            }}
          </Button>
        </div>

        <div v-else class="settings-page-section__stack">
          <div class="settings-page-section__action-row">
            <div>
              <span class="settings-page-section__label">
                {{ t("admin.settings.adminApiKey.currentKey") }}
              </span>
              <code class="settings-page-section__code">{{ adminApiKeyMasked }}</code>
            </div>
            <div class="settings-page-section__actions">
              <Button
                type="button"
                variant="secondary"
                size="sm"
                :disabled="adminApiKeyOperating"
                :loading="adminApiKeyOperating"
                @click="regenerateAdminApiKey"
              >
                {{
                  adminApiKeyOperating
                    ? t("admin.settings.adminApiKey.regenerating")
                    : t("admin.settings.adminApiKey.regenerate")
                }}
              </Button>
              <Button
                type="button"
                variant="danger"
                size="sm"
                :disabled="adminApiKeyOperating"
                @click="deleteAdminApiKey"
              >
                {{ t("admin.settings.adminApiKey.delete") }}
              </Button>
            </div>
          </div>

          <div v-if="newAdminApiKey" class="settings-page-section__notice settings-page-section__notice--success">
            <div class="settings-page-section__stack settings-page-section__stack--tight">
              <p class="settings-page-section__label">
                {{ t("admin.settings.adminApiKey.keyWarning") }}
              </p>
              <div class="settings-page-section__action-row">
                <code class="settings-page-section__code settings-page-section__code--expanded">{{ newAdminApiKey }}</code>
                <Button type="button" variant="solid" size="sm" @click="copyNewKey">
                  {{ t("admin.settings.adminApiKey.copyKey") }}
                </Button>
              </div>
              <p class="settings-page-section__hint">
                {{ t("admin.settings.adminApiKey.usage") }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script lang="ts">
import { defineComponent } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import Button from '@/components/common/Button.vue'
import { useSettingsPageBindings } from './settingsPageBindings'

export default defineComponent({
  name: 'AdminApiKeySettingsPage',
  components: { Button, Icon },
  setup() {
    return useSettingsPageBindings()
  },
})
</script>

<style scoped>
.settings-page-section {
  width: 100%;
}

.settings-page-section__surface {
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
  overflow: hidden;
}

.settings-page-section__header {
  padding: 16px 18px 13px;
  border-bottom: 1px solid var(--border-subtle);
}

.settings-page-section__header h2 {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.settings-page-section__header p,
.settings-page-section__hint,
.settings-page-section__muted {
  margin: 4px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  line-height: 1.45;
}

.settings-page-section__body,
.settings-page-section__stack {
  display: grid;
  gap: 16px;
  padding: 18px;
}

.settings-page-section__stack--tight {
  gap: 8px;
  padding: 0;
}

.settings-page-section__notice {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  color: var(--body-copy);
  background: var(--surface-subtle);
}

.settings-page-section__notice :deep(.icon) {
  flex: none;
  color: var(--s2a-attn);
}

.settings-page-section__notice p {
  margin: 0;
  font-size: var(--fs-sm);
  line-height: 1.45;
}

.settings-page-section__notice--attention {
  border-color: var(--s2a-attn-soft);
}

.settings-page-section__notice--success {
  border-color: color-mix(in oklch, var(--success) 28%, var(--border-subtle));
  background: color-mix(in oklch, var(--success) 7%, var(--card));
}

.settings-page-section__loading,
.settings-page-section__action-row,
.settings-page-section__actions {
  display: flex;
  align-items: center;
}

.settings-page-section__loading,
.settings-page-section__action-row {
  justify-content: space-between;
  gap: 16px;
}

.settings-page-section__actions {
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.settings-page-section__spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  margin-right: 6px;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: var(--r-pill);
  animation: settings-page-section-spin var(--t-slow) linear infinite;
}

.settings-page-section__label {
  display: block;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
}

.settings-page-section__code {
  display: inline-block;
  max-width: 100%;
  margin-top: 5px;
  padding: 4px 7px;
  overflow-wrap: anywhere;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-sm);
  color: var(--body-copy);
  background: var(--surface-subtle);
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
}

.settings-page-section__code--expanded {
  flex: 1;
  margin-top: 0;
}

.settings-page-section__danger-action {
  color: var(--destructive);
}

@keyframes settings-page-section-spin {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .settings-page-section__spinner { animation: none; }
}

@media (max-width: 600px) {
  .settings-page-section__action-row {
    align-items: stretch;
    flex-direction: column;
  }

  .settings-page-section__actions {
    justify-content: flex-start;
  }
}
</style>
