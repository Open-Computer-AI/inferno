<template>
  <div ref="rootRef" v-if="showUsageWindows" class="uc">
    <!-- Anthropic OAuth and Setup Token accounts: fetch real usage data -->
    <template
      v-if="
        account.platform === 'anthropic' &&
        (account.type === 'oauth' || account.type === 'setup-token')
      "
    >
      <div v-if="loading" class="uc-skel">
        <span class="uc-skel__label" />
        <span class="uc-skel__bar" />
      </div>

      <div v-else-if="error" class="uc-error">{{ error }}</div>

      <div v-else-if="usageInfo" class="uc-body">
        <div v-if="usageInfo.error" class="uc-inline-warn" :title="usageInfo.error">{{ usageInfo.error }}</div>

        <template v-if="primaryWindow">
          <CapacityBar
            size="sm"
            :percent="primaryWindow.percent"
            :label="primaryWindow.label"
            :trailing="barTrailing(primaryWindow)"
          />
          <button
            v-if="otherWindows.length"
            type="button"
            class="uc-expand"
            :aria-expanded="expanded"
            @click="expanded = !expanded"
          >
            <i
              class="hgi-stroke hgi-arrow-down-01 uc-expand__chevron"
              :class="{ 'uc-expand__chevron--open': expanded }"
              aria-hidden="true"
              style="font-size: 9px; /* june-lint-disable ground-rule-4: icon glyph */"
            />
            {{ expanded ? t('admin.accounts.usageWindow.hideWindows') : t('admin.accounts.usageWindow.moreWindows', { count: otherWindows.length }) }}
          </button>
          <div v-if="expanded" class="uc-expandlist">
            <CapacityBar
              v-for="w in otherWindows"
              :key="w.key"
              size="sm"
              :percent="w.percent"
              :label="w.label"
              :trailing="pctLabel(w.percent)"
            >
              {{ expandedFoot(w) }}
            </CapacityBar>
          </div>
        </template>
        <div v-else class="uc-muted">-</div>

        <div class="uc-actionrow">
          <span v-if="usageInfo.source === 'passive'" class="uc-hint">
            {{ t('admin.accounts.usageWindow.passiveSampled') }}
          </span>
          <button
            type="button"
            class="uc-linkbtn"
            :disabled="activeQueryLoading"
            @click="loadActiveUsage"
          >
            <i
              class="hgi-stroke hgi-refresh-01"
              :class="{ 's2a-spinner': activeQueryLoading }"
              aria-hidden="true"
              style="font-size: 10px; /* june-lint-disable ground-rule-4: icon glyph */"
            />
            {{ t('admin.accounts.usageWindow.activeQuery') }}
          </button>
        </div>
      </div>

      <!-- No data yet -->
      <div v-else class="uc-muted">-</div>
    </template>

    <!-- OpenAI OAuth accounts: single source from /usage API -->
    <template v-else-if="account.platform === 'openai' && account.type === 'oauth'">
      <div v-if="hasOpenAIUsageFallback && primaryWindow" class="uc-body">
        <CapacityBar
          size="sm"
          :percent="primaryWindow.percent"
          :label="primaryWindow.label"
          :trailing="barTrailing(primaryWindow)"
        />
        <button
          v-if="otherWindows.length"
          type="button"
          class="uc-expand"
          :aria-expanded="expanded"
          @click="expanded = !expanded"
        >
          <i
            class="hgi-stroke hgi-arrow-down-01 uc-expand__chevron"
            :class="{ 'uc-expand__chevron--open': expanded }"
            aria-hidden="true"
            style="font-size: 9px; /* june-lint-disable ground-rule-4: icon glyph */"
          />
          {{ expanded ? t('admin.accounts.usageWindow.hideWindows') : t('admin.accounts.usageWindow.moreWindows', { count: otherWindows.length }) }}
        </button>
        <div v-if="expanded" class="uc-expandlist">
          <CapacityBar
            v-for="w in otherWindows"
            :key="w.key"
            size="sm"
            :percent="w.percent"
            :label="w.label"
            :trailing="pctLabel(w.percent)"
          >
            {{ expandedFoot(w) }}
          </CapacityBar>
        </div>

        <!--
          Upstream codex /wham/usage quota query + reset. The local active-sampling
          refresh button is rendered via the pre-actions slot so the user sees a
          single row of related buttons instead of two stacked rows.
        -->
        <OpenAIQuotaResetCell :account="account" @account-updated="handleQuotaResetAccountUpdated">
          <template #pre-actions>
            <button
              type="button"
              class="uc-linkbtn"
              :disabled="activeQueryLoading"
              @click="loadActiveUsage"
            >
              <i
                class="hgi-stroke hgi-refresh-01"
                :class="{ 's2a-spinner': activeQueryLoading }"
                aria-hidden="true"
                style="font-size: 10px; /* june-lint-disable ground-rule-4: icon glyph */"
              />
              {{ t('admin.accounts.usageWindow.activeQuery') }}
            </button>
          </template>
        </OpenAIQuotaResetCell>
      </div>
      <div v-else-if="loading" class="uc-skel">
        <span class="uc-skel__label" />
        <span class="uc-skel__bar" />
      </div>
      <div v-else class="uc-body">
        <div class="uc-muted">-</div>
        <!-- Always allow on-demand upstream quota query, even before local data exists. -->
        <OpenAIQuotaResetCell
          :account="account"
          class="uc-mt"
          @account-updated="handleQuotaResetAccountUpdated"
        />
      </div>
    </template>

    <!-- Antigravity OAuth accounts: fetch usage from API -->
    <template v-else-if="account.platform === 'antigravity' && account.type === 'oauth'">
      <!-- Account tier badge -->
      <div v-if="antigravityTierLabel" class="uc-badgerow">
        <span class="uc-badge">{{ antigravityTierLabel }}</span>
        <!-- Ineligible-account warning -->
        <span v-if="hasIneligibleTiers" class="uc-tip" tabindex="0">
          <i
            class="hgi-stroke hgi-alert-circle"
            aria-hidden="true"
            style="font-size: 13px; color: var(--destructive); /* june-lint-disable ground-rule-4: icon glyph */"
          />
          <span class="uc-tip__bubble uc-tip__bubble--wide">{{ t('admin.accounts.ineligibleWarning') }}</span>
        </span>
      </div>

      <!-- Forbidden state (403) -->
      <div v-if="isForbidden" class="uc-body">
        <span class="uc-badge" :data-tone="forbiddenTone">{{ forbiddenLabel }}</span>
        <div v-if="validationURL" class="uc-actionrow">
          <a
            :href="validationURL"
            target="_blank"
            rel="noopener noreferrer"
            class="uc-linkbtn"
            :title="t('admin.accounts.openVerification')"
          >
            {{ t('admin.accounts.openVerification') }}
          </a>
          <button
            type="button"
            class="uc-linkbtn"
            :title="t('admin.accounts.copyLink')"
            @click="copyValidationURL"
          >
            {{ linkCopied ? t('admin.accounts.linkCopied') : t('admin.accounts.copyLink') }}
          </button>
        </div>
      </div>

      <!-- Needs reauth (401) -->
      <div v-else-if="needsReauth" class="uc-body">
        <span class="uc-badge" data-tone="attn">{{ t('admin.accounts.needsReauth') }}</span>
      </div>

      <!-- Degraded error (non-403, non-401) -->
      <div v-else-if="usageInfo?.error" class="uc-body">
        <span class="uc-badge" data-tone="attn">{{ usageErrorLabel }}</span>
      </div>

      <!-- Loading state -->
      <div v-else-if="loading" class="uc-skel">
        <span class="uc-skel__label" />
        <span class="uc-skel__bar" />
      </div>

      <!-- Error state -->
      <div v-else-if="error" class="uc-error">{{ error }}</div>

      <!-- Usage data from API -->
      <div v-else-if="hasAntigravityQuotaFromAPI" class="uc-body">
        <template v-if="primaryWindow">
          <CapacityBar
            size="sm"
            :percent="primaryWindow.percent"
            :label="primaryWindow.label"
            :trailing="barTrailing(primaryWindow)"
          />
          <button
            v-if="otherWindows.length"
            type="button"
            class="uc-expand"
            :aria-expanded="expanded"
            @click="expanded = !expanded"
          >
            <i
              class="hgi-stroke hgi-arrow-down-01 uc-expand__chevron"
              :class="{ 'uc-expand__chevron--open': expanded }"
              aria-hidden="true"
              style="font-size: 9px; /* june-lint-disable ground-rule-4: icon glyph */"
            />
            {{ expanded ? t('admin.accounts.usageWindow.hideWindows') : t('admin.accounts.usageWindow.moreWindows', { count: otherWindows.length }) }}
          </button>
          <div v-if="expanded" class="uc-expandlist">
            <CapacityBar
              v-for="w in otherWindows"
              :key="w.key"
              size="sm"
              :percent="w.percent"
              :label="w.label"
              :trailing="pctLabel(w.percent)"
            >
              {{ expandedFoot(w) }}
            </CapacityBar>
          </div>
        </template>
        <div v-if="aiCreditsDisplay" class="uc-muted uc-mt">
          {{ t('admin.accounts.aiCreditsBalance') }}: {{ aiCreditsDisplay }}
        </div>
      </div>
      <div v-else-if="aiCreditsDisplay" class="uc-muted">
        {{ t('admin.accounts.aiCreditsBalance') }}: {{ aiCreditsDisplay }}
      </div>
      <div v-else class="uc-muted">-</div>
    </template>

    <!-- Grok OAuth accounts: passive xAI quota headers + local Sub2API usage -->
    <template v-else-if="account.platform === 'grok' && account.type === 'oauth'">
      <div v-if="loading" class="uc-skel">
        <span class="uc-skel__label" />
        <span class="uc-skel__bar" />
      </div>
      <div v-else-if="error" class="uc-error">{{ error }}</div>
      <div v-else-if="needsReauth" class="uc-body">
        <span class="uc-badge" data-tone="attn">{{ t('admin.accounts.needsReauth') }}</span>
      </div>
      <div v-else-if="isForbidden" class="uc-body">
        <span class="uc-badge" data-tone="danger">{{ usageInfo?.grok_entitlement_status || t('admin.accounts.forbidden') }}</span>
      </div>
      <div v-else-if="usageInfo" class="uc-body">
        <!-- Free: only rolling 24h soft-gate bar. Paid: 7d + 30d billing. -->
        <template v-if="primaryWindow">
          <CapacityBar
            size="sm"
            :percent="primaryWindow.percent"
            :label="primaryWindow.label"
            :trailing="barTrailing(primaryWindow)"
          />
          <button
            v-if="otherWindows.length"
            type="button"
            class="uc-expand"
            :aria-expanded="expanded"
            @click="expanded = !expanded"
          >
            <i
              class="hgi-stroke hgi-arrow-down-01 uc-expand__chevron"
              :class="{ 'uc-expand__chevron--open': expanded }"
              aria-hidden="true"
              style="font-size: 9px; /* june-lint-disable ground-rule-4: icon glyph */"
            />
            {{ expanded ? t('admin.accounts.usageWindow.hideWindows') : t('admin.accounts.usageWindow.moreWindows', { count: otherWindows.length }) }}
          </button>
          <div v-if="expanded" class="uc-expandlist">
            <CapacityBar
              v-for="w in otherWindows"
              :key="w.key"
              size="sm"
              :percent="w.percent"
              :label="w.label"
              :trailing="pctLabel(w.percent)"
            >
              {{ expandedFoot(w) }}
            </CapacityBar>
          </div>
        </template>
        <div v-else-if="grokQuotaUnknown" class="uc-muted">{{ grokQuotaUnknownLabel }}</div>
        <div v-else class="uc-muted">-</div>

        <div v-if="grokPrepaidMoneyLine" class="uc-moneyrow">
          <span class="uc-chip uc-chip--brand" :title="t('admin.accounts.usageWindow.grokPrepaid')">
            {{ t('admin.accounts.usageWindow.grokPrepaid') }} ${{ grokPrepaidMoneyLine.prepaid }}
          </span>
          <span class="uc-hint" :title="t('admin.accounts.usageWindow.grokMonthlyLimit')">
            {{ t('admin.accounts.usageWindow.grokUsed') }}
            {{ grokPrepaidMoneyLine.used }}/{{ grokPrepaidMoneyLine.limit }}
          </span>
        </div>
        <div v-if="usageInfo.error" class="uc-inline-warn" :title="usageInfo.error">{{ usageErrorLabel }}</div>
        <div v-if="grokRetryAfterLabel" class="uc-inline-warn">
          {{ t('admin.accounts.usageWindow.grokRetryAfter', { time: grokRetryAfterLabel }) }}
        </div>
      </div>
      <div v-else class="uc-muted">-</div>
    </template>

    <!-- CN providers (Kimi / Zhipu / DeepSeek): coding-plan quota or payg balance -->
    <template v-else-if="account.platform === 'kimi' || account.platform === 'zhipu' || account.platform === 'deepseek'">
      <div class="uc-body">
        <!-- 子单元格各自按 模式×平台 判定可见；两者都不可见时（智谱 payg 无公开
             余额端点、coding 探测也不适用）才回落到占位符。 -->
        <div
          v-if="!cnQuotaCellVisible && !cnBalanceCellVisible"
          class="uc-muted"
          :title="t('admin.accounts.cnProviders.noBalanceEndpoint')"
        >-</div>
        <CNProviderQuotaCell :account="account" />
        <CNProviderBalanceCell :account="account" />
      </div>
    </template>

    <!-- Gemini platform: show quota + local usage window -->
    <template v-else-if="account.platform === 'gemini'">
      <!-- Auth Type + Tier Badge (first line) -->
      <div v-if="geminiAuthTypeLabel" class="uc-badgerow">
        <span class="uc-badge">{{ geminiAuthTypeLabel }}</span>
        <!-- Help icon -->
        <span class="uc-tip" tabindex="0">
          <i
            class="hgi-stroke hgi-help-circle"
            aria-hidden="true"
            style="font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */"
          />
          <span class="uc-tip__bubble uc-tip__bubble--wide">
            <span class="uc-tip__title">{{ t('admin.accounts.gemini.quotaPolicy.title') }}</span>
            <span class="uc-tip__note">{{ t('admin.accounts.gemini.quotaPolicy.note') }}</span>
            <span class="uc-tip__row"><strong>{{ geminiQuotaPolicyChannel }}</strong></span>
            <span class="uc-tip__row">{{ geminiQuotaPolicyLimits }}</span>
            <a :href="geminiQuotaPolicyDocsUrl" target="_blank" rel="noopener noreferrer" class="uc-tip__link">
              {{ t('admin.accounts.gemini.quotaPolicy.columns.docs') }}
            </a>
          </span>
        </span>
      </div>

      <!-- Usage data or unlimited flow -->
      <div class="uc-body">
        <div v-if="showGeminiTodayStats && todayStats" class="uc-chips">
          <span class="uc-chip">{{ formatKeyRequests }} req</span>
          <span class="uc-chip">{{ formatKeyTokens }}</span>
          <span class="uc-chip" :title="t('usage.accountBilled')">A ${{ formatKeyCost }}</span>
          <span v-if="todayStats.user_cost != null" class="uc-chip" :title="t('usage.userBilled')">
            U ${{ formatKeyUserCost }}
          </span>
        </div>
        <div v-else-if="showGeminiTodayStats && todayStatsLoading" class="uc-chips-skel" />

        <div v-if="loading" class="uc-skel">
          <span class="uc-skel__label" />
          <span class="uc-skel__bar" />
        </div>
        <div v-else-if="error" class="uc-error">{{ error }}</div>
        <!-- Gemini: show daily usage bar(s) when available -->
        <template v-else-if="geminiUsageAvailable && primaryWindow">
          <CapacityBar
            size="sm"
            :percent="primaryWindow.percent"
            :label="primaryWindow.label"
            :trailing="barTrailing(primaryWindow)"
          />
          <button
            v-if="otherWindows.length"
            type="button"
            class="uc-expand"
            :aria-expanded="expanded"
            @click="expanded = !expanded"
          >
            <i
              class="hgi-stroke hgi-arrow-down-01 uc-expand__chevron"
              :class="{ 'uc-expand__chevron--open': expanded }"
              aria-hidden="true"
              style="font-size: 9px; /* june-lint-disable ground-rule-4: icon glyph */"
            />
            {{ expanded ? t('admin.accounts.usageWindow.hideWindows') : t('admin.accounts.usageWindow.moreWindows', { count: otherWindows.length }) }}
          </button>
          <div v-if="expanded" class="uc-expandlist">
            <CapacityBar
              v-for="w in otherWindows"
              :key="w.key"
              size="sm"
              :percent="w.percent"
              :label="w.label"
              :trailing="pctLabel(w.percent)"
            >
              {{ expandedFoot(w) }}
            </CapacityBar>
          </div>
          <p class="uc-footnote">* {{ t('admin.accounts.gemini.quotaPolicy.simulatedNote') || 'Simulated quota' }}</p>
        </template>
        <!-- AI Studio Client OAuth: show unlimited flow (no usage tracking) -->
        <div v-else class="uc-muted">
          {{ t('admin.accounts.gemini.rateLimit.unlimited') }}
        </div>
      </div>
    </template>

    <!-- Other accounts: no usage window -->
    <template v-else>
      <div class="uc-muted">-</div>
    </template>
  </div>

  <!-- Non-OAuth/Setup-Token accounts -->
  <div ref="rootRef" v-else class="uc">
    <!-- Gemini API Key accounts: show quota info -->
    <AccountQuotaInfo v-if="account.platform === 'gemini'" :account="account" />
    <!-- Key/Bedrock accounts: show today stats + optional quota bars -->
    <div v-else class="uc-body">
      <OllamaCloudUsageCell
        v-if="account.ollama_cloud_usage?.eligible"
        :account="account"
      />
      <!-- Today stats row (requests, tokens, cost, user_cost) -->
      <div v-if="todayStats" class="uc-chips">
        <span class="uc-chip">{{ formatKeyRequests }} req</span>
        <span class="uc-chip">{{ formatKeyTokens }}</span>
        <span class="uc-chip" :title="t('usage.accountBilled')">A ${{ formatKeyCost }}</span>
        <span v-if="todayStats.user_cost != null" class="uc-chip" :title="t('usage.userBilled')">
          U ${{ formatKeyUserCost }}
        </span>
      </div>
      <!-- Loading skeleton for today stats -->
      <div v-else-if="todayStatsLoading" class="uc-chips-skel" />

      <!-- API Key accounts with quota limits: closest-limit bar + expansion -->
      <template v-if="primaryWindow">
        <CapacityBar
          size="sm"
          :percent="primaryWindow.percent"
          :label="primaryWindow.label"
          :trailing="barTrailing(primaryWindow)"
        />
        <button
          v-if="otherWindows.length"
          type="button"
          class="uc-expand"
          :aria-expanded="expanded"
          @click="expanded = !expanded"
        >
          <i
            class="hgi-stroke hgi-arrow-down-01 uc-expand__chevron"
            :class="{ 'uc-expand__chevron--open': expanded }"
            aria-hidden="true"
            style="font-size: 9px; /* june-lint-disable ground-rule-4: icon glyph */"
          />
          {{ expanded ? t('admin.accounts.usageWindow.hideWindows') : t('admin.accounts.usageWindow.moreWindows', { count: otherWindows.length }) }}
        </button>
        <div v-if="expanded" class="uc-expandlist">
          <CapacityBar
            v-for="w in otherWindows"
            :key="w.key"
            size="sm"
            :percent="w.percent"
            :label="w.label"
            :trailing="pctLabel(w.percent)"
          >
            {{ expandedFoot(w) }}
          </CapacityBar>
        </div>
      </template>

      <!-- No data at all -->
      <div
        v-if="!todayStats && !todayStatsLoading && !hasApiKeyQuota && !account.ollama_cloud_usage?.eligible"
        class="uc-muted"
      >-</div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * AccountUsageCell — part 08, section 02 ("the hardest cell in the
 * product"). Implements the approved UX departure "one bar, with a window
 * picker on the column" (part 08, "Three decisions, taken").
 *
 * Migration note from the prototype:
 *   "The 1,599 lines are mostly derivation and they all stay. The change is
 *    in the template: pick the maximum utilisation window, render one bar,
 *    move the rest into the expansion... Delete the color field from the
 *    bar objects."
 *
 * Every platform derivation below is unchanged from before this pass (the
 * per-platform computed values that used to feed a stack of
 * `UsageProgressBar`s). What changed is that they now all funnel into one
 * normalized `UsageWindowBar[]` per platform, from which the template
 * renders exactly one `CapacityBar` (sm, the 6px continuous fill, since a
 * 36px table cell has no room for ticks) — the window closest to its limit
 * — colour spent on risk via CapacityBar's own threshold, never on which
 * window it is. Every other window is one click away in a local disclosure
 * (`expanded`) rather than a table-owned row expansion: this file does not
 * own DataTable, and DataTable.vue's virtualizer already remeasures a row
 * when its rendered content grows (it hooks a ResizeObserver via
 * `measureElement`), so a cell that grows its own height on expand works
 * today without any DataTable change.
 *
 * A column-header window picker (pin the column to one window, e.g. "5
 * hour", so every row shows that window and the column sorts by it) is a
 * DataTable column-header concern, not this cell's — see `pinnedWindowKey`
 * below and the build report for exactly what the header needs to pass in.
 *
 * GrokQuotaProbeCell no longer renders inside this cell: its probe is a
 * real billable call to xAI, so per the same "three decisions" it becomes
 * its own opt-in column, off by default, rather than an affordance offered
 * on every visible Grok row. See the build report for what that column
 * needs.
 */
import { ref, computed, onMounted, onBeforeUnmount, onUnmounted, watch } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageInfo, GeminiCredentials, WindowStats } from '@/types'
import { buildOpenAIUsageRefreshKey } from '@/utils/accountUsageRefresh'
import { enqueueUsageRequest } from '@/utils/usageLoadQueue'
import { formatCompactNumber } from '@/utils/format'
import CapacityBar from '@/components/common/CapacityBar.vue'
import AccountQuotaInfo from './AccountQuotaInfo.vue'
import OpenAIQuotaResetCell from './OpenAIQuotaResetCell.vue'
import CNProviderQuotaCell from './CNProviderQuotaCell.vue'
import CNProviderBalanceCell from './CNProviderBalanceCell.vue'
import OllamaCloudUsageCell from './OllamaCloudUsageCell.vue'
import { cnQuotaCellVisible as cnQuotaCellVisibleFn, cnBalanceCellVisible as cnBalanceCellVisibleFn } from './credentialsBuilder'

// Module-level cache shared across all AccountUsageCell instances
const _usageCache = new Map<number, { data: AccountUsageInfo; ts: number }>()
const USAGE_CACHE_TTL = 5 * 60 * 1000 // 5 minutes
// How long a quota-reset response may suppress the row-patch usage refetch.
const SUPPRESS_USAGE_REFRESH_WINDOW_MS = 5 * 1000

const props = withDefaults(
  defineProps<{
    account: Account
    todayStats?: WindowStats | null
    todayStatsLoading?: boolean
    manualRefreshToken?: number
    batchedUsage?: AccountUsageInfo | null
    batchedUsageError?: string | null
    batchedUsageLoading?: boolean
    requestBatchedUsage?: ((account: Account, options?: { force?: boolean }) => void) | null
    /**
     * External override for which window the primary bar shows, keyed by
     * the same `key` used in the expansion list (e.g. "five_hour"). Not
     * used by any current caller: it exists so a future column-header
     * picker can pin every row to one window without this cell changing.
     * Falls back to "closest to its limit" when unset or not found.
     */
    pinnedWindowKey?: string | null
  }>(),
  {
    todayStats: null,
    todayStatsLoading: false,
    manualRefreshToken: 0,
    batchedUsage: null,
    batchedUsageError: null,
    batchedUsageLoading: false,
    requestBatchedUsage: null,
    pinnedWindowKey: null
  }
)

const emit = defineEmits<{
  'account-updated': [account: Account]
  'usage-loaded': [usage: AccountUsageInfo]
}>()

const { t } = useI18n()
const desktopViewportQuery = '(min-width: 768px)'

const unmounted = ref(false)
onBeforeUnmount(() => {
  unmounted.value = true
  pauseExpandedClock()
})

const loading = ref(false)
const activeQueryLoading = ref(false)
const error = ref<string | null>(null)
const usageInfo = ref<AccountUsageInfo | null>(null)
watch(usageInfo, (usage) => {
  if (usage) emit('usage-loaded', usage)
})
const suppressOpenAIUsageRefreshUntil = ref(0)
const rootRef = ref<HTMLElement | null>(null)
const isDesktopViewport = ref(
  typeof window === 'undefined' ? true : window.matchMedia(desktopViewportQuery).matches
)
const hasEnteredViewport = ref(false)
const pendingAutoLoad = ref(false)
const pendingAutoLoadSource = ref<'passive' | 'active' | undefined>(undefined)

let desktopViewportMediaQuery: MediaQueryList | null = null
let desktopViewportListener: ((event: MediaQueryListEvent) => void) | null = null
let visibilityObserver: IntersectionObserver | null = null

// Show usage windows for OAuth and Setup Token accounts
const showUsageWindows = computed(() => {
  // Gemini: we can always compute local usage windows from DB logs (simulated quotas).
  if (props.account.platform === 'gemini') return true
  // CN providers: apikey 账号也有滚动用量窗口（coding plan）或余额（payg），
  // 由 CNProviderQuotaCell / CNProviderBalanceCell 自行探测与展示。
  if (
    props.account.platform === 'kimi' ||
    props.account.platform === 'zhipu' ||
    props.account.platform === 'deepseek'
  ) {
    return true
  }
  return props.account.type === 'oauth' || props.account.type === 'setup-token'
})

const shouldFetchUsage = computed(() => {
  if (props.account.platform === 'anthropic') {
    return props.account.type === 'oauth' || props.account.type === 'setup-token'
  }
  if (props.account.platform === 'gemini') {
    return true
  }
  if (props.account.platform === 'antigravity') {
    return props.account.type === 'oauth'
  }
  if (props.account.platform === 'grok') {
    return props.account.type === 'oauth'
  }
  if (props.account.platform === 'openai') {
    return props.account.type === 'oauth'
  }
  return false
})

// CN 供应商子单元格可见性（与 CNProviderQuotaCell / CNProviderBalanceCell 共用
// credentialsBuilder 的单一实现）：都不可见时显示 `-` 占位符。
const cnAccountMode = computed(() => {
  const mode = props.account.credentials?.account_mode
  return typeof mode === 'string' ? mode : ''
})
const cnQuotaCellVisible = computed(() => cnQuotaCellVisibleFn(props.account.platform, cnAccountMode.value))
const cnBalanceCellVisible = computed(() => cnBalanceCellVisibleFn(props.account.platform, cnAccountMode.value))

const isBatchManaged = computed(() => typeof props.requestBatchedUsage === 'function')

const showGeminiTodayStats = computed(() => {
  return props.account.platform === 'gemini' && props.account.type === 'service_account'
})

const geminiUsageAvailable = computed(() => {
  return (
    !!usageInfo.value?.gemini_shared_daily ||
    !!usageInfo.value?.gemini_pro_daily ||
    !!usageInfo.value?.gemini_flash_daily ||
    !!usageInfo.value?.gemini_shared_minute ||
    !!usageInfo.value?.gemini_pro_minute ||
    !!usageInfo.value?.gemini_flash_minute
  )
})

const hasOpenAIUsageFallback = computed(() => {
  if (props.account.platform !== 'openai' || props.account.type !== 'oauth') return false
  return !!usageInfo.value?.five_hour || !!usageInfo.value?.seven_day
})

const openAIUsageRefreshKey = computed(() => buildOpenAIUsageRefreshKey(props.account))

const shouldAutoLoadUsageOnMount = computed(() => {
  return shouldFetchUsage.value
})

const shouldLazyLoadOnMobile = computed(() => {
  return shouldFetchUsage.value && !isDesktopViewport.value
})

// Antigravity quota types (用于 API 返回的数据)
interface AntigravityUsageResult {
  utilization: number
  resetTime: string | null
}

// ===== Antigravity quota from API (usageInfo.antigravity_quota) =====

// 检查是否有从 API 获取的配额数据
const hasAntigravityQuotaFromAPI = computed(() => {
  return usageInfo.value?.antigravity_quota && Object.keys(usageInfo.value.antigravity_quota).length > 0
})

// 从 API 配额数据中获取使用率（多模型取最高使用率）
const getAntigravityUsageFromAPI = (
  modelNames: string[]
): AntigravityUsageResult | null => {
  const quota = usageInfo.value?.antigravity_quota
  if (!quota) return null

  let maxUtilization = 0
  let earliestReset: string | null = null

  for (const model of modelNames) {
    const modelQuota = quota[model]
    if (!modelQuota) continue

    if (modelQuota.utilization > maxUtilization) {
      maxUtilization = modelQuota.utilization
    }
    if (modelQuota.reset_time) {
      if (!earliestReset || modelQuota.reset_time < earliestReset) {
        earliestReset = modelQuota.reset_time
      }
    }
  }

  // 如果没有找到任何匹配的模型
  if (maxUtilization === 0 && earliestReset === null) {
    const hasAnyData = modelNames.some((m) => quota[m])
    if (!hasAnyData) return null
  }

  return {
    utilization: maxUtilization,
    resetTime: earliestReset
  }
}

// Gemini 3 Pro from API
const antigravity3ProUsageFromAPI = computed(() =>
  getAntigravityUsageFromAPI(['gemini-3-pro-low', 'gemini-3-pro-high', 'gemini-3-pro-preview'])
)

// Gemini 3 Flash from API
const antigravity3FlashUsageFromAPI = computed(() => getAntigravityUsageFromAPI(['gemini-3-flash']))

// Gemini Image from API
const antigravity3ImageUsageFromAPI = computed(() =>
  getAntigravityUsageFromAPI(['gemini-2.5-flash-image', 'gemini-3.1-flash-image', 'gemini-3-pro-image'])
)

// Claude from API (all Claude model variants)
const antigravityClaudeUsageFromAPI = computed(() =>
  getAntigravityUsageFromAPI([
    'claude-fable-5',
    'claude-sonnet-4-5', 'claude-opus-4-5-thinking',
    'claude-sonnet-4-6', 'claude-opus-4-6', 'claude-opus-4-6-thinking',
    'claude-opus-4-7', 'claude-opus-4-8',
  ])
)

const aiCreditsDisplay = computed(() => {
  const credits = usageInfo.value?.ai_credits
  if (!credits || credits.length === 0) return null
  const total = credits.reduce((sum, credit) => sum + (credit.amount ?? 0), 0)
  if (total <= 0) return null
  return total.toFixed(0)
})

// Antigravity 账户类型（从 load_code_assist 响应中提取）
const antigravityTier = computed(() => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  if (!extra) return null

  const loadCodeAssist = extra.load_code_assist as Record<string, unknown> | undefined
  if (!loadCodeAssist) return null

  // 优先取 paidTier，否则取 currentTier
  const paidTier = loadCodeAssist.paidTier as Record<string, unknown> | undefined
  if (paidTier && typeof paidTier.id === 'string') {
    return paidTier.id
  }

  const currentTier = loadCodeAssist.currentTier as Record<string, unknown> | undefined
  if (currentTier && typeof currentTier.id === 'string') {
    return currentTier.id
  }

  return null
})

