<template>
  <AuthLayout>
    <template #subtitle>{{ t('auth.signInToAccount') }}</template>

    <form class="lg" @submit.prevent="onSubmit">
      <!-- Step one: the providers, then a rule, then the address. Part 17 orders
           it "Google, GitHub, then email" -- three methods fit above the fold
           with room, so nothing is hidden and nothing has to be remembered. -->
      <template v-if="step === 1">
        <EmailOAuthButtons
          :disabled="authActionDisabled"
          :github-enabled="githubOAuthEnabled"
          :google-enabled="googleOAuthEnabled"
          :show-divider="false"
          @start="handleOAuthStart"
        />

        <button
          v-if="showPasskeyLogin"
          type="button"
          class="lg__provider"
          :disabled="authActionDisabled"
          @click="handlePasskeyLogin"
        >
          <Icon name="key" size="md" />
          <span>{{ passkeyLoading ? t('auth.passkeySigningIn') : t('auth.passkeySignIn') }}</span>
        </button>

        <LinuxDoOAuthSection v-if="linuxdoOAuthEnabled" :disabled="authActionDisabled" :show-divider="false" @start="handleOAuthStart" />
        <DingTalkOAuthSection v-if="dingtalkOAuthEnabled" :disabled="authActionDisabled" :show-divider="false" @start="handleOAuthStart" />
        <WechatOAuthSection v-if="wechatOAuthEnabled" :disabled="authActionDisabled" :show-divider="false" @start="handleOAuthStart" />
        <OidcOAuthSection v-if="oidcOAuthEnabled" :disabled="authActionDisabled" :provider-name="oidcOAuthProviderName" :show-divider="false" @start="handleOAuthStart" />

        <div v-if="showPasskeyLogin || showOAuthLogin" class="lg__rule">
          <span class="lg__rule-label">{{ t('auth.or') }}</span>
        </div>

        <input
          id="email"
          v-model="formData.email"
          type="email"
          required
          autofocus
          autocomplete="email"
          class="lg__field"
          :class="{ 'lg__field--invalid': errors.email }"
          :disabled="authActionDisabled"
          :placeholder="t('auth.emailPlaceholder')"
        />

        <button type="submit" class="lg__cta" :disabled="authActionDisabled || !emailLooksValid">
          {{ t('auth.continue') }}
        </button>
      </template>

      <!-- Step two. The address stays visible and ticked: you can see what you
           are signing in as without going back, which is the whole failure of a
           two step form done badly. -->
      <template v-else>
        <div class="lg__identity">
          <i class="hgi-stroke hgi-tick-02" aria-hidden="true" />
          <span class="lg__identity-email">{{ formData.email }}</span>
        </div>

        <div class="lg__pw">
          <input
            id="password"
            ref="passwordFieldRef"
            v-model="formData.password"
            :type="showPassword ? 'text' : 'password'"
            required
            autocomplete="current-password"
            class="lg__field"
            :class="{ 'lg__field--invalid': errors.password }"
            :disabled="authActionDisabled"
            :placeholder="t('auth.passwordPlaceholder')"
          />
          <button
            type="button"
            class="lg__reveal"
            :disabled="authActionDisabled"
            :aria-label="showPassword ? t('auth.hidePassword') : t('auth.showPassword')"
            @click="showPassword = !showPassword"
          >
            <Icon v-if="showPassword" name="eyeOff" size="md" />
            <Icon v-else name="eye" size="md" />
          </button>
        </div>



        <button
          type="submit"
          class="lg__cta"
          :disabled="authActionDisabled || (turnstileEnabled && !turnstileToken)"
        >
          <span v-if="isLoading" class="lg__spinner" aria-hidden="true" />
          {{ isLoading ? t('auth.signingIn') : t('auth.signIn') }}
        </button>

        <!-- Under the button, always. Two step forms trap people when the only
             way back is the browser button, and a trapped person retypes an
             email they already typed. -->
        <button type="button" class="lg__back" @click="goBackToEmail">
          {{ t('auth.goBack') }}
        </button>

        <router-link
          v-if="passwordResetEnabled && !backendModeEnabled"
          to="/forgot-password"
          class="lg__back lg__back--link"
        >
          {{ t('auth.forgotPassword') }}
        </router-link>
      </template>

      <!-- Mounted for BOTH steps, deliberately. `handleOAuthStart` and the
           passkey handler both call `turnstileRef.verifyAction()`, and those
           buttons live on step one -- when this was inside the step-two branch
           the ref was null there, the optional chain returned undefined, and
           OAuth sign-in silently did nothing whenever the action gate was on. -->
      <div v-if="captchaEnabled" class="lg__captcha">
        <TurnstileWidget
          ref="turnstileRef"
          :turnstile-enabled="turnstileEnabled"
          :turnstile-site-key="turnstileSiteKey"
          :tencent-enabled="tencentCaptchaEnabled"
          :tencent-app-id="tencentCaptchaAppId"
          :tencent-region="tencentCaptchaRegion"
          :aliyun-enabled="aliyunCaptchaEnabled"
          :aliyun-scene-id="aliyunCaptchaSceneId"
          :aliyun-prefix="aliyunCaptchaPrefix"
          :aliyun-region="aliyunCaptchaRegion"
          @verify="onTurnstileVerify"
          @expire="onTurnstileExpire"
          @error="onTurnstileError"
        />
      </div>

      <LoginAgreementPrompt
        v-if="loginAgreementEnabled"
        :accepted="agreementAccepted"
        :documents="loginAgreementDocuments"
        :mode="loginAgreementMode"
        :updated-at="loginAgreementUpdatedAt"
        :visible="showAgreementModal"
        @accept="acceptLoginAgreement"
        @reject="rejectLoginAgreement"
        @open="showAgreementModal = true"
      />
    </form>

    <template #footer>
      <!-- Only rendered when the operator has actually configured the documents.
           A hard-coded /terms link would be a dead link in most deployments. -->
      <p class="lg__legal">
        {{ t('auth.legalPrefix') }}&nbsp;
        <template v-for="(doc, i) in legalDocuments" :key="doc.to">
          <router-link :to="doc.to" class="lg__legal-link">{{ doc.title }}</router-link>
          <span v-if="i < legalDocuments.length - 1">&nbsp;{{ t('auth.legalAnd') }}&nbsp;</span>
        </template>
      </p>

      <p v-if="!backendModeEnabled" class="lg__signup">
        {{ t('auth.dontHaveAccount') }}
        <router-link to="/register" class="lg__signup-link">{{ t('auth.signUp') }}</router-link>
      </p>
    </template>
  </AuthLayout>

  <!-- 2FA Modal -->
  <TotpLoginModal
    v-if="show2FAModal"
    ref="totpModalRef"
    :temp-token="totpTempToken"
    :user-email-masked="totpUserEmailMasked"
    @verify="handle2FAVerify"
    @cancel="handle2FACancel"
  />
