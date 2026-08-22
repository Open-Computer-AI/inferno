<template>
  <AppLayout>
    <div class="avc">
      <PageHeader />

      <section class="avc__card">
        <InterstitialState
          v-if="phase === 'loading' || phase === 'redirecting'"
          tone="waiting"
          :title="phase === 'redirecting' ? t('authorize.redirecting') : t('authorize.loading')"
        />

        <InterstitialState
          v-else-if="phase === 'error'"
          tone="failed"
          icon="hgi-alert-circle"
          :title="t('authorize.title')"
          :body="errorMessage"
        />

        <template v-else-if="phase === 'consent'">
          <!-- The consent block. Nothing here is decoration: RFC 6749's
               authorization endpoint must show the human who is asking and
               what they are asking for before Approve exists in the DOM at
               all -- mirrors DeviceApprovalView's two-step pattern for the
               same reason (RFC 8628 5.4's requirement, applied here even
               though this flow's own RFC doesn't name it explicitly). -->
          <div v-if="pending" class="avc__consent" data-test="consent">
            <div class="avc__field">
              <p class="avc__label">{{ t('authorize.requestedBy') }}</p>
              <p class="avc__client" data-test="client-name">{{ pending.client_name }}</p>
              <p class="avc__clientid">{{ pending.client_id }}</p>
            </div>

            <div class="avc__field">
              <p class="avc__label">{{ t('authorize.requestedAccess') }}</p>
              <ul class="avc__scopes" data-test="scopes">
                <li v-for="scope in scopeList" :key="scope" class="avc__scope">
                  {{ scopeLabel(scope) }}
                </li>
              </ul>
            </div>

            <p class="avc__warning" data-test="grant-warning">{{ t('authorize.grantWarning') }}</p>

            <div class="avc__actions">
              <Button
                type="button"
                variant="solid"
                size="lg"
                data-test="approve"
                :loading="isSubmitting === 'approve'"
                :disabled="isSubmitting !== null"
                @click="decide('approve')"
              >
                {{ isSubmitting === 'approve' ? t('authorize.approving') : t('authorize.approve') }}
              </Button>
              <Button
                type="button"
                variant="secondary"
                size="lg"
                data-test="deny"
                :loading="isSubmitting === 'deny'"
                :disabled="isSubmitting !== null"
                @click="decide('deny')"
              >
                {{ isSubmitting === 'deny' ? t('authorize.denying') : t('authorize.deny') }}
              </Button>
            </div>
          </div>
        </template>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * The authorization_code + PKCE consent screen -- the only human-facing
 * step of RFC 6749 4.1's authorization_code grant that this application
 * ever shows. Route path is load-bearing: this component only ever mounts
 * AFTER backend/internal/handler/oauth_authorize_handler.go's GET
 * /oauth/authorize has already validated client_id and redirect_uri (that
 * handler owns the RFC 6749 4.1.2.1 error-page/redirect split; this
 * component never re-derives it) and, for an unauthenticated visitor, sent
 * the browser through /login and back here with the original query string
 * intact (see router/index.ts's comment on this route and LoginView.vue's
 * post-login `router.push(redirectTo)`).
 *
 * On mount this re-issues the SAME request that handler already saw, this
 * time authenticated (api/oauth.ts's checkAuthorize) -- either it resolves
 * immediately to a target URL (the common case: the requested scope was
 * exactly the desktop-login scope and this user owns the client, so no
 * human decision is needed) or it reports "consent required", and only
 * then does this screen render anything past a loading state.
 */
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AppLayout, PageHeader, InterstitialState } from '@/components/layout'
import Button from '@/components/common/Button.vue'
import {
  checkAuthorize,
  decideAuthorize,
  fetchPendingAuthorization,
  type AuthorizeParams,
  type PendingAuthorization
} from '@/api/oauth'
import { extractApiErrorMessage } from '@/utils/apiError'

const route = useRoute()
const { t } = useI18n()

type Phase = 'loading' | 'consent' | 'redirecting' | 'error'

/** Same indirection DeviceApprovalView.vue uses and for the same reason:
 *  the scope vocabulary contains colons, which vue-i18n message paths
 *  cannot embed. An unmapped scope falls back to a "deny this" warning
 *  rather than being rendered raw or hidden -- the backend allowlist
 *  (service.ValidateScope) should make an unknown value impossible here, so
 *  seeing one means the two sides have drifted and the safe reading is that
 *  the person cannot know what they are granting. */
