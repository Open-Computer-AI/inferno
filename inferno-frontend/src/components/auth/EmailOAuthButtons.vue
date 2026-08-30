<template>
  <div v-if="hasProviders" class="oauth">
    <div v-if="showDivider" class="oauth__rule">
      <span class="oauth__rule-label">{{ t('auth.oauthOrContinue') }}</span>
    </div>

    <button
      v-for="provider in visibleProviders"
      :key="provider"
      type="button"
      class="oauth__btn"
      :disabled="disabled"
      @click="startLogin(provider)"
    >
      <GoogleMark v-if="provider === 'google'" class="oauth__mark" />
      <GitHubMark v-else class="oauth__mark" />
      <span>{{ providerLabel(provider) }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import GitHubMark from './GitHubMark.vue'
import GoogleMark from './GoogleMark.vue'
import type { OAuthLoginStart } from '@/api/auth'
import { resolveAffiliateReferralCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'

type EmailOAuthProvider = 'github' | 'google'
const EMAIL_OAUTH_PENDING_PROVIDER_KEY = 'email_oauth_pending_provider'

const props = withDefaults(defineProps<{
  disabled?: boolean
  affCode?: string
  promoCode?: string
  githubEnabled?: boolean
  googleEnabled?: boolean
  showDivider?: boolean
}>(), {
  showDivider: true
})
const emit = defineEmits<{
  start: [request: OAuthLoginStart]
}>()

const route = useRoute()
const { t } = useI18n()

const visibleProviders = computed<EmailOAuthProvider[]>(() => {
  // Part 17 orders these "Google, GitHub, then email".
  const providers: EmailOAuthProvider[] = []
  if (props.googleEnabled) providers.push('google')
  if (props.githubEnabled) providers.push('github')
  return providers
})

const hasProviders = computed(() => visibleProviders.value.length > 0)
/**
 * Always the full "Continue with X", never a bare provider name.
 *
 * This used to shorten to "GitHub" / "Google" whenever both were enabled, which
 * made sense when they sat side by side in a two-column grid. They are stacked
 * rows now, and part 17 labels them "Continue with Google" / "Continue with
 * GitHub" in exactly that case.
 */
function providerLabel(provider: EmailOAuthProvider): string {
  const name = provider === 'github' ? 'GitHub' : 'Google'
  return t('auth.emailOAuth.signIn', { providerName: name })
}

function startLogin(provider: EmailOAuthProvider): void {
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  const affiliateCode = resolveAffiliateReferralCode(props.affCode, route.query.aff, route.query.aff_code)
  storeOAuthAffiliateCode(affiliateCode)
  window.sessionStorage.setItem(EMAIL_OAUTH_PENDING_PROVIDER_KEY, provider)
  const params: Record<string, string> = { redirect: redirectTo }
  if (affiliateCode) {
    params.aff_code = affiliateCode
  }
  const promoCode = props.promoCode?.trim()
  if (promoCode) {
    params.promo_code = promoCode
  }
  emit('start', { provider, params })
}
</script>

<style scoped>
/* Measured from part 17's mock: 38px pill, hairline on --card, 9px between the
   mark and its label, label at the body size in regular weight. */
.oauth {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.oauth__btn {
  display: flex;
  height: var(--s2a-h-auth);
  width: 100%;
  align-items: center;
  justify-content: center;
  gap: 9px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--card);
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-lg);
  font-weight: 400;
  cursor: pointer;
  /* Background only, never border-color (ground rule 6). */
  transition: background var(--motion-hover);
}

.oauth__btn:hover:not(:disabled) {
  background: var(--sidebar-accent);
}

.oauth__btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.oauth__btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

/* A provider mark is fixed chrome and must not scale with --font-scale. */
.oauth__mark {
  height: 17px;
  width: 17px;
  flex: none;
}

.oauth__rule {
  position: relative;
  display: flex;
  justify-content: center;
  margin: 6px 0 2px;
}

.oauth__rule::before {
  position: absolute;
  top: 50%;
  right: 0;
  left: 0;
  height: 1px;
  background: color-mix(in oklch, var(--border) 55%, transparent);
  content: '';
}

.oauth__rule-label {
  position: relative;
  padding: 0 10px;
  background: var(--background);
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}
</style>
