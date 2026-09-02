<script setup lang="ts">
/**
 * Specimen board — development only, never routed in production.
 *
 * Mirrors the layout of the prototype documents so a screenshot of this can be
 * put beside a screenshot of the corresponding `.dc.html` and compared
 * directly. Every part gets a section here as it is built.
 *
 * It is also where the printed measurements get checked: the prototypes print
 * numbers next to components ("28 / 32 / 36 / 40"), and README says "Where a
 * prototype prints a measurement next to a component, that measurement is the
 * spec". `data-expect-height` below carries that number into the DOM so it can
 * be asserted against getComputedStyle rather than trusted by eye.
 */
import { ref } from 'vue'
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'
import Input from '@/components/common/Input.vue'
import TextArea from '@/components/common/TextArea.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Checkbox from '@/components/common/Checkbox.vue'
import Radio from '@/components/common/Radio.vue'
import Segmented from '@/components/common/Segmented.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import AmountInput from '@/components/payment/AmountInput.vue'
import ModelTagInput from '@/components/admin/channel/ModelTagInput.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import type { AdminGroup, GroupPlatform, Proxy } from '@/types'

const text = ref('sk-inferno-3f9a')
const mono = ref('sk-a1b2c3d4e5f6\nsk-9z8y7x6w5v4u\nsk-m3n4o5p6q7r8')

const theme = ref<'light' | 'dark'>('light')
function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  document.documentElement.classList.toggle('dark', theme.value === 'dark')
}

const brands = ['clay', 'rose', 'sage', 'ocean', 'plum'] as const
function setBrand(b: string) {
  document.documentElement.setAttribute('data-brand', b)
}

const variants = [
  { variant: 'solid', icon: 'hgi-add-01', label: 'New key' },
  { variant: 'secondary', icon: 'hgi-arrow-reload-horizontal', label: 'Refresh' },
  { variant: 'ghost', icon: '', label: 'Cancel' },
  { variant: 'danger', icon: 'hgi-delete-02', label: 'Revoke' },
  { variant: 'success', icon: 'hgi-tick-01', label: 'Approve' },
  { variant: 'warning', icon: 'hgi-pause', label: 'Cooldown' }
] as const

const sizes = [
  { size: 'xs', label: 'Row action', h: 28 },
  { size: 'sm', label: 'Toolbar', h: 32 },
  { size: 'md', label: 'Dialog', h: 36 },
  { size: 'lg', label: 'Sign in', h: 40 }
] as const

const iconSizes = [
  { size: 'xs', h: 28 },
  { size: 'sm', h: 32 },
  { size: 'md', h: 36 }
] as const

// --- 05 Select ---------------------------------------------------------
const selectModel = ref<string | null>('claude-sonnet-4-5')
const selectEmpty = ref<string | null>(null)
const selectCleared = ref<string | null>('claude-opus-4-5')
const selectGrouped = ref<string | null>('gpt-5')
const selectCreatable = ref<string | null>(null)

const modelOptions = [
  { value: 'claude-sonnet-4-5', label: 'claude-sonnet-4-5' },
  { value: 'claude-opus-4-5', label: 'claude-opus-4-5' },
  { value: 'claude-haiku-4-5', label: 'claude-haiku-4-5' },
  { value: 'gpt-5', label: 'gpt-5' }
]

// A group header row is inert (kind: 'group'); each needs a value distinct
// from every other option so Select's `${type}:${value}` v-for key stays
// unique across the three headers below.
const groupedModelOptions = [
  { value: '__group_anthropic', label: 'Anthropic', kind: 'group' },
  { value: 'claude-opus-4-5', label: 'claude-opus-4-5' },
  { value: 'claude-sonnet-4-5', label: 'claude-sonnet-4-5' },
  { value: '__group_openai', label: 'OpenAI', kind: 'group' },
  { value: 'gpt-5', label: 'gpt-5' },
  { value: 'gpt-5-mini', label: 'gpt-5-mini' },
  { value: '__group_gemini', label: 'Gemini', kind: 'group' },
  { value: 'gemini-2.5-pro', label: 'gemini-2.5-pro' },
  { value: 'gemini-2.5-flash', label: 'gemini-2.5-flash' }
]