// Gemini 账户类型（从 credentials 中提取）
const geminiTier = computed(() => {
  if (props.account.platform !== 'gemini') return null
  const creds = props.account.credentials as GeminiCredentials | undefined
  return creds?.tier_id || null
})

const geminiOAuthType = computed(() => {
  if (props.account.platform !== 'gemini') return null
  const creds = props.account.credentials as GeminiCredentials | undefined
  return (creds?.oauth_type || '').trim() || null
})

// Gemini 是否为 Code Assist OAuth
const isGeminiCodeAssist = computed(() => {
  if (props.account.platform !== 'gemini') return false
  const creds = props.account.credentials as GeminiCredentials | undefined
  return creds?.oauth_type === 'code_assist' || (!creds?.oauth_type && !!creds?.project_id)
})

const geminiChannelShort = computed((): 'ai studio' | 'gcp' | 'google one' | 'client' | null => {
  if (props.account.platform !== 'gemini') return null

  // API Key accounts are AI Studio.
  if (props.account.type === 'apikey') return 'ai studio'

  if (geminiOAuthType.value === 'google_one') return 'google one'
  if (isGeminiCodeAssist.value) return 'gcp'
  if (geminiOAuthType.value === 'ai_studio') return 'client'

  // Fallback (unknown legacy data): treat as AI Studio.
  return 'ai studio'
})