const SCOPE_LABEL_KEYS: Record<string, string> = {
  'agent_dashboard:access': 'agentDashboardAccess',
  'inference:invoke': 'inferenceInvoke',
  'tool:invoke': 'toolInvoke',
  'billing:read': 'billingRead',
  'billing:manage': 'billingManage',
  'agents:read': 'agentsRead',
  'agents:manage': 'agentsManage'
}

function scopeLabel(scope: string): string {
  const key = SCOPE_LABEL_KEYS[scope]
  return key ? t(`authorize.scopes.${key}`) : t('authorize.unknownScope')
}

function queryString(value: unknown): string {
  if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
  return typeof value === 'string' ? value : ''
}

const params = computed<AuthorizeParams>(() => ({
  response_type: queryString(route.query.response_type),
  client_id: queryString(route.query.client_id),
  redirect_uri: queryString(route.query.redirect_uri),
  scope: queryString(route.query.scope),
  state: queryString(route.query.state),
  code_challenge: queryString(route.query.code_challenge),
  code_challenge_method: queryString(route.query.code_challenge_method)
}))

const scopeList = computed(() => params.value.scope.split(/\s+/).filter(Boolean))

const phase = ref<Phase>('loading')
const errorMessage = ref('')
const pending = ref<PendingAuthorization | null>(null)
const isSubmitting = ref<'approve' | 'deny' | null>(null)

/** Isolated so tests can stub navigation instead of driving jsdom through a
 *  real cross-origin page load. Uses `replace`, not `assign`: this is the
 *  terminal step of a redirect-shaped flow, matching a real HTTP 302's
 *  semantics (no history entry) rather than `assign`'s "navigate as if the
 *  person typed the URL" -- `assign` would leave
 *  `/oauth/authorize?...&state=...&code_challenge=...` in the back stack,
 *  letting Back re-enter a flow that has already been decided. */
function navigateTo(url: string): void {
  window.location.replace(url)
}

function fail(err: unknown): void {
  errorMessage.value = extractApiErrorMessage(err, t('authorize.errors.generic'))
  phase.value = 'error'
}

async function start(): Promise<void> {
  const p = params.value
  if (!p.response_type || !p.client_id || !p.redirect_uri || !p.code_challenge || !p.code_challenge_method) {
    fail(null)
    return
  }

  try {
    const result = await checkAuthorize(p)
    if (result.kind === 'redirect') {
      phase.value = 'redirecting'
      navigateTo(result.url)
      return
    }
    pending.value = await fetchPendingAuthorization(p.client_id)
    phase.value = 'consent'
  } catch (err: unknown) {
    fail(err)
  }
}

async function decide(decision: 'approve' | 'deny'): Promise<void> {
  if (isSubmitting.value !== null) return
  isSubmitting.value = decision
  try {
    const url = await decideAuthorize(params.value, decision)
    phase.value = 'redirecting'
    navigateTo(url)
  } catch (err: unknown) {
    fail(err)
  } finally {
    isSubmitting.value = null
  }
}

onMounted(() => {
  void start()
})
</script>

<style scoped>
.avc {
  max-width: 420px;
  margin: 0 auto;
}

/* Ground rule 9: cards carry no shadow, only a hairline and the fill. */
.avc__card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 24px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.avc__consent {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.avc__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.avc__label {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  line-height: 1.4;
}

.avc__client {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
  line-height: 1.4;
}

.avc__clientid {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  line-height: 1.4;
}

.avc__scopes {
  margin: 0;
  padding-left: 18px;
  list-style: disc;
}

.avc__scope {
  color: var(--body-copy);
  font-size: var(--fs-md);
  line-height: 1.5;
}

/* Ground rule 5: colour marks state. This line states a consequence the
   person is about to accept, so it carries the attention colour rather than
   sitting in body copy where it reads as boilerplate. */
.avc__warning {
  margin: 0;
  color: var(--s2a-attn);
  font-size: var(--fs-sm);
  line-height: 1.5;
}

.avc__actions {
  display: flex;
  gap: 8px;
}

.avc__actions > .btn2 {
  flex: 1;
}
</style>