// --- 06 Toggle -----------------------------------------------------------
const toggleOn = ref(true)
const toggleOff = ref(false)
const toggleDisabledOn = ref(true)
const toggleDisabledOff = ref(false)

// --- 07 Checkbox -----------------------------------------------------------
const checkBare = ref(true)
const checkUnchecked = ref(false)
const checkChecked = ref(true)
const checkDisabledOff = ref(false)
const checkDisabledOn = ref(true)

// --- 08 Radio -----------------------------------------------------------
const radioPlan = ref<'starter' | 'pro' | 'enterprise'>('pro')
const radioDisabledPlan = ref<'starter' | 'pro'>('starter')

// --- 09 Segmented ---------------------------------------------------------
const segmentedMetric = ref('requests')
const segmentedItems = [
  { value: 'requests', label: 'Requests' },
  { value: 'errors', label: 'Errors' },
  { value: 'latency', label: 'Latency', disabled: true }
]
const tabsView = ref('overview')
const tabItems = [
  { value: 'overview', label: 'Overview' },
  { value: 'accounts', label: 'Accounts' },
  { value: 'groups', label: 'Groups' },
  { value: 'proxies', label: 'Proxies' },
  { value: 'billing', label: 'Billing' },
  { value: 'audit-log', label: 'Audit log' }
]

// --- 10 SearchInput --------------------------------------------------------
const searchIdle = ref('')
const searchWithResults = ref('claude')

// --- 11 DateRangePicker ------------------------------------------------
// Today is 2026-08-09 in this environment, so this pair matches the
// component's own "last7Days" preset and the trigger names it accordingly.
const rangeLast7Start = ref('2026-08-03')
const rangeLast7End = ref('2026-08-09')
const rangeCustomStart = ref('2026-07-01')
const rangeCustomEnd = ref('2026-07-15')
const rangeEmptyStart = ref('')
const rangeEmptyEnd = ref('')

// --- 12 GroupSelector -----------------------------------------------------
// AdminGroup carries ~45 billing/routing fields the specimen board has no
// use for; this factory fills them with inert defaults so only the fields
// GroupSelector actually reads (name, platform, rate_multiplier,
// account_count, rate_limited_account_count, description) need overriding.
function makeGroup(overrides: Partial<AdminGroup> & { id: number; name: string; platform: GroupPlatform }): AdminGroup {
  return {
    description: null,
    rate_multiplier: 1,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'standard',
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    long_context_pricing_enabled: false,
    force_openai_fast: false,
    model_pricing: [],
    allow_image_generation: false,
    allow_batch_image_generation: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    batch_image_discount_multiplier: 1,
    batch_image_hold_multiplier: 1,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    video_price_480p: null,
    video_price_720p: null,
    video_price_1080p: null,
    web_search_price_per_call: null,
    search_price_per_1k: null,
    audio_realtime_price_per_min: null,
    audio_tts_price_per_million_chars: null,
    audio_stt_price_per_hour: null,
    peak_rate_enabled: false,
    peak_start: '00:00',
    peak_end: '00:00',
    peak_rate_multiplier: 1,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    allow_live: false,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    profit_control_enabled: false,
    profit_min_margin: 0,
    profit_safety_buffer: 0,
    model_routing: null,
    model_routing_enabled: false,
    mcp_xml_inject: false,
    sort_order: 0,
    account_count: 0,
    rate_limited_account_count: 0,
    ...overrides
  }
}

const specimenGroups: AdminGroup[] = [
  makeGroup({ id: 1, name: 'Claude power users', platform: 'anthropic', description: 'High-volume Claude Code seats', rate_multiplier: 1.2, account_count: 42, rate_limited_account_count: 3 }),
  makeGroup({ id: 2, name: 'OpenAI standard', platform: 'openai', account_count: 18, rate_limited_account_count: 15 }),
  makeGroup({ id: 3, name: 'Gemini trial', platform: 'gemini', rate_multiplier: 0.8, account_count: 0 }),
  makeGroup({ id: 4, name: 'Antigravity mixed', platform: 'antigravity', rate_multiplier: 1.5, account_count: 9, rate_limited_account_count: 1 }),
  makeGroup({ id: 5, name: 'Grok early access', platform: 'grok', account_count: 4 }),
  makeGroup({ id: 6, name: 'Composite fallback', platform: 'composite', account_count: 12, rate_limited_account_count: 2 })
]
const groupSelection = ref<number[]>([1, 4])
const groupSelectionEmpty = ref<number[]>([])