</template>

<script setup lang="ts">
import { computed, ref, reactive, onMounted, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import LinuxDoOAuthSection from '@/components/auth/LinuxDoOAuthSection.vue'
import DingTalkOAuthSection from '@/components/auth/DingTalkOAuthSection.vue'
import OidcOAuthSection from '@/components/auth/OidcOAuthSection.vue'
import WechatOAuthSection from '@/components/auth/WechatOAuthSection.vue'
import EmailOAuthButtons from '@/components/auth/EmailOAuthButtons.vue'
import LoginAgreementPrompt from '@/components/auth/LoginAgreementPrompt.vue'
import TotpLoginModal from '@/components/auth/TotpLoginModal.vue'
import Icon from '@/components/icons/Icon.vue'
import TurnstileWidget from '@/components/CaptchaChallenge.vue'
import { useAuthStore, useAppStore } from '@/stores'
import {
  buildOAuthLoginStartURL,
  getPublicSettings,
  isTotp2FARequired,
  isWeChatWebOAuthEnabled,
  startOAuthLogin,
  type OAuthLoginStart
} from '@/api/auth'
import type {
  ActionCaptchaRequestProof,
  LoginAgreementDocument,
  TotpLoginResponse
} from '@/types'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { clearAllAffiliateReferralCodes } from '@/utils/oauthAffiliate'

const { t } = useI18n()
const LOGIN_AGREEMENT_STORAGE_KEY = 'sub2api_login_agreement_consent'

// ==================== Router & Stores ====================

const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()

// ==================== State ====================

const isLoading = ref<boolean>(false)
const passkeyLoading = ref<boolean>(false)
const errorMessage = ref<string>('')
const showPassword = ref<boolean>(false)

/**
 * Two steps, not one: email first, password second.
 *
 * Part 17 specifies this so the screen "learns who you are before deciding what
 * to ask". The full benefit -- a GitHub user never seeing a password field --
 * needs an identity lookup the backend does not expose, and an endpoint that
 * reveals whether an address is registered is an account-enumeration hole. So
 * step one never asks the server anything: it collects the address and advances
 * unconditionally, which leaks nothing and still gives the shape the design
 * wants (the address stays visible, and going back is a real control).
 */
const step = ref<1 | 2>(1)
const passwordFieldRef = ref<HTMLInputElement | null>(null)

const emailLooksValid = computed(() => /.+@.+\..+/.test(formData.email.trim()))

async function onSubmit(): Promise<void> {
  if (step.value === 1) {
    if (!emailLooksValid.value) return
    step.value = 2
    // Focus follows the step, or a keyboard user lands nowhere.
    await nextTick()
    passwordFieldRef.value?.focus()
    return
  }
  await handleLogin()
}

function goBackToEmail(): void {
  step.value = 1
  formData.password = ''
  errors.password = ''
  errorMessage.value = ''
}

/**
 * The legal line's two links, always these two.
 *
 * NOT the operator's agreement documents: those belong to the compliance
 * acceptance dialog, are seeded by the backend with Chinese titles, and there
 * can be four of them -- wiring this line to them put "服务条款and使用政策and…"
 * under an English sign-in form. Part 17 shows exactly two links here.
 */
const legalDocuments = computed<{ to: string; title: string }[]>(() => [
  { to: '/legal/terms', title: t('auth.legalTerms') },
  { to: '/legal/privacy', title: t('auth.legalPrivacy') }
])

const publicSettingsLoaded = ref<boolean>(false)

// Public settings
const turnstileEnabled = ref<boolean>(false)
const turnstileSiteKey = ref<string>('')
const tencentCaptchaEnabled = ref<boolean>(false)
const tencentCaptchaAppId = ref<string>('')
const tencentCaptchaRegion = ref<string>('cn')
const aliyunCaptchaEnabled = ref<boolean>(false)
const aliyunCaptchaSceneId = ref<string>('')
const aliyunCaptchaPrefix = ref<string>('')
const aliyunCaptchaRegion = ref<string>('cn')
const linuxdoOAuthEnabled = ref<boolean>(false)
const dingtalkOAuthEnabled = ref<boolean>(false)
const wechatOAuthEnabled = ref<boolean>(false)
const backendModeEnabled = ref<boolean>(false)
const oidcOAuthEnabled = ref<boolean>(false)
const oidcOAuthProviderName = ref<string>('OIDC')
const githubOAuthEnabled = ref<boolean>(false)
const googleOAuthEnabled = ref<boolean>(false)
const passwordResetEnabled = ref<boolean>(false)
const passkeyEnabled = ref<boolean>(false)
const loginAgreementEnabled = ref<boolean>(false)
const loginAgreementMode = ref<'modal' | 'checkbox' | string>('modal')
const loginAgreementUpdatedAt = ref<string>('')
const loginAgreementRevision = ref<string>('')
const loginAgreementDocuments = ref<LoginAgreementDocument[]>([])
const agreementAccepted = ref<boolean>(false)
const showAgreementModal = ref<boolean>(false)

// Turnstile
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const turnstileToken = ref<string>('')
const tencentCaptchaRandstr = ref<string>('')
const aliyunCaptchaReady = computed(
  () =>
    aliyunCaptchaEnabled.value &&
    Boolean(aliyunCaptchaSceneId.value) &&
    Boolean(aliyunCaptchaPrefix.value)
)
// 动作触发式验证码（腾讯/阿里云）：提交、OAuth 启动、passkey 时弹窗验证
const actionCaptchaEnabled = computed(
  () =>
    (tencentCaptchaEnabled.value && Boolean(tencentCaptchaAppId.value)) ||
    aliyunCaptchaReady.value
)
const captchaEnabled = computed(
  () =>
    (turnstileEnabled.value && Boolean(turnstileSiteKey.value)) || actionCaptchaEnabled.value
)

// 2FA state
const show2FAModal = ref<boolean>(false)
const totpTempToken = ref<string>('')
const totpUserEmailMasked = ref<string>('')
const totpModalRef = ref<InstanceType<typeof TotpLoginModal> | null>(null)

const formData = reactive({
  email: '',
  password: ''
})

const errors = reactive({
  email: '',
  password: '',
  turnstile: ''
})

const validationToastMessage = computed(
  () => errors.email || errors.password || errors.turnstile || ''
)

const agreementGateActive = computed(
  () => loginAgreementEnabled.value && !agreementAccepted.value
)

const authActionDisabled = computed(
  () => isLoading.value || passkeyLoading.value || !publicSettingsLoaded.value || agreementGateActive.value
)

const showPasskeyLogin = computed(
  () => passkeyEnabled.value && typeof window.PublicKeyCredential !== 'undefined'
)

const showOAuthLogin = computed(
  () =>
    !backendModeEnabled.value &&
    (linuxdoOAuthEnabled.value ||
      dingtalkOAuthEnabled.value ||
      wechatOAuthEnabled.value ||
      oidcOAuthEnabled.value ||
      githubOAuthEnabled.value ||
      googleOAuthEnabled.value)
)

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

// ==================== Lifecycle ====================

onMounted(async () => {
  const expiredFlag = sessionStorage.getItem('auth_expired')
  if (expiredFlag) {
    sessionStorage.removeItem('auth_expired')
    const message = t('auth.reloginRequired')
    errorMessage.value = message
    appStore.showWarning(message)
  }

  try {
    const settings = await getPublicSettings()
    turnstileEnabled.value = settings.turnstile_enabled
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    tencentCaptchaEnabled.value = settings.tencent_captcha_enabled === true
    tencentCaptchaAppId.value = settings.tencent_captcha_app_id || ''
    tencentCaptchaRegion.value = settings.tencent_captcha_region || 'cn'
    aliyunCaptchaEnabled.value = settings.aliyun_captcha_enabled === true
    aliyunCaptchaSceneId.value = settings.aliyun_captcha_scene_id || ''
    aliyunCaptchaPrefix.value = settings.aliyun_captcha_prefix || ''
    aliyunCaptchaRegion.value = settings.aliyun_captcha_region || 'cn'
    linuxdoOAuthEnabled.value = settings.linuxdo_oauth_enabled
    dingtalkOAuthEnabled.value = settings.dingtalk_oauth_enabled ?? false
    wechatOAuthEnabled.value = isWeChatWebOAuthEnabled(settings)
    backendModeEnabled.value = settings.backend_mode_enabled
    oidcOAuthEnabled.value = settings.oidc_oauth_enabled
    oidcOAuthProviderName.value = settings.oidc_oauth_provider_name || 'OIDC'
    githubOAuthEnabled.value = settings.github_oauth_enabled
    googleOAuthEnabled.value = settings.google_oauth_enabled
    backendModeEnabled.value = settings.backend_mode_enabled
    passwordResetEnabled.value = settings.password_reset_enabled
    passkeyEnabled.value = settings.passkey_enabled === true
    applyLoginAgreementSettings(settings)
  } catch (error) {
    console.error('Failed to load public settings:', error)
    loginAgreementEnabled.value = false
    agreementAccepted.value = true
  } finally {
    publicSettingsLoaded.value = true
  }
})

// ==================== Login Agreement ====================

function applyLoginAgreementSettings(settings: {
  login_agreement_enabled?: boolean
  login_agreement_mode?: string
  login_agreement_updated_at?: string
  login_agreement_revision?: string
  login_agreement_documents?: LoginAgreementDocument[]
}): void {
  const documents = Array.isArray(settings.login_agreement_documents)
    ? settings.login_agreement_documents.filter((doc) => doc.title?.trim())
    : []
  loginAgreementDocuments.value = documents
  loginAgreementEnabled.value = settings.login_agreement_enabled === true && documents.length > 0
  loginAgreementMode.value = settings.login_agreement_mode === 'checkbox' ? 'checkbox' : 'modal'
  loginAgreementUpdatedAt.value = settings.login_agreement_updated_at || ''
  loginAgreementRevision.value =
    settings.login_agreement_revision ||
    `${loginAgreementUpdatedAt.value}:${documents.map((doc) => `${doc.id}:${doc.title}`).join('|')}`

  agreementAccepted.value = !loginAgreementEnabled.value || hasAcceptedLoginAgreement(loginAgreementRevision.value)
  showAgreementModal.value =
    loginAgreementEnabled.value && !agreementAccepted.value && loginAgreementMode.value !== 'checkbox'
}

function hasAcceptedLoginAgreement(revision: string): boolean {
  if (!revision) {
    return false
  }
  try {
    const raw = localStorage.getItem(LOGIN_AGREEMENT_STORAGE_KEY)
    if (!raw) {
      return false
    }
    const parsed = JSON.parse(raw) as { revision?: string }
    return parsed.revision === revision
  } catch {
    return false
  }
}

function acceptLoginAgreement(): void {
  if (loginAgreementRevision.value) {
    localStorage.setItem(
      LOGIN_AGREEMENT_STORAGE_KEY,
      JSON.stringify({
        revision: loginAgreementRevision.value,
        accepted_at: new Date().toISOString()
      })
    )
  }
  agreementAccepted.value = true
  showAgreementModal.value = false
}

function rejectLoginAgreement(): void {
  localStorage.removeItem(LOGIN_AGREEMENT_STORAGE_KEY)
  agreementAccepted.value = false
  showAgreementModal.value = false
  appStore.showWarning(t('legal.loginAgreementPrompt.loginRejectedWarning'))
}

// ==================== Turnstile Handlers ====================

function onTurnstileVerify(token: string, randstr = ''): void {
  turnstileToken.value = token
  tencentCaptchaRandstr.value = randstr
  errors.turnstile = ''
}

function onTurnstileExpire(): void {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  errors.turnstile = t('auth.turnstileExpired')
}

function onTurnstileError(): void {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  errors.turnstile = t('auth.turnstileFailed')
}

function resetCaptchaProof(): void {
  turnstileRef.value?.reset()
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  errors.turnstile = ''
}

async function acquireActionProof(): Promise<boolean> {
  if (!actionCaptchaEnabled.value) return true

  const proof = await turnstileRef.value?.verifyAction()
  if (!proof) return false

  turnstileToken.value = proof.token
  tencentCaptchaRandstr.value = proof.randstr
  return true
}

// ==================== Validation ====================

function validateForm(): boolean {
  // Reset errors
  errors.email = ''
  errors.password = ''
  errors.turnstile = ''

  let isValid = true

  if (agreementGateActive.value) {
    appStore.showWarning(t('legal.loginAgreementPrompt.loginRequiredWarning'))
    if (loginAgreementMode.value !== 'checkbox') {
      showAgreementModal.value = true
    }
    return false
  }

  // Email validation
  if (!formData.email.trim()) {
    errors.email = t('auth.emailRequired')
    isValid = false
  } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
    errors.email = t('auth.invalidEmail')
    isValid = false
  }

  // Password validation
  if (!formData.password) {
    errors.password = t('auth.passwordRequired')
    isValid = false
  } else if (formData.password.length < 6) {
    errors.password = t('auth.passwordMinLength')
    isValid = false
  }

  // Turnstile validation
  if (turnstileEnabled.value && !turnstileToken.value) {
    errors.turnstile = t('auth.completeVerification')
    isValid = false
  }

  return isValid
}

