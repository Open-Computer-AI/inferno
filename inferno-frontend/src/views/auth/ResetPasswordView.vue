<template>
  <AuthLayout>
    <template #subtitle>{{ t('auth.resetPasswordTitle') }}</template>

    <!-- Dead link. Stated plainly rather than in a coloured alert panel: the
         recovery is one action, so the action is what the screen shows. -->
    <div v-if="isInvalidLink" class="auth-form">
      <p class="auth-note rp__lead">{{ t('auth.invalidResetLinkHint') }}</p>
      <router-link to="/forgot-password" class="auth-cta rp__cta-link">
        {{ t('auth.requestNewResetLink') }}
      </router-link>
    </div>

    <div v-else-if="isSuccess" class="auth-form">
      <p class="auth-note rp__lead">{{ t('auth.passwordResetSuccessHint') }}</p>
      <router-link to="/login" class="auth-cta rp__cta-link">{{ t('auth.signIn') }}</router-link>
    </div>

    <form v-else class="auth-form" @submit.prevent="handleSubmit">
      <!-- Read-only and greyed: you can see which account you are resetting
           without being able to change it here. -->
      <input
        id="email"
        :value="email"
        type="email"
        readonly
        class="auth-field rp__readonly"
        :aria-label="t('auth.emailLabel')"
      />

      <div class="rp__pw">
        <input
          id="password"
          v-model="formData.password"
          :type="showPassword ? 'text' : 'password'"
          required
          autocomplete="new-password"
          class="auth-field"
          :class="{ 'auth-field--invalid': errors.password }"
          :disabled="isLoading"
          :aria-label="t('auth.newPassword')"
          :placeholder="t('auth.newPasswordPlaceholder')"
        />
        <button
          type="button"
          class="rp__reveal"
          :disabled="isLoading"
          :aria-label="showPassword ? t('auth.hidePassword') : t('auth.showPassword')"
          @click="showPassword = !showPassword"
        >
          <Icon v-if="showPassword" name="eyeOff" size="md" />
          <Icon v-else name="eye" size="md" />
        </button>
      </div>

      <div class="rp__pw">
        <input
          id="confirmPassword"
          v-model="formData.confirmPassword"
          :type="showConfirmPassword ? 'text' : 'password'"
          required
          autocomplete="new-password"
          class="auth-field"
          :class="{ 'auth-field--invalid': errors.confirmPassword }"
          :disabled="isLoading"
          :aria-label="t('auth.confirmPassword')"
          :placeholder="t('auth.confirmPasswordPlaceholder')"
        />
        <button
          type="button"
          class="rp__reveal"
          :disabled="isLoading"
          :aria-label="showConfirmPassword ? t('auth.hidePassword') : t('auth.showPassword')"
          @click="showConfirmPassword = !showConfirmPassword"
        >
          <Icon v-if="showConfirmPassword" name="eyeOff" size="md" />
          <Icon v-else name="eye" size="md" />
        </button>
      </div>

      <button type="submit" class="auth-cta" :disabled="isLoading">
        <span v-if="isLoading" class="auth-spinner" aria-hidden="true" />
        {{ isLoading ? t('auth.resettingPassword') : t('auth.resetPassword') }}
      </button>
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
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { resetPassword } from '@/api/auth'

const { t } = useI18n()

// ==================== Router & Stores ====================

const route = useRoute()
const appStore = useAppStore()

// ==================== State ====================

const isLoading = ref<boolean>(false)
const isSuccess = ref<boolean>(false)
const errorMessage = ref<string>('')
const showPassword = ref<boolean>(false)
const showConfirmPassword = ref<boolean>(false)

// URL parameters
const email = ref<string>('')
const token = ref<string>('')

const formData = reactive({
  password: '',
  confirmPassword: ''
})

const errors = reactive({
  password: '',
  confirmPassword: ''
})

const validationToastMessage = computed(
  () => errors.password || errors.confirmPassword || ''
)

watch(validationToastMessage, (value, previousValue) => {
  if (value && value !== previousValue) {
    appStore.showError(value)
  }
})

// Check if the reset link is valid (has email and token)
const isInvalidLink = computed(() => !email.value || !token.value)

// ==================== Lifecycle ====================

onMounted(() => {
  // Get email and token from URL query parameters
  email.value = (route.query.email as string) || ''
  token.value = (route.query.token as string) || ''

  if (!email.value || !token.value) {
    appStore.showError(t('auth.invalidResetLink'))
  }
})

// ==================== Validation ====================

function validateForm(): boolean {
  errors.password = ''
  errors.confirmPassword = ''

  let isValid = true

  // Password validation
  if (!formData.password) {
    errors.password = t('auth.passwordRequired')
    isValid = false
  } else if (formData.password.length < 6) {
    errors.password = t('auth.passwordMinLength')
    isValid = false
  }

  // Confirm password validation
  if (!formData.confirmPassword) {
    errors.confirmPassword = t('auth.confirmPasswordRequired')
    isValid = false
  } else if (formData.password !== formData.confirmPassword) {
    errors.confirmPassword = t('auth.passwordsDoNotMatch')
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

  isLoading.value = true

  try {
    await resetPassword({
      email: email.value,
      token: token.value,
      new_password: formData.password
    })

    isSuccess.value = true
    appStore.showSuccess(t('auth.passwordResetSuccess'))
  } catch (error: unknown) {
    const err = error as { message?: string; response?: { data?: { detail?: string; code?: string } } }

    // Check for invalid/expired token error
    if (err.response?.data?.code === 'INVALID_RESET_TOKEN') {
      errorMessage.value = t('auth.invalidOrExpiredToken')
    } else if (err.response?.data?.detail) {
      errorMessage.value = err.response.data.detail
    } else if (err.message) {
      errorMessage.value = err.message
    } else {
      errorMessage.value = t('auth.resetPasswordFailed')
    }

    appStore.showError(errorMessage.value)
  } finally {
    isLoading.value = false
  }
}
</script>

<style scoped>
.rp__lead {
  margin-bottom: 6px;
  text-align: center;
}

/* A link wearing the CTA: needs the line-height reset a button gets for free. */
.rp__cta-link {
  text-decoration: none;
}

/* Real data you cannot edit. Distinct from disabled on purpose -- disabled
   means "not now", read-only means "not here". */
.rp__readonly {
  background: var(--surface-subtle);
  border-color: var(--border-subtle);
  color: var(--muted-foreground);
}

.rp__pw {
  position: relative;
}

.rp__pw .auth-field {
  padding-right: 44px;
}

.rp__reveal {
  position: absolute;
  top: 0;
  right: 0;
  display: flex;
  height: var(--s2a-h-auth);
  width: 42px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: var(--muted-foreground);
  cursor: pointer;
  transition: color var(--motion-hover);
}

.rp__reveal:hover:not(:disabled) {
  color: var(--foreground);
}

.rp__reveal:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
</style>
