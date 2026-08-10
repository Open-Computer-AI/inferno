<template>
  <AuthLayout>
    <template #subtitle>{{ t('auth.forgotPasswordTitle') }}</template>

    <!-- Sent. Deliberately does not confirm the address exists: saying "we sent
         it" only for real accounts is an enumeration hole, so the message is
         the same either way. -->
    <div v-if="isSubmitted" class="auth-form">
      <p class="auth-note fp__sent">{{ t('auth.resetEmailSentHint') }}</p>
      <router-link to="/login" class="auth-back">{{ t('auth.backToLogin') }}</router-link>
    </div>

    <form v-else class="auth-form" @submit.prevent="handleSubmit">
      <p class="auth-note fp__hint">{{ t('auth.forgotPasswordHint') }}</p>

      <input
        id="email"
        v-model="formData.email"
        type="email"
        required
        autofocus
        autocomplete="email"
        class="auth-field"
        :class="{ 'auth-field--invalid': errors.email }"
        :disabled="isLoading"
        :placeholder="t('auth.emailPlaceholder')"
      />

      <div v-if="captchaEnabled" class="fp__captcha">
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

      <button
        type="submit"
        class="auth-cta"
        :disabled="isLoading || (turnstileEnabled && !turnstileToken)"
      >
        <span v-if="isLoading" class="auth-spinner" aria-hidden="true" />
        {{ isLoading ? t('auth.sendingResetLink') : t('auth.sendResetLink') }}
      </button>

      <router-link to="/login" class="auth-back">{{ t('auth.backToLogin') }}</router-link>
    </form>

    <template #footer>
      <p class="auth-note">
        {{ t('auth.rememberedPassword') }}
        <router-link to="/login" class="auth-link">{{ t('auth.signIn') }}</router-link>
      </p>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, ref, reactive, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import TurnstileWidget from '@/components/CaptchaChallenge.vue'
import { useAppStore } from '@/stores'
import { getPublicSettings, forgotPassword } from '@/api/auth'

const { t } = useI18n()

// ==================== Stores ====================

const appStore = useAppStore()

// ==================== State ====================

const isLoading = ref<boolean>(false)
const isSubmitted = ref<boolean>(false)
const errorMessage = ref<string>('')

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
// 动作触发式验证码（腾讯/阿里云）：提交时弹窗验证
const actionCaptchaEnabled = computed(
  () =>
    (tencentCaptchaEnabled.value && Boolean(tencentCaptchaAppId.value)) ||
    aliyunCaptchaReady.value
)
const captchaEnabled = computed(
  () =>
    (turnstileEnabled.value && Boolean(turnstileSiteKey.value)) || actionCaptchaEnabled.value
)

const formData = reactive({
  email: ''
})

const errors = reactive({
  email: '',
  turnstile: ''
})

const validationToastMessage = computed(() => errors.email || errors.turnstile || '')

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

// ==================== Lifecycle ====================

onMounted(async () => {
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
  } catch (error) {
    console.error('Failed to load public settings:', error)
  }
})

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
  errors.email = ''
  errors.turnstile = ''

  let isValid = true

  // Email validation
  if (!formData.email.trim()) {
    errors.email = t('auth.emailRequired')
    isValid = false
  } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
    errors.email = t('auth.invalidEmail')
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

async function handleSubmit(): Promise<void> {
  errorMessage.value = ''

  if (!validateForm()) {
    return
  }

  if (!(await acquireActionProof())) {
    return
  }

  isLoading.value = true

  try {
    await forgotPassword({
      email: formData.email,
      turnstile_token:
        turnstileEnabled.value || aliyunCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_ticket: tencentCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_randstr: tencentCaptchaEnabled.value ? tencentCaptchaRandstr.value : undefined
    })

    isSubmitted.value = true
    appStore.showSuccess(t('auth.resetEmailSent'))
  } catch (error: unknown) {
    const err = error as { message?: string; response?: { data?: { detail?: string } } }

    if (err.response?.data?.detail) {
      errorMessage.value = err.response.data.detail
    } else if (err.message) {
      errorMessage.value = err.message
    } else {
      errorMessage.value = t('auth.sendResetLinkFailed')
    }

    appStore.showError(errorMessage.value)
  } finally {
    if (captchaEnabled.value) {
      resetCaptchaProof()
    }
    isLoading.value = false
  }
}
</script>

<style scoped>
/* Sits above the field it explains, so the gap below it is the form's own 8px. */
.fp__hint,
.fp__sent {
  margin-bottom: 6px;
  text-align: center;
}

.fp__captcha {
  display: flex;
  justify-content: center;
}
</style>