// ==================== Form Handlers ====================

async function handleLogin(): Promise<void> {
  // Clear previous error
  errorMessage.value = ''

  // Validate form
  if (!validateForm()) {
    return
  }

  if (!(await acquireActionProof())) {
    return
  }

  isLoading.value = true

  try {
    // Call auth store login（阿里云 captchaVerifyParam 复用 turnstile_token 字段）
    const response = await authStore.login({
      email: formData.email,
      password: formData.password,
      turnstile_token:
        turnstileEnabled.value || aliyunCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_ticket: tencentCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_randstr: tencentCaptchaEnabled.value
        ? tencentCaptchaRandstr.value
        : undefined
    })

    // Check if 2FA is required
    if (isTotp2FARequired(response)) {
      const totpResponse = response as TotpLoginResponse
      totpTempToken.value = totpResponse.temp_token || ''
      totpUserEmailMasked.value = totpResponse.user_email_masked || ''
      show2FAModal.value = true
      isLoading.value = false
      return
    }

    // Show success toast
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(t('auth.loginSuccess'))

    // Redirect to dashboard or intended route
    const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
    await router.push(redirectTo)
  } catch (error: unknown) {
    errorMessage.value = extractI18nErrorMessage(error, t, 'auth.errors', t('auth.loginFailed'))

    // Also show error toast
    appStore.showError(errorMessage.value)
  } finally {
    if (captchaEnabled.value) {
      resetCaptchaProof()
    }
    isLoading.value = false
  }
}

