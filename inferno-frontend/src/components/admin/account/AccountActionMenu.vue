<template>
  <Teleport to="body">
    <div v-if="show && anchorRect">
      <!-- Backdrop: click anywhere outside to close -->
      <div class="fixed inset-0 z-[9998]" @click="emit('close')"></div>
      <div
        ref="menuRef"
        class="action-menu-content am-menu overflow-y-auto overscroll-contain"
        :style="menuStyle"
        role="menu"
        :aria-label="t('admin.accounts.moreActions')"
        @click.stop
        @mousedown.stop
        @keydown="onMenuKeydown"
      >
        <template v-if="account">
          <div class="am-group">
            <button type="button" role="menuitem" class="am-item" @click="$emit('test', account); $emit('close')">
              <Icon name="play" size="sm" aria-hidden="true" />
              <span>{{ t('admin.accounts.testConnection') }}</span>
            </button>
            <button type="button" role="menuitem" class="am-item" @click="$emit('stats', account); $emit('close')">
              <Icon name="chart" size="sm" aria-hidden="true" />
              <span>{{ t('admin.accounts.viewStats') }}</span>
            </button>
            <button type="button" role="menuitem" class="am-item" @click="$emit('schedule', account); $emit('close')">
              <Icon name="clock" size="sm" aria-hidden="true" />
              <span>{{ t('admin.scheduledTests.schedule') }}</span>
            </button>
          </div>

          <div v-if="showChangeStateGroup" class="am-group">
            <button v-if="canDuplicate" type="button" role="menuitem" class="am-item" @click="$emit('duplicate', account); $emit('close')">
              <Icon name="copy" size="sm" aria-hidden="true" />
              <span>{{ t('admin.accounts.duplicateAccount') }}</span>
            </button>
            <button v-if="isOpenAIOAuthParent" type="button" role="menuitem" class="am-item" @click="$emit('create-spark-shadow', account); $emit('close')">
              <Icon name="sparkles" size="sm" aria-hidden="true" />
              <span>{{ t('admin.accounts.createSparkShadow') }}</span>
            </button>
            <button v-if="supportsPrivacy" type="button" role="menuitem" class="am-item" @click="$emit('set-privacy', account); $emit('close')">
              <Icon name="shield" size="sm" aria-hidden="true" />
              <span>{{ t('admin.accounts.setPrivacy') }}</span>
            </button>
            <button v-if="hasRecoverableState" type="button" role="menuitem" class="am-item" @click="$emit('recover-state', account); $emit('close')">
              <Icon name="sync" size="sm" aria-hidden="true" />
              <span>{{ t('admin.accounts.recoverState') }}</span>
            </button>
            <button v-if="hasQuotaLimit" type="button" role="menuitem" class="am-item" @click="$emit('reset-quota', account); $emit('close')">
              <Icon name="refresh" size="sm" aria-hidden="true" />
              <span>{{ t('admin.accounts.resetQuota') }}</span>
            </button>
          </div>

          <div v-if="showReauthGroup" class="am-group">
            <button type="button" role="menuitem" class="am-item" @click="$emit('reauth', account); $emit('close')">
              <Icon name="link" size="sm" aria-hidden="true" />
              <span>{{ t('admin.accounts.reAuthorize') }}</span>
            </button>
            <button type="button" role="menuitem" class="am-item" @click="$emit('refresh-token', account); $emit('close')">
              <Icon name="refresh" size="sm" aria-hidden="true" />
              <span>{{ t('admin.accounts.refreshToken') }}</span>
            </button>
          </div>
        </template>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch, onUnmounted } from 'vue'
import { useResizeObserver, useWindowSize } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import type { Account } from '@/types'

const props = defineProps<{ show: boolean; account: Account | null; anchorRect: DOMRect | null }>()
const emit = defineEmits(['close', 'test', 'stats', 'schedule', 'duplicate', 'reauth', 'refresh-token', 'recover-state', 'reset-quota', 'set-privacy', 'create-spark-shadow'])
const { t } = useI18n()
const menuRef = ref<HTMLElement | null>(null)
const { width: viewportWidth, height: viewportHeight } = useWindowSize()
const viewportPadding = 8
const menuPosition = ref({ top: viewportPadding, left: viewportPadding })
const menuStyle = computed(() => ({
  top: `${menuPosition.value.top}px`,
  left: `${menuPosition.value.left}px`,
  maxWidth: `${Math.max(0, viewportWidth.value - viewportPadding * 2)}px`,
  maxHeight: `${Math.max(0, viewportHeight.value - viewportPadding * 2)}px`
}))

const updatePosition = () => {
  if (!menuRef.value || !props.anchorRect) return

  const { width, height } = menuRef.value.getBoundingClientRect()
  const anchor = props.anchorRect
  const gap = 4
  const maxTop = viewportHeight.value - height - viewportPadding
  const top = anchor.bottom + gap <= maxTop
    ? anchor.bottom + gap
    : anchor.top - height - gap
  const left = viewportWidth.value < 768
    ? anchor.left + anchor.width / 2 - width / 2
    : anchor.right - width

  menuPosition.value.top = Math.max(viewportPadding, Math.min(top, maxTop))
  menuPosition.value.left = Math.max(viewportPadding, Math.min(left, viewportWidth.value - width - viewportPadding))
}