const geminiUserLevel = computed((): string | null => {
  if (props.account.platform !== 'gemini') return null

  const tier = (geminiTier.value || '').toString().trim()
  const tierLower = tier.toLowerCase()
  const tierUpper = tier.toUpperCase()

  // Google One: free / pro / ultra
  if (geminiOAuthType.value === 'google_one') {
    if (tierLower === 'google_one_free') return 'free'
    if (tierLower === 'google_ai_pro') return 'pro'
    if (tierLower === 'google_ai_ultra') return 'ultra'

    // Backward compatibility (legacy tier markers)
    if (tierUpper === 'AI_PREMIUM' || tierUpper === 'GOOGLE_ONE_STANDARD') return 'pro'
    if (tierUpper === 'GOOGLE_ONE_UNLIMITED') return 'ultra'
    if (tierUpper === 'FREE' || tierUpper === 'GOOGLE_ONE_BASIC' || tierUpper === 'GOOGLE_ONE_UNKNOWN' || tierUpper === '') return 'free'

    return null
  }

  // GCP Code Assist: standard / enterprise
  if (isGeminiCodeAssist.value) {
    if (tierLower === 'gcp_enterprise') return 'enterprise'
    if (tierLower === 'gcp_standard') return 'standard'

    // Backward compatibility
    if (tierUpper.includes('ULTRA') || tierUpper.includes('ENTERPRISE')) return 'enterprise'
    return 'standard'
  }

  // AI Studio (API Key) and Client OAuth: free / paid
  if (props.account.type === 'apikey' || geminiOAuthType.value === 'ai_studio') {
    if (tierLower === 'aistudio_paid') return 'paid'
    if (tierLower === 'aistudio_free') return 'free'

    // Backward compatibility
    if (tierUpper.includes('PAID') || tierUpper.includes('PAYG') || tierUpper.includes('PAY')) return 'paid'
    if (tierUpper.includes('FREE')) return 'free'
    if (props.account.type === 'apikey') return 'free'
    return null
  }

  return null
})