async function handlePasskeyLogin(): Promise<void> {
  if (agreementGateActive.value) {
    appStore.showWarning(t('legal.loginAgreementPrompt.loginRequiredWarning'))
    if (loginAgreementMode.value !== 'checkbox') {
      showAgreementModal.value = true
    }
    return
  }

  passkeyLoading.value = true
  try {
    let proof: ActionCaptchaRequestProof | undefined
    if (actionCaptchaEnabled.value) {
      const result = await turnstileRef.value?.verifyAction()
      if (!result) return
      proof = tencentCaptchaEnabled.value
        ? {
            tencent_captcha_ticket: result.token,
            tencent_captcha_randstr: result.randstr
          }
        : { turnstile_token: result.token }
    }

    await authStore.loginWithPasskey(proof)
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(t('auth.loginSuccess'))
    const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
    await router.push(redirectTo)
  } catch (error: unknown) {
    const fallback = error instanceof DOMException && error.name === 'NotAllowedError'
      ? t('auth.passkeyCancelled')
      : t('auth.passkeyFailed')
    errorMessage.value = extractI18nErrorMessage(error, t, 'auth.errors', fallback)
    appStore.showError(errorMessage.value)
  } finally {
    if (actionCaptchaEnabled.value) {
      resetCaptchaProof()
    }
    passkeyLoading.value = false
  }
}

