<template>
  <div class="mt-5 grid grid-cols-2 gap-2">
    <div
      class="rounded-xl p-3 bg-[var(--brand-tint)/80] bg-[var(--brand-tint)/40] border border-[var(--brand-line)] border-[var(--brand-line)/50]"
    >
      <div
        class="flex items-center gap-1.5 text-[10px] font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)]"
      >
        <Icon :name="primaryIcon" size="xs" />
        <span>{{ primaryLabel }}</span>
      </div>
      <div class="mt-1.5 text-lg font-[var(--fw-medium)] font-mono tabular-nums text-[var(--foreground)] text-[var(--muted-foreground)]">
        {{ primaryValue }}<span class="text-xs font-normal text-[var(--muted-foreground)] ml-0.5">{{ primaryUnit }}</span>
      </div>
    </div>
    <div
      class="rounded-xl p-3 bg-[var(--brand-tint)/80] bg-[var(--brand-tint)/40] border border-[var(--brand-line)] border-[var(--brand-line)/50]"
    >
      <div
        class="flex items-center gap-1.5 text-[10px] font-[var(--fw-medium)]  tracking-wider text-[var(--muted-foreground)]"
      >
        <Icon :name="secondaryIcon" size="xs" />
        <span>{{ secondaryLabel }}</span>
      </div>
      <div class="mt-1.5 text-lg font-[var(--fw-medium)] font-mono tabular-nums text-[var(--foreground)] text-[var(--muted-foreground)]">
        {{ secondaryValue }}<span class="text-xs font-normal text-[var(--muted-foreground)] ml-0.5">{{ secondaryUnit }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  primaryLabel: string
  primaryValue: string
  primaryUnit: string
  primaryIcon: 'bolt' | 'globe' | 'clock' | 'link'
  secondaryLabel: string
  secondaryValue: string
  secondaryUnit: string
  secondaryIcon: 'bolt' | 'globe' | 'clock' | 'link'
}>()
</script>