// Gemini 认证类型（按要求：授权方式简称 + 用户等级）
const geminiAuthTypeLabel = computed(() => {
  if (props.account.platform !== 'gemini') return null
  if (!geminiChannelShort.value) return null
  return geminiUserLevel.value ? `${geminiChannelShort.value} ${geminiUserLevel.value}` : geminiChannelShort.value
})

// Gemini 配额政策信息
const geminiQuotaPolicyChannel = computed(() => {
  if (geminiOAuthType.value === 'google_one') {
    return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.channel')
  }
  if (isGeminiCodeAssist.value) {
    return t('admin.accounts.gemini.quotaPolicy.rows.gcp.channel')
  }
  return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.channel')
})

const geminiQuotaPolicyLimits = computed(() => {
  const tierLower = (geminiTier.value || '').toString().trim().toLowerCase()

  if (geminiOAuthType.value === 'google_one') {
    if (tierLower === 'google_ai_ultra' || geminiUserLevel.value === 'ultra') {
      return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsUltra')
    }
    if (tierLower === 'google_ai_pro' || geminiUserLevel.value === 'pro') {
      return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsPro')
    }
    return t('admin.accounts.gemini.quotaPolicy.rows.googleOne.limitsFree')
  }

  if (isGeminiCodeAssist.value) {
    if (tierLower === 'gcp_enterprise' || geminiUserLevel.value === 'enterprise') {
      return t('admin.accounts.gemini.quotaPolicy.rows.gcp.limitsEnterprise')
    }
    return t('admin.accounts.gemini.quotaPolicy.rows.gcp.limitsStandard')
  }

  // AI Studio (API Key / custom OAuth)
  if (tierLower === 'aistudio_paid' || geminiUserLevel.value === 'paid') {
    return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsPaid')
  }
  return t('admin.accounts.gemini.quotaPolicy.rows.aiStudio.limitsFree')
})

const geminiQuotaPolicyDocsUrl = computed(() => {
  if (geminiOAuthType.value === 'google_one' || isGeminiCodeAssist.value) {
    return 'https://developers.google.com/gemini-code-assist/resources/quotas'
  }
  return 'https://ai.google.dev/pricing'
})

