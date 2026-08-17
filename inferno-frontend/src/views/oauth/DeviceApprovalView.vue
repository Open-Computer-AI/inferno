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
            :disabled="isBusy"
            @keyup.enter="onEnter"
          />

          <!-- The consent block. Nothing here is decoration: RFC 8628 5.4
               requires the authorization server to show who is asking and what
               they are asking for, and the Approve button below stays disabled
               until it is on screen. -->
          <div v-if="pending" class="dvc__consent" data-test="consent">
            <div class="dvc__field">
              <p class="dvc__label">{{ t('device.requestedBy') }}</p>
              <p class="dvc__client" data-test="client-name">{{ pending.client_name }}</p>
              <p class="dvc__clientid">{{ pending.client_id }}</p>
            </div>

            <div class="dvc__field">
              <p class="dvc__label">{{ t('device.requestedAccess') }}</p>
              <ul class="dvc__scopes" data-test="scopes">
                <li v-for="scope in pending.scopes" :key="scope" class="dvc__scope">
                  {{ scopeLabel(scope) }}
                </li>
              </ul>
            </div>

            <p class="dvc__warning" data-test="grant-warning">{{ t('device.grantWarning') }}</p>
          </div>

          <div class="dvc__actions">
            <Button
              v-if="!pending"
              type="button"
              variant="solid"
              size="lg"
              data-test="check"
              :loading="isChecking"
              :disabled="!codeComplete || isBusy"
              @click="loadPending()"
            >
              {{ isChecking ? t('device.checking') : t('device.check') }}
            </Button>
            <Button
              v-else
              type="button"
              variant="solid"
              size="lg"
              data-test="approve"
              :loading="isSubmitting === 'approve'"
              :disabled="!codeComplete || isBusy"
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
              :disabled="!codeComplete || isBusy"
              @click="submit('deny')"
            >
              {{ isSubmitting === 'deny' ? t('device.denying') : t('device.deny') }}
            </Button>
          </div>
        </template>

        <InterstitialState
          v-else-if="phase === 'approved'"
          tone="success"
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
          v-else-if="phase === 'decided'"
          tone="attention"
          icon="hgi-alert-circle"
          :title="t('device.decidedTitle')"
          :body="t('device.decidedBody')"
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
 * the prefilled code matches what their terminal is showing, sees WHO is
 * asking and WHAT they are asking for, and approves or denies. Route path is
 * load-bearing: OAuthDeviceService.RequestCode
 * (backend/internal/service/oauth_device_service.go) hardcodes
 * "{portal}/device" as verification_uri, so this view must stay mounted at
 * exactly /device (see router/index.ts).
 *
 * TWO STEPS, NOT ONE, and the split is the security property. This screen used
 * to render a code box and Approve/Deny and nothing else. That is
 * indistinguishable from a routine login prompt, which is what makes a phished
 * user_code work: an attacker starts a device flow against the well-known
 * public `hermes-cli` client asking for billing:manage and agents:manage,
 * sends the victim the code with a plausible "paste this to finish setup", and
 * the victim approves scopes they never saw. So Approve is not offered at all
 * until the pending authorization has been fetched and rendered -- the person
 * must have been shown the requester and the permissions before they can grant
 * them. Deny stays available at every point, because denying is always safe.
 *
 * Four outcomes past the form. Approve/deny (200) end the flow. An expired
 * code (410) is terminal too, but for a different reason -- the human's only
 * remedy is re-running the CLI command, so it gets its own screen that says
 * so. A code that already carries a decision (409) is its own terminal state:
 * saying "expired" there would be a lie, and if the person did not make that
 * decision they need to know. An unknown code (404) is the one case that is
 * NOT terminal: it usually means a mistyped digit, so it stays on the form as
 * an inline error the person can fix and resubmit immediately, the same way a
 * wrong password does on the login screen rather than bouncing to a dead end.
 */
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AppLayout, PageHeader, InterstitialState } from '@/components/layout'
import Button from '@/components/common/Button.vue'
import Input from '@/components/common/Input.vue'
import {
  approveDevice,
  denyDevice,
  fetchPendingDeviceAuthorization,
  type PendingDeviceAuthorization
} from '@/api/oauth'
import { useAppStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'

const route = useRoute()
const { t } = useI18n()
const appStore = useAppStore()

const codeInputRef = ref<InstanceType<typeof Input> | null>(null)

type Phase = 'form' | 'approved' | 'denied' | 'expired' | 'decided'

/**
 * Scope string to i18n key. Indirection rather than `t('device.scopes.' +
 * scope)` because the scope vocabulary contains colons, which are not safe to
 * embed in a vue-i18n message path. An unmapped scope falls back to a "deny
 * this" warning rather than being rendered raw or, worse, hidden: the backend
 * allowlist (service.ValidateScope) should make an unknown value impossible,
 * so seeing one means the two sides have drifted and the safe reading is that
 * the person cannot know what they are granting.
 */
const SCOPE_LABEL_KEYS: Record<string, string> = {
  'inference:invoke': 'inferenceInvoke',
  'billing:read': 'billingRead',
  'billing:manage': 'billingManage',
  'agents:read': 'agentsRead',
  'agents:manage': 'agentsManage'
}

function scopeLabel(scope: string): string {
  const key = SCOPE_LABEL_KEYS[scope]
  return key ? t(`device.scopes.${key}`) : t('device.unknownScope')
}

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
const pending = ref<PendingDeviceAuthorization | null>(null)

const userCode = computed({
  get: () => rawUserCode.value,
  set: (value: string) => {
    rawUserCode.value = formatUserCodeInput(value)
    // Editing the code invalidates whatever consent block is on screen -- the
    // person must not be able to review one authorization and then approve a
    // different one.
    pending.value = null
  }
})

const codeComplete = computed(() => rawUserCode.value.replace(/[^A-Z0-9]/g, '').length === 8)

const phase = ref<Phase>('form')
const formError = ref('')
const isChecking = ref(false)
const isSubmitting = ref<'approve' | 'deny' | null>(null)
const isBusy = computed(() => isChecking.value || isSubmitting.value !== null)

/** The interceptor's rejection shape (api/client.ts) always carries `status`. */
function errorStatus(err: unknown): number | undefined {
  if (!err || typeof err !== 'object') return undefined
  const status = (err as { status?: unknown }).status
  return typeof status === 'number' ? status : undefined
}

/**
 * Maps a rejection to a phase or an inline error. Shared by the lookup and the
 * decision calls: the same four statuses mean the same four things on both,
 * and duplicating the mapping is how the two drift apart.
 */
function handleFailure(err: unknown): void {
  const status = errorStatus(err)
  if (status === 404) {
    formError.value = t('device.errors.notFound')
  } else if (status === 410) {
    phase.value = 'expired'
  } else if (status === 409) {
    phase.value = 'decided'
  } else {
    const message = extractI18nErrorMessage(err, t, 'device.errors', t('device.errors.generic'))
    formError.value = message
    appStore.showError(message)
  }
}

async function loadPending(): Promise<void> {
  if (!codeComplete.value || isBusy.value) return

  formError.value = ''
  isChecking.value = true
  try {
    pending.value = await fetchPendingDeviceAuthorization(userCode.value)
  } catch (err: unknown) {
    pending.value = null
    handleFailure(err)
  } finally {
    isChecking.value = false
  }
}

async function submit(action: 'approve' | 'deny'): Promise<void> {
  if (!codeComplete.value || isBusy.value) return
  // Belt and braces alongside the template's v-if: approval requires that the
  // consent block was rendered.
  if (action === 'approve' && !pending.value) return

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
    handleFailure(err)
  } finally {
    isSubmitting.value = null
  }
}