// --- 13 ProxySelector -----------------------------------------------------
function makeProxy(overrides: Partial<Proxy> & { id: number; name: string; host: string }): Proxy {
  return {
    protocol: 'http',
    port: 8080,
    username: null,
    status: 'active',
    expires_at: null,
    fallback_mode: 'none',
    expiry_warn_days: 7,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides
  }
}

const specimenProxies: Proxy[] = [
  makeProxy({ id: 1, name: 'US East residential', host: 'proxy-us-east.oc-net.io', port: 8443, protocol: 'https', region: 'Virginia', country: 'United States' }),
  makeProxy({ id: 2, name: 'EU pool', host: 'proxy-eu-fra.oc-net.io', port: 1080, protocol: 'socks5', city: 'Frankfurt', country: 'Germany' }),
  makeProxy({ id: 3, name: 'APAC backup', host: 'proxy-ap-sgp.oc-net.io', port: 3128, protocol: 'http', country: 'Singapore' })
]
const proxySelected = ref<number | null>(1)
const proxyDirect = ref<number | null>(null)

// --- 14 AmountInput ---------------------------------------------------
const amountPreset = ref<number | null>(50)
const amountCustomValid = ref<number | null>(75)
const amountCustomInvalid = ref<number | null>(9000)
const amountEmpty = ref<number | null>(null)

// --- 15 ModelTagInput -------------------------------------------------
const anthropicModels = ref(['claude-opus-4-5', 'claude-sonnet-4-5', 'claude-haiku-4-5'])
const openaiModels = ref(['gpt-5', 'gpt-5-mini'])
const emptyModels = ref<string[]>([])

// --- 16 BaseDialog -------------------------------------------------------
const dialogNarrowOpen = ref(false)
const dialogNormalOpen = ref(false)
const dialogWideOpen = ref(false)
const dialogExtraWideOpen = ref(false)
const dialogFullOpen = ref(false)

// --- 17 ConfirmDialog ----------------------------------------------------
const confirmOpen = ref(false)
const confirmDangerOpen = ref(false)
</script>