const geminiUsesSharedDaily = computed(() => {
  if (props.account.platform !== 'gemini') return false
  // Per requirement: Google One & GCP are shared RPD pools (no per-model breakdown).
  return (
    !!usageInfo.value?.gemini_shared_daily ||
    !!usageInfo.value?.gemini_shared_minute ||
    geminiOAuthType.value === 'google_one' ||
    isGeminiCodeAssist.value
  )
})

const geminiUsageBars = computed(() => {
  if (props.account.platform !== 'gemini') return []
  if (!usageInfo.value) return []

  const bars: Array<{
    key: string
    label: string
    utilization: number
    resetsAt: string | null
    windowStats?: WindowStats | null
  }> = []

  if (geminiUsesSharedDaily.value) {
    const sharedDaily = usageInfo.value.gemini_shared_daily
    if (sharedDaily) {
      bars.push({
        key: 'shared_daily',
        label: t('admin.accounts.usageWindow.oneDay'),
        utilization: sharedDaily.utilization,
        resetsAt: sharedDaily.resets_at,
        windowStats: sharedDaily.window_stats
      })
    }
    return bars
  }

  const pro = usageInfo.value.gemini_pro_daily
  if (pro) {
    bars.push({
      key: 'pro_daily',
      label: t('admin.accounts.usageWindow.geminiProDaily'),
      utilization: pro.utilization,
      resetsAt: pro.resets_at,
      windowStats: pro.window_stats
    })
  }

  const flash = usageInfo.value.gemini_flash_daily
  if (flash) {
    bars.push({
      key: 'flash_daily',
      label: t('admin.accounts.usageWindow.geminiFlashDaily'),
      utilization: flash.utilization,
      resetsAt: flash.resets_at,
      windowStats: flash.window_stats
    })
  }

  return bars
})

interface GrokQuotaBarInfo {
  utilization: number
  resetsAt: string | null
}

const grokBilling = computed(() => usageInfo.value?.grok_billing || null)
const grokWeeklyBillingBar = computed((): GrokQuotaBarInfo | null => {
  const billing = grokBilling.value
  if (billing?.period_type?.toLowerCase() !== 'weekly' || billing.usage_percent == null) {
    return null
  }
  return {
    utilization: Math.min(100, Math.max(0, billing.usage_percent)),
    resetsAt: billing.period_end || null
  }
})
// Monthly used/limit % from billing probe (used_percent or derived from cents).
const grokMonthlyBillingBar = computed((): GrokQuotaBarInfo | null => {
  const billing = grokBilling.value
  if (!billing) return null
  let utilization: number | null = null
  if (billing.used_percent != null && Number.isFinite(billing.used_percent)) {
    utilization = billing.used_percent
  } else if (
    billing.monthly_limit_cents != null &&
    billing.monthly_limit_cents > 0 &&
    billing.used_cents != null
  ) {
    utilization = (billing.used_cents / billing.monthly_limit_cents) * 100
  }
  if (utilization == null) return null
  // Avoid duplicating the weekly bar when period_type is weekly-only without monthly.
  if (billing.period_type?.toLowerCase() === 'weekly' && billing.monthly_limit_cents == null) {
    return null
  }
  return {
    utilization: Math.min(100, Math.max(0, utilization)),
    resetsAt: billing.billing_period_end || billing.period_end || null
  }
})
const formatGrokMoney = (value?: number | null) => {
  if (value == null || Number.isNaN(value)) return '0'
  if (value >= 1000) return formatCompactNumber(value)
  if (value >= 100) return value.toFixed(0)
  if (value >= 10) return value.toFixed(1)
  return value.toFixed(2)
}
// Prepaid money line for paid Grok: show when prepaid_balance is present.
// Monthly used/limit numbers are optional context; primary progress is the 30d bar.
const grokPrepaidMoneyLine = computed(() => {
  const billing = grokBilling.value
  if (!billing) return null
  const prepaid = billing.prepaid_balance
  // "只针对预付": only render when prepaid field exists (including $0.00).
  if (prepaid == null || !Number.isFinite(prepaid)) return null
  const used =
    billing.monthly_used != null
      ? billing.monthly_used
      : billing.used_cents != null
        ? billing.used_cents / 100
        : 0
  const limit =
    billing.monthly_limit != null
      ? billing.monthly_limit
      : billing.monthly_limit_cents != null
        ? billing.monthly_limit_cents / 100
        : 0
  return {
    prepaid: formatGrokMoney(prepaid),
    used: formatGrokMoney(used),
    limit: formatGrokMoney(limit)
  }
})
const grokPlanLabelIsFree = (value: string) => value.includes('free') || value.includes('basic')
const grokPlanLabelIsPaid = (value: string) => {
  return value !== '' && !grokPlanLabelIsFree(value) && !value.includes('unknown')
}
const grokIsFree = computed(() => {
  if (props.account.platform !== 'grok' || props.account.type !== 'oauth') return false
  const billing = grokBilling.value
  const plan = (billing?.plan || '').trim().toLowerCase()
  const tier = (usageInfo.value?.subscription_tier || '').trim().toLowerCase()
  const entitlement = (usageInfo.value?.grok_entitlement_status || '').toLowerCase()
  // The live credential tier decides FIRST. When a subscription lapses the
  // credential refreshes to free immediately, but the leftover
  // monthly_limit_cents / usage_percent from the last billing probe linger in
  // extra -- so checking billing metrics first kept paid 7d/30d money bars on
  // an account that is already free. supergrok_lite still resolves paid, so
  // Lite keeps its 7d bar.
  if (grokPlanLabelIsFree(tier)) return true
  if (grokPlanLabelIsPaid(tier)) return false
  if (
    billing?.usage_percent != null ||
    billing?.used_percent != null ||
    (billing?.monthly_limit_cents != null && billing.monthly_limit_cents > 0)
  ) return false
  if (grokPlanLabelIsPaid(plan)) return false
  if (
    grokPlanLabelIsFree(plan) ||
    grokPlanLabelIsFree(entitlement)
  ) return true
  return billing != null
})
const grokFreeQuotaUsage = computed(() => usageInfo.value?.grok_local_usage_24h || null)
const grokFreeTokenBar = computed(() => {
  if (!grokIsFree.value || !grokFreeQuotaUsage.value) return null
  const limit = usageInfo.value?.grok_free_token_limit
  if (typeof limit !== 'number' || limit <= 0) return null
  const used = Math.max(0, grokFreeQuotaUsage.value.tokens || 0)
  return { utilization: Math.min(100, (used / limit) * 100), limit }
})
const grokQuotaUnknown = computed(() => {
  if (props.account.platform !== 'grok') return false
  if (grokIsFree.value) {
    return !grokFreeTokenBar.value
  }
  if (grokWeeklyBillingBar.value || grokMonthlyBillingBar.value || grokPrepaidMoneyLine.value) {
    return false
  }
  return usageInfo.value?.grok_quota_snapshot_state !== 'observed'
})
const grokQuotaUnknownLabel = computed(() => {
  return usageInfo.value?.grok_quota_snapshot_state === 'no_headers'
    ? t('admin.accounts.usageWindow.grokNoHeaders')
    : t('admin.accounts.usageWindow.grokUnknown')
})
const grokRetryAfterLabel = computed(() => {
  const seconds = usageInfo.value?.grok_retry_after_seconds
  if (seconds == null || seconds <= 0) return null
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.ceil(seconds / 60)
  return `${minutes}m`
})

// 账户类型显示标签
const antigravityTierLabel = computed(() => {
  switch (antigravityTier.value) {
    case 'free-tier':
      return t('admin.accounts.tier.free')
    case 'g1-pro-tier':
      return t('admin.accounts.tier.pro')
    case 'g1-ultra-tier':
      return t('admin.accounts.tier.ultra')
    default:
      return null
  }
})

// 检测账户是否有不合格状态（ineligibleTiers）
const hasIneligibleTiers = computed(() => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  if (!extra) return false

  const loadCodeAssist = extra.load_code_assist as Record<string, unknown> | undefined
  if (!loadCodeAssist) return false

  const ineligibleTiers = loadCodeAssist.ineligibleTiers as unknown[] | undefined
  return Array.isArray(ineligibleTiers) && ineligibleTiers.length > 0
})

// Antigravity 403 forbidden 状态
const isForbidden = computed(() => !!usageInfo.value?.is_forbidden)
const forbiddenType = computed(() => usageInfo.value?.forbidden_type || 'forbidden')
const validationURL = computed(() => usageInfo.value?.validation_url || '')

// 需要重新授权（401）
const needsReauth = computed(() => !!usageInfo.value?.needs_reauth)

// 降级错误标签（rate_limited / network_error）
const usageErrorLabel = computed(() => {
  const code = usageInfo.value?.error_code
  if (code === 'rate_limited') return t('admin.accounts.rateLimited')
  return t('admin.accounts.usageError')
})

const forbiddenLabel = computed(() => {
  switch (forbiddenType.value) {
    case 'validation':
      return t('admin.accounts.forbiddenValidation')
    case 'violation':
      return t('admin.accounts.forbiddenViolation')
    default:
      return t('admin.accounts.forbidden')
  }
})