async function handleOAuthStart(request: OAuthLoginStart): Promise<void> {
  if (authActionDisabled.value) return

  if (!actionCaptchaEnabled.value) {
    window.location.href = buildOAuthLoginStartURL(request)
    return
  }

  isLoading.value = true
  try {
    const proof = await turnstileRef.value?.verifyAction()
    if (!proof) return

    const result = await startOAuthLogin(
      request,
      tencentCaptchaEnabled.value
        ? {
            tencent_captcha_ticket: proof.token,
            tencent_captcha_randstr: proof.randstr
          }
        : { turnstile_token: proof.token }
    )
    window.location.href = result.authorize_url
  } catch (error: unknown) {
    errorMessage.value = extractI18nErrorMessage(
      error,
      t,
      'auth.errors',
      t('auth.turnstileFailed')
    )
    appStore.showError(errorMessage.value)
  } finally {
    resetCaptchaProof()
    isLoading.value = false
  }
}

// ==================== 2FA Handlers ====================

async function handle2FAVerify(code: string): Promise<void> {
  if (totpModalRef.value) {
    totpModalRef.value.setVerifying(true)
  }

  try {
    await authStore.login2FA(totpTempToken.value, code)

    // Close modal and show success
    show2FAModal.value = false
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(t('auth.loginSuccess'))

    // Redirect to dashboard or intended route
    const redirectTo = (router.currentRoute.value.query.redirect as string) || '/dashboard'
    await router.push(redirectTo)
  } catch (error: unknown) {
    const err = error as { message?: string; response?: { data?: { message?: string } } }
    const message = err.response?.data?.message || err.message || t('profile.totp.loginFailed')

    if (totpModalRef.value) {
      totpModalRef.value.setError(message)
      totpModalRef.value.setVerifying(false)
    }
  }
}