<template>
  <div class="spec">
    <header class="spec__head">
      <div>
        <div class="spec__eyebrow">Specimen, development only</div>
        <h1 class="spec__title">Controls</h1>
      </div>
      <div class="spec__tools">
        <Button variant="secondary" size="xs" :icon="theme === 'dark' ? 'hgi-sun-03' : 'hgi-moon-02'" @click="toggleTheme">
          {{ theme === 'dark' ? 'Light' : 'Dark' }}
        </Button>
        <Button v-for="b in brands" :key="b" variant="ghost" size="xs" @click="setBrand(b)">{{ b }}</Button>
      </div>
    </header>

    <!-- 01 BUTTON -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">01</span>
        <h2 class="spec__h2">Button</h2>
        <span class="spec__src">styles.css .btn family</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__row">
            <Button v-for="v in variants" :key="v.variant" :variant="v.variant" :icon="v.icon">{{ v.label }}</Button>
          </div>

          <div class="spec__row spec__row--divided">
            <Button
              v-for="s in sizes"
              :key="s.size"
              variant="solid"
              :size="s.size"
              :data-expect-height="s.h"
            >{{ s.label }}</Button>
            <span class="spec__measure">28 / 32 / 36 / 40</span>
          </div>

          <div class="spec__row spec__row--divided">
            <div class="spec__state"><Button variant="solid">Create key</Button><span class="spec__label">Rest</span></div>
            <div class="spec__state"><Button variant="solid" class="is-hover">Create key</Button><span class="spec__label">Hover</span></div>
            <div class="spec__state"><Button variant="solid" class="is-focus">Create key</Button><span class="spec__label">Focus</span></div>
            <div class="spec__state"><Button variant="solid" class="is-active">Create key</Button><span class="spec__label">Active</span></div>
            <div class="spec__state"><Button variant="solid" loading>Creating</Button><span class="spec__label">Loading</span></div>
            <div class="spec__state"><Button variant="solid" disabled>Create key</Button><span class="spec__label">Disabled</span></div>
          </div>
        </div>
      </div>
    </section>

    <!-- 02 ICON BUTTON -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">02</span>
        <h2 class="spec__h2">Icon button</h2>
        <span class="spec__src">styles.css .btn-icon</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__groups">
            <div class="spec__group">
              <span class="spec__label">Sizes, 28 / 32 / 36</span>
              <div class="spec__row">
                <IconButton
                  v-for="s in iconSizes"
                  :key="s.size"
                  :size="s.size"
                  icon="hgi-arrow-reload-horizontal"
                  label="Refresh"
                  :data-expect-height="s.h"
                />
              </div>
            </div>
            <div class="spec__group">
              <span class="spec__label">Tones</span>
              <div class="spec__row">
                <IconButton icon="hgi-pencil-edit-02" label="Edit" />
                <IconButton icon="hgi-copy-01" label="Copy" />
                <IconButton icon="hgi-delete-02" label="Revoke" tone="danger" />
                <IconButton icon="hgi-more-horizontal" label="More" />
              </div>
            </div>
            <div class="spec__group">
              <span class="spec__label">Disabled</span>
              <div class="spec__row">
                <IconButton icon="hgi-delete-02" label="Revoke" disabled />
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 03 INPUT -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">03</span>
        <h2 class="spec__h2">Input</h2>
        <span class="spec__src">common/Input.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__grid">
            <!-- No data-expect-height here: Input's root is .fld, the whole
                 label + field + hint group (~85px). The 36px belongs to
                 .fld__input, a descendant with no component boundary to attach
                 to, so the verifier queries that class directly instead. -->
            <Input v-model="text" label="API key name" placeholder="Production key" />
            <Input v-model="text" label="With prefix" placeholder="Search">
              <template #prefix><i class="hgi-stroke hgi-search-01" /></template>
            </Input>
            <Input v-model="text" label="Required" required hint="Shown to nobody but you" />
            <Input v-model="text" label="Error" error="That name is already in use" />
            <Input v-model="text" label="Read only" readonly hint="Real value, not editable" />
            <Input v-model="text" label="Disabled" disabled hint="The control is off" />
          </div>
        </div>
      </div>
    </section>

    <!-- 04 TEXT AREA -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">04</span>
        <h2 class="spec__h2">Text area</h2>
        <span class="spec__src">common/TextArea.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__grid">
            <TextArea v-model="text" label="Note" :rows="3" hint="Vertical resize only" />
            <TextArea v-model="mono" label="Bulk import" variant="mono" :rows="3" hint="One key per line" />
          </div>
        </div>
      </div>
    </section>

    <!-- 05 SELECT -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">05</span>
        <h2 class="spec__h2">Select</h2>
        <span class="spec__src">common/Select.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__grid">
            <div class="spec__state">
              <Select v-model="selectModel" :options="modelOptions" :data-expect-height="36" />
              <span class="spec__label">Rest, value set. Trigger 36</span>
            </div>
            <div class="spec__state">
              <Select v-model="selectEmpty" :options="modelOptions" placeholder="Select a model" />
              <span class="spec__label">Placeholder, no value</span>
            </div>
            <div class="spec__state">
              <Select v-model="selectCleared" :options="modelOptions" clearable />
              <span class="spec__label">Clearable, has value</span>
            </div>
            <div class="spec__state">
              <Select v-model="selectGrouped" :options="groupedModelOptions" searchable />
              <span class="spec__label">Searchable, grouped (9 options)</span>
            </div>
            <div class="spec__state">
              <Select v-model="selectCreatable" :options="modelOptions" creatable creatable-prefix="Add model" />
              <span class="spec__label">Creatable</span>
            </div>
            <div class="spec__state">
              <Select v-model="selectModel" :options="modelOptions" error />
              <span class="spec__label">Error</span>
            </div>
            <div class="spec__state">
              <Select v-model="selectModel" :options="modelOptions" disabled />
              <span class="spec__label">Disabled</span>
            </div>
            <div class="spec__state">
              <Select v-model="selectEmpty" :options="[]" placeholder="No models available" />
              <span class="spec__label">Empty options (open to see "no options found")</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 06 TOGGLE -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">06</span>
        <h2 class="spec__h2">Toggle</h2>
        <span class="spec__src">common/Toggle.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__row">
            <div class="spec__state">
              <!-- Toggle sets inheritAttrs:false and forwards $attrs to its
                   hidden <input> (see Toggle.vue's header comment), so
                   data-expect-height can't reach the visible track by binding
                   it to the component tag. This bare, padding-free wrapper
                   div is sized purely by its one child, so its own computed
                   height/width equal the track's. -->
              <div class="spec__probe" :data-expect-height="20" :data-expect-width="34">
                <Toggle v-model="toggleOff" />
              </div>
              <span class="spec__label">Off. Track 34x20</span>
            </div>
            <div class="spec__state">
              <Toggle v-model="toggleOn" />
              <span class="spec__label">On</span>
            </div>
            <div class="spec__state">
              <Toggle v-model="toggleDisabledOff" disabled />
              <span class="spec__label">Disabled, off</span>
            </div>
            <div class="spec__state">
              <Toggle v-model="toggleDisabledOn" disabled />
              <span class="spec__label">Disabled, on</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 07 CHECKBOX -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">07</span>
        <h2 class="spec__h2">Checkbox</h2>
        <span class="spec__src">common/Checkbox.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__row">
            <div class="spec__state">
              <!-- Same fallthrough issue as Toggle: Checkbox forwards $attrs
                   to its hidden <input>, not the visible box, so the probe
                   wrapper carries the expectation instead. -->
              <div class="spec__probe" :data-expect-height="15">
                <Checkbox v-model="checkBare" />
              </div>
              <span class="spec__label">Bare, no label. Box 15</span>
            </div>
            <div class="spec__state">
              <Checkbox v-model="checkUnchecked">Send usage emails</Checkbox>
              <span class="spec__label">Unchecked</span>
            </div>
            <div class="spec__state">
              <Checkbox v-model="checkChecked">Send usage emails</Checkbox>
              <span class="spec__label">Checked</span>
            </div>
            <div class="spec__state">
              <Checkbox :model-value="false" indeterminate>Select all accounts</Checkbox>
              <span class="spec__label">Indeterminate</span>
            </div>
            <div class="spec__state">
              <Checkbox v-model="checkDisabledOff" disabled>Auto-renew</Checkbox>
              <span class="spec__label">Disabled, unchecked</span>
            </div>
            <div class="spec__state">
              <Checkbox v-model="checkDisabledOn" disabled>Auto-renew</Checkbox>
              <span class="spec__label">Disabled, checked</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 08 RADIO -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">08</span>
        <h2 class="spec__h2">Radio</h2>
        <span class="spec__src">common/Radio.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__groups">
            <div class="spec__group">
              <span class="spec__label">Plan, grouped with notes</span>
              <div class="spec__stack">
                <Radio v-model="radioPlan" value="starter" name="specimen-plan" label="Starter" note="Up to 5 seats, community support" />
                <Radio v-model="radioPlan" value="pro" name="specimen-plan" label="Pro" note="Unlimited seats, priority support" />
                <Radio v-model="radioPlan" value="enterprise" name="specimen-plan" label="Enterprise" note="Dedicated infrastructure, SLA" />
              </div>
            </div>
            <div class="spec__group">
              <span class="spec__label">Disabled</span>
              <div class="spec__stack">
                <Radio v-model="radioDisabledPlan" value="starter" name="specimen-plan-disabled" label="Starter" disabled />
                <Radio v-model="radioDisabledPlan" value="pro" name="specimen-plan-disabled" label="Pro" disabled />
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 09 SEGMENTED -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">09</span>
        <h2 class="spec__h2">Segmented</h2>
        <span class="spec__src">common/Segmented.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__groups">
            <div class="spec__group">
              <span class="spec__label">Segmented, one item disabled. Tray 28</span>
              <Segmented
                v-model="segmentedMetric"
                :items="segmentedItems"
                variant="segmented"
                aria-label="Metric"
                :data-expect-height="28"
              />
            </div>
            <div class="spec__group">
              <span class="spec__label">Tabs, scrolls with overflow</span>
              <div class="spec__narrow">
                <Segmented v-model="tabsView" :items="tabItems" variant="tabs" aria-label="View" />
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 10 SEARCH INPUT -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">10</span>
        <h2 class="spec__h2">Search input</h2>
        <span class="spec__src">common/SearchInput.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__grid">
            <div class="spec__state">
              <SearchInput v-model="searchIdle" placeholder="Search accounts..." />
              <span class="spec__label">Idle, empty. Field 32</span>
            </div>
            <div class="spec__state">
              <SearchInput v-model="searchWithResults" placeholder="Search accounts..." result-text="5 of 96 accounts" />
              <span class="spec__label">Has value, with result count and clear button</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 11 DATE RANGE PICKER -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">11</span>
        <h2 class="spec__h2">Date range picker</h2>
        <span class="spec__src">common/DateRangePicker.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__grid">
            <div class="spec__state">
              <DateRangePicker
                v-model:start-date="rangeLast7Start"
                v-model:end-date="rangeLast7End"
                :data-expect-height="32"
              />
              <span class="spec__label">Preset match, last 7 days. Trigger 32</span>
            </div>
            <div class="spec__state">
              <DateRangePicker v-model:start-date="rangeCustomStart" v-model:end-date="rangeCustomEnd" />
              <span class="spec__label">Custom range</span>
            </div>
            <div class="spec__state">
              <DateRangePicker v-model:start-date="rangeEmptyStart" v-model:end-date="rangeEmptyEnd" />
              <span class="spec__label">Empty</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 12 GROUP SELECTOR -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">12</span>
        <h2 class="spec__h2">Group selector</h2>
        <span class="spec__src">common/GroupSelector.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__grid">
            <div class="spec__state">
              <GroupSelector v-model="groupSelection" :groups="specimenGroups" />
              <span class="spec__label">Searchable, 6 groups, 2 selected, capacity bars</span>
            </div>
            <div class="spec__state">
              <GroupSelector v-model="groupSelectionEmpty" :groups="specimenGroups.slice(0, 3)" />
              <span class="spec__label">Few groups, no search, none selected</span>
            </div>
            <div class="spec__state">
              <GroupSelector v-model="groupSelectionEmpty" :groups="specimenGroups" platform="anthropic" />
              <span class="spec__label">Platform filtered to anthropic, hint footer</span>
            </div>
            <div class="spec__state">
              <GroupSelector v-model="groupSelectionEmpty" :groups="[]" />
              <span class="spec__label">Empty</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 13 PROXY SELECTOR -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">13</span>
        <h2 class="spec__h2">Proxy selector</h2>
        <span class="spec__src">common/ProxySelector.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__grid">
            <div class="spec__state">
              <ProxySelector v-model="proxySelected" :proxies="specimenProxies" />
              <span class="spec__label">Proxy selected</span>
            </div>
            <div class="spec__state">
              <ProxySelector v-model="proxyDirect" :proxies="specimenProxies" />
              <span class="spec__label">Direct, no proxy selected</span>
            </div>
            <div class="spec__state">
              <ProxySelector v-model="proxyDirect" :proxies="specimenProxies" disabled />
              <span class="spec__label">Disabled</span>
            </div>
            <div class="spec__state">
              <ProxySelector v-model="proxyDirect" :proxies="[]" />
              <span class="spec__label">Empty</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 14 AMOUNT INPUT -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">14</span>
        <h2 class="spec__h2">Amount input</h2>
        <span class="spec__src">payment/AmountInput.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__grid">
            <div class="spec__state">
              <AmountInput v-model="amountPreset" :min="10" :max="2000" />
              <span class="spec__label">Preset chip selected. Chip 32, custom field 40</span>
            </div>
            <div class="spec__state">
              <AmountInput v-model="amountCustomValid" :min="10" :max="2000" />
              <span class="spec__label">Custom amount, in range</span>
            </div>
            <div class="spec__state">
              <AmountInput v-model="amountCustomInvalid" :min="10" :max="2000" />
              <span class="spec__label">Custom amount, over max (error)</span>
            </div>
            <div class="spec__state">
              <AmountInput v-model="amountEmpty" />
              <span class="spec__label">Empty, no min or max</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 15 MODEL TAG INPUT -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">15</span>
        <h2 class="spec__h2">Model tag input</h2>
        <span class="spec__src">admin/channel/ModelTagInput.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__grid">
            <div class="spec__state">
              <ModelTagInput v-model:models="anthropicModels" platform="anthropic" placeholder="Add a model id" />
              <span class="spec__label">Anthropic, 3 tags. Wrapper min-height 36, tag 24, dot 5</span>
            </div>
            <div class="spec__state">
              <ModelTagInput v-model:models="openaiModels" platform="openai" placeholder="Add a model id" />
              <span class="spec__label">OpenAI, 2 tags</span>
            </div>
            <div class="spec__state">
              <ModelTagInput v-model:models="emptyModels" platform="gemini" placeholder="Applies to every model in the channel" />
              <span class="spec__label">Empty, placeholder</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 16 BASE DIALOG -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">16</span>
        <h2 class="spec__h2">Base dialog</h2>
        <span class="spec__src">common/BaseDialog.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__row">
            <Button variant="secondary" size="sm" @click="dialogNarrowOpen = true">Open narrow</Button>
            <Button variant="secondary" size="sm" @click="dialogNormalOpen = true">Open normal</Button>
            <Button variant="secondary" size="sm" @click="dialogWideOpen = true">Open wide</Button>
            <Button variant="secondary" size="sm" @click="dialogExtraWideOpen = true">Open extra-wide</Button>
            <Button variant="secondary" size="sm" @click="dialogFullOpen = true">Open full</Button>
            <span class="spec__measure">420 / 520 / 720 / 960 (full 1120, header 44, close 28, footer 52)</span>
          </div>
        </div>
      </div>

      <BaseDialog :show="dialogNarrowOpen" title="Revoke API key" width="narrow" @close="dialogNarrowOpen = false">
        <p>This immediately invalidates <code>sk-inferno-3f9a</code>. Any request using it starts failing right away.</p>
        <template #footer>
          <Button variant="ghost" size="sm" @click="dialogNarrowOpen = false">Cancel</Button>
          <Button variant="danger" size="sm" @click="dialogNarrowOpen = false">Revoke</Button>
        </template>
      </BaseDialog>

      <BaseDialog :show="dialogNormalOpen" title="Edit group: OpenAI standard" width="normal" @close="dialogNormalOpen = false">
        <div class="spec__grid">
          <Input :model-value="'OpenAI standard'" label="Group name" />
          <div>
            <span class="spec__label">Platform</span>
            <Select :model-value="'openai'" :options="[{ value: 'openai', label: 'OpenAI' }]" />
          </div>
        </div>
        <template #footer>
          <Button variant="ghost" size="sm" @click="dialogNormalOpen = false">Cancel</Button>
          <Button variant="solid" size="sm" @click="dialogNormalOpen = false">Save</Button>
        </template>
      </BaseDialog>

      <BaseDialog :show="dialogWideOpen" title="Account routing preview" width="wide" @close="dialogWideOpen = false">
        <p>Two groups currently route requests for this channel.</p>
        <GroupSelector v-model="groupSelection" :groups="specimenGroups" />
        <template #footer>
          <Button variant="ghost" size="sm" @click="dialogWideOpen = false">Close</Button>
        </template>
      </BaseDialog>

      <BaseDialog :show="dialogExtraWideOpen" title="Bulk import accounts" width="extra-wide" @close="dialogExtraWideOpen = false">
        <TextArea :model-value="mono" label="Paste API keys" variant="mono" :rows="6" hint="One key per line" />
        <template #footer>
          <Button variant="ghost" size="sm" @click="dialogExtraWideOpen = false">Cancel</Button>
          <Button variant="solid" size="sm" @click="dialogExtraWideOpen = false">Import</Button>
        </template>
      </BaseDialog>

      <BaseDialog :show="dialogFullOpen" title="Request detail" width="full" @close="dialogFullOpen = false">
        <TextArea
          :model-value="'{\n  &quot;model&quot;: &quot;claude-sonnet-4-5&quot;,\n  &quot;status&quot;: 200,\n  &quot;latency_ms&quot;: 842\n}'"
          variant="mono"
          :rows="8"
          readonly
        />
        <template #footer>
          <Button variant="ghost" size="sm" @click="dialogFullOpen = false">Close</Button>
        </template>
      </BaseDialog>
    </section>

    <!-- 17 CONFIRM DIALOG -->
    <section class="spec__section">
      <div class="spec__sectionHead">
        <span class="spec__num">17</span>
        <h2 class="spec__h2">Confirm dialog</h2>
        <span class="spec__src">common/ConfirmDialog.vue</span>
      </div>

      <div class="spec__card">
        <div class="spec__stage">
          <div class="spec__row">
            <Button variant="secondary" size="sm" @click="confirmOpen = true">Open confirm</Button>
            <Button variant="secondary" size="sm" @click="confirmDangerOpen = true">Open danger confirm</Button>
          </div>
        </div>
      </div>

      <ConfirmDialog
        :show="confirmOpen"
        title="Apply group changes"
        message="Claude power users moves from a 1.1x to a 1.2x rate multiplier. This applies to every account in the group immediately."
        confirm-text="Apply"
        @confirm="confirmOpen = false"
        @cancel="confirmOpen = false"
      />

      <ConfirmDialog
        :show="confirmDangerOpen"
        title="Revoke this account"
        message="This immediately signs the account out everywhere and stops it from serving traffic. This cannot be undone."
        confirm-text="Revoke"
        danger
        @confirm="confirmDangerOpen = false"
        @cancel="confirmDangerOpen = false"
      />
    </section>
  </div>
</template>

<style scoped>
.spec {
  padding: 44px 40px 96px;
  max-width: 1120px;
  background: var(--background);
  min-height: 100vh;
}
.spec__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}
.spec__eyebrow {
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
.spec__title {
  margin: 6px 0 0;
  font-family: var(--font-serif);
  font-size: var(--fs-display);
  font-weight: 400;
  line-height: 1.1;
  letter-spacing: -0.01em;
}
.spec__tools {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.spec__section {
  margin-top: 44px;
}
.spec__sectionHead {
  display: flex;
  align-items: baseline;
  gap: 10px;
  flex-wrap: wrap;
}
.spec__num {
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
.spec__h2 {
  margin: 0;
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
}
.spec__src {
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
  color: var(--muted-foreground);
}
.spec__card {
  margin-top: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
  overflow: hidden;
}
.spec__stage {
  padding: 22px;
  background: var(--surface-subtle);
}
.spec__row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}
.spec__row--divided {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid var(--border-subtle);
}
.spec__state {
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: flex-start;
}
.spec__label {
  font-size: var(--fs-2xs);
  color: var(--muted-foreground);
}
.spec__measure {
  margin-left: 6px;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
.spec__groups {
  display: flex;
  flex-wrap: wrap;
  gap: 24px;
}
.spec__group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.spec__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 18px;
}
.spec__stack {
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: flex-start;
}
.spec__narrow {
  max-width: 260px;
}
/* Bare measurement wrapper: no padding/border/margin of its own, so it
   auto-sizes to exactly its one child and can carry a data-expect-* that the
   child's own component would route to a different node (see Toggle and
   Checkbox usages above). */
.spec__probe {
  /* flex, not inline-block: an inline-block's content forms an inline
     formatting context, which pads its measured height with leftover
     line-height around the child (the classic "phantom gap under an
     inline-block" quirk) and would throw this probe's own reading off by a
     few px. A flex container has no such line box. */
  display: flex;
}

/* Force the pseudo states so a screenshot can show all six side by side, the
   way the prototype does. Specimen-only: never ship these. */
.spec :deep(.is-hover[data-variant='solid']) {
  background: color-mix(in oklch, var(--primary) 92%, var(--foreground));
}
.spec :deep(.is-active[data-variant='solid']) {
  background: color-mix(in oklch, var(--primary) 84%, var(--foreground));
}
.spec :deep(.is-focus) {
  box-shadow: 0 0 0 3px var(--focus-ring);
}
</style>
