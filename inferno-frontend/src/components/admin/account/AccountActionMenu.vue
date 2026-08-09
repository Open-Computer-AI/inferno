<template>
  <Teleport to="body">
    <div v-if="show && position">
      <!-- Backdrop: click anywhere outside to close. This popover is
           controlled by a `position` prop computed from a trigger this
           component does not render (AccountsView.vue owns the row's "more"
           button), so it cannot reuse Select.vue's own click-outside
           listener, which is anchored to a local trigger ref. The full
           viewport backdrop is the equivalent for an externally-positioned
           popover. -->
      <div class="am-backdrop" @click="emit('close')"></div>
      <div
        ref="menuRef"
        class="am-menu"
        :style="{ top: position.top + 'px', left: position.left + 'px' }"
        role="menu"
        :aria-label="t('admin.accounts.moreActions')"
        @click.stop
        @mousedown.stop
        @keydown="onMenuKeydown"
      >
        <template v-if="account">
          <!-- Group 1, inspect: read-only / diagnostic actions, always available. -->
          <div class="am-group">
            <button type="button" role="menuitem" class="am-item" @click="$emit('test', account); $emit('close')">
              <i class="hgi-stroke hgi-play" aria-hidden="true" />
              <span>{{ t('admin.accounts.testConnection') }}</span>
            </button>
            <button type="button" role="menuitem" class="am-item" @click="$emit('stats', account); $emit('close')">
              <i class="hgi-stroke hgi-analytics-01" aria-hidden="true" />
              <span>{{ t('admin.accounts.viewStats') }}</span>
            </button>
            <button type="button" role="menuitem" class="am-item" @click="$emit('schedule', account); $emit('close')">
              <i class="hgi-stroke hgi-clock-01" aria-hidden="true" />
              <span>{{ t('admin.scheduledTests.schedule') }}</span>
            </button>
          </div>

          <!-- Group 2, change state: config/lifecycle actions. Hairline only
               renders between two groups that both actually render an item,
               via the adjacent-sibling rule below -- no group ever shows an
               empty divider. -->
          <div v-if="showChangeStateGroup" class="am-group">
            <button v-if="canDuplicate" type="button" role="menuitem" class="am-item" @click="$emit('duplicate', account); $emit('close')">
              <i class="hgi-stroke hgi-copy-01" aria-hidden="true" />
              <span>{{ t('admin.accounts.duplicateAccount') }}</span>
            </button>
            <button v-if="isOpenAIOAuthParent" type="button" role="menuitem" class="am-item" @click="$emit('create-spark-shadow', account); $emit('close')">
              <i class="hgi-stroke hgi-sparkles" aria-hidden="true" />
              <span>{{ t('admin.accounts.createSparkShadow') }}</span>
            </button>
            <button v-if="supportsPrivacy" type="button" role="menuitem" class="am-item" @click="$emit('set-privacy', account); $emit('close')">
              <i class="hgi-stroke hgi-shield-01" aria-hidden="true" />
              <span>{{ t('admin.accounts.setPrivacy') }}</span>
            </button>
            <button v-if="hasRecoverableState" type="button" role="menuitem" class="am-item" @click="$emit('recover-state', account); $emit('close')">
              <i class="hgi-stroke hgi-arrow-reload-horizontal" aria-hidden="true" />
              <span>{{ t('admin.accounts.recoverState') }}</span>
            </button>
            <button v-if="hasQuotaLimit" type="button" role="menuitem" class="am-item" @click="$emit('reset-quota', account); $emit('close')">
              <i class="hgi-stroke hgi-refresh-01" aria-hidden="true" />
              <span>{{ t('admin.accounts.resetQuota') }}</span>
            </button>
          </div>

          <!-- Group 3, re-auth: credential actions. 影子账号不持凭据:重授权/刷新
               token 对其无效(后端拒绝),故隐藏(外审 G4)。 -->
          <div v-if="showReauthGroup" class="am-group">
            <button type="button" role="menuitem" class="am-item" @click="$emit('reauth', account); $emit('close')">
              <i class="hgi-stroke hgi-link-01" aria-hidden="true" />
              <span>{{ t('admin.accounts.reAuthorize') }}</span>
            </button>
            <button type="button" role="menuitem" class="am-item" @click="$emit('refresh-token', account); $emit('close')">
              <i class="hgi-stroke hgi-refresh" aria-hidden="true" />
              <span>{{ t('admin.accounts.refreshToken') }}</span>
            </button>
          </div>
        </template>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
