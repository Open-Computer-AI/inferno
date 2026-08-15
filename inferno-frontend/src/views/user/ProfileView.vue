<template>
  <AppLayout variant="settings">
    <template #sidebar>
      <ProfileSettingsSidebar />
    </template>

    <section data-testid="profile-shell" class="profile-settings-main">
      <div class="profile-settings-main__inner">
        <header class="profile-settings-header">
          <h1>{{ t('profile.title') }}</h1>
          <p>{{ t('profile.description') }}</p>
        </header>

        <div class="profile-settings-groups">
          <ProfileSettingsGroup id="account" :title="t('profile.settings.groups.account')">
            <ProfileSettingsRow
              :label="primaryEmailDisplay || t('profile.settings.emailFallback')"
              :description="memberSinceLabel"
              :value="user?.status === 'active' ? t('common.active') : t('common.disabled')"
              :tone="user?.status === 'active' ? 'success' : 'muted'"
            />
            <ProfileSettingsRow
              :label="t('profile.settings.avatarLabel')"
              :description="t('profile.settings.avatarDescription')"
              :action="t('profile.settings.actions.change')"
              :close-action="t('profile.settings.actions.close')"
              :open="openRows.avatar"
              @update:open="openRows.avatar = $event"
            >
              <ProfileAvatarCard :user="user" embedded />
            </ProfileSettingsRow>
            <ProfileSettingsRow
              :label="t('profile.username')"
              :description="t('profile.settings.usernameDescription')"
              :value="user?.username || t('profile.settings.notSet')"
              :action="t('profile.settings.actions.edit')"
              :close-action="t('profile.settings.actions.close')"
              :open="openRows.username"
              @update:open="openRows.username = $event"
            >
              <ProfileEditForm :initial-username="user?.username || ''" embedded />
            </ProfileSettingsRow>
          </ProfileSettingsGroup>

          <ProfileSettingsGroup
            id="security"
            :title="t('profile.settings.groups.security')"
            :description="t('profile.settings.securityDescription')"
          >
            <ProfileSettingsRow
              :label="t('profile.changePassword')"
              :description="t('profile.settings.passwordDescription')"
              :action="t('profile.settings.actions.change')"
              :close-action="t('profile.settings.actions.close')"
              :open="openRows.password"
              @update:open="openRows.password = $event"
            >
              <ProfilePasswordForm embedded />
            </ProfileSettingsRow>
            <ProfileSettingsRow
              :label="t('profile.totp.title')"
              :description="t('profile.totp.description')"
              :action="t('profile.settings.actions.manage')"
              :close-action="t('profile.settings.actions.close')"
              :open="openRows.totp"
              @update:open="openRows.totp = $event"
            >
              <ProfileTotpCard embedded />
            </ProfileSettingsRow>
            <ProfileSettingsRow
              :label="t('profile.passkey.title')"
              :description="t('profile.passkey.description')"
              :action="t('profile.settings.actions.manage')"
              :close-action="t('profile.settings.actions.close')"
              :open="openRows.passkeys"
              @update:open="openRows.passkeys = $event"
            >
              <ProfilePasskeyCard :enabled="passkeyEnabled" embedded />
            </ProfileSettingsRow>
          </ProfileSettingsGroup>

          <ProfileSettingsGroup id="notifications" :title="t('profile.settings.groups.notifications')">
            <ProfileSettingsRow
              :label="t('profile.balanceNotify.title')"
              :description="t('profile.balanceNotify.description')"
              :value="user?.balance_notify_enabled ? t('profile.settings.status.on') : t('profile.settings.status.off')"
              :tone="user?.balance_notify_enabled ? 'success' : 'muted'"
              :action="t('profile.settings.actions.manage')"
              :close-action="t('profile.settings.actions.close')"
              :open="openRows.notifications"
              @update:open="openRows.notifications = $event"
            >
              <ProfileBalanceNotifyCard
                v-if="user && balanceLowNotifyEnabled"
                :enabled="user.balance_notify_enabled ?? true"
                :threshold="user.balance_notify_threshold"
                :extra-emails="user.balance_notify_extra_emails ?? []"
                :system-default-threshold="systemDefaultThreshold"
                :user-email="user.email"
                embedded
              />
              <p v-else class="profile-settings-empty">{{ t('profile.settings.notificationsDisabled') }}</p>
            </ProfileSettingsRow>
          </ProfileSettingsGroup>

          <ProfileSettingsGroup
            id="sign-in-methods"
            :title="t('profile.settings.groups.signInMethods')"
            :description="t('profile.authBindings.description')"
          >
            <ProfileSettingsRow
              :label="t('profile.authBindings.title')"
              :description="t('profile.settings.signInMethodsDescription')"
              :action="t('profile.settings.actions.manage')"
              :close-action="t('profile.settings.actions.close')"
              :open="openRows.bindings"
              @update:open="openRows.bindings = $event"
            >
              <ProfileIdentityBindingsSection
                :user="user"
                :linuxdo-enabled="linuxdoOAuthEnabled"
                :dingtalk-enabled="dingtalkOAuthEnabled"
                :oidc-enabled="oidcOAuthEnabled"
                :oidc-provider-name="oidcOAuthProviderName"
                :wechat-enabled="wechatOAuthEnabled"
                :wechat-open-enabled="wechatOAuthOpenEnabled"
                :wechat-mp-enabled="wechatOAuthMPEnabled"
                embedded
                compact
              />
            </ProfileSettingsRow>
          </ProfileSettingsGroup>

          <div v-if="contactInfo" class="profile-settings-support">
            <Icon name="chat" size="sm" aria-hidden="true" />
            <span><strong>{{ t('common.contactSupport') }}</strong> {{ contactInfo }}</span>
          </div>
        </div>
      </div>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import ProfileBalanceNotifyCard from '@/components/user/profile/ProfileBalanceNotifyCard.vue'
