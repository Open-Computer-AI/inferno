<template>
  <div class="space-y-1 text-xs" data-test="split-concurrency-summary">
    <div class="flex items-center justify-between gap-3">
      <span class="font-medium text-gray-600 dark:text-gray-300">{{ t('admin.concurrencyPolicy.live') }}</span>
      <span data-test="live-concurrency-summary">{{ formatPair(liveCurrent, liveLimit) }}</span>
    </div>
    <div class="flex items-center justify-between gap-3">
      <span class="font-medium text-gray-600 dark:text-gray-300">{{ t('admin.concurrencyPolicy.requests') }}</span>
      <span data-test="request-concurrency-summary">{{ formatPair(requestCurrent, requestLimit) }}</span>
    </div>
    <div class="flex items-center justify-between gap-3 border-t border-gray-200 pt-1 dark:border-dark-600">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.concurrencyPolicy.aggregate') }}</span>
      <span data-test="aggregate-concurrency-summary">
        {{ t('admin.concurrencyPolicy.aggregateValue', { value: formatValue(aggregateCurrent) }) }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  liveCurrent?: number | null
  liveLimit?: number | null
  requestCurrent?: number | null
  requestLimit?: number | null
  aggregateCurrent?: number | null
}>()

const { t } = useI18n()

const formatValue = (value: number | null | undefined) =>
  value == null ? t('admin.concurrencyPolicy.unknown') : String(value)

const formatPair = (current: number | null | undefined, limit: number | null | undefined) =>
  formatValue(current) + ' / ' + formatValue(limit)
</script>