// Colour is risk, never category (ground rule 5): validation is a
// recoverable state (attention), a violation ban is not (danger).
const forbiddenTone = computed(() => (forbiddenType.value === 'validation' ? 'attn' : 'danger'))

const linkCopied = ref(false)
const copyValidationURL = async () => {
  if (!validationURL.value) return
  try {
    await navigator.clipboard.writeText(validationURL.value)
    linkCopied.value = true
    setTimeout(() => { linkCopied.value = false }, 2000)
  } catch {
    // fallback: ignore
  }
}

const isAnthropicOAuthOrSetupToken = computed(() => {
  return props.account.platform === 'anthropic' && (props.account.type === 'oauth' || props.account.type === 'setup-token')
})

// ===== One dimension object per window, normalized across platforms =====
// (the same shape the departure asks for on the quota family, applied here
// to the bars this cell already derives above; see the build report for
// why QuotaLimitCard/QuotaDimensionRow are a separate file this pass does
// not own).
interface UsageWindowBar {
  key: string
  label: string
  percent: number
  resetsAt: string | null
  /**
   * Regression fix (see build report, "WindowStats is fetched, typed, and
   * silently dropped"): per-window requests/tokens/cost as it arrives on
   * the wire (`UsageProgress.window_stats`). Optional because not every
   * source has it -- Antigravity's per-model quota and Grok's billing
   * summary are utilization/reset only, no request or token breakdown.
   * Rendered in the expansion only (see `uc-expandlist`), never on the
   * primary bar, which has no room for it inside 36px.
   */
  windowStats?: WindowStats | null
}

const anthropicWindows = computed<UsageWindowBar[]>(() => {
  if (!isAnthropicOAuthOrSetupToken.value || !usageInfo.value) return []
  const info = usageInfo.value
  const windows: UsageWindowBar[] = []
  if (info.five_hour) {
    windows.push({ key: 'five_hour', label: t('admin.accounts.usageWindow.fiveHour'), percent: info.five_hour.utilization, resetsAt: info.five_hour.resets_at, windowStats: info.five_hour.window_stats })
  }
  if (info.seven_day) {
    windows.push({ key: 'seven_day', label: t('admin.accounts.usageWindow.sevenDay'), percent: info.seven_day.utilization, resetsAt: info.seven_day.resets_at, windowStats: info.seven_day.window_stats })
  }
  if (info.seven_day_sonnet) {
    windows.push({ key: 'seven_day_sonnet', label: t('admin.accounts.usageWindow.sevenDaySonnet'), percent: info.seven_day_sonnet.utilization, resetsAt: info.seven_day_sonnet.resets_at, windowStats: info.seven_day_sonnet.window_stats })
  }
  if (info.seven_day_fable) {
    windows.push({ key: 'seven_day_fable', label: t('admin.accounts.usageWindow.sevenDayFable'), percent: info.seven_day_fable.utilization, resetsAt: info.seven_day_fable.resets_at, windowStats: info.seven_day_fable.window_stats })
  }
  return windows
})

const openAIWindows = computed<UsageWindowBar[]>(() => {
  if (!hasOpenAIUsageFallback.value) return []
  const windows: UsageWindowBar[] = []
  if (usageInfo.value?.five_hour) {
    windows.push({ key: 'five_hour', label: t('admin.accounts.usageWindow.fiveHour'), percent: usageInfo.value.five_hour.utilization, resetsAt: usageInfo.value.five_hour.resets_at, windowStats: usageInfo.value.five_hour.window_stats })
  }
  if (usageInfo.value?.seven_day) {
    windows.push({ key: 'seven_day', label: t('admin.accounts.usageWindow.sevenDay'), percent: usageInfo.value.seven_day.utilization, resetsAt: usageInfo.value.seven_day.resets_at, windowStats: usageInfo.value.seven_day.window_stats })
  }
  return windows
})

// No `windowStats` on any pushed window here: `AntigravityModelQuota` (the
// wire shape for `antigravity_quota`) is utilization + reset_time only --
// there never was a per-window request/token/cost breakdown to drop.
const antigravityWindows = computed<UsageWindowBar[]>(() => {
  if (!hasAntigravityQuotaFromAPI.value) return []
  const windows: UsageWindowBar[] = []
  if (antigravity3ProUsageFromAPI.value) {
    windows.push({ key: 'gemini3_pro', label: t('admin.accounts.usageWindow.gemini3Pro'), percent: antigravity3ProUsageFromAPI.value.utilization, resetsAt: antigravity3ProUsageFromAPI.value.resetTime })
  }
  if (antigravity3FlashUsageFromAPI.value) {
    windows.push({ key: 'gemini3_flash', label: t('admin.accounts.usageWindow.gemini3Flash'), percent: antigravity3FlashUsageFromAPI.value.utilization, resetsAt: antigravity3FlashUsageFromAPI.value.resetTime })
  }
  if (antigravity3ImageUsageFromAPI.value) {
    windows.push({ key: 'gemini3_image', label: t('admin.accounts.usageWindow.gemini3Image'), percent: antigravity3ImageUsageFromAPI.value.utilization, resetsAt: antigravity3ImageUsageFromAPI.value.resetTime })
  }
  if (antigravityClaudeUsageFromAPI.value) {
    windows.push({ key: 'claude', label: t('admin.accounts.usageWindow.claude'), percent: antigravityClaudeUsageFromAPI.value.utilization, resetsAt: antigravityClaudeUsageFromAPI.value.resetTime })
  }
  return windows
})

const grokWindows = computed<UsageWindowBar[]>(() => {
  const windows: UsageWindowBar[] = []
  if (grokIsFree.value) {
    if (grokFreeTokenBar.value) {
      // grok_local_usage_24h IS a WindowStats (it is the local usage log
      // this bar's own percent is derived from), unlike the two billing
      // bars below -- pass it through instead of dropping it a second time.
      windows.push({ key: 'grok_24h', label: t('admin.accounts.usageWindow.grok24h'), percent: grokFreeTokenBar.value.utilization, resetsAt: null, windowStats: grokFreeQuotaUsage.value })
    }
    return windows
  }
  // grokWeeklyBillingBar / grokMonthlyBillingBar come from GrokBillingSummary
  // (xAI's billing probe): money and a percent, no request/token count --
  // there is no windowStats to attach here either.
  if (grokWeeklyBillingBar.value) {
    windows.push({ key: 'grok_7d', label: t('admin.accounts.usageWindow.sevenDay'), percent: grokWeeklyBillingBar.value.utilization, resetsAt: grokWeeklyBillingBar.value.resetsAt })
  }
  if (grokMonthlyBillingBar.value) {
    windows.push({ key: 'grok_30d', label: t('admin.accounts.usageWindow.thirtyDay'), percent: grokMonthlyBillingBar.value.utilization, resetsAt: grokMonthlyBillingBar.value.resetsAt })
  }
  return windows
})

const geminiWindows = computed<UsageWindowBar[]>(() =>
  geminiUsageBars.value.map((bar) => ({ key: bar.key, label: bar.label, percent: bar.utilization, resetsAt: bar.resetsAt, windowStats: bar.windowStats }))
)

const keyAccountWindows = computed<UsageWindowBar[]>(() => {
  const windows: UsageWindowBar[] = []
  if (quotaDailyBar.value) {
    windows.push({ key: 'quota_daily', label: t('admin.accounts.usageWindow.oneDay'), percent: quotaDailyBar.value.utilization, resetsAt: quotaDailyBar.value.resetsAt })
  }
  if (quotaWeeklyBar.value) {
    windows.push({ key: 'quota_weekly', label: t('admin.accounts.usageWindow.sevenDay'), percent: quotaWeeklyBar.value.utilization, resetsAt: quotaWeeklyBar.value.resetsAt })
  }
  if (quotaTotalBar.value) {
    windows.push({ key: 'quota_total', label: t('admin.accounts.usageWindow.total'), percent: quotaTotalBar.value.utilization, resetsAt: null })
  }
  return windows
})

// The one list this cell actually renders: whichever platform's windows
// apply to this row. Only one of the source lists above is ever non-empty
// for a given account, so this is a lookup, not a merge.
const activeWindows = computed<UsageWindowBar[]>(() => {
  if (props.account.platform === 'anthropic') return anthropicWindows.value
  if (props.account.platform === 'openai') return openAIWindows.value
  if (props.account.platform === 'antigravity') return antigravityWindows.value
  if (props.account.platform === 'grok') return grokWindows.value
  if (props.account.platform === 'gemini') return geminiWindows.value
  if (props.account.type === 'apikey' || props.account.type === 'bedrock') return keyAccountWindows.value
  return []
})

// The bar shown is the window closest to its limit -- the only one that
// can cause an incident -- unless a caller has pinned the column to a
// specific window.
const primaryWindow = computed<UsageWindowBar | null>(() => {
  const windows = activeWindows.value
  if (!windows.length) return null
  if (props.pinnedWindowKey) {
    const pinned = windows.find((w) => w.key === props.pinnedWindowKey)
    if (pinned) return pinned
  }
  return windows.reduce((max, w) => (w.percent > max.percent ? w : max), windows[0])
})

const otherWindows = computed<UsageWindowBar[]>(() => {
  const primary = primaryWindow.value
  if (!primary) return []
  return activeWindows.value.filter((w) => w.key !== primary.key)
})