import ProfileAvatarCard from '@/components/user/profile/ProfileAvatarCard.vue'
import ProfileEditForm from '@/components/user/profile/ProfileEditForm.vue'
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import ProfilePasswordForm from '@/components/user/profile/ProfilePasswordForm.vue'
import ProfilePasskeyCard from '@/components/user/profile/ProfilePasskeyCard.vue'
import ProfileSettingsGroup from '@/components/user/profile/ProfileSettingsGroup.vue'
import ProfileSettingsRow from '@/components/user/profile/ProfileSettingsRow.vue'
import ProfileSettingsSidebar from '@/components/user/profile/ProfileSettingsSidebar.vue'
import ProfileTotpCard from '@/components/user/profile/ProfileTotpCard.vue'
import { isWeChatWebOAuthEnabled } from '@/api/auth'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const user = computed(() => authStore.user)

const contactInfo = ref('')
const balanceLowNotifyEnabled = ref(false)
const systemDefaultThreshold = ref(0)
const linuxdoOAuthEnabled = ref(false)
const dingtalkOAuthEnabled = ref(false)
const wechatOAuthEnabled = ref(false)
const wechatOAuthOpenEnabled = ref<boolean | undefined>(undefined)
const wechatOAuthMPEnabled = ref<boolean | undefined>(undefined)
const oidcOAuthEnabled = ref(false)
const oidcOAuthProviderName = ref('OIDC')
const passkeyEnabled = ref(false)
const openRows = reactive({
  avatar: false,
  username: false,
  password: false,
  totp: false,
  passkeys: false,
  notifications: false,
  bindings: false,
})

const primaryEmailDisplay = computed(() => {
  const email = user.value?.email?.trim() || ''
  return email.endsWith('.invalid') ? '' : email
})

const memberSinceLabel = computed(() => {
  const raw = user.value?.created_at?.trim()
  if (!raw) return t('profile.settings.memberSinceUnknown')
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return t('profile.settings.memberSinceUnknown')
  return t('profile.settings.memberSince', {
    date: new Intl.DateTimeFormat(undefined, { year: 'numeric', month: 'long' }).format(date),
  })
})

onMounted(async () => {
  const profileRefresh = authStore.refreshUser().catch((error) => {
    console.error('Failed to refresh profile:', error)
  })

  const settingsLoad = appStore.fetchPublicSettings()
    .then((settings) => {
      if (!settings) {
        return
      }
      contactInfo.value = settings.contact_info || ''
      balanceLowNotifyEnabled.value = settings.balance_low_notify_enabled ?? false
      systemDefaultThreshold.value = settings.balance_low_notify_threshold ?? 0
      linuxdoOAuthEnabled.value = settings.linuxdo_oauth_enabled ?? false
      dingtalkOAuthEnabled.value = settings.dingtalk_oauth_enabled ?? false
      wechatOAuthEnabled.value = isWeChatWebOAuthEnabled(settings)
      wechatOAuthOpenEnabled.value = typeof settings.wechat_oauth_open_enabled === 'boolean'
        ? settings.wechat_oauth_open_enabled
        : undefined
      wechatOAuthMPEnabled.value = typeof settings.wechat_oauth_mp_enabled === 'boolean'
        ? settings.wechat_oauth_mp_enabled
        : undefined
      oidcOAuthEnabled.value = settings.oidc_oauth_enabled ?? false
      oidcOAuthProviderName.value = settings.oidc_oauth_provider_name || 'OIDC'
      passkeyEnabled.value = settings.passkey_enabled === true
    })
    .catch((error) => {
      console.error('Failed to load settings:', error)
    })

  await Promise.all([profileRefresh, settingsLoad])
})
</script>

<style scoped>
.profile-settings-main {
  min-width: 0;
  min-height: 100%;
  background: var(--card);
}

.profile-settings-main__inner {
  width: min(100%, 900px);
  margin: 0 auto;
  padding: 44px 56px 72px;
}

.profile-settings-header {
  margin-bottom: 30px;
}

.profile-settings-header h1 {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: clamp(1.7rem, 2.2vw, 2.35rem);
  font-weight: 400;
  letter-spacing: -0.025em;
  line-height: 1.15;
}

.profile-settings-header p {
  max-width: 680px;
  margin: 10px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-lg);
  line-height: 1.45;
}

.profile-settings-groups {
  display: grid;
  gap: 14px;
}

.profile-settings-support {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border: 1px solid var(--brand-line);
  border-radius: var(--r-lg);
  background: color-mix(in oklch, var(--brand) 7%, transparent);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.profile-settings-support strong {
  color: var(--foreground);
  font-weight: var(--fw-medium);
}

.profile-settings-empty {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

@media (max-width: 900px) {
  .profile-settings-main__inner {
    padding: 32px 28px 56px;
  }
}

@media (max-width: 700px) {
  .profile-settings-main__inner {
    padding: 24px 16px 48px;
  }

  .profile-settings-header {
    margin-bottom: 22px;
  }
}
</style>