// Measure after rendering; menu items and translated labels can change its size.
watch([menuRef, () => props.anchorRect, viewportWidth, viewportHeight], updatePosition, { flush: 'post' })
useResizeObserver(menuRef, updatePosition)

const canDuplicate = computed(() => {
  if (!props.account || props.account.parent_account_id != null) return false
  return ['apikey', 'upstream', 'bedrock', 'service_account'].includes(props.account.type)
})
const isRateLimited = computed(() => {
  if (props.account?.rate_limit_reset_at && new Date(props.account.rate_limit_reset_at) > new Date()) {
    return true
  }
  const modelLimits = (props.account?.extra as Record<string, unknown> | undefined)?.model_rate_limits as
    | Record<string, { rate_limit_reset_at: string }>
    | undefined
  if (modelLimits) {
    const now = new Date()
    return Object.values(modelLimits).some(info => new Date(info.rate_limit_reset_at) > now)
  }
  return false
})
const isOverloaded = computed(() => props.account?.overload_until && new Date(props.account.overload_until) > new Date())
const isTempUnschedulable = computed(() => props.account?.temp_unschedulable_until && new Date(props.account.temp_unschedulable_until) > new Date())
const hasRecoverableState = computed(() => {
  return props.account?.status === 'error' || Boolean(isRateLimited.value) || Boolean(isOverloaded.value) || Boolean(isTempUnschedulable.value)
})
const isAntigravityOAuth = computed(() => props.account?.platform === 'antigravity' && props.account?.type === 'oauth')
const isOpenAIOAuth = computed(() => props.account?.platform === 'openai' && props.account?.type === 'oauth')
// 影子账号(链接型,持 parent_account_id)不持凭据、type 不可变,凭据/隐私类操作对其无效。
const isShadow = computed(() => props.account?.parent_account_id != null)
// A "parent" OpenAI OAuth account is one that is NOT itself a shadow (parent_account_id == null)
const isOpenAIOAuthParent = computed(() => isOpenAIOAuth.value && !isShadow.value)
const supportsPrivacy = computed(() => (isAntigravityOAuth.value || isOpenAIOAuth.value) && !isShadow.value)
const hasQuotaLimit = computed(() => {
  return (props.account?.type === 'apikey' || props.account?.type === 'bedrock') && (
    (props.account?.quota_limit ?? 0) > 0 ||
    (props.account?.quota_daily_limit ?? 0) > 0 ||
    (props.account?.quota_weekly_limit ?? 0) > 0
  )
})

const showChangeStateGroup = computed(() =>
  canDuplicate.value || isOpenAIOAuthParent.value || supportsPrivacy.value || hasRecoverableState.value || hasQuotaLimit.value
)
const showReauthGroup = computed(() => {
  if (!props.account) return false
  return (props.account.type === 'oauth' || props.account.type === 'setup-token') && !isShadow.value
})

const getMenuItems = (): HTMLButtonElement[] => {
  if (!menuRef.value) return []
  return Array.from(menuRef.value.querySelectorAll<HTMLButtonElement>('button[role="menuitem"]'))
}

const onMenuKeydown = (event: KeyboardEvent) => {
  const items = getMenuItems()
  if (items.length === 0) return
  const currentIndex = items.indexOf(document.activeElement as HTMLButtonElement)

  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      items[(currentIndex + 1 + items.length) % items.length]?.focus()
      break
    case 'ArrowUp':
      event.preventDefault()
      items[(currentIndex - 1 + items.length) % items.length]?.focus()
      break
    case 'Home':
      event.preventDefault()
      items[0]?.focus()
      break
    case 'End':
      event.preventDefault()
      items[items.length - 1]?.focus()
      break
  }
}

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') emit('close')
}

watch(
  () => props.show,
  (visible) => {
    if (visible) {
      window.addEventListener('keydown', handleKeydown)
      nextTick(() => getMenuItems()[0]?.focus())
    } else {
      window.removeEventListener('keydown', handleKeydown)
    }
  },
  { immediate: true }
)

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
.am-menu {
  position: fixed;
  z-index: 9999;
  width: 236px;
  padding: 4px;
  border: 1px solid var(--popover-border);
  border-radius: var(--r-md);
  background: var(--popover);
  box-shadow: var(--shadow-md);
}

.am-group {
  display: flex;
  flex-direction: column;
}

.am-group + .am-group {
  margin-top: 4px;
  padding-top: 4px;
  border-top: 1px solid var(--border-subtle);
}

.am-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 28px;
  padding: 0 8px;
  border: 0;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  text-align: left;
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}

.am-item:hover,
.am-item:focus-visible {
  background: var(--sidebar-accent);
}

.am-item:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.am-item :deep(svg) {
  flex-shrink: 0;
  color: var(--muted-foreground);
  transition: color var(--motion-hover);
}

.am-item:hover :deep(svg),
.am-item:focus-visible :deep(svg) {
  color: var(--foreground);
}

@media (prefers-reduced-motion: reduce) {
  .am-item,
  .am-item :deep(svg) {
    transition: none;
  }
}
</style>