/**
 * AccountActionMenu -- part 04, section 05 "Row actions menu".
 *
 * Migration note from the prototype:
 *   "Replaces the expandable actions column, which is where three of the
 *    four sticky gradients came from. The nine actions and their handlers
 *    are unchanged; only the container changes."
 *
 * This is a presentation rewrite: `show` / `account` / `position` and every
 * emit are unchanged. The prototype's own anatomy mock uses illustrative
 * labels ("View stats", "Start cooldown", "Delete account" ...) that do not
 * match this component's real ten conditional actions; the real actions are
 * kept, grouped by the same idea the mock states in prose -- "inspect first,
 * then the things that change state, then re-auth, then destroy" -- rather
 * than copying the mock's placeholder rows verbatim. There is no destructive
 * action in this menu's real API (account deletion is a separate inline
 * button in AccountsView.vue, out of this file's scope), so the fourth
 * "destroy" group and its --destructive styling do not appear here; see the
 * report for detail.
 *
 * This component does not render the 28px `hgi-more-horizontal` trigger the
 * prototype anatomy describes -- AccountsView.vue computes `position` from a
 * trigger it owns and passes it in as a prop, so the trigger's
 * aria-haspopup/aria-expanded and "return focus on close" are that file's
 * responsibility, not this one's. Documented as a gap in the report rather
 * than silently left out.
 */
import { computed, nextTick, ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'

const props = defineProps<{ show: boolean; account: Account | null; position: { top: number; left: number } | null }>()
const emit = defineEmits(['close', 'test', 'stats', 'schedule', 'duplicate', 'reauth', 'refresh-token', 'recover-state', 'reset-quota', 'set-privacy', 'create-spark-shadow'])
const { t } = useI18n()
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

// Group visibility: a group renders (and can therefore start a hairline)
// only when at least one of its items would render.
const showChangeStateGroup = computed(() =>
  canDuplicate.value || isOpenAIOAuthParent.value || supportsPrivacy.value || hasRecoverableState.value || hasQuotaLimit.value
)
const showReauthGroup = computed(() => {
  if (!props.account) return false
  return (props.account.type === 'oauth' || props.account.type === 'setup-token') && !isShadow.value
})

const menuRef = ref<HTMLElement | null>(null)

const getMenuItems = (): HTMLButtonElement[] => {
  if (!menuRef.value) return []
  return Array.from(menuRef.value.querySelectorAll<HTMLButtonElement>('button[role="menuitem"]'))
}

// Arrow-key roving focus across whatever items are currently rendered
// (conditional per account), rather than a fixed index list that would
// drift out of sync with the v-if groups above.
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
      // Focus moves to the first item on open, not the backdrop or nothing.
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
.am-backdrop {
  position: fixed;
  inset: 0;
  z-index: 9998;
}

/* Popover chrome matches Select.vue's teleported dropdown: --popover fill,
   1px --popover-border, one --shadow-md, --r-md radius. 236px fixed width
   per the anatomy spec -- items never set their own width, so the menu
   cannot jitter row to row. */
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

/* Hairline between groups, never a heading -- and only between two groups
   that both actually rendered, since an empty group never mounts its
   wrapper div. */
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
  height: 28px;
  padding: 0 8px;
  border: 0;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  text-align: left;
  cursor: pointer;
  /* Background only (ground rule 6); colour is not spent on a hover state. */
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

.am-item i {
  flex-shrink: 0;
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
  transition: color var(--motion-hover);
}

.am-item:hover i,
.am-item:focus-visible i {
  color: var(--foreground);
}

@media (prefers-reduced-motion: reduce) {
  .am-item,
  .am-item i {
    transition: none;
  }
}
</style>
