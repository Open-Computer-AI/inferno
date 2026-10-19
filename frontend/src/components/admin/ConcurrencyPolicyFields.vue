<template>
  <div
    class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600"
    data-test="concurrency-policy-fields"
  >
    <div>
      <label class="input-label" for="concurrency-policy-mode">
        {{ t('admin.concurrencyPolicy.mode') }}
      </label>
      <select
        id="concurrency-policy-mode"
        class="input"
        data-test="concurrency-mode"
        :value="mode"
        :disabled="disabled"
        @change="updateMode"
      >
        <option value="shared">{{ t('admin.concurrencyPolicy.shared') }}</option>
        <option value="split">{{ t('admin.concurrencyPolicy.split') }}</option>
      </select>
    </div>

    <p class="input-hint">{{ t('admin.concurrencyPolicy.hint') }}</p>

    <div v-if="mode === 'split'" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div>
        <label class="input-label" for="live-concurrency-input">
          {{ t('admin.concurrencyPolicy.liveLimit') }}
        </label>
        <input
          id="live-concurrency-input"
          class="input"
          data-test="live-concurrency-input"
          type="number"
          min="1"
          :max="MAX_CONCURRENCY"
          step="1"
          inputmode="numeric"
          :value="live"
          :disabled="disabled"
          :aria-invalid="liveInvalid"
          @input="updateLive"
        />
        <p v-if="liveInvalid" class="input-hint text-red-600 dark:text-red-400" data-test="live-concurrency-error">
          {{ t('admin.concurrencyPolicy.positiveInteger', { max: MAX_CONCURRENCY }) }}
        </p>
      </div>

      <div>
        <label class="input-label" for="request-concurrency-input">
          {{ t('admin.concurrencyPolicy.requestsLimit') }}
        </label>
        <input
          id="request-concurrency-input"
          class="input"
          data-test="request-concurrency-input"
          type="number"
          min="1"
          :max="MAX_CONCURRENCY"
          step="1"
          inputmode="numeric"
          :value="requests"
          :disabled="disabled"
          :aria-invalid="requestInvalid"
          @input="updateRequests"
        />
        <p v-if="requestInvalid" class="input-hint text-red-600 dark:text-red-400" data-test="request-concurrency-error">
          {{ t('admin.concurrencyPolicy.positiveInteger', { max: MAX_CONCURRENCY }) }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ConcurrencyMode } from '@/types'
import {
  isPositiveConcurrencyLimit,
  MAX_CONCURRENCY,
  type ConcurrencyLimitInput
} from '@/utils/concurrencyPolicy'

const props = withDefaults(defineProps<{
  mode: ConcurrencyMode
  live: ConcurrencyLimitInput
  requests: ConcurrencyLimitInput
  disabled?: boolean
}>(), {
  disabled: false
})

const emit = defineEmits<{
  'update:mode': [value: ConcurrencyMode]
  'update:live': [value: string]
  'update:requests': [value: string]
}>()

const { t } = useI18n()

const liveInvalid = computed(() =>
  props.mode === 'split' && !isPositiveConcurrencyLimit(props.live)
)
const requestInvalid = computed(() =>
  props.mode === 'split' && !isPositiveConcurrencyLimit(props.requests)
)

const updateMode = (event: Event) => {
  const value = (event.target as HTMLSelectElement).value
  emit('update:mode', value === 'split' ? 'split' : 'shared')
}

const updateLive = (event: Event) => {
  emit('update:live', (event.target as HTMLInputElement).value)
}

const updateRequests = (event: Event) => {
  emit('update:requests', (event.target as HTMLInputElement).value)
}
</script>