/** Enter advances one step: look the code up, then approve what was shown. */
function onEnter(): void {
  if (pending.value) {
    void submit('approve')
  } else {
    void loadPending()
  }
}

// Input.vue's root is a wrapper <div>, so a plain `autofocus` attribute would
// land there instead of the inner <input> -- focus it explicitly instead.
// A prefilled code (the normal path, from verification_uri_complete) is looked
// up immediately so the person lands directly on the consent block.
onMounted(() => {
  codeInputRef.value?.focus()
  if (codeComplete.value) void loadPending()
})
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

.dvc__consent {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--muted);
}

.dvc__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.dvc__label {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  line-height: 1.4;
}

.dvc__client {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
  line-height: 1.4;
}

.dvc__clientid {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  line-height: 1.4;
}

.dvc__scopes {
  margin: 0;
  padding-left: 18px;
  list-style: disc;
}

.dvc__scope {
  color: var(--body-copy);
  font-size: var(--fs-md);
  line-height: 1.5;
}

/* Ground rule 5: colour marks state. This line states a consequence the
   person is about to accept, so it carries the attention colour rather than
   sitting in body copy where it reads as boilerplate. */
.dvc__warning {
  margin: 0;
  color: var(--s2a-attn);
  font-size: var(--fs-sm);
  line-height: 1.5;
}

.dvc__actions {
  display: flex;
  gap: 8px;
}

.dvc__actions > .btn2 {
  flex: 1;
}
</style>