// Local disclosure rather than a table-owned row expansion (see the file
// header comment). Collapses whenever the row now refers to a different
// account, so a virtualized row recycled for a new account never opens
// pre-expanded.
const expanded = ref(false)
watch(
  () => [props.account.id, props.account.platform, props.account.type] as const,
  () => { expanded.value = false }
)

const pctLabel = (percent: number) => {
  const rounded = Math.round(percent)
  return rounded > 999 ? '>999%' : `${rounded}%`
}

// Countdown clock for the expansion's reset column only: ticking a timer
// per row for a bar nobody has opened is the exact per-row cost this
// redesign removes elsewhere, so it only runs while `expanded` is true.
const expandedClockNow = ref(new Date())
const { pause: pauseExpandedClock, resume: resumeExpandedClock } = useIntervalFn(
  () => { expandedClockNow.value = new Date() },
  60_000,
  { immediate: false }
)
watch(expanded, (isExpanded) => {
  if (isExpanded) {
    expandedClockNow.value = new Date()
    resumeExpandedClock()
  } else {
    pauseExpandedClock()
  }
})

const resetsLabel = (resetsAt: string | null, percent: number) => {
  if (!resetsAt) return t('usage.resetNow')
  const date = new Date(resetsAt)
  const diffMs = date.getTime() - expandedClockNow.value.getTime()
  if (diffMs <= 0) return percent > 0 ? t('usage.resetPending') : t('usage.resetNow')
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffMins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))
  if (diffHours >= 24) {
    const days = Math.floor(diffHours / 24)
    return `${days}d ${diffHours % 24}h`
  }
  if (diffHours > 0) return `${diffHours}h ${diffMins}m`
  return `${diffMins}m`
}

// Regression fix (see build report): the primary bar is the ONLY window
// shown for a single-window account (Grok, single-image Antigravity), so
// its reset time cannot be deferred to the expansion the way otherWindows
// defer theirs -- there may be no expansion at all. A 36px table row has no
// room for the reset as its own line (that costs ~21px via CapacityBar's
// slot, the treatment otherWindows use), so it rides inline with the
// percent on the bar's existing head row instead. Reuses `pctLabel` and
// `resetsLabel` as-is -- no second formatter.
const barTrailing = (w: UsageWindowBar) => `${pctLabel(w.percent)} · ${resetsLabel(w.resetsAt, w.percent)}`

// Regression fix (see build report, "WindowStats is fetched, typed, and
// silently dropped"): same req/tokens/A$/U$ formatting the today-stats chips
// already use elsewhere in this file (formatKeyRequests etc.), reused here
// rather than inventing a second convention for the same four numbers.
const formatWindowStats = (stats: WindowStats) => {
  const parts = [
    `${formatCompactNumber(stats.requests, { allowBillions: false })} req`,
    formatCompactNumber(stats.tokens),
    `A $${stats.cost.toFixed(2)}`
  ]
  if (stats.user_cost != null) parts.push(`U $${stats.user_cost.toFixed(2)}`)
  return parts.join(' · ')
}

// Expansion rows have room the primary bar does not (part 08's own
// reasoning for why the primary bar shows one window and everything else is
// "one click away"), so this is where the dropped window_stats resurfaces --
// appended after the reset time on the same footer line, not a new bar.
const expandedFoot = (w: UsageWindowBar) => {
  const reset = resetsLabel(w.resetsAt, w.percent)
  return w.windowStats ? `${reset} · ${formatWindowStats(w.windowStats)}` : reset
}

const requestParentBatchUsage = (options?: { force?: boolean }) => {
  if (!isBatchManaged.value || !shouldFetchUsage.value) return
  props.requestBatchedUsage?.(props.account, options)
}

const syncManagedUsageState = () => {
  if (!isBatchManaged.value) return
  usageInfo.value = props.batchedUsage ?? null
  error.value = props.batchedUsageError ?? null
  loading.value = props.batchedUsageLoading === true
}

const loadUsage = async (options?: { source?: 'passive' | 'active'; bypassCache?: boolean }) => {
  if (!shouldFetchUsage.value) return
  if (isBatchManaged.value) {
    requestParentBatchUsage({ force: options?.bypassCache === true })
    return
  }

  // Check cache
  if (!options?.bypassCache) {
    const cached = _usageCache.get(props.account.id)
    if (cached && Date.now() - cached.ts < USAGE_CACHE_TTL) {
      usageInfo.value = cached.data
      loading.value = false
      return
    }
  }

  loading.value = true
  error.value = null

  try {
		const fetchFn = () => options?.source
			? adminAPI.accounts.getUsage(props.account.id, options.source, options.bypassCache === true)
			: adminAPI.accounts.getUsage(props.account.id)
    const result = await enqueueUsageRequest(props.account, fetchFn)
    if (!unmounted.value) {
      usageInfo.value = result
      _usageCache.set(props.account.id, { data: result, ts: Date.now() })
    }
  } catch (e: any) {
    if (!unmounted.value) {
      error.value = t('common.error')
      console.error('Failed to load usage:', e)
    }
  } finally {
    if (!unmounted.value) loading.value = false
  }
}

const flushPendingAutoLoad = () => {
  if (!pendingAutoLoad.value) return
  const source = pendingAutoLoadSource.value
  pendingAutoLoad.value = false
  pendingAutoLoadSource.value = undefined
  loadUsage({ source }).catch((e) => {
    console.error('Failed to load deferred usage:', e)
  })
}

const requestAutoLoad = (source?: 'passive' | 'active') => {
  if (!shouldFetchUsage.value) return
  if (shouldLazyLoadOnMobile.value && !hasEnteredViewport.value) {
    pendingAutoLoad.value = true
    pendingAutoLoadSource.value = source
    return
  }
  loadUsage({ source }).catch((e) => {
    console.error('Failed to auto load usage:', e)
  })
}

const detachVisibilityObserver = () => {
  visibilityObserver?.disconnect()
  visibilityObserver = null
}

const attachVisibilityObserver = () => {
  detachVisibilityObserver()
  if (!shouldLazyLoadOnMobile.value || hasEnteredViewport.value) return
  if (typeof window === 'undefined' || typeof IntersectionObserver === 'undefined') {
    hasEnteredViewport.value = true
    flushPendingAutoLoad()
    return
  }
  if (!rootRef.value) return

  visibilityObserver = new IntersectionObserver((entries) => {
    if (!entries.some((entry) => entry.isIntersecting)) return
    hasEnteredViewport.value = true
    detachVisibilityObserver()
    flushPendingAutoLoad()
  }, {
    root: null,
    rootMargin: '200px 0px',
    threshold: 0.01
  })
  visibilityObserver.observe(rootRef.value)
}

const loadActiveUsage = async () => {
  activeQueryLoading.value = true
  try {
    usageInfo.value = await adminAPI.accounts.getUsage(props.account.id, 'active', true)
  } catch (e: any) {
    console.error('Failed to load active usage:', e)
  } finally {
    activeQueryLoading.value = false
  }
}

// ===== API Key quota progress bars =====

interface QuotaBarInfo {
  utilization: number
  resetsAt: string | null
}

const makeQuotaBar = (
  used: number,
  limit: number,
  startKey?: string
): QuotaBarInfo => {
  const utilization = limit > 0 ? (used / limit) * 100 : 0
  let resetsAt: string | null = null
  if (startKey) {
    const extra = props.account.extra as Record<string, unknown> | undefined
    const isDaily = startKey.includes('daily')
    const mode = isDaily
      ? (extra?.quota_daily_reset_mode as string) || 'rolling'
      : (extra?.quota_weekly_reset_mode as string) || 'rolling'

    if (mode === 'fixed') {
      // Use pre-computed next reset time for fixed mode
      const resetAtKey = isDaily ? 'quota_daily_reset_at' : 'quota_weekly_reset_at'
      resetsAt = (extra?.[resetAtKey] as string) || null
    } else {
      // Rolling mode: compute from start + period
      const startStr = extra?.[startKey] as string | undefined
      if (startStr) {
        const startDate = new Date(startStr)
        const periodMs = isDaily ? 24 * 60 * 60 * 1000 : 7 * 24 * 60 * 60 * 1000
        resetsAt = new Date(startDate.getTime() + periodMs).toISOString()
      }
    }
  }
  return { utilization, resetsAt }
}

const hasApiKeyQuota = computed(() => {
  if (props.account.type !== 'apikey' && props.account.type !== 'bedrock') return false
  return (
    (props.account.quota_daily_limit ?? 0) > 0 ||
    (props.account.quota_weekly_limit ?? 0) > 0 ||
    (props.account.quota_limit ?? 0) > 0
  )
})

const quotaDailyBar = computed((): QuotaBarInfo | null => {
  const limit = props.account.quota_daily_limit ?? 0
  if (limit <= 0) return null
  return makeQuotaBar(props.account.quota_daily_used ?? 0, limit, 'quota_daily_start')
})

const quotaWeeklyBar = computed((): QuotaBarInfo | null => {
  const limit = props.account.quota_weekly_limit ?? 0
  if (limit <= 0) return null
  return makeQuotaBar(props.account.quota_weekly_used ?? 0, limit, 'quota_weekly_start')
})

const quotaTotalBar = computed((): QuotaBarInfo | null => {
  const limit = props.account.quota_limit ?? 0
  if (limit <= 0) return null
  return makeQuotaBar(props.account.quota_used ?? 0, limit)
})

