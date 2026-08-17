<template>
  <AppLayout>
    <div class="dvc">
      <PageHeader />

      <section class="dvc__card">
        <template v-if="phase === 'form'">
          <Input
            id="device-user-code"
            ref="codeInputRef"
            v-model="userCode"
            :label="t('device.codeLabel')"
            :placeholder="t('device.codePlaceholder')"
            :error="formError"
            :hint="t('device.codeHint')"
            autocomplete="off"
            :disabled="isSubmitting !== null"
            @keyup.enter="submit('approve')"
          />

          <div class="dvc__actions">
            <Button
              type="button"
              variant="solid"
              size="lg"
              data-test="approve"
              :loading="isSubmitting === 'approve'"
              :disabled="!canSubmit || isSubmitting !== null"
              @click="submit('approve')"
            >
              {{ isSubmitting === 'approve' ? t('device.approving') : t('device.approve') }}
            </Button>
            <Button
              type="button"
              variant="secondary"
              size="lg"
              data-test="deny"
              :loading="isSubmitting === 'deny'"
              :disabled="!canSubmit || isSubmitting !== null"
              @click="submit('deny')"
            >
              {{ isSubmitting === 'deny' ? t('device.denying') : t('device.deny') }}
            </Button>
          </div>
        </template>

        <InterstitialState
          v-else-if="phase === 'approved'"
          tone="attention"
          icon="hgi-tick-02"
          :title="t('device.approvedTitle')"
          :body="t('device.approvedBody')"
        />
        <InterstitialState
          v-else-if="phase === 'denied'"
          tone="attention"
          icon="hgi-cancel-01"
          :title="t('device.deniedTitle')"
          :body="t('device.deniedBody')"
        />
        <InterstitialState
          v-else
          tone="failed"
          icon="hgi-time-04"
          :title="t('device.expiredTitle')"
          :body="t('device.expiredBody')"
        />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * The device approval screen -- the only human-facing step of the RFC 8628
 * device flow. A headless hermes CLI prints verification_uri_complete
 * ("{frontend}/device?user_code=XXXX-XXXX"); the human opens it here, checks
 * the prefilled code matches what their terminal is showing, and approves or
 * denies. Route path is load-bearing: OAuthDeviceService.RequestCode
 * (backend/internal/service/oauth_device_service.go) hardcodes
 * "{portal}/device" as verification_uri, so this view must stay mounted at
 * exactly /device (see router/index.ts).
 *
 * Three outcomes past the form, not one: approve/deny (200) end the flow;
 * an expired code (410) is terminal too, but for a different reason -- the
 * human's only remedy is re-running the CLI command, so it gets its own
 * screen that says so rather than an inline field error. An unknown code
 * (404) is the one case that is NOT terminal: it usually means a mistyped
 * digit, so it stays on the form as an inline error the person can fix and
 * resubmit immediately, the same way a wrong password does on the login
 * screen rather than bouncing to a dead end.
 */
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AppLayout, PageHeader, InterstitialState } from '@/components/layout'
import Button from '@/components/common/Button.vue'
import Input from '@/components/common/Input.vue'
import { approveDevice, denyDevice } from '@/api/oauth'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'

const route = useRoute()
const { t } = useI18n()
const appStore = useAppStore()

const codeInputRef = ref<InstanceType<typeof Input> | null>(null)
// Input.vue's root is a wrapper <div>, so a plain `autofocus` attribute would
// land there instead of the inner <input> -- focus it explicitly instead.
onMounted(() => codeInputRef.value?.focus())

type Phase = 'form' | 'approved' | 'denied' | 'expired'

/**
 * Uppercases, strips anything outside the code alphabet, and reinserts the
 * XXXX-XXXX hyphen at its canonical position. Applied on every keystroke (not
 * only on submit) so a person who pastes a lowercase code, a code with no
 * hyphen, or one with surrounding whitespace sees it snap into the shape
 * they are meant to be comparing against their terminal, rather than
 * discovering a mismatch only after they click Approve. The backend already
 * trims and uppercases on its side (OAuthDeviceService.Approve/Deny) -- this
 * is purely a "does what I'm looking at match what I typed" UX aid.
 */
function formatUserCodeInput(value: string): string {
  const compact = value
    .toUpperCase()
    .replace(/[^A-Z0-9]/g, '')
    .slice(0, 8)
  return compact.length <= 4 ? compact : `${compact.slice(0, 4)}-${compact.slice(4)}`
}

function initialUserCode(): string {
  const raw = route.query.user_code
  const value = Array.isArray(raw) ? raw[0] : raw
  return formatUserCodeInput(value ?? '')
}

const rawUserCode = ref(initialUserCode())
const userCode = computed({
  get: () => rawUserCode.value,
  set: (value: string) => {
    rawUserCode.value = formatUserCodeInput(value)
  }
})

const canSubmit = computed(() => rawUserCode.value.replace(/[^A-Z0-9]/g, '').length === 8)

const phase = ref<Phase>('form')
const formError = ref('')
const isSubmitting = ref<'approve' | 'deny' | null>(null)

/** The interceptor's rejection shape (api/client.ts) always carries `status`. */
function errorStatus(err: unknown): number | undefined {
  if (!err || typeof err !== 'object') return undefined
  const status = (err as { status?: unknown }).status
  return typeof status === 'number' ? status : undefined
}

async function submit(action: 'approve' | 'deny'): Promise<void> {
  if (!canSubmit.value || isSubmitting.value !== null) return

  formError.value = ''
  isSubmitting.value = action

  try {
    if (action === 'approve') {
      await approveDevice(userCode.value)
      phase.value = 'approved'
    } else {
      await denyDevice(userCode.value)
      phase.value = 'denied'
    }
  } catch (err: unknown) {
    const status = errorStatus(err)
    if (status === 404) {
      formError.value = t('device.errors.notFound')
    } else if (status === 410) {
      phase.value = 'expired'
    } else {
      const message = extractI18nErrorMessage(err, t, 'device.errors', t('device.errors.generic'))
      formError.value = message
      appStore.showError(message)
    }
  } finally {
    isSubmitting.value = null
  }
}
</script>

<style scoped>
.dvc {
  max-width: 420px;
  margin: 0 auto;
}

/* Ground rule 9: cards carry no shadow, only a hairline and the fill. */
.dvc__card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 24px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.dvc__actions {
  display: flex;
  gap: 8px;
}

.dvc__actions > .btn2 {
  flex: 1;
}
</style>
