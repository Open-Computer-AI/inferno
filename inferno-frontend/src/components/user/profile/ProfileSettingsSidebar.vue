<template>
  <aside class="profile-settings-sidebar">
    <RouterLink class="profile-settings-sidebar__back" to="/dashboard">
      <Icon name="chevronLeft" size="sm" aria-hidden="true" />
      <span>{{ t('admin.settings.backToApp') }}</span>
    </RouterLink>

    <nav class="profile-settings-sidebar__nav" :aria-label="t('profile.title')">
      <section class="profile-settings-sidebar__group">
        <h2 class="profile-settings-sidebar__group-title">{{ t('profile.settings.sidebarTitle') }}</h2>
        <a
          v-for="group in groups"
          :key="group.key"
          :href="`#${group.key}`"
          class="profile-settings-sidebar__item"
          :class="{ 'profile-settings-sidebar__item--active': activeGroup === group.key }"
          :aria-current="activeGroup === group.key ? 'location' : undefined"
        >
          <span class="profile-settings-sidebar__icon">
            <Icon :name="group.icon" size="sm" aria-hidden="true" />
          </span>
          <span class="profile-settings-sidebar__label">{{ group.label }}</span>
        </a>
      </section>
    </nav>

    <div class="profile-settings-sidebar__account">
      <span class="profile-settings-sidebar__avatar">
        <img v-if="avatarUrl" :src="avatarUrl" :alt="displayName" />
        <AccountPatternAvatar v-else :seed="accountSeed" :size="22" />
      </span>
      <span class="profile-settings-sidebar__account-copy">
        <strong>{{ displayName }}</strong>
        <small>{{ roleLabel }}</small>
      </span>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import Icon from '@/components/icons/Icon.vue'
import AccountPatternAvatar from '@/components/admin/settings/AccountPatternAvatar.vue'

const { t } = useI18n()
const authStore = useAuthStore()
const route = useRoute()

type ProfileSidebarIcon = 'user' | 'shield' | 'bell' | 'link'
type ProfileSidebarGroup = { key: string; label: string; icon: ProfileSidebarIcon }

const groups = computed<ProfileSidebarGroup[]>(() => [
  { key: 'account', label: t('profile.settings.groups.account'), icon: 'user' },
  { key: 'security', label: t('profile.settings.groups.security'), icon: 'shield' },
  { key: 'notifications', label: t('profile.settings.groups.notifications'), icon: 'bell' },
  { key: 'sign-in-methods', label: t('profile.settings.groups.signInMethods'), icon: 'link' },
])
const activeGroup = computed(() => route.hash.replace(/^#/, '') || 'account')

const displayName = computed(() => authStore.user?.username?.trim() || authStore.user?.email?.split('@')[0] || t('profile.user'))
const roleLabel = computed(() => authStore.user?.role === 'admin' ? t('profile.administrator') : t('profile.user'))
const avatarUrl = computed(() => authStore.user?.avatar_url?.trim() || '')
const accountSeed = computed(() => authStore.user?.avatar_seed || String(authStore.user?.id || authStore.user?.email || displayName.value))
</script>

<style scoped>
.profile-settings-sidebar {
  display: flex;
  flex: 0 0 var(--sidebar-w);
  width: var(--sidebar-w);
  min-height: 100%;
  flex-direction: column;
  gap: 8px;
  padding: 10px 8px;
  background: var(--sidebar);
  color: var(--sidebar-foreground);
}

.profile-settings-sidebar__back,
.profile-settings-sidebar__item {
  display: flex;
  align-items: center;
  text-decoration: none;
  transition: background-color var(--t-fast) var(--ease-out), color var(--t-fast) var(--ease-out);
}

.profile-settings-sidebar__back {
  min-height: 30px;
  gap: 8px;
  padding: 0 8px;
  border-radius: var(--r-sm);
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}

.profile-settings-sidebar__back:hover,
.profile-settings-sidebar__back:focus-visible,
.profile-settings-sidebar__item:hover,
.profile-settings-sidebar__item:focus-visible,
.profile-settings-sidebar__item--active {
  background: var(--sidebar-accent);
  color: var(--foreground);
}

.profile-settings-sidebar__nav {
  display: grid;
  flex: 1 1 auto;
  min-height: 0;
  align-content: start;
  gap: 10px;
  margin-top: 10px;
  overflow: auto;
  scrollbar-width: none;
  -ms-overflow-style: none;
}

.profile-settings-sidebar__nav::-webkit-scrollbar {
  display: none;
}

.profile-settings-sidebar__group {
  display: grid;
  gap: 1px;
}

.profile-settings-sidebar__group-title {
  margin: 0;
  padding: 7px 8px 3px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
}

.profile-settings-sidebar__item {
  min-height: 30px;
  gap: 8px;
  padding: 0 8px;
  border-radius: var(--r-md);
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}

.profile-settings-sidebar__icon {
  display: inline-grid;
  flex: 0 0 18px;
  place-items: center;
}

.profile-settings-sidebar__label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-settings-sidebar__account {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  gap: 8px;
  padding: 7px 6px 0;
  border-top: 1px solid var(--sidebar-border);
}

.profile-settings-sidebar__avatar {
  display: inline-grid;
  flex: 0 0 22px;
  place-items: center;
  width: 22px;
  height: 22px;
  overflow: hidden;
  border-radius: var(--r-pill);
}

.profile-settings-sidebar__avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.profile-settings-sidebar__account-copy {
  display: grid;
  min-width: 0;
  gap: 1px;
}

.profile-settings-sidebar__account-copy strong,
.profile-settings-sidebar__account-copy small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-settings-sidebar__account-copy strong {
  color: var(--foreground);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}

.profile-settings-sidebar__account-copy small {
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

@media (max-width: 1023px) {
  .profile-settings-sidebar {
    width: auto;
    min-height: auto;
    padding: 10px 8px;
  }

  .profile-settings-sidebar__nav {
    display: flex;
    gap: 2px;
    overflow-x: auto;
    overflow-y: hidden;
    padding-bottom: 2px;
  }

  .profile-settings-sidebar__group {
    min-width: max-content;
  }

  .profile-settings-sidebar__group-title {
    display: none;
  }

  .profile-settings-sidebar__account {
    display: none;
  }
}
</style>