const handleQuotaResetAccountUpdated = (account: Account) => {
  // The reset response already carries authoritative quota and account data.
  // Avoid turning the parent patch into a second automatic /usage request.
  // The suppression is time-boxed so an unhandled emit (parent that ignores
  // account-updated) cannot latch it and swallow a later, unrelated refresh.
  suppressOpenAIUsageRefreshUntil.value = Date.now() + SUPPRESS_USAGE_REFRESH_WINDOW_MS
  emit('account-updated', account)
}

// ===== Key account today stats formatters =====

const formatKeyRequests = computed(() => {
  if (!props.todayStats) return ''
  return formatCompactNumber(props.todayStats.requests, { allowBillions: false })
})

const formatKeyTokens = computed(() => {
  if (!props.todayStats) return ''
  return formatCompactNumber(props.todayStats.tokens)
})

const formatKeyCost = computed(() => {
  if (!props.todayStats) return '0.00'
  return props.todayStats.cost.toFixed(2)
})

const formatKeyUserCost = computed(() => {
  if (!props.todayStats || props.todayStats.user_cost == null) return '0.00'
  return props.todayStats.user_cost.toFixed(2)
})

onMounted(() => {
  if (typeof window !== 'undefined') {
    desktopViewportMediaQuery = window.matchMedia(desktopViewportQuery)
    isDesktopViewport.value = desktopViewportMediaQuery.matches
    desktopViewportListener = (event: MediaQueryListEvent) => {
      isDesktopViewport.value = event.matches
    }
    if (typeof desktopViewportMediaQuery.addEventListener === 'function') {
      desktopViewportMediaQuery.addEventListener('change', desktopViewportListener)
    } else {
      desktopViewportMediaQuery.addListener(desktopViewportListener)
    }
  }

  if (isBatchManaged.value) {
    syncManagedUsageState()
    requestParentBatchUsage()
    return
  }

  if (!shouldAutoLoadUsageOnMount.value) return
  const source = isAnthropicOAuthOrSetupToken.value ? 'passive' : undefined
  requestAutoLoad(source)
})

watch(
  () => [props.batchedUsage, props.batchedUsageError, props.batchedUsageLoading, isBatchManaged.value] as const,
  () => {
    syncManagedUsageState()
  },
  { immediate: true, deep: true }
)

watch(isBatchManaged, (managed, wasManaged) => {
  if (managed && !wasManaged) {
    syncManagedUsageState()
    requestParentBatchUsage()
  }
})

watch(
  () => [props.account.id, props.account.platform, props.account.type, isBatchManaged.value] as const,
  ([accountID, platform, accountType, managed], [previousAccountID, previousPlatform, previousAccountType]) => {
    if (
      accountID === previousAccountID &&
      platform === previousPlatform &&
      accountType === previousAccountType
    ) {
      return
    }
    if (!managed || !shouldFetchUsage.value) return
    syncManagedUsageState()
    requestParentBatchUsage()
  },
  { flush: 'post' }
)

watch(openAIUsageRefreshKey, (nextKey, prevKey) => {
  if (!prevKey || nextKey === prevKey) return
  if (props.account.platform !== 'openai' || props.account.type !== 'oauth') return
  if (Date.now() < suppressOpenAIUsageRefreshUntil.value) {
    suppressOpenAIUsageRefreshUntil.value = 0
    return
  }

  if (isBatchManaged.value) {
    requestParentBatchUsage({ force: true })
    return
  }

  _usageCache.delete(props.account.id)
  requestAutoLoad()
})

watch(
  () => props.manualRefreshToken,
  (nextToken, prevToken) => {
    if (nextToken === prevToken) return
    if (!shouldFetchUsage.value) return

    if (isBatchManaged.value) {
      requestParentBatchUsage({ force: true })
      return
    }

    const source = isAnthropicOAuthOrSetupToken.value ? 'passive' : undefined
    _usageCache.delete(props.account.id)
    loadUsage({ source, bypassCache: true }).catch((e) => {
      console.error('Failed to refresh usage after manual refresh:', e)
    })
  }
)

watch(
  [rootRef, shouldLazyLoadOnMobile],
  () => {
    if (shouldLazyLoadOnMobile.value) {
      attachVisibilityObserver()
      return
    }
    detachVisibilityObserver()
  },
  { immediate: true, flush: 'post' }
)

watch(isDesktopViewport, (isDesktop) => {
  if (isDesktop) {
    detachVisibilityObserver()
    hasEnteredViewport.value = true
    flushPendingAutoLoad()
    return
  }
  hasEnteredViewport.value = false
  attachVisibilityObserver()
})

onUnmounted(() => {
  detachVisibilityObserver()
  if (desktopViewportMediaQuery && desktopViewportListener) {
    if (typeof desktopViewportMediaQuery.removeEventListener === 'function') {
      desktopViewportMediaQuery.removeEventListener('change', desktopViewportListener)
    } else {
      desktopViewportMediaQuery.removeListener(desktopViewportListener)
    }
  }
  desktopViewportListener = null
  desktopViewportMediaQuery = null
})
</script>

<style scoped>
.uc {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.uc-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.uc-muted {
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}

.uc-mt {
  margin-top: 4px;
}

.uc-error {
  font-size: var(--fs-sm);
  color: var(--destructive);
}

.uc-inline-warn {
  overflow: hidden;
  max-width: 200px;
  color: var(--s2a-attn);
  font-size: var(--fs-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Loading: flat static bars, no pulse (ground rule 7 / the loading rule). */
.uc-skel {
  display: flex;
  align-items: center;
  gap: 6px;
}
.uc-skel__label {
  width: 32px;
  height: 12px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
}
.uc-skel__bar {
  flex: 1;
  min-width: 24px;
  height: 6px;
  border-radius: 3px;
  background: var(--surface-subtle);
}

.uc-badgerow {
  display: flex;
  align-items: center;
  gap: 5px;
}

/* Tier/channel labels are category, not state (ground rule 5): one flat
   neutral pill regardless of which tier. `data-tone` is reserved for the
   genuinely risk-bearing badges (forbidden, reauth, degraded). */
.uc-badge {
  display: inline-flex;
  align-items: center;
  height: 16px;
  padding: 0 6px;
  border-radius: var(--r-xs);
  background: var(--sidebar-accent);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
  white-space: nowrap;
}
.uc-badge[data-tone='attn'] {
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
}
.uc-badge[data-tone='danger'] {
  background: var(--destructive-soft);
  color: var(--destructive);
}

/* Hover-disclosed tooltip, plain CSS in place of the old group-hover
   Tailwind pattern. Opacity only, never a layout change. */
.uc-tip {
  position: relative;
  display: inline-flex;
  cursor: help;
}
.uc-tip__bubble {
  position: absolute;
  top: 100%;
  left: 0;
  z-index: 50;
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: max-content;
  max-width: 260px;
  margin-top: 4px;
  padding: 8px 10px;
  border-radius: var(--r-md);
  background: var(--foreground);
  color: var(--on-solid);
  font-size: var(--fs-sm);
  line-height: 1.45;
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--motion-hover);
}
.uc-tip__bubble--wide {
  max-width: 320px;
}
.uc-tip:hover .uc-tip__bubble,
.uc-tip:focus-visible .uc-tip__bubble {
  opacity: 1;
}
.uc-tip__title {
  font-weight: var(--fw-medium);
}
.uc-tip__note {
  color: color-mix(in oklch, var(--on-solid) 75%, transparent);
}
.uc-tip__link {
  color: color-mix(in oklch, var(--on-solid) 85%, var(--brand-tint));
  text-decoration: underline;
}

.uc-actionrow {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.uc-hint {
  font-size: var(--fs-2xs);
  color: var(--muted-foreground);
  font-style: italic;
}

.uc-linkbtn {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  border: 0;
  border-radius: var(--r-xs);
  padding: 1px 5px;
  background: transparent;
  color: var(--brand);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
  cursor: pointer;
  text-decoration: none;
  transition: background var(--motion-hover);
}
.uc-linkbtn:hover:not(:disabled) {
  background: var(--sidebar-accent);
}
.uc-linkbtn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.uc-linkbtn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

/* Every other window is one click away (part 08's migration note). */
.uc-expand {
  display: inline-flex;
  align-self: flex-start;
  align-items: center;
  gap: 3px;
  border: 0;
  padding: 0;
  background: transparent;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  cursor: pointer;
}
.uc-expand:hover {
  color: var(--foreground);
}
.uc-expand:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
.uc-expand__chevron {
  transition: transform var(--t-fast) var(--ease-out);
}
.uc-expand__chevron--open {
  transform: rotate(180deg);
}

.uc-expandlist {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 6px 0 2px;
}

.uc-moneyrow {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-2xs);
  color: var(--muted-foreground);
}

.uc-chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
}
.uc-chips-skel {
  height: 12px;
  width: 60%;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
}

.uc-chip {
  display: inline-flex;
  align-items: center;
  height: 15px;
  padding: 0 5px;
  border-radius: var(--r-xs);
  background: var(--sidebar-accent);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  white-space: nowrap;
}
.uc-chip--brand {
  background: var(--brand-tint);
  color: var(--brand);
}

.uc-footnote {
  margin: 2px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-style: italic;
  line-height: 1.4;
}
</style>