function handle2FACancel(): void {
  show2FAModal.value = false
  totpTempToken.value = ''
  totpUserEmailMasked.value = ''
}
</script>

<style scoped>
/* Every number here is measured from part 17's own mock, not interpreted:
   38px controls, fully rounded, 8px between them, the body size throughout. */
.lg {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.lg__provider,
.lg__field,
.lg__cta {
  height: var(--s2a-h-auth);
  width: 100%;
  border-radius: 999px;
  font-family: inherit;
  font-size: var(--fs-lg);
}

.lg__provider {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  border: 1px solid var(--border);
  background: var(--card);
  color: var(--foreground);
  font-weight: 400;
  cursor: pointer;
  transition: background var(--motion-hover);
}
.lg__provider:hover:not(:disabled) {
  background: var(--sidebar-accent);
}

.lg__field {
  padding: 0 17px;
  border: 1px solid var(--input);
  background: var(--card);
  color: var(--foreground);
  transition: background var(--motion-hover);
}
.lg__field::placeholder {
  color: var(--muted-foreground);
}
.lg__field:focus {
  outline: none;
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}
.lg__field--invalid {
  border-color: var(--destructive);
}

/* The only filled control on the screen, and the only one at the medium weight. */
.lg__cta {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 0;
  background: var(--primary);
  color: var(--primary-foreground);
  font-weight: var(--fw-medium);
  cursor: pointer;
  transition: background var(--motion-hover);
}
.lg__cta:hover:not(:disabled) {
  background: color-mix(in oklch, var(--primary) 92%, var(--foreground));
}
.lg__cta:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.lg__provider:focus-visible,
.lg__cta:focus-visible,
.lg__back:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.lg__provider:disabled,
.lg__field:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

/* 6px above and 2px below, so the rule sits closer to the field it introduces
   than to the providers it separates from. */
.lg__rule {
  position: relative;
  display: flex;
  justify-content: center;
  margin: 6px 0 2px;
}
.lg__rule::before {
  position: absolute;
  top: 50%;
  right: 0;
  left: 0;
  height: 1px;
  background: color-mix(in oklch, var(--border) 55%, transparent);
  content: '';
}
.lg__rule-label {
  position: relative;
  padding: 0 10px;
  background: var(--background);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

/* Greyed and ticked: what you are signing in as, without going back for it. */
.lg__identity {
  display: flex;
  height: var(--s2a-h-auth);
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-radius: 999px;
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}
.lg__identity-email {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.lg__pw {
  position: relative;
}

.lg__reveal {
  position: absolute;
  top: 0;
  right: 0;
  display: flex;
  height: var(--s2a-h-auth);
  width: 44px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: var(--muted-foreground);
  cursor: pointer;
  transition: color var(--motion-hover);
}
.lg__reveal:hover:not(:disabled) {
  color: var(--foreground);
}
.lg__pw .lg__field {
  padding-right: 46px;
}

.lg__captcha {
  display: flex;
  justify-content: center;
}

.lg__back {
  border: 0;
  background: transparent;
  color: var(--muted-foreground);
  font-family: inherit;
  font-size: var(--fs-sm);
  cursor: pointer;
  text-align: center;
  transition: color var(--motion-hover);
}
.lg__back:hover {
  color: var(--foreground);
}
.lg__back--link {
  display: block;
  text-decoration: none;
}

.lg__spinner {
  display: inline-block;
  height: 13px;
  width: 13px;
  border: 1.5px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  opacity: 0.7;
  animation: s2a-spin 0.7s linear infinite;
}

.lg__legal,
.lg__signup {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
  line-height: 1.5;
}
.lg__signup {
  margin-top: 10px;
  font-size: var(--fs-md);
}

/* Warm, not the body ink: these are the only links on the screen. */
.lg__legal-link {
  color: var(--warm-strong);
  text-decoration: none;
}
.lg__legal-link:hover {
  text-decoration: underline;
}

.lg__signup-link {
  color: var(--foreground);
  text-decoration: underline;
  text-underline-offset: 2px;
  text-decoration-color: var(--border);
  transition: text-decoration-color var(--motion-hover);
}
.lg__signup-link:hover {
  text-decoration-color: var(--foreground);
}
</style>
